package linha.client.auth

import linha.CredentialException
import io.circe.Json
import io.circe.parser.parse
import java.net.URI
import java.net.http.{HttpClient, HttpRequest, HttpResponse}
import java.nio.charset.StandardCharsets.UTF_8
import java.nio.file.{Files, Path}
import java.security.{KeyStore, SecureRandom}
import java.security.cert.{CertificateFactory, X509Certificate}
import java.time.Duration
import javax.net.ssl.{SSLContext, SSLEngine, TrustManagerFactory, X509ExtendedTrustManager}
import java.net.Socket
import java.nio.ByteBuffer
import java.io.ByteArrayOutputStream
import java.util.concurrent.{CompletableFuture, CompletionStage, Flow, TimeUnit}
import scala.collection.JavaConverters._
import scala.util.control.NonFatal

final case class IdentityHttpOptions(
    caFile: Option[Path] = None,
    insecureSkipVerify: Boolean = false,
    allowHttp: Boolean = false,
    connectTimeout: Duration = Duration.ofSeconds(5),
    requestTimeout: Duration = Duration.ofSeconds(10),
    maxResponseBytes: Int = 1048576
) {
  require(
    connectTimeout.toMillis > 0 && requestTimeout.toMillis > 0,
    "Identity HTTP timeouts must be positive"
  )
  require(
    maxResponseBytes > 0 && maxResponseBytes <= 16777216,
    "Identity response limit must be 1-16777216 bytes"
  )
  require(!(caFile.nonEmpty && insecureSkipVerify), "Choose a CA file or explicit TLS bypass")
}

private[auth] final class IdentityHttp(options: IdentityHttpOptions) {
  private val builder = HttpClient
    .newBuilder()
    .connectTimeout(options.connectTimeout)
    .followRedirects(HttpClient.Redirect.NEVER)
  try {
    if (options.insecureSkipVerify) {
      val context = SSLContext.getInstance("TLS")
      val trust = new X509ExtendedTrustManager {
        def getAcceptedIssuers(): Array[X509Certificate] = Array.empty
        def checkClientTrusted(c: Array[X509Certificate], a: String): Unit = ()
        def checkServerTrusted(c: Array[X509Certificate], a: String): Unit = ()
        def checkClientTrusted(c: Array[X509Certificate], a: String, s: Socket): Unit = ()
        def checkServerTrusted(c: Array[X509Certificate], a: String, s: Socket): Unit = ()
        def checkClientTrusted(c: Array[X509Certificate], a: String, e: SSLEngine): Unit = ()
        def checkServerTrusted(c: Array[X509Certificate], a: String, e: SSLEngine): Unit = ()
      }
      context.init(null, Array(trust), new SecureRandom())
      builder.sslContext(context)
    } else
      options.caFile.foreach { path =>
        val store = KeyStore.getInstance(KeyStore.getDefaultType)
        store.load(null, null)
        val stream = Files.newInputStream(path)
        val certificates =
          try CertificateFactory.getInstance("X.509").generateCertificates(stream)
          finally stream.close()
        if (certificates.isEmpty)
          throw new CredentialException("Identity CA file has no certificates")
        certificates.asScala.zipWithIndex.foreach { case (cert, index) =>
          store.setCertificateEntry(index.toString, cert)
        }
        val factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm)
        factory.init(store)
        val context = SSLContext.getInstance("TLS")
        context.init(null, factory.getTrustManagers, new SecureRandom())
        builder.sslContext(context)
      }
  } catch {
    case NonFatal(_) => throw new CredentialException("Cannot configure identity-provider TLS")
  }
  private val client = builder.build()

  def endpoint(raw: String): URI = try {
    val uri = URI.create(raw)
    if (
      uri.getHost == null || uri.getUserInfo != null || uri.getFragment != null ||
      !(uri.getScheme == "https" || (options.allowHttp && uri.getScheme == "http"))
    )
      throw new CredentialException(
        "Identity endpoint must be HTTPS without credentials or fragment"
      )
    uri
  } catch {
    case NonFatal(_) => throw new CredentialException("Invalid identity-provider endpoint")
  }

  def call(uri: URI, operation: String, body: Option[String], headers: Map[String, String]): Json =
    try {
      val builder = HttpRequest.newBuilder(uri).timeout(options.requestTimeout)
      headers.foreach { case (name, value) => builder.header(name, value) }
      body match {
        case Some(value) => builder.POST(HttpRequest.BodyPublishers.ofString(value))
        case None        => builder.GET()
      }
      val pending = client.sendAsync(
        builder.build(),
        new HttpResponse.BodyHandler[Array[Byte]] {
          def apply(info: HttpResponse.ResponseInfo): HttpResponse.BodySubscriber[Array[Byte]] =
            new HttpResponse.BodySubscriber[Array[Byte]] {
              private val completed = new CompletableFuture[Array[Byte]]()
              private val bytes = new ByteArrayOutputStream()
              private var subscription: Flow.Subscription = null
              def getBody(): CompletionStage[Array[Byte]] = completed
              def onSubscribe(s: Flow.Subscription): Unit = {
                subscription = s; s.request(Long.MaxValue)
              }
              def onNext(chunks: java.util.List[ByteBuffer]): Unit = {
                chunks.asScala.foreach { chunk =>
                  if (!completed.isDone) {
                    if (chunk.remaining() > options.maxResponseBytes - bytes.size()) {
                      subscription.cancel()
                      completed.completeExceptionally(
                        new CredentialException("Identity response is too large")
                      )
                    } else {
                      val part = new Array[Byte](chunk.remaining())
                      chunk.get(part)
                      bytes.write(part)
                    }
                  }
                }
              }
              def onError(error: Throwable): Unit = { completed.completeExceptionally(error); () }
              def onComplete(): Unit = { completed.complete(bytes.toByteArray); () }
            }
        }
      )
      val response =
        try pending.get(options.requestTimeout.toMillis, TimeUnit.MILLISECONDS)
        catch { case NonFatal(error) => pending.cancel(true); throw error }
      if (response.statusCode() < 200 || response.statusCode() >= 300)
        throw new CredentialException(
          operation + " failed (HTTP " + response.statusCode() + ")",
          Some(response.statusCode())
        )
      if (response.body().length > options.maxResponseBytes)
        throw new CredentialException(operation + " response is too large")
      parse(new String(response.body(), UTF_8))
        .fold(_ => throw new CredentialException(operation + " returned invalid JSON"), identity)
    } catch {
      case e: CredentialException => throw e
      case NonFatal(_)            => throw new CredentialException(operation + " connection failed")
    }
}
