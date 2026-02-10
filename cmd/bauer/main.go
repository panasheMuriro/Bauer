package main

import (
	"bauer/internal/config"
	"bauer/internal/orchestrator"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func run() error {

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Bauer - A tool to automate BAU tasks")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading configuration\n%w", err)
	}

	// 2. Setup Logging
	// For now, mirroring POC behavior: logging to log.json in the current directory
	// TODO disable with a flag or env var
	logFile, err := os.OpenFile("bauer-log.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close log file: %v\n", err)
		}
	}()

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting BAU CLI",
		slog.String("doc_id", cfg.DocID),
		slog.Bool("dry_run", cfg.DryRun),
	)

	ctx := context.Background()

	// 3. Create and execute orchestrator
	orch := orchestrator.NewOrchestrator()
	result, err := orch.Execute(ctx, cfg)
	if err != nil {
		slog.Error("Orchestration failed", slog.String("error", err.Error()))
		// Check if the error is credentials-related and provide more context
		errMsg := err.Error()
		if strings.Contains(errMsg, "credentials") || strings.Contains(errMsg, "private_key") || strings.Contains(errMsg, "client_email") {
			fmt.Fprintf(os.Stderr, "\n⚠️  CREDENTIALS ERROR:\n")
			fmt.Fprintf(os.Stderr, "  %v\n\n", err)
			fmt.Fprintf(os.Stderr, "Please verify:\n")
			fmt.Fprintf(os.Stderr, "  1. Credentials file exists at: %s\n", cfg.CredentialsPath)
			fmt.Fprintf(os.Stderr, "  2. Credentials file is valid JSON\n")
			fmt.Fprintf(os.Stderr, "  3. Credentials file contains required fields:\n")
			fmt.Fprintf(os.Stderr, "     - type\n")
			fmt.Fprintf(os.Stderr, "     - project_id\n")
			fmt.Fprintf(os.Stderr, "     - private_key\n")
			fmt.Fprintf(os.Stderr, "     - client_email\n")
			fmt.Fprintf(os.Stderr, "     - token_uri\n\n")
		}
		return err
	}

	// 4. Print results
	outputFile := "bauer-doc-suggestions.json"
	fmt.Println("[1/4] Extracting from Google Doc...")
	fmt.Printf("  ✓ Extraction completed in %s\n", result.ExtractionDuration.Round(time.Millisecond))
	fmt.Println()

	fmt.Println("[2/4] Generating technical plan...")
	fmt.Printf("  ✓ Saved: %s\n", outputFile)
	fmt.Printf("  ✓ Generated %d chunk file(s) in '%s/'\n", len(result.Chunks), cfg.OutputDir)
	fmt.Printf("  ✓ Planning completed in %s\n", result.PlanDuration.Round(time.Millisecond))
	for _, chunk := range result.Chunks {
		slog.Info("Generated chunk",
			slog.Int("chunk_number", chunk.ChunkNumber),
			slog.String("filename", chunk.Filename),
			slog.Int("location_count", chunk.LocationCount),
		)
	}
	fmt.Println()

	if result.DryRun {
		fmt.Println("[3/4] Copilot execution (skipped - dry run)")
		fmt.Println()
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println("DRY RUN COMPLETE")
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("  Summary:\n")
		fmt.Printf("    • Extracted: %d suggestions\n", len(result.ExtractionResult.ActionableSuggestions))
		fmt.Printf("    • Grouped into: %d location(s)\n", len(result.ExtractionResult.GroupedSuggestions))
		fmt.Printf("    • Generated: %d chunk file(s) in '%s/'\n", len(result.Chunks), cfg.OutputDir)
		fmt.Printf("\n  Timing:\n")
		fmt.Printf("    • Extraction: %s\n", result.ExtractionDuration.Round(time.Millisecond))
		fmt.Printf("    • Planning: %s\n", result.PlanDuration.Round(time.Millisecond))
		fmt.Printf("    • Total: %s\n", result.TotalDuration.Round(time.Millisecond))
		fmt.Printf("\n  Next steps:\n")
		fmt.Printf("    1. Review generated chunks in '%s/'\n", cfg.OutputDir)
		fmt.Printf("    2. Run without --dry-run to execute changes via Copilot\n")
		fmt.Printf("    3. Or manually pass chunk files to: gh copilot\n")
		fmt.Println(strings.Repeat("=", 80))
		return nil
	}

	// 5. Copilot execution results
	fmt.Println("[3/4] Executing changes via Copilot...")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  ✓ All %d chunk(s) processed successfully\n", len(result.CopilotOutputs))
	fmt.Printf("  ✓ Total Copilot execution time: %s\n", result.CopilotDuration.Round(time.Millisecond))
	fmt.Println()

	// 6. Summary results if multiple chunks
	if len(result.Chunks) > 1 {
		fmt.Println("[4/5] Generating summary...")
		fmt.Printf("  ✓ Summary completed in %s\n", result.SummaryDuration.Round(time.Millisecond))
		fmt.Println()
	}

	// 7. Final summary
	stepLabel := "[4/4]"
	if len(result.Chunks) > 1 {
		stepLabel = "[5/5]"
	}

	fmt.Printf("%s Complete!\n", stepLabel)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("SUCCESS: Feedback applied!")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  Summary:\n")
	fmt.Printf("    • Extracted: %d suggestions\n", len(result.ExtractionResult.ActionableSuggestions))
	fmt.Printf("    • Processed: %d chunk(s)\n", len(result.Chunks))
	fmt.Printf("\n  Timing:\n")
	fmt.Printf("    • Extraction: %s\n", result.ExtractionDuration.Round(time.Millisecond))
	fmt.Printf("    • Planning: %s\n", result.PlanDuration.Round(time.Millisecond))
	fmt.Printf("    • Copilot execution: %s\n", result.CopilotDuration.Round(time.Millisecond))
	fmt.Printf("    • Total: %s\n", result.TotalDuration.Round(time.Millisecond))
	fmt.Printf("\n  Next steps:\n")
	fmt.Printf("    • Review the changes made by Copilot\n")
	fmt.Printf("    • Create a PR with: gh pr create\n")
	fmt.Println(strings.Repeat("=", 80))

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
