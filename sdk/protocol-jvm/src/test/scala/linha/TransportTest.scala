package linha

import com.sun.net.httpserver.HttpServer
import java.net.InetSocketAddress
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.atomic.AtomicReference
import org.junit.Test
import org.junit.Assert._

class TransportTest {
  @Test def absentAndRefreshingCredentials(): Unit = {
    val captured = new LinkedBlockingQueue[(Option[String], String)]()
    val server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0)
    server.createContext(
      "/v1/check",
      exchange => {
        captured.add(
          (
            Option(exchange.getRequestHeaders.getFirst("Authorization")),
            exchange.getRequestHeaders.getFirst("X-Linha-Worker-Id")
          )
        )
        val bytes = "{}".getBytes("UTF-8")
        exchange.sendResponseHeaders(200, bytes.length)
        exchange.getResponseBody.write(bytes)
        exchange.close()
      }
    )
    server.start()
    try {
      val base = "http://127.0.0.1:" + server.getAddress.getPort
      new Transport(base, CredentialProvider.none, Map("X-Linha-Worker-Id" -> "worker"))
        .call("POST", "/v1/check")
      assertEquals((None, "worker"), captured.take())
      val token = new AtomicReference("first")
      val client = new Transport(base, CredentialProvider(() => token.get()))
      client.call("GET", "/v1/check")
      assertEquals(Some("Bearer first"), captured.take()._1)
      token.set("second")
      client.call("GET", "/v1/check")
      assertEquals(Some("Bearer second"), captured.take()._1)
    } finally server.stop(0)
  }
}
