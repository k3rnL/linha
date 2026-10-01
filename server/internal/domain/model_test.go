package domain

import (
	"encoding/json"
	"testing"
)

func TestConfigurationHashIsCanonicalAndIncludesImage(t *testing.T) {
	a := BackendSpec{Image: "image@sha256:first", Engine: EngineSpec{Type: "spark", Settings: json.RawMessage(`{"b":{"y":2,"x":1},"a":true}`)}}
	b := a
	b.Engine.Settings = json.RawMessage(`{ "a": true, "b": {"x":1, "y":2} }`)
	if ConfigVersion(a) != ConfigVersion(b) {
		t.Fatal("object order changed version")
	}
	b.Image = "image@sha256:second"
	if ConfigVersion(a) == ConfigVersion(b) {
		t.Fatal("image update did not create a new version")
	}
}
