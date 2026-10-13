# Result expiry and physical cleanup

A job ID identifies durable metadata. Result retention controls how long its
payload remains available. These lifetimes are separate: an expired payload does
not erase status, attempt history, completion receipts or idempotency records.

Expiry is enforced at retrieval even when physical cleanup is disabled.
`results.cleanup.enabled` is an independent opt-in deletion policy. Eligible
allocations must belong to terminal attempts and be unprotected by a retained
result, delegated writer or grace period.

Cleanup uses persisted allocation identity, not a scan of arbitrary user roots
or buckets. Repeated sweeps tolerate partial failures. S3 object versions,
backups and filesystem snapshots remain subject to external policies.

Set external storage lifecycle rules so they do not remove retained payloads
before Linha's promised period. A missing object retains its metadata and returns
an explicit storage failure; it is not silently treated as an empty result.

For the configuration and operational steps, see
[bound storage and enable cleanup](dataset-results.md#bound-storage-and-enable-cleanup).
