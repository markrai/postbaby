package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/entitlement"
	"postbaby-backend/internal/httpapi"
	appserver "postbaby-backend/internal/server"
	"postbaby-backend/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := cfg.EnsureDBDir(); err != nil {
		log.Fatalf("prepare database directory: %v", err)
	}

	staticDir, err := cfg.ResolveStaticDir()
	if err != nil {
		log.Fatalf("resolve static directory: %v", err)
	}

	sqliteStore, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() {
		if err := sqliteStore.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	authManager := auth.NewManager(sqliteStore, auth.Options{
		CookieSecure: cfg.CookieSecure,
		SessionTTL:   cfg.SessionTTL,
	})
	entitlementManager := entitlement.NewManager(sqliteStore)
	apiHandler := httpapi.NewHandler(sqliteStore, authManager, entitlementManager, cfg.DeploymentMode)
	handler := appserver.NewHandler(apiHandler, authManager, entitlementManager, staticDir, cfg.DeploymentMode)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("postbaby backend listening on %s in %s mode and serving static files from %s", cfg.Addr, cfg.DeploymentMode, staticDir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}
