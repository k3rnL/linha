package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linha/server/internal/domain"
	"linha/server/internal/kube"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

type Kubernetes struct {
	SecurityDisabled                  bool
	RequireApplication                bool
	TemplateDefaults                  PodTemplates
	ImagePullSecrets, RegistrySecrets []string
	Client                            *kube.Client
	ServerURL, ServiceAccount         string
	AllowedImages                     []string
	Volumes, Mounts                   []map[string]any
}

func (k *Kubernetes) Validate(s domain.EngineSpec) error {
	settings, err := ParseSpark(s)
	if err != nil {
		return err
	}
	if k.RequireApplication && settings.Application == nil {
		return domain.Bad("Spark settings.application is required (mainClass and mainApplicationFile)")
	}
	return k.validateTemplates(settings.Kubernetes, false)
}
func (k *Kubernetes) Normalize(s domain.EngineSpec) (domain.EngineSpec, error) {
	settings, err := ParseSpark(s)
	if err != nil {
		return s, err
	}
	if err = k.validateTemplates(settings.Kubernetes, false); err != nil {
		return s, err
	}
	s.Settings = domain.JSON(settings)
	return s, nil
}
func (k *Kubernetes) CheckImage(image string) error {
	ref, err := name.ParseReference(image)
	if err != nil {
		return domain.Bad("invalid image reference")
	}
	repository := ref.Context().Name()
	for _, allowed := range k.AllowedImages {
		if repository == allowed || strings.HasPrefix(repository, strings.TrimSuffix(allowed, "/")+"/") {
			return nil
		}
	}
	return domain.Bad("image repository is not allowed")
}
func (k *Kubernetes) Ensure(ctx context.Context, c domain.BackendContext, i Instance) error {
	settings, err := ParseSpark(c.Spec.Engine)
	if err != nil {
		return err
	}
	if err = k.validateTemplates(settings.Kubernetes, true); err != nil {
		return err
	}
	if settings.Application != nil {
		return k.ensureApplication(ctx, c, i, settings)
	}
	if settings.Kubernetes != nil {
		if err = k.ensureExecutorTemplate(ctx, c.ID, settings.Kubernetes.Executor); err != nil {
			return err
		}
	}
	podName := "linha-" + i.ID
	labels := map[string]string{"app.kubernetes.io/managed-by": "linha", "linha.io/context": c.ID, "linha.io/instance": i.ID}
	host := podName + "." + k.Client.Namespace + ".svc"
	service := map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": podName, "labels": labels}, "spec": map[string]any{"clusterIP": "None", "publishNotReadyAddresses": true, "selector": map[string]string{"linha.io/instance": i.ID}, "ports": []map[string]any{{"name": "driver", "port": 7078, "targetPort": 7078}, {"name": "blocks", "port": 7079, "targetPort": 7079}}}}
	if err = k.create(ctx, "services", service); err != nil {
		return err
	}
	conf := settings.Conf(k.Client.Namespace, i.Image, host, podName, k.ServiceAccount)
	if settings.Kubernetes == nil {
		for _, volume := range k.Volumes {
			claim, ok := volume["persistentVolumeClaim"].(map[string]any)
			if !ok {
				continue
			}
			name, _ := volume["name"].(string)
			claimName, _ := claim["claimName"].(string)
			if name == "" || claimName == "" {
				return domain.Bad("invalid administrator PVC configuration")
			}
			for _, mount := range k.Mounts {
				if mount["name"] != name {
					continue
				}
				prefix := "spark.kubernetes.executor.volumes.persistentVolumeClaim." + name
				conf[prefix+".options.claimName"] = claimName
				conf[prefix+".mount.path"] = fmt.Sprint(mount["mountPath"])
				if readOnly, ok := mount["readOnly"].(bool); ok {
					conf[prefix+".mount.readOnly"] = fmt.Sprint(readOnly)
				}
				if subPath, ok := mount["subPath"].(string); ok {
					conf[prefix+".mount.subPath"] = subPath
				}
			}
		}
	}
	if settings.Kubernetes != nil {
		conf["spark.kubernetes.executor.podTemplateFile"] = executorTemplateMount + "/executor.json"
		conf["spark.kubernetes.executor.podTemplateContainerName"] = "main"
		if spec, ok := settings.Kubernetes.Executor["spec"].(map[string]any); ok {
			if containers, ok := spec["containers"].([]any); ok {
				for _, raw := range containers {
					c := raw.(map[string]any)
					if c["name"] == "main" {
						if policy, ok := c["imagePullPolicy"].(string); ok {
							conf["spark.kubernetes.container.image.pullPolicy"] = policy
						}
					}
				}
			}
		}
	}
	env := []map[string]any{{"name": "LINHA_DRIVER_MEMORY", "value": fmt.Sprintf("%dm", settings.Drivers.MemoryMi)}, {"name": "LINHA_SERVER_URL", "value": k.ServerURL}, {"name": "LINHA_CONTEXT_ID", "value": c.ID}, {"name": "LINHA_CAPACITY", "value": fmt.Sprint(settings.Drivers.Concurrency)}, {"name": "LINHA_SPARK_CONF", "value": string(domain.JSON(conf))}, {"name": "LINHA_POD_UID", "valueFrom": map[string]any{"fieldRef": map[string]string{"fieldPath": "metadata.uid"}}}, {"name": "LINHA_SECURITY_ENABLED", "value": fmt.Sprint(!k.SecurityDisabled)}}
	legacyVolumes, legacyMounts := k.Volumes, k.Mounts
	if settings.Kubernetes != nil {
		legacyVolumes = nil
		legacyMounts = nil
	}
	volumes := append([]map[string]any{}, legacyVolumes...)
	mounts := append([]map[string]any{}, legacyMounts...)
	if !k.SecurityDisabled {
		env = append(env, map[string]any{"name": "LINHA_WORKER_TOKEN_FILE", "value": "/var/run/linha/token"})
		volumes = append(volumes, map[string]any{"name": "linha-identity", "projected": map[string]any{"sources": []map[string]any{{"serviceAccountToken": map[string]any{"audience": "linha-worker", "expirationSeconds": 3600, "path": "token"}}}}})
		mounts = append(mounts, map[string]any{"name": "linha-identity", "mountPath": "/var/run/linha", "readOnly": true})
	}
	containerName := "driver"
	if settings.Kubernetes != nil {
		containerName = "main"
		volumes = append(volumes, map[string]any{"name": "linha-executor-template", "configMap": map[string]any{"name": executorTemplateName(c.ID)}})
		mounts = append(mounts, map[string]any{"name": "linha-executor-template", "mountPath": executorTemplateMount, "readOnly": true})
	}
	pod := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": podName, "labels": labels}, "spec": map[string]any{"serviceAccountName": k.ServiceAccount, "restartPolicy": "Never", "terminationGracePeriodSeconds": 30, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 185, "fsGroup": 10001}, "volumes": volumes, "containers": []map[string]any{{"name": containerName, "image": i.Image, "imagePullPolicy": "IfNotPresent", "command": []string{"/opt/linha/bin/worker"}, "env": env, "volumeMounts": mounts, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []string{"ALL"}}}, "resources": map[string]any{"requests": map[string]string{"cpu": fmt.Sprint(settings.Drivers.Cores), "memory": fmt.Sprintf("%dMi", settings.Drivers.MemoryMi+512)}, "limits": map[string]string{"cpu": fmt.Sprint(settings.Drivers.Cores), "memory": fmt.Sprintf("%dMi", settings.Drivers.MemoryMi+512)}}}}}}
	if settings.Kubernetes != nil {
		// The template was validated before provisioning, so managed fields cannot be replaced.
		merged := MergePodTemplates(PodTemplate(pod), settings.Kubernetes.Driver)
		pod = map[string]any(merged)
	}
	return k.create(ctx, "pods", pod)
}
func (k *Kubernetes) create(ctx context.Context, kind string, body any) error {
	err := k.Client.Do(ctx, "POST", k.Client.Path(kind, ""), body, nil)
	var api *kube.APIError
	if errors.As(err, &api) && api.Status == 409 {
		var desired struct {
			Metadata struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		}
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
		if decodeErr := json.Unmarshal(encoded, &desired); decodeErr != nil {
			return decodeErr
		}
		var existing struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
		}
		if err = k.Client.Do(ctx, "GET", k.Client.Path(kind, desired.Metadata.Name), nil, &existing); err != nil {
			return err
		}
		for key, value := range desired.Metadata.Labels {
			if existing.Metadata.Labels[key] != value {
				return domain.Conflict("existing resource has different ownership")
			}
		}
		return nil
	}
	return err
}
func (k *Kubernetes) Observe(ctx context.Context, i Instance) (Instance, error) {
	if i.Kind == "sparkapplication" {
		return k.observeApplication(ctx, i)
	}
	var pod struct {
		Metadata struct {
			UID               string            `json:"uid"`
			Labels            map[string]string `json:"labels"`
			DeletionTimestamp *string           `json:"deletionTimestamp"`
		} `json:"metadata"`
		Status struct {
			Phase      string                                           `json:"phase"`
			Conditions []struct{ Type, Status, Reason, Message string } `json:"conditions"`
		} `json:"status"`
	}
	err := k.Client.Do(ctx, "GET", k.Client.Path("pods", "linha-"+i.ID), nil, &pod)
	var api *kube.APIError
	if errors.As(err, &api) && api.Status == 404 {
		return i, domain.NotFound
	}
	if err != nil {
		return i, err
	}
	if pod.Metadata.Labels["linha.io/context"] != i.ContextID || pod.Metadata.Labels["linha.io/instance"] != i.ID {
		return i, domain.Conflict("existing pod is not the assigned managed instance")
	}
	if i.Incarnation != "" && i.Incarnation != pod.Metadata.UID {
		return i, domain.Conflict("managed pod incarnation changed")
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == "PodScheduled" && condition.Status == "False" {
			i.Condition = condition.Reason + ": " + condition.Message
		}
	}
	i.Incarnation = pod.Metadata.UID
	i.Ready = pod.Status.Phase == "Running"
	i.Draining = pod.Metadata.DeletionTimestamp != nil || pod.Status.Phase == "Failed" || pod.Status.Phase == "Succeeded"
	return i, nil
}
func (k *Kubernetes) Drain(ctx context.Context, i Instance) error { return nil } // Durable worker drain is coordinated by the controller.
func (k *Kubernetes) Stop(ctx context.Context, i Instance) error {
	if i.Kind == "sparkapplication" {
		return k.stopApplication(ctx, i)
	}
	if i.Incarnation == "" {
		return fmt.Errorf("refusing to delete pod without UID precondition")
	}
	err := k.Client.Do(ctx, "DELETE", k.Client.Path("pods", "linha-"+i.ID), map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]string{"uid": i.Incarnation}}, nil)
	var api *kube.APIError
	if err != nil && !(errors.As(err, &api) && api.Status == 404) {
		return err
	}
	var service struct {
		Metadata struct {
			UID    string            `json:"uid"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	err = k.Client.Do(ctx, "GET", k.Client.Path("services", "linha-"+i.ID), nil, &service)
	if errors.As(err, &api) && api.Status == 404 {
		return nil
	}
	if err != nil {
		return err
	}
	if service.Metadata.Labels["linha.io/instance"] != i.ID {
		return domain.Conflict("refusing to delete unrelated service")
	}
	return k.Client.Do(ctx, "DELETE", k.Client.Path("services", "linha-"+i.ID), map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]string{"uid": service.Metadata.UID}}, nil)
}
