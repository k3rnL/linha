package linha

import io.circe.{Json, Printer}
import io.circe.parser.parse
import io.circe.syntax._
import linha.json._
import java.nio.file.{Files, Paths}
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import org.junit.Test
import org.junit.Assert._

final case class FixtureCount(size: Long, label: String)
class ProtocolFixtureTest {
  @Test def canonicalSpecAndRawTypedEquivalence(): Unit = {
    val values = parse(
      new String(
        Files.readAllBytes(Paths.get("../../api/fixtures/canonical-protocol.json")),
        StandardCharsets.UTF_8
      )
    ).fold(throw _, identity).asArray.get
    values.foreach { fixture =>
      val c = fixture.hcursor
      val spec = c.get[Json]("spec").fold(throw _, identity)
      val canonical = Printer.noSpaces
        .copy(sortKeys = true)
        .print(spec)
        .replace("&", "\\u0026")
        .replace("<", "\\u003c")
        .replace(">", "\\u003e")
      assertEquals(c.get[String]("canonicalSpec").fold(throw _, identity), canonical)
      val hash = MessageDigest
        .getInstance("SHA-256")
        .digest(canonical.getBytes(StandardCharsets.UTF_8))
        .map(b => f"${b & 0xff}%02x")
        .mkString
      assertEquals(c.get[String]("configVersion").fold(throw _, identity), hash)
      val codec = EntrypointCodec.derived[FixtureCount, Long]("fixture.count", 1)
      val request = c.downField("request")
      val typed = codec.requestDecoder
        .decodeJson(request.get[Json]("payload").fold(throw _, identity))
        .fold(throw _, identity)
      assertEquals(
        request.get[Json]("payload").fold(throw _, identity),
        codec.requestEncoder(typed)
      )
      assertEquals(
        c.get[ResultDescriptor]("resultDescriptor").fold(throw _, identity),
        codec.descriptor
      )
    }
  }
}
