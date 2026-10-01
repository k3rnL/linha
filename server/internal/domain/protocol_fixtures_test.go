package domain

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestCanonicalProtocolFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../../api/fixtures/canonical-protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Spec             BackendSpec
		ConfigVersion    string
		Request          SubmitRequest
		ResultDescriptor ResultDescriptor
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		if got := ConfigVersion(f.Spec); got != f.ConfigVersion {
			t.Fatal(got, f.ConfigVersion)
		}
		round, err := Decode[SubmitRequest](JSON(f.Request))
		if err != nil || !samePayload(round.Payload, f.Request.Payload) || round.Handler != "fixture.count" {
			t.Fatal(round, err)
		}
		if f.ResultDescriptor != (ResultDescriptor{Kind: "json", Schema: "fixture.count.result", Version: 1}) {
			t.Fatal(f.ResultDescriptor)
		}
	}
}

func samePayload(a, b []byte) bool {
	var x, y any
	json.Unmarshal(a, &x)
	json.Unmarshal(b, &y)
	return reflect.DeepEqual(x, y)
}
