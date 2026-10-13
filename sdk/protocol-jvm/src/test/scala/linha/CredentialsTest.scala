package linha

import java.nio.file.Files
import java.time.{Clock, Instant, ZoneId}
import java.util.concurrent.atomic.{AtomicInteger, AtomicReference}
import org.junit.Test
import org.junit.Assert._
import scala.concurrent.{Await, Future}
import scala.concurrent.duration._
import scala.concurrent.ExecutionContext.Implicits.global

class CredentialsTest {
  final class TestClock extends Clock {
    val now = new AtomicReference(Instant.parse("2026-01-01T00:00:00Z"))
    def instant(): Instant = now.get()
    def getZone(): ZoneId = ZoneId.of("UTC")
    override def withZone(zone: ZoneId): Clock = this
    def advance(seconds: Long): Unit = now.set(now.get().plusSeconds(seconds))
  }
  @Test def concurrentRefreshExpiryOutageAndRecovery(): Unit = {
    val clock = new TestClock
    val count = new AtomicInteger()
    @volatile var unavailable = false
    val provider = CredentialProvider.refreshing(
      () => {
        count.incrementAndGet()
        if (unavailable) throw new RuntimeException("private-secret-response")
        AccessToken("token-" + count.get(), clock.instant().plusSeconds(100))
      },
      clock
    )
    def concurrent(): Seq[String] =
      Await.result(Future.sequence((1 to 32).map(_ => Future(provider.bearerToken()))), 5.seconds)
    assertEquals(Set("token-1"), concurrent().toSet); assertEquals(1, count.get())
    clock.advance(81)
    assertEquals(Set("token-2"), concurrent().toSet); assertEquals(2, count.get())
    clock.advance(81); unavailable = true
    assertEquals("token-2", provider.bearerToken())
    assertEquals(Set("token-2"), concurrent().toSet); assertEquals(3, count.get())
    clock.advance(20)
    val failure =
      try { provider.bearerToken(); fail("expired token accepted"); null }
      catch { case e: CredentialException => e }
    assertFalse(failure.toString.contains("private-secret")); assertNull(failure.getCause)
    unavailable = false; clock.advance(6)
    assertEquals("token-5", provider.bearerToken())
  }
  @Test def filesRotateAndFailClosed(): Unit = {
    val file = Files.createTempFile("linha-credential", ".txt")
    val provider = CredentialProvider.file(file)
    try {
      Files.writeString(file, "first\n"); assertEquals("first", provider.bearerToken())
      Files.writeString(file, "second\n"); assertEquals("second", provider.bearerToken())
      Files.writeString(file, " \n")
      try { provider.bearerToken(); fail("empty token accepted") }
      catch { case _: CredentialException => () }
      Files.delete(file)
      try { provider.bearerToken(); fail("missing token accepted") }
      catch { case e: CredentialException => assertNull(e.getCause) }
      assertEquals("", CredentialProvider.none.bearerToken())
    } finally Files.deleteIfExists(file)
  }
  @Test def tokensDoNotRenderValues(): Unit = {
    val token = AccessToken("private-value", Instant.now().plusSeconds(60))
    assertFalse(token.toString.contains("private-value"))
    try { CredentialProvider.static("\r\n"); fail("invalid token accepted") }
    catch { case _: CredentialException => () }
  }
}
