package auth

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCContext scopes an optional certificate-verification bypass to provider
// discovery, signing-key retrieval and OAuth requests using this context.
// Neither the parent context nor the default HTTP transport is modified.
func OIDCContext(ctx context.Context, insecureSkipVerify bool) context.Context {
	if !insecureSkipVerify {
		return ctx
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	// Explicit operator opt-in; JWT verification remains enforced separately.
	transport.TLSClientConfig.InsecureSkipVerify = true
	client := *http.DefaultClient
	client.Transport = transport
	return oidc.ClientContext(ctx, &client)
}
