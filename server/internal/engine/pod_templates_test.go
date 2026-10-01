package engine

import (
	"context"
	"encoding/json"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func template(t *testing.T, s string) PodTemplate {
	t.Helper()
	var p PodTemplate
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestSharedPodMergeFixtures(t *testing.T) {
	data, err := os.ReadFile("../../../api/fixtures/pod-templates.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name                     string
		Base, Override, Expected PodTemplate
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			before := string(domain.JSON(f.Base))
			got := MergePodTemplates(f.Base, f.Override)
			if !reflect.DeepEqual(got, f.Expected) {
				t.Fatalf("got %s; expected %s", domain.JSON(got), domain.JSON(f.Expected))
			}
			if before != string(domain.JSON(f.Base)) {
				t.Fatal("base mutated")
			}
		})
	}
}
func TestPodTemplateManagedFields(t *testing.T) {
	valid := template(t, `{"metadata":{"labels":{"env":"prod","containers":"main"}},"spec":{"nodeSelector":{"volumes":"ssd"},"containers":[{"name":"main","env":[{"name":"APP","value":"prod"}]}]}}`)
	if err := ValidatePodTemplate(valid, "driver", true); err != nil {
		t.Fatal("metadata keys treated as Pod fields", err)
	}

	tests := []struct{ body, field string }{
		{`{"metadata":{"namespace":"other"}}`, "namespace"},
		{`{"metadata":{"labels":{"linha.io/context":"other"}}}`, "linha.io/context"},
		{`{"metadata":{"ownerReferences":[]}}`, "ownerReferences"},
		{`{"spec":{"serviceAccountName":"admin"}}`, "serviceAccountName"},
		{`{"spec":{"restartPolicy":"Always"}}`, "restartPolicy"},
		{`{"spec":{"automountServiceAccountToken":false}}`, "automountServiceAccountToken"},
		{`{"spec":{"containers":[{"name":"main","command":["sh"]}]}}`, "command"},
		{`{"spec":{"containers":[{"name":"main","image":"evil"}]}}`, "image"},
		{`{"spec":{"containers":[{"name":"main","resources":{"limits":{"memory":"32Gi"}}}]}}`, "resources.memory"},
		{`{"spec":{"containers":[{"name":"main","env":[{"name":"LINHA_CONTEXT_ID","value":"bad"}]}]}}`, "LINHA_CONTEXT_ID"},
		{`{"spec":{"containers":[{"name":"main","env":[{"name":"SPARK_EXECUTOR_ID","value":"bad"}]}]}}`, "SPARK_EXECUTOR_ID"},
		{`{"spec":{"containers":[{"name":"main","volumeMounts":[{"name":"a","mountPath":"/opt"}]}]}}`, "volumeMounts"},
		{`{"spec":{"containers":[{"name":"main","volumeMounts":[{"name":"a","mountPath":"/etc/app"}]}]}}`, "not defined"},
		{`{"spec":{"volumes":[{"name":"linha-identity","emptyDir":{}}]}}`, "reserved"},
		{`{"spec":{"containers":[{"name":"main"},{"name":"main"}]}}`, "unique"},
		{`{"spec":{"containers":[{"name":"main","env":[{"name":"A","value":"a"},{"name":"A","value":"b"}]}]}}`, "unique"},
		{`{"spec":{"imagePullSecrets":[{"name":"../other"}]}}`, "Secret name"},
		{`{"spec":{"containers":[{"name":"main","volumeMountz":[]}]}}`, "volumeMountz"},
		{`{"spec":null}`, "null"},
	}
	for _, tc := range tests {
		if err := ValidatePodTemplate(template(t, tc.body), "driver", true); err == nil || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("%s: %v; expected %s", tc.body, err, tc.field)
		}
	}
	big := PodTemplate{"metadata": map[string]any{"annotations": map[string]any{"large": strings.Repeat("x", maxPodTemplateBytes)}}}
	if err := ValidatePodTemplate(big, "driver", true); err == nil {
		t.Fatal("oversized template accepted")
	}
	k := &Kubernetes{AllowedImages: []string{"registry.example/apps"}, RegistrySecrets: []string{"pull"}}
	for _, raw := range []string{`{"spec":{"containers":[{"name":"sidecar","image":"evil.example/a:1"}]}}`, `{"spec":{"imagePullSecrets":[{"name":"other"}]}}`} {
		if err := k.validateTemplates(&PodTemplates{Driver: template(t, raw)}, true); err == nil {
			t.Fatal("policy bypass accepted")
		}
	}
}
func TestPodTemplatesPersistedRenderingAndReplacement(t *testing.T) {
	var mu sync.Mutex
	objects := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/test/") {
			t.Errorf("escaped namespace: %s", r.URL.Path)
			http.Error(w, "forbidden", 403)
			return
		}
		if r.Method == "GET" {
			if old, ok := objects[r.URL.Path]; ok {
				json.NewEncoder(w).Encode(old)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method %s", r.Method)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		key := r.URL.Path + "/" + body["metadata"].(map[string]any)["name"].(string)
		if _, exists := objects[key]; exists {
			w.WriteHeader(409)
			return
		}
		objects[key] = body
		w.WriteHeader(201)
	}))
	defer server.Close()
	k := &Kubernetes{Client: &kube.Client{URL: server.URL, HTTP: server.Client(), Namespace: "test"}, ServiceAccount: "worker", ServerURL: "http://linha", ImagePullSecrets: []string{"pull"}, AllowedImages: []string{"registry.example/apps"}, TemplateDefaults: PodTemplates{Driver: template(t, `{"spec":{"containers":[{"name":"main","env":[{"name":"DEFAULT","value":"original"}]}]}}`)}}
	user := template(t, `{"metadata":{"labels":{"app":"metoc"}},"spec":{"volumes":[{"name":"config","configMap":{"name":"app-v1"}}],"containers":[{"name":"main","env":[{"name":"APP_CONFIG","value":"/etc/app/config"}],"volumeMounts":[{"name":"config","mountPath":"/etc/app","readOnly":true}]},{"name":"sidecar","image":"registry.example/apps/sidecar@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"initContainers":[{"name":"init","image":"registry.example/apps/init:1","command":["true"]}],"nodeSelector":{"pool":"compute"}}}`)
	requested := domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: domain.JSON(SparkSettings{})}
	// Use parser defaults instead of zero resource bounds.
	requested.Settings = domain.JSON(map[string]any{"kubernetes": PodTemplates{Driver: user, Executor: user}})
	normalized, err := k.Normalize(requested)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := k.Prepare(normalized)
	if err != nil {
		t.Fatal(err)
	}
	c := domain.BackendContext{ID: "context", Spec: domain.BackendSpec{Image: "registry.example/apps/worker:1", Engine: effective}}
	instance := Instance{ID: "first", ContextID: c.ID, Image: "registry.example/apps/worker@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err = k.Ensure(context.Background(), c, instance); err != nil {
		t.Fatal(err)
	}
	if err = k.Ensure(context.Background(), c, instance); err != nil {
		t.Fatal("idempotent ensure", err)
	}
	// A restarted adapter has different defaults; only persisted data may influence replacement.
	k.TemplateDefaults = PodTemplates{Driver: template(t, `{"spec":{"containers":[{"name":"main","env":[{"name":"DEFAULT","value":"changed"}]}]}}`)}
	k.Volumes = []map[string]any{{"name": "new-default", "emptyDir": map[string]any{}}}
	instance.ID = "replacement"
	if err = k.Ensure(context.Background(), c, instance); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	driver := objects["/api/v1/namespaces/test/pods/linha-replacement"]
	cm := objects["/api/v1/namespaces/test/configmaps/linha-context-executor"]
	text := string(domain.JSON(driver))
	if !strings.Contains(text, `"value":"original"`) || strings.Contains(text, "changed") || strings.Contains(text, "new-default") {
		t.Fatal("replacement used mutable defaults", text)
	}
	podSpec := driver["spec"].(map[string]any)
	containers := podSpec["containers"].([]any)
	if len(containers) != 2 || containers[0].(map[string]any)["name"] != "main" {
		t.Fatal("main container selection changed")
	}
	var conf map[string]string
	for _, raw := range containers[0].(map[string]any)["env"].([]any) {
		env := raw.(map[string]any)
		if env["name"] == "LINHA_SPARK_CONF" {
			json.Unmarshal([]byte(env["value"].(string)), &conf)
		}
	}
	if conf["spark.kubernetes.executor.podTemplateFile"] != executorTemplateMount+"/executor.json" || conf["spark.kubernetes.executor.podTemplateContainerName"] != "main" {
		t.Fatal("Spark template settings absent")
	}
	executor := cm["data"].(map[string]any)["executor.json"].(string)
	for _, part := range []string{"app-v1", "APP_CONFIG", "pull", "sidecar", "init", "nodeSelector"} {
		if !strings.Contains(executor, part) {
			t.Fatal("executor template dropped", part)
		}
	}
	cm["data"].(map[string]any)["executor.json"] = "tampered"
	mu.Unlock()
	if err = k.Ensure(context.Background(), c, instance); err == nil {
		t.Fatal("changed template resource accepted")
	}
}
