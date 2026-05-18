package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/homesignal-io/homesignal-home-assistant-app/telemetry-ingest/internal/app"
	"github.com/homesignal-io/homesignal-home-assistant-app/telemetry-ingest/internal/pipeline"
	"github.com/homesignal-io/homesignal-home-assistant-app/telemetry-ingest/internal/storage/postgres"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var writer pipeline.PersistenceWriter = &pipeline.MemoryWriter{}
	var failures pipeline.FailureSink = &pipeline.MemoryFailureSink{}
	if databaseURL := firstNonEmpty(os.Getenv("HOMESIGNAL_DATABASE_URL"), os.Getenv("DATABASE_URL")); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		pool, err := postgres.Open(ctx, databaseURL)
		cancel()
		if err != nil {
			logger.Error("postgres telemetry persistence unavailable", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
		postgresWriter := postgres.Writer{Pool: pool}
		writer = postgresWriter
		failures = postgresWriter
		logger.Info("postgres telemetry persistence enabled")
	} else {
		logger.Info("memory telemetry persistence enabled")
	}
	handler := app.NewHandler(app.Server{
		Pipeline: pipeline.NewRuntimePipeline(writer, failures),
		Version:  version,
		Commit:   commit,
	})

	logger.Info("telemetry ingest ready", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Error("telemetry ingest stopped", "error", err)
		os.Exit(1)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
