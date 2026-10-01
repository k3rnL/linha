package linha.worker

import io.circe.Json
import io.circe.syntax._
import java.io.{PrintWriter, Writer}
import java.util.IdentityHashMap
import scala.util.control.NonFatal

private[worker] object FailureDiagnostics {
  // A UTF-16 code unit consumes at most three UTF-8 bytes. This keeps the trace
  // below the 64 KiB wire limit without an unbounded StringWriter allocation.
  private val MaxTraceChars = 16384
  private object TraceLimit extends RuntimeException(null, null, false, false)
  private final class TraceWriter extends Writer {
    private val buffer = new java.lang.StringBuilder(2048)
    var truncated = false
    private def appendCharacter(value: Char): Unit = {
      if (buffer.length >= MaxTraceChars) { truncated = true; throw TraceLimit }
      buffer.append(value)
    }
    override def write(chars: Array[Char], start: Int, length: Int): Unit = {
      var i = start; while (i < start + length) { appendCharacter(chars(i)); i += 1 }
    }
    override def write(value: String, start: Int, length: Int): Unit = {
      var i = start; while (i < start + length) { appendCharacter(value.charAt(i)); i += 1 }
    }
    def flush(): Unit = ()
    def close(): Unit = ()
    def result: String = {
      if (buffer.length > 0 && Character.isHighSurrogate(buffer.charAt(buffer.length - 1)))
        buffer.setLength(buffer.length - 1)
      buffer.toString + (if (truncated) "\n... [stack trace truncated]" else "")
    }
  }
  private[worker] def utf8Prefix(value: String, bytes: Int): String = {
    var end = 0; var used = 0
    while (end < value.length) {
      val point = value.codePointAt(end)
      val size = if (point <= 0x7f) 1 else if (point <= 0x7ff) 2 else if (point <= 0xffff) 3 else 4
      if (used + size > bytes) return value.substring(0, end)
      used += size; end += Character.charCount(point)
    }
    value
  }
  def outOfMemory(error: Throwable): Boolean = {
    val seen = new IdentityHashMap[Throwable, java.lang.Boolean]()
    var current = error; var depth = 0
    while (current != null && depth < 64 && !seen.containsKey(current)) {
      if (current.isInstanceOf[OutOfMemoryError]) return true
      seen.put(current, java.lang.Boolean.TRUE); current = current.getCause; depth += 1
    }
    false
  }
  def capture(error: Throwable, code: String, retryable: Boolean, phase: String): Json = {
    val writer = new TraceWriter
    try error.printStackTrace(new PrintWriter(writer))
    catch {
      case TraceLimit  => ()
      case NonFatal(_) => writer.truncated = true
    }
    Json.obj(
      "code" -> code.asJson,
      "message" -> utf8Prefix(
        Option(error.getMessage).getOrElse(error.getClass.getName),
        8192
      ).asJson,
      "retryable" -> retryable.asJson,
      "exceptionType" -> utf8Prefix(error.getClass.getName, 1024).asJson,
      "stackTrace" -> writer.result.asJson,
      "stackTraceTruncated" -> writer.truncated.asJson,
      "phase" -> phase.asJson
    )
  }
  def log(correlation: String, error: Throwable): Unit = {
    System.err.println(correlation)
    error.printStackTrace(System.err)
  }
}
