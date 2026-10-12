package engine

import (
	"context"
	"encoding/json"
	"errors"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func inventoryItem(id, contextID, namespace, uid string) map[string]any {
	return map[string]any{"metadata": map[string]any{
		"name": "linha-" + id, "namespace": namespace, "uid": uid,
		"labels": map[string]string{"app.kubernetes.io/managed-by": "linha", "linha.io/instance": id, "linha.io/context": contextID},
	}}
}

func TestManagedResourceInventoryPaginatesAndFilters(t *testing.T) {
	id, contextID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if r.Method != http.MethodGet || r.URL.Query().Get("labelSelector") != managedResourceSelector || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unfiltered inventory: %s %s", r.Method, r.URL)
		}
		var items []any
		continuation := ""
		switch r.URL.Path {
		case "/api/v1/namespaces/test/pods":
			if r.URL.Query().Get("continue") == "" {
				child := inventoryItem(id, contextID, "test", "child")
				child["metadata"].(map[string]any)["name"] = "linha-" + id + "-driver"
				foreign := inventoryItem(id, contextID, "test", "foreign")
				foreign["metadata"].(map[string]any)["labels"].(map[string]string)["app.kubernetes.io/managed-by"] = "someone-else"
				items = []any{child, foreign, inventoryItem(id, contextID, "elsewhere", "wrong-namespace"), inventoryItem(id, contextID, "test", ""), inventoryItem(id, "", "test", "no-context"), inventoryItem("../bad", contextID, "test", "bad-id")}
				continuation = "next/part?+=token"
			} else if r.URL.Query().Get("continue") == "next/part?+=token" {
				items = []any{inventoryItem(id, contextID, "test", "pod-uid")}
			} else {
				t.Errorf("continuation not preserved: %s", r.URL)
			}
		case "/api/v1/namespaces/test/services":
			// The Service can exist without its Pod.
			items = []any{inventoryItem(id, contextID, "test", "service-uid")}
		case "/apis/sparkoperator.k8s.io/v1beta2/namespaces/test/sparkapplications":
			items = []any{inventoryItem(id, contextID, "test", "app-uid")}
		default:
			t.Errorf("unexpected resource request: %s", r.URL)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]string{"continue": continuation}, "items": items})
	}))
	defer server.Close()
	k := &Kubernetes{Client: &kube.Client{URL: server.URL, Namespace: "test", HTTP: server.Client()}}
	got, err := k.ListResources(context.Background())
	want := []ManagedResource{
		{InstanceID: id, ContextID: contextID, InstanceKind: "pod", Kind: "pods", UID: "pod-uid"},
		{InstanceID: id, ContextID: contextID, InstanceKind: "pod", Kind: "services", UID: "service-uid"},
		{InstanceID: id, ContextID: contextID, InstanceKind: "sparkapplication", Kind: "sparkapplications", UID: "app-uid"},
	}
	if err != nil || !reflect.DeepEqual(got, want) || len(calls) != 3 || calls["/api/v1/namespaces/test/pods"] != 2 {
		t.Fatalf("inventory = %+v, %v; calls = %+v", got, err, calls)
	}
}

func TestManagedResourceInventoryErrors(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		status    int
		repeated  bool
		wantError bool
	}{
		{"operator not installed", http.StatusNotFound, false, false},
		{"access denied", http.StatusForbidden, false, true},
		{"API unavailable", http.StatusServiceUnavailable, false, true},
		{"repeated continuation", http.StatusOK, true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.HasSuffix(r.URL.Path, "/sparkapplications") {
					w.WriteHeader(scenario.status)
					if scenario.repeated {
						_, _ = w.Write([]byte(`{"metadata":{"continue":"same"},"items":[]}`))
					}
					return
				}
				_, _ = w.Write([]byte(`{"items":[]}`))
			}))
			defer server.Close()
			k := &Kubernetes{Client: &kube.Client{URL: server.URL, Namespace: "test", HTTP: server.Client()}}
			_, err := k.ListResources(context.Background())
			if (err != nil) != scenario.wantError || calls > 4 {
				t.Fatalf("error = %v, calls = %d", err, calls)
			}
		})
	}
}

func TestDeleteInventoriedResourceUsesUIDWithoutProbing(t *testing.T) {
	for _, kind := range []string{"pods", "services", "sparkapplications"} {
		t.Run(kind, func(t *testing.T) {
			id, contextID := strings.Repeat("a", 32), strings.Repeat("b", 32)
			calls, status := 0, http.StatusOK
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodDelete || !strings.HasSuffix(r.URL.Path, "/"+kind+"/linha-"+id) {
					t.Errorf("unexpected probe: %s %s", r.Method, r.URL)
				}
				var body struct {
					Preconditions     struct{ UID string }
					PropagationPolicy string
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Preconditions.UID != "observed-uid" {
					t.Errorf("missing UID precondition: %+v, %v", body, err)
				}
				if kind == "sparkapplications" && body.PropagationPolicy != "Foreground" {
					t.Errorf("lost application cascade: %+v", body)
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			k := &Kubernetes{Client: &kube.Client{URL: server.URL, Namespace: "test", HTTP: server.Client()}}
			r := ManagedResource{InstanceID: id, ContextID: contextID, InstanceKind: "pod", Kind: kind, UID: "observed-uid"}
			if kind == "sparkapplications" {
				r.InstanceKind = "sparkapplication"
			}
			for _, code := range []int{http.StatusOK, http.StatusNotFound, http.StatusConflict} {
				status = code
				err := k.DeleteResource(context.Background(), r)
				var api *kube.APIError
				if code == http.StatusConflict {
					if !errors.As(err, &api) || api.Status != code {
						t.Fatalf("UID conflict swallowed: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
			r.UID = ""
			if err := k.DeleteResource(context.Background(), r); err == nil || calls != 3 {
				t.Fatalf("unsafe deletion or extra probes: %v, calls=%d", err, calls)
			}
		})
	}
}
