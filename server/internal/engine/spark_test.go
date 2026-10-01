package engine

import (
	"encoding/json"
	"linha/server/internal/domain"
	"testing"
)

func TestSparkBoundsAndIndependentExecutorAllocation(t *testing.T) {
	spec := domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: json.RawMessage(`{"drivers":{"minDrivers":0,"maxDrivers":3,"maxConcurrentRequestsPerDriver":2},"executors":{"dynamicAllocation":true,"minExecutors":0,"initialExecutors":1,"maxExecutors":5}}`)}
	settings, e := ParseSpark(spec)
	if e != nil {
		t.Fatal(e)
	}
	conf := settings.Conf("ns", "image@sha256:abc", "host", "pod", "worker")
	if conf["spark.dynamicAllocation.maxExecutors"] != "5" || conf["spark.dynamicAllocation.shuffleTracking.enabled"] != "true" || conf["spark.scheduler.mode"] != "FAIR" || conf["spark.kubernetes.driver.pod.name"] != "pod" {
		t.Fatalf("configuration: %+v", conf)
	}
	settings.Executors.Initial = 0
	zero := settings.Conf("ns", "image", "host", "pod", "worker")
	if _, present := zero["spark.executor.instances"]; present || zero["spark.dynamicAllocation.initialExecutors"] != "0" {
		t.Fatal("static instances overrode dynamic initial capacity")
	}
	for _, bad := range []string{`{"drivers":{"minDrivers":3,"maxDrivers":2}}`, `{"drivers":{"maxConcurrentRequestsPerDriver":0}}`, `{"executors":{"dynamicAllocation":true,"minExecutors":2,"initialExecutors":1}}`, `{"executors":{"memoryMi":256}}`, `{"arbitrarySparkConfig":{}}`} {
		spec.Settings = json.RawMessage(bad)
		if _, e = ParseSpark(spec); e == nil {
			t.Fatalf("invalid settings accepted %s", bad)
		}
	}
}
