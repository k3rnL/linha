package auth

import (
	"context"
	"linha/server/internal/domain"
	"net/http"
	"strings"
)

type WorkerIdentity struct {
	ContextID   string `json:"contextId"`
	ID          string `json:"id"`
	Incarnation string `json:"incarnation"`
	ExpiresAt   int64  `json:"expiresAt"`
}
type Authenticator interface {
	Owner(context.Context, *http.Request) (string, error)
	Worker(context.Context, *http.Request) (WorkerIdentity, error)
}

var Unauthorized = &domain.Error{Code: "UNAUTHORIZED", Message: "valid credentials required", Status: 401}

const AnonymousOwner = "anonymous"

func Bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

// Disabled carries routing identity only: every caller has the same owner and
// worker identity headers are untrusted. Attempt fencing still applies.
type Disabled struct{}

func (*Disabled) Owner(context.Context, *http.Request) (string, error) { return AnonymousOwner, nil }
func (*Disabled) Worker(_ context.Context, r *http.Request) (WorkerIdentity, error) {
	w := WorkerIdentity{ContextID: r.Header.Get("X-Linha-Context-Id"), ID: r.Header.Get("X-Linha-Worker-Id"), Incarnation: r.Header.Get("X-Linha-Worker-Incarnation")}
	if w.ContextID == "" || w.ID == "" || w.Incarnation == "" {
		return w, domain.Bad("worker context, ID and incarnation headers are required")
	}
	return w, nil
}
