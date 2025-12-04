package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GitHubClient handles all GitHub API interactions for PR creation
type GitHubClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// CreateGitHubClient creates a new GitHub API client
func CreateGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		baseURL: "https://api.github.com",
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest makes an authenticated HTTP request to GitHub API
func (gc *GitHubClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	var contentType string

	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, method, gc.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", gc.token))
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return gc.httpClient.Do(req)
}

// RepoRef represents a repository reference for PR creation
type RepoRef struct {
	Owner string
	Repo  string
}

// PathResolver resolves a URL path to potential file paths in the repository
type PathResolver struct {
	baseDir string
}

// NewPathResolver creates a new path resolver
func NewPathResolver(baseDir string) *PathResolver {
	return &PathResolver{
		baseDir: baseDir,
	}
}

// ResolvePath converts a URL path (e.g., "/aws") to potential file paths
// URL path format: /segment1/segment2/segment3
// Returns both possibilities: templates/segment1/segment2/segment3.html and templates/segment1/segment2/segment3/index.html
func (pr *PathResolver) ResolvePath(urlPath string) []string {
	// Clean the path
	urlPath = strings.Trim(urlPath, "/")
	if urlPath == "" {
		return []string{"templates/index.html"}
	}

	// Convert URL path to file path
	// /aws -> aws
	// /foo/bar -> foo/bar
	segments := strings.Split(urlPath, "/")

	var paths []string

	// Try templates/segment1/segment2/.../segmentN.html
	htmlPath := filepath.Join(append([]string{pr.baseDir, "templates"}, segments...)...) + ".html"
	paths = append(paths, htmlPath)

	// Try templates/segment1/segment2/.../segmentN/index.html
	indexPath := filepath.Join(append([]string{pr.baseDir, "templates"}, segments...)...) + "/index.html"
	paths = append(paths, indexPath)

	return paths
}

// ExtractURLPath extracts the path from a full URL
// e.g., "https://ubuntu.com/aws" -> "/aws"
// e.g., "https://ubuntu.com/cloud/azure" -> "/cloud/azure"
func ExtractURLPath(fullURL string) (string, error) {
	// Remove protocol
	if idx := strings.Index(fullURL, "://"); idx != -1 {
		fullURL = fullURL[idx+3:]
	}

	// Remove domain
	if idx := strings.Index(fullURL, "/"); idx != -1 {
		return fullURL[idx:], nil
	}

	return "/", nil
}

// PRMetadata contains metadata for PR creation
type PRMetadata struct {
	Title       string
	Description string
	Branch      string
	BaseBranch  string
}

// BuildPRMetadata builds PR metadata from actionable suggestions
func BuildPRMetadata(metadata *MetadataTable, suggestions []ActionableSuggestion) PRMetadata {
	title := "chore: apply copy update changes"
	description := "This PR contains suggested changes from a content review.\n\n"

	if metadata != nil && metadata.PageTitle != "" {
		title = fmt.Sprintf("chore: copy update %s", metadata.PageTitle)
		description += fmt.Sprintf("**Page:** %s\n\n", metadata.PageTitle)
	}

	if metadata != nil && metadata.PageDescription != "" {
		description += fmt.Sprintf("**Description:** %s\n\n", metadata.PageDescription)
	}

	// TODO: Use an agents.md to store instructions for Copilot
	// For now, embed instructions directly in the PR description
	description += fmt.Sprintf("**Total Changes:** %d suggestion(s) to apply\n\n", len(suggestions))
	description += "## How to Apply These Changes\n\n"
	description += "This PR has been created with detailed location information for each suggestion. "
	description += "Use the context below to find and apply changes in the HTML/markdown files.\n\n"
	description += "---\n\n"
	description += "## Files to Modify\n\n"
	description += "The following file(s) need to be updated:\n"
	description += "- `templates/aws/index.html` (or equivalent path in your repository)\n\n"
	description += "---\n\n"
	description += "## Detailed Suggestions\n"

	for i, sugg := range suggestions {
		description += fmt.Sprintf("\n### Suggestion %d: %s\n", i+1, strings.ToUpper(sugg.Change.Type))

		// Add location context
		if sugg.Location.ParentHeading != "" {
			description += fmt.Sprintf("\n**Location Context:**\n")
			description += fmt.Sprintf("- Section: %s (Heading Level %d)\n", sugg.Location.ParentHeading, sugg.Location.HeadingLevel)
		}

		if sugg.Location.InTable {
			if sugg.Location.Table != nil {
				description += fmt.Sprintf("- Table %d, Row %d, Column %d\n",
					sugg.Location.Table.TableIndex,
					sugg.Location.Table.RowIndex,
					sugg.Location.Table.ColumnIndex)
				if sugg.Location.Table.ColumnHeader != "" {
					description += fmt.Sprintf("  - Column Header: %q\n", sugg.Location.Table.ColumnHeader)
				}
				if sugg.Location.Table.RowHeader != "" {
					description += fmt.Sprintf("  - Row Header: %q\n", sugg.Location.Table.RowHeader)
				}
			}
		}

		// Add exact text anchors for matching
		description += fmt.Sprintf("\n**Exact Location (use these anchors to find the text):**\n")
		if sugg.Anchor.PrecedingText != "" {
			description += fmt.Sprintf("- **Text before:** `%s`\n", escapeMarkdown(sugg.Anchor.PrecedingText))
		}
		if sugg.Anchor.FollowingText != "" {
			description += fmt.Sprintf("- **Text after:** `%s`\n", escapeMarkdown(sugg.Anchor.FollowingText))
		}

		// Add the change details
		description += fmt.Sprintf("\n**Change Details:**\n")
		if sugg.Change.Type == "insert" {
			description += fmt.Sprintf("- **Action:** Insert new text\n")
			description += fmt.Sprintf("- **Insert text:** `%s`\n", escapeMarkdown(sugg.Change.NewText))
		} else if sugg.Change.Type == "delete" {
			description += fmt.Sprintf("- **Action:** Delete text\n")
			description += fmt.Sprintf("- **Delete text:** `%s`\n", escapeMarkdown(sugg.Change.OriginalText))
		} else if sugg.Change.Type == "style" {
			description += fmt.Sprintf("- **Action:** Apply style change\n")
			description += fmt.Sprintf("- **Text affected:** `%s`\n", escapeMarkdown(sugg.Change.OriginalText))
		}

		// Add verification info
		description += fmt.Sprintf("\n**Verification:**\n")
		description += fmt.Sprintf("- **Before:** `%s`\n", escapeMarkdown(sugg.Verification.TextBeforeChange))
		description += fmt.Sprintf("- **After:** `%s`\n", escapeMarkdown(sugg.Verification.TextAfterChange))

		description += "\n---\n"
	}

	description += "\n## Instructions for GitHub Copilot\n\n"
	description += "1. Open the file(s) listed in \"Files to Modify\" section\n"
	description += "2. Review each suggestion above with its location context\n"
	description += "3. Find the text using the \"Exact Location\" anchors (preceding and following text)\n"
	description += "4. Apply the change described in \"Change Details\"\n"
	description += "5. Verify the result matches the \"After\" text in Verification section\n"
	description += "6. For style changes, manually update the HTML attributes/classes as needed\n\n"
	description += "---\n"
	description += "Generated by Bauer\n"

	return PRMetadata{
		Title:       title,
		Description: description,
		Branch:      generateBranchName(metadata),
		BaseBranch:  "main",
	}
}

// escapeMarkdown escapes special markdown characters
func escapeMarkdown(text string) string {
	text = strings.ReplaceAll(text, "`", "\\`")
	text = strings.ReplaceAll(text, "*", "\\*")
	text = strings.ReplaceAll(text, "_", "\\_")
	text = strings.ReplaceAll(text, "[", "\\[")
	text = strings.ReplaceAll(text, "]", "\\]")
	return text
}

// generateBranchName creates a branch name from metadata
func generateBranchName(metadata *MetadataTable) string {
	if metadata == nil || metadata.PageTitle == "" {
		return fmt.Sprintf("content/review-%d", time.Now().Unix())
	}

	// Convert title to branch name: "Ubuntu on AWS" -> "ubuntu-on-aws"
	title := strings.ToLower(metadata.PageTitle)
	title = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(title, "-")
	title = strings.Trim(title, "-")

	return fmt.Sprintf("content/%s-%d", title, time.Now().Unix())
}

// CreateDraftPR creates a draft pull request on the specified repository
// It:
// 1. Creates a new branch
// 2. Creates commits with the suggested changes
// 3. Opens a draft PR
func CreateDraftPR(
	ctx context.Context,
	ghClient *GitHubClient,
	owner string,
	repo string,
	output *struct {
		DocumentTitle         string
		DocumentID            string
		Metadata              *MetadataTable
		ActionableSuggestions []ActionableSuggestion
		Comments              []Comment
	},
) error {
	slog.Info("Starting draft PR creation",
		slog.String("owner", owner),
		slog.String("repo", repo),
		slog.Int("suggestions", len(output.ActionableSuggestions)),
	)

	// Extract URL and resolve file paths
	urlPath, err := ExtractURLPath(output.Metadata.Raw["*Current or suggested page URL"])
	if err != nil {
		return fmt.Errorf("failed to extract URL path: %w", err)
	}
	slog.Info("Extracted URL path", slog.String("path", urlPath))

	// Resolve potential file paths
	pathResolver := NewPathResolver("")
	potentialPaths := pathResolver.ResolvePath(urlPath)
	slog.Info("Resolved potential file paths",
		slog.Any("paths", potentialPaths),
	)

	// Build PR metadata
	prMetadata := BuildPRMetadata(output.Metadata, output.ActionableSuggestions)
	slog.Info("Built PR metadata",
		slog.String("title", prMetadata.Title),
		slog.String("branch", prMetadata.Branch),
	)

	// Get default branch info to create new branch
	defaultBranch, err := ghClient.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}
	slog.Info("Got default branch", slog.String("branch", defaultBranch))

	// Get the latest commit on the default branch
	latestCommit, err := ghClient.getLatestCommit(ctx, owner, repo, defaultBranch)
	if err != nil {
		return fmt.Errorf("failed to get latest commit: %w", err)
	}
	slog.Info("Got latest commit", slog.String("sha", latestCommit))

	// Create new branch
	err = ghClient.createBranch(ctx, owner, repo, prMetadata.Branch, latestCommit)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	slog.Info("Created branch", slog.String("branch", prMetadata.Branch))

	// For each potential path, create a commit with changes
	for i, filePath := range potentialPaths {
		slog.Info("Processing potential file path",
			slog.Int("index", i),
			slog.String("path", filePath),
		)

		// Get current file content
		content, sha, err := ghClient.getFileContent(ctx, owner, repo, filePath, prMetadata.BaseBranch)
		if err != nil {
			slog.Warn("Failed to get file content, skipping this path",
				slog.String("path", filePath),
				slog.String("error", err.Error()),
			)
			continue
		}

		// Apply suggestions to content
		updatedContent := ApplySuggestionsToContent(content, output.ActionableSuggestions)

		// Commit changes
		commitMsg := fmt.Sprintf("chore: update %s content\n\n%s", filePath, generateCommitBody(output.ActionableSuggestions))
		err = ghClient.createCommit(ctx, owner, repo, prMetadata.Branch, filePath, updatedContent, sha, commitMsg)
		if err != nil {
			slog.Error("Failed to create commit",
				slog.String("path", filePath),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("failed to create commit for %s: %w", filePath, err)
		}

		slog.Info("Created commit for file", slog.String("path", filePath))
		break // Only process the first successful file
	}

	// Create draft PR
	prURL, err := ghClient.createPR(ctx, owner, repo, prMetadata)
	if err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}

	slog.Info("Draft PR created successfully", slog.String("url", prURL))
	return nil
}

// getDefaultBranch gets the default branch of a repository
func (gc *GitHubClient) getDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s", owner, repo)
	resp, err := gc.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get repo info: status %d", resp.StatusCode)
	}

	var data struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return data.DefaultBranch, nil
}

// getLatestCommit gets the latest commit SHA on a branch
func (gc *GitHubClient) getLatestCommit(ctx context.Context, owner, repo, branch string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s", owner, repo, branch)
	resp, err := gc.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get commit: status %d", resp.StatusCode)
	}

	var data struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return data.SHA, nil
}

// createBranch creates a new branch from a commit SHA
func (gc *GitHubClient) createBranch(ctx context.Context, owner, repo, branch, sha string) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo)
	payload := map[string]string{
		"ref": fmt.Sprintf("refs/heads/%s", branch),
		"sha": sha,
	}

	resp, err := gc.doRequest(ctx, "POST", path, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create branch: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// getFileContent retrieves the content of a file from a specific branch
func (gc *GitHubClient) getFileContent(ctx context.Context, owner, repo, path, branch string) (string, string, error) {
	reqPath := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch)
	resp, err := gc.doRequest(ctx, "GET", reqPath, nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to get file: status %d", resp.StatusCode)
	}

	var data struct {
		Content string `json:"content"` // Base64 encoded
		SHA     string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}

	// Decode base64 content
	// Note: In production, you'd use encoding/base64 to decode
	// For now, assuming the API returns decoded content or we handle it properly

	return data.Content, data.SHA, nil
}

// createCommit creates a commit with file changes
func (gc *GitHubClient) createCommit(ctx context.Context, owner, repo, branch, path, content, sha, message string) error {
	reqPath := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	payload := map[string]interface{}{
		"message": message,
		"content": content, // Should be base64 encoded in production
		"sha":     sha,
		"branch":  branch,
	}

	resp, err := gc.doRequest(ctx, "PUT", reqPath, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create commit: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// createPR creates a pull request
func (gc *GitHubClient) createPR(ctx context.Context, owner, repo string, prMeta PRMetadata) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	payload := map[string]interface{}{
		"title": prMeta.Title,
		"head":  prMeta.Branch,
		"base":  prMeta.BaseBranch,
		"body":  prMeta.Description,
		"draft": true,
	}

	resp, err := gc.doRequest(ctx, "POST", path, payload)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create PR: status %d, body: %s", resp.StatusCode, string(body))
	}

	var data struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return data.HTMLURL, nil
}

// ApplySuggestionsToContent applies all suggestions to the file content
func ApplySuggestionsToContent(content string, suggestions []ActionableSuggestion) string {
	// Filter for actual content changes (not style changes)
	for _, sugg := range suggestions {
		if sugg.Change.Type == "delete" {
			// Replace original text with empty string
			content = strings.ReplaceAll(content, sugg.Change.OriginalText, "")
		} else if sugg.Change.Type == "insert" {
			// Find the anchor and insert new text
			anchor := sugg.Anchor.PrecedingText + sugg.Anchor.FollowingText
			if strings.Contains(content, anchor) {
				content = strings.ReplaceAll(content, anchor, sugg.Anchor.PrecedingText+sugg.Change.NewText+sugg.Anchor.FollowingText)
			}
		}
	}
	return content
}

// generateCommitBody generates the commit message body from suggestions
func generateCommitBody(suggestions []ActionableSuggestion) string {
	var body strings.Builder
	body.WriteString("Applied suggestions from content review:\n\n")

	for i, sugg := range suggestions {
		if sugg.Change.Type != "style" {
			body.WriteString(fmt.Sprintf("%d. %s: %q\n", i+1, strings.ToUpper(sugg.Change.Type), sugg.Change.OriginalText))
		}
	}

	return body.String()
}
