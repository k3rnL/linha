package engine

import (
	"context"
	"encoding/json"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSparkApplicationResourcesAndProtectedTemplates(t *testing.T) {
	var application map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/apis/sparkoperator.k8s.io/v1beta2/namespaces/jobs/sparkapplications" {
			t.Error(r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&application); err != nil {
			t.Error(err)
		}
		w.WriteHeader(201)
	}))
	defer api.Close()
	k := &Kubernetes{RequireApplication: true, Client: &kube.Client{Namespace: "jobs", URL: api.URL, HTTP: api.Client()}, ServerURL: "http://linha", ServiceAccount: "worker"}
	spec := domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: json.RawMessage(`{"application":{"mainClass":"example.Main","mainApplicationFile":"local:///opt/job.jar"},"drivers":{"memory":"10g","memoryOverhead":"2g","coreRequest":"500m","coreLimit":"2"},"executors":{"memory":"2g"},"kubernetes":{"driverPodTemplate":{"spec":{"containers":[{"name":"main","env":[{"name":"APP_ENV","value":"test"}]}]}}}}`)}
	prepared, err := k.Prepare(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err = k.Ensure(context.Background(), domain.BackendContext{ID: "context", Spec: domain.BackendSpec{Engine: prepared}}, Instance{ID: "instance", Image: "image@sha256:test"}); err != nil {
		t.Fatal(err)
	}
	s := application["spec"].(map[string]any)
	driver := s["driver"].(map[string]any)
	if s["mode"] != "cluster" || driver["memory"] != "10240m" || driver["memoryOverhead"] != "2048m" || driver["coreRequest"] != "0.5" || driver["coreLimit"] != "2" {
		t.Fatal(s)
	}
	template := driver["template"].(map[string]any)
	container := template["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)
	if container["name"] != "spark-kubernetes-driver" || container["command"] != nil || container["resources"] != nil {
		t.Fatal(container)
	}
	if err = k.Validate(domain.EngineSpec{Version: "3.5.6", Settings: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("missing application accepted")
	}
	for _, settings := range []string{`{"drivers":{"memoryMi":1024,"memory":"1g"}}`, `{"drivers":{"coreRequest":"2","coreLimit":"1"}}`, `{"drivers":{"javaOptions":"-Xmx10g"}}`, `{"drivers":{"memory":"10Gi"}}`} {
		if _, err := ParseSpark(domain.EngineSpec{Version: "3.5.6", Settings: json.RawMessage(settings)}); err == nil {
			t.Fatal("invalid resource settings", settings)
		}
	}
}

func TestSparkApplicationPendingLossAndDeletionFencing(t *testing.T) {
	driverName := ""
	podMissing := false
	deletes := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes++
			var options map[string]any
			json.NewDecoder(r.Body).Decode(&options)
			if options["preconditions"].(map[string]any)["uid"] != "app-uid" || options["propagationPolicy"] != "Foreground" {
				t.Error(options)
			}
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/apis/sparkoperator.k8s.io/v1beta2/namespaces/jobs/sparkapplications/linha-instance" {
			json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"uid": "app-uid", "labels": map[string]string{"linha.io/context": "context", "linha.io/instance": "instance"}}, "status": map[string]any{"driverInfo": map[string]string{"podName": driverName}}})
			return
		}
		if podMissing {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"uid": "pod-uid", "labels": map[string]string{"linha.io/context": "context", "linha.io/instance": "instance"}, "ownerReferences": []map[string]string{{"uid": "app-uid"}}}, "status": map[string]string{"phase": "Running"}})
	}))
	defer api.Close()
	k := &Kubernetes{Client: &kube.Client{Namespace: "jobs", URL: api.URL, HTTP: api.Client()}}
	i := Instance{ID: "instance", ContextID: "context", Kind: "sparkapplication"}
	pending, err := k.Observe(context.Background(), i)
	if err != nil || pending.ResourceUID != "app-uid" || pending.Incarnation != "" || pending.Draining {
		t.Fatal(pending, err)
	}
	driverName = "spark-driver"
	ready, err := k.Observe(context.Background(), pending)
	if err != nil || !ready.Ready || ready.Incarnation != "pod-uid" {
		t.Fatal(ready, err)
	}
	stale := ready
	stale.ResourceUID = "other-app"
	if err = k.Stop(context.Background(), stale); err == nil || deletes != 0 {
		t.Fatal("unrelated application deleted", err)
	}
	stale = ready
	stale.Incarnation = "other-pod"
	if _, err = k.Observe(context.Background(), stale); err == nil {
		t.Fatal("pod UID changed without fencing")
	}
	podMissing = true
	lost, err := k.Observe(context.Background(), ready)
	if err != nil || !lost.Draining {
		t.Fatal(lost, err)
	}
	if err = k.Stop(context.Background(), lost); err != nil || deletes != 1 {
		t.Fatal(deletes, err)
	}
}
