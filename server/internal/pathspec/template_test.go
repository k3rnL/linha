package pathspec

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestConformance(t *testing.T) {
	data, err := os.ReadFile("../../../api/fixtures/path-templates.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Template, SubmittedAt, ContextID, RequestID, AttemptID, File, Expected string }
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		stamp, err := time.Parse(time.RFC3339Nano, f.SubmittedAt)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Expand(f.Template, Values{f.ContextID, f.RequestID, f.AttemptID, f.File, stamp})
		if err != nil || got != f.Expected {
			t.Fatalf("%s: got %q (%v), want %q", f.Template, got, err, f.Expected)
		}
	}
}
func TestRejectUnsafe(t *testing.T) {
	for _, s := range []string{"{requestId}/{file}", "{attemptId}/{file}", "../{requestId}/{attemptId}/{file}", "/{requestId}/{attemptId}/{file}", "s3://bucket/{requestId}/{attemptId}/{file}", "{requestId}/{attemptId}/{unknown}/{file}", "{requestId}/{attemptId}/{submittedAt:YYYY}/{file}", "{requestId}/{attemptId}/{file}/suffix", "{requestId}/{attemptId}/{requestId}/{file}", "{requestId}/{attemptId}/%2e%2e/{file}"} {
		if Validate(s) == nil {
			t.Errorf("accepted %s", s)
		}
	}
	for _, s := range []string{"../x", "x/../y", "/tmp/a", "x//y", "x\\y", "_linha/manifest", "%252e%252e/x", "a\x00b"} {
		if Name(s) == nil {
			t.Errorf("accepted name %q", s)
		}
	}
}
