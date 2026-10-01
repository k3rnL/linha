package linha.spark

import linha._
import org.apache.hadoop.conf.Configuration
import org.apache.hadoop.fs.{FileSystem, Path}
import org.apache.spark.sql.DataFrame
import java.net.URI
import java.security.MessageDigest

private[spark] object DistributedResults {
  def parquet(
      writer: ResultWriter,
      frame: DataFrame,
      name: String,
      partitions: Seq[String]
  ): DatasetResult = {
    require(
      partitions.distinct.size == partitions.size && partitions.forall(frame.columns.contains),
      "invalid partition columns"
    )
    writer.dataset(name, "parquet") { location =>
      val options = location.hadoopOptions
      val hadoop = new Configuration(frame.sparkSession.sparkContext.hadoopConfiguration)
      options.foreach { case (key, value) => hadoop.set(key, value) }
      // A separate FileSystem instance prevents credentials leaking between concurrent requests.
      val uri = URI.create(location.uri)
      val fs = FileSystem.newInstance(uri, hadoop)
      val target = new Path(uri)
      try {
        location.probePath.foreach { probe =>
          val expected = location.probeValue.getOrElse(
            throw new IllegalArgumentException("missing local mount probe")
          )
          // Every writing partition validates the same server-created mount marker.
          // A worker-local directory with the same pathname must not pass this check.
          frame.rdd
            .mapPartitions { rows =>
              val bytes = java.nio.file.Files.readAllBytes(java.nio.file.Paths.get(probe))
              require(
                new String(bytes, java.nio.charset.StandardCharsets.UTF_8) == expected,
                "RESULT_STORAGE_UNAVAILABLE: shared result mount mismatch"
              )
              Iterator(1)
            }
            .count()
        }
        val session = frame.sparkSession
        val keys = Vector(
          "spark.sql.sources.commitProtocolClass",
          "spark.sql.parquet.output.committer.class"
        )
        val old = keys.map(k => k -> session.conf.getOption(k))
        try {
          if (uri.getScheme == "s3a") {
            Class.forName("org.apache.hadoop.fs.s3a.S3AFileSystem")
            Class.forName("org.apache.spark.internal.io.cloud.PathOutputCommitProtocol")
            session.conf.set(keys(0), "org.apache.spark.internal.io.cloud.PathOutputCommitProtocol")
            session.conf.set(
              keys(1),
              "org.apache.spark.internal.io.cloud.BindingParquetOutputCommitter"
            )
          }
          val output = frame.write.options(options).mode("errorifexists")
          if (partitions.nonEmpty) output.partitionBy(partitions: _*).parquet(location.uri)
          else output.parquet(location.uri)
        } finally
          old.foreach { case (key, value) =>
            value match {
              case Some(v) => session.conf.set(key, v); case None => session.conf.unset(key)
            }
          }
        // Metadata enumeration is bounded; data never becomes a collected DataFrame.
        // Hash each part incrementally, closing each stream before moving to the next.
        val listed = fs.listFiles(target, true)
        val parts = scala.collection.mutable.ArrayBuffer.empty[DatasetPart]
        while (listed.hasNext) {
          val status = listed.next()
          val filename = status.getPath.getName
          if (filename.startsWith("part-") && filename.endsWith(".parquet")) {
            if (parts.size >= 100000)
              throw LinhaException(413, "RESULT_TOO_LARGE", "dataset part limit exceeded")
            val relative =
              status.getPath.toUri.getPath.stripPrefix(target.toUri.getPath.stripSuffix("/") + "/")
            val digest = MessageDigest.getInstance("SHA-256")
            val input = fs.open(status.getPath)
            try {
              val buffer = new Array[Byte](65536)
              var n = input.read(buffer)
              while (n >= 0) { if (n > 0) digest.update(buffer, 0, n); n = input.read(buffer) }
            } finally input.close()
            parts += DatasetPart(
              relative,
              status.getLen,
              digest.digest().map(b => f"${b & 0xff}%02x").mkString
            )
          }
        }
        parts.iterator
      } finally fs.close()
    }
  }
}
