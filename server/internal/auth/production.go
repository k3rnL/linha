package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"linha/server/internal/kube"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Kubernetes struct {
	WorkerVerifier    *oidc.IDTokenVerifier
	TokenReview       bool
	Verifier          *oidc.IDTokenVerifier
	Kube              *kube.Client
	ServiceAccount    string
	AuthorizeInstance func(context.Context, string, string, string) error
}

func NewOIDCVerifier(ctx context.Context, issuer, audience string) (*oidc.IDTokenVerifier, error) {
	if issuer == "" || audience == "" {
		return nil, fmt.Errorf("OIDC issuer and audience are required")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	return provider.Verifier(&oidc.Config{ClientID: audience}), nil
}
func (p *Kubernetes) Owner(ctx context.Context, r *http.Request) (string, error) {
	if p.Verifier == nil {
		return AnonymousOwner, nil
	}
	token, err := p.Verifier.Verify(ctx, Bearer(r))
	if err != nil || token.Subject == "" {
		return "", Unauthorized
	}
	sum := sha256.Sum256([]byte(token.Issuer + "\x00" + token.Subject))
	return hex.EncodeToString(sum[:]), nil
}
func (p *Kubernetes) Worker(ctx context.Context, r *http.Request) (identity WorkerIdentity, err error) {
	token := Bearer(r)
	if token == "" {
		return identity, Unauthorized
	}
	if !p.TokenReview {
		return p.projectedWorker(ctx, token)
	}
	var review struct {
		Status struct {
			Authenticated bool     `json:"authenticated"`
			Audiences     []string `json:"audiences"`
			User          struct {
				Username string              `json:"username"`
				Extra    map[string][]string `json:"extra"`
			} `json:"user"`
		} `json:"status"`
	}
	err = p.Kube.Do(ctx, "POST", "/apis/authentication.k8s.io/v1/tokenreviews", map[string]any{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenReview", "spec": map[string]any{"token": token, "audiences": []string{"linha-worker"}}}, &review)
	if err != nil {
		return identity, err
	}
	validAudience := false
	for _, a := range review.Status.Audiences {
		if a == "linha-worker" {
			validAudience = true
		}
	}
	if !review.Status.Authenticated || !validAudience || review.Status.User.Username != "system:serviceaccount:"+p.Kube.Namespace+":"+p.ServiceAccount {
		return identity, Unauthorized
	}
	names := review.Status.User.Extra["authentication.kubernetes.io/pod-name"]
	uids := review.Status.User.Extra["authentication.kubernetes.io/pod-uid"]
	if len(names) != 1 || len(uids) != 1 {
		return identity, Unauthorized
	}
	return p.authorizePod(ctx, names[0], uids[0])
}

func (p *Kubernetes) authorizePod(ctx context.Context, name, uid string) (identity WorkerIdentity, err error) {
	if name == "" || uid == "" || strings.ContainsAny(name, "/%?#") {
		return identity, Unauthorized
	}
	var pod struct {
		Metadata struct {
			UID               string            `json:"uid"`
			DeletionTimestamp *string           `json:"deletionTimestamp"`
			Labels            map[string]string `json:"labels"`
			OwnerReferences   []struct {
				APIVersion string `json:"apiVersion"`
				Kind       string `json:"kind"`
				Name       string `json:"name"`
			} `json:"ownerReferences"`
		} `json:"metadata"`
		Spec struct {
			ServiceAccountName string `json:"serviceAccountName"`
		} `json:"spec"`
	}
	if err = p.Kube.Do(ctx, "GET", p.Kube.Path("pods", name), nil, &pod); err != nil {
		return identity, Unauthorized
	}
	if pod.Metadata.DeletionTimestamp != nil || pod.Metadata.UID != uid || pod.Spec.ServiceAccountName != p.ServiceAccount {
		return identity, Unauthorized
	}
	contextID := pod.Metadata.Labels["linha.io/context"]
	instanceID := pod.Metadata.Labels["linha.io/instance"]
	validName := name == "linha-"+instanceID
	if name == "linha-"+instanceID+"-driver" && pod.Metadata.Labels["spark-role"] == "driver" {
		for _, owner := range pod.Metadata.OwnerReferences {
			if owner.APIVersion == "sparkoperator.k8s.io/v1beta2" && owner.Kind == "SparkApplication" && owner.Name == "linha-"+instanceID {
				validName = true
			}
		}
	}
	if contextID == "" || instanceID == "" || !validName {
		return identity, Unauthorized
	}
	if err = p.AuthorizeInstance(ctx, contextID, instanceID, uid); err != nil {
		return identity, Unauthorized
	}
	return WorkerIdentity{ContextID: contextID, ID: uid, Incarnation: uid}, nil
}
