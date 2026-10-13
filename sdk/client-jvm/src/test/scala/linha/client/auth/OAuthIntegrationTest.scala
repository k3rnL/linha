package linha.client.auth

import linha.CredentialProvider
import java.net.URI
import java.net.http.{HttpClient, HttpRequest, HttpResponse}
import java.nio.file.Paths
import java.security.{KeyFactory, Signature}
import java.security.spec.RSAPublicKeySpec
import java.math.BigInteger
import java.time.{Clock, Instant, ZoneId}
import java.util.Base64
import io.circe.{Json, parser}
import org.junit.{Assume, Test}
import org.junit.Assert._

/** Enabled by scripts/test-oauth-auth.py against a disposable OAuth 2.0 identity provider. */
class OAuthIntegrationTest {
  final class TestClock extends Clock {
    var now = Instant.now()
    def instant(): Instant = now
    def getZone(): ZoneId = ZoneId.of("UTC")
    override def withZone(zone: ZoneId): Clock = this
    def advance(seconds: Long): Unit = now = now.plusSeconds(seconds)
  }
  @Test def realClientCredentialsRefreshAndStableApplicationIdentity(): Unit = {
    val issuer = sys.env.get("LINHA_TEST_OAUTH_ISSUER")
    Assume.assumeTrue("requires disposable OAuth fixture", issuer.nonEmpty)
    val clock = new TestClock
    val secret = CredentialProvider.fileSecret(Paths.get(sys.env("LINHA_TEST_OAUTH_SECRET_FILE")))
    def provider(id: String = "report-api") = CredentialProviders.oauthClientCredentials(
      issuer.get + "/protocol/openid-connect/token",
      id,
      secret,
      options = IdentityHttpOptions(allowHttp = true),
      clock = clock
    )
    val client = provider()
    val first = client.bearerToken()
    assertEquals("valid token should be cached", first, client.bearerToken())
    clock.advance(25)
    val renewed = client.bearerToken()
    assertNotEquals("refresh should acquire a new token", first, renewed)
    val restarted = provider().bearerToken()
    val other = provider("import-api").bearerToken()
    val decode = Base64.getUrlDecoder
    def claims(token: String): Json =
      parser
        .parse(new String(decode.decode(token.split("\\.")(1)), "UTF-8"))
        .fold(throw _, identity)
    val firstClaims = claims(first)
    val subject = firstClaims.hcursor.get[String]("sub").fold(throw _, identity)
    assertTrue(subject.nonEmpty)
    for (token <- Seq(first, renewed, restarted, other)) {
      val value = claims(token)
      assertEquals(issuer.get, value.hcursor.get[String]("iss").fold(throw _, identity))
      val audience = value.hcursor.downField("aud").focus.get
      assertTrue(
        audience.asString.contains("linha") || audience.asArray.exists(
          _.exists(_.asString.contains("linha"))
        )
      )
      assertTrue(
        value.hcursor.get[Long]("exp").fold(throw _, identity) > Instant.now().getEpochSecond
      )
    }
    assertEquals(subject, claims(renewed).hcursor.get[String]("sub").fold(throw _, identity))
    assertEquals(subject, claims(restarted).hcursor.get[String]("sub").fold(throw _, identity))
    assertNotEquals(subject, claims(other).hcursor.get[String]("sub").fold(throw _, identity))
    val response = HttpClient
      .newHttpClient()
      .send(
        HttpRequest
          .newBuilder(URI.create(issuer.get + "/protocol/openid-connect/certs"))
          .GET()
          .build(),
        HttpResponse.BodyHandlers.ofString()
      )
    assertEquals(200, response.statusCode())
    val keys = parser
      .parse(response.body())
      .fold(throw _, identity)
      .hcursor
      .downField("keys")
      .focus
      .get
      .asArray
      .get
    for (token <- Seq(first, renewed, restarted, other)) {
      val parts = token.split("\\.")
      val header =
        parser.parse(new String(decode.decode(parts(0)), "UTF-8")).fold(throw _, identity)
      assertEquals("RS256", header.hcursor.get[String]("alg").fold(throw _, identity))
      val kid = header.hcursor.get[String]("kid").fold(throw _, identity)
      val key = keys.find(_.hcursor.get[String]("kid").toOption.contains(kid)).get
      val publicKey = KeyFactory
        .getInstance("RSA")
        .generatePublic(
          new RSAPublicKeySpec(
            new BigInteger(1, decode.decode(key.hcursor.get[String]("n").fold(throw _, identity))),
            new BigInteger(1, decode.decode(key.hcursor.get[String]("e").fold(throw _, identity)))
          )
        )
      val verifier = Signature.getInstance("SHA256withRSA")
      verifier.initVerify(publicKey)
      verifier.update((parts(0) + "." + parts(1)).getBytes("UTF-8"))
      assertTrue("access token signature invalid", verifier.verify(decode.decode(parts(2))))
    }
  }
}
