package linha.client

import io.circe.Json
import io.circe.syntax._
import linha._
import linha.json._
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Path}
import java.util.UUID
import scala.concurrent.{ExecutionContext, Future, blocking}

/** Own this client for the API process lifetime; close contexts when no longer used. */
final class LinhaClient(baseUrl: String, credentials: CredentialProvider)(implicit
    ec: ExecutionContext
) extends AutoCloseable {
  private[client] val transport = new Transport(baseUrl, credentials)
  private val lifecycle = new Transport(baseUrl, credentials, timeoutSeconds = 5, retries = 0)
  private val clientId = UUID.randomUUID().toString
  private val hostname = scala.util
    .Try(sys.env.get("HOSTNAME"))
    .toOption
    .flatten
    .filter(_.nonEmpty)
    .orElse(scala.util.Try(java.net.InetAddress.getLocalHost.getHostName).toOption)
    .filter(h =>
      h.getBytes(StandardCharsets.UTF_8).length <= 253 && !h
        .exists(c => c.isWhitespace || c.isControl)
    )
    .getOrElse("")
  private val clientMetadata =
    Json.obj("clientId" -> clientId.asJson, "hostname" -> hostname.asJson)
  private val attachments = scala.collection.mutable.Map.empty[String, Attachment]
  private var closed = false
  private val scheduler = java.util.concurrent.Executors.newScheduledThreadPool(
    2,
    new java.util.concurrent.ThreadFactory {
      def newThread(r: Runnable): Thread = {
        val t = new Thread(r, "linha-client-lease"); t.setDaemon(true); t
      }
    }
  )
  private val shutdown =
    new Thread(new Runnable { def run(): Unit = close() }, "linha-client-shutdown")
  Runtime.getRuntime.addShutdownHook(shutdown)
  private[client] final class Attachment(val id: String, val version: String, initial: Json) {
    var references = 0 // protected by the client monitor
    private var lease =
      initial.hcursor.downField("clientLease").get[String]("id").fold(throw _, identity)
    private var stopped = false
    private val seconds =
      initial.hcursor.downField("clientLease").get[Long]("durationSeconds").getOrElse(90L)
    private val path = "/v1/contexts/" + segment(id)
    private def acquire(): Unit = {
      val json = lifecycle.call("POST", path + "/clients", clientMetadata)
      lease = json.hcursor.downField("clientLease").get[String]("id").fold(throw _, identity)
    }
    private def renew(): Unit = synchronized {
      if (!stopped) {
        try lifecycle.call("POST", path + "/clients/" + segment(lease) + "/renew")
        catch { case e: LinhaException if e.code == "CLIENT_LEASE_EXPIRED" => acquire() }
      }
    }
    private val renewal = scheduler.scheduleWithFixedDelay(
      new Runnable {
        def run(): Unit = try renew()
        catch {
          case scala.util.control.NonFatal(_) => ()
        } // retry next heartbeat; server expiry is authoritative
      },
      math.max(1L, seconds / 3),
      math.max(1L, seconds / 3),
      java.util.concurrent.TimeUnit.SECONDS
    )
    def refresh(json: Json): Unit = synchronized {
      lease = json.hcursor.downField("clientLease").get[String]("id").fold(throw _, identity)
    }
    def submit(body: Json): Json = synchronized {
      require(!stopped, "LinhaContext is closed")
      def send(): Json =
        transport.call("POST", path + "/jobs", body, Map("X-Linha-Client-Lease" -> lease))
      try send()
      catch { case e: LinhaException if e.code == "CLIENT_LEASE_EXPIRED" => acquire(); send() }
    }
    def stop(): Unit = synchronized {
      if (!stopped) {
        stopped = true; renewal.cancel(false)
        try lifecycle.call("DELETE", path + "/clients/" + segment(lease))
        catch { case scala.util.control.NonFatal(_) => () }
      }
    }
  }
  private def handle(json: Json): LinhaContext = {
    val id = json.hcursor.get[String]("id").fold(throw _, identity)
    val attachment = attachments.getOrElseUpdate(
      id,
      new Attachment(id, json.hcursor.get[String]("version").fold(throw _, identity), json)
    )
    attachment.refresh(json)
    attachment.references += 1
    new LinhaContext(id, attachment.version, this, attachment)
  }
  def ensureBackend(name: String, spec: Json): Future[LinhaContext] = Future(blocking {
    synchronized {
      require(!closed, "LinhaClient is closed")
      handle(
        transport.call(
          "POST",
          "/v1/contexts:ensure",
          Json.obj("name" -> name.asJson, "spec" -> spec).deepMerge(clientMetadata)
        )
      )
    }
  })
  def restoreContext(id: String): Future[LinhaContext] = Future(blocking {
    synchronized {
      require(!closed, "LinhaClient is closed")
      handle(
        lifecycle.call(
          "POST",
          "/v1/contexts/" + segment(id) + "/clients",
          clientMetadata
        )
      )
    }
  })
  private[client] def release(attachment: LinhaClient#Attachment): Unit = synchronized {
    attachments.get(attachment.id).filter(_ eq attachment).foreach { current =>
      current.references -= 1
      if (current.references == 0) { attachments.remove(current.id); current.stop() }
    }
  }
  override def close(): Unit = synchronized {
    if (!closed) {
      closed = true; scheduler.shutdownNow()
      attachments.values.foreach(_.stop()); attachments.clear()
      try Runtime.getRuntime.removeShutdownHook(shutdown)
      catch { case _: IllegalStateException => () }
    }
  }
  def job[J, R](id: String, codec: EntrypointCodec[J, R]): JobHandle[R] =
    new JobHandle(id, codec.descriptor, codec.resultDecoder, this)
  def rawJob(id: String): RawJobHandle = new RawJobHandle(id, this)
  def list(
      contextId: String = "",
      state: String = "",
      cursor: String = "",
      limit: Int = 50
  ): Future[Json] = Future(blocking {
    transport.call(
      "GET",
      s"/v1/jobs?contextId=${segment(contextId)}&state=${segment(state)}&cursor=${segment(cursor)}&limit=$limit"
    )
  })
  private[client] def segment(s: String): String =
    URLEncoder.encode(s, StandardCharsets.UTF_8.name())
}
final class LinhaContext private[client] (
    val id: String,
    val version: String,
    client: LinhaClient,
    attachment: LinhaClient#Attachment
)(implicit ec: ExecutionContext)
    extends AutoCloseable {
  private val closed = new java.util.concurrent.atomic.AtomicBoolean(false)
  override def close(): Unit = if (closed.compareAndSet(false, true)) client.release(attachment)
  def status(): Future[Json] = Future(
    blocking(client.transport.call("GET", "/v1/contexts/" + client.segment(id)))
  )
  def submitRaw(
      request: RawRequest,
      idempotencyKey: String = UUID.randomUUID().toString,
      maxAttempts: Int = 1
  ): Future[RawJobHandle] = Future(blocking {
    require(!closed.get(), "LinhaContext is closed")
    val body = Json.obj(
      "handler" -> request.handler.asJson,
      "version" -> request.version.asJson,
      "payload" -> request.payload,
      "idempotencyKey" -> idempotencyKey.asJson,
      "retry" -> Json.obj("maxAttempts" -> maxAttempts.asJson, "backoffSeconds" -> 1.asJson)
    )
    val response = attachment.submit(body)
    client.rawJob(response.hcursor.get[String]("id").fold(throw _, identity))
  })
  def submit[J, R](
      entrypoint: J,
      idempotencyKey: String = UUID.randomUUID().toString,
      maxAttempts: Int = 1
  )(implicit codec: EntrypointCodec[J, R]): Future[JobHandle[R]] =
    submitRaw(
      RawRequest(codec.id, codec.version, codec.requestEncoder(entrypoint)),
      idempotencyKey,
      maxAttempts
    ).map(j => client.job(j.id, codec))
  def developmentWorker(): Future[Json] = Future(
    blocking(client.transport.call("POST", "/v1/contexts/" + client.segment(id) + "/dev-workers"))
  )
}
class RawJobHandle(val id: String, private[client] val client: LinhaClient)(implicit
    ec: ExecutionContext
) {
  private[client] val path = "/v1/jobs/" + client.segment(id)
  def status(): Future[Json] = Future(blocking(client.transport.call("GET", path)))
  def attempts(): Future[Json] = Future(blocking(client.transport.call("GET", path + "/attempts")))
  def cancel(): Future[Json] = Future(blocking(client.transport.call("POST", path + ":cancel")))
  def resultMetadata(): Future[Json] = Future(
    blocking(client.transport.call("GET", path + "/result"))
  )
  def parts(cursor: String = "", limit: Int = 100): Future[Json] = Future(blocking {
    client.transport
      .call("GET", path + "/parts?cursor=" + client.segment(cursor) + "&limit=" + limit)
  })
  def downloadPart(part: String, destination: Path): Future[Path] = Future(blocking {
    val input = client.transport.open(path + "/part?path=" + client.segment(part))
    try Files.copy(input, destination)
    finally input.close()
    destination
  })
  def download(allocationId: String, destination: Path): Future[Path] = Future(blocking {
    val input = client.transport.open(path + "/files/" + client.segment(allocationId))
    try Files.copy(input, destination)
    finally input.close()
    destination
  })
}
final class JobHandle[R](
    id: String,
    expected: ResultDescriptor,
    decoder: io.circe.Decoder[R],
    client: LinhaClient
)(implicit ec: ExecutionContext)
    extends RawJobHandle(id, client) {
  def result(): Future[R] = resultMetadata().map { metadata =>
    blocking {
      val actual = metadata.hcursor.get[ResultDescriptor]("descriptor").fold(throw _, identity)
      if (actual != expected)
        throw LinhaException(409, "SCHEMA_MISMATCH", "stored result does not match this handle")
      val files = metadata.hcursor.get[Vector[ResultFile]]("files").fold(throw _, identity)
      val json = if (expected.kind == "json") {
        require(files.size == 1, "JSON result must have one file")
        val input =
          client.transport.open(path + "/files/" + client.segment(files.head.allocationId))
        try {
          val bytes = input.readNBytes(64 * 1024 * 1024 + 1)
          if (bytes.length > 64 * 1024 * 1024)
            throw LinhaException(413, "RESULT_TOO_LARGE", "use streaming download for large values")
          io.circe.parser.parse(new String(bytes, StandardCharsets.UTF_8)).fold(throw _, identity)
        } finally input.close()
      } else
        Json.obj(
          "files" -> files.asJson,
          "dataset" -> metadata.hcursor.get[Json]("dataset").getOrElse(Json.Null)
        )
      decoder.decodeJson(json).fold(throw _, identity)
    }
  }
}
object ResultStorage {
  def Local(root: String): Json = local(root)
  def S3(
      destination: String,
      path: ResultPath = ResultPath.template(ResultPath.defaultTemplate)
  ): Json = s3(destination, path.template)

  def local(root: String): Json = Json.obj("type" -> "local".asJson, "root" -> root.asJson)
  def s3(destination: String, path: String): Json = Json.obj(
    "type" -> "s3".asJson,
    "destination" -> destination.asJson,
    "path" -> Json.obj("version" -> 1.asJson, "template" -> path.asJson)
  )
}
