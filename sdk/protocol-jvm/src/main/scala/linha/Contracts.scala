package linha

import io.circe.{Decoder, Encoder, Json}
import java.io.OutputStream
import java.nio.file.Path
import scala.reflect.ClassTag

trait LinhaEntrypoint[R, E] { def execute(context: LinhaJobContext[E]): R }
trait LinhaJobContext[E] {
  def jobId: String
  def attemptId: String
  def engine: E
  def progress(fraction: Double, message: String): Unit
  def isCancelled: Boolean
  def checkCancelled(): Unit
  def results: ResultWriter
  def onCleanup(action: () => Unit): Unit
}
trait ResultWriter {
  def dataset(name: String, format: String)(
      write: DatasetLocation => Iterator[DatasetPart]
  ): DatasetResult =
    throw new UnsupportedOperationException("dataset writer unavailable")
  def file(name: String, contentType: String)(write: Path => Unit): FileResult
  def stream(name: String, contentType: String)(write: OutputStream => Unit): FileResult
}
final case class ResultFile(
    allocationId: String,
    name: String,
    size: Long,
    sha256: String,
    contentType: String
)
final case class FileResult(files: Vector[ResultFile])
final case class DatasetPart(path: String, size: Long, sha256: String)
final case class DatasetManifest(
    allocationId: String,
    format: String,
    partCount: Long,
    totalBytes: Long,
    manifest: ResultFile
)
final case class DatasetLocation(
    allocationId: String,
    uri: String,
    hadoopOptions: Map[String, String],
    expiresAt: Option[String],
    probePath: Option[String],
    probeValue: Option[String]
)
final case class DatasetResult(files: Vector[ResultFile], dataset: Option[DatasetManifest] = None)
final case class ResultDescriptor(kind: String, schema: String, version: Int)
final case class Capability(handler: String, version: Int, result: ResultDescriptor)
final case class RawRequest(handler: String, version: Int, payload: Json)
sealed trait ProducedResult
final case class JsonResult(value: Json) extends ProducedResult
final case class ArtifactResult(files: Vector[ResultFile]) extends ProducedResult
final case class DatasetProduced(value: DatasetResult) extends ProducedResult

final case class EntrypointCodec[J, R](
    id: String,
    version: Int,
    descriptor: ResultDescriptor,
    requestEncoder: Encoder[J],
    requestDecoder: Decoder[J],
    resultEncoder: Encoder[R],
    resultDecoder: Decoder[R]
) {
  def capability: Capability = Capability(id, version, descriptor)
  def encodeResult(value: R): ProducedResult = value match {
    case file: FileResult       => ArtifactResult(file.files)
    case dataset: DatasetResult => DatasetProduced(dataset)
    case _                      => JsonResult(resultEncoder(value))
  }
}
object EntrypointCodec {
  def derived[J: Encoder: Decoder, R: Encoder: Decoder: ClassTag](
      id: String,
      version: Int
  ): EntrypointCodec[J, R] = {
    val resultClass = implicitly[ClassTag[R]].runtimeClass
    val kind =
      if (resultClass == classOf[FileResult]) "file"
      else if (resultClass == classOf[DatasetResult]) "dataset"
      else "json"
    EntrypointCodec(
      id,
      version,
      ResultDescriptor(kind, id + ".result", version),
      implicitly[Encoder[J]],
      implicitly[Decoder[J]],
      implicitly[Encoder[R]],
      implicitly[Decoder[R]]
    )
  }
}
object json extends io.circe.generic.AutoDerivation
