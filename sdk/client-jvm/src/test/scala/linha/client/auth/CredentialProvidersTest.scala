package linha.client.auth

import com.sun.net.httpserver.{HttpExchange, HttpServer}
import linha.{CredentialException, CredentialProvider}
import java.net.InetSocketAddress
import java.nio.file.Files
import java.time.{Clock, Duration, Instant, ZoneId}
import java.util.Base64
import java.util.concurrent.{ConcurrentLinkedQueue, Executors}
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Test
import org.junit.Assert._
import scala.collection.JavaConverters._

class CredentialProvidersTest {
  final class TestClock extends Clock {
    var now = Instant.parse("2026-01-01T00:00:00Z")
    def instant(): Instant = now
    def getZone(): ZoneId = ZoneId.of("UTC")
    override def withZone(zone: ZoneId): Clock = this
    def advance(seconds: Long): Unit = now = now.plusSeconds(seconds)
  }
  def send(e: HttpExchange, body: String, code: Int = 200): Unit = {
    val bytes = body.getBytes("UTF-8")
    e.sendResponseHeaders(code, bytes.length)
    try e.getResponseBody.write(bytes)
    finally e.close()
  }
  def fixture(run: (HttpServer, String) => Unit): Unit = {
    val server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0)
    val executor = Executors.newCachedThreadPool()
    server.setExecutor(executor)
    server.start()
    try run(server, "http://127.0.0.1:" + server.getAddress.getPort)
    finally { server.stop(0); executor.shutdownNow() }
  }
  private val testOptions = IdentityHttpOptions(allowHttp = true)

  @Test def oauthExchangeIsCachedAndErrorsAreRedacted(): Unit = fixture { (server, base) =>
    val clock = new TestClock
    val requests = new ConcurrentLinkedQueue[(String, String)]()
    @volatile var failExchange = false
    server.createContext(
      "/token",
      e => {
        requests.add(
          (
            e.getRequestHeaders.getFirst("Authorization"),
            new String(e.getRequestBody.readAllBytes(), "UTF-8")
          )
        )
        if (failExchange) send(e, "{\"error_description\":\"private-client-secret\"}", 401)
        else
          send(e, "{\"token_type\":\"Bearer\",\"access_token\":\"oauth-token\",\"expires_in\":100}")
      }
    )
    val provider = CredentialProviders.oauthClientCredentials(
      base + "/token",
      "application",
      () => "private-client-secret",
      Seq("linha:submit", "linha:read"),
      testOptions,
      clock
    )
    assertEquals("oauth-token", provider.bearerToken());
    assertEquals("oauth-token", provider.bearerToken())
    assertEquals(1, requests.size())
    assertEquals(
      "application:private-client-secret",
      new String(Base64.getDecoder.decode(requests.peek()._1.stripPrefix("Basic ")), "UTF-8")
    )
    assertEquals(
      "grant_type=client_credentials&scope=linha%3Asubmit+linha%3Aread",
      requests.peek()._2
    )
    clock.advance(101); failExchange = true
    try { provider.bearerToken(); fail("failed exchange accepted") }
    catch {
      case e: CredentialException =>
        assertEquals(Some(401), e.status);
        assertFalse(e.toString.contains("private-client-secret")); assertNull(e.getCause)
    }
  }

  @Test def refreshedExchangeReadsRotatedClientSecret(): Unit = fixture { (server, base) =>
    val clock = new TestClock
    val secretFile = Files.createTempFile("linha-oauth-client-secret", ".txt")
    val requests = new ConcurrentLinkedQueue[String]()
    server.createContext(
      "/token",
      e => {
        requests.add(
          new String(
            Base64.getDecoder.decode(
              e.getRequestHeaders.getFirst("Authorization").stripPrefix("Basic ")
            ),
            "UTF-8"
          )
        )
        send(
          e,
          s"""{"token_type":"Bearer","access_token":"token-${requests.size()}","expires_in":100}"""
        )
      }
    )
    try {
      Files.writeString(secretFile, "first-secret\n")
      val provider = CredentialProviders.oauthClientCredentials(
        base + "/token",
        "application",
        CredentialProvider.fileSecret(secretFile),
        options = testOptions,
        clock = clock
      )
      assertEquals("token-1", provider.bearerToken())
      Files.writeString(secretFile, "rotated-secret\n")
      assertEquals("token-1", provider.bearerToken())
      clock.advance(81)
      assertEquals("token-2", provider.bearerToken())
      assertEquals(
        Seq("application:first-secret", "application:rotated-secret"),
        requests.asScala.toSeq
      )
      val restarted = CredentialProviders.oauthClientCredentials(
        base + "/token",
        "application",
        CredentialProvider.fileSecret(secretFile),
        options = testOptions,
        clock = clock
      )
      assertEquals("token-3", restarted.bearerToken())
      assertEquals("application:rotated-secret", requests.asScala.last)
    } finally Files.deleteIfExists(secretFile)
  }

  @Test def invalidTokenResponsesFailClosed(): Unit = fixture { (server, base) =>
    @volatile var response = "{}"
    server.createContext("/token", e => send(e, response))
    for (
      body <- Seq(
        "{}",
        """{"token_type":"Basic","access_token":"private-value","expires_in":100}""",
        """{"token_type":"Bearer","access_token":"","expires_in":100}""",
        """{"token_type":"Bearer","access_token":"private-value","expires_in":0}""",
        """{"token_type":"Bearer","access_token":"private-value","expires_in":-1}""",
        """{"token_type":"Bearer","access_token":"private-value","expires_in":100000000}""",
        """{"token_type":"Bearer","access_token":"private-value"}"""
      )
    ) {
      response = body
      val provider = CredentialProviders.oauthClientCredentials(
        base + "/token",
        "application",
        () => "private-secret",
        options = testOptions
      )
      try { provider.bearerToken(); fail("invalid token response accepted") }
      catch {
        case e: CredentialException =>
          assertFalse(e.toString.contains("private-value"))
          assertFalse(e.toString.contains("private-secret"))
          assertNull(e.getCause)
      }
    }
  }

  @Test def acquisitionLatencyDoesNotExtendTokenLifetime(): Unit = fixture { (server, base) =>
    val clock = new TestClock
    server.createContext(
      "/token",
      e => {
        clock.advance(101)
        send(
          e,
          "{\"token_type\":\"Bearer\",\"access_token\":\"expired-in-transit\",\"expires_in\":100}"
        )
      }
    )
    val provider = CredentialProviders.oauthClientCredentials(
      base + "/token?realm=linha",
      "id",
      () => "secret",
      options = testOptions,
      clock = clock
    )
    try { provider.bearerToken(); fail("network latency extended token lifetime") }
    catch { case _: CredentialException => () }
  }

  @Test def redirectsResponseBoundsAndTimeouts(): Unit = fixture { (server, base) =>
    val leaked = new AtomicInteger()
    server.createContext("/leak", e => { leaked.incrementAndGet(); send(e, "{}") })
    server.createContext(
      "/redirect",
      e => { e.getResponseHeaders.add("Location", base + "/leak"); send(e, "{}", 307) }
    )
    server.createContext("/large", e => send(e, "x" * 1025))
    server.createContext("/slow", e => { Thread.sleep(1000); send(e, "{}") })
    for (
      (path, options) <- Seq(
        ("/redirect", testOptions),
        ("/large", testOptions.copy(maxResponseBytes = 1024)),
        ("/slow", testOptions.copy(requestTimeout = Duration.ofMillis(100)))
      )
    ) {
      val provider = CredentialProviders.oauthClientCredentials(
        base + path,
        "id",
        () => "private-secret",
        options = options
      )
      try { provider.bearerToken(); fail("invalid response accepted") }
      catch {
        case e: CredentialException =>
          assertFalse(e.toString.contains("private-secret")); assertNull(e.getCause)
      }
    }
    assertEquals(0, leaked.get())
    try {
      CredentialProviders.oauthClientCredentials(base + "/token", "id", () => "secret");
      fail("HTTP accepted by default")
    } catch { case _: CredentialException => () }
  }
}
