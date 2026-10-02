package engine

import (
	"context"
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
)

func (k *Kubernetes) ensureApplication(ctx context.Context, c domain.BackendContext, i Instance, s SparkSettings) error {
	labels := map[string]any{"app.kubernetes.io/managed-by": "linha", "linha.io/context": c.ID, "linha.io/instance": i.ID}
	templates := s.Kubernetes
	if templates == nil {
		templates = &PodTemplates{}
	}
	makeTemplate := func(input PodTemplate, driver bool) PodTemplate {
		container := map[string]any{"name": "main", "securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []string{"ALL"}}}}
		podSpec := map[string]any{"restartPolicy": "Never", "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 185, "fsGroup": 10001}, "containers": []any{container}}
		if driver {
			env := []any{
				map[string]any{"name": "LINHA_SERVER_URL", "value": k.ServerURL},
				map[string]any{"name": "LINHA_CONTEXT_ID", "value": c.ID},
				map[string]any{"name": "LINHA_CAPACITY", "value": fmt.Sprint(s.Drivers.Concurrency)},
				// Spark submit supplies deployment configuration; never replace it in user main().
				map[string]any{"name": "LINHA_SPARK_CONF", "value": "{}"},
				map[string]any{"name": "LINHA_POD_UID", "valueFrom": map[string]any{"fieldRef": map[string]any{"fieldPath": "metadata.uid"}}},
				map[string]any{"name": "LINHA_SECURITY_ENABLED", "value": fmt.Sprint(!k.SecurityDisabled)},
			}
			podSpec["serviceAccountName"] = k.ServiceAccount
			if !k.SecurityDisabled {
				env = append(env, map[string]any{"name": "LINHA_WORKER_TOKEN_FILE", "value": "/var/run/linha/token"})
				podSpec["volumes"] = []any{map[string]any{"name": "linha-identity", "projected": map[string]any{"sources": []any{map[string]any{"serviceAccountToken": map[string]any{"audience": "linha-worker", "expirationSeconds": 3600, "path": "token"}}}}}}
				container["volumeMounts"] = []any{map[string]any{"name": "linha-identity", "mountPath": "/var/run/linha", "readOnly": true}}
			}
			container["env"] = env
		}
		t := MergePodTemplates(PodTemplate{"metadata": map[string]any{"labels": labels}, "spec": podSpec}, input)
		for _, raw := range t["spec"].(map[string]any)["containers"].([]any) {
			item := raw.(map[string]any)
			if item["name"] == "main" {
				if driver {
					item["name"] = "spark-kubernetes-driver"
				} else {
					item["name"] = "spark-kubernetes-executor"
				}
			}
		}
		return t
	}
	driver := map[string]any{"cores": s.Drivers.Cores, "coreRequest": s.Drivers.CoreRequest, "coreLimit": s.Drivers.CoreLimit, "memory": s.Drivers.Memory, "memoryOverhead": s.Drivers.MemoryOverhead, "javaOptions": s.Drivers.JavaOptions, "serviceAccount": k.ServiceAccount, "labels": labels, "template": makeTemplate(templates.Driver, true)}
	executor := map[string]any{"cores": s.Executors.Cores, "coreRequest": s.Executors.CoreRequest, "coreLimit": s.Executors.CoreLimit, "memory": s.Executors.Memory, "memoryOverhead": s.Executors.MemoryOverhead, "javaOptions": s.Executors.JavaOptions, "labels": labels, "template": makeTemplate(templates.Executor, false)}
	conf := s.runtimeConf()
	spec := map[string]any{"type": "Scala", "mode": "cluster", "sparkVersion": c.Spec.Engine.Version, "image": i.Image, "mainClass": s.Application.MainClass, "mainApplicationFile": s.Application.MainApplicationFile, "arguments": s.Application.Arguments, "restartPolicy": map[string]any{"type": "Never"}, "driver": driver, "executor": executor, "sparkConf": conf}
	if s.Executors.Dynamic {
		spec["dynamicAllocation"] = map[string]any{"enabled": true, "minExecutors": s.Executors.Min, "initialExecutors": s.Executors.Initial, "maxExecutors": s.Executors.Max}
	} else {
		executor["instances"] = s.Executors.Instances
	}
	return k.create(ctx, "sparkapplications", map[string]any{"apiVersion": "sparkoperator.k8s.io/v1beta2", "kind": "SparkApplication", "metadata": map[string]any{"name": "linha-" + i.ID, "namespace": k.Client.Namespace, "labels": labels}, "spec": spec})
}

type applicationStatus struct {
	Metadata struct {
		UID               string            `json:"uid"`
		Labels            map[string]string `json:"labels"`
		DeletionTimestamp *string           `json:"deletionTimestamp"`
	} `json:"metadata"`
	Status struct {
		DriverInfo struct {
			PodName             string `json:"podName"`
			WebUIIngressAddress string `json:"webUIIngressAddress"`
			WebUIServiceName    string `json:"webUIServiceName"`
			WebUIPort           int    `json:"webUIPort"`
		} `json:"driverInfo"`
		ApplicationState struct {
			State        string `json:"state"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"applicationState"`
	} `json:"status"`
}

func (k *Kubernetes) application(ctx context.Context, i Instance) (applicationStatus, error) {
	var app applicationStatus
	err := k.Client.Do(ctx, "GET", k.Client.Path("sparkapplications", "linha-"+i.ID), nil, &app)
	var api *kube.APIError
	if errors.As(err, &api) && api.Status == 404 {
		return app, domain.NotFound
	}
	if err != nil {
		return app, err
	}
	if app.Metadata.Labels["linha.io/context"] != i.ContextID || app.Metadata.Labels["linha.io/instance"] != i.ID || i.ResourceUID != "" && i.ResourceUID != app.Metadata.UID {
		return app, domain.Conflict("SparkApplication ownership or UID changed")
	}
	return app, nil
}
func (k *Kubernetes) observeApplication(ctx context.Context, i Instance) (Instance, error) {
	app, err := k.application(ctx, i)
	if err != nil {
		return i, err
	}
	i.ResourceUID = app.Metadata.UID
	i.Observation = &domain.EngineObservation{Application: "linha-" + i.ID, ApplicationState: app.Status.ApplicationState.State, DriverPod: app.Status.DriverInfo.PodName, UIAddress: app.Status.DriverInfo.WebUIIngressAddress, UIService: app.Status.DriverInfo.WebUIServiceName, UIPort: app.Status.DriverInfo.WebUIPort, Condition: app.Status.ApplicationState.ErrorMessage}
	i.Condition = app.Status.ApplicationState.ErrorMessage
	state := app.Status.ApplicationState.State
	i.Draining = app.Metadata.DeletionTimestamp != nil || state == "FAILED" || state == "COMPLETED" || state == "FAILED_SUBMISSION" || state == "SUBMISSION_FAILED" || state == "UNKNOWN"
	if app.Status.DriverInfo.PodName == "" {
		return i, nil
	}
	var pod struct {
		Spec     corev1.PodSpec `json:"spec"`
		Metadata struct {
			UID             string            `json:"uid"`
			Labels          map[string]string `json:"labels"`
			OwnerReferences []struct {
				UID string `json:"uid"`
			} `json:"ownerReferences"`
			DeletionTimestamp *string `json:"deletionTimestamp"`
		} `json:"metadata"`
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	err = k.Client.Do(ctx, "GET", k.Client.Path("pods", app.Status.DriverInfo.PodName), nil, &pod)
	var api *kube.APIError
	if errors.As(err, &api) && api.Status == 404 {
		if i.Incarnation != "" {
			i.Draining = true
		}
		return i, nil
	}
	if err != nil {
		return i, err
	}
	owned := false
	for _, o := range pod.Metadata.OwnerReferences {
		if o.UID == i.ResourceUID {
			owned = true
		}
	}
	if !owned || pod.Metadata.Labels["linha.io/context"] != i.ContextID || pod.Metadata.Labels["linha.io/instance"] != i.ID || i.Incarnation != "" && i.Incarnation != pod.Metadata.UID {
		return i, domain.Conflict("Spark driver ownership or incarnation changed")
	}
	i.Incarnation = pod.Metadata.UID
	i.Ready = pod.Status.Phase == "Running"
	i.Observation.DriverPhase = pod.Status.Phase
	if pod.Metadata.DeletionTimestamp != nil {
		i.Observation.DriverPhase = "Terminating"
	}
	i.Observation.Resources = map[string]float64{}
	addPodResources(i.Observation.Resources, "driver", pod.Spec)
	if err := k.completeSparkObservation(ctx, i, pod.Metadata.Labels); err != nil {
		i.Observation.Condition = "runtime observation unavailable: " + err.Error()
	}
	i.Draining = i.Draining || pod.Metadata.DeletionTimestamp != nil || pod.Status.Phase == "Failed" || pod.Status.Phase == "Succeeded"
	return i, nil
}
func (k *Kubernetes) stopApplication(ctx context.Context, i Instance) error {
	app, err := k.application(ctx, i)
	if errors.Is(err, domain.NotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Deleting the owned application cascades to its driver and executor Pods.
	err = k.Client.Do(ctx, "DELETE", k.Client.Path("sparkapplications", "linha-"+i.ID), map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]string{"uid": app.Metadata.UID}}, nil)
	var api *kube.APIError
	if errors.As(err, &api) && api.Status == 404 {
		return nil
	}
	return err
}
