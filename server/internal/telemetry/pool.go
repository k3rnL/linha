package telemetry

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type LoopState struct {
	Outcome         string    `json:"outcome"`
	ObservedAt      time.Time `json:"observedAt"`
	LastSuccess     time.Time `json:"lastSuccess"`
	DurationSeconds float64   `json:"durationSeconds"`
}

func (m *Monitor) Loops() map[string]LoopState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]LoopState, len(m.loops))
	for k, v := range m.loops {
		out[k] = v
	}
	return out
}

// Query tracing is intentionally empty: SQL and job identifiers are not labels.
func (m *Monitor) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (m *Monitor) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (m *Monitor) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return ctx
}
func (m *Monitor) TraceAcquireEnd(_ context.Context, _ *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	if data.Err == nil {
		return
	}
	reason := "error"
	if errors.Is(data.Err, context.DeadlineExceeded) {
		reason = "timeout"
	} else if errors.Is(data.Err, context.Canceled) {
		reason = "cancel"
	}
	m.Add("linha_database_pool_acquire_failures_total", 1, reason)
}
