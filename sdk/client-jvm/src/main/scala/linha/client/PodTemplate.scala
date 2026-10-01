package linha.client

import io.circe.{Json, JsonObject}
import io.circe.syntax._
import java.nio.charset.StandardCharsets.UTF_8
import java.nio.file.{Files, Path}
import org.snakeyaml.engine.v2.api.{Load, LoadSettings}
import org.snakeyaml.engine.v2.schema.JsonSchema
import scala.collection.JavaConverters._

/** Immutable native Kubernetes template; the server validates lifecycle fields. */
final class PodTemplate private (val json: Json) {
  def merge(overrides: PodTemplate): PodTemplate =
    PodTemplate.fromJson(PodTemplate.merge(json, overrides.json))
  def withImagePullSecret(name: String): PodTemplate = merge(
    PodTemplate.fromJson(
      Json.obj("spec" -> Json.obj("imagePullSecrets" -> Json.arr(Json.obj("name" -> name.asJson))))
    )
  )
  def withEnv(name: String, value: String): PodTemplate = main(
    Json.obj("env" -> Json.arr(Json.obj("name" -> name.asJson, "value" -> value.asJson)))
  )
  def withSecretEnv(name: String, secret: String, key: String): PodTemplate = main(
    Json.obj(
      "env" -> Json.arr(
        Json.obj(
          "name" -> name.asJson,
          "valueFrom" -> Json.obj(
            "secretKeyRef" -> Json.obj("name" -> secret.asJson, "key" -> key.asJson)
          )
        )
      )
    )
  )
  def withConfigMap(name: String, configMap: String, mountPath: String): PodTemplate = volume(
    name,
    Json.obj("configMap" -> Json.obj("name" -> configMap.asJson)),
    mountPath,
    readOnly = true
  )
  def withSecretVolume(name: String, secret: String, mountPath: String): PodTemplate = volume(
    name,
    Json.obj("secret" -> Json.obj("secretName" -> secret.asJson)),
    mountPath,
    readOnly = true
  )
  def withPVC(
      name: String,
      claim: String,
      mountPath: String,
      readOnly: Boolean = false
  ): PodTemplate = volume(
    name,
    Json.obj("persistentVolumeClaim" -> Json.obj("claimName" -> claim.asJson)),
    mountPath,
    readOnly
  )
  private def volume(name: String, source: Json, path: String, readOnly: Boolean): PodTemplate =
    merge(
      PodTemplate.fromJson(
        Json.obj(
          "spec" -> Json.obj(
            "volumes" -> Json.arr(source.deepMerge(Json.obj("name" -> name.asJson)))
          )
        )
      )
    )
      .main(
        Json.obj(
          "volumeMounts" -> Json.arr(
            Json.obj(
              "name" -> name.asJson,
              "mountPath" -> path.asJson,
              "readOnly" -> readOnly.asJson
            )
          )
        )
      )
  private def main(fields: Json): PodTemplate = merge(
    PodTemplate.fromJson(
      Json.obj(
        "spec" -> Json.obj(
          "containers" -> Json.arr(fields.deepMerge(Json.obj("name" -> "main".asJson)))
        )
      )
    )
  )
}
object PodTemplate {
  val maxBytes: Int = 64 * 1024
  def empty: PodTemplate = fromJson(Json.obj())
  def fromJson(json: Json): PodTemplate = {
    require(json.isObject, "Pod template must be an object")
    require(json.noSpaces.getBytes(UTF_8).length <= maxBytes, "Pod template exceeds 64 KiB")
    validate(json, "template", 0)
    require(
      json.asObject.get.keys.forall(Set("metadata", "spec")),
      "Pod template contains only metadata and spec"
    )
    new PodTemplate(json)
  }

  /** YAML is parsed on the client. No tags can instantiate classes or read env/files. */
  def fromYaml(yaml: String): PodTemplate = {
    require(yaml.getBytes(UTF_8).length <= maxBytes, "Pod template exceeds 64 KiB")
    val settings = LoadSettings
      .builder()
      .setSchema(new JsonSchema())
      .setAllowDuplicateKeys(false)
      .setAllowRecursiveKeys(false)
      .setMaxAliasesForCollections(0)
      .setCodePointLimit(maxBytes)
      .build()
    fromJson(toJson(new Load(settings).loadFromString(yaml), 0))
  }

  /** Both JSON and YAML are accepted; file contents, never a local path, cross the wire. */
  def fromFile(path: Path): PodTemplate = {
    val input = Files.newInputStream(path)
    val bytes =
      try input.readNBytes(maxBytes + 1)
      finally input.close()
    require(bytes.length <= maxBytes, "Pod template exceeds 64 KiB")
    fromYaml(new String(bytes, UTF_8))
  }
  private def toJson(value: Any, depth: Int): Json = {
    require(depth < 48, "Pod template nesting is too deep")
    value match {
      case null                   => Json.Null
      case x: String              => x.asJson
      case x: java.lang.Boolean   => x.booleanValue.asJson
      case x: java.lang.Number    => io.circe.parser.parse(x.toString).fold(throw _, identity)
      case x: java.util.Map[_, _] =>
        Json.fromFields(x.asScala.iterator.map { case (key, child) =>
          require(key.isInstanceOf[String], "Pod template keys must be strings")
          key.asInstanceOf[String] -> toJson(child, depth + 1)
        }.toVector)
      case x: java.util.List[_] => Json.fromValues(x.asScala.map(toJson(_, depth + 1)))
      case _                    => throw new IllegalArgumentException("Unsupported YAML value")
    }
  }
  private def identityKey(key: String): Option[String] = key match {
    case "containers" | "initContainers" | "env" | "volumes" | "imagePullSecrets" => Some("name")
    case "volumeMounts" => Some("mountPath")
    case _              => None
  }
  private def validate(value: Json, path: String, depth: Int): Unit = {
    require(depth < 48, "Pod template nesting is too deep")
    require(!value.isNull, path + ": omit null fields")
    value.asObject.foreach(_.toVector.foreach { case (key, child) =>
      identityKey(key)
        .filter { _ =>
          if (key == "env" || key == "volumeMounts")
            path.endsWith(".spec.containers") || path.endsWith(".spec.initContainers")
          else path.endsWith(".spec")
        }
        .foreach { id =>
          val values = child.asArray
            .getOrElse(throw new IllegalArgumentException(path + "." + key + " must be an array"))
          val names = values.map(_.hcursor.get[String](id).fold(throw _, identity))
          require(
            names.forall(_.nonEmpty) && names.distinct.size == names.size,
            path + "." + key + " needs unique " + id
          )
        }
      validate(child, path + "." + key, depth + 1)
    })
    value.asArray.foreach(_.foreach(validate(_, path, depth + 1)))
  }
  private[client] def merge(base: Json, overrideValue: Json): Json = {
    val previous = base.asObject.getOrElse(JsonObject.empty)
    val fields =
      overrideValue.asObject.getOrElse(throw new IllegalArgumentException("object required"))
    Json.fromJsonObject(fields.toVector.foldLeft(previous) { case (result, (key, value)) =>
      val old = result(key).getOrElse(Json.Null)
      val merged =
        if (value.isObject) merge(old, value)
        else
          identityKey(key).filter(_ => value.isArray) match {
            case Some(id) =>
              val updated =
                value.asArray.get.foldLeft(old.asArray.getOrElse(Vector.empty)) { (items, entry) =>
                  val name = entry.hcursor.get[String](id).fold(throw _, identity)
                  val index = items.indexWhere(_.hcursor.get[String](id).toOption.contains(name))
                  if (index < 0) items :+ entry
                  else
                    items.updated(
                      index,
                      if (key == "containers" || key == "initContainers") merge(items(index), entry)
                      else entry
                    )
                }
              Json.fromValues(updated)
            case None => value
          }
      result.add(key, merged)
    })
  }
}

final case class SparkPodTemplates(
    driver: PodTemplate = PodTemplate.empty,
    executor: PodTemplate = PodTemplate.empty
) {
  def json: Json =
    Json.obj("driverPodTemplate" -> driver.json, "executorPodTemplate" -> executor.json)
  def withSettings(settings: Json): Json = {
    require(settings.isObject, "Spark settings must be an object")
    Json.fromJsonObject(settings.asObject.get.add("kubernetes", json))
  }
}
