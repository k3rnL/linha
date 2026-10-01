// Package kube contains the small Kubernetes REST surface needed by Linha.
package kube

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	URL, TokenFile, Namespace string
	HTTP                      *http.Client
}
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("Kubernetes %d: %s", e.Status, e.Message) }
func InCluster(namespace string) (*Client, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		return nil, fmt.Errorf("Kubernetes service address is missing")
	}
	cert, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		return nil, fmt.Errorf("invalid Kubernetes CA")
	}
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if port == "" {
		port = "443"
	}
	return &Client{URL: "https://" + host + ":" + port, TokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token", Namespace: namespace, HTTP: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}}, nil
}
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if c.TokenFile != "" {
		token, err := os.ReadFile(c.TokenFile)
		if err != nil {
			return err
		}
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	r.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		var status struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(b, &status)
		return &APIError{response.StatusCode, status.Message}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(out)
	}
	return nil
}
func (c *Client) Path(resource, name string) string {
	path := "/api/v1/namespaces/" + c.Namespace + "/" + resource
	if resource == "sparkapplications" {
		path = "/apis/sparkoperator.k8s.io/v1beta2/namespaces/" + c.Namespace + "/" + resource
	}
	if name != "" {
		path += "/" + name
	}
	return path
}

// AuthenticatedHTTPClient is restricted to this API server and never follows
// redirects with a service-account credential.
func (c *Client) AuthenticatedHTTPClient() *http.Client {
	return &http.Client{Timeout: c.HTTP.Timeout, Transport: &authenticatedTransport{client: c}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type authenticatedTransport struct{ client *Client }

func (t *authenticatedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != strings.TrimRight(t.client.URL, "/") {
		return nil, fmt.Errorf("Kubernetes credential destination mismatch")
	}
	request := r.Clone(r.Context())
	if t.client.TokenFile != "" {
		token, err := os.ReadFile(t.client.TokenFile)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	transport := t.client.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(request)
}
