package linha.spark

import linha._
import linha.worker._
import org.apache.spark.{SparkConf, SparkContext}
import org.apache.spark.sql.{DataFrame, SparkSession}
import java.util.concurrent.ArrayBlockingQueue
import scala.reflect.ClassTag

final case class Spark(session: SparkSession, private[spark] val pool: String)
trait SparkEntrypoint[R] extends LinhaEntrypoint[R, Spark]

/** One shared SparkContext with bounded request sessions and fixed scheduler pools. */
final class SparkRuntime(conf: SparkConf, capacity: Int) extends AutoCloseable {
  require(capacity > 0)
  private val slots = new ArrayBlockingQueue[String](capacity)
  (0 until capacity).foreach(i => slots.add("linha-slot-" + i))
  private val session =
    SparkSession.builder().config(conf).config("spark.scheduler.mode", "FAIR").getOrCreate()
  private val context = session.sparkContext
  def open(jobId: String): Spark = {
    val pool = slots.take()
    try {
      val request = session.newSession()
      context.setJobGroup(jobId, "Linha request " + jobId, interruptOnCancel = true)
      context.setLocalProperty("spark.scheduler.pool", pool)
      SparkSession.setActiveSession(request)
      Spark(request, pool)
    } catch { case t: Throwable => slots.put(pool); throw t }
  }
  def release(value: Spark): Unit = {
    context.clearJobGroup()
    context.setLocalProperty("spark.scheduler.pool", null)
    SparkSession.clearActiveSession()
    slots.put(value.pool)
  }
  def cancel(jobId: String): Unit = context.cancelJobGroup(jobId)
  def worker(
      baseUrl: String,
      credential: WorkerCredential,
      credentials: Option[CredentialProvider] = None
  ): LinhaWorker[Spark] =
    new LinhaWorker[Spark](baseUrl, credential, open, cancel, release, capacity, credentials)
  def close(): Unit = session.stop()
}
object syntax {
  implicit class SparkResultWriter(private val writer: ResultWriter) extends AnyVal {
    def parquet(
        dataframe: DataFrame,
        name: String,
        partitionBy: Seq[String] = Seq.empty
    ): DatasetResult =
      DistributedResults.parquet(writer, dataframe, name, partitionBy)
  }
  implicit class SparkJobContext(private val context: LinhaJobContext[Spark]) extends AnyVal {
    def spark: SparkSession = context.engine.session
    def cache(dataframe: DataFrame): DataFrame = {
      context.onCleanup(ScopedCache.acquire(dataframe))
      dataframe
    }
    def broadcast[T: ClassTag](value: T): org.apache.spark.broadcast.Broadcast[T] = {
      val broadcast = spark.sparkContext.broadcast(value)
      context.onCleanup(() => broadcast.destroy())
      broadcast
    }
  }
}

/** Identical logical plans may share Spark's cache across request sessions. */
private[spark] object ScopedCache {
  private final class Lease(val frame: DataFrame, var references: Int)
  private val entries = scala.collection.mutable.Map
    .empty[(SparkContext, org.apache.spark.sql.catalyst.plans.logical.LogicalPlan), Lease]
  def acquire(frame: DataFrame): () => Unit = synchronized {
    val key = (frame.sparkSession.sparkContext, frame.queryExecution.analyzed.canonicalized)
    val lease = entries.get(key) match {
      case Some(existing) => existing.references += 1; Some(existing)
      case None if frame.storageLevel != org.apache.spark.storage.StorageLevel.NONE =>
        None // User-owned cache.
      case None =>
        frame.cache(); val created = new Lease(frame, 1); entries(key) = created; Some(created)
    }
    val released = new java.util.concurrent.atomic.AtomicBoolean(false)
    () =>
      if (released.compareAndSet(false, true)) synchronized {
        lease.foreach { owned =>
          owned.references -= 1
          if (owned.references == 0) {
            owned.frame.unpersist(blocking = false); entries.remove(key)
          }
        }
      }
  }
}
