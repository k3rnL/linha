package linha.hadoop

import org.junit.Test
import org.junit.Assert._
import org.junit.Assume.assumeTrue
import linha.StorageSession
import io.circe.parser.parse
import org.apache.hadoop.conf.Configuration
import org.apache.hadoop.fs.Path
import java.nio.file.Files

class StorageIntegrationTest {
  @Test def rangeReadsAndSerializedConfiguration(): Unit = {
    assumeTrue("run with disposable S3/PG fixture", sys.env.contains("LINHA_TEST_STORAGE_SESSION"))
    val storage = StorageSession.fromJson(
      sys.env("LINHA_TEST_STORAGE_URL"),
      parse(sys.env("LINHA_TEST_STORAGE_SESSION")).fold(throw _, identity)
    )
    val config = new Configuration(false)
    storage.hadoopOptions.foreach { case (key, value) => config.set(key, value) }
    val source = new Path(storage.path("sdk-io/input.nc"))
    val fs = source.getFileSystem(config)
    try {
      fs.mkdirs(source.getParent)
      val output = fs.create(source, true)
      try output.write(Array.tabulate[Byte](8192)(i => (i % 251).toByte))
      finally output.close()
      // Restore config as Spark does on another executor rather than sharing a stream.
      val copy = new Configuration(false)
      val bytes = new java.io.ByteArrayOutputStream()
      val data = new java.io.DataOutputStream(bytes); config.write(data); data.close()
      copy.readFields(
        new java.io.DataInputStream(new java.io.ByteArrayInputStream(bytes.toByteArray))
      )
      val remote = org.apache.hadoop.fs.FileSystem.newInstance(source.toUri, copy)
      try {
        val in = remote.open(source)
        try {
          in.seek(7900); assertEquals(7900 % 251, in.read()); in.seek(23);
          assertEquals(23, in.read()); assertEquals(24L, in.getPos)
        } finally in.close()
        assertEquals(8192L, remote.getFileStatus(source).getLen)
        assertEquals(1, remote.listStatus(source.getParent).length)
        val target = new Path(source.getParent, "renamed.nc")
        assertTrue(remote.rename(source, target)); assertFalse(remote.exists(source));
        assertTrue(remote.exists(target))
      } finally remote.close()
    } finally { fs.delete(source.getParent, true); fs.close() }
  }
}
