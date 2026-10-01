package linha.worker

import java.io.{ByteArrayOutputStream, IOException, PrintStream}
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets.UTF_8
import java.util.concurrent.{ConcurrentHashMap, CountDownLatch, Executors, TimeUnit}
import java.util.concurrent.atomic.AtomicInteger
import com.sun.net.httpserver.HttpServer
import io.circe.Json
import io.circe.parser.parse
import io.circe.syntax._
import linha._
import org.junit.Test
import org.junit.Assert._

class FailureDiagnosticsTest {
  @Test def boundedUnicodeTraceAndCauseCycles(): Unit = {
    val error = new IllegalStateException("é😀" * 20000, new IOException("inner"))
    val diagnostic = FailureDiagnostics.capture(error, "HANDLER_FAILED", false, "handler").hcursor
    val message = diagnostic.get[String]("message").fold(throw _, identity)
    val trace = diagnostic.get[String]("stackTrace").fold(throw _, identity)
    assertTrue(message.getBytes(UTF_8).length <= 8192)
    assertEquals(message, new String(message.getBytes(UTF_8), UTF_8))
    assertTrue(trace.getBytes(UTF_8).length <= 65536)
    assertEquals(trace, new String(trace.getBytes(UTF_8), UTF_8))
    assertTrue(diagnostic.get[Boolean]("stackTraceTruncated").fold(throw _, identity))
    assertTrue(trace.endsWith("[stack trace truncated]"))
    val a = new RuntimeException("a"); val b = new RuntimeException("b", a); a.initCause(b)
    assertFalse(FailureDiagnostics.outOfMemory(a))
    assertTrue(
      FailureDiagnostics
        .capture(a, "HANDLER_FAILED", false, "handler")
        .noSpaces
        .contains("CIRCULAR REFERENCE")
    )
    assertTrue(
      FailureDiagnostics.outOfMemory(
        new RuntimeException("Spark wrapper", new OutOfMemoryError("Java heap space"))
      )
    )
  }

  @Test def workerReportsEveryPhaseAndLogsSecondaryFailures(): Unit = {
    val next = new AtomicInteger()
    val reports = new ConcurrentHashMap[String, Json]()
    val reported = new CountDownLatch(7)
    val http = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0)
    val network = Executors.newCachedThreadPool(); http.setExecutor(network)
    http.createContext(
      "/",
      exchange => {
        val path = exchange.getRequestURI.getPath
        val input = new String(exchange.getRequestBody.readAllBytes(), UTF_8)
        var status = 200
        val body = if (path.endsWith("/claim")) {
          val n = next.getAndIncrement()
          if (n >= 7) { status = 204; "" }
          else {
            var assignment = Json.obj(
              "job" -> Json.obj(
                "id" -> s"job-$n".asJson,
                "request" -> Json.obj(
                  "handler" -> "test".asJson,
                  "version" -> 1.asJson,
                  "payload" -> Json.obj("case" -> n.asJson)
                )
              ),
              "attemptId" -> s"attempt-$n".asJson,
              "fence" -> 1.asJson,
              "leaseDurationMillis" -> 30000.asJson,
              "resultDescriptor" -> Json.obj(
                "kind" -> "json".asJson,
                "schema" -> "test.result".asJson,
                "version" -> 1.asJson
              )
            )
            if (n != 1)
              assignment =
                assignment.deepMerge(Json.obj("results" -> Json.obj("maxBytes" -> 1024.asJson)))
            assignment.noSpaces
          }
        } else if (path.endsWith("/fail")) {
          val job = path.split('/')(3)
          reports.put(
            job,
            parse(input)
              .fold(throw _, identity)
              .hcursor
              .get[Json]("failure")
              .fold(throw _, identity)
          ); reported.countDown()
          if (job == "job-6") {
            status = 400;
            """{"error":{"code":"REPORT_REJECTED","message":"injected report failure"}}"""
          } else "{}"
        } else if (path == "/v1/jobs/job-4/outputs" || path.endsWith("/complete")) {
          status = 409;
          """{"error":{"code":"STORAGE_ERROR","message":"injected publication failure"}}"""
        } else if (path.endsWith("/outputs")) """{"uploadUrl":"/v1/mock-upload"}"""
        else if (path == "/v1/mock-upload")
          """{"allocationId":"output","name":"result.json","size":4,"sha256":"hash","contentType":"application/json"}"""
        else """{"draining":false}"""
        val bytes = body.getBytes(UTF_8)
        exchange.getResponseHeaders.set("Content-Type", "application/json")
        exchange.sendResponseHeaders(status, if (status == 204) -1 else bytes.length)
        if (status != 204) exchange.getResponseBody.write(bytes)
        exchange.close()
      }
    )
    http.start()
    val worker = new LinhaWorker[Unit](
      s"http://127.0.0.1:${http.getAddress.getPort}",
      WorkerCredential(WorkerIdentity("context", "worker", "incarnation", 0), ""),
      attempt =>
        if (attempt == "attempt-2") throw new IllegalArgumentException("engine failed") else ()
    )
    worker.handleRaw(Capability("test", 1, ResultDescriptor("json", "test.result", 1))) {
      (payload, ctx) =>
        payload.hcursor.get[Int]("case").fold(throw _, identity) match {
          case 0 =>
            ctx.onCleanup(() => throw new IllegalArgumentException("cleanup hook failed"))
            val error = new IllegalStateException("outer", new IOException("root cause"))
            error.addSuppressed(new IllegalArgumentException("suppressed detail")); throw error
          case 3 =>
            throw new OutOfMemoryError("Java heap space") // Injected; do not exhaust the test JVM.
          case 6 => throw new IllegalStateException("primary despite reporting failure")
          case _ => JsonResult(Json.Null)
        }
    }
    val captured = new ByteArrayOutputStream(); val original = System.err
    System.setErr(new PrintStream(captured, true, "UTF-8"))
    val run = new Thread(() => worker.run())
    try {
      run.start(); assertTrue("failure reports timed out", reported.await(15, TimeUnit.SECONDS))
      val expected =
        Vector("handler", "initialization", "engine", "handler", "result", "completion", "handler")
      expected.zipWithIndex.foreach { case (phase, n) =>
        val c = reports.get(s"job-$n").hcursor
        assertEquals(phase, c.get[String]("phase").fold(throw _, identity))
        assertTrue(c.get[String]("exceptionType").fold(throw _, identity).nonEmpty)
        val trace = c.get[String]("stackTrace").fold(throw _, identity)
        // Circe deliberately uses NoStackTrace for decoding failures; preserve
        // its field-path diagnostic without inventing original frames.
        if (n == 1) assertTrue(trace, trace.contains("DecodingFailure"))
        else assertTrue(trace, trace.contains("\tat "))
      }
      val nested = reports.get("job-0").hcursor.get[String]("stackTrace").fold(throw _, identity)
      assertTrue(nested.contains("Caused by: java.io.IOException: root cause"))
      assertTrue(
        nested.contains("Suppressed: java.lang.IllegalArgumentException: suppressed detail")
      )
      assertEquals(
        "OUT_OF_MEMORY",
        reports.get("job-3").hcursor.get[String]("code").fold(throw _, identity)
      )
      assertEquals(
        "java.lang.OutOfMemoryError",
        reports.get("job-3").hcursor.get[String]("exceptionType").fold(throw _, identity)
      )
      val until = System.nanoTime() + TimeUnit.SECONDS.toNanos(3)
      while (
        !captured.toString("UTF-8").contains("phase=failure-report") && System.nanoTime() < until
      ) Thread.sleep(10)
      worker.close(); run.join(3000)
      val logs = captured.toString("UTF-8")
      assertTrue(logs.contains("jobId=job-0 attemptId=attempt-0 workerId=worker phase=handler"))
      assertTrue(logs.contains("java.io.IOException: root cause"))
      assertTrue(logs.contains("cleanup hook failed"))
      assertTrue(logs.contains("phase=failure-report"))
      assertTrue(logs.contains("primary despite reporting failure"))
    } finally {
      worker.close(); run.join(3000); System.setErr(original); http.stop(0); network.shutdownNow()
    }
  }
}
