package main

import (
	"context"
	"linha/server/internal/domain"
	"linha/server/internal/httpapi"
	"linha/server/internal/postgres"
	"linha/server/internal/telemetry"
	"log/slog"
	"os"
	"time"
)

var revision = "unknown"

func observeRuntime(ctx context.Context, repo *postgres.Store, api *httpapi.Server, m *telemetry.Monitor, metrics bool) {
	id := domain.ID()
	started := time.Now()
	pod := os.Getenv("HOSTNAME")
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		start := time.Now()
		call, cancel := context.WithTimeout(ctx, 2*time.Second)
		dbErr := repo.Ping(call)
		cancel()
		localErr := error(nil)
		m.Set("linha_dependency_available", boolValue(dbErr == nil), "database")
		if api.Storage.Local != nil {
			probe, done := context.WithTimeout(ctx, 2*time.Second)
			localErr = api.Storage.Local.CheckContext(probe)
			done()
			m.Set("linha_dependency_available", boolValue(localErr == nil), "local_storage")
		}
		ready := dbErr == nil && localErr == nil
		m.Set("linha_server_ready", boolValue(ready))
		err := dbErr
		if err == nil {
			err = localErr
		}
		m.Collection("readiness", start, err)
		state := "unready"
		if ready {
			state = "ready"
		}
		heartbeat := domain.ServerObservation{ID: id, Pod: pod, Version: version, StartedAt: started, State: state, Detail: domain.JSON(map[string]any{"ready": ready, "database": dbErr == nil, "localStorageConfigured": api.Storage.Local != nil, "localStorage": api.Storage.Local != nil && localErr == nil, "loops": m.Loops(), "observedAt": time.Now()})}
		call, cancel = context.WithTimeout(ctx, 2*time.Second)
		start = time.Now()
		err = repo.ServerHeartbeat(call, heartbeat)
		if err == nil {
			_, err = repo.Pool.Exec(call, "DELETE FROM linha_servers WHERE heartbeat_at<clock_timestamp()-interval '24 hours'")
		}
		cancel()
		m.Loop("heartbeat", start, err)
		call, cancel = context.WithTimeout(ctx, 2*time.Second)
		err = repo.RecordClientExpiries(call)
		if err == nil {
			err = repo.ExpireBrowserRecords(call)
		}
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("lease expiry accounting unavailable", "error", err)
		}
		if metrics {
			if err = m.Refresh(ctx, repo); err != nil && ctx.Err() == nil {
				slog.Debug("metric snapshot unavailable", "error", err)
			}
		}
		stats := repo.Pool.Stat()
		m.Set("linha_database_pool_connections", float64(stats.AcquiredConns()), "acquired")
		m.Set("linha_database_pool_connections", float64(stats.IdleConns()), "idle")
		m.Set("linha_database_pool_connections", float64(stats.ConstructingConns()), "constructing")
		m.Set("linha_database_pool_max_connections", float64(stats.MaxConns()))
		// pgx pool exposes cumulative monotonic statistics directly.
		m.SetPoolWait(stats.EmptyAcquireWaitTime().Seconds())
		reserved, limit := api.Staging.Stats()
		m.Set("linha_staging_reserved_bytes", float64(reserved))
		m.Set("linha_staging_limit_bytes", float64(limit))
		select {
		case <-ctx.Done():
			heartbeat.State = "stopping"
			call, cancel = context.WithTimeout(context.Background(), 2*time.Second)
			_ = repo.ServerHeartbeat(call, heartbeat)
			cancel()
			return
		case <-tick.C:
		}
	}
}
func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
