package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"linha/server/internal/kube"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Discover signing keys through the trusted Kubernetes endpoint. Never send the
// server's service-account credential to a discovery-supplied external URL.
func NewWorkerVerifier(ctx context.Context, k *kube.Client) (*oidc.IDTokenVerifier, error) {
	var discovery struct {
		Issuer     string   `json:"issuer"`
		Algorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := k.Do(ctx, http.MethodGet, "/.well-known/openid-configuration", nil, &discovery); err != nil {
		return nil, fmt.Errorf("Kubernetes service-account discovery unavailable: %w", err)
	}
	if discovery.Issuer == "" {
		return nil, fmt.Errorf("Kubernetes discovery has no issuer")
	}
	remote := oidc.NewRemoteKeySet(oidc.ClientContext(ctx, k.AuthenticatedHTTPClient()), k.URL+"/openid/v1/jwks")
	return oidc.NewVerifier(discovery.Issuer, remote, &oidc.Config{ClientID: "linha-worker", SupportedSigningAlgs: discovery.Algorithms}), nil
}

func (p *Kubernetes) projectedWorker(ctx context.Context, raw string) (identity WorkerIdentity, err error) {
	if p.WorkerVerifier == nil {
		return identity, Unauthorized
	}
	token, err := p.WorkerVerifier.Verify(ctx, raw)
	if err != nil || token.Subject != "system:serviceaccount:"+p.Kube.Namespace+":"+p.ServiceAccount {
		return identity, Unauthorized
	}
	var claims struct {
		Kubernetes struct {
			Namespace      string                     `json:"namespace"`
			Pod            struct{ Name, UID string } `json:"pod"`
			ServiceAccount struct{ Name, UID string } `json:"serviceaccount"`
		} `json:"kubernetes.io"`
	}
	if token.Claims(&claims) != nil {
		return identity, Unauthorized
	}
	bound := claims.Kubernetes
	if bound.Namespace != p.Kube.Namespace || bound.ServiceAccount.Name != p.ServiceAccount || bound.ServiceAccount.UID == "" || strings.ContainsAny(p.ServiceAccount, "/%?#") {
		return identity, Unauthorized
	}
	// Offline signature verification alone does not revoke a deleted/recreated SA
	// or Pod. Read both identities on every call before authorizing the assignment.
	var sa struct {
		Metadata struct {
			UID               string  `json:"uid"`
			DeletionTimestamp *string `json:"deletionTimestamp"`
		} `json:"metadata"`
	}
	if p.Kube.Do(ctx, http.MethodGet, p.Kube.Path("serviceaccounts", p.ServiceAccount), nil, &sa) != nil || sa.Metadata.UID != bound.ServiceAccount.UID || sa.Metadata.DeletionTimestamp != nil {
		return identity, Unauthorized
	}
	return p.authorizePod(ctx, bound.Pod.Name, bound.Pod.UID)
}
