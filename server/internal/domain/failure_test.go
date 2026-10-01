package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFailureCompatibilityAndBounds(t *testing.T) {
	var old Failure
	if err := json.Unmarshal([]byte(`{"code":"HANDLER_FAILED","message":"Java heap space","retryable":false}`), &old); err != nil || old.Validate() != nil {
		t.Fatal(old, err)
	}
	if strings.Contains(string(JSON(old)), "stackTrace") {
		t.Fatal("fabricated legacy diagnostics")
	}
	base := Failure{Code: "HANDLER_FAILED", Message: "original", ExceptionType: "java.lang.IllegalStateException", Phase: "handler", StackTrace: strings.Repeat("x", 65536)}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Failure){func(f *Failure) { f.StackTrace += "x" }, func(f *Failure) { f.Message = strings.Repeat("é", 4097) }, func(f *Failure) { f.ExceptionType = strings.Repeat("x", 1025) }, func(f *Failure) { f.Phase = "anything" }} {
		f := base
		mutate(&f)
		if f.Validate() == nil {
			t.Fatal("invalid diagnostics accepted")
		}
	}
}
