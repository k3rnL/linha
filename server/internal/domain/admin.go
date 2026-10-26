package domain

import (
	"context"
	"encoding/json"
	"time"
)

type AdminSubmitRequest struct {
	SubmitRequest
	ReplayedFrom      string `json:"replayedFrom,omitempty"`
	ActivateIfStopped bool   `json:"activateIfStopped,omitempty"`
}
type AdminJobSelection struct {
	ID        string `json:"id"`
	ContextID string `json:"contextId"`
}
type AdminBatchRequest struct {
	Action          string              `json:"action"`
	Jobs            []AdminJobSelection `json:"jobs"`
	TargetContextID string              `json:"targetContextId,omitempty"`
}
type AdminBatchJob struct {
	ID           string    `json:"id"`
	ContextID    string    `json:"contextId"`
	State        string    `json:"state"`
	AttemptCount int       `json:"attemptCount"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type AdminBatchItem struct {
	ID      string         `json:"id"`
	Outcome string         `json:"outcome"`
	Job     *AdminBatchJob `json:"job,omitempty"`
	Error   *Error         `json:"error,omitempty"`
}
type AdminBatchResult struct {
	Items []AdminBatchItem `json:"items"`
}
type AuditRecord struct {
	SourceContextID string    `json:"sourceContextId,omitempty"`
	ID              string    `json:"id"`
	Actor           string    `json:"actor"`
	Action          string    `json:"action"`
	Owner           string    `json:"owner"`
	ContextID       string    `json:"contextId"`
	JobID           string    `json:"jobId"`
	ReplayedFrom    string    `json:"replayedFrom,omitempty"`
	AcceptedAt      time.Time `json:"acceptedAt"`
	Outcome         string    `json:"outcome"`
}
type WorkerObservation struct {
	Worker
	HeartbeatAt   time.Time `json:"heartbeatAt"`
	State         string    `json:"state"`
	OccupiedSlots int       `json:"occupiedSlots"`
}
type AdminRepository interface {
	AdminOverview(context.Context) (AdminOverview, error)
	OperationalRepository
	OperationalWorkers(context.Context, string, string, OperationalFilter) (Page[WorkerObservation], error)
	AdminJobs(context.Context, OperationalFilter) (Page[AdministrativeJob], error)
	AdminJob(context.Context, string) (AdministrativeJob, error)
	AdminSubmit(context.Context, string, string, AdminSubmitRequest) (Job, error)
	AdminCancel(context.Context, string, string) (Job, error)
	AdminBatch(context.Context, string, AdminBatchRequest) (AdminBatchResult, error)
	AdminAudit(context.Context, string, OperationalFilter) (Page[AuditRecord], error)
}
type BrowserSession struct {
	AuthExpiresAt *time.Time
	RefreshCipher []byte
	Hash          string
	Claims        json.RawMessage
	CSRF          string
	ExpiresAt     time.Time
}
type LoginTransaction struct {
	Hash, Nonce, Verifier string
	ExpiresAt             time.Time
}
type BrowserRepository interface {
	SaveLogin(context.Context, LoginTransaction) error
	TakeLogin(context.Context, string) (LoginTransaction, error)
	SaveSession(context.Context, BrowserSession) error
	Session(context.Context, string) (BrowserSession, error)
	RefreshSession(context.Context, string, func(BrowserSession) (BrowserSession, error)) (BrowserSession, error)
	DeleteSession(context.Context, string) error
	ExpireBrowserRecords(context.Context) error
}

// AdminOverview is a coherent, read-only snapshot; detail lists are bounded,
// while every count covers the full selected population.
type AdminOverview struct {
	ObservedAt               time.Time         `json:"observedAt"`
	Since                    time.Time         `json:"since"`
	RunningJobs              int64             `json:"runningJobs"`
	QueuedJobs               int64             `json:"queuedJobs"`
	RetryingJobs             int64             `json:"retryingJobs"`
	CancellingJobs           int64             `json:"cancellingJobs"`
	SucceededJobs24h         int64             `json:"succeededJobs24h"`
	FailedJobs24h            int64             `json:"failedJobs24h"`
	CancelledJobs24h         int64             `json:"cancelledJobs24h"`
	OldestQueuedSeconds      float64           `json:"oldestQueuedSeconds"`
	MeanProcessingSeconds24h *float64          `json:"meanProcessingSeconds24h"`
	ActiveContexts           int64             `json:"activeContexts"`
	LiveClients              int64             `json:"liveClients"`
	ReadyWorkers             int64             `json:"readyWorkers"`
	WorkerSlots              int64             `json:"workerSlots"`
	OccupiedSlots            int64             `json:"occupiedSlots"`
	ServersReady             int64             `json:"serversReady"`
	ServersUnready           int64             `json:"serversUnready"`
	ServersStale             int64             `json:"serversStale"`
	ActiveContextDetails     []OverviewContext `json:"activeContextDetails"`
	RecentFailures           []OverviewFailure `json:"recentFailures"`
}
type OverviewContext struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	State       string `json:"state"`
	Condition   string `json:"condition,omitempty"`
	RunningJobs int64  `json:"runningJobs"`
	QueuedJobs  int64  `json:"queuedJobs"`
}
type OverviewFailure struct {
	ID          string    `json:"id"`
	ContextID   string    `json:"contextId"`
	ContextName string    `json:"contextName"`
	Owner       string    `json:"owner"`
	Handler     string    `json:"handler"`
	Message     string    `json:"message"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
