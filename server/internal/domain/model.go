// Package domain defines engine-independent wire contracts and durable job identities.
package domain

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string      { return e.Code + ": " + e.Message }
func Bad(message string) error      { return &Error{"INVALID_ARGUMENT", message, 400} }
func Conflict(message string) error { return &Error{"CONFLICT", message, 409} }

var NotFound = &Error{"NOT_FOUND", "resource not found", 404}
var ClientLeaseExpired = &Error{"CLIENT_LEASE_EXPIRED", "a live client lease is required; attach to this context again", 409}
var Stale = &Error{"STALE_ATTEMPT", "attempt no longer owns this job", 409}

func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type EngineSpec struct {
	Type     string          `json:"type"`
	Version  string          `json:"version"`
	Settings json.RawMessage `json:"settings"`
}
type PathTemplate struct {
	Version  int    `json:"version"`
	Template string `json:"template"`
}
type ResultPolicy struct {
	Type                   string       `json:"type"`
	Root                   string       `json:"root,omitempty"`
	Destination            string       `json:"destination,omitempty"`
	DestinationFingerprint string       `json:"destinationFingerprint,omitempty"`
	Path                   PathTemplate `json:"path"`
	MaxBytes               int64        `json:"maxBytes"`
	RetentionSeconds       int64        `json:"retentionSeconds"`
}
type BackendSpec struct {
	Image   string       `json:"image"`
	Engine  EngineSpec   `json:"engine"`
	Results ResultPolicy `json:"results"`
}
type ClientLease struct {
	ID              string    `json:"id"`
	ClientID        string    `json:"clientId"`
	ExpiresAt       time.Time `json:"expiresAt"`
	DurationSeconds int       `json:"durationSeconds"`
}
type BackendContext struct {
	Version     string       `json:"version"`
	ClientLease *ClientLease `json:"clientLease,omitempty"`
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Owner       string       `json:"-"`
	Spec        BackendSpec  `json:"spec"`
	State       string       `json:"state"`
	Condition   string       `json:"condition,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}
type EnsureRequest struct {
	ClientID      string       `json:"clientId,omitempty"`
	RequestedSpec *BackendSpec `json:"-"`
	Name          string       `json:"name"`
	Spec          BackendSpec  `json:"spec"`
}
type RetryPolicy struct {
	MaxAttempts    int `json:"maxAttempts"`
	BackoffSeconds int `json:"backoffSeconds"`
}
type SubmitRequest struct {
	LeaseID        string          `json:"-"`
	Handler        string          `json:"handler"`
	Version        int             `json:"version"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotencyKey"`
	Retry          RetryPolicy     `json:"retry"`
	Deadline       *time.Time      `json:"deadline,omitempty"`
}
type Progress struct {
	Fraction float64 `json:"fraction"`
	Message  string  `json:"message"`
}
type Failure struct {
	ExceptionType       string `json:"exceptionType,omitempty"`
	StackTrace          string `json:"stackTrace,omitempty"`
	StackTraceTruncated bool   `json:"stackTraceTruncated,omitempty"`
	Phase               string `json:"phase,omitempty"`
	Code                string `json:"code"`
	Message             string `json:"message"`
	Retryable           bool   `json:"retryable"`
}
type ResultDescriptor struct {
	Kind    string `json:"kind"`
	Schema  string `json:"schema"`
	Version int    `json:"version"`
}
type Capability struct {
	Handler string           `json:"handler"`
	Version int              `json:"version"`
	Result  ResultDescriptor `json:"result"`
}
type Job struct {
	ID           string        `json:"id"`
	ContextID    string        `json:"contextId"`
	Request      SubmitRequest `json:"request"`
	State        string        `json:"state"`
	SubmittedAt  time.Time     `json:"submittedAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
	AttemptCount int           `json:"attemptCount"`
	Progress     *Progress     `json:"progress,omitempty"`
	Failure      *Failure      `json:"failure,omitempty"`
	Result       *Result       `json:"result,omitempty"`
}
type JobPage struct {
	Items      []Job  `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
type Worker struct {
	ID           string       `json:"id"`
	ContextID    string       `json:"contextId"`
	Incarnation  string       `json:"incarnation"`
	Capacity     int          `json:"capacity"`
	Capabilities []Capability `json:"capabilities"`
	Draining     bool         `json:"draining"`
}
type Assignment struct {
	LeaseDurationMillis int64            `json:"leaseDurationMillis"`
	Job                 Job              `json:"job"`
	AttemptID           string           `json:"attemptId"`
	Fence               int64            `json:"fence"`
	LeaseExpiresAt      time.Time        `json:"leaseExpiresAt"`
	Policy              ResultPolicy     `json:"results"`
	ResultDescriptor    ResultDescriptor `json:"resultDescriptor"`
}
type AttemptUpdate struct {
	WorkerID    string `json:"workerId"`
	Incarnation string `json:"incarnation"`
	AttemptID   string `json:"attemptId"`
	Fence       int64  `json:"fence"`
}
type OutputRequest struct {
	AttemptUpdate
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	ContentType string `json:"contentType"`
}
type Allocation struct {
	ID          string       `json:"id"`
	JobID       string       `json:"jobId"`
	AttemptID   string       `json:"attemptId"`
	Name        string       `json:"name"`
	Kind        string       `json:"kind"`
	ContentType string       `json:"contentType"`
	Key         string       `json:"key"`
	Policy      ResultPolicy `json:"-"`
	URI         string       `json:"uri,omitempty"`
	UploadURL   string       `json:"uploadUrl,omitempty"`
}
type File struct {
	AllocationID string `json:"allocationId"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	ContentType  string `json:"contentType"`
}
type Result struct {
	Dataset    *Dataset         `json:"dataset,omitempty"`
	Descriptor ResultDescriptor `json:"descriptor"`
	Files      []File           `json:"files"`
	ExpiresAt  time.Time        `json:"expiresAt"`
	Expired    bool             `json:"expired"`
}
type Completion struct {
	Dataset *Dataset `json:"dataset,omitempty"`
	AttemptUpdate
	Descriptor ResultDescriptor `json:"descriptor"`
	Files      []File           `json:"files"`
}
type Attempt struct {
	ID         string     `json:"id"`
	Number     int        `json:"number"`
	State      string     `json:"state"`
	WorkerID   string     `json:"workerId"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Failure    *Failure   `json:"failure,omitempty"`
}

// Repository methods form the transaction boundary; successful mutations are durable.
type Repository interface {
	Ensure(context.Context, string, EnsureRequest) (BackendContext, error)
	Context(context.Context, string, string) (BackendContext, error)
	Attach(context.Context, string, string, string) (BackendContext, error)
	RenewClient(context.Context, string, string, string) (ClientLease, error)
	ReleaseClient(context.Context, string, string, string) error
	Submit(context.Context, string, string, SubmitRequest) (Job, error)
	Job(context.Context, string, string) (Job, error)
	List(context.Context, string, string, string, string, int) (JobPage, error)
	Attempts(context.Context, string, string) ([]Attempt, error)
	Cancel(context.Context, string, string) (Job, error)
	Register(context.Context, Worker) error
	Heartbeat(context.Context, string, string) (bool, error)
	Claim(context.Context, string, string) (*Assignment, error)
	Renew(context.Context, string, AttemptUpdate, *Progress) (time.Time, error)
	Fail(context.Context, string, AttemptUpdate, Failure) error
	Allocate(context.Context, string, OutputRequest) (Allocation, error)
	Output(context.Context, string, AttemptUpdate, string) (Allocation, error)
	Complete(context.Context, string, Completion) (Result, error)
	ResultOutput(context.Context, string, string, string) (Allocation, File, error)
	Recover(context.Context) error
	Ping(context.Context) error
}

func Decode[T any](data []byte) (T, error) { var t T; err := json.Unmarshal(data, &t); return t, err }
func JSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("invalid internal value: %v", err))
	}
	return b
}

// ConfigVersion hashes JSON values, independent of object key order and whitespace.
func ConfigVersion(spec BackendSpec) string {
	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(JSON(spec)))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		panic(err)
	}
	hash := sha256.Sum256(JSON(canonical))
	return hex.EncodeToString(hash[:])
}
