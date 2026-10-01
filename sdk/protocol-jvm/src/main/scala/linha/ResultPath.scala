package linha

import java.time.{Instant, ZoneOffset}
import java.util.regex.Matcher

final case class PathVariables(
    contextId: String,
    requestId: String,
    attemptId: String,
    file: String,
    submittedAt: Instant
)
final case class ResultPath private (version: Int, template: String) {
  def resolve(values: PathVariables): String = {
    Seq(values.contextId, values.requestId, values.attemptId).foreach { id =>
      ResultPath.validateName(id); require(!id.contains("/"), "invalid identity")
    }
    ResultPath.validateName(values.file)
    val resolved = ResultPath.placeholder.replaceAllIn(
      template,
      m =>
        Matcher.quoteReplacement(m.group(1) match {
          case "contextId"   => values.contextId
          case "requestId"   => values.requestId
          case "attemptId"   => values.attemptId
          case "file"        => values.file
          case "submittedAt" => ResultPath.format("yyyyMMddTHHmmssSSSZ", values.submittedAt)
          case key if key.startsWith("submittedAt:") =>
            ResultPath.format(key.stripPrefix("submittedAt:"), values.submittedAt)
          case unknown => throw new IllegalArgumentException("unknown placeholder " + unknown)
        })
    )
    ResultPath.validateName(resolved)
    resolved
  }
}
object ResultPath {
  private val placeholder = "\\{([^{}]+)\\}".r
  private val tokens = Seq("yyyy", "SSS", "MM", "dd", "HH", "mm", "ss")
  val defaultTemplate = "{contextId}/{requestId}/{attemptId}/{file}"
  def template(text: String): ResultPath = {
    require(
      text.getBytes("UTF-8").length <= 1024 && !text.contains("://"),
      "template must be a relative key"
    )
    Seq("requestId", "attemptId").foreach { name =>
      val expression = "{" + name + "}"
      require(
        text.split("/", -1).count(_ == expression) == 1 && text
          .sliding(expression.length)
          .count(_ == expression) == 1,
        "required identity segment " + name
      )
    }
    require(
      text.sliding(6).count(_ == "{file}") == 1 && text.endsWith("/{file}"),
      "file must be final suffix"
    )
    val result = ResultPath(1, text)
    result.resolve(
      PathVariables(
        "context",
        "request",
        "attempt",
        "result.json",
        Instant.parse("2026-09-28T23:59:58.123Z")
      )
    )
    result
  }
  def validateName(name: String): Unit = {
    require(
      name.nonEmpty && name.getBytes("UTF-8").length <= 1024 && !name.exists(
        Character.isISOControl
      ),
      "invalid path"
    )
    require(!name.exists(c => "\\{}%".contains(c)), "invalid path characters")
    require(
      name
        .split("/", -1)
        .forall(p => p.nonEmpty && p != "." && p != ".." && !p.startsWith("_linha")),
      "invalid path segment"
    )
  }
  private def format(pattern: String, instant: Instant): String = {
    require(pattern.nonEmpty, "empty date format")
    val t = instant.atOffset(ZoneOffset.UTC)
    val values = Map(
      "yyyy" -> f"${t.getYear}%04d",
      "MM" -> f"${t.getMonthValue}%02d",
      "dd" -> f"${t.getDayOfMonth}%02d",
      "HH" -> f"${t.getHour}%02d",
      "mm" -> f"${t.getMinute}%02d",
      "ss" -> f"${t.getSecond}%02d",
      "SSS" -> f"${t.getNano / 1000000}%03d"
    )
    val out = new StringBuilder
    var remaining = pattern
    while (remaining.nonEmpty) {
      tokens.find(remaining.startsWith) match {
        case Some(token) => out.append(values(token)); remaining = remaining.substring(token.length)
        case None        =>
          require("/-_TZ".contains(remaining.head), "unsupported date format");
          out.append(remaining.head); remaining = remaining.tail
      }
    }
    out.toString
  }
}
