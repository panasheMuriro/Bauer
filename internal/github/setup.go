package github

import (
	"fmt"
	"log/slog"
	"time"
)

// GitHubSetupInput represents input for GitHub setup phase
type GitHubSetupInput struct {
	GitHubRepo    string
	GitHubToken   string
	BranchPrefix  string
	LocalRepoPath string
}

// GitHubSetupOutput represents the result of GitHub setup phase
type GitHubSetupOutput struct {
	Repo          *Repository
	LocalPath     string
	BranchName    string
	DefaultBranch string
	CurrentBranch string
}

// SetupGitHubPhase performs Phase 1: GitHub Setup
// This function is reusable by both CLI (runGithub) and API (ExecuteWorkflow)
func SetupGitHubPhase(input GitHubSetupInput) (*GitHubSetupOutput, error) {
	logger := slog.Default()

	// Validate GH CLI installation and authentication
	if !IsGhCLIInstalled() {
		return nil, fmt.Errorf("gh CLI not installed. Please install it from https://cli.github.com")
	}
	logger.Info("github setup: gh CLI detected")

	if err := ValidateGitHubAuth(); err != nil {
		return nil, fmt.Errorf("GitHub authentication failed: %w. Run 'gh auth login' to authenticate", err)
	}
	logger.Info("github setup: GitHub authenticated")

	// Parse repository
	repo, err := ParseGitHubRepo(input.GitHubRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub repo: %w", err)
	}
	logger.Info("github setup: parsed repo", "owner", repo.Owner, "repo", repo.Name)

	// Setup GitHub authentication
	if err := SetupGitHubAuth(input.GitHubToken); err != nil {
		return nil, fmt.Errorf("failed to setup GitHub auth: %w", err)
	}
	logger.Info("github setup: authentication configured")

	// Clone/update repository
	if err := CloneOrUpdateRepo(repo, input.LocalRepoPath); err != nil {
		return nil, fmt.Errorf("failed to clone/update repo: %w", err)
	}
	logger.Info("github setup: repository ready", "local_path", input.LocalRepoPath)

	// Get default branch
	defaultBranch, err := GetDefaultBranch(input.LocalRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get default branch: %w", err)
	}
	logger.Info("github setup: default branch detected", "branch", defaultBranch)

	// Create feature branch
	branchName := fmt.Sprintf("%s/doc-suggestions-%d", input.BranchPrefix, time.Now().Unix())
	if err := CreateFeatureBranch(input.LocalRepoPath, branchName); err != nil {
		return nil, fmt.Errorf("failed to create feature branch: %w", err)
	}
	logger.Info("github setup: feature branch created", "branch", branchName)

	// Get current branch
	currentBranch, err := GetCurrentBranch(input.LocalRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	output := &GitHubSetupOutput{
		Repo:          repo,
		LocalPath:     input.LocalRepoPath,
		BranchName:    branchName,
		DefaultBranch: defaultBranch,
		CurrentBranch: currentBranch,
	}

	logger.Info("github setup: phase complete",
		"owner", repo.Owner,
		"repo", repo.Name,
		"branch", branchName,
		"local_path", input.LocalRepoPath,
	)

	return output, nil
}

// GitHubFinalizationInput represents input for GitHub finalization phase
type GitHubFinalizationInput struct {
	LocalRepoPath string
	BranchName    string
	DefaultBranch string
	Owner         string
	Repo          string
	CommitMessage string
	DryRun        bool
	PRTitle       string
	PRBody        string
	Labels        []string
}

// GitHubFinalizationOutput represents the result of GitHub finalization phase
type GitHubFinalizationOutput struct {
	CommitMessage string
	BranchPushed  bool
	PullRequest   struct {
		URL    string
		Number int
		Title  string
	}
	Errors   []string
	Warnings []string
}

// FinalizeGitHubPhase performs Phase 3: GitHub Finalization
// This function is reusable by both CLI and API
func FinalizeGitHubPhase(input GitHubFinalizationInput) (*GitHubFinalizationOutput, error) {
	logger := slog.Default()
	output := &GitHubFinalizationOutput{
		Errors:   []string{},
		Warnings: []string{},
	}

	// 3.1 Check for changes
	status, err := GetStatus(input.LocalRepoPath)
	if err != nil {
		output.Warnings = append(output.Warnings, fmt.Sprintf("failed to check git status: %v", err))
		logger.Warn("github finalize: failed to check status", "error", err)
	}

	// 3.2 Commit changes (if there are any)
	if status != "" {
		if err := CommitChanges(input.LocalRepoPath, input.CommitMessage); err != nil {
			output.Errors = append(output.Errors, fmt.Sprintf("failed to commit changes: %v", err))
			logger.Warn("github finalize: failed to commit", "error", err)
		} else {
			output.CommitMessage = input.CommitMessage
			logger.Info("github finalize: changes committed", "message", input.CommitMessage)
		}
	} else {
		logger.Info("github finalize: no changes to commit")
	}

	// 3.3 Push branch
	if err := PushBranch(input.LocalRepoPath, input.BranchName); err != nil {
		output.Errors = append(output.Errors, fmt.Sprintf("failed to push branch: %v", err))
		logger.Warn("github finalize: failed to push", "error", err)
		return output, nil
	}
	output.BranchPushed = true
	logger.Info("github finalize: branch pushed", "branch", input.BranchName)

	// 3.4 Create PR (only if not dry run)
	if !input.DryRun && output.BranchPushed {
		prOpts := CreatePROptions{
			Title:      input.PRTitle,
			Body:       input.PRBody,
			HeadBranch: input.BranchName,
			BaseBranch: input.DefaultBranch,
			Labels:     input.Labels,
		}

		prURL, err := CreatePR(input.Owner, input.Repo, prOpts)
		if err != nil {
			output.Errors = append(output.Errors, fmt.Sprintf("failed to create PR: %v", err))
			logger.Warn("github finalize: failed to create PR", "error", err)
		} else {
			output.PullRequest.URL = prURL
			output.PullRequest.Title = prOpts.Title
			logger.Info("github finalize: PR created", "url", prURL)
		}
	}

	logger.Info("github finalize: phase complete",
		"branch_pushed", output.BranchPushed,
		"errors", len(output.Errors),
	)

	return output, nil
}
