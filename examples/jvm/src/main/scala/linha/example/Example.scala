package linha.example

import linha._
import linha.json._
import linha.worker._
import io.circe.syntax._
import io.circe.parser.parse
import java.nio.file.{Files, Paths}

final case class SumResult(value: Long)
final case class SumNumbers(numbers: Vector[Long]) extends LinhaEntrypoint[SumResult, Unit] {
  def execute(ctx: LinhaJobContext[Unit]): SumResult = {
    ctx.progress(0.5, "Summing values"); SumResult(numbers.sum)
  }
}
object SumNumbers {
  implicit val codec: EntrypointCodec[SumNumbers, SumResult] =
    EntrypointCodec.derived("example.sum", 1)
}
final case class WriteSummary(text: String) extends LinhaEntrypoint[FileResult, Unit] {
  def execute(ctx: LinhaJobContext[Unit]): FileResult =
    ctx.results.stream("summary.txt", "text/plain")(out => out.write(text.getBytes("UTF-8")))
}
object WriteSummary {
  implicit val codec: EntrypointCodec[WriteSummary, FileResult] =
    EntrypointCodec.derived("example.summary", 1)
}
final case class FailAfterWriting() extends LinhaEntrypoint[FileResult, Unit] {
  def execute(ctx: LinhaJobContext[Unit]): FileResult =
    ctx.results.file("partial.txt", "text/plain") { path =>
      Files.write(path, "partial bytes must not be published".getBytes("UTF-8"))
      val error = new IllegalStateException(
        "failure after local staging",
        new java.io.IOException("example source read failed")
      )
      error.addSuppressed(new IllegalArgumentException("example suppressed detail"))
      throw error
    }
}
object FailAfterWriting {
  implicit val codec: EntrypointCodec[FailAfterWriting, FileResult] =
    EntrypointCodec.derived("example.partial-failure", 1)
}
object WorkerMain {
  def main(args: Array[String]): Unit = {
    require(args.length == 2, "usage: WorkerMain <server-url> <worker-credential-json-file>")
    val credential = parse(new String(Files.readAllBytes(Paths.get(args(1))), "UTF-8"))
      .flatMap(_.as[WorkerCredential])
      .fold(throw _, identity)
    val worker = LinhaWorker
      .local(args(0), credential, capacity = 2)
      .register(SumNumbers.codec)
      .register(WriteSummary.codec)
      .register(FailAfterWriting.codec)
      .handleRaw(
        Capability("example.raw-sum", 1, ResultDescriptor("json", "example.raw-sum.result", 1))
      ) { (payload, ctx) =>
        val numbers = payload.hcursor.get[Vector[Long]]("numbers").fold(throw _, identity)
        ctx.progress(0.5, "Raw JSON dispatch")
        JsonResult(io.circe.Json.obj("value" -> numbers.sum.asJson))
      }
    Runtime.getRuntime.addShutdownHook(new Thread(() => worker.close()))
    worker.run()
  }
}
