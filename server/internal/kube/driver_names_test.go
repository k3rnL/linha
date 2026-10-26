package kube

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestDriverPodNames(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	for _, tt := range []struct{ context, prefix string }{
		{"reporting", "reporting"},
		{"sample-crawler-service", "sample-crawler-service"},
		{"  Reports__EU / DAILY.v2  ", "reports-eu-daily-v2"},
		{"Été / Reports", "t-reports"},
		{"!☀_", "context"},
		{"", "context"},
		{strings.Repeat("x", 128), strings.Repeat("x", 23)},
		{strings.Repeat("x", 22) + "--tail", strings.Repeat("x", 22)},
	} {
		t.Run(tt.context, func(t *testing.T) {
			name := DriverPodName(tt.context, id)
			if name != tt.prefix+"-"+id+"-driver" {
				t.Fatal(name)
			}
			if errors := validation.IsDNS1123Label(name); len(errors) != 0 {
				t.Fatal(name, errors)
			}
			if !MatchesDriverPodName(name, id) || MatchesDriverPodName(name, "different-instance") {
				t.Fatal("instance suffix was not fenced", name)
			}
			if name != DriverPodName(tt.context, id) || name == DriverPodName(tt.context, "fedcba9876543210fedcba9876543210") {
				t.Fatal("unstable or colliding instance name", name)
			}
		})
	}
	if !MatchesDriverPodName("linha-"+id+"-driver", id) {
		t.Fatal("existing operator driver rejected")
	}
	for _, name := range []string{"-" + id + "-driver", "UPPER-" + id + "-driver", "other-driver", strings.Repeat("x", 24) + "-" + id + "-driver"} {
		if MatchesDriverPodName(name, id) {
			t.Fatal("invalid name accepted", name)
		}
	}
}
