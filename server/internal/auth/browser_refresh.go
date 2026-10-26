package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"linha/server/internal/domain"
)

type browserTokens struct {
	Token    oauth2.Token
	Nonce    string
	ClientID string
}

func browserCipher(cookie string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("linha/browser-refresh/v1\x00" + cookie))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func encryptBrowserTokens(cookie string, tokens browserTokens) ([]byte, error) {
	aead, err := browserCipher(cookie)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(tokens)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, raw, []byte(secretHash(cookie))), nil
}
func decryptBrowserTokens(cookie string, encrypted []byte) (tokens browserTokens, err error) {
	aead, err := browserCipher(cookie)
	if err != nil {
		return tokens, err
	}
	if len(encrypted) < aead.NonceSize() {
		return tokens, Unauthorized
	}
	raw, err := aead.Open(nil, encrypted[:aead.NonceSize()], encrypted[aead.NonceSize():], []byte(secretHash(cookie)))
	if err != nil {
		return tokens, Unauthorized
	}
	if json.Unmarshal(raw, &tokens) != nil {
		return tokens, Unauthorized
	}
	return tokens, nil
}
func authenticationExpiry(identity time.Time, token *oauth2.Token, absolute time.Time) time.Time {
	expiry := absolute
	if !identity.IsZero() && identity.Before(expiry) {
		expiry = identity
	}
	if !token.Expiry.IsZero() && token.Expiry.Before(expiry) {
		expiry = token.Expiry
	}
	return expiry
}
func providerUnavailable() error {
	return &domain.Error{Code: "UNAVAILABLE", Message: "identity provider temporarily unavailable", Status: 503}
}
func (a *Admin) renewBrowserSession(ctx context.Context, cookie string, current domain.BrowserSession) (domain.BrowserSession, error) {
	if current.AuthExpiresAt == nil {
		return current, nil
	}
	if len(current.RefreshCipher) == 0 {
		if !current.AuthExpiresAt.After(time.Now()) {
			return current, Unauthorized
		}
		return current, nil
	}
	if current.AuthExpiresAt.After(time.Now().Add(30 * time.Second)) {
		return current, nil
	}
	refreshed, err := a.Repo.RefreshSession(ctx, current.Hash, func(locked domain.BrowserSession) (domain.BrowserSession, error) {
		if locked.AuthExpiresAt != nil && locked.AuthExpiresAt.After(time.Now().Add(30*time.Second)) {
			return locked, nil
		}
		return a.refreshBrowserIdentity(ctx, cookie, locked)
	})
	if errors.Is(err, domain.NotFound) {
		return current, Unauthorized
	}
	if err != nil {
		var typed *domain.Error
		if errors.As(err, &typed) && typed.Status >= 500 && current.AuthExpiresAt.After(time.Now()) {
			return current, nil
		}
		return current, err
	}
	if refreshed.AuthExpiresAt != nil && !refreshed.AuthExpiresAt.After(time.Now()) {
		return refreshed, providerUnavailable()
	}
	return refreshed, nil
}
func (a *Admin) refreshBrowserIdentity(ctx context.Context, cookie string, session domain.BrowserSession) (domain.BrowserSession, error) {
	previous, err := decryptBrowserTokens(cookie, session.RefreshCipher)
	if err != nil || previous.Token.RefreshToken == "" {
		return session, Unauthorized
	}
	var oldClaims map[string]any
	if json.Unmarshal(session.Claims, &oldClaims) != nil {
		return session, Unauthorized
	}
	if issuer, ok := oldClaims["iss"].(string); !ok || issuer != a.Config.Issuer {
		return session, Unauthorized
	}
	if subject, ok := oldClaims["sub"].(string); !ok || subject == "" || previous.ClientID != a.Config.ClientID {
		return session, Unauthorized
	}

	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if a.oidcHTTPClient != nil {
		call = oidc.ClientContext(call, a.oidcHTTPClient)
	}
	previous.Token.Expiry = time.Now().Add(-time.Minute)
	token, err := a.OAuth.TokenSource(call, &previous.Token).Token()
	if err != nil {
		var response *oauth2.RetrieveError
		if errors.As(err, &response) && (response.ErrorCode == "invalid_grant" || response.ErrorCode == "access_denied") {
			slog.InfoContext(ctx, "browser session ended", "reason", "refresh_rejected")
			return session, Unauthorized
		}
		slog.WarnContext(ctx, "browser session renewal unavailable", "reason", "provider_unavailable")
		return session, providerUnavailable()
	}
	if !session.ExpiresAt.After(time.Now()) {
		return session, Unauthorized
	}
	// Persist a rotated refresh credential even if the subsequent UserInfo call is temporarily unavailable.
	if token.RefreshToken == "" {
		token.RefreshToken = previous.Token.RefreshToken
	}
	session.RefreshCipher, err = encryptBrowserTokens(cookie, browserTokens{Token: *token, Nonce: previous.Nonce, ClientID: previous.ClientID})
	if err != nil {
		return session, providerUnavailable()
	}
	var claims map[string]any
	identityExpiry := time.Time{}
	if raw := token.Extra("id_token"); raw != nil {
		text, ok := raw.(string)
		if !ok || text == "" || a.Verifier == nil {
			return session, Unauthorized
		}
		identity, e := a.Verifier.Verify(call, text)
		if e != nil || identity.Subject != oldClaims["sub"] || identity.Nonce != "" && identity.Nonce != previous.Nonce {
			return session, Unauthorized
		}
		if identity.AccessTokenHash != "" && identity.VerifyAccessToken(token.AccessToken) != nil {
			return session, Unauthorized
		}
		if identity.Claims(&claims) != nil {
			return session, Unauthorized
		}
		identityExpiry = identity.Expiry
	} else {
		if a.provider == nil {
			return session, nil
		}
		info, e := a.provider.UserInfo(call, oauth2.StaticTokenSource(token))
		if e != nil {
			slog.WarnContext(ctx, "browser session renewal unavailable", "reason", "userinfo_unavailable")
			return session, nil
		}
		if info.Subject == "" || info.Subject != oldClaims["sub"] || info.Claims(&claims) != nil {
			return session, Unauthorized
		}
		if iss, ok := claims["iss"]; ok && iss != a.Config.Issuer {
			return session, Unauthorized
		}
		claims["iss"] = a.Config.Issuer
		claims["aud"] = a.Config.ClientID
		// UserInfo is freshly authenticated; bound its cache even if no access-token expiry is supplied.
		identityExpiry = time.Now().Add(5 * time.Minute)
	}
	if a.role(claims) == "" {
		a.logMembershipDenied(ctx, claims)
		return session, Forbidden
	}
	expiry := authenticationExpiry(identityExpiry, token, session.ExpiresAt)
	if !expiry.After(time.Now()) {
		return session, Unauthorized
	}
	if token.RefreshToken == "" {
		token.RefreshToken = previous.Token.RefreshToken
	}
	encrypted, err := encryptBrowserTokens(cookie, browserTokens{Token: *token, Nonce: previous.Nonce, ClientID: previous.ClientID})
	if err != nil {
		return session, providerUnavailable()
	}
	session.Claims, session.AuthExpiresAt, session.RefreshCipher = domain.JSON(claims), &expiry, encrypted
	return session, nil
}
