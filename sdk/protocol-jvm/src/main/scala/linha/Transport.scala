package linha

import io.circe.Json
import io.circe.parser.parse
import java.net.URI
import java.net.http.{HttpClient, HttpRequest, HttpResponse}
import java.time.Duration
import java.nio.file.{Files, Path}
import java.io.{IOException, InputStream}

final case class LinhaException(status: Int, code: String, detail: String)
    extends RuntimeException(code + ": " + detail)
trait CredentialProvider { def bearerToken(): String }
object CredentialProvider {
  val none: CredentialProvider = apply(() => "");
  def apply(get: () => String): CredentialProvider = new CredentialProvider {
    def bearerToken(): String = get()
  }
}
final class Transport(
    val baseUrl: String,
    credentials: CredentialProvider,
    headers: Map[String, String] = Map.empty,
    timeoutSeconds: Long = 40,
    retries: Int = 2
) {
  private val base = URI.create(baseUrl.stripSuffix("/"))
  require(
    Set("http", "https")(base.getScheme) && base.getHost != null,
    "baseUrl must be an HTTP(S) origin"
  )
  private val http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build()
  private def request(path: String): HttpRequest.Builder = {
    require(path.startsWith("/v1/") && !path.startsWith("//"), "invalid API path")
    val builder = HttpRequest
      .newBuilder(URI.create(base.toString + path))
      .timeout(Duration.ofSeconds(timeoutSeconds))
    val token = credentials.bearerToken()
    if (token.nonEmpty) builder.header("Authorization", "Bearer " + token)
    headers.foreach { case (name, value) => builder.header(name, value) }
    builder
  }
  private def decode(status: Int, body: String): Json = {
    if (status == 204) return Json.Null
    val json =
      parse(body).fold(e => throw LinhaException(status, "INVALID_RESPONSE", e.message), identity)
    if (status >= 400) {
      val c = json.hcursor.downField("error")
      throw LinhaException(
        status,
        c.get[String]("code").getOrElse("HTTP_ERROR"),
        c.get[String]("message").getOrElse("request failed")
      )
    }
    json
  }
  private def retry[A](call: => A): A = {
    var attempt = 0
    while (true) {
      try return call
      catch {
        case e: IOException if attempt < retries                                            => ()
        case e: LinhaException if (e.status == 503 || e.status == 429) && attempt < retries => ()
      }
      attempt += 1
      Thread.sleep(100L * attempt)
    }
    throw new IllegalStateException("unreachable")
  }
  def call(
      method: String,
      path: String,
      body: Json = Json.obj(),
      extraHeaders: Map[String, String] = Map.empty
  ): Json = retry {
    val publisher =
      if (method == "GET") HttpRequest.BodyPublishers.noBody()
      else HttpRequest.BodyPublishers.ofString(body.noSpaces)
    val builder = request(path).header("Content-Type", "application/json").method(method, publisher)
    extraHeaders.foreach { case (name, value) => builder.header(name, value) }
    val response = http.send(builder.build(), HttpResponse.BodyHandlers.ofString())
    decode(response.statusCode(), response.body())
  }
  def upload(path: String, file: Path, attemptId: String, fence: Long): Json = retry {
    val response = http.send(
      request(path)
        .header("X-Linha-Attempt", attemptId)
        .header("X-Linha-Fence", fence.toString)
        .PUT(HttpRequest.BodyPublishers.ofFile(file))
        .build(),
      HttpResponse.BodyHandlers.ofString()
    )
    decode(response.statusCode(), response.body())
  }
  def open(path: String): InputStream = {
    val response = http.send(request(path).GET().build(), HttpResponse.BodyHandlers.ofInputStream())
    if (response.statusCode() >= 400) {
      val stream = response.body()
      try
        decode(
          response.statusCode(),
          new String(stream.readNBytes(65536), java.nio.charset.StandardCharsets.UTF_8)
        )
      finally stream.close()
    }
    response.body()
  }
}
