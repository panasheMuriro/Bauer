package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"bauer/internal/config"
	"bauer/internal/gdocs"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Setup Logging
	// For now, mirroring POC behavior: logging to log.json in the current directory
	// TODO disable with a flag or env var
	logFile, err := os.OpenFile("log.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting BAU CLI",
		slog.String("doc_id", cfg.DocID),
		slog.Bool("dry_run", cfg.DryRun),
	)

	ctx := context.Background()

	// 3. Initialize GDocs Client
	client, err := gdocs.NewClient(ctx, cfg.CredentialsPath)
	if err != nil {
		slog.Error("Failed to initialize Google Docs client", slog.String("error", err.Error()))
		fmt.Fprintf(os.Stderr, "Failed to initialize Google Docs client: %v\n", err)
		os.Exit(1)
	}

	// 4. Process Document
	result, err := client.ProcessDocument(ctx, cfg.DocID)
	if err != nil {
		// Error logging is handled in ProcessDocument
		os.Exit(1)
	}

	// 5. Generate Output
	outputJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal output", slog.String("error", err.Error()))
		fmt.Fprintf(os.Stderr, "Failed to generate output JSON: %v\n", err)
		os.Exit(1)
	}

	// Write to file
	outputFile := "doc-suggestions.json"
	err = os.WriteFile(outputFile, outputJSON, 0644)
	if err != nil {
		slog.Error("Failed to write output file", slog.String("error", err.Error()))
		fmt.Fprintf(os.Stderr, "Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	slog.Info("Extraction complete", slog.String("output_file", outputFile))
	fmt.Printf("Extraction complete. Output written to %s\n", outputFile)

	if cfg.DryRun {
		fmt.Println("Dry run enabled: Skipping Copilot execution and PR creation.")
	}
}
