package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSparkRuntimeSettingsValidation(t *testing.T) {
	for _, port := range []string{"0", "-1", "1023", "65536", "4040.5", `"4040"`} {
		t.Run("port/"+port, func(t *testing.T) {
			_, err := ParseSpark(domain.EngineSpec{Version: "3.5.6", Settings: json.RawMessage(`{"ui":{"port":` + port + `}}`)})
			if err == nil {
				t.Fatal("invalid UI port accepted")
			}
		})
	}
	for _, name := range []string{"executorIdleTimeout", "cachedExecutorIdleTimeout", "shuffleTrackingTimeout"} {
		for _, value := range []string{"0s", "-1s", "1.5s", "100ms", "60", "1w", " 1m", "01s", "Infinity", "2147483648s", "24856d", "99999999999999999999999999h"} {
			t.Run(name+"/"+value, func(t *testing.T) {
				_, err := ParseSpark(domain.EngineSpec{Version: "3.5.6", Settings: domain.JSON(map[string]any{"executors": map[string]string{name: value}})})
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Fatal("expected field-specific error", err)
				}
			})
		}
	}
	for _, bad := range []string{`{"ui":{"enabled":"true"}}`, `{"ui":{"unknown":true}}`, `{"executors":{"executorIdleTimeout":"infinity"}}`, `{"executors":{"shuffleTrackingTimeout":120}}`} {
		if _, err := ParseSpark(domain.EngineSpec{Version: "3.5.6", Settings: json.RawMessage(bad)}); err == nil {
			t.Fatal("invalid settings accepted", bad)
		}
	}
}

func TestSparkRuntimeSettingsCanonicalVersions(t *testing.T) {
	k := &Kubernetes{}
	normalize := func(raw string) domain.BackendSpec {
		t.Helper()
		e, err := k.Normalize(domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: json.RawMessage(raw)})
		if err != nil {
			t.Fatal(err)
		}
		return domain.BackendSpec{Image: "image@sha256:test", Engine: e}
	}
	defaults := normalize(`{}`)
	for _, raw := range []string{`{"ui":{}}`, `{"ui":{"enabled":false,"port":4040}}`, `{"executors":{"executorIdleTimeout":"1m","cachedExecutorIdleTimeout":"infinity","shuffleTrackingTimeout":"infinity"}}`} {
		if got := normalize(raw); string(got.Engine.Settings) != string(defaults.Engine.Settings) || domain.ConfigVersion(got) != domain.ConfigVersion(defaults) {
			t.Fatal("explicit defaults changed version", raw)
		}
	}
	for _, key := range []string{`"ui"`, `"executorIdleTimeout"`, `"cachedExecutorIdleTimeout"`, `"shuffleTrackingTimeout"`} {
		if strings.Contains(string(defaults.Engine.Settings), key) {
			t.Fatal("new defaults changed old normalized settings", key)
		}
	}
	a := normalize(`{"ui":{"enabled":true},"executors":{"dynamicAllocation":true,"executorIdleTimeout":"2m","cachedExecutorIdleTimeout":"1h","shuffleTrackingTimeout":"1d"}}`)
	b := normalize(`{"ui":{"enabled":true,"port":4040},"executors":{"dynamicAllocation":true,"executorIdleTimeout":"120s","cachedExecutorIdleTimeout":"3600s","shuffleTrackingTimeout":"86400s"}}`)
	if domain.ConfigVersion(a) != domain.ConfigVersion(b) {
		t.Fatal("equivalent durations produced different versions")
	}
	if got := normalize(string(a.Engine.Settings)); domain.ConfigVersion(got) != domain.ConfigVersion(a) {
		t.Fatal("normalization is not idempotent")
	}
	for _, raw := range []string{`{"ui":{"enabled":true}}`, `{"ui":{"port":4050}}`, `{"executors":{"executorIdleTimeout":"2m"}}`, `{"executors":{"cachedExecutorIdleTimeout":"2m"}}`, `{"executors":{"shuffleTrackingTimeout":"2m"}}`} {
		if domain.ConfigVersion(normalize(raw)) == domain.ConfigVersion(defaults) {
			t.Fatal("changed setting did not change version", raw)
		}
	}
	for _, port := range []int{1024, 65535} {
		normalize(fmt.Sprintf(`{"ui":{"port":%d},"executors":{"executorIdleTimeout":"2147483647s"}}`, port))
	}
}

func TestSparkRuntimeSettingsReachBothLaunchPaths(t *testing.T) {
	cases := []struct {
		name, raw string
		want      map[string]string
	}{
		{"defaults", `{}`, map[string]string{"spark.ui.enabled": "false", "spark.dynamicAllocation.enabled": "false"}},
		{"ui-default-port", `{"ui":{"enabled":true}}`, map[string]string{"spark.ui.enabled": "true", "spark.ui.port": "4040"}},
		{"disabled-custom-port", `{"ui":{"enabled":false,"port":4050}}`, map[string]string{"spark.ui.enabled": "false", "spark.ui.port": "4050"}},
		{"dynamic", `{"ui":{"enabled":true,"port":4050},"executors":{"dynamicAllocation":true,"minExecutors":0,"initialExecutors":1,"maxExecutors":6,"executorIdleTimeout":"90s","cachedExecutorIdleTimeout":"2m","shuffleTrackingTimeout":"3m"}}`, map[string]string{
			"spark.ui.enabled": "true", "spark.ui.port": "4050", "spark.dynamicAllocation.enabled": "true", "spark.dynamicAllocation.shuffleTracking.enabled": "true",
			"spark.dynamicAllocation.executorIdleTimeout": "90s", "spark.dynamicAllocation.cachedExecutorIdleTimeout": "120s", "spark.dynamicAllocation.shuffleTracking.timeout": "180s",
		}},
		{"static-timeouts-inactive", `{"executors":{"instances":3,"executorIdleTimeout":"90s","cachedExecutorIdleTimeout":"2m","shuffleTrackingTimeout":"3m"}}`, map[string]string{"spark.dynamicAllocation.enabled": "false"}},
		{"unlimited", `{"executors":{"dynamicAllocation":true,"cachedExecutorIdleTimeout":"infinity","shuffleTrackingTimeout":"infinity"}}`, map[string]string{"spark.dynamicAllocation.enabled": "true"}},
	}
	for _, tc := range cases {
		for _, application := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/application=%t", tc.name, application), func(t *testing.T) {
				var raw map[string]any
				if err := json.Unmarshal([]byte(tc.raw), &raw); err != nil {
					t.Fatal(err)
				}
				if application {
					raw["application"] = map[string]string{"mainClass": "example.Main", "mainApplicationFile": "local:///opt/job.jar"}
				}
				var conf map[string]string
				api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var resource map[string]any
					if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
						t.Error(err)
					}
					spec, _ := resource["spec"].(map[string]any)
					switch resource["kind"] {
					case "SparkApplication":
						encoded, _ := json.Marshal(spec["sparkConf"])
						if err := json.Unmarshal(encoded, &conf); err != nil {
							t.Error(err)
						}
						if tc.name == "dynamic" {
							d := spec["dynamicAllocation"].(map[string]any)
							if d["minExecutors"] != float64(0) || d["initialExecutors"] != float64(1) || d["maxExecutors"] != float64(6) {
								t.Error(d)
							}
						}
						if tc.name == "static-timeouts-inactive" && spec["executor"].(map[string]any)["instances"] != float64(3) {
							t.Error(spec)
						}
					case "Pod":
						for _, item := range spec["containers"].([]any)[0].(map[string]any)["env"].([]any) {
							env := item.(map[string]any)
							if env["name"] == "LINHA_SPARK_CONF" {
								if err := json.Unmarshal([]byte(env["value"].(string)), &conf); err != nil {
									t.Error(err)
								}
							}
						}
					}
					w.WriteHeader(http.StatusCreated)
				}))
				defer api.Close()
				k := &Kubernetes{Client: &kube.Client{Namespace: "jobs", URL: api.URL, HTTP: api.Client()}, ServerURL: "http://linha", ServiceAccount: "worker"}
				e, err := k.Prepare(domain.EngineSpec{Type: "spark", Version: "3.5.6", Settings: domain.JSON(raw)})
				if err != nil {
					t.Fatal(err)
				}
				if err = k.Ensure(context.Background(), domain.BackendContext{ID: "context", Spec: domain.BackendSpec{Engine: e}}, Instance{ID: "instance", Image: "image@sha256:test"}); err != nil {
					t.Fatal(err)
				}
				if conf == nil {
					t.Fatal("no driver configuration captured")
				}
				for key, want := range tc.want {
					if conf[key] != want {
						t.Errorf("%s = %q, want %q", key, conf[key], want)
					}
				}
				if tc.name != "dynamic" {
					for _, key := range []string{"spark.dynamicAllocation.executorIdleTimeout", "spark.dynamicAllocation.cachedExecutorIdleTimeout", "spark.dynamicAllocation.shuffleTracking.timeout"} {
						if _, ok := conf[key]; ok {
							t.Error("unexpected timeout override", key, conf[key])
						}
					}
				}
				if tc.name == "dynamic" && !application && (conf["spark.dynamicAllocation.minExecutors"] != "0" || conf["spark.dynamicAllocation.maxExecutors"] != "6") {
					t.Fatal(conf)
				}
			})
		}
	}
}
