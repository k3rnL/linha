# Optional physical result cleanup

Implemented in schema v6 after authorization to complete the platform. Cleanup
remains disabled by default; administrators enable it with
`results.cleanup.enabled: true` or `LINHA_CLEANUP_ENABLED=true`.

See [the implemented cleanup contract](dataset-results.md#limits-and-cleanup)
and [validation evidence](validation.md). Only allocated outputs from terminal
attempts are eligible after their writer authorization and grace period expire.
Retained results and active attempts are protected. PostgreSQL locks coordinate
replicas; retries are idempotent. No user bucket/root is removed. Public job IDs,
completion receipts and idempotency records remain after payload expiry.

S3 object versions, external backups and filesystem snapshots follow their own
retention policies. No production cleanup has been enabled by this development work.
