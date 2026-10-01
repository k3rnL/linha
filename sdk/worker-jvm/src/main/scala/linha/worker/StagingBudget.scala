package linha.worker

import linha.LinhaException
private[worker] object StagingBudget {
  private val maximum = sys.env.get("LINHA_MAX_STAGING_BYTES").map(_.toLong).getOrElse(5L << 30)
  require(maximum > 0, "LINHA_MAX_STAGING_BYTES must be positive")
  private var used = 0L
  def reserve(bytes: Long): () => Unit = synchronized {
    if (bytes < 0 || bytes > maximum - used)
      throw LinhaException(
        429,
        "STAGING_LIMIT_EXCEEDED",
        "worker concurrent staging budget exhausted"
      )
    used += bytes
    val done = new java.util.concurrent.atomic.AtomicBoolean(false)
    () => if (done.compareAndSet(false, true)) synchronized { used -= bytes }
  }
}
