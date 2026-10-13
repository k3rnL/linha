package linha.client.auth

import com.sun.net.httpserver.{HttpsServer, HttpsConfigurator}
import java.net.InetSocketAddress
import java.nio.file.Files
import java.security.KeyStore
import javax.net.ssl.{KeyManagerFactory, SSLContext}
import linha.CredentialException
import org.junit.Test
import org.junit.Assert._

class IdentityTLSTest {
  @Test def strictCustomCAAndIsolatedBypass(): Unit = {
    val directory = Files.createTempDirectory("linha-tls-test")
    val storeFile = directory.resolve("identity.p12")
    val caFile = directory.resolve("ca.pem")
    val keytool =
      java.nio.file.Paths.get(System.getProperty("java.home"), "bin", "keytool").toString
    def run(args: String*): Unit = {
      val process = new ProcessBuilder((Seq(keytool) ++ args): _*)
        .redirectErrorStream(true)
        .redirectOutput(ProcessBuilder.Redirect.DISCARD)
        .start()
      assertEquals(0, process.waitFor())
    }
    run(
      "-genkeypair",
      "-alias",
      "identity",
      "-keyalg",
      "RSA",
      "-keystore",
      storeFile.toString,
      "-storetype",
      "PKCS12",
      "-storepass",
      "test-only-password",
      "-dname",
      "CN=localhost",
      "-ext",
      "SAN=DNS:localhost",
      "-validity",
      "1"
    )
    run(
      "-exportcert",
      "-rfc",
      "-alias",
      "identity",
      "-keystore",
      storeFile.toString,
      "-storepass",
      "test-only-password",
      "-file",
      caFile.toString
    )
    val store = KeyStore.getInstance("PKCS12")
    val input = Files.newInputStream(storeFile)
    try store.load(input, "test-only-password".toCharArray)
    finally input.close()
    val keys = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm)
    keys.init(store, "test-only-password".toCharArray)
    val context = SSLContext.getInstance("TLS"); context.init(keys.getKeyManagers, null, null)
    val server = HttpsServer.create(new InetSocketAddress("127.0.0.1", 0), 0)
    server.setHttpsConfigurator(new HttpsConfigurator(context))
    server.createContext(
      "/token",
      e => {
        val bytes =
          "{\"token_type\":\"Bearer\",\"access_token\":\"tls-token\",\"expires_in\":60}".getBytes(
            "UTF-8"
          )
        e.sendResponseHeaders(200, bytes.length); e.getResponseBody.write(bytes); e.close()
      }
    )
    server.start()
    def provider(host: String, options: IdentityHttpOptions) =
      CredentialProviders.oauthClientCredentials(
        "https://" + host + ":" + server.getAddress.getPort + "/token",
        "client",
        () => "private-secret",
        options = options
      )
    def rejects(options: IdentityHttpOptions, host: String = "localhost"): Unit = {
      try { provider(host, options).bearerToken(); fail("untrusted endpoint accepted") }
      catch {
        case e: CredentialException =>
          assertNull(e.getCause); assertFalse(e.toString.contains("private-secret"))
      }
    }
    try {
      rejects(IdentityHttpOptions())
      assertEquals(
        "tls-token",
        provider("localhost", IdentityHttpOptions(caFile = Some(caFile))).bearerToken()
      )
      rejects(IdentityHttpOptions(caFile = Some(caFile)), "127.0.0.1")
      assertEquals(
        "tls-token",
        provider("127.0.0.1", IdentityHttpOptions(insecureSkipVerify = true)).bearerToken()
      )
      rejects(IdentityHttpOptions())
    } finally {
      server.stop(0); Files.deleteIfExists(storeFile); Files.deleteIfExists(caFile);
      Files.deleteIfExists(directory)
    }
  }
}
