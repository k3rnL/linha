package engine

import (
	"context"
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSparkObservationUsesOnlyCurrentOwnedExecutors(t *testing.T) {
	spec := corev1.PodSpec{Containers: []corev1.Container{{Name: "spark-kubernetes-executor", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("1Gi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}}}}}
	pod := func(uid string) corev1.Pod {
		return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"linha.io/context": "context", "linha.io/instance": "instance"}, OwnerReferences: []metav1.OwnerReference{{UID: types.UID(uid)}}}, Spec: spec, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	}
	owned := pod("driver")
	deleted := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := []corev1.Pod{pod("previous-driver")}
		if !deleted {
			p = append(p, owned)
		}
		json.NewEncoder(w).Encode(corev1.PodList{Items: p})
	}))
	defer api.Close()
	k := &Kubernetes{Client: &kube.Client{Namespace: "jobs", URL: api.URL, HTTP: api.Client()}}
	observe := func() *domain.EngineObservation {
		o := &domain.EngineObservation{Resources: map[string]float64{"driver_cpu_request": 1, "driver_cpu_limit": 1, "driver_memory_request": 1024, "driver_memory_limit": 1024}}
		if e := k.completeSparkObservation(context.Background(), Instance{ID: "instance", ContextID: "context", ResourceUID: "app", Incarnation: "driver", Observation: o}, nil); e != nil {
			t.Fatal(e)
		}
		return o
	}
	o := observe()
	if o.ExecutorStates["Running"] != 1 || o.Resources["executor_cpu_request"] != .1 || !o.Available {
		t.Fatal(o)
	}
	owned.Spec.Containers[0].Resources.Limits = nil
	o = observe()
	if o.Available || o.Condition != "missing_resources" {
		t.Fatal("partial allocations presented as complete", o)
	}
	deleted = true
	o = observe()
	if o.ExecutorStates["Running"] != 0 || o.Resources["executor_memory_limit"] != 0 || !o.Available {
		t.Fatal("deleted executor retained", o)
	}
}
