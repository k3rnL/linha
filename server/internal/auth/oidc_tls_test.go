package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func TestOIDCTLSHostnameVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"provider.example"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys"})
	}))
	provider.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	provider.StartTLS()
	defer provider.Close()
	issuer = provider.URL // The trusted certificate deliberately has no loopback IP SAN.
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	defer transport.CloseIdleConnections()
	strict := oidc.ClientContext(context.Background(), &http.Client{Transport: transport})
	_, err = NewOIDCVerifier(strict, issuer, "linha")
	var hostnameError x509.HostnameError
	if !errors.As(err, &hostnameError) {
		t.Fatalf("expected hostname mismatch for a trusted certificate, got %v", err)
	}
	if _, err = NewOIDCVerifier(OIDCContext(context.Background(), true), issuer, "linha"); err != nil {
		t.Fatalf("explicit bypass did not skip hostname verification: %v", err)
	}
}

func TestOIDCTLSTrustedCAWithoutBypass(t *testing.T) {
	var issuer string
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys"})
	}))
	defer provider.Close()
	issuer = provider.URL
	trusted := oidc.ClientContext(context.Background(), provider.Client())
	if _, err := NewOIDCVerifier(OIDCContext(trusted, false), issuer, "linha"); err != nil {
		t.Fatalf("trusted private CA failed with verification enabled: %v", err)
	}
}
