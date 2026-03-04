package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"pqq/be/internal/config"
	httpHandlers "pqq/be/internal/http/handlers"
	"pqq/be/internal/http/router"
	"pqq/be/internal/media"
	"pqq/be/internal/postgres"
	"pqq/be/internal/storage"
	"pqq/be/internal/sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("create postgres pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	if err := postgres.EnsureSchema(ctx, pool); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	var storageService *storage.MinIOService
	if cfg.Storage.Enabled {
		storageService, err = storage.NewMinIOService(cfg.Storage)
		if err != nil {
			log.Fatalf("create minio service: %v", err)
		}

		if err := storageService.EnsureBucket(ctx); err != nil {
			log.Fatalf("ensure minio bucket: %v", err)
		}

		log.Printf("minio storage ready: endpoint=%s bucket=%s", cfg.Storage.Endpoint, storageService.Bucket())
	}

	store := postgres.NewSyncStore(pool)
	hub := sync.NewHub()
	service := sync.NewService(store, hub)
	var studentMediaHandler *httpHandlers.StudentMediaHandler

	syncHandler := httpHandlers.NewSyncHandler(service)
	wsHandler := httpHandlers.NewRealtimeHandler(hub)
	if storageService != nil {
		mediaStore := media.NewStore(pool)
		mediaService := media.NewService(mediaStore, storageService, cfg.Storage.PresignExpiryMinutes)
		studentMediaHandler = httpHandlers.NewStudentMediaHandler(mediaService)
	}

	engine := router.New(cfg, syncHandler, wsHandler, studentMediaHandler)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("server listening on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
