package main

import (
	"bauer/cmd/app/core/middleware"
	"bauer/cmd/app/types"
	v1 "bauer/cmd/app/v1"
	"bauer/internal/orchestrator"
	"bauer/internal/workflow"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	slog.Info("startup", "status", "initializing API")
	defer slog.Info("shutdown complete")

	orchestrator := orchestrator.NewOrchestrator()
	cfg, err := types.LoadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err.Error())
		return err
	}

	rc := types.RouteConfig{
		APIConfig:    *cfg,
		Orchestrator: orchestrator,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/job", v1.JobPost(rc))
	mux.HandleFunc("/api/v1/health", v1.GetHealth)
	mux.HandleFunc("/api/v1/workflow", workflow.ExecuteWorkflowHandler(orchestrator))
	slog.Info("starting server", "address", ":8090")
	err = http.ListenAndServe(":8090", middleware.RequestTrace(mux))

	if err != nil {
		slog.Error("server error", "error", err.Error())
		slog.Info("shutdown complete with errors")
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
