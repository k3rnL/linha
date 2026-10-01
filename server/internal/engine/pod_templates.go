package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"linha/server/internal/domain"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const maxPodTemplateBytes = 64 << 10
const executorTemplateMount = "/var/run/linha-pod-templates"

type PodTemplate map[string]any
type PodTemplates struct {
	Driver   PodTemplate `json:"driverPodTemplate,omitempty"`
	Executor PodTemplate `json:"executorPodTemplate,omitempty"`
}

// MergePodTemplates has explicit list semantics: containers merge recursively by
// name, env/volumes/pull secrets replace by name, mounts replace by mountPath.
// Other lists replace. Neither input is mutated and deletion directives are unsupported.
func MergePodTemplates(base, override PodTemplate) PodTemplate {
	a, b := cloneTemplate(base), cloneTemplate(override)
	return mergeObject(a, b)
}
func cloneTemplate(t PodTemplate) PodTemplate {
	var copy PodTemplate
	_ = json.Unmarshal(domain.JSON(t), &copy)
	if copy == nil {
		copy = PodTemplate{}
	}
	return copy
}
func mergeObject(a, b map[string]any) map[string]any {
	for key, value := range b {
		if bm, ok := value.(map[string]any); ok {
			am, _ := a[key].(map[string]any)
			if am == nil {
				am = map[string]any{}
			}
			a[key] = mergeObject(am, bm)
			continue
		}
		list, listOK := value.([]any)
		identity := ""
		switch key {
		case "containers", "initContainers", "env", "volumes", "imagePullSecrets":
			identity = "name"
		case "volumeMounts":
			identity = "mountPath"
		}
		if listOK && identity != "" {
			previous, _ := a[key].([]any)
			for _, entry := range list {
				item, ok := entry.(map[string]any)
				if !ok {
					previous = append(previous, entry)
					continue
				}
				found := false
				for i, old := range previous {
					oldMap, ok := old.(map[string]any)
					if ok && item[identity] != nil && oldMap[identity] == item[identity] {
						if key == "containers" || key == "initContainers" {
							previous[i] = mergeObject(oldMap, item)
						} else {
							previous[i] = item
						}
						found = true
						break
					}
				}
				if !found {
					previous = append(previous, item)
				}
			}
			// Keep empty arrays as arrays, never null.
			if previous == nil {
				previous = []any{}
			}
			a[key] = previous
			continue
		}
		a[key] = value
	}
	return a
}
func templateError(field, message string) error { return domain.Bad(field + ": " + message) }
func validateLists(value any, field string) error {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if child == nil {
				return templateError(field+"."+key, "null values are not supported; omit the field")
			}
			identity := ""
			switch key {
			case "containers", "initContainers", "volumes", "env", "imagePullSecrets":
				identity = "name"
			case "volumeMounts":
				identity = "mountPath"
			}
			// Keys in arbitrary label/annotation/selector maps are not Pod fields.
			parent := field
			if i := strings.LastIndex(parent, "["); i >= 0 && strings.HasSuffix(parent, "]") {
				parent = parent[:i]
			}
			isContainer := strings.HasSuffix(parent, ".spec.containers") || strings.HasSuffix(parent, ".spec.initContainers")
			if (key == "env" || key == "volumeMounts") && !isContainer {
				identity = ""
			}
			if key != "env" && key != "volumeMounts" && !strings.HasSuffix(field, ".spec") {
				identity = ""
			}
			if identity != "" {
				list, ok := child.([]any)
				if !ok {
					return templateError(field+"."+key, "must be an array")
				}
				seen := map[string]bool{}
				for _, item := range list {
					m, ok := item.(map[string]any)
					if !ok {
						return templateError(field+"."+key, "entries must be objects")
					}
					name, _ := m[identity].(string)
					if name == "" || seen[name] {
						return templateError(field+"."+key, "entries need unique "+identity+" values")
					}
					seen[name] = true
				}
			}
			if err := validateLists(child, field+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range v {
			if err := validateLists(child, fmt.Sprintf("%s[%d]", field, i)); err != nil {
				return err
			}
		}
	}
	return nil
}
func ValidatePodTemplate(t PodTemplate, field string, complete bool) error {
	if t == nil {
		return nil
	}
	data := domain.JSON(t)
	if len(data) > maxPodTemplateBytes {
		return templateError(field, "template exceeds 64 KiB")
	}
	if err := validateLists(map[string]any(t), field); err != nil {
		return err
	}
	var decoded struct {
		Metadata struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec corev1.PodSpec `json:"spec"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return templateError(field, "invalid Kubernetes template: "+err.Error())
	}
	for _, labels := range []map[string]string{decoded.Metadata.Labels, decoded.Metadata.Annotations} {
		for key := range labels {
			if strings.HasPrefix(key, "linha.io/") || strings.HasPrefix(key, "spark-") || strings.HasPrefix(key, "sparkoperator.k8s.io/") || key == "app.kubernetes.io/managed-by" {
				return templateError(field+".metadata."+key, "managed by Linha or Spark")
			}
		}
	}
	spec, _ := t["spec"].(map[string]any)
	for _, key := range []string{"serviceAccountName", "serviceAccount", "automountServiceAccountToken", "restartPolicy", "ephemeralContainers", "resources"} {
		if _, exists := spec[key]; exists {
			return templateError(field+".spec."+key, "managed by Linha/Spark")
		}
	}
	volumeNames := map[string]bool{}
	for _, v := range decoded.Spec.Volumes {
		if len(validation.IsDNS1123Label(v.Name)) > 0 || reservedVolume(v.Name) {
			return templateError(field+".spec.volumes", "invalid or reserved volume name "+v.Name)
		}
		volumeNames[v.Name] = true
	}
	for _, s := range decoded.Spec.ImagePullSecrets {
		if len(validation.IsDNS1123Subdomain(s.Name)) > 0 {
			return templateError(field+".spec.imagePullSecrets", "invalid Secret name")
		}
	}
	containerNames := map[string]bool{}
	for _, group := range []struct {
		name       string
		containers []corev1.Container
	}{{"containers", decoded.Spec.Containers}, {"initContainers", decoded.Spec.InitContainers}} {
		for _, c := range group.containers {
			f := field + ".spec." + group.name + "[" + c.Name + "]"
			if len(validation.IsDNS1123Label(c.Name)) > 0 || containerNames[c.Name] {
				return templateError(f, "invalid or duplicate container name")
			}
			containerNames[c.Name] = true
			main := c.Name == "main" && group.name == "containers"
			if c.Name == "main" && !main {
				return templateError(f, "main is reserved for the runtime container")
			}
			if main {
				for _, raw := range spec[group.name].([]any) {
					m := raw.(map[string]any)
					if m["name"] != "main" {
						continue
					}
					for _, key := range []string{"command", "args", "image", "workingDir", "restartPolicy"} {
						if _, exists := m[key]; exists {
							return templateError(f+"."+key, "managed by Linha/Spark")
						}
					}
				}
				for _, values := range []corev1.ResourceList{c.Resources.Requests, c.Resources.Limits} {
					for _, key := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
						if _, exists := values[key]; exists {
							return templateError(f+".resources."+string(key), "use Spark driver/executor resource settings")
						}
					}
				}
				for _, env := range c.Env {
					if strings.HasPrefix(env.Name, "LINHA_") || strings.HasPrefix(env.Name, "SPARK_") {
						return templateError(f+".env["+env.Name+"]", "managed by Linha/Spark")
					}
				}
			} else if complete && c.Image == "" {
				return templateError(f+".image", "an image is required")
			}
			for _, m := range c.VolumeMounts {
				if reservedVolume(m.Name) || protectedMount(m.MountPath) {
					return templateError(f+".volumeMounts["+m.Name+"]", "overlaps a managed runtime mount")
				}
				if complete && !volumeNames[m.Name] {
					return templateError(f+".volumeMounts["+m.Name+"]", "volume is not defined")
				}
			}
		}
	}
	return nil
}
func reservedVolume(name string) bool {
	return strings.HasPrefix(name, "linha-") || strings.HasPrefix(name, "spark-") || strings.HasPrefix(name, "kube-api-access-")
}
func protectedMount(mount string) bool {
	cleaned := path.Clean(mount)
	if !strings.HasPrefix(mount, "/") || mount != cleaned {
		return true
	}
	for _, reserved := range []string{"/opt/linha", "/var/run/linha", executorTemplateMount, "/opt/spark/conf", "/var/run/secrets/kubernetes.io/serviceaccount"} {
		if cleaned == reserved || strings.HasPrefix(cleaned, reserved+"/") || strings.HasPrefix(reserved, strings.TrimSuffix(cleaned, "/")+"/") {
			return true
		}
	}
	return false
}
func (k *Kubernetes) validateTemplates(p *PodTemplates, complete bool) error {
	if p == nil {
		return nil
	}
	for _, role := range []struct {
		name     string
		template PodTemplate
	}{{"driverPodTemplate", p.Driver}, {"executorPodTemplate", p.Executor}} {
		field := "engine.settings.kubernetes." + role.name
		if err := ValidatePodTemplate(role.template, field, complete); err != nil {
			return err
		}
		spec, _ := role.template["spec"].(map[string]any)
		for _, group := range []string{"containers", "initContainers"} {
			containers, _ := spec[group].([]any)
			for _, raw := range containers {
				c := raw.(map[string]any)
				if group == "containers" && c["name"] == "main" {
					continue
				}
				image, _ := c["image"].(string)
				if image == "" {
					continue
				}
				if err := k.CheckImage(image); err != nil {
					return templateError(field+".spec."+group+"["+c["name"].(string)+"].image", "image repository is not allowed")
				}
			}
		}
		for _, name := range pullSecrets(role.template) {
			if !k.allowedRegistrySecret(name) {
				return templateError(field+".spec.imagePullSecrets["+name+"]", "Secret is not permitted by registrySecrets.allowedNames or deployment imagePullSecrets")
			}
		}
	}
	return nil
}
func pullSecrets(t PodTemplate) []string {
	spec, _ := t["spec"].(map[string]any)
	list, _ := spec["imagePullSecrets"].([]any)
	names := []string{}
	for _, v := range list {
		entry, ok := v.(map[string]any)
		if ok {
			name, _ := entry["name"].(string)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}
func (k *Kubernetes) allowedRegistrySecret(name string) bool {
	for _, allowed := range append(append([]string{}, k.RegistrySecrets...), k.ImagePullSecrets...) {
		if name == allowed {
			return true
		}
	}
	return false
}

// Prepare snapshots administrator defaults only for newly accepted contexts.
func (k *Kubernetes) Prepare(s domain.EngineSpec) (domain.EngineSpec, error) {
	settings, err := ParseSpark(s)
	if err != nil {
		return s, err
	}
	client := settings.Kubernetes
	if client == nil {
		client = &PodTemplates{}
	}
	defaults := k.TemplateDefaults
	if err = k.validateTemplates(&defaults, false); err != nil {
		return s, err
	}
	driver, executor := k.legacyTemplates()
	driver = MergePodTemplates(driver, defaults.Driver)
	executor = MergePodTemplates(executor, defaults.Executor)
	main := PodTemplate{"spec": map[string]any{"containers": []any{map[string]any{"name": "main"}}}}
	effective := &PodTemplates{Driver: MergePodTemplates(MergePodTemplates(main, driver), client.Driver), Executor: MergePodTemplates(MergePodTemplates(main, executor), client.Executor)}
	if err = k.validateTemplates(effective, true); err != nil {
		return s, err
	}
	settings.Kubernetes = effective
	s.Settings = domain.JSON(settings)
	return s, nil
}
func (k *Kubernetes) legacyTemplates() (PodTemplate, PodTemplate) {
	secrets := []any{}
	for _, name := range k.ImagePullSecrets {
		secrets = append(secrets, map[string]any{"name": name})
	}
	driverVolumes, driverMounts, executorVolumes, executorMounts := []any{}, []any{}, []any{}, []any{}
	for _, v := range k.Volumes {
		driverVolumes = append(driverVolumes, v)
		if _, ok := v["persistentVolumeClaim"]; ok {
			executorVolumes = append(executorVolumes, v)
			for _, m := range k.Mounts {
				if m["name"] == v["name"] {
					executorMounts = append(executorMounts, m)
				}
			}
		}
	}
	for _, m := range k.Mounts {
		driverMounts = append(driverMounts, m)
	}
	makeTemplate := func(volumes, mounts []any) PodTemplate {
		return cloneTemplate(PodTemplate{"spec": map[string]any{"imagePullSecrets": secrets, "volumes": volumes, "containers": []any{map[string]any{"name": "main", "volumeMounts": mounts}}}})
	}
	return makeTemplate(driverVolumes, driverMounts), makeTemplate(executorVolumes, executorMounts)
}
