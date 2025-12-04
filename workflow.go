package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Command-line flags for PR creation
var (
	createPRFlag  = flag.Bool("create-pr", false, "Create a draft PR instead of just outputting JSON")
	repoOwnerFlag = flag.String("repo-owner", "canonical", "GitHub repository owner")
	repoNameFlag  = flag.String("repo-name", "ubuntu.com", "GitHub repository name")
)

// loadEnv loads environment variables from .env files.
// It loads in this order (later files override earlier ones):
// 1. .env (default values, should be committed)
// 2. .env.local (local overrides, should NOT be committed)
func loadEnv() error {
	envFiles := []string{".env", ".env.local"}

	for _, filename := range envFiles {
		slog.Info("Loading environment file", slog.String("file", filename))
		if err := loadEnvFile(filename); err != nil {
			return err
		}
	}

	return nil
}

// loadEnvFile reads a single .env file and loads environment variables
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("Environment file not found (optional)", slog.String("file", filename))
			return nil // .env file is optional
		}
		return fmt.Errorf("failed to open %s: %w", filename, err)
	}
	defer file.Close()

	var loadedCount int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
		}

		os.Setenv(key, value)
		if key == "GITHUB_TOKEN" {
			slog.Debug("Loaded GITHUB_TOKEN from environment file", slog.String("file", filename))
		}
		loadedCount++
	}

	if loadedCount > 0 {
		slog.Info("Environment file loaded", slog.String("file", filename), slog.Int("variables", loadedCount))
	}

	return scanner.Err()
}

// getGitHubToken retrieves the GitHub token from environment variables
func getGitHubToken() string {
	return os.Getenv("GITHUB_TOKEN")
}

// CreatePRFromJSON reads the output.json and creates a draft PR
// The GitHub token is read from the GITHUB_TOKEN environment variable (set in .env file)
func CreatePRFromJSON(ctx context.Context, outputFile string) error {
	githubToken := getGitHubToken()
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable not set (set it in .env file)")
	}
	// Read output.json
	outputJSON, err := os.ReadFile(outputFile)
	if err != nil {
		return fmt.Errorf("failed to read output file: %w", err)
	}

	// Parse JSON
	var output struct {
		DocumentTitle         string
		DocumentID            string
		Metadata              *MetadataTable
		ActionableSuggestions []ActionableSuggestion
		Comments              []Comment
	}

	if err := json.Unmarshal(outputJSON, &output); err != nil {
		return fmt.Errorf("failed to parse output JSON: %w", err)
	}

	slog.Info("Loaded output.json",
		slog.Int("suggestions", len(output.ActionableSuggestions)),
	)

	// Create GitHub client
	ghClient := CreateGitHubClient(githubToken)

	// Create draft PR
	err = CreateDraftPR(ctx, ghClient, *repoOwnerFlag, *repoNameFlag, &output)
	if err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}

	return nil
}

// ProcessAndCreatePR is the main entry point for PR creation from Google Docs.
//
// Usage:
//
//	# Create output.json only:
//	go run main.go workflow.go github.go
//
//	# Create output.json and draft PR:
//	go run main.go workflow.go github.go -create-pr
//
// Configuration:
//
//	GitHub token must be set in .env file:
//	echo "GITHUB_TOKEN=ghp_xxxxxxxxxxxx" > .env
//
//	Optionally customize repository (defaults to canonical/ubuntu.com):
//	echo "GITHUB_REPO_OWNER=myorg" >> .env
//	echo "GITHUB_REPO_NAME=myrepo" >> .env
//
// The function:
// 1. Loads environment variables from .env file
// 2. Extracts suggestions from a Google Doc
// 3. Generates output.json
// 4. Optionally creates a draft PR on GitHub
func ProcessAndCreatePR(
	ctx context.Context,
	googleDocURL string,
	shouldCreatePR bool,
) error {
	// Load .env files (.env.local overrides .env)
	if err := loadEnv(); err != nil {
		slog.Warn("Failed to load .env files", slog.String("error", err.Error()))
	}

	slog.Info("Starting content review workflow",
		slog.String("google_doc", googleDocURL),
		slog.Bool("create_pr", shouldCreatePR),
	)

	// Extract document ID
	docID, err := extractDocumentID(googleDocURL)
	if err != nil {
		return fmt.Errorf("failed to extract document ID: %w", err)
	}

	// Build Google Docs service
	email := delegationEmail
	docsService, err := buildDocsService(ctx, &email, useDelegation)
	if err != nil {
		return fmt.Errorf("failed to build Docs service: %w", err)
	}

	// Build Google Drive service
	driveService, err := buildDriveService(ctx, &email, useDelegation)
	if err != nil {
		return fmt.Errorf("failed to build Drive service: %w", err)
	}

	// Fetch document
	doc, err := fetchDocumentContent(ctx, docsService, docID)
	if err != nil {
		return fmt.Errorf("failed to fetch document: %w", err)
	}

	slog.Info("Document fetched",
		slog.String("title", doc.Title),
	)

	// Extract suggestions
	suggestions := extractSuggestions(doc)
	slog.Info("Suggestions extracted",
		slog.Int("count", len(suggestions)),
	)

	// Fetch comments
	comments, err := fetchComments(ctx, driveService, docID)
	if err != nil {
		slog.Warn("Failed to fetch comments", slog.String("error", err.Error()))
		comments = []Comment{}
	}

	// Extract metadata
	metadata := extractMetadataTable(doc)

	// Build document structure
	docStructure := buildDocumentStructure(doc)

	// Build actionable suggestions
	actionableSuggestions := buildActionableSuggestions(suggestions, docStructure, metadata)

	// Create output structure
	output := struct {
		DocumentTitle         string
		DocumentID            string
		Metadata              *MetadataTable
		ActionableSuggestions []ActionableSuggestion
		Comments              []Comment
	}{
		DocumentTitle:         doc.Title,
		DocumentID:            doc.DocumentId,
		Metadata:              metadata,
		ActionableSuggestions: actionableSuggestions,
		Comments:              comments,
	}

	// Write to file
	outputJSON, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	outputFile := "output.json"
	if err := os.WriteFile(outputFile, outputJSON, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	slog.Info("Output written to file", slog.String("file", outputFile))

	// Create PR if requested
	if shouldCreatePR {
		githubToken := getGitHubToken()
		if githubToken == "" {
			return fmt.Errorf("GITHUB_TOKEN environment variable not set (set it in .env or .env.local)")
		}

		slog.Info("GitHub token loaded", slog.String("token_preview", githubToken[:20]+"..."))

		err := CreateDraftPR(ctx, CreateGitHubClient(githubToken), *repoOwnerFlag, *repoNameFlag, &output)
		if err != nil {
			return fmt.Errorf("failed to create PR: %w", err)
		}
	}

	return nil
}
