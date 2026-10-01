package linha.spark

import org.junit.Test
import org.junit.Assert._
import org.apache.spark.SparkConf
import java.util.concurrent.{CountDownLatch, Executors, TimeUnit}

class SparkRuntimeTest {
  @Test def concurrentSessionsAndCancellation(): Unit = {
    val conf = new SparkConf()
      .setMaster("local[4]")
      .setAppName("Linha conformance")
      .set("spark.ui.enabled", "false")
      .set("spark.driver.host", "127.0.0.1")
      .set("spark.sql.shuffle.partitions", "2")
    val runtime = new SparkRuntime(conf, 2)
    val executor = Executors.newFixedThreadPool(2)
    val slowStarted = new CountDownLatch(1)
    try {
      val slow = executor.submit(new java.util.concurrent.Callable[Boolean] {
        def call(): Boolean = {
          val engine = runtime.open("slow")
          try {
            engine.session.conf.set("spark.sql.shuffle.partitions", "3")
            slowStarted.countDown()
            engine.session.sparkContext
              .parallelize(1 to 20, 2)
              .mapPartitions { values => Thread.sleep(10000); values }
              .count()
            false
          } catch { case _: org.apache.spark.SparkException => true }
          finally runtime.release(engine)
        }
      })
      assertTrue(slowStarted.await(10, TimeUnit.SECONDS))
      val fast = executor.submit(new java.util.concurrent.Callable[Long] {
        def call(): Long = {
          val engine = runtime.open("fast")
          try {
            assertEquals("2", engine.session.conf.get("spark.sql.shuffle.partitions"));
            engine.session.range(0, 100).count()
          } finally runtime.release(engine)
        }
      })
      // Wait until Spark has registered the slow group before targeting its cancellation.
      val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(10)
      while (!slow.isDone && System.nanoTime() < deadline && !fast.isDone) {
        Thread.sleep(100); runtime.cancel("slow")
      }
      runtime.cancel("slow")
      assertEquals(100L, fast.get(30, TimeUnit.SECONDS).longValue())
      assertTrue(
        "cancelled request should fail without stopping the other request",
        slow.get(30, TimeUnit.SECONDS)
      )
      val again = runtime.open("next")
      try {
        assertEquals(5L, again.session.range(0, 5).count())
        val first = again.session.range(0, 7).toDF()
        val second = again.session.newSession().range(0, 7).toDF()
        val releaseFirst = ScopedCache.acquire(first)
        val releaseSecond = ScopedCache.acquire(second)
        releaseFirst()
        assertNotEquals(org.apache.spark.storage.StorageLevel.NONE, second.storageLevel)
        releaseSecond(); releaseSecond()
        assertEquals(org.apache.spark.storage.StorageLevel.NONE, first.storageLevel)
        first.cache()
        val releaseBorrowed = ScopedCache.acquire(first)
        releaseBorrowed()
        assertNotEquals(org.apache.spark.storage.StorageLevel.NONE, first.storageLevel)
        first.unpersist()
      } finally runtime.release(again)
    } finally { executor.shutdownNow(); runtime.close() }
  }
}
