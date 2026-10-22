package linha.hadoop

import java.net.URI
import java.io.{FileNotFoundException, IOException, OutputStream}
import java.nio.file.Files
import org.apache.hadoop.conf.Configuration
import org.apache.hadoop.fs._
import org.apache.hadoop.fs.permission.FsPermission
import org.apache.hadoop.util.Progressable
import linha.{LinhaException, StorageSession}
import java.util.concurrent.ConcurrentHashMap

/** S3-backed application files through scoped Linha sessions. Renames copy/delete. */
final class LinhaFileSystem extends FileSystem {
  private var origin: URI = _
  private var cwd: Path = _
  private var session: StorageSession = _
  override def initialize(uri: URI, config: Configuration): Unit = {
    super.initialize(uri, config)
    require(uri.getScheme == "linha" && uri.getAuthority != null, "Linha session URI required")
    origin = URI.create("linha://" + uri.getAuthority + "/")
    cwd = new Path(origin)
    val key = "fs.linha.session." + uri.getAuthority + "."
    session = StorageSession.restore(
      config.get(key + "server"),
      uri.getAuthority,
      config.get(key + "token"),
      config.get(key + "destination"),
      config.get(key + "root")
    )
    LinhaFileSystem.instances.put(uri.getAuthority, new java.lang.ref.WeakReference(this))
  }
  override def getUri: URI = origin
  override def getScheme: String = "linha"
  override def getWorkingDirectory: Path = cwd
  override def setWorkingDirectory(path: Path): Unit = { cwd = path.makeQualified(origin, cwd) }
  private def name(path: Path): String =
    session.relative(path.makeQualified(origin, cwd).toUri.toString)
  private def io[A](action: => A): A = try action
  catch {
    case e: LinhaException if e.status == 404 => throw new FileNotFoundException(e.detail)
    case e: LinhaException                    => throw new IOException(e.code + ": " + e.detail)
  }
  override def getFileStatus(path: Path): FileStatus = io {
    val v = session.stat(name(path))
    val modified = scala.util.Try(java.time.Instant.parse(v.modifiedAt).toEpochMilli).getOrElse(0L)
    new FileStatus(
      v.size,
      v.directory,
      1,
      64L * 1024 * 1024,
      modified,
      path.makeQualified(origin, cwd)
    )
  }
  override def listStatus(path: Path): Array[FileStatus] = io {
    val status = getFileStatus(path)
    if (!status.isDirectory) Array(status)
    else
      session
        .list(name(path))
        .map(v =>
          new FileStatus(
            v.size,
            v.directory,
            1,
            64L * 1024 * 1024,
            scala.util.Try(java.time.Instant.parse(v.modifiedAt).toEpochMilli).getOrElse(0L),
            new Path(session.path(v.path))
          )
        )
        .toArray
  }
  override def open(path: Path, bufferSize: Int): FSDataInputStream = io {
    val key = name(path)
    val status = getFileStatus(path)
    if (status.isDirectory) throw new FileNotFoundException("cannot open a directory")
    new FSDataInputStream(new FSInputStream {
      private var position = 0L
      private var stream: java.io.InputStream = null
      private var closed = false
      override def getPos: Long = synchronized { position }
      override def seek(target: Long): Unit = synchronized {
        if (target < 0) throw new IOException("negative seek")
        if (closed) throw new IOException("stream closed")
        if (position != target) {
          if (stream != null) stream.close(); stream = null; position = target
        }
      }
      override def seekToNewSource(target: Long): Boolean = false
      override def read(): Int = {
        val one = new Array[Byte](1); if (read(one, 0, 1) < 0) -1 else one(0) & 0xff
      }
      override def read(bytes: Array[Byte], offset: Int, length: Int): Int = synchronized {
        io {
          if (closed) throw new IOException("stream closed")
          if (length == 0) 0
          else if (position >= status.getLen) -1
          else {
            if (stream == null) stream = session.open(key, position);
            val n = stream.read(bytes, offset, length); if (n > 0) position += n; n
          }
        }
      }
      override def close(): Unit = synchronized {
        closed = true; if (stream != null) stream.close()
      }
    })
  }
  override def create(
      path: Path,
      permission: FsPermission,
      overwrite: Boolean,
      bufferSize: Int,
      replication: Short,
      blockSize: Long,
      progress: Progressable
  ): FSDataOutputStream = io {
    if (!overwrite && exists(path)) throw new FileAlreadyExistsException(path.toString)
    val key = name(path)
    val temporary = Files.createTempFile("linha-fs-", ".upload")
    val output = Files.newOutputStream(temporary)
    new FSDataOutputStream(
      new OutputStream {
        private var closed = false
        private var failed = false
        private var size = 0L
        private def reserve(length: Int): Unit = {
          if (closed || failed) throw new IOException("output is closed or failed")
          if (length < 0 || size > (5L << 30) - length) {
            failed = true; throw new IOException("object exceeds 5 GiB")
          }
          size += length
        }
        override def write(value: Int): Unit = { reserve(1); output.write(value) }
        override def write(bytes: Array[Byte], offset: Int, length: Int): Unit = {
          reserve(length); output.write(bytes, offset, length)
        }
        override def flush(): Unit = output.flush()
        override def close(): Unit = if (!closed) {
          closed = true
          try { output.close(); if (!failed) io(session.upload(key, temporary)) }
          finally Files.deleteIfExists(temporary)
        }
      },
      statistics
    )
  }
  override def append(path: Path, bufferSize: Int, progress: Progressable): FSDataOutputStream =
    throw new IOException("append is unsupported")
  override def mkdirs(path: Path, permission: FsPermission): Boolean = io {
    session.mkdir(name(path)); true
  }
  override def delete(path: Path, recursive: Boolean): Boolean = io {
    if (!exists(path)) false else { session.delete(name(path), recursive); true }
  }
  override def rename(source: Path, destination: Path): Boolean = io {
    if (!exists(source)) false
    else {
      val target =
        if (exists(destination) && getFileStatus(destination).isDirectory)
          new Path(destination, source.getName)
        else destination
      if (exists(target)) false else { session.rename(name(source), name(target)); true }
    }
  }
}
object LinhaFileSystem {
  private val instances =
    new ConcurrentHashMap[String, java.lang.ref.WeakReference[LinhaFileSystem]]()

  /** For libraries that open a URI without accepting a Hadoop Configuration. */
  def lookup(uri: URI): LinhaFileSystem = Option(instances.get(uri.getAuthority))
    .flatMap(ref => Option(ref.get()))
    .getOrElse(throw new IOException("Linha filesystem has not been initialized for this session"))
}
