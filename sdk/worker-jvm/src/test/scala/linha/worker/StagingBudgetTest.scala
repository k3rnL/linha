package linha.worker
import linha.LinhaException
import org.junit.Test
import org.junit.Assert._
class StagingBudgetTest {
  @Test def reservesConcurrentSpaceAndReleasesOnce(): Unit = {
    val release = StagingBudget.reserve(5L << 30)
    try {
      try { StagingBudget.reserve(1); fail("overcommitted staging") }
      catch { case e: LinhaException => assertEquals("STAGING_LIMIT_EXCEEDED", e.code) }
    } finally { release(); release() }
    StagingBudget.reserve(5L << 30)()
  }
}
