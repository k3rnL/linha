package storage

import (
	"linha/server/internal/domain"
	"sync"
)

// StagingBudget reserves worst-case space before accepting a concurrent upload.
// Limits protect SDK/server-owned staging; arbitrary user callbacks still require
// filesystem quotas for a hard kernel-enforced bound on third-party writes.
type StagingBudget struct {
	mu    sync.Mutex
	used  int64
	Limit int64
}

func (b *StagingBudget) Reserve(bytes int64) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := b.Limit
	if limit == 0 {
		limit = 5 << 30
	}
	if bytes < 0 || bytes > limit-b.used {
		return nil, &domain.Error{Code: "STAGING_LIMIT_EXCEEDED", Message: "concurrent staging byte budget exhausted", Status: 429}
	}
	b.used += bytes
	var once sync.Once
	return func() { once.Do(func() { b.mu.Lock(); b.used -= bytes; b.mu.Unlock() }) }, nil
}

// Stats returns the reservation total without racing concurrent uploads.
func (b *StagingBudget) Stats() (int64, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := b.Limit
	if limit == 0 {
		limit = 5 << 30
	}
	return b.used, limit
}
