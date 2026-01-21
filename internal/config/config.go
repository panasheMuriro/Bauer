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

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nExample:")
		fmt.Fprintf(os.Stderr, "  %s --doc-id \"1b9...\" --credentials ./creds.json --dry-run\n", os.Args[0])
	}

	flag.Parse()

	cfg := &Config{
		DocID:           *docID,
		CredentialsPath: *credentialsPath,
		DryRun:          *dryRun,
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
