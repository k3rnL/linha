package linha.client.auth

import linha.{AccessToken, CredentialException, CredentialProvider}
import io.circe.Json
import java.net.URLEncoder
import java.nio.charset.StandardCharsets.UTF_8
import java.time.{Clock, Instant}
import java.util.Base64

/** OAuth 2.0 client credentials; transports depend only on linha.CredentialProvider. */
object CredentialProviders {
  private def nonempty(value: String, what: String): String = {
    if (value == null || value.trim.isEmpty || value.exists(_.isControl))
      throw new CredentialException(what + " must be nonempty")
    value
  }
  private def text(json: Json, key: String): String =
    json.hcursor
      .get[String](key)
      .toOption
      .filter(_.nonEmpty)
      .getOrElse(throw new CredentialException("Identity response is missing " + key))
  private def expiry(json: Json, key: String, started: Instant): Instant = {
    val seconds = json.hcursor
      .get[Long](key)
      .getOrElse(throw new CredentialException("Identity response is missing expiry"))
    if (seconds <= 0 || seconds > 31536000)
      throw new CredentialException("Identity response has invalid expiry")
    started.plusSeconds(seconds)
  }
  private def encoded(value: String): String = URLEncoder.encode(value, "UTF-8")

  def oauthClientCredentials(
      tokenEndpoint: String,
      clientId: String,
      clientSecret: () => String,
      scopes: Seq[String] = Seq.empty,
      options: IdentityHttpOptions = IdentityHttpOptions(),
      clock: Clock = Clock.systemUTC()
  ): CredentialProvider = {
    nonempty(clientId, "OAuth client ID")
    require(
      scopes.forall(s =>
        s.nonEmpty && s.forall(c => c >= 0x21 && c <= 0x7e && c != '"' && c != '\\')
      ),
      "Invalid OAuth scope"
    )
    val http = new IdentityHttp(options)
    val endpoint = http.endpoint(tokenEndpoint)
    CredentialProvider.refreshing(
      () => {
        val credentials =
          encoded(clientId) + ":" + encoded(nonempty(clientSecret(), "OAuth client secret"))
        val basic = Base64.getEncoder.encodeToString(credentials.getBytes(UTF_8))
        val scope = if (scopes.isEmpty) "" else "&scope=" + encoded(scopes.mkString(" "))
        val started = clock.instant()
        val json = http.call(
          endpoint,
          "OAuth token exchange",
          Some("grant_type=client_credentials" + scope),
          Map(
            "Content-Type" -> "application/x-www-form-urlencoded",
            "Authorization" -> ("Basic " + basic)
          )
        )
        if (!text(json, "token_type").equalsIgnoreCase("Bearer"))
          throw new CredentialException("OAuth response must use Bearer token type")
        AccessToken(text(json, "access_token"), expiry(json, "expires_in", started))
      },
      clock
    )
  }

}
