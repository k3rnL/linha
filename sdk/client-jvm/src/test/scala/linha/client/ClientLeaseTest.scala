package linha.client

import com.sun.net.httpserver.{HttpExchange, HttpHandler, HttpServer}
import java.net.InetSocketAddress
import java.util.concurrent.atomic.{AtomicBoolean, AtomicInteger}
import scala.concurrent.{Await, Future}
import scala.concurrent.duration._
import scala.concurrent.ExecutionContext.Implicits.global
import io.circe.Json
import io.circe.parser.parse
import linha.{CredentialProvider, RawRequest}
import org.junit.Test
import org.junit.Assert._

class ClientLeaseTest {
  @Test def sharedHandlesRenewRecoverAndRelease(): Unit = {
    val renewals = new AtomicInteger(0); val releases = new AtomicInteger(0);
    val attaches = new AtomicInteger(0)
    val expired = new AtomicBoolean(false)
    val server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0)
    server.createContext(
      "/v1/",
      new HttpHandler {
        def handle(exchange: HttpExchange): Unit = {
          val path = exchange.getRequestURI.getPath
          val (status, body) = if (path.endsWith("/renew")) {
            renewals.incrementAndGet()
            if (expired.compareAndSet(true, false))
              (409, """{"error":{"code":"CLIENT_LEASE_EXPIRED","message":"expired"}}""")
            else (200, "{}")
          } else if (exchange.getRequestMethod == "DELETE") {
            releases.incrementAndGet(); (204, "")
          } else if (path.endsWith("/jobs")) {
            assertNotNull(exchange.getRequestHeaders.getFirst("X-Linha-Client-Lease"))
            (202, """{"id":"job"}""")
          } else {
            if (path.endsWith("/clients")) attaches.incrementAndGet()
            (
              200,
              """{"id":"context","version":"hash","clientLease":{"id":"lease","durationSeconds":3}}"""
            )
          }
          val bytes = body.getBytes("UTF-8")
          exchange.sendResponseHeaders(status, if (status == 204) -1 else bytes.length)
          if (bytes.nonEmpty) exchange.getResponseBody.write(bytes)
          exchange.close()
        }
      }
    )
    server.start()
    val client =
      new LinhaClient("http://127.0.0.1:" + server.getAddress.getPort, CredentialProvider.none)
    def await[A](f: Future[A]): A = Await.result(f, 5.seconds)
    def eventually(p: => Boolean): Unit = {
      val until = System.nanoTime() + 5.seconds.toNanos
      while (!p && System.nanoTime() < until) Thread.sleep(20)
      assertTrue(p)
    }
    try {
      val first = await(client.ensureBackend("stable", Json.obj()))
      val second = await(client.ensureBackend("stable", Json.obj()))
      assertEquals(first.id, second.id); assertEquals("hash", first.version)
      first.close(); assertEquals(0, releases.get())
      eventually(renewals.get() > 0)
      expired.set(true); eventually(attaches.get() > 0)
      assertEquals("job", await(second.submitRaw(RawRequest("job", 1, Json.obj()))).id)
      second.close(); eventually(releases.get() == 1)
      second.close(); assertEquals(1, releases.get())
      val restored = await(client.restoreContext("context")); assertEquals("context", restored.id)
      client.close(); assertEquals(2, releases.get())
      // Job retrieval handles have no attachment and remain independent of context closure.
      assertEquals("job", client.rawJob("job").id)
    } finally { client.close(); server.stop(0) }
  }
}
