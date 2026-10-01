package domain

import (
	"context"
	"time"
)

// Dataset metadata is bounded independently of the number of distributed parts.
type Dataset struct {
	AllocationID string `json:"allocationId"`
	Format       string `json:"format"`
	PartCount    int64  `json:"partCount"`
	TotalBytes   int64  `json:"totalBytes"`
	Manifest     File   `json:"manifest"`
}
type DatasetPart struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type PartPage struct {
	Items      []DatasetPart `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}
type DatasetAccess struct {
	AllocationID  string            `json:"allocationId"`
	URI           string            `json:"uri"`
	HadoopOptions map[string]string `json:"hadoopOptions"`
	ExpiresAt     time.Time         `json:"expiresAt,omitempty"`
	ProbePath     string            `json:"probePath,omitempty"`
	ProbeValue    string            `json:"probeValue,omitempty"`
}
type DatasetRepository interface {
	DelegateOutput(context.Context, string, AttemptUpdate, string, time.Time) error
	RegisterParts(context.Context, string, AttemptUpdate, string, []DatasetPart) error
	DatasetParts(context.Context, string, string, int) (PartPage, error)
	FreezeDataset(context.Context, string, AttemptUpdate, string) (int64, int64, error)
	SealDataset(context.Context, string, AttemptUpdate, Dataset) error
	PublishedDataset(context.Context, string, string) (Allocation, Dataset, error)
}
