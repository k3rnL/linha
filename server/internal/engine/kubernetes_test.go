package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDriverSecurityConfiguration(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			var pod map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("unexpected method %s", r.Method)
				}
				switch r.URL.Path {
				case "/api/v1/namespaces/isolated/pods":
					if err := json.NewDecoder(r.Body).Decode(&pod); err != nil {
						t.Error(err)
					}
				case "/api/v1/namespaces/isolated/services":
				default:
					t.Errorf("resource escaped namespace: %s", r.URL.Path)
				}
				w.WriteHeader(201)
			}))
			defer server.Close()
			k := &Kubernetes{SecurityDisabled: disabled, Client: &kube.Client{URL: server.URL, HTTP: server.Client(), Namespace: "isolated"}, ServerURL: "http://linha", ServiceAccount: "worker"}
			c := domain.BackendContext{ID: "context", Spec: domain.BackendSpec{Engine: domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: json.RawMessage(`{}`)}}}
			if err := k.Ensure(context.Background(), c, Instance{ID: "instance", Image: "worker:1"}); err != nil {
				t.Fatal(err)
			}
			spec := pod["spec"].(map[string]any)
			container := spec["containers"].([]any)[0].(map[string]any)
			environment := map[string]string{}
			for _, item := range container["env"].([]any) {
				e := item.(map[string]any)
				value, _ := e["value"].(string)
				environment[e["name"].(string)] = value
			}
			if environment["LINHA_SECURITY_ENABLED"] != fmt.Sprint(!disabled) {
				t.Fatal("security flag not propagated")
			}
			_, hasToken := environment["LINHA_WORKER_TOKEN_FILE"]
			if hasToken == disabled {
				t.Fatal("token file setting does not match security")
			}
			for _, field := range []struct {
				value any
				key   string
			}{{spec["volumes"], "name"}, {container["volumeMounts"], "name"}} {
				hasIdentity := false
				for _, item := range field.value.([]any) {
					if item.(map[string]any)[field.key] == "linha-identity" {
						hasIdentity = true
					}
				}
				if hasIdentity == disabled {
					t.Fatal("projected token mounted with security disabled, or absent with security enabled")
				}
			}
			var conf map[string]string
			if err := json.Unmarshal([]byte(environment["LINHA_SPARK_CONF"]), &conf); err != nil {
				t.Fatal(err)
			}
			if conf["spark.kubernetes.namespace"] != "isolated" {
				t.Fatal("executor namespace escaped")
			}
		})
	}
}
