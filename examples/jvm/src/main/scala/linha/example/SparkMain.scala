package linha.example

import io.circe.parser.decode
import io.circe.syntax._
import linha._
import linha.json._
import linha.spark._
import linha.spark.syntax._
import linha.worker.LinhaWorker
import org.apache.spark.SparkConf

final case class CountRows(size: Long, delayMillis: Long = 0L, cacheAndBroadcast: Boolean = false)
    extends SparkEntrypoint[RowsCount] {
  def execute(ctx: LinhaJobContext[Spark]): RowsCount = {
    require(size >= 0 && size <= 1000000000L && delayMillis >= 0 && delayMillis <= 300000)
    ctx.progress(0.1, "Counting rows")
    val delay = delayMillis // Copy only data into Spark executor closures.
    val frame = ctx.spark.range(size).repartition(4).toDF()
    val input = if (cacheAndBroadcast) ctx.cache(frame) else frame
    val marker = if (cacheAndBroadcast) Some(ctx.broadcast(1L)) else None
    val count = input.rdd
      .mapPartitions { values =>
        if (delay > 0) Thread.sleep(delay)
        Iterator(values.size.toLong * marker.map(_.value).getOrElse(1L))
      }
      .sum()
      .toLong
    ctx.progress(1.0, "Count complete")
    RowsCount(count)
  }
}
object CountRows {
  implicit val decoder: io.circe.Decoder[CountRows] = io.circe.Decoder.instance { c =>
    for {
      size <- c.get[Long]("size")
      delay <- c.get[Option[Long]]("delayMillis")
      cache <- c.get[Option[Boolean]]("cacheAndBroadcast")
    } yield CountRows(size, delay.getOrElse(0L), cache.getOrElse(false))
  }
}
final case class RowsCount(value: Long)
object SparkMain {
  val exportCodec = EntrypointCodec.derived[ExportRows, DatasetResult]("export-rows", 1)
  val countCodec = EntrypointCodec.derived[CountRows, RowsCount]("count-rows", 1)
  def main(args: Array[String]): Unit = {
    val conf = new SparkConf()
    decode[Map[String, String]](sys.env("LINHA_SPARK_CONF")).fold(throw _, identity).foreach {
      case (key, value) => conf.set(key, value)
    }
    val runtime = new SparkRuntime(conf, sys.env("LINHA_CAPACITY").toInt)
    val worker = runtime
      .worker(
        sys.env("LINHA_SERVER_URL"),
        LinhaWorker.projectedIdentity(),
        Some(LinhaWorker.projectedCredentials())
      )
      .register(countCodec)
      .register(exportCodec)
      .handleRaw(Capability("raw-count", 1, ResultDescriptor("json", "raw-count.result", 1))) {
        (payload, ctx) =>
          val size = payload.hcursor.get[Long]("size").fold(throw _, identity)
          require(size >= 0 && size <= 1000000000L)
          JsonResult(io.circe.Json.obj("value" -> ctx.spark.range(size).count().asJson))
      }
    sys.addShutdownHook { worker.close(); runtime.close() }
    try worker.run()
    finally { worker.close(); runtime.close() }
  }
}

final case class ExportRows(size: Long, fail: Boolean = false)
    extends SparkEntrypoint[DatasetResult] {
  def execute(ctx: LinhaJobContext[Spark]): DatasetResult = {
    import org.apache.spark.sql.functions.col
    val frame = ctx.spark.range(size).withColumn("group", col("id") % 2).repartition(4)
    if (fail) {
      val broken = frame.rdd.mapPartitions { rows =>
        if (org.apache.spark.TaskContext.getPartitionId() == 1)
          throw new IllegalStateException("injected executor failure"); rows
      }
      ctx.results.parquet(ctx.spark.createDataFrame(broken, frame.schema), "events", Seq("group"))
    } else ctx.results.parquet(frame, "events", Seq("group"))
  }
}

object ExportRows {
  implicit val decoder: io.circe.Decoder[ExportRows] = io.circe.Decoder.instance { c =>
    for { size <- c.get[Long]("size"); fail <- c.get[Option[Boolean]]("fail") } yield ExportRows(
      size,
      fail.getOrElse(false)
    )
  }
}
