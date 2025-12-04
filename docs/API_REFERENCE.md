# Bauer PR Creation API Reference

This document describes the main functions for creating draft PRs from Google Docs suggestions.

## High-Level Workflow Functions

### `ProcessAndCreatePR()`

**Purpose**: End-to-end workflow from Google Doc to draft PR

**Signature**:
```go
func ProcessAndCreatePR(
    ctx context.Context,
    googleDocURL string,
    githubToken string,
    shouldCreatePR bool,
) error
```

**Parameters**:
- `ctx`: Context for API calls
- `googleDocURL`: Full URL to the Google Doc (e.g., `https://docs.google.com/document/d/{docID}/edit`)
- `githubToken`: GitHub Personal Access Token with `repo` scope
- `shouldCreatePR`: Whether to create a PR (if false, only generates output.json)

**Returns**: Error if any step fails

**Example**:
```go
ctx := context.Background()
err := ProcessAndCreatePR(
    ctx,
    "https://docs.google.com/document/d/1vMw8X7Y8yoiubnUEwt6o9f-s0YK5F6J4I1hQ6tevVqg/edit",
    "ghp_xxxxxxxxxxxxx",
    true,
)
if err != nil {
    log.Fatal(err)
}
```

**Steps**:
1. Extracts document ID from URL
2. Authenticates with Google Docs API
3. Fetches document with suggestions
4. Extracts suggestions, metadata, and comments
5. Builds actionable suggestion objects
6. Writes output.json
7. (If shouldCreatePR) Creates draft PR on github.com/canonical/ubuntu.com

---

### `CreatePRFromJSON()`

**Purpose**: Create a draft PR from an existing output.json file

**Signature**:
```go
func CreatePRFromJSON(
    ctx context.Context,
    outputFile string,
    githubToken string,
) error
```

**Parameters**:
- `ctx`: Context for API calls
- `outputFile`: Path to output.json file
- `githubToken`: GitHub Personal Access Token

**Returns**: Error if PR creation fails

**Example**:
```go
ctx := context.Background()
err := CreatePRFromJSON(
    ctx,
    "output.json",
    "ghp_xxxxxxxxxxxxx",
)
if err != nil {
    log.Fatal(err)
}
```

---

## GitHub API Functions

### `CreateDraftPR()`

**Purpose**: Creates a draft PR with suggested changes

**Signature**:
```go
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
) error
```

**Flow**:
1. Extracts URL path from metadata
2. Resolves file paths (tries both .html and /index.html)
3. Creates new branch
4. Fetches current file content
5. Applies suggestions
6. Creates commit
7. Opens draft PR
8. Logs PR URL

---

## Path Resolution Functions

### `ExtractURLPath()`

**Purpose**: Extracts the path component from a full URL

**Signature**:
```go
func ExtractURLPath(fullURL string) (string, error)
```

**Examples**:
- Input: `https://ubuntu.com/aws`
- Output: `/aws`

- Input: `https://ubuntu.com/cloud/azure`
- Output: `/cloud/azure`

---

### `PathResolver.ResolvePath()`

**Purpose**: Converts a URL path to potential file paths in the repository

**Signature**:
```go
func (pr *PathResolver) ResolvePath(urlPath string) []string
```

**Returns**: Slice of potential file paths to try (in order)

**Examples**:
```go
resolver := NewPathResolver("")

// /aws → [templates/aws.html, templates/aws/index.html]
paths := resolver.ResolvePath("/aws")

// /cloud/azure → [templates/cloud/azure.html, templates/cloud/azure/index.html]
paths := resolver.ResolvePath("/cloud/azure")

// / → [templates/index.html]
paths := resolver.ResolvePath("/")
```

---

## Metadata and Suggestion Builders

### `BuildPRMetadata()`

**Purpose**: Creates PR metadata (title, description, branch name) from suggestions

**Signature**:
```go
func BuildPRMetadata(
    metadata *MetadataTable,
    suggestions []ActionableSuggestion,
) PRMetadata
```

**Returns**: PR title, description, branch name, and base branch

**Example Output**:
```json
{
  "title": "chore: update Ubuntu on AWS",
  "description": "This PR contains suggested changes from a content review.\n\n**Page:** Ubuntu on AWS\n\n**Changes:** 3 suggestion(s) to apply\n\n...",
  "branch": "content/ubuntu-on-aws-1733328872",
  "base_branch": "main"
}
```

---

### `ApplySuggestionsToContent()`

**Purpose**: Applies all suggestions to file content

**Signature**:
```go
func ApplySuggestionsToContent(
    content string,
    suggestions []ActionableSuggestion,
) string
```

**Handles**:
- Deletions: Removes original text
- Insertions: Adds new text at anchor points
- Style changes: Skipped (don't affect text content)

---

## GitHub Client Functions

### `CreateGitHubClient()`

**Purpose**: Creates an authenticated GitHub API client

**Signature**:
```go
func CreateGitHubClient(token string) *GitHubClient
```

**Example**:
```go
ghClient := CreateGitHubClient("ghp_xxxxxxxxxxxxx")
```

---

### `GitHubClient.createBranch()`

**Purpose**: Creates a new branch in a repository

**Signature**:
```go
func (gc *GitHubClient) createBranch(
    ctx context.Context,
    owner string,
    repo string,
    branch string,
    sha string,
) error
```

**Parameters**:
- `owner`: Repository owner (e.g., "canonical")
- `repo`: Repository name (e.g., "ubuntu.com")
- `branch`: New branch name (e.g., "content/ubuntu-on-aws-1733328872")
- `sha`: Commit SHA to branch from (from `getLatestCommit()`)

---

### `GitHubClient.getFileContent()`

**Purpose**: Retrieves a file's content and SHA from a branch

**Signature**:
```go
func (gc *GitHubClient) getFileContent(
    ctx context.Context,
    owner string,
    repo string,
    path string,
    branch string,
) (string, string, error)
```

**Returns**: File content, file SHA (needed for commits), error

---

### `GitHubClient.createCommit()`

**Purpose**: Creates a commit with file changes

**Signature**:
```go
func (gc *GitHubClient) createCommit(
    ctx context.Context,
    owner string,
    repo string,
    branch string,
    path string,
    content string,
    sha string,
    message string,
) error
```

**Parameters**:
- `branch`: Branch to commit to
- `path`: File path to modify
- `content`: New file content
- `sha`: Current file SHA (from `getFileContent()`)
- `message`: Commit message

---

### `GitHubClient.createPR()`

**Purpose**: Creates a pull request

**Signature**:
```go
func (gc *GitHubClient) createPR(
    ctx context.Context,
    owner string,
    repo string,
    prMeta PRMetadata,
) (string, error)
```

**Returns**: PR URL (e.g., `https://github.com/canonical/ubuntu.com/pull/1234`), error

---

## Data Structures

### `ActionableSuggestion`

```go
type ActionableSuggestion struct {
    ID           string                    // Unique ID from Google Docs
    Anchor       SuggestionAnchor          // Before/after text for matching
    Change       SuggestionChange          // What to change (type, original, new)
    Verification SuggestionVerification    // Before/after validation
    Location     SuggestionLocation        // Metadata (heading, table, etc.)
    Position     struct {
        StartIndex int64
        EndIndex   int64
    }
}

type SuggestionChange struct {
    Type         string // "insert", "delete", or "style"
    OriginalText string
    NewText      string
}

type SuggestionAnchor struct {
    PrecedingText string // Text before the change
    FollowingText string // Text after the change
}
```

---

## Error Handling

All functions return `error` for failure cases. Common errors:

- **Authentication errors**: Invalid token or credentials
- **Network errors**: GitHub/Google API unavailable
- **File not found**: Path resolution failed
- **Branch already exists**: Branch name collision
- **Rate limiting**: API rate limit exceeded

Example error handling:

```go
err := CreateDraftPR(ctx, ghClient, "canonical", "ubuntu.com", &output)
if err != nil {
    slog.Error("Failed to create PR", slog.String("error", err.Error()))
    // Handle error appropriately
}
```

---

## Usage Patterns

### Pattern 1: Full Workflow (Google Doc → PR)

```go
ctx := context.Background()
err := ProcessAndCreatePR(
    ctx,
    "https://docs.google.com/document/d/1vMw8X7Y8yoiubnUEwt6o9f-s0YK5F6J4I1hQ6tevVqg/edit",
    os.Getenv("GITHUB_TOKEN"),
    true, // Create PR
)
```

### Pattern 2: Two-Step (Generate JSON, then PR)

```go
// Step 1: Generate output.json
err := ProcessAndCreatePR(ctx, docURL, "", false)

// Step 2: Create PR from JSON
err := CreatePRFromJSON(ctx, "output.json", githubToken)
```

### Pattern 3: Custom Repository

```go
ghClient := CreateGitHubClient(githubToken)
err := CreateDraftPR(ctx, ghClient, "my-org", "my-repo", &output)
```

---

## Logging

All functions use structured logging via `slog`. Enable debug logging:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
slog.SetDefault(logger)
```

Log levels:
- **ERROR**: Critical failures
- **WARN**: Non-critical issues (file not found, missing access)
- **INFO**: Progress milestones
- **DEBUG**: Detailed state information
