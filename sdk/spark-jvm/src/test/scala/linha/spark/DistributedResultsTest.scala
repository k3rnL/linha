package linha.spark

import linha._
import linha.spark.syntax._
import org.junit.Test
import org.junit.Assert._
import org.apache.spark.SparkConf
import java.nio.file.{Files, Path}
import java.io.OutputStream

class DistributedResultsTest {
  @Test def partitionedParquetAndMissingMount(): Unit = {
    val runtime = new SparkRuntime(
      new SparkConf()
        .setMaster("local[2]")
        .setAppName("Linha dataset conformance")
        .set("spark.ui.enabled", "false")
        .set("spark.driver.host", "127.0.0.1"),
      1
    )
    val engine = runtime.open("dataset")
    val root = Files.createTempDirectory("linha-dataset-test-")
    val probe = root.resolve("probe"); Files.write(probe, "allocated".getBytes("UTF-8"))
    var recorded = Vector.empty[DatasetPart]
    var target = root.resolve("output")
    val writer = new ResultWriter {
      def file(name: String, contentType: String)(write: Path => Unit): FileResult =
        throw new UnsupportedOperationException
      def stream(name: String, contentType: String)(write: OutputStream => Unit): FileResult =
        throw new UnsupportedOperationException
      override def dataset(name: String, format: String)(
          write: DatasetLocation => Iterator[DatasetPart]
      ): DatasetResult = {
        recorded = write(
          DatasetLocation(
            "allocated",
            target.toUri.toString,
            Map("fs.file.impl" -> "org.apache.hadoop.fs.RawLocalFileSystem"),
            None,
            Some(probe.toString),
            Some("allocated")
          )
        ).toVector
        DatasetResult(Vector.empty)
      }
    }
    try {
      val frame = engine.session.range(20).selectExpr("id", "id % 2 AS group").repartition(2)
      writer.parquet(frame, "events", Seq("group"))
      assertTrue(recorded.size >= 2)
      assertTrue(
        recorded.forall(p => p.path.startsWith("group=") && p.sha256.length == 64 && p.size > 0)
      )
      assertEquals(20L, engine.session.read.parquet(target.toString).count())
      target = root.resolve("missing-output"); Files.delete(probe)
      try { writer.parquet(frame, "missing"); fail("missing shared mount accepted") }
      catch { case _: org.apache.spark.SparkException => () }
      assertFalse(Files.exists(target))
    } finally {
      runtime.release(engine); runtime.close()
      val paths = Files.walk(root);
      try {
        import scala.collection.JavaConverters._;
        paths.iterator.asScala.toVector.reverse.foreach(Files.delete)
      } finally paths.close()
    }
  }
}
