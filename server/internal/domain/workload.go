package domain

import (
	"context"
	"time"
)

type StorageSession struct {
	ID          string        `json:"id"`
	Token       string        `json:"token"`
	Destination string        `json:"destination"`
	Fingerprint string        `json:"-"`
	Root        string        `json:"root"`
	URI         string        `json:"uri"`
	ContextID   string        `json:"-"`
	Owner       string        `json:"-"`
	ClientLease string        `json:"-"`
	JobID       string        `json:"-"`
	Identity    AttemptUpdate `json:"-"`
}
type StorageSessionInput struct {
	Destination string `json:"destination"`
	ClientLease string `json:"clientLease,omitempty"`
	AttemptUpdate
}
type StorageObject struct {
	Path       string    `json:"path"`
	Directory  bool      `json:"directory"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
}
type WorkloadRepository interface {
	CreateStorageSession(context.Context, StorageSession) (StorageSession, error)
	StorageSession(context.Context, string, string) (StorageSession, error)
	RevokeStorageSession(context.Context, string, string) error
	AcquireStorageLock(context.Context, StorageSession, string) error
	RenewStorageLock(context.Context, StorageSession, string) error
	ReleaseStorageLock(context.Context, StorageSession, string) error
	ProtectStorageWrite(context.Context, StorageSession, string, func() error) error
}
