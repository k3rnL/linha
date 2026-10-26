package linha

import io.circe.Json
import java.net.{URI, URLEncoder}
import java.nio.file.Path
import java.io.InputStream
import java.util.concurrent.{ConcurrentHashMap, Executors, TimeUnit}
import java.util.concurrent.atomic.AtomicBoolean
import scala.collection.JavaConverters._

final case class StorageObject(path: String, directory: Boolean, size: Long, modifiedAt: String)

/** Scoped application access. Credentials are never placed in paths or request payloads. */
final class StorageSession private (
    val serverURL: String,
    val id: String,
    private val token: String,
    val destination: String,
    val root: String
) extends AutoCloseable {
  private val transport =
    new Transport(serverURL, CredentialProvider.static(token), timeoutSeconds = 600)
  private val held = new ConcurrentHashMap[String, StorageLock]()
  val uri: String = "linha://" + id + "/"
  override def toString: String = s"StorageSession($destination, $id)"
  def hadoopOptions: Map[String, String] = Map(
    "fs.linha.impl" -> "linha.hadoop.LinhaFileSystem",
    "fs.linha.impl.disable.cache" -> "true",
    s"fs.linha.session.$id.server" -> serverURL,
    s"fs.linha.session.$id.token" -> token,
    s"fs.linha.session.$id.root" -> root,
    s"fs.linha.session.$id.destination" -> destination
  )
  def relative(value: String): String = {
    val parsed = URI.create(value.replace(" ", "%20"))
    val name = if (parsed.getScheme == "linha") {
      require(parsed.getAuthority == id, "storage session mismatch")
      parsed.getPath.stripPrefix("/")
    } else if (Set("s3", "s3a")(Option(parsed.getScheme).getOrElse(""))) {
      val base = URI.create(root)
      require(parsed.getAuthority == base.getAuthority, "storage bucket mismatch")
      val prefix = base.getPath.stripSuffix("/") + "/"
      require(
        parsed.getPath == base.getPath || parsed.getPath.startsWith(prefix),
        "path is outside storage destination"
      )
      if (parsed.getPath == base.getPath) "" else parsed.getPath.stripPrefix(prefix)
    } else { require(parsed.getScheme == null, "unsupported storage URI"); value.stripPrefix("/") }
    require(
      !name.split("/", -1).exists(p => p == "." || p == ".." || p.exists(_.isControl)),
      "invalid storage path"
    )
    name.stripSuffix("/")
  }
  def path(value: String): String = new URI("linha", id, "/" + relative(value), null).toString
  private def route(op: String, value: String): String =
    "/v1/storage/sessions/" + id + "/" + op + "?path=" + URLEncoder.encode(relative(value), "UTF-8")
  private def protection(value: String): Map[String, String] = {
    val name = relative(value)
    held.asScala.toVector
      .sortBy(-_._1.length)
      .collectFirst {
        case (key, lock) if name == key || name.startsWith(key + "/") =>
          lock.check(); Map("X-Linha-Storage-Lock" -> key)
      }
      .getOrElse(Map.empty)
  }
  def stat(value: String): StorageObject = {
    val c = transport.call("GET", route("stat", value)).hcursor
    StorageObject(
      c.get[String]("path").getOrElse(""),
      c.get[Boolean]("directory").fold(throw _, identity),
      c.get[Long]("size").fold(throw _, identity),
      c.get[String]("modifiedAt").getOrElse("")
    )
  }
  def list(value: String): Vector[StorageObject] =
    transport
      .call("GET", route("list", value))
      .hcursor
      .get[Vector[Json]]("items")
      .fold(throw _, identity)
      .map { json =>
        val c = json.hcursor
        StorageObject(
          c.get[String]("path").fold(throw _, identity),
          c.get[Boolean]("directory").fold(throw _, identity),
          c.get[Long]("size").fold(throw _, identity),
          c.get[String]("modifiedAt").getOrElse("")
        )
      }
  def open(value: String, offset: Long = 0L): InputStream = {
    require(offset >= 0, "negative offset")
    transport.open(
      route("data", value),
      if (offset == 0) Map.empty else Map("Range" -> s"bytes=$offset-")
    )
  }
  def upload(value: String, source: Path): Unit = {
    transport.put(route("data", value), source, protection(value)); ()
  }
  def mkdir(value: String): Unit = {
    transport.call("POST", route("mkdir", value), extraHeaders = protection(value)); ()
  }
  def delete(value: String, recursive: Boolean = false): Unit = {
    transport.call(
      "DELETE",
      route("delete", value) + "&recursive=" + recursive,
      extraHeaders = protection(value)
    ); ()
  }
  def rename(source: String, target: String): Unit = {
    transport.call(
      "POST",
      route("rename", source) + "&target=" + URLEncoder.encode(relative(target), "UTF-8"),
      extraHeaders = protection(source)
    ); ()
  }
  def lock(value: String, waitSeconds: Long = 120): StorageLock = synchronized {
    val key = relative(value)
    require(key.nonEmpty && !held.containsKey(key), "lock is already held or names the root")
    val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(waitSeconds)
    var acquired = false
    while (!acquired) {
      try { transport.call("POST", route("lock", key)); acquired = true }
      catch {
        case e: LinhaException if e.status == 409 && System.nanoTime() < deadline =>
          Thread.sleep(250)
      }
    }
    val lock = new StorageLock(key)
    held.put(key, lock)
    lock
  }
  final class StorageLock private[StorageSession] (key: String) extends AutoCloseable {
    private val valid = new AtomicBoolean(true)
    private val renewal = StorageSession.scheduler.scheduleWithFixedDelay(
      new Runnable {
        def run(): Unit = try { transport.call("POST", route("lock", key) + "&renew=true"); () }
        catch { case scala.util.control.NonFatal(_) => valid.set(false) }
      },
      8,
      8,
      TimeUnit.SECONDS
    )
    def check(): Unit =
      if (!valid.get()) throw LinhaException(409, "STALE_ATTEMPT", "storage lock was lost")
    def close(): Unit = {
      renewal.cancel(false)
      // Keep the failed lock as a local fence until a fresh session is obtained.
      try transport.call("DELETE", route("lock", key))
      finally { valid.set(false); held.remove(key, this) }
    }
  }
  def close(): Unit = {
    var failure: Throwable = null
    def cleanup(action: => Unit): Unit = try action
    catch {
      case scala.util.control.NonFatal(error) =>
        if (failure == null) failure = error else failure.addSuppressed(error)
    }
    try {
      held.values().asScala.foreach(lock => cleanup(lock.close()))
      cleanup { transport.call("DELETE", "/v1/storage/sessions/" + id + "/close"); () }
    } finally StorageSession.sessions.remove(id, this)
    if (failure != null) throw failure
  }
}
object StorageSession {
  private val sessions = new ConcurrentHashMap[String, StorageSession]()
  private[linha] val scheduler = Executors.newScheduledThreadPool(
    1,
    new java.util.concurrent.ThreadFactory {
      def newThread(r: Runnable): Thread = {
        val t = new Thread(r, "linha-storage-lock"); t.setDaemon(true); t
      }
    }
  )
  def fromJson(server: String, json: Json): StorageSession = {
    val c = json.hcursor
    restore(
      server,
      c.get[String]("id").fold(throw _, identity),
      c.get[String]("token").fold(throw _, identity),
      c.get[String]("destination").fold(throw _, identity),
      c.get[String]("root").fold(throw _, identity)
    )
  }
  def restore(
      server: String,
      id: String,
      token: String,
      destination: String,
      root: String
  ): StorageSession = {
    val fresh = new StorageSession(server, id, token, destination, root)
    val existing = sessions.putIfAbsent(id, fresh)
    if (existing == null) fresh
    else {
      require(
        existing.token == token && existing.serverURL == server && existing.root == root,
        "storage session configuration mismatch"
      ); existing
    }
  }
}
