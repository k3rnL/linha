package linha
import java.nio.file.{Files, Paths}
import java.time.OffsetDateTime
import io.circe.parser.parse
import org.junit.Test
import org.junit.Assert._
class ResultPathTest {
  @Test def sharedFixtures(): Unit = {
    val fixtures = parse(
      new String(Files.readAllBytes(Paths.get("../../api/fixtures/path-templates.json")), "UTF-8")
    ).fold(throw _, identity).asArray.get
    fixtures.foreach { fixture =>
      val c = fixture.hcursor
      def str(key: String): String = c.get[String](key).fold(throw _, identity)
      val path = ResultPath.template(str("template"))
      val values = PathVariables(
        str("contextId"),
        str("requestId"),
        str("attemptId"),
        str("file"),
        OffsetDateTime.parse(str("submittedAt")).toInstant
      )
      assertEquals(str("expected"), path.resolve(values))
    }
  }
  @Test def rejectUnsafe(): Unit = {
    Seq(
      "{requestId}/{file}",
      "../{requestId}/{attemptId}/{file}",
      "{requestId}/{attemptId}/{submittedAt:YYYY}/{file}",
      "{requestId}/{attemptId}/{unknown}/{file}"
    ).foreach { template =>
      try { ResultPath.template(template); fail("accepted " + template) }
      catch { case _: IllegalArgumentException => () }
    }
    Seq("../x", "/tmp/a", "x//a", "%2e%2e/a", "_linha/manifest").foreach { name =>
      try { ResultPath.validateName(name); fail("accepted " + name) }
      catch { case _: IllegalArgumentException => () }
    }
  }
}
