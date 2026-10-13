package linha.example
import linha._
import linha.client._
import linha.client.auth.CredentialProviders
import java.nio.file.Paths
import io.circe.Json
import io.circe.syntax._
import linha.json._
import scala.concurrent.{Await, Future}
import scala.concurrent.duration._
import scala.concurrent.ExecutionContext.Implicits.global
object ClientMain {
  def main(args: Array[String]): Unit = {
    require(args.length == 3, "usage: ClientMain submit|result <url> <root-or-job-id>")
    val credentials = sys.env.get("LINHA_OAUTH_TOKEN_ENDPOINT") match {
      case Some(endpoint) =>
        CredentialProviders.oauthClientCredentials(
          endpoint,
          sys.env("LINHA_OAUTH_CLIENT_ID"),
          CredentialProvider.fileSecret(Paths.get(sys.env("LINHA_OAUTH_CLIENT_SECRET_FILE")))
        )
      case None => CredentialProvider.none // Anonymous local example and restart smoke test.
    }
    val client = new LinhaClient(args(1), credentials)
    if (args(0) == "submit") {
      val accepted = for {
        ctx <- client.ensureBackend(
          "scala-example",
          Json.obj(
            "image" -> "linha-example:dev".asJson,
            "engine" -> Json
              .obj("type" -> "fake".asJson, "version" -> "1".asJson, "settings" -> Json.obj()),
            "results" -> ResultStorage.local(args(2))
          )
        )
        job <- ctx.submit(SumNumbers(Vector(1, 2, 3)), "scala-e2e")
      } yield Json.obj("contextId" -> ctx.id.asJson, "jobId" -> job.id.asJson)
      println(Await.result(accepted, 30.seconds).noSpaces)
    } else if (args(0) == "result") {
      val value = Await.result(client.job(args(2), SumNumbers.codec).result(), 30.seconds)
      println(value.asJson.noSpaces)
    } else throw new IllegalArgumentException("unknown command")
  }
}
