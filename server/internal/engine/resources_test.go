package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"linha/server/internal/domain"
	"linha/server/internal/kube"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Check the actual Pod body and the executor configuration handed to the worker,
// not just helpers; the second provision represents a replacement from stored spec.
func TestSparkPodResourceBudgets(t *testing.T) {
	for _, custom := range []bool{false, true} {
		for _, templates := range []bool{false, true} {
			for _, dynamic := range []bool{false, true} {
				t.Run(fmt.Sprintf("custom=%t/templates=%t/dynamic=%t", custom, templates, dynamic), func(t *testing.T) {
					var mu sync.Mutex
					var pods []corev1.Pod
					api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != "POST" {
							t.Errorf("unexpected method %s", r.Method)
						}
						switch r.URL.Path {
						case "/api/v1/namespaces/test/pods":
							var pod corev1.Pod
							if err := json.NewDecoder(r.Body).Decode(&pod); err != nil {
								t.Error(err)
							}
							mu.Lock()
							pods = append(pods, pod)
							mu.Unlock()
						case "/api/v1/namespaces/test/services", "/api/v1/namespaces/test/configmaps":
						default:
							t.Errorf("unexpected API path %s", r.URL.Path)
						}
						w.WriteHeader(http.StatusCreated)
					}))
					defer api.Close()
					raw := map[string]any{"executors": map[string]any{"dynamicAllocation": dynamic}}
					driverCPU, executorCPU, driverHeap, executorHeap := 1, 1, 1024, 1024
					if custom {
						driverCPU, executorCPU, driverHeap, executorHeap = 2, 3, 2048, 3072
						raw["drivers"] = map[string]any{"cores": driverCPU, "memoryMi": driverHeap}
						raw["executors"] = map[string]any{"dynamicAllocation": dynamic, "cores": executorCPU, "memoryMi": executorHeap}
					}
					if templates {
						raw["kubernetes"] = map[string]any{"driverPodTemplate": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "main", "resources": map[string]any{"limits": map[string]string{"ephemeral-storage": "2Gi"}}}}}}}
					}
					adapter := &Kubernetes{Client: &kube.Client{URL: api.URL, HTTP: api.Client(), Namespace: "test"}, ServerURL: "http://linha", ServiceAccount: "worker"}
					engine, err := adapter.Normalize(domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: domain.JSON(raw)})
					if err != nil {
						t.Fatal(err)
					}
					if templates {
						engine, err = adapter.Prepare(engine)
						if err != nil {
							t.Fatal(err)
						}
					}
					persisted := domain.BackendContext{ID: "context", Spec: domain.BackendSpec{Engine: engine}}
					for _, id := range []string{"initial", "replacement"} {
						if err := adapter.Ensure(context.Background(), persisted, Instance{ID: id, Image: "worker:1"}); err != nil {
							t.Fatal(err)
						}
					}
					mu.Lock()
					defer mu.Unlock()
					if len(pods) != 2 {
						t.Fatalf("expected two provisions, got %d", len(pods))
					}
					for _, pod := range pods {
						main := pod.Spec.Containers[0]
						for label, values := range map[string]corev1.ResourceList{"requests": main.Resources.Requests, "limits": main.Resources.Limits} {
							for key, want := range map[corev1.ResourceName]string{corev1.ResourceCPU: fmt.Sprint(driverCPU), corev1.ResourceMemory: fmt.Sprintf("%dMi", driverHeap+512)} {
								got, ok := values[key]
								if !ok || got.Cmp(resource.MustParse(want)) != 0 {
									t.Errorf("%s %s.%s = %s, want %s", pod.Name, label, key, got.String(), want)
								}
							}
						}
						if templates {
							extra := main.Resources.Limits[corev1.ResourceEphemeralStorage]
							if extra.Cmp(resource.MustParse("2Gi")) != 0 {
								t.Error("template lost extra resource limit")
							}
						}
						env := map[string]string{}
						for _, value := range main.Env {
							env[value.Name] = value.Value
						}
						if env["LINHA_DRIVER_MEMORY"] != fmt.Sprintf("%dm", driverHeap) {
							t.Error("driver heap changed")
						}
						var conf map[string]string
						if err := json.Unmarshal([]byte(env["LINHA_SPARK_CONF"]), &conf); err != nil {
							t.Fatal(err)
						}
						for key, want := range map[string]string{
							"spark.executor.cores":                    fmt.Sprint(executorCPU),
							"spark.kubernetes.executor.request.cores": fmt.Sprint(executorCPU),
							"spark.kubernetes.executor.limit.cores":   fmt.Sprint(executorCPU),
							"spark.executor.memory":                   fmt.Sprintf("%dm", executorHeap),
							"spark.dynamicAllocation.enabled":         fmt.Sprint(dynamic),
						} {
							if conf[key] != want {
								t.Errorf("%s %s = %q, want %q", pod.Name, key, conf[key], want)
							}
						}
					}
				})
			}
		}
	}
}
