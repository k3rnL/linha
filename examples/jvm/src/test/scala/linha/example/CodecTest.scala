package linha.example
import org.junit.Test
import org.junit.Assert._
import linha._
import linha.json._
import linha.client._
import scala.concurrent.ExecutionContext.Implicits.global
class CodecTest {
  @Test def roundTripUsesParametersOnly(): Unit = {
    val codec = SumNumbers.codec
    val encoded = codec.requestEncoder(SumNumbers(Vector(1, 2, 3)))
    assertEquals("{\"numbers\":[1,2,3]}", encoded.noSpaces)
    assertEquals(Right(SumNumbers(Vector(1, 2, 3))), codec.requestDecoder.decodeJson(encoded))
    assertEquals("json", codec.descriptor.kind)
    assertEquals("file", WriteSummary.codec.descriptor.kind)
  }
  @Test def rawSparkArgumentsKeepOptionalDefaults(): Unit = {
    val raw = io.circe.Json.obj("size" -> io.circe.Json.fromLong(42))
    assertEquals(Right(CountRows(42)), SparkMain.countCodec.requestDecoder.decodeJson(raw))
    assertEquals(Right(ExportRows(42)), SparkMain.exportCodec.requestDecoder.decodeJson(raw))
  }
  // This compiles the public inference ergonomics without making network calls.
  def typedSubmissionCompiles(ctx: LinhaContext): scala.concurrent.Future[JobHandle[SumResult]] =
    ctx.submit(SumNumbers(Vector(1, 2)), "stable-key")
}
