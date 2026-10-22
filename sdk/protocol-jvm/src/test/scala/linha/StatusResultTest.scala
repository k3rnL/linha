package linha
import org.junit.Test
import org.junit.Assert._
import io.circe.Json
class StatusResultTest {
  @Test def unitResultsNeedNoArtifact(): Unit = {
    val codec = EntrypointCodec.derived[Json, Unit]("ingest", 1)
    assertEquals("status", codec.descriptor.kind)
    assertEquals(StatusResult, codec.encodeResult(()))
    assertTrue(codec.resultDecoder.decodeJson(Json.Null).isRight)
  }
}
