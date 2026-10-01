package linha.worker

import com.sun.net.httpserver.HttpServer
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import java.util.concurrent.{ConcurrentHashMap, CountDownLatch, Executors, TimeUnit}
import java.util.concurrent.atomic.AtomicInteger
import io.circe.Json
import io.circe.syntax._
import linha._
import org.junit.Test
import org.junit.Assert._

class WorkerCancellationTest {
  @Test def leaseExpirySurvivesThrowingHooksAndUsesAttemptGroups(): Unit = {
    val claimed = new AtomicInteger()
    val observed = ConcurrentHashMap.newKeySet[String]()
    val cancelled = new CountDownLatch(2)
    val http = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0)
    val network = Executors.newCachedThreadPool()
    http.setExecutor(network)
    http.createContext(
      "/",
      exchange => {
        val path = exchange.getRequestURI.getPath
        var status = 200
        val body = if (path.endsWith("/claim")) {
          val n = claimed.getAndIncrement()
          if (n >= 2) { status = 204; "" }
          else
            Json
              .obj(
                "job" -> Json.obj(
                  "id" -> s"job-$n".asJson,
                  "request" -> Json
                    .obj("handler" -> "wait".asJson, "version" -> 1.asJson, "payload" -> Json.obj())
                ),
                "attemptId" -> s"attempt-$n".asJson,
                "fence" -> 1.asJson,
                "leaseDurationMillis" -> 500.asJson,
                "results" -> Json.obj("maxBytes" -> 1024.asJson),
                "resultDescriptor" -> Json.obj(
                  "kind" -> "json".asJson,
                  "schema" -> "wait.result".asJson,
                  "version" -> 1.asJson
                )
              )
              .noSpaces
        } else if (path.endsWith("/renew")) {
          status = 503; "{\"error\":{\"code\":\"UNAVAILABLE\",\"message\":\"partition\"}}"
        } else "{\"draining\":false}"
        val bytes = body.getBytes(StandardCharsets.UTF_8)
        exchange.getRequestBody.close()
        exchange.getResponseHeaders.set("Content-Type", "application/json")
        exchange.sendResponseHeaders(status, if (status == 204) -1 else bytes.length)
        if (status != 204) exchange.getResponseBody.write(bytes)
        exchange.close()
      }
    )
    http.start()
    val credential =
      WorkerCredential(WorkerIdentity("context", "worker", "incarnation", 0L), "test")
    val worker = new LinhaWorker[Unit](
      s"http://127.0.0.1:${http.getAddress.getPort}",
      credential,
      _ => (),
      cancelEngine = key => {
        observed.add(key); cancelled.countDown();
        throw new IllegalStateException("faulty engine hook")
      },
      capacity = 2
    )
    worker.handleRaw(Capability("wait", 1, ResultDescriptor("json", "wait.result", 1))) { (_, _) =>
      Thread.sleep(30000); JsonResult(Json.Null)
    }
    val run = new Thread(() => worker.run())
    try {
      run.start()
      assertTrue("both expired attempts must be cancelled", cancelled.await(8, TimeUnit.SECONDS))
      assertTrue(observed.contains("attempt-0")); assertTrue(observed.contains("attempt-1"))
      assertFalse(observed.contains("job-0"))
    } finally { worker.close(); run.join(3000); http.stop(0); network.shutdownNow() }
  }
}
