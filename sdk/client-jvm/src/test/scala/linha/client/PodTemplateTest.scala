package linha.client

import io.circe.Json
import io.circe.parser.parse
import java.nio.file.{Files, Paths}
import org.junit.Test
import org.junit.Assert._

class PodTemplateTest {
  @Test def sharedMergeFixtures(): Unit = {
    val fixtures = parse(
      new String(Files.readAllBytes(Paths.get("../../api/fixtures/pod-templates.json")), "UTF-8")
    ).fold(throw _, identity).asArray.get
    fixtures.foreach { fixture =>
      val c = fixture.hcursor
      def field(name: String): Json = c.get[Json](name).fold(throw _, identity)
      val base = PodTemplate.fromJson(field("base"))
      assertEquals(field("expected"), base.merge(PodTemplate.fromJson(field("override"))).json)
      assertEquals(field("base"), base.json)
    }
  }
  @Test def yamlAndHelpersUseRawContract(): Unit = {
    val loaded = PodTemplate.fromYaml("""metadata:
  labels: {app: metoc}
spec:
  containers:
    - name: main
      env: [{name: MODE, value: initial}]
""")
    val configured = loaded
      .withEnv("MODE", "final")
      .withSecretEnv("ACCESS_KEY", "storage", "access")
      .withConfigMap("config", "app-v1", "/etc/app")
      .withPVC("data", "data-pvc", "/data")
      .withSecretVolume("secret", "storage", "/etc/storage")
      .withImagePullSecret("pull")
    val templates = SparkPodTemplates(configured, configured)
    val settings = templates.withSettings(Json.obj("drivers" -> Json.obj()))
    assertEquals(
      configured.json,
      settings.hcursor
        .downField("kubernetes")
        .get[Json]("driverPodTemplate")
        .fold(throw _, identity)
    )
    val main = configured.json.hcursor.downField("spec").downField("containers").downArray
    assertEquals(
      "final",
      main.downField("env").downArray.get[String]("value").fold(throw _, identity)
    )
    assertEquals(
      3,
      configured.json.hcursor
        .downField("spec")
        .get[Vector[Json]]("volumes")
        .fold(throw _, identity)
        .size
    )
    val file = Files.createTempFile("linha-template", ".yaml")
    try {
      Files.write(file, configured.json.noSpaces.getBytes("UTF-8"));
      assertEquals(configured.json, PodTemplate.fromFile(file).json)
    } finally Files.delete(file)
  }
  @Test def rejectAmbiguousAndExecutableYaml(): Unit = {
    Seq(
      "spec: {}\nspec: {}",
      "!!java.net.URL [https://example.invalid]",
      "spec: &x {volumes: [*x]}",
      "spec: {containers: [{name: main}, {name: main}]}",
      "spec: null",
      "a" * (PodTemplate.maxBytes + 1)
    ).foreach { data =>
      var failed = false
      try PodTemplate.fromYaml(data)
      catch { case _: RuntimeException => failed = true }
      assertTrue("invalid template accepted", failed)
    }
  }
}
