package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// Config holds the runtime configuration for BAU.
type Config struct {
	// DocID is the Google Doc ID to extract feedback from.
	DocID string

	// CredentialsPath is the path to the Google Cloud service account JSON key file.
	CredentialsPath string

	// DryRun indicates if the tool should skip side-effect operations (Copilot CLI, PR creation).
	DryRun bool

	// ChunkSize is the total number of chunks to create from all locations.
	// Default is 1 if not specified, or 5 if PageRefresh is true.
	ChunkSize int

	// PageRefresh indicates if the page refresh mode should be used.
	// When true, uses page-refresh-instructions.md template and defaults ChunkSize to 5.
	PageRefresh bool

	// OutputDir is the directory where generated prompt files will be saved.
	// Default is "bauer-output" if not specified.
	OutputDir string

	// Model is the Copilot model to use for sessions.
	// Default is "gpt-5-mini-high" if not specified.
	Model string

	// SummaryModel is the Copilot model to use for the summary session.
	// Default is "gpt-5-mini-high" if not specified.
	SummaryModel string
}

// Load parses command-line flags and returns a validated Config.
func Load() (*Config, error) {
	// Define flags
	// Note: We use a new FlagSet to facilitate testing if needed later,
	// but for now relying on the default flag set is sufficient for the main entry point.
	// To avoid conflicts if Load is called multiple times (e.g. in tests), we reset if needed,
	// but standard `flag` usage usually assumes run once per process.

	docID := flag.String("doc-id", "", "Google Doc ID to extract feedback from (required)")
	credentialsPath := flag.String("credentials", "", "Path to service account JSON (required)")
	dryRun := flag.Bool("dry-run", false, "Run extraction and planning only; skip Copilot and PR creation")
	chunkSize := flag.Int("chunk-size", 0, "Total number of chunks to create (default: 1, or 5 if --page-refresh is set)")
	pageRefresh := flag.Bool("page-refresh", false, "Use page refresh mode with page-refresh-instructions template (default chunk size: 5)")
	outputDir := flag.String("output-dir", "bauer-output", "Directory for generated prompt files (default: bauer-output)")
	model := flag.String("model", "gpt-5-mini-high", "Copilot model to use for sessions (default: gpt-5-mini-high)")
	summaryModel := flag.String("summary-model", "gpt-5-mini-high", "Copilot model to use for summary session (default: gpt-5-mini-high)")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n\n")
		fmt.Fprintf(os.Stderr, "\t%s --doc-id <doc-id> --credentials <path> [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n\n")

		// Manually format flags
		flags := []struct {
			name string
			typ  string
			desc string
		}{
			{"--doc-id", "<string>", "Google Doc ID to extract feedback from (required)"},
			{"--credentials", "<string>", "Path to service account JSON (required)"},
			{"--dry-run", "", "Run extraction and planning only; skip Copilot and PR creation"},
			{"--page-refresh", "", "Use page refresh mode with page-refresh-instructions template"},
			{"--chunk-size", "<int>", "Total number of chunks to create (default: 1, or 5 if --page-refresh is set)"},
			{"--output-dir", "<string>", "Directory for generated prompt files (default: bauer-output)"},
			{"--model", "<string>", "Copilot model to use for sessions (default: gpt-5-mini-high)"},
			{"--summary-model", "<string>", "Copilot model to use for summary session (default: gpt-5-mini-high)"},
		}

		for _, f := range flags {
			if f.typ != "" {
				fmt.Fprintf(os.Stderr, "\t%-25s %s\n", f.name+" "+f.typ, f.desc)
			} else {
				fmt.Fprintf(os.Stderr, "\t%-25s %s\n", f.name, f.desc)
			}
		}

		fmt.Fprintf(os.Stderr, "\nUse \"%s --help\" to display this message.\n\n", os.Args[0])
	}

	flag.Parse()

	// If no required flags are provided, show usage and exit
	if *docID == "" && *credentialsPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Determine effective chunk size based on flags
	effectiveChunkSize := *chunkSize
	if effectiveChunkSize == 0 {
		// User didn't specify chunk size, apply defaults
		if *pageRefresh {
			effectiveChunkSize = 5
		} else {
			effectiveChunkSize = 1
		}
	}

	cfg := &Config{
		DocID:           *docID,
		CredentialsPath: *credentialsPath,
		DryRun:          *dryRun,
		ChunkSize:       effectiveChunkSize,
		PageRefresh:     *pageRefresh,
		OutputDir:       *outputDir,
		Model:           *model,
		SummaryModel:    *summaryModel,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.DocID == "" {
		return errors.New("missing required flag: --doc-id")
	}

	if c.CredentialsPath == "" {
		return errors.New("missing required flag: --credentials")
	}

	if c.ChunkSize <= 0 {
		return errors.New("chunk-size must be greater than 0")
	}

	// Verify credentials file exists
	info, err := os.Stat(c.CredentialsPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("credentials file not found: %s", c.CredentialsPath)
	}
	if err != nil {
		return fmt.Errorf("error checking credentials file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("credentials path is a directory, expected a file: %s", c.CredentialsPath)
	}

	return nil
}
