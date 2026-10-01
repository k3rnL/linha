package linha.worker

import io.circe.Json
import io.circe.syntax._
import linha._
import linha.json._
import java.io.{FilterOutputStream, OutputStream}
import java.nio.file.{Files, Path}
import java.util.concurrent.{
  ConcurrentHashMap,
  Executors,
  ScheduledExecutorService,
  ScheduledFuture,
  TimeUnit
}
import java.util.concurrent.atomic.{AtomicBoolean, AtomicLong, AtomicReference}
import scala.collection.JavaConverters._
import scala.collection.mutable
import scala.util.control.NonFatal

final case class WorkerIdentity(contextId: String, id: String, incarnation: String, expiresAt: Long)
final case class WorkerCredential(identity: WorkerIdentity, token: String)
final case class RetryableJobException(message: String) extends RuntimeException(message)

final class LinhaWorker[E](
    baseUrl: String,
    credential: WorkerCredential,
    makeEngine: String => E,
    cancelEngine: String => Unit = (_: String) => (),
    releaseEngine: E => Unit = (_: E) => (),
    capacity: Int = 1,
    credentials: Option[CredentialProvider] = None
) extends AutoCloseable {
  require(capacity > 0 && capacity <= 128)
  private val transport = new Transport(
    baseUrl,
    credentials.getOrElse(CredentialProvider(() => credential.token)),
    Map(
      "X-Linha-Context-Id" -> credential.identity.contextId,
      "X-Linha-Worker-Id" -> credential.identity.id,
      "X-Linha-Worker-Incarnation" -> credential.identity.incarnation
    )
  )
  private val registry = mutable.LinkedHashMap
    .empty[(String, Int), (Capability, (Json, LinhaJobContext[E]) => ProducedResult)]
  private val executor = Executors.newFixedThreadPool(capacity)
  private val maintenance = Executors.newScheduledThreadPool(capacity + 1)
  private val watchdog = Executors.newSingleThreadScheduledExecutor()
  private val cancellations = Executors.newFixedThreadPool(math.min(capacity, 4))
  private val stopped = new AtomicBoolean(false)
  private val draining = new AtomicBoolean(false)
  private val active = new ConcurrentHashMap[String, Running]()
  private final class Running(
      val id: String,
      val update: Json,
      val leaseMillis: Long,
      initialBudget: Long
  ) {
    val engineKey = update.hcursor.get[String]("attemptId").fold(throw _, identity)
    val cancelled = new AtomicBoolean(false)
    val deadline = new AtomicLong(System.nanoTime() + math.max(0L, initialBudget) * 1000000L)
    val thread = new AtomicReference[Thread]()
    def cancel(): Unit = if (cancelled.compareAndSet(false, true)) {
      Option(thread.get()).foreach(_.interrupt())
      try
        cancellations.submit(new Runnable {
          def run(): Unit = {
            try cancelEngine(engineKey)
            catch { case NonFatal(_) => () }
          }
        })
      catch { case _: java.util.concurrent.RejectedExecutionException => () }
    }
  }
  def register[J <: LinhaEntrypoint[R, E], R](codec: EntrypointCodec[J, R]): this.type = {
    handleRaw(codec.capability) { (payload, ctx) =>
      val entrypoint = codec.requestDecoder.decodeJson(payload).fold(throw _, identity)
      codec.encodeResult(entrypoint.execute(ctx))
    }
  }
  def handleRaw(
      capability: Capability
  )(handler: (Json, LinhaJobContext[E]) => ProducedResult): this.type = {
    require(!registry.contains((capability.handler, capability.version)), "duplicate handler")
    registry((capability.handler, capability.version)) = (capability, handler)
    this
  }
  def run(): Unit = {
    require(registry.nonEmpty, "register at least one handler")
    val id = credential.identity
    transport.call(
      "POST",
      "/v1/workers/register",
      Json.obj(
        "id" -> id.id.asJson,
        "contextId" -> id.contextId.asJson,
        "incarnation" -> id.incarnation.asJson,
        "capacity" -> capacity.asJson,
        "capabilities" -> registry.values.map(_._1).toVector.asJson
      )
    )
    maintenance.scheduleAtFixedRate(
      new Runnable {
        def run(): Unit = {
          try
            draining.set(
              transport
                .call("POST", "/v1/workers/heartbeat")
                .hcursor
                .get[Boolean]("draining")
                .getOrElse(false)
            )
          catch { case NonFatal(_) => () }
        }
      },
      0,
      5,
      TimeUnit.SECONDS
    )
    watchdog.scheduleAtFixedRate(
      new Runnable {
        def run(): Unit = {
          active.values().asScala.foreach { task =>
            if (System.nanoTime() >= task.deadline.get()) task.cancel()
          }
        }
      },
      0,
      100,
      TimeUnit.MILLISECONDS
    )
    while (!stopped.get()) {
      if (draining.get() || active.size() >= capacity) Thread.sleep(100)
      else {
        val started = System.nanoTime()
        try {
          val assignment = transport.call("POST", "/v1/workers/claim?waitSeconds=2")
          if (!assignment.isNull) {
            val c = assignment.hcursor
            val jobId = c.downField("job").get[String]("id").fold(throw _, identity)
            val attempt = c.get[String]("attemptId").fold(throw _, identity)
            val fence = c.get[Long]("fence").fold(throw _, identity)
            val lease = c.get[Long]("leaseDurationMillis").fold(throw _, identity)
            val elapsed = (System.nanoTime() - started) / 1000000L
            val update = Json.obj(
              "workerId" -> id.id.asJson,
              "incarnation" -> id.incarnation.asJson,
              "attemptId" -> attempt.asJson,
              "fence" -> fence.asJson
            )
            val running =
              new Running(jobId, update, lease, lease - elapsed - math.min(1000L, lease / 10))
            active.put(jobId, running)
            executor.submit(new Runnable {
              def run(): Unit = {
                try execute(assignment, running)
                catch {
                  case NonFatal(error) =>
                    logFailure(running, "initialization", "unexpected attempt failure", error)
                } finally active.remove(jobId)
              }
            })
          }
        } catch { case NonFatal(_) => Thread.sleep(250) }
      }
    }
  }
  private def execute(assignment: Json, task: Running): Unit = {
    task.thread.set(Thread.currentThread())
    val path = "/v1/jobs/" + task.id
    var phase = "initialization"
    var temp: Path = null
    var renew: ScheduledFuture[_] = null
    val cleanup = mutable.ArrayBuffer.empty[() => Unit]
    var engineInstance: Option[E] = None
    try {
      val c = assignment.hcursor
      val request = c.downField("job").downField("request")
      val handlerId = request.get[String]("handler").fold(throw _, identity)
      val version = request.get[Int]("version").fold(throw _, identity)
      val (_, handler) = registry((handlerId, version))
      val descriptor = c.get[ResultDescriptor]("resultDescriptor").fold(throw _, identity)
      val maxBytes = c.downField("results").get[Long]("maxBytes").fold(throw _, identity)
      temp = Files.createTempDirectory("linha-attempt-")
      renew = maintenance.scheduleAtFixedRate(
        new Runnable {
          def run(): Unit = {
            val start = System.nanoTime()
            if (!task.cancelled.get()) try {
              transport.call("POST", path + "/renew", task.update)
              val elapsed = (System.nanoTime() - start) / 1000000L
              task.deadline.set(
                System.nanoTime() + math.max(
                  0L,
                  task.leaseMillis - elapsed - math.min(1000L, task.leaseMillis / 10)
                ) * 1000000L
              )
            } catch {
              case e: LinhaException if e.status == 409 || e.status == 401 => task.cancel()
              case NonFatal(_) => if (System.nanoTime() >= task.deadline.get()) task.cancel()
            }
          }
        },
        math.max(100L, task.leaseMillis / 3),
        math.max(100L, task.leaseMillis / 3),
        TimeUnit.MILLISECONDS
      )
      def check(): Unit = if (task.cancelled.get() || System.nanoTime() >= task.deadline.get())
        throw new InterruptedException("attempt cancelled or lease lost")
      def produce(name: String, contentType: String, kind: String)(
          write: Path => Unit
      ): ResultFile = {
        check()
        val allocation = transport.call(
          "POST",
          path + "/outputs",
          task.update.deepMerge(
            Json.obj(
              "name" -> name.asJson,
              "kind" -> kind.asJson,
              "contentType" -> contentType.asJson
            )
          )
        )
        val releaseStaging = StagingBudget.reserve(maxBytes)
        val file =
          try Files.createTempFile(temp, "result-", ".tmp")
          catch { case t: Throwable => releaseStaging(); throw t }
        preservingFailure {
          write(file); check()
          if (Files.size(file) > maxBytes)
            throw LinhaException(413, "RESULT_TOO_LARGE", "result staging limit exceeded")
          val update = task.update.hcursor
          transport
            .upload(
              allocation.hcursor.get[String]("uploadUrl").fold(throw _, identity),
              file,
              update.get[String]("attemptId").fold(throw _, identity),
              update.get[Long]("fence").fold(throw _, identity)
            )
            .as[ResultFile]
            .fold(throw _, identity)
        }(
          try Files.deleteIfExists(file)
          finally releaseStaging()
        )
      }
      val writer = new ResultWriter {
        override def dataset(name: String, format: String)(
            write: DatasetLocation => Iterator[DatasetPart]
        ): DatasetResult = {
          check()
          val allocation = transport.call(
            "POST",
            path + "/outputs",
            task.update.deepMerge(
              Json.obj(
                "name" -> name.asJson,
                "kind" -> "dataset".asJson,
                "contentType" -> "application/x-parquet".asJson
              )
            )
          )
          val output =
            path + "/outputs/" + allocation.hcursor.get[String]("id").fold(throw _, identity)
          val location = transport
            .call("POST", output + "/access", task.update)
            .as[DatasetLocation]
            .fold(throw _, identity)
          val parts = write(location)
          var bytes = 0L
          var count = 0L
          parts.grouped(100).foreach { batch =>
            check()
            batch.foreach { p =>
              if (p.size < 0 || p.size > maxBytes - bytes)
                throw LinhaException(413, "RESULT_TOO_LARGE", "dataset byte limit exceeded")
              bytes += p.size; count += 1
              if (count > 100000)
                throw LinhaException(413, "RESULT_TOO_LARGE", "dataset part limit exceeded")
            }
            transport.call(
              "POST",
              output + "/parts",
              task.update.deepMerge(Json.obj("parts" -> batch.asJson))
            )
          }
          check()
          val manifest = transport
            .call(
              "POST",
              output + "/seal",
              task.update.deepMerge(Json.obj("format" -> format.asJson))
            )
            .as[DatasetManifest]
            .fold(throw _, identity)
          DatasetResult(Vector(manifest.manifest), Some(manifest))
        }
        def file(name: String, contentType: String)(write: Path => Unit): FileResult = FileResult(
          Vector(produce(name, contentType, "file")(write))
        )
        def stream(name: String, contentType: String)(write: OutputStream => Unit): FileResult =
          file(name, contentType) { path =>
            val output = new FilterOutputStream(Files.newOutputStream(path)) {
              private var count = 0L
              override def write(value: Int): Unit = {
                if (count >= maxBytes)
                  throw LinhaException(413, "RESULT_TOO_LARGE", "stream limit exceeded");
                out.write(value); count += 1
              }
              override def write(bytes: Array[Byte], off: Int, length: Int): Unit = {
                if (length > maxBytes - count)
                  throw LinhaException(413, "RESULT_TOO_LARGE", "stream limit exceeded");
                out.write(bytes, off, length); count += length
              }
            }
            preservingFailure(write(output))(output.close())
          }
      }
      phase = "engine"
      check(); engineInstance = Some(makeEngine(task.engineKey))
      val ctx = new LinhaJobContext[E] {
        def jobId: String = task.id
        def attemptId: String = task.update.hcursor.get[String]("attemptId").fold(throw _, identity)
        def engine: E = engineValue
        private val engineValue = engineInstance.get
        def progress(fraction: Double, message: String): Unit = {
          check();
          transport.call(
            "POST",
            path + "/renew",
            task.update.deepMerge(
              Json.obj(
                "progress" -> Json.obj("fraction" -> fraction.asJson, "message" -> message.asJson)
              )
            )
          )
        }
        def isCancelled: Boolean = task.cancelled.get() || System.nanoTime() >= task.deadline.get()
        def checkCancelled(): Unit = check()
        def results: ResultWriter = writer
        def onCleanup(action: () => Unit): Unit = cleanup += action
      }
      phase = "handler"
      val produced = handler(request.get[Json]("payload").fold(throw _, identity), ctx)
      check()
      phase = "result"
      val files = produced match {
        case JsonResult(value) =>
          Vector(
            produce("result.json", "application/json", "json")(path =>
              Files.write(path, value.noSpaces.getBytes(java.nio.charset.StandardCharsets.UTF_8))
            )
          )
        case ArtifactResult(files)  => files
        case DatasetProduced(value) => value.files
      }
      check()
      phase = "completion"
      val completion =
        task.update.deepMerge(Json.obj("descriptor" -> descriptor.asJson, "files" -> files.asJson))
      val body = produced match {
        case DatasetProduced(value) =>
          completion.deepMerge(Json.obj("dataset" -> value.dataset.asJson))
        case _ => completion
      }
      transport.call("POST", path + "/complete", body)
    } catch {
      case failure: Throwable =>
        // Clear interruption long enough to attempt the authoritative failure/cancel acknowledgement.
        Thread.interrupted()
        val code =
          if (stopped.get()) "WORKER_STOPPED"
          else if (FailureDiagnostics.outOfMemory(failure)) "OUT_OF_MEMORY"
          else
            failure match {
              case e: LinhaException              => e.code
              case _: InterruptedException        => "CANCELLED"
              case _ if phase == "initialization" => "INITIALIZATION_FAILED"
              case _ if phase == "engine"         => "ENGINE_INITIALIZATION_FAILED"
              case _ if phase == "result"         => "RESULT_FAILED"
              case _ if phase == "completion"     => "RESULT_PUBLICATION_FAILED"
              case _                              => "HANDLER_FAILED"
            }
        val request = assignment.hcursor.downField("job").downField("request")
        val handler = request.get[String]("handler").getOrElse("unknown")
        val version = request.get[Int]("version").getOrElse(0)
        logFailure(task, phase, s"code=$code handler=$handler version=$version", failure)
        val retryable =
          stopped.get() || failure.isInstanceOf[RetryableJobException] || (failure match {
            case e: LinhaException => e.status >= 500 || e.status == 429
            case _                 => false
          })
        val diagnostics = FailureDiagnostics.capture(failure, code, retryable, phase)
        try
          transport.call(
            "POST",
            path + "/fail",
            task.update.deepMerge(Json.obj("failure" -> diagnostics))
          )
        catch {
          case reportError: InterruptedException =>
            logFailure(task, "failure-report", "failure report was not acknowledged", reportError)
            Thread.currentThread().interrupt()
          case NonFatal(reportError) =>
            logFailure(task, "failure-report", "failure report was not acknowledged", reportError)
        }
        if (!NonFatal(failure) && !failure.isInstanceOf[InterruptedException]) throw failure
    } finally {
      if (renew != null) renew.cancel(false)
      cleanup.reverseIterator.foreach(action =>
        try action()
        catch { case NonFatal(error) => logFailure(task, "cleanup", "cleanup hook failed", error) }
      )
      engineInstance.foreach(value =>
        try releaseEngine(value)
        catch {
          case NonFatal(error) => logFailure(task, "cleanup", "engine release failed", error)
        }
      )
      if (temp != null)
        try Files.deleteIfExists(temp)
        catch {
          case NonFatal(error) =>
            logFailure(task, "cleanup", "temporary directory cleanup failed", error)
        }
      active.remove(task.id)
    }
  }
  private def preservingFailure[A](body: => A)(cleanup: => Unit): A = {
    var primary: Throwable = null
    try body
    catch { case error: Throwable => primary = error; throw error }
    finally {
      if (primary == null) cleanup
      else
        try cleanup
        catch { case error: Throwable => if (error ne primary) primary.addSuppressed(error) }
    }
  }
  private def logFailure(task: Running, phase: String, detail: String, error: Throwable): Unit =
    FailureDiagnostics.log(
      s"Linha jobId=${task.id} attemptId=${task.engineKey} workerId=${credential.identity.id} phase=$phase $detail",
      error
    )
  def close(): Unit = {
    stopped.set(true); active.values().asScala.foreach(_.cancel())
    executor.shutdown(); executor.awaitTermination(10, TimeUnit.SECONDS)
    watchdog.shutdownNow(); maintenance.shutdownNow(); cancellations.shutdownNow();
    executor.shutdownNow()
  }
}
object LinhaWorker {
  def projectedIdentity(): WorkerCredential = {
    val uid = sys.env("LINHA_POD_UID")
    WorkerCredential(WorkerIdentity(sys.env("LINHA_CONTEXT_ID"), uid, uid, 0L), "")
  }
  def projectedCredentials(): CredentialProvider = if (
    sys.env.get("LINHA_SECURITY_ENABLED").contains("false")
  ) CredentialProvider.none
  else
    CredentialProvider(() =>
      new String(
        Files.readAllBytes(java.nio.file.Paths.get(sys.env("LINHA_WORKER_TOKEN_FILE"))),
        java.nio.charset.StandardCharsets.UTF_8
      ).trim
    )

  def local(baseUrl: String, credential: WorkerCredential, capacity: Int = 1): LinhaWorker[Unit] =
    new LinhaWorker[Unit](baseUrl, credential, _ => (), capacity = capacity)
}
