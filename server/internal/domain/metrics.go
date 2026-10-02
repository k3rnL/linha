package domain

// MetricSample is an engine-neutral aggregate projection, not an authorization API.
type MetricSample struct {
	Name   string
	Labels map[string]string
	Kind   string
	Bucket string
	Value  float64
}
