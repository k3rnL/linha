package engine

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"linha/server/internal/domain"
	"net/url"
	"strings"
)

func addPodResources(out map[string]float64, role string, spec corev1.PodSpec) bool {
	complete := false
	for _, c := range spec.Containers {
		if c.Name != "main" && c.Name != "spark-kubernetes-driver" && c.Name != "spark-kubernetes-executor" {
			continue
		}
		complete = true
		for allocation, resources := range map[string]corev1.ResourceList{"request": c.Resources.Requests, "limit": c.Resources.Limits} {
			if q, ok := resources[corev1.ResourceCPU]; ok {
				out[role+"_cpu_"+allocation] += q.AsApproximateFloat64()
			} else {
				complete = false
			}
			if q, ok := resources[corev1.ResourceMemory]; ok {
				out[role+"_memory_"+allocation] += float64(q.Value())
			} else {
				complete = false
			}
		}
	}
	return complete
}
func (k *Kubernetes) completeSparkObservation(ctx context.Context, i Instance, driverLabels map[string]string) error {
	o := i.Observation
	o.ExecutorStates = map[string]int{}
	complete := true
	for _, r := range []string{"cpu_request", "cpu_limit", "memory_request", "memory_limit"} {
		if _, ok := o.Resources["driver_"+r]; !ok {
			complete = false
		}
		o.Resources["executor_"+r] = 0
	}
	selector := "linha.io/context=" + i.ContextID + ",linha.io/instance=" + i.ID + ",spark-role=executor"
	query := url.Values{"labelSelector": {selector}, "limit": {"200"}}
	for pages := 0; pages < 64; pages++ {
		var list corev1.PodList
		if e := k.Client.Do(ctx, "GET", k.Client.Path("pods", "")+"?"+query.Encode(), nil, &list); e != nil {
			return e
		}
		for _, p := range list.Items {
			owned := false
			for _, r := range p.OwnerReferences {
				if string(r.UID) == i.Incarnation || string(r.UID) == i.ResourceUID {
					owned = true
				}
			}
			if !owned || p.Labels["linha.io/context"] != i.ContextID || p.Labels["linha.io/instance"] != i.ID {
				continue
			}
			phase := string(p.Status.Phase)
			switch phase {
			case "Pending", "Running", "Succeeded", "Failed":
			default:
				phase = "Unknown"
			}
			if p.DeletionTimestamp != nil {
				phase = "Terminating"
			}
			o.ExecutorStates[phase]++
			if !addPodResources(o.Resources, "executor", p.Spec) {
				complete = false
			}
		}
		if list.Continue == "" {
			break
		}
		if pages == 63 {
			return fmt.Errorf("executor observation exceeded page budget")
		}
		query.Set("continue", list.Continue)
	}
	o.Available = complete
	if !complete {
		o.Condition = "missing_resources"
	}
	if i.Draining || !i.Ready {
		return nil
	}
	address := strings.TrimSpace(o.UIAddress)
	if address == "" {
		return nil
	}
	u, e := url.Parse(address)
	if e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil {
		o.Links = []domain.EngineLink{{Name: "spark-ui", URL: u.String()}}
		return nil
	}
	if strings.Contains(address, "://") || strings.HasPrefix(address, "//") || strings.ContainsAny(address, "\r\n") {
		return nil
	}
	u, e = url.Parse("//" + address)
	if e != nil || u.Hostname() == "" || u.User != nil {
		return nil
	}
	// A scheme-less operator address needs matching owned ingress metadata.
	var ingresses networkingv1.IngressList
	if e = k.Client.Do(ctx, "GET", k.Client.Path("ingresses", "")+"?limit=200", nil, &ingresses); e != nil {
		return nil
	}
	for _, in := range ingresses.Items {
		owned := false
		for _, r := range in.OwnerReferences {
			if string(r.UID) == i.ResourceUID || string(r.UID) == i.Incarnation {
				owned = true
			}
		}
		if !owned {
			continue
		}
		for _, rule := range in.Spec.Rules {
			if rule.Host != u.Hostname() || rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if !strings.HasPrefix(u.Path, path.Path) && !(u.Path == "" && path.Path == "/") {
					continue
				}
				u.Scheme = "http"
				for _, tls := range in.Spec.TLS {
					for _, host := range tls.Hosts {
						if host == rule.Host {
							u.Scheme = "https"
						}
					}
				}
				o.Links = []domain.EngineLink{{Name: "spark-ui", URL: u.String()}}
				return nil
			}
		}
	}
	return nil
}
