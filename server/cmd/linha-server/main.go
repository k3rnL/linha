package main

import (
	"context"
	"encoding/json"
	"fmt"
	"linha/server/internal/auth"
	"linha/server/internal/config"
	"linha/server/internal/controller"
	"linha/server/internal/domain"
	"linha/server/internal/engine"
	"linha/server/internal/httpapi"
	"linha/server/internal/kube"
	"linha/server/internal/postgres"
	"linha/server/internal/storage"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	level := slog.LevelInfo
	if raw := os.Getenv("LINHA_LOG_LEVEL"); raw != "" {
		if err := level.UnmarshalText([]byte(raw)); err != nil {
			fmt.Fprintln(os.Stderr, "invalid LINHA_LOG_LEVEL")
			os.Exit(1)
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("Linha " + version)
		return
	}
	if err := run(); err != nil {
		slog.Error("Linha stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	security, err := config.ReadSecurity(os.Getenv)
	if err != nil {
		return err
	}
	dsn, err := config.DatabaseURL(os.Getenv)
	if err != nil {
		return err
	}
	fakeEnabled, err := config.Bool(os.Getenv, "LINHA_ENABLE_FAKE_ENGINE", false)
	if err != nil {
		return err
	}
	root := os.Getenv("LINHA_LOCAL_ROOT")
	repo, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer repo.Close()
	var local *storage.Local
	if root != "" {
		local, err = storage.NewLocal(root)
		if err != nil {
			return err
		}
		defer local.Close()
	}
	engines := engine.Registry{}
	var managed *auth.Kubernetes
	var reconciler *controller.Controller
	if namespace := os.Getenv("LINHA_NAMESPACE"); namespace != "" {
		client, e := kube.InCluster(namespace)
		if e != nil {
			return e
		}
		adapter := &engine.Kubernetes{RequireApplication: true, Client: client, SecurityDisabled: !security.Enabled, ServerURL: os.Getenv("LINHA_SERVER_URL"), ServiceAccount: os.Getenv("LINHA_WORKER_SERVICE_ACCOUNT"), AllowedImages: strings.Split(os.Getenv("LINHA_ALLOWED_IMAGES"), ",")}
		if adapter.ServerURL == "" || adapter.ServiceAccount == "" || os.Getenv("LINHA_ALLOWED_IMAGES") == "" {
			return fmt.Errorf("managed backends require server URL, worker service account, and allowed image repositories")
		}
		if raw := os.Getenv("LINHA_WORKER_VOLUMES"); raw != "" {
			if e = json.Unmarshal([]byte(raw), &adapter.Volumes); e != nil {
				return e
			}
		}
		if raw := os.Getenv("LINHA_WORKER_MOUNTS"); raw != "" {
			if e = json.Unmarshal([]byte(raw), &adapter.Mounts); e != nil {
				return e
			}
		}
		for key, target := range map[string]any{"LINHA_WORKER_POD_TEMPLATES": &adapter.TemplateDefaults, "LINHA_WORKER_IMAGE_PULL_SECRETS": &adapter.ImagePullSecrets, "LINHA_REGISTRY_SECRETS": &adapter.RegistrySecrets} {
			if raw := os.Getenv(key); raw != "" {
				if e = json.Unmarshal([]byte(raw), target); e != nil {
					return fmt.Errorf("invalid %s configuration", key)
				}
			}
		}
		reconciler = &controller.Controller{Pool: repo.Pool, Adapter: adapter, Resolve: adapter.Resolve, ID: domain.ID()}
		managed = &auth.Kubernetes{Kube: client, ServiceAccount: adapter.ServiceAccount, AuthorizeInstance: reconciler.AuthorizeInstance}
		engines["spark"] = adapter
	}
	var authentication auth.Authenticator
	if !security.Enabled {
		authentication = &auth.Disabled{}
	} else {
		if managed == nil {
			return fmt.Errorf("worker authentication requires LINHA_NAMESPACE; standalone local runs can set LINHA_SECURITY_ENABLED=false")
		}
		managed.TokenReview = security.WorkerAuth == "token-review"
		if !managed.TokenReview {
			managed.WorkerVerifier, err = auth.NewWorkerVerifier(ctx, managed.Kube)
			if err != nil {
				return err
			}
		}
		if security.OIDCEnabled {
			managed.Verifier, err = auth.NewOIDCVerifier(ctx, security.Issuer, security.Audience)
			if err != nil {
				return err
			}
		}
		authentication = managed
	}
	if fakeEnabled {
		engines["fake"] = engine.NewFake()
	}
	destinations := map[string]storage.Backend{}
	if raw := os.Getenv("LINHA_S3_DESTINATIONS"); raw != "" {
		var configs map[string]storage.S3Config
		if err := json.Unmarshal([]byte(raw), &configs); err != nil {
			return fmt.Errorf("invalid S3 destination configuration: %w", err)
		}
		for name, configuration := range configs {
			provider, err := storage.NewS3(ctx, name, configuration)
			if err != nil {
				return err
			}
			destinations[name] = provider
		}
	}
	if reconciler != nil {
		go reconciler.Run(ctx)
	}
	registry := storage.Registry{Local: local, Destinations: destinations}
	api := &httpapi.Server{Repo: repo, Auth: authentication, Storage: registry, Engines: engines}
	cleanupEnabled, err := config.Bool(os.Getenv, "LINHA_CLEANUP_ENABLED", false)
	if err != nil {
		return err
	}
	if raw := os.Getenv("LINHA_MAX_STAGING_BYTES"); raw != "" {
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || n < 1 {
			return fmt.Errorf("invalid LINHA_MAX_STAGING_BYTES")
		}
		api.Staging.Limit = n
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := repo.ExpireResults(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("expiry metadata update unavailable", "error", err)
				}
				if cleanupEnabled {
					if _, err := repo.CleanupOutputs(ctx, registry, 10*time.Minute, 100); err != nil && ctx.Err() == nil {
						slog.Warn("result cleanup unavailable", "error", err)
					}
				}
			}
		}
	}()
	address := os.Getenv("LINHA_LISTEN")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32768}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := repo.Recover(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("recovery unavailable", "error", err)
				}
			}
		}
	}()
	slog.Info("Linha server listening", "address", address)
	select {
	case err := <-done:
		if err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
