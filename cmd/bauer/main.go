package main

import (
	"bauer/internal/github"
	"bauer/internal/orchestrator"
	"bauer/internal/workflow"
	"context"
	"fmt"
	"os"
)

func main() {
	// Create workflow input from CLI flags/config
	ghToken, err := github.GetGitHubToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Could not get GitHub token: %v\n", err)
		ghToken = ""
	}

	workflowInput := workflow.WorkflowInput{
		GitHubRepo:    "canonical/ubuntu.com",
		GitHubToken:   ghToken,
		BranchPrefix:  "bauer",
		DocID:         "16AMRADqnW2Ssi1zYL68LqBB3__VIkL9Jegv-ZubWTGg",
		Credentials:   "bau-test-creds.json",
		LocalRepoPath: "/tmp/ubuntu.com",
		DryRun:        false,
		OutputDir:     "bauer-output",
	}

	orch := orchestrator.NewOrchestrator()

	// Execute the complete workflow
	result, err := workflow.ExecuteWorkflow(context.Background(), workflowInput, orch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Branch: %s\n", result.RepositoryInfo.BranchName)
	fmt.Printf("PR: %s\n", result.FinalizationInfo.PullRequest.URL)
}
