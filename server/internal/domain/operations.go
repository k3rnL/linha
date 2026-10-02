package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Operational reads never create demand or renew an execution/client lease.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
type OperationalFilter struct {
	Owner, Name, Engine, State, ContextID, MetricContext, Cursor string
	Limit                                                        int
	IncludeStopped                                               bool
	From, Until                                                  *time.Time
}
type ContextObservation struct {
	BackendContext
	Owner            string    `json:"owner"`
	MetricContext    string    `json:"metricContext"`
	ObservedAt       time.Time `json:"observedAt"`
	LiveClients      int       `json:"liveClients"`
	QueuedJobs       int       `json:"queuedJobs"`
	RunningJobs      int       `json:"runningJobs"`
	ReadyWorkers     int       `json:"readyWorkers"`
	DesiredInstances int       `json:"desiredInstances"`
}
type ClientObservation struct {
	ID            string     `json:"id"`
	ClientID      string     `json:"clientId"`
	Hostname      string     `json:"hostname,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	LastRenewedAt *time.Time `json:"lastRenewedAt,omitempty"`
	State         string     `json:"state"`
}
type InstanceObservation struct {
	ID           string          `json:"id"`
	ContextID    string          `json:"contextId"`
	Incarnation  string          `json:"incarnation"`
	ResourceKind string          `json:"resourceKind"`
	ResourceUID  string          `json:"resourceUid"`
	State        string          `json:"state"`
	CreatedAt    time.Time       `json:"createdAt"`
	ObservedAt   *time.Time      `json:"observedAt,omitempty"`
	Available    bool            `json:"available"`
	Stale        bool            `json:"stale"`
	Detail       json.RawMessage `json:"detail,omitempty"`
}
type EngineLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type EngineObservation struct {
	Application      string             `json:"application,omitempty"`
	ApplicationState string             `json:"applicationState,omitempty"`
	DriverPod        string             `json:"driverPod,omitempty"`
	DriverPhase      string             `json:"driverPhase,omitempty"`
	ExecutorStates   map[string]int     `json:"executorStates,omitempty"`
	Resources        map[string]float64 `json:"resources,omitempty"`
	Links            []EngineLink       `json:"links,omitempty"`
	UIAddress        string             `json:"uiAddress,omitempty"`
	UIService        string             `json:"uiService,omitempty"`
	UIPort           int                `json:"uiPort,omitempty"`
	Condition        string             `json:"condition,omitempty"`
	Available        bool               `json:"available"`
}
type ServerObservation struct {
	ID          string          `json:"id"`
	Pod         string          `json:"pod"`
	Version     string          `json:"version"`
	StartedAt   time.Time       `json:"startedAt"`
	HeartbeatAt time.Time       `json:"heartbeatAt"`
	State       string          `json:"state"`
	Detail      json.RawMessage `json:"detail"`
}
type AdministrativeJob struct {
	Job
	Owner          string `json:"owner"`
	ContextName    string `json:"contextName"`
	ContextVersion string `json:"contextVersion"`
	MetricContext  string `json:"metricContext"`
	SubmittedBy    string `json:"submittedBy,omitempty"`
	ReplayedFrom   string `json:"replayedFrom,omitempty"`
}
type OperationalRepository interface {
	OperationalContexts(context.Context, OperationalFilter) (Page[ContextObservation], error)
	OperationalContext(context.Context, string, string) (ContextObservation, error)
	OperationalClients(context.Context, string, string, OperationalFilter) (Page[ClientObservation], error)
	OperationalInstances(context.Context, string, string, OperationalFilter) (Page[InstanceObservation], error)
	OperationalServers(context.Context, OperationalFilter) (Page[ServerObservation], error)
}
