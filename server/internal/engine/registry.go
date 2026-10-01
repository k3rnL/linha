package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"linha/server/internal/domain"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"k8s.io/apimachinery/pkg/util/validation"
)

type registryCredential struct {
	scope string
	auth  authn.AuthConfig
}
type registryKeychain []registryCredential

func (k registryKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	for _, credential := range k {
		if resource.String() == credential.scope || strings.HasPrefix(resource.String(), credential.scope+"/") || resource.RegistryStr() == credential.scope {
			return authn.FromConfig(credential.auth), nil
		}
	}
	return authn.Anonymous, nil
}
func registryScope(scope string) string {
	scope = strings.TrimPrefix(strings.TrimPrefix(scope, "https://"), "http://")
	scope = strings.TrimSuffix(scope, "/")
	if scope == "index.docker.io/v1" || scope == "registry-1.docker.io" || scope == "docker.io" {
		return "index.docker.io"
	}
	return scope
}
func (k *Kubernetes) registryKeychain(ctx context.Context, refs []string) (authn.Keychain, error) {
	credentials := registryKeychain{}
	seen := map[string]bool{}
	// Validate the entire set before issuing any Secret GET.
	for _, ref := range refs {
		if len(validation.IsDNS1123Subdomain(ref)) != 0 {
			return nil, domain.Bad("invalid image pull Secret name")
		}
		if !k.allowedRegistrySecret(ref) {
			return nil, domain.Bad("image pull Secret is not permitted: " + ref)
		}
	}
	for _, ref := range refs {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		var secret struct {
			Type string            `json:"type"`
			Data map[string][]byte `json:"data"`
		}
		if err := k.Client.Do(ctx, "GET", k.Client.Path("secrets", ref), nil, &secret); err != nil {
			return nil, fmt.Errorf("registry Secret %s is unavailable", ref)
		}
		var entries map[string]authn.AuthConfig
		switch secret.Type {
		case "kubernetes.io/dockerconfigjson":
			var config struct {
				Auths map[string]authn.AuthConfig `json:"auths"`
			}
			if err := json.Unmarshal(secret.Data[".dockerconfigjson"], &config); err != nil {
				return nil, fmt.Errorf("registry Secret %s contains invalid dockerconfigjson", ref)
			}
			entries = config.Auths
		case "kubernetes.io/dockercfg":
			if err := json.Unmarshal(secret.Data[".dockercfg"], &entries); err != nil {
				return nil, fmt.Errorf("registry Secret %s contains invalid dockercfg", ref)
			}
		default:
			return nil, fmt.Errorf("registry Secret %s must be dockerconfigjson or dockercfg", ref)
		}
		scopes := make([]string, 0, len(entries))
		for scope := range entries {
			scopes = append(scopes, scope)
		}
		sort.Strings(scopes)
		for _, scope := range scopes {
			credentials = append(credentials, registryCredential{registryScope(scope), entries[scope]})
		}
	}
	sort.SliceStable(credentials, func(i, j int) bool { return len(credentials[i].scope) > len(credentials[j].scope) })
	return authn.NewMultiKeychain(credentials, authn.DefaultKeychain), nil
}
func (k *Kubernetes) Resolve(ctx context.Context, spec domain.BackendSpec) (string, error) {
	if err := k.CheckImage(spec.Image); err != nil {
		return "", err
	}
	ref, err := name.ParseReference(spec.Image)
	if err != nil {
		return "", err
	}
	if _, ok := ref.(name.Digest); ok {
		return ref.Name(), nil
	}
	settings, err := ParseSpark(spec.Engine)
	if err != nil {
		return "", err
	}
	refs := append([]string{}, k.ImagePullSecrets...)
	if settings.Kubernetes != nil {
		refs = append(pullSecrets(settings.Kubernetes.Driver), pullSecrets(settings.Kubernetes.Executor)...)
	}
	keychain, err := k.registryKeychain(ctx, refs)
	if err != nil {
		return "", err
	}
	descriptor, err := remote.Head(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(keychain))
	if err != nil {
		// Registry response bodies and token endpoint URLs may contain credentials.
		var status *transport.Error
		if errors.As(err, &status) {
			return "", fmt.Errorf("registry lookup failed: HTTP %d %s", status.StatusCode, http.StatusText(status.StatusCode))
		}
		return "", fmt.Errorf("registry lookup failed; check credentials, network connectivity and registry CA")
	}
	return ref.Context().Digest(descriptor.Digest.String()).String(), nil
}
