package kube

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// DriverPodName reserves a full 32-character Linha instance ID and role suffix
// within one DNS label, leaving 23 characters for the readable context prefix.
func DriverPodName(contextName, instanceID string) string {
	var slug strings.Builder
	separator := false
	for _, c := range contextName {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(c)
			separator = false
		} else {
			separator = true
		}
	}
	prefix := slug.String()
	if prefix == "" {
		prefix = "context"
	}
	if len(prefix) > 23 {
		prefix = strings.TrimRight(prefix[:23], "-")
	}
	return prefix + "-" + instanceID + "-driver"
}

// MatchesDriverPodName recognizes both readable names and the original
// linha-<instance>-driver form. Role, application ownership and UID checks
// remain necessary before treating a matching name as a worker identity.
func MatchesDriverPodName(name, instanceID string) bool {
	suffix := "-" + instanceID + "-driver"
	return instanceID != "" && len(name) > len(suffix) &&
		strings.HasSuffix(name, suffix) && len(validation.IsDNS1123Label(name)) == 0
}
