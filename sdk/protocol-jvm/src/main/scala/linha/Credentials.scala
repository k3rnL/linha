package linha

import java.nio.charset.StandardCharsets.UTF_8
import java.nio.file.{Files, Path}
import java.time.{Clock, Duration, Instant}
import scala.util.control.NonFatal

/** Errors intentionally exclude identity-provider responses, credentials and nested exceptions. */
final class CredentialException(message: String, val status: Option[Int] = None)
    extends RuntimeException(message)

/** Expiry is acquisition metadata; the Linha server is responsible for validating the token. */
final class AccessToken private (val value: String, val expiresAt: Instant) {
  override def toString: String = "AccessToken(<redacted>, " + expiresAt + ")"
}
object AccessToken {
  def apply(value: String, expiresAt: Instant): AccessToken = {
    if (expiresAt == null) throw new CredentialException("Credential expiry is required")
    new AccessToken(CredentialProvider.checked(value), expiresAt)
  }
}

/** Providers are shareable between clients/transports; refresh requires no background thread. */
trait CredentialProvider { def bearerToken(): String }
object CredentialProvider {
  val none: CredentialProvider = new CredentialProvider { def bearerToken(): String = "" }

  private[linha] def checked(value: String): String = {
    if (
      value == null || value.trim.isEmpty || value.trim.exists(c => c.isWhitespace || c.isControl)
    )
      throw new CredentialException("Credential must be a nonempty bearer token")
    value.trim
  }

  def apply(get: () => String): CredentialProvider = new CredentialProvider {
    def bearerToken(): String = try checked(get())
    catch {
      case e: CredentialException => throw e
      case NonFatal(_)            => throw new CredentialException("Credential acquisition failed")
    }
  }

  def static(token: String): CredentialProvider = {
    val value = checked(token)
    apply(() => value)
  }

  /** Reads the current file on every call, including after Kubernetes Secret rotation. */
  def file(path: Path): CredentialProvider = apply(() => readSecret(path))

  /** Re-reads mounted OAuth client secrets or worker credentials when acquisition runs. */
  def fileSecret(path: Path): () => String = () => readSecret(path)

  private def readSecret(path: Path): String = try {
    val input = Files.newInputStream(path)
    val bytes =
      try input.readNBytes(65537)
      finally input.close()
    if (bytes.length > 65536) throw new CredentialException("Credential file is too large")
    val value = new String(bytes, UTF_8).trim
    if (value.isEmpty || value.exists(_.isControl))
      throw new CredentialException("Credential file is empty or invalid")
    value
  } catch {
    case e: CredentialException => throw e
    case NonFatal(_)            => throw new CredentialException("Cannot read credential file")
  }

  def refreshing(
      acquire: () => AccessToken,
      clock: Clock = Clock.systemUTC(),
      refreshBefore: Duration = Duration.ofSeconds(30),
      retryDelay: Duration = Duration.ofSeconds(5)
  ): CredentialProvider = {
    require(
      !refreshBefore.isNegative && !retryDelay.isNegative,
      "Credential timings must be nonnegative"
    )
    new CredentialProvider {
      private var cached: AccessToken = null
      private var refreshAt = Instant.MIN
      private var retryAt = Instant.MIN
      private var failure: CredentialException = null
      def bearerToken(): String = synchronized {
        val now = clock.instant()
        if (cached != null && now.isBefore(refreshAt) && now.isBefore(cached.expiresAt))
          return cached.value
        if (now.isBefore(retryAt)) {
          if (cached != null && now.isBefore(cached.expiresAt)) return cached.value
          throw failure
        }
        try {
          val fresh = acquire()
          val acquired = clock.instant()
          if (fresh == null || !acquired.isBefore(fresh.expiresAt))
            throw new CredentialException("Acquired credential is already expired")
          val adaptive = Duration.between(acquired, fresh.expiresAt).dividedBy(5)
          val window = if (refreshBefore.compareTo(adaptive) < 0) refreshBefore else adaptive
          cached = fresh
          refreshAt = fresh.expiresAt.minus(window)
          retryAt = Instant.MIN
          failure = null
          fresh.value
        } catch {
          case NonFatal(error) =>
            failure = error match {
              case e: CredentialException => e
              case _ => new CredentialException("Credential acquisition failed")
            }
            retryAt = clock.instant().plus(retryDelay)
            if (cached != null && clock.instant().isBefore(cached.expiresAt)) cached.value
            else throw failure
        }
      }
    }
  }
}
