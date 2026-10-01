package linha.example

import linha.client._
import io.circe.Json
import io.circe.syntax._

/** Use this specification with client.ensureBackend("metoc", backendSpec). */
object PodTemplatesExample {
  val common: PodTemplate = PodTemplate.empty
    .withConfigMap("app-config", "metoc-config-v1", "/etc/metoc")
    .withSecretEnv("APP_PASSWORD", "metoc-app", "password")
    .withImagePullSecret("metoc-registry")

  val settings: Json = SparkPodTemplates(
    driver = common.withEnv("APP_ROLE", "driver"),
    executor = common.withEnv("APP_ROLE", "executor")
  ).withSettings(
    Json.obj(
      "application" -> Json.obj(
        "mainClass" -> "linha.example.SparkMain".asJson,
        "mainApplicationFile" -> "local:///opt/linha/linha-examples.jar".asJson
      ),
      "drivers" -> Json.obj("minDrivers" -> 1.asJson, "maxDrivers" -> 2.asJson),
      "executors" -> Json.obj("instances" -> 2.asJson)
    )
  )
  val backendSpec: Json = Json.obj(
    "image" -> "registry.example/metoc/worker:1".asJson,
    "engine" -> Json
      .obj("type" -> "spark".asJson, "version" -> "3.5.6".asJson, "settings" -> settings),
    "results" -> ResultStorage.Local("/var/lib/linha/results")
  )
}
