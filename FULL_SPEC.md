# BAU CLI - Full Technical Specification

> **Document Version:** 1.1  
> **Status:** Draft  
> **Last Updated:** 2025-01-XX

## Table of Contents

- [Executive Summary](#executive-summary)
- [Requirements Analysis](#requirements-analysis)
  - [R1: CLI Interface](#r1-cli-interface)
  - [R2: Credentials Management](#r2-credentials-management)
  - [R3: Technical Plan Generation](#r3-technical-plan-generation)
  - [R4: GitHub Copilot CLI Integration](#r4-github-copilot-cli-integration)
  - [R5: Git Change Detection](#r5-git-change-detection)
  - [R6: Pull Request Creation](#r6-pull-request-creation)
- [Architecture Design](#architecture-design)
- [POC Cleanup Integration](#poc-cleanup-integration)
- [Template Files Specification](#template-files-specification)
- [Copilot CLI Configuration](#copilot-cli-configuration)
- [Security Considerations](#security-considerations)
- [Learning Resources](#learning-resources)
- [Detailed Implementation Plan](#detailed-implementation-plan)
- [Risk Assessment](#risk-assessment)
- [Missing Details & Assumptions](#missing-details--assumptions)
- [Phased Implementation](#phased-implementation)
- [Testing Strategy](#testing-strategy)
- [Appendices](#appendices)

---

## Executive Summary

This document specifies the transformation of the Google Docs Content & Feedback Extraction POC into a full-featured CLI tool called **BAU** (Build Automation Utility). The tool will:

1. Extract suggestions and comments from a Google Doc via CLI arguments
2. Generate a technical implementation plan from the extracted feedback
3. Invoke GitHub Copilot CLI to implement the plan automatically
4. Detect changes and create a Pull Request on GitHub

The goal is to create an end-to-end automation pipeline that takes document feedback and produces implemented code changes with minimal human intervention.

---

## Requirements Analysis

### R1: CLI Interface

#### Description
The program should run as a CLI tool, accepting the Google Doc ID as an argument rather than using hardcoded values.

#### Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. Standard `flag` package** | Use Go's built-in flag package for argument parsing | - No external dependencies<br>- Simple to implement<br>- Familiar to Go developers | - Limited features (no subcommands)<br>- Manual help formatting |
| **B. Cobra library** | Use spf13/cobra for CLI framework | - Industry standard<br>- Subcommand support<br>- Auto-generated help<br>- Shell completions | - External dependency<br>- More boilerplate for simple CLIs |

#### Recommendation: **Approach A (Standard `flag` package)**

**Rationale:**
- Simple requirements that don't yet justify the complexity of a full CLI framework.
- Standard library keeps dependencies minimal.
- Easy to upgrade to Cobra later if subcommands are needed.

#### Proposed CLI Interface

```bash
# Basic usage
bau --doc-id "1b9F1Av8tRNG8xkPHgjvtBKrQogXRDaRb0Lw7pEZxr9I" --credentials ./creds.json

# Dry run (skips Copilot and PR creation)
bau \
  --doc-id "1b9F1Av8tRNG8xkPHgjvtBKrQogXRDaRb0Lw7pEZxr9I" \
  --credentials ./creds.json \
  --dry-run
```

#### CLI Flags Specification

| Flag | Short | Type | Required | Default | Description |
|------|-------|------|----------|---------|-------------|
| `--doc-id` | `-d` | string | Yes | - | Google Doc ID to extract feedback from |
| `--credentials` | `-c` | string | Yes | - | Path to service account JSON |
| `--dry-run` | `-n` | bool | No | false | Run extraction and planning only; skip Copilot and PR creation |

---

### R2: Credentials Management

#### Description
How should users provide Google Cloud service account credentials to the tool?

#### Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. User-specified path only** | Require `--credentials` flag every time | - Explicit<br>- No magic<br>- Flexible | - Tedious for repeated use<br>- No sensible default |
| **B. Environment variable** | Use `GOOGLE_CREDENTIALS_PATH` env var | - Standard practice<br>- Works in CI/CD<br>- Secure | - Must be set up<br>- Can be forgotten |
| **C. Default location** | Look in `~/.config/bau/credentials.json` | - Convenient<br>- Follows XDG spec<br>- Set once, forget | - Security concerns if not secured<br>- User might not know location |
| **D. Multiple sources with precedence** | Try multiple locations in order | - Most flexible<br>- Best UX<br>- Covers all use cases | - More complex logic<br>- Might be confusing |

#### Recommendation: **Approach A (User-specified path only)**

**Rationale:**
- Explicit and simple for MVP.
- Avoids complexity of precedence logic.

#### Future Potential Improvements

- **Environment variable support:** `GOOGLE_CREDENTIALS_PATH` or `GOOGLE_APPLICATION_CREDENTIALS`.
- **Default location:** `~/.config/bau/credentials.json`.
- **Precedence logic:** Combining flags, env vars, and default paths.

#### Credentials Resolution

```
1. --credentials flag (Required)
2. Error: No credentials provided or file not found
```

#### Security Considerations

```go
// Validate credentials file permissions on Unix systems
func validateCredentialsPermissions(path string) error {
    info, err := os.Stat(path)
    if err != nil {
        return err
    }
    
    // Warn if file is world-readable
    mode := info.Mode()
    if mode&0044 != 0 {
        slog.Warn("Credentials file has insecure permissions",
            "path", path,
            "mode", mode.String(),
            "recommended", "0600")
    }
    return nil
}
```

#### First-Run Setup Experience

```bash
$ bau --doc-id "..."
Error: No credentials found.

To set up credentials:
  1. Create a service account at https://console.cloud.google.com/iam-admin/serviceaccounts
  2. Download the JSON key file
  3. Save it to one of these locations:
     - ~/.config/bau/credentials.json (recommended)
     - Set GOOGLE_CREDENTIALS_PATH environment variable
     - Pass --credentials /path/to/file.json
  4. Ensure the service account has manual access to the document.

For detailed instructions, see: https://github.com/canonical/bau#authentication
```

---

### R3: Technical Plan Generation

#### Description
After extracting feedback from the Google Doc, the tool should generate a detailed technical implementation plan that can be consumed by GitHub Copilot CLI (or another agent).

#### Target File Resolution

The tool uses multiple sources to determine which file(s) Copilot should modify:

**Resolution Order:**
1. **Metadata table `Page path` field** - Primary source from the Google Doc's metadata table
2. **Metadata table `Page title` field** - Used to search for files with matching names
3. **Document title** - Fallback, parsed to extract potential file paths
4. **First H1 heading** - Additional fallback if title doesn't contain path info

**Implementation:**
```go
type TargetFileInfo struct {
    PrimaryPath      string   `json:"primary_path"`       // From metadata table
    AlternativePaths []string `json:"alternative_paths"`  // From title/headings
    SearchPatterns   []string `json:"search_patterns"`    // Glob patterns to try
    Confidence       string   `json:"confidence"`         // "high", "medium", "low"
}

func resolveTargetFile(metadata *MetadataTable, docTitle string, firstHeading string) *TargetFileInfo {
    info := &TargetFileInfo{}
    
    // Check metadata table for explicit path
    if metadata != nil && metadata.Raw["Page path"] != "" {
        info.PrimaryPath = metadata.Raw["Page path"]
        info.Confidence = "high"
    } else if metadata != nil && metadata.PageTitle != "" {
        // Use page title to generate search patterns
        info.SearchPatterns = append(info.SearchPatterns, 
            fmt.Sprintf("**/*%s*", sanitizeForGlob(metadata.PageTitle)))
        info.Confidence = "medium"
    }
    
    // Add alternatives from document title
    if path := extractPathFromTitle(docTitle); path != "" {
        info.AlternativePaths = append(info.AlternativePaths, path)
    }
    
    // Add alternatives from first heading
    if path := extractPathFromTitle(firstHeading); path != "" {
        info.AlternativePaths = append(info.AlternativePaths, path)
    }
    
    if info.Confidence == "" {
        info.Confidence = "low"
    }
    
    return info
}
```

#### Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. Static template with placeholders** | Use Go text/template with extracted data | - Simple to implement<br>- Easy to customize<br>- Predictable output | - Limited flexibility<br>- Template maintenance |
| **B. LLM-generated plan** | Call an LLM API to generate the plan | - More intelligent<br>- Context-aware<br>- Better prose | - External dependency<br>- Cost<br>- Latency<br>- Variability |
| **C. Structured JSON output** | Output machine-readable JSON for agents | - Precise<br>- Deterministic<br>- Easy to parse | - Less readable for humans<br>- Copilot prefers natural language |
| **D. Hybrid: Template + embedded JSON** | Markdown template with embedded structured data | - Best of both worlds<br>- Human and machine readable<br>- Flexible | - More complex templates<br>- Two formats to maintain |

#### Recommendation: **Approach D (Hybrid: Template + embedded JSON)**

**Rationale:**
- Copilot CLI works best with natural language prompts
- Embedded JSON provides precision for specific changes
- Humans can review the plan before execution
- Templates can be versioned and customized per project

#### Template System Design

**Template Location Resolution:**
```
1. --template flag
2. BAU_TEMPLATE_PATH environment variable
3. ./.bau/templates/default.md (project-specific)
4. ~/.config/bau/templates/default.md (user-wide)
5. Embedded default template (compiled into binary)
```

**Template Embedding (Go 1.16+ embed):**
```go
import "embed"

//go:embed templates/*.md
var embeddedTemplates embed.FS

func loadTemplate(customPath string) (*template.Template, error) {
    // Try custom path first
    if customPath != "" {
        return template.ParseFiles(customPath)
    }
    
    // Try project-specific
    if _, err := os.Stat(".bau/templates/default.md"); err == nil {
        return template.ParseFiles(".bau/templates/default.md")
    }
    
    // Fall back to embedded
    content, err := embeddedTemplates.ReadFile("templates/default.md")
    if err != nil {
        return nil, err
    }
    return template.New("default").Parse(string(content))
}
```

**Template Data Model:**

```go
type TemplatePlanData struct {
    // Document metadata
    DocumentTitle    string    `json:"document_title"`
    DocumentID       string    `json:"document_id"`
    DocumentURL      string    `json:"document_url"`
    ExtractionTime   time.Time `json:"extraction_time"`
    
    // Target file information
    TargetFile       *TargetFileInfo `json:"target_file"`
    
    // Metadata table (if present)
    Metadata         *MetadataTable `json:"metadata,omitempty"`
    
    // Extracted feedback
    Suggestions      []ActionableSuggestion `json:"suggestions"`
    SuggestionCount  int                    `json:"suggestion_count"`
    Comments         []Comment              `json:"comments"`
    CommentCount     int                    `json:"comment_count"`
    
    // Grouped by type for easier template iteration
    Insertions       []ActionableSuggestion `json:"insertions"`
    Deletions        []ActionableSuggestion `json:"deletions"`
    Replacements     []ActionableSuggestion `json:"replacements"`
    StyleChanges     []ActionableSuggestion `json:"style_changes"`
    
    // Target context
    TargetRepo       string `json:"target_repo"`
    TargetBranch     string `json:"target_branch"`
    
    // Configuration
    Config           TemplateConfig `json:"config"`
}

type TemplateConfig struct {
    IncludeRawJSON      bool   `json:"include_raw_json"`
    IncludeVerification bool   `json:"include_verification"`
    VerboseAnchors      bool   `json:"verbose_anchors"`
}
```

#### Output Files Generated

```
<output-dir>/
├── extraction.json          # Raw extracted data
├── technical-plan.md        # Generated implementation plan
├── suggestions-detailed.json # Detailed suggestion data
├── copilot-prompt.md        # Optimized prompt for Copilot CLI
└── execution-log.txt        # Log of operations (after execution)
```

---

### R4: GitHub Copilot CLI Integration

#### Description
The tool should verify that GitHub Copilot CLI is installed, then invoke it with a prompt to implement the technical plan.

#### Approaches for Detection

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. Check binary in PATH** | Use `exec.LookPath("copilot")` | - Simple<br>- Fast<br>- Standard practice | - Doesn't verify it works |
| **B. Run version command** | Execute `copilot --version` | - Confirms it works<br>- Gets version info | - Slower<br>- Process overhead |
| **C. Both checks** | LookPath first, then version | - Most robust<br>- Good error messages | - Slightly more complex |

#### Recommendation: **Approach C (Both checks)**

```go
func checkCopilotCLI() (*CopilotInfo, error) {
    // Step 1: Check if binary exists
    path, err := exec.LookPath("copilot")
    if err != nil {
        return nil, fmt.Errorf("GitHub Copilot CLI not found in PATH. " +
            "Install it from: https://docs.github.com/en/copilot/github-copilot-in-the-cli/installing-github-copilot-in-the-cli")
    }
    
    // Step 2: Verify it works and get version
    cmd := exec.Command(path, "--version")
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("GitHub Copilot CLI found but failed to execute: %w", err)
    }
    
    return &CopilotInfo{
        Path:    path,
        Version: strings.TrimSpace(string(output)),
    }, nil
}
```

#### User Interaction Mode

**Question: Can the user interact with Copilot CLI, and once it closes, our program resumes?**

**Answer: Yes!** This is the recommended approach. Here's how it works:

```go
type CopilotExecutionMode int

const (
    // ModeInteractive - User can interact with Copilot, BAU waits for completion
    ModeInteractive CopilotExecutionMode = iota
    // ModeNonInteractive - Single prompt, no user interaction
    ModeNonInteractive
    // ModeSkip - Skip Copilot entirely, user will run manually
    ModeSkip
)
```

**Interactive Execution Flow:**

```
BAU starts
    │
    ▼
┌─────────────────────────────────┐
│ Generate technical plan         │
│ Save to ./technical-plan.md     │
└────────────────┬────────────────┘
                 │
                 ▼
┌─────────────────────────────────┐
│ Print instructions to user      │
│ "Starting Copilot CLI..."       │
│ "Press Ctrl+C when done"        │
└────────────────┬────────────────┘
                 │
                 ▼
┌─────────────────────────────────┐
│ exec.Command("copilot")         │
│ cmd.Stdin = os.Stdin            │  ◄── User can interact!
│ cmd.Stdout = os.Stdout          │
│ cmd.Stderr = os.Stderr          │
│ cmd.Run() // BLOCKS             │
└────────────────┬────────────────┘
                 │
                 │ Copilot exits (user finishes or Ctrl+C)
                 ▼
┌─────────────────────────────────┐
│ Check git status for changes    │
│ Continue with PR creation       │
└─────────────────────────────────┘
```

**Implementation:**

```go
func (e *CopilotExecutor) ExecuteInteractive(ctx context.Context) (*ExecutionResult, error) {
    // Print pre-execution instructions
    fmt.Println("\n" + strings.Repeat("=", 60))
    fmt.Println("STARTING GITHUB COPILOT CLI")
    fmt.Println(strings.Repeat("=", 60))
    fmt.Printf("\nTechnical plan saved to: %s\n", e.planFile)
    fmt.Println("\nCopilot will start with the following prompt:")
    fmt.Println("  \"Implement the changes described in @" + e.planFile + "\"")
    fmt.Println("\nYou can:")
    fmt.Println("  - Interact with Copilot normally")
    fmt.Println("  - Ask clarifying questions")
    fmt.Println("  - Guide the implementation")
    fmt.Println("  - Type 'exit' or press Ctrl+D when done")
    fmt.Println("\nBAU will resume after Copilot exits to check changes and create PR.")
    fmt.Println(strings.Repeat("-", 60) + "\n")
    
    // Build initial prompt
    initialPrompt := fmt.Sprintf(
        "Read and implement the technical plan in @%s. "+
        "The target file is likely: %s. "+
        "If you cannot find the target file, report an error. "+
        "Apply each change in order, using the anchor text to locate positions.",
        e.planFile,
        e.targetFile,
    )
    
    // Start Copilot with initial prompt
    cmd := exec.CommandContext(ctx, e.copilotPath, "--prompt", initialPrompt)
    cmd.Dir = e.workingDir
    cmd.Stdin = os.Stdin   // Allow user input
    cmd.Stdout = os.Stdout // Show Copilot output
    cmd.Stderr = os.Stderr
    
    // Run and wait for completion
    err := cmd.Run()
    
    fmt.Println("\n" + strings.Repeat("-", 60))
    fmt.Println("Copilot CLI exited. Checking for changes...")
    fmt.Println(strings.Repeat("=", 60) + "\n")
    
    if err != nil {
        // Check if it was user cancellation
        if ctx.Err() == context.Canceled {
            return &ExecutionResult{
                Success:      false,
                UserCanceled: true,
                Message:      "Copilot execution was canceled by user",
            }, nil
        }
        return &ExecutionResult{
            Success: false,
            Error:   err,
        }, nil
    }
    
    return &ExecutionResult{Success: true}, nil
}
```

#### Alternative: Skip Copilot, User Creates PR Manually

For users who prefer more control, provide `--skip-copilot` and `--skip-pr`:

```bash
# Only extract and generate plan
bau --doc-url "..." --skip-copilot --skip-pr

# Output:
# ✓ Extracted 5 suggestions from document
# ✓ Technical plan saved to: ./output/technical-plan.md
# 
# To implement manually:
#   1. Review the plan: cat ./output/technical-plan.md
#   2. Run Copilot: copilot --prompt "Implement @./output/technical-plan.md"
#   3. Create PR: gh pr create --title "Apply feedback" --body-file ./output/pr-body.md
```

#### Error Handling: Target File Not Found

The prompt should instruct Copilot to report errors clearly:

```markdown
## Critical Instructions

1. **Locate Target File First**
   - Primary path: `{{ .TargetFile.PrimaryPath }}`
   - Alternative paths to try: {{ range .TargetFile.AlternativePaths }}`{{ . }}`, {{ end }}
   - Search patterns: {{ range .TargetFile.SearchPatterns }}`{{ . }}`, {{ end }}

2. **If Target File Cannot Be Found**
   - DO NOT create a new file
   - Report the error: "ERROR: Could not locate target file. Tried: [list paths]"
   - Ask the user to specify the correct path

3. **If Anchor Text Cannot Be Found**
   - Report which anchor failed
   - Show nearby text that might be similar
   - Ask user for guidance before proceeding
```

---

### R5: Git Change Detection & Branch Management

#### Description
After Copilot CLI runs, verify that changes were made in the repository. Also ensure we're running inside a valid git repository. **Branch creation is critical** to avoid polluting the main branch.

#### Branch Strategy

```
main/master (protected)
    │
    └── bau/2025-01-15-103045-ubuntu-aws  (feature branch)
            │
            └── Changes committed here
                    │
                    └── PR created from this branch
```

**Branch Naming Convention:**
```
bau/<date>-<time>-<slug>

Examples:
- bau/2025-01-15-103045-ubuntu-aws
- bau/2025-01-15-143022-security-guide
```

#### Implementation

```go
type GitRepo struct {
    path       string
    mainBranch string
}

func NewGitRepo(path string) (*GitRepo, error) {
    repo := &GitRepo{path: path}
    
    // Verify we're in a git repo
    if !repo.IsGitRepo() {
        return nil, fmt.Errorf("not a git repository: %s\n"+
            "Please run this command from within a git repository", path)
    }
    
    // Detect main branch
    repo.mainBranch = repo.detectMainBranch()
    
    return repo, nil
}

func (r *GitRepo) IsGitRepo() bool {
    cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
    cmd.Dir = r.path
    output, err := cmd.Output()
    return err == nil && strings.TrimSpace(string(output)) == "true"
}

func (r *GitRepo) detectMainBranch() string {
    // Try to get default branch from remote
    cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
    cmd.Dir = r.path
    output, err := cmd.Output()
    if err == nil {
        // Output: refs/remotes/origin/main
        parts := strings.Split(strings.TrimSpace(string(output)), "/")
        return parts[len(parts)-1]
    }
    
    // Fallback: check if main or master exists
    for _, branch := range []string{"main", "master"} {
        cmd := exec.Command("git", "rev-parse", "--verify", branch)
        cmd.Dir = r.path
        if cmd.Run() == nil {
            return branch
        }
    }
    
    return "main" // Default assumption
}

func (r *GitRepo) GetStatus() (*GitStatus, error) {
    cmd := exec.Command("git", "status", "--porcelain", "-uall")
    cmd.Dir = r.path
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("git status failed: %w", err)
    }
    
    return parseGitStatus(string(output)), nil
}

func (r *GitRepo) HasUncommittedChanges() (bool, error) {
    status, err := r.GetStatus()
    if err != nil {
        return false, err
    }
    return len(status.Files) > 0, nil
}

func (r *GitRepo) HasChanges() (bool, error) {
    status, err := r.GetStatus()
    if err != nil {
        return false, err
    }
    return len(status.Files) > 0, nil
}

func (r *GitRepo) GetCurrentBranch() (string, error) {
    cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
    cmd.Dir = r.path
    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("failed to get current branch: %w", err)
    }
    return strings.TrimSpace(string(output)), nil
}

func (r *GitRepo) CreateAndCheckoutBranch(name string) error {
    // First, ensure we're on the main branch and it's clean
    cmd := exec.Command("git", "checkout", r.mainBranch)
    cmd.Dir = r.path
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to checkout %s: %w", r.mainBranch, err)
    }
    
    // Pull latest changes
    cmd = exec.Command("git", "pull", "--ff-only")
    cmd.Dir = r.path
    cmd.Run() // Ignore error, might be offline
    
    // Create and checkout new branch
    cmd = exec.Command("git", "checkout", "-b", name)
    cmd.Dir = r.path
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to create branch %s: %w", name, err)
    }
    
    return nil
}

func (r *GitRepo) GenerateBranchName(docTitle string) string {
    timestamp := time.Now().Format("2006-01-02-150405")
    slug := slugify(docTitle)
    if len(slug) > 30 {
        slug = slug[:30]
    }
    return fmt.Sprintf("bau/%s-%s", timestamp, slug)
}

func (r *GitRepo) AddAll() error {
    cmd := exec.Command("git", "add", "-A")
    cmd.Dir = r.path
    return cmd.Run()
}

func (r *GitRepo) Commit(message string) error {
    cmd := exec.Command("git", "commit", "-m", message)
    cmd.Dir = r.path
    return cmd.Run()
}

func (r *GitRepo) Push(remote, branch string) error {
    cmd := exec.Command("git", "push", "-u", remote, branch)
    cmd.Dir = r.path
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

func slugify(s string) string {
    // Convert to lowercase, replace spaces/special chars with dashes
    s = strings.ToLower(s)
    s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
    s = strings.Trim(s, "-")
    return s
}
```

#### Pre-execution Checks

Before starting Copilot, verify the repository is in a good state:

```go
func (r *GitRepo) ValidateForBAU() error {
    // Check for uncommitted changes
    hasChanges, err := r.HasUncommittedChanges()
    if err != nil {
        return err
    }
    if hasChanges {
        return fmt.Errorf("repository has uncommitted changes.\n" +
            "Please commit or stash your changes before running BAU:\n" +
            "  git stash        # Temporarily save changes\n" +
            "  git commit -am 'WIP'  # Or commit them")
    }
    
    // Ensure we can reach the remote
    cmd := exec.Command("git", "fetch", "--dry-run")
    cmd.Dir = r.path
    if err := cmd.Run(); err != nil {
        slog.Warn("Cannot reach remote, will work offline", "error", err)
    }
    
    return nil
}
```

---

### R6: Pull Request Creation

#### Description
Once changes are committed, automatically create a Pull Request on GitHub.

#### Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. go-git library** | Use go-git for git operations | - Pure Go<br>- No external deps | - go-git doesn't support PRs (PRs are GitHub-specific) |
| **B. GitHub API (go-github)** | Use google/go-github library | - Full GitHub API access<br>- Pure Go<br>- Official library | - Need auth token management<br>- More code |
| **C. GitHub CLI (gh)** | Shell out to `gh pr create` | - Handles auth automatically<br>- Simple<br>- Feature-rich | - External dependency<br>- Must be installed |
| **D. Hybrid: gh preferred, API fallback** | Try gh first, fall back to API | - Best UX when gh available<br>- Always works | - Two implementations<br>- More maintenance |

#### Recommendation: **Approach C (GitHub CLI) with graceful error**

**Rationale:**
- `gh` CLI is widely installed (comes with GitHub Desktop, Homebrew, etc.)
- Handles OAuth, SSH, and token auth transparently
- Has been stable for years
- Reduces our code complexity significantly
- If not installed, provide clear instructions

#### gh CLI Detection

```go
func checkGitHubCLI() (*GitHubCLIInfo, error) {
    path, err := exec.LookPath("gh")
    if err != nil {
        return nil, fmt.Errorf("GitHub CLI (gh) not found. " +
            "Install it from: https://cli.github.com/\n" +
            "After installation, run: gh auth login")
    }
    
    // Check auth status
    cmd := exec.Command(path, "auth", "status")
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("GitHub CLI found but not authenticated. " +
            "Run: gh auth login")
    }
    
    // Get version
    cmd = exec.Command(path, "--version")
    output, _ := cmd.Output()
    
    return &GitHubCLIInfo{
        Path:          path,
        Version:       parseGHVersion(string(output)),
        Authenticated: true,
    }, nil
}
```

#### PR Creation Implementation

```go
type PRCreator struct {
    ghPath    string
    repoPath  string
}

type PROptions struct {
    Title       string
    Body        string
    BaseBranch  string   // Target branch (e.g., "main")
    HeadBranch  string   // Source branch (e.g., "bau/20250115-123456")
    Labels      []string
    Reviewers   []string
    Draft       bool
}

func (p *PRCreator) CreatePR(opts PROptions) (*PRResult, error) {
    args := []string{
        "pr", "create",
        "--title", opts.Title,
        "--body", opts.Body,
        "--base", opts.BaseBranch,
        "--head", opts.HeadBranch,
    }
    
    if opts.Draft {
        args = append(args, "--draft")
    }
    
    for _, label := range opts.Labels {
        args = append(args, "--label", label)
    }
    
    for _, reviewer := range opts.Reviewers {
        args = append(args, "--reviewer", reviewer)
    }
    
    cmd := exec.Command(p.ghPath, args...)
    cmd.Dir = p.repoPath
    
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("failed to create PR: %s\n%w", string(output), err)
    }
    
    // Parse PR URL from output
    prURL := extractPRURL(string(output))
    
    return &PRResult{
        URL:     prURL,
        Created: true,
    }, nil
}
```

#### PR Content Generation

```go
func generatePRTitle(docTitle string, suggestionCount int) string {
    return fmt.Sprintf("Apply feedback from: %s (%d changes)", docTitle, suggestionCount)
}

func generatePRBody(data *TemplatePlanData) string {
    var body strings.Builder
    
    body.WriteString("## Summary\n\n")
    body.WriteString(fmt.Sprintf("This PR implements feedback from the Google Doc: [%s](%s)\n\n",
        data.DocumentTitle, data.DocumentURL))
    
    body.WriteString("## Changes Applied\n\n")
    body.WriteString(fmt.Sprintf("- **Insertions:** %d\n", len(data.Insertions)))
    body.WriteString(fmt.Sprintf("- **Deletions:** %d\n", len(data.Deletions)))
    body.WriteString(fmt.Sprintf("- **Replacements:** %d\n", len(data.Replacements)))
    body.WriteString(fmt.Sprintf("- **Comments Addressed:** %d\n\n", data.CommentCount))
    
    body.WriteString("## Details\n\n")
    body.WriteString("<details>\n<summary>Click to expand change details</summary>\n\n")
    
    for i, s := range data.Suggestions {
        body.WriteString(fmt.Sprintf("### Change %d: %s\n", i+1, s.Change.Type))
        body.WriteString(fmt.Sprintf("- **Location:** %s\n", s.Location.ParentHeading))
        if s.Change.OriginalText != "" {
            body.WriteString(fmt.Sprintf("- **Original:** `%s`\n", truncate(s.Change.OriginalText, 50)))
        }
        if s.Change.NewText != "" {
            body.WriteString(fmt.Sprintf("- **New:** `%s`\n", truncate(s.Change.NewText, 50)))
        }
        body.WriteString("\n")
    }
    
    body.WriteString("</details>\n\n")
    
    body.WriteString("---\n")
    body.WriteString("*Generated by [BAU](https://github.com/canonical/bau)*\n")
    
    return body.String()
}
```

---

## Architecture Design

### High-Level Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              BAU CLI                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐  │
│  │ 1. Parse │──▶│ 2.Extract│──▶│ 3.Generate│──▶│4.Execute │──▶│ 5.Create │  │
│  │   Args   │   │   Docs   │   │   Plan   │   │  Copilot │   │    PR    │  │
│  └──────────┘   └──────────┘   └──────────┘   └──────────┘   └──────────┘  │
│       │              │              │              │              │          │
│       ▼              ▼              ▼              ▼              ▼          │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐  │
│  │ Validate │   │Google API│   │ Template │   │ Copilot  │   │  gh CLI  │  │
│  │  Inputs  │   │  Client  │   │  Engine  │   │   CLI    │   │  Client  │  │
│  └──────────┘   └──────────┘   └──────────┘   └──────────┘   └──────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Proposed Directory Structure

```
projects/bau/
├── cmd/
│   └── bau/
│       └── main.go                 # Entry point
├── internal/
│   ├── cli/
│   │   ├── root.go                 # Root command
│   │   ├── extract.go              # Extract subcommand
│   │   ├── plan.go                 # Plan subcommand
│   │   ├── implement.go            # Implement subcommand
│   │   └── pr.go                   # PR subcommand
│   ├── config/
│   │   ├── config.go               # Configuration loading
│   │   ├── credentials.go          # Credential resolution
│   │   └── validate.go             # Input validation
│   ├── gdocs/
│   │   ├── client.go               # Docs/Drive API client
│   │   ├── extractor.go            # Document extraction
│   │   ├── suggestions.go          # Suggestion parsing
│   │   ├── comments.go             # Comment parsing
│   │   └── structure.go            # Document structure analysis
│   ├── planner/
│   │   ├── planner.go              # Plan generation
│   │   ├── template.go             # Template loading/execution
│   │   ├── funcs.go                # Template helper functions
│   │   └── target.go               # Target file resolution
│   ├── copilot/
│   │   ├── detector.go             # CLI detection
│   │   ├── executor.go             # Execution wrapper
│   │   └── config.go               # Copilot configuration
│   ├── git/
│   │   ├── repo.go                 # Repository operations
│   │   ├── status.go               # Status parsing
│   │   └── branch.go               # Branch operations
│   └── github/
│       ├── cli.go                  # gh CLI wrapper
│       └── pr.go                   # PR creation
├── pkg/
│   ├── models/
│   │   └── models.go               # Shared data types
│   └── errors/
│       └── errors.go               # Custom errors
├── templates/                       # Embedded templates
│   ├── default.md                  # Default technical plan template
│   ├── copilot-prompt.md           # Copilot-optimized prompt
│   ├── pr-body.md                  # PR description template
│   └── copilot-instructions.md     # Instructions for Copilot
├── testdata/
│   ├── sample-extraction.json      # Test fixtures
│   └── sample-plan.md
├── go.mod
├── go.sum
├── README.md
├── FULL_SPEC.md                    # This document
└── Makefile
```

---

## Security Considerations

This section details security risks specific to BAU and their mitigations.

### Credential Security

#### Risk: Credential Exposure in Logs

**Severity:** Critical  
**Likelihood:** Medium

Service account credentials could accidentally be logged or included in error messages.

**Mitigations:**

```go
// NEVER log credential file paths or contents
func loadCredentials(path string) ([]byte, error) {
    // Bad: slog.Info("Loading credentials", "path", path)
    // Good: Log without sensitive info
    slog.Debug("Loading credentials from configured path")
    
    data, err := os.ReadFile(path)
    if err != nil {
        // Bad: return nil, fmt.Errorf("failed to read %s: %w", path, err)
        // Good: Sanitize the error
        return nil, fmt.Errorf("failed to read credentials file: %w", sanitizeError(err))
    }
    return data, nil
}

func sanitizeError(err error) error {
    // Remove file paths from error messages
    msg := err.Error()
    // Replace any path that looks like it contains credentials
    re := regexp.MustCompile(`(?i)(cred|key|secret|token)[^:]*\.json`)
    msg = re.ReplaceAllString(msg, "[CREDENTIALS_FILE]")
    return errors.New(msg)
}
```

#### Risk: Insecure Credential File Permissions

**Severity:** High  
**Likelihood:** Medium

Credential files with world-readable permissions could be accessed by other users.

**Mitigations:**

```go
func validateCredentialsPermissions(path string) error {
    info, err := os.Stat(path)
    if err != nil {
        return err
    }
    
    // Check permissions on Unix systems
    if runtime.GOOS != "windows" {
        mode := info.Mode().Perm()
        if mode&0077 != 0 { // Check if group/other have any permissions
            slog.Warn("⚠️  Credentials file has insecure permissions",
                "path", "[CREDENTIALS]",
                "current_mode", fmt.Sprintf("%04o", mode),
                "recommended_mode", "0600",
                "fix", fmt.Sprintf("chmod 600 %s", path))
        }
    }
    return nil
}
```

#### Risk: Credentials Committed to Git

**Severity:** Critical  
**Likelihood:** Low

Credentials could accidentally be committed to the repository.

**Mitigations:**

1. **Add to .gitignore** (BAU should check/warn):
```gitignore
# Credentials
**/credentials.json
**/creds.json
**/*-creds.json
**/*.pem
**/*-key.json
```

2. **Pre-commit hook suggestion:**
```bash
#!/bin/sh
# .git/hooks/pre-commit
if git diff --cached --name-only | grep -qE '(cred|key|secret).*\.json$'; then
    echo "ERROR: Attempting to commit potential credentials file!"
    exit 1
fi
```

3. **BAU check at startup:**
```go
func checkCredentialsNotInRepo(repoPath, credsPath string) {
    // Check if credentials file is inside the repo
    absRepo, _ := filepath.Abs(repoPath)
    absCreds, _ := filepath.Abs(credsPath)
    
    if strings.HasPrefix(absCreds, absRepo) {
        slog.Warn("⚠️  Credentials file is inside the repository!",
            "suggestion", "Move credentials outside the repo or add to .gitignore")
    }
}
```

### API Security

#### Risk: OAuth Token Scope Over-Provisioning

**Severity:** Medium  
**Likelihood:** Medium

Using broader OAuth scopes than necessary violates principle of least privilege.

**Required Scopes (Minimum):**
| Scope | Purpose | Read-Only Alternative |
|-------|---------|----------------------|
| `https://www.googleapis.com/auth/documents.readonly` | Read doc content | ✓ Already read-only |
| `https://www.googleapis.com/auth/drive.readonly` | Read comments | ✓ Already read-only |

**Mitigation:** Never request write scopes unless explicitly needed.

#### Risk: Service Account Over-Privileged

**Severity:** Medium  
**Likelihood:** Medium

Service account may have access to more documents than necessary.

**Mitigations:**
1. Use direct document sharing (not domain-wide delegation) when possible
2. Create a dedicated service account for BAU only
3. Audit service account access periodically
4. Use short-lived credentials where possible

### Git & GitHub Security

#### Risk: Force Push to Protected Branch

**Severity:** High  
**Likelihood:** Low

Accidental force push could overwrite history.

**Mitigations:**
```go
func (r *GitRepo) Push(remote, branch string) error {
    // NEVER use --force
    cmd := exec.Command("git", "push", "-u", remote, branch)
    // ...
}

// Verify we're not pushing to main/master directly
func (r *GitRepo) ValidateBranchForPush(branch string) error {
    protectedBranches := []string{"main", "master", "develop", "production"}
    for _, protected := range protectedBranches {
        if branch == protected {
            return fmt.Errorf("refusing to push directly to protected branch: %s", branch)
        }
    }
    return nil
}
```

#### Risk: GitHub Token Exposure

**Severity:** High  
**Likelihood:** Low

If using go-github directly (fallback), tokens could be exposed.

**Mitigations:**
1. Prefer `gh` CLI (handles token securely)
2. Use environment variables, never flags for tokens
3. Never log GitHub API responses containing tokens

### Process Security

#### Risk: Command Injection

**Severity:** Critical  
**Likelihood:** Low

User-supplied input could be injected into shell commands.

**Mitigations:**
```go
// BAD: Shell injection vulnerability
func badGitCommit(message string) error {
    cmd := exec.Command("sh", "-c", fmt.Sprintf("git commit -m '%s'", message))
    return cmd.Run()
}

// GOOD: Use argument array, not shell
func goodGitCommit(message string) error {
    cmd := exec.Command("git", "commit", "-m", message)
    return cmd.Run()
}

// Sanitize any user input used in file paths
func sanitizePath(input string) string {
    // Remove path traversal attempts
    input = strings.ReplaceAll(input, "..", "")
    input = strings.ReplaceAll(input, "~", "")
    // Remove shell metacharacters
    input = regexp.MustCompile(`[;&|$` + "`" + `]`).ReplaceAllString(input, "")
    return input
}
```

#### Risk: Copilot CLI Executing Malicious Code

**Severity:** Medium  
**Likelihood:** Low

Copilot might be tricked into executing malicious commands via crafted suggestions.

**Mitigations:**
1. Interactive mode (default) - user approves commands
2. Copilot CLI has built-in approval prompts for file modifications
3. Work on feature branches, not main
4. Review changes before committing

### Data Security

#### Risk: Sensitive Document Content in Logs/Output

**Severity:** Medium  
**Likelihood:** Medium

Document content might contain sensitive information.

**Mitigations:**
```go
// Truncate content in logs
func truncateForLog(content string, maxLen int) string {
    if len(content) <= maxLen {
        return content
    }
    return content[:maxLen] + "..."
}

// Don't log full document content
slog.Debug("Extracted suggestion",
    "id", suggestion.ID,
    "type", suggestion.Type,
    "content_preview", truncateForLog(suggestion.Content, 50))
```

### Security Checklist

Before deploying BAU:

- [ ] Credentials file has 0600 permissions
- [ ] Credentials file is outside repository or in .gitignore
- [ ] Service account has minimum required scopes
- [ ] Service account only has access to necessary documents
- [ ] `gh` CLI is authenticated (not using hardcoded tokens)
- [ ] Protected branches are configured in GitHub
- [ ] Pre-commit hooks check for credential files
- [ ] Verbose logging is disabled in production

---

## Learning Resources

This section provides curated learning resources for the technologies used in BAU.

### Go Language Fundamentals

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **Go Tour** | Interactive | Official interactive Go tutorial | https://go.dev/tour/ |
| **Effective Go** | Guide | Official best practices guide | https://go.dev/doc/effective_go |
| **Go by Example** | Examples | Annotated example programs | https://gobyexample.com/ |
| **Go Documentation** | Reference | Official Go docs | https://go.dev/doc/ |
| **Go Playground** | Tool | Online Go code runner | https://go.dev/play/ |

**Recommended Learning Path:**
1. Complete the Go Tour (2-3 hours)
2. Read Effective Go (reference as needed)
3. Use Go by Example for specific patterns

### Go standard `flag` package

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **Flag Package Docs** | Docs | Official documentation | https://pkg.go.dev/flag |
| **Go by Example: Flags** | Examples | Practical examples | https://gobyexample.com/command-line-flags |

**Key `flag` Concepts:**
```go
import "flag"

var docID string
flag.StringVar(&docID, "doc-id", "", "Google Doc ID")
flag.Parse()

if docID == "" {
    fmt.Println("Error: --doc-id is required")
    flag.Usage()
    os.Exit(1)
}
```

### Google APIs for Go

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **google-api-go-client** | Repo | Auto-generated Go clients | https://github.com/googleapis/google-api-go-client |
| **Getting Started Guide** | Docs | Setup and authentication | https://github.com/googleapis/google-api-go-client/blob/main/GettingStarted.md |
| **Docs API Reference** | API | Google Docs API v1 | https://developers.google.com/docs/api/reference/rest/v1/documents |
| **Drive API Reference** | API | Google Drive API v3 | https://developers.google.com/drive/api/reference/rest/v3 |
| **OAuth2 for Go** | Package | Authentication library | https://pkg.go.dev/golang.org/x/oauth2 |
| **Service Account Guide** | Docs | Service account setup | https://cloud.google.com/iam/docs/service-accounts |

**Key Authentication Pattern:**
```go
import (
    "golang.org/x/oauth2/google"
    "google.golang.org/api/docs/v1"
    "google.golang.org/api/option"
)

// Load service account credentials
ctx := context.Background()
creds, err := os.ReadFile("credentials.json")
config, err := google.JWTConfigFromJSON(creds, 
    docs.DocumentsReadonlyScope,
    drive.DriveReadonlyScope,
)

// Create authenticated client
client := config.Client(ctx)

// Create API service
docsService, err := docs.NewService(ctx, option.WithHTTPClient(client))
```

### go-github Library

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **go-github README** | Docs | Usage and examples | https://github.com/google/go-github |
| **pkg.go.dev/go-github** | API Docs | Package documentation | https://pkg.go.dev/github.com/google/go-github/v68/github |
| **Example Code** | Examples | Code snippets | https://github.com/google/go-github/tree/master/example |

**Version:** Use latest v68+ (check for updates)
```bash
go get github.com/google/go-github/v68@latest
```

**Key Pattern (PR Creation):**
```go
import "github.com/google/go-github/v68/github"

client := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

newPR := &github.NewPullRequest{
    Title: github.Ptr("Apply feedback from doc"),
    Head:  github.Ptr("bau/feature-branch"),
    Base:  github.Ptr("main"),
    Body:  github.Ptr("PR description here"),
}

pr, _, err := client.PullRequests.Create(ctx, "owner", "repo", newPR)
```

### GitHub CLI (gh)

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **GitHub CLI Manual** | Docs | Official documentation | https://cli.github.com/manual/ |
| **gh Installation** | Guide | Installation instructions | https://github.com/cli/cli#installation |
| **gh pr create** | Reference | PR creation command | https://cli.github.com/manual/gh_pr_create |

**Key Commands:**
```bash
# Check installation
gh --version

# Authenticate
gh auth login

# Check auth status
gh auth status

# Create PR
gh pr create --title "Title" --body "Body" --base main --head feature-branch
```

### GitHub Copilot CLI

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **Copilot CLI Docs** | Docs | Official documentation | https://docs.github.com/en/copilot/github-copilot-in-the-cli |
| **Using Copilot CLI** | Guide | Usage guide | https://docs.github.com/en/copilot/how-tos/use-copilot-agents/use-copilot-cli |
| **Installing Copilot CLI** | Guide | Installation | https://docs.github.com/en/copilot/github-copilot-in-the-cli/installing-github-copilot-in-the-cli |
| **Custom Instructions** | Guide | Repository instructions | https://docs.github.com/en/copilot/customizing-copilot/adding-repository-custom-instructions-for-github-copilot |

**Key Features:**
- `copilot --prompt "..."` - Non-interactive execution
- `@file/path` - Include file contents in prompt
- `/delegate` - Push to Copilot coding agent
- Custom agents in `.github/agents/`

### Go Testing

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **Testing Package** | API Docs | Standard library testing | https://pkg.go.dev/testing |
| **Table-Driven Tests** | Guide | Testing pattern | https://go.dev/wiki/TableDrivenTests |
| **testify** | Library | Assertions and mocks | https://github.com/stretchr/testify |

**Example Test Pattern:**
```go
func TestExtractDocumentID(t *testing.T) {
    tests := []struct {
        name    string
        url     string
        want    string
        wantErr bool
    }{
        {
            name: "valid URL",
            url:  "https://docs.google.com/document/d/abc123/edit",
            want: "abc123",
        },
        {
            name:    "invalid URL",
            url:     "not-a-url",
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := extractDocumentID(tt.url)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Go Project Structure

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **Standard Go Project Layout** | Guide | Community conventions | https://github.com/golang-standards/project-layout |
| **Organizing Go Code** | Blog | Official blog post | https://go.dev/blog/organizing-go-code |
| **Go Modules** | Reference | Module system | https://go.dev/ref/mod |

### Structured Logging (slog)

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **slog Package** | API Docs | Go 1.21+ logging | https://pkg.go.dev/log/slog |
| **slog Guide** | Blog | Official introduction | https://go.dev/blog/slog |

**Example:**
```go
import "log/slog"

// Setup
slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})))

// Usage
slog.Info("Processing document", "doc_id", docID, "suggestions", count)
slog.Error("API call failed", "error", err, "endpoint", "documents.get")
```

### Go Embed (Template Embedding)

| Resource | Type | Description | Link |
|----------|------|-------------|------|
| **embed Package** | API Docs | File embedding | https://pkg.go.dev/embed |
| **Embedding Files** | Tutorial | Usage guide | https://blog.jetbrains.com/go/2021/06/09/how-to-use-go-embed-in-go-1-16/ |

**Example:**
```go
import "embed"

//go:embed templates/*.md
var templates embed.FS

func loadTemplate(name string) (string, error) {
    data, err := templates.ReadFile("templates/" + name)
    return string(data), err
}
```

### Additional Tools & References

| Tool | Purpose | Link |
|------|---------|------|
| **golangci-lint** | Linting | https://golangci-lint.run/ |
| **goreleaser** | Release automation | https://goreleaser.com/ |
| **air** | Hot reload | https://github.com/cosmtrek/air |
| **delve** | Debugger | https://github.com/go-delve/delve |

### Recommended Books

| Book | Author | Description |
|------|--------|-------------|
| **The Go Programming Language** | Donovan & Kernighan | Comprehensive Go guide |
| **Learning Go** | Jon Bodner | Modern Go practices (O'Reilly) |
| **Concurrency in Go** | Katherine Cox-Buday | Go concurrency patterns |

### Video Resources

| Resource | Platform | Link |
|----------|----------|------|
| **Go Class** | Matt Holiday (YouTube) | https://www.youtube.com/playlist?list=PLoILbKo9rG3skRCj37Kn5Zj803hhiuRK6 |
| **JustForFunc** | Francesc Campoy (YouTube) | https://www.youtube.com/c/JustForFunc |
| **Gophercises** | Calhoun (Course) | https://gophercises.com/ |

---

## Detailed Implementation Plan
## POC Cleanup Integration

The phased implementation plan includes all cleanup tasks from the README's "Future Improvements" section:

### Tasks from README to Integrate

| README Section | Task | Phase |
|----------------|------|-------|
| Remove Hardcoded Configuration | Extract `googleDocURL` to CLI flag | Phase 1 |
| Remove Hardcoded Configuration | Extract `delegationEmail` to CLI flag | Phase 1 |
| Remove Hardcoded Configuration | Extract `useDelegation` to CLI flag | Phase 1 |
| Remove Hardcoded Configuration | Extract credentials path to flag/env | Phase 1 |
| Remove Hardcoded Configuration | Extract anchor length to config | Phase 1 |
| Proposed File Structure | Split `main.go` into packages | Phase 1 |
| Proposed File Structure | Create `config/` package | Phase 1 |
| Proposed File Structure | Create `auth/` package | Phase 1 |
| Proposed File Structure | Create `docs/` package | Phase 1 |
| Proposed File Structure | Create `drive/` package | Phase 1 |
| Proposed File Structure | Create `models/` package | Phase 1 |
| Proposed File Structure | Create `output/` package | Phase 2 |
| Enhancement Ideas | CLI interface (Cobra) | Phase 1 |
| Enhancement Ideas | Environment variables support | Phase 1 |
| Enhancement Ideas | Output to file (`--output`) | Phase 2 |
| Testing Plan | Unit tests for extractors | Phase 1 |
| Testing Plan | Integration tests | Phase 3 |
| Testing Plan | CI/CD Integration | Phase 3 |

### Refactoring Map

Current `main.go` functions and their new locations:

| Current Function | New Location | Notes |
|------------------|--------------|-------|
| `buildDocsService()` | `internal/gdocs/client.go` | Rename to `NewDocsClient()` |
| `buildDriveService()` | `internal/gdocs/client.go` | Rename to `NewDriveClient()` |
| `extractDocumentID()` | `internal/gdocs/extractor.go` | Keep name |
| `fetchDocumentContent()` | `internal/gdocs/extractor.go` | Keep name |
| `extractSuggestions()` | `internal/gdocs/suggestions.go` | Keep name |
| `buildDocumentStructure()` | `internal/gdocs/structure.go` | Keep name |
| `findParentHeading()` | `internal/gdocs/structure.go` | Keep name |
| `findTableLocation()` | `internal/gdocs/structure.go` | Keep name |
| `getTextAround()` | `internal/gdocs/structure.go` | Keep name |
| `buildActionableSuggestions()` | `internal/gdocs/suggestions.go` | Keep name |
| `extractCellText()` | `internal/gdocs/structure.go` | Keep name |
| `extractMetadataTable()` | `internal/gdocs/structure.go` | Keep name |
| `fetchComments()` | `internal/gdocs/comments.go` | Keep name |
| `main()` | `cmd/bau/main.go` | Minimal, calls CLI |
| Types (Suggestion, Comment, etc.) | `pkg/models/models.go` | Group all types |

---

## Template Files Specification

### Template 1: `templates/default.md` (Technical Plan)

**Purpose:** Main technical implementation plan for human review and Copilot consumption.

**Required Sections:**
1. Overview header with document metadata
2. Target file information with confidence level
3. Implementation tasks (numbered, actionable)
4. Change details with anchor text
5. Verification data (embedded JSON)
6. Comments section
7. Execution notes

**Template Variables:**
- `{{ .DocumentTitle }}` - Title of the Google Doc
- `{{ .DocumentID }}` - Google Doc ID
- `{{ .DocumentURL }}` - Full URL to the doc
- `{{ .ExtractionTime }}` - ISO timestamp
- `{{ .TargetFile.PrimaryPath }}` - Most likely target file
- `{{ .TargetFile.AlternativePaths }}` - Backup paths to try
- `{{ .TargetFile.Confidence }}` - "high", "medium", "low"
- `{{ .Metadata.PageTitle }}` - From metadata table
- `{{ .Metadata.PageDescription }}` - From metadata table
- `{{ .SuggestionCount }}` - Total suggestions
- `{{ .CommentCount }}` - Total comments
- `{{ .Suggestions }}` - Array of ActionableSuggestion
- `{{ .Insertions }}` - Filtered suggestions
- `{{ .Deletions }}` - Filtered suggestions
- `{{ .Replacements }}` - Filtered suggestions
- `{{ .Comments }}` - Array of Comment
- `{{ .TargetRepo }}` - Path to repository
- `{{ .TargetBranch }}` - Branch name

**Template Functions:**
- `{{ toJSON .Anchor }}` - Serialize to JSON
- `{{ truncate .Text 50 }}` - Truncate with ellipsis
- `{{ add .Index 1 }}` - Arithmetic
- `{{ title .Type }}` - Title case

---

### Template 2: `templates/copilot-prompt.md`

**Purpose:** Optimized prompt specifically for Copilot CLI's `--prompt` flag.

**Characteristics:**
- Concise, action-oriented language
- Clear error handling instructions
- File references using `@` syntax
- Step-by-step execution order

**Example Content:**
```markdown
# Task: Implement Document Feedback

## Target File
Primary: `{{ .TargetFile.PrimaryPath }}`
{{- if .TargetFile.AlternativePaths }}
Alternatives: {{ range .TargetFile.AlternativePaths }}`{{ . }}` {{ end }}
{{- end }}

## Instructions
1. Locate the target file (error if not found)
2. Apply {{ .SuggestionCount }} changes in order below
3. Use anchor text to find exact positions
4. Verify each change matches expected result

## Changes
{{ range $i, $s := .Suggestions }}
### {{ add $i 1 }}. {{ $s.Change.Type | title }}
- Find: `{{ truncate $s.Anchor.PrecedingText 40 }}`
- {{ if eq $s.Change.Type "insert" }}Insert after: `{{ $s.Change.NewText }}`
{{- else if eq $s.Change.Type "delete" }}Delete: `{{ $s.Change.OriginalText }}`
{{- else }}Replace `{{ $s.Change.OriginalText }}` with `{{ $s.Change.NewText }}`
{{- end }}
{{ end }}

## Error Handling
- If file not found: STOP and report error
- If anchor not found: Report which anchor failed, show similar text
- If ambiguous match: Ask for clarification
```

---

### Template 3: `templates/pr-body.md`

**Purpose:** Pull request description template.

**Required Sections:**
1. Summary with link to source doc
2. Change statistics
3. Detailed change list (collapsible)
4. Comments addressed (if any)
5. Verification checklist
6. Footer with BAU attribution

**Example Content:**
```markdown
## Summary

This PR implements feedback from the Google Doc: [{{ .DocumentTitle }}]({{ .DocumentURL }})

**Generated by BAU** on {{ .ExtractionTime.Format "2006-01-02 15:04" }}

## Changes Applied

| Type | Count |
|------|-------|
| Insertions | {{ len .Insertions }} |
| Deletions | {{ len .Deletions }} |
| Replacements | {{ len .Replacements }} |
| Comments Addressed | {{ .CommentCount }} |

<details>
<summary>📝 Detailed Changes</summary>

{{ range $i, $s := .Suggestions }}
### Change {{ add $i 1 }}: {{ $s.Change.Type | title }}

**Location:** {{ if $s.Location.ParentHeading }}{{ $s.Location.ParentHeading }}{{ else }}Document root{{ end }}

{{ if eq $s.Change.Type "insert" -}}
**Inserted:** `{{ truncate $s.Change.NewText 100 }}`
{{- else if eq $s.Change.Type "delete" -}}
**Deleted:** `{{ truncate $s.Change.OriginalText 100 }}`
{{- else -}}
**Before:** `{{ truncate $s.Change.OriginalText 50 }}`
**After:** `{{ truncate $s.Change.NewText 50 }}`
{{- end }}

{{ end }}
</details>

{{ if .Comments }}
## Comments Addressed

{{ range .Comments }}
- **{{ .Author }}**: {{ truncate .Content 100 }}
{{ end }}
{{ end }}

## Verification Checklist

- [ ] Changes match the suggestions in the source document
- [ ] No unintended modifications
- [ ] Page renders correctly (if applicable)
- [ ] Tests pass (if applicable)

---

*Generated by [BAU](https://github.com/canonical/bau) - Build Automation Utility*
```

---

### Template 4: `templates/copilot-instructions.md`

**Purpose:** Project-level Copilot instructions to include in `.github/copilot-instructions.md`

**Note:** This is a template for users to customize, not used directly by BAU.

**Example Content:**
```markdown
# BAU Implementation Instructions

When implementing changes from a BAU technical plan:

## Finding Locations

1. **Use anchor text exactly** - The `preceding_text` and `following_text` fields contain exact strings
2. **Search globally first** - Don't assume the location, search the entire file
3. **Report if not found** - Never guess, always report missing anchors

## Applying Changes

| Change Type | Action |
|-------------|--------|
| `insert` | Add `new_text` immediately after `preceding_text` |
| `delete` | Remove `original_text` between anchors |
| `replace` | Replace `original_text` with `new_text` |

## Verification

After each change, verify:
- The surrounding text matches `verification.text_after_change`
- No duplicate insertions occurred
- Formatting is preserved

## Error Handling

If you encounter issues:
1. Stop and report the specific problem
2. Show the text you found vs. expected
3. Ask for guidance before proceeding

## Commit Messages

Use format: `Apply feedback: [brief description]`
```

---

### Template 5: `templates/extraction-summary.json` (Schema)

**Purpose:** JSON schema reference for extraction output.

**Note:** Not a Go template, but a schema definition for documentation.

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "BAU Extraction Output",
  "type": "object",
  "required": ["document_title", "document_id", "suggestions"],
  "properties": {
    "document_title": { "type": "string" },
    "document_id": { "type": "string" },
    "document_url": { "type": "string", "format": "uri" },
    "extraction_time": { "type": "string", "format": "date-time" },
    "target_file": {
      "type": "object",
      "properties": {
        "primary_path": { "type": "string" },
        "alternative_paths": { "type": "array", "items": { "type": "string" } },
        "search_patterns": { "type": "array", "items": { "type": "string" } },
        "confidence": { "type": "string", "enum": ["high", "medium", "low"] }
      }
    },
    "metadata": {
      "type": "object",
      "properties": {
        "raw": { "type": "object" },
        "page_title": { "type": "string" },
        "page_description": { "type": "string" }
      }
    },
    "suggestion_count": { "type": "integer" },
    "comment_count": { "type": "integer" },
    "suggestions": {
      "type": "array",
      "items": { "$ref": "#/definitions/ActionableSuggestion" }
    },
    "comments": {
      "type": "array",
      "items": { "$ref": "#/definitions/Comment" }
    }
  },
  "definitions": {
    "ActionableSuggestion": {
      "type": "object",
      "properties": {
        "id": { "type": "string" },
        "anchor": {
          "type": "object",
          "properties": {
            "preceding_text": { "type": "string" },
            "following_text": { "type": "string" }
          }
        },
        "change": {
          "type": "object",
          "properties": {
            "type": { "type": "string", "enum": ["insert", "delete", "replace"] },
            "original_text": { "type": "string" },
            "new_text": { "type": "string" }
          }
        },
        "verification": {
          "type": "object",
          "properties": {
            "text_before_change": { "type": "string" },
            "text_after_change": { "type": "string" }
          }
        },
        "location": { "type": "object" },
        "position": {
          "type": "object",
          "properties": {
            "start_index": { "type": "integer" },
            "end_index": { "type": "integer" }
          }
        }
      }
    },
    "Comment": {
      "type": "object",
      "properties": {
        "id": { "type": "string" },
        "author": { "type": "string" },
        "content": { "type": "string" },
        "quoted_content": { "type": "string" },
        "resolved": { "type": "boolean" },
        "replies": { "type": "array" }
      }
    }
  }
}
```

---

## Copilot CLI Configuration

### Can We Configure Copilot CLI Programmatically?

**Yes, partially.** Copilot CLI stores configuration in `~/.copilot/` directory:

| File | Purpose | Can Modify? |
|------|---------|-------------|
| `config.json` | General settings | Yes, but undocumented |
| `mcp-config.json` | MCP server configurations | Yes, documented |
| `trusted-directories.json` | Trusted directories list | Yes, but risky |
| `permissions.json` | Tool permissions | Yes, but risky |

### MCP Server Configuration

**What is MCP?** Model Context Protocol - allows extending Copilot's capabilities with external tools.

**Relevant MCP Servers for BAU:**

| Server | Purpose | Why Useful |
|--------|---------|------------|
| **GitHub MCP** | GitHub API operations | Already built-in, used for PR operations |
| **Filesystem MCP** | Enhanced file operations | Better file search/read capabilities |
| **Git MCP** | Git operations | Could help with commits, branches |

**How to Add MCP Servers:**

Option 1: Interactive (user does manually):
```bash
copilot
/mcp add
# Fill in server details
```

Option 2: Programmatic (BAU could do this):
```go
func configureMCPServer(name string, command string, args []string) error {
    configPath := filepath.Join(os.Getenv("HOME"), ".copilot", "mcp-config.json")
    
    // Read existing config
    data, err := os.ReadFile(configPath)
    if err != nil && !os.IsNotExist(err) {
        return err
    }
    
    var config map[string]interface{}
    if len(data) > 0 {
        json.Unmarshal(data, &config)
    } else {
        config = make(map[string]interface{})
    }
    
    // Add server
    servers, _ := config["servers"].(map[string]interface{})
    if servers == nil {
        servers = make(map[string]interface{})
    }
    
    servers[name] = map[string]interface{}{
        "command": command,
        "args":    args,
    }
    config["servers"] = servers
    
    // Write back
    newData, _ := json.MarshalIndent(config, "", "  ")
    return os.WriteFile(configPath, newData, 0600)
}
```

**Recommendation:** Don't auto-configure MCP servers. Instead:
1. Document recommended MCP servers in README
2. Provide a `bau setup-copilot` command that guides users
3. Check for required MCP servers and warn if missing

### Trust Directory Handling

**Problem:** Copilot asks for trust confirmation per directory.

**Solutions:**

1. **Document the requirement** (Recommended)
   ```bash
   # First run in a new repo:
   cd /path/to/repo
   copilot  # Accept trust prompt
   exit
   # Now BAU can work
   ```

2. **Programmatic trust addition** (Not recommended - security risk)
   ```go
   // This would modify ~/.copilot/trusted-directories.json
   // But it bypasses user consent - don't do this
   ```

3. **Check trust status and prompt user**
   ```go
   func ensureDirectoryTrusted(dir string) error {
       // Check if trusted
       if isTrusted(dir) {
           return nil
       }
       
       fmt.Printf("Directory %s is not trusted by Copilot CLI.\n", dir)
       fmt.Println("Please run: copilot")
       fmt.Println("And select 'Yes, and remember this folder for future sessions'")
       fmt.Println("Then run BAU again.")
       return fmt.Errorf("directory not trusted")
   }
   ```

### Custom Instructions

Copilot CLI automatically reads `.github/copilot-instructions.md` in the repository.

**BAU can:**
1. Check if this file exists
2. Suggest creating it with BAU-specific instructions
3. Provide a template (see Template 4 above)

```go
func checkCopilotInstructions(repoPath string) {
    instructionsPath := filepath.Join(repoPath, ".github", "copilot-instructions.md")
    
    if _, err := os.Stat(instructionsPath); os.IsNotExist(err) {
        slog.Info("Tip: Create .github/copilot-instructions.md for better Copilot results",
            "path", instructionsPath)
        fmt.Println("\nRun: bau init-copilot-instructions")
        fmt.Println("To create a template with BAU-optimized instructions.")
    }
}
```

### Other Copilot CLI Considerations

1. **Custom Agents:**
   - Copilot supports custom agents in `~/.copilot/agents/` or `.github/agents/`
   - BAU could provide a custom agent optimized for document feedback
   - Future enhancement

2. **Environment Variables:**
   | Variable | Purpose |
   |----------|---------|
   | `XDG_CONFIG_HOME` | Changes Copilot config location |
   | `COPILOT_DEBUG` | Enable debug logging |
   | `NO_COLOR` | Disable colored output |

3. **Token Limits:**
   - Copilot CLI has context token limits
   - Large technical plans may need to be chunked
   - `/usage` command shows token usage

4. **Session Persistence:**
   - `copilot --continue` resumes last session
   - Could be useful if BAU needs to restart

---

## Detailed Implementation Plan

### Complete Execution Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         BAU Execution Flow                                   │
└─────────────────────────────────────────────────────────────────────────────┘

START
  │
  ▼
┌─────────────────┐     ┌─────────────────┐
│ Parse CLI Args  │────▶│ Load Config     │
└─────────────────┘     └────────┬────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │ Validate Inputs │
                        │ - Doc URL valid │
                        │ - Creds exist   │
                        │ - Repo is git   │
                        └────────┬────────┘
                                 │
                     ┌───────────┴───────────┐
                     │ Prerequisites Check   │
                     └───────────┬───────────┘
                                 │
         ┌───────────────────────┼───────────────────────┐
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ Check: copilot  │     │ Check: gh CLI   │     │ Check: git repo │
│ CLI installed   │     │ authenticated   │     │ clean state     │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │ All checks pass?│──No──▶ EXIT with error
                        └────────┬────────┘
                                 │Yes
                                 ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                        PHASE 1: EXTRACTION                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│  │ Build Google    │────▶│ Fetch Document  │────▶│ Extract         │       │
│  │ API Clients     │     │ with Suggestions│     │ Suggestions     │       │
│  └─────────────────┘     └─────────────────┘     └─────────────────┘       │
│                                                          │                  │
│                                                          ▼                  │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│  │ Build Actionable│◀────│ Build Document  │◀────│ Extract         │       │
│  │ Suggestions     │     │ Structure       │     │ Comments        │       │
│  └────────┬────────┘     └─────────────────┘     └─────────────────┘       │
│           │                                                                  │
│           ▼                                                                  │
│  ┌─────────────────┐     ┌─────────────────┐                                │
│  │ Resolve Target  │────▶│ Save extraction │                                │
│  │ File Path       │     │ to JSON file    │                                │
│  └─────────────────┘     └────────┬────────┘                                │
│                                   │                                          │
└───────────────────────────────────┼──────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                     PHASE 2: PLAN GENERATION                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│  │ Load Template   │────▶│ Prepare Template│────▶│ Execute         │       │
│  │ (embedded/file) │     │ Data Model      │     │ Template        │       │
│  └─────────────────┘     └─────────────────┘     └────────┬────────┘       │
│                                                           │                 │
│                                                           ▼                 │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│  │ Save pr-body.md │◀────│ Save copilot-   │◀────│ Save technical- │       │
│  │                 │     │ prompt.md       │     │ plan.md         │       │
│  └─────────────────┘     └─────────────────┘     └────────┬────────┘       │
│                                                           │                 │
└───────────────────────────────────────────────────────────┼─────────────────┘
                                                            │
                        ┌───────────────────────────────────┘
                        │
                        ▼
               ┌─────────────────┐
               │ --skip-copilot? │──Yes──▶ Skip to Git Check
               └────────┬────────┘         (print manual instructions)
                        │No
                        ▼
               ┌─────────────────┐
               │ Create feature  │
               │ branch          │
               └────────┬────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    PHASE 3: COPILOT EXECUTION                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Print instructions:                                                  │    │
│  │   "Starting Copilot CLI in interactive mode..."                     │    │
│  │   "You can interact with Copilot normally."                         │    │
│  │   "Type 'exit' or press Ctrl+D when done."                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                         │
│                                    ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ exec.Command("copilot", "--prompt", "Implement @technical-plan.md") │    │
│  │ cmd.Stdin = os.Stdin   // User can interact!                        │    │
│  │ cmd.Stdout = os.Stdout                                              │    │
│  │ cmd.Run()  // BLOCKS until Copilot exits                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                         │
│                                    │ User finishes and exits Copilot         │
│                                    ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Print: "Copilot exited. Checking for changes..."                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PHASE 4: GIT OPERATIONS                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│  │ Check git       │────▶│ Has changes?    │──No─▶│ WARN: No changes│       │
│  │ status          │     └────────┬────────┘      │ Ask: continue?  │       │
│  └─────────────────┘              │Yes            └────────┬────────┘       │
│                                   │                        │No              │
│                                   │◀───────────────────────┘Yes             │
│                                   ▼                                          │
│                          ┌─────────────────┐                                │
│                          │ Stage all       │                                │
│                          │ changes (git add)│                               │
│                          └────────┬────────┘                                │
│                                   │                                          │
│                                   ▼                                          │
│                          ┌─────────────────┐                                │
│                          │ Commit with     │                                │
│                          │ generated msg   │                                │
│                          └────────┬────────┘                                │
│                                   │                                          │
│                                   ▼                                          │
│                          ┌─────────────────┐                                │
│                          │ Push to origin  │                                │
│                          └────────┬────────┘                                │
│                                   │                                          │
└───────────────────────────────────┼──────────────────────────────────────────┘
                                    │
                        ┌───────────┘
                        │
                        ▼
               ┌─────────────────┐
               │ --skip-pr?      │──Yes──▶ EXIT: Success
               └────────┬────────┘         (print: "Run 'gh pr create' manually")
                        │No
                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PHASE 5: PR CREATION                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│  │ Generate PR     │────▶│ Create PR via   │────▶│ Output PR URL   │       │
│  │ Title & Body    │     │ gh pr create    │     │                 │       │
│  └─────────────────┘     └─────────────────┘     └────────┬────────┘       │
│                                                           │                 │
└───────────────────────────────────────────────────────────┼─────────────────┘
                                                            │
                                                            ▼
                                                    ┌─────────────────┐
                                                    │ EXIT: Success   │
                                                    │ Print summary   │
                                                    └─────────────────┘
```

---

## Progress Reporting

### Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. Simple println** | Print status messages | - Simple<br>- No deps | - No structure<br>- Hard to parse |
| **B. Structured logging (slog)** | Use Go's slog package | - Built-in<br>- Configurable levels<br>- JSON output option | - More verbose for simple cases |
| **C. Progress bars** | Use a library like progressbar | - Visual feedback<br>- Good UX | - External dependency<br>- Overkill for quick operations |
| **D. Hybrid: slog + spinners** | Structured logging with visual spinners for long ops | - Best UX<br>- Informative<br>- Professional | - More complex |

### Recommendation: **Approach B (Structured logging with slog)**

For Phase 1, use simple structured logging. Can enhance with spinners later.

### Implementation

```go
func setupLogging(verbose bool) {
    level := slog.LevelInfo
    if verbose {
        level = slog.LevelDebug
    }
    
    handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: level,
    })
    slog.SetDefault(slog.New(handler))
}

// Usage throughout the code:
slog.Info("Extracting document", "doc_id", docID)
slog.Debug("Found suggestion", "id", suggestion.ID, "type", suggestion.Type)
slog.Warn("No changes detected after Copilot execution")
slog.Error("Failed to create PR", "error", err)
```

### Progress Output Example

```
$ bau --doc-url "https://docs.google.com/..."

BAU - Build Automation Utility v1.0.0
=====================================

[1/5] Validating environment...
  ✓ Git repository detected
  ✓ Repository is clean
  ✓ GitHub Copilot CLI found (v1.0.0)
  ✓ GitHub CLI authenticated

[2/5] Extracting from Google Doc...
  ✓ Document fetched: "Ubuntu on AWS - index.html"
  ✓ Found 5 suggestions (3 insertions, 2 deletions)
  ✓ Found 2 comments
  ✓ Target file: templates/aws/ubuntu.html (confidence: high)

[3/5] Generating technical plan...
  ✓ Saved: ./output/extraction.json
  ✓ Saved: ./output/technical-plan.md
  ✓ Saved: ./output/copilot-prompt.md

[4/5] Starting Copilot CLI...
============================================================
  Copilot will start with the technical plan.
  You can interact normally. Exit when done.
============================================================

[Copilot CLI session - user interacts here]

============================================================
  Copilot exited. Checking for changes...
============================================================

[5/5] Creating pull request...
  ✓ Branch created: bau/2025-01-15-103045-ubuntu-aws
  ✓ Changes committed: 3 files modified
  ✓ Pushed to origin
  ✓ PR created: https://github.com/org/repo/pull/123

=====================================
SUCCESS: Feedback applied and PR created!
  PR: https://github.com/org/repo/pull/123
  Branch: bau/2025-01-15-103045-ubuntu-aws
  Changes: 5 suggestions applied
=====================================
```

---

## Risk Assessment

### Comprehensive Risk Matrix

| ID | Risk | Category | Impact | Likelihood | Severity | Mitigation | Status |
|----|------|----------|--------|------------|----------|------------|--------|
| R1 | Copilot CLI interactive prompts block automation | Technical | High | High | Critical | Allow interactive mode; user controls Copilot directly | Mitigated |
| R2 | Copilot CLI API/behavior changes | External | High | Medium | High | Version check, defensive coding, --skip-copilot option | Mitigated |
| R3 | Google API rate limits hit | External | Medium | Medium | Medium | Exponential backoff, single document mode | Mitigated |
| R4 | Credentials exposed in logs/errors | Security | Critical | Low | High | Never log creds, sanitize errors, permission warnings | Mitigated |
| R5 | Git repo has uncommitted changes | Operational | Medium | High | Medium | Pre-check for clean state, refuse to proceed | Mitigated |
| R6 | Template syntax errors at runtime | Technical | Medium | Medium | Medium | Validate templates at startup, good error messages | Mitigated |
| R7 | gh CLI not installed or authenticated | Technical | High | Medium | High | Upfront check, clear installation instructions | Mitigated |
| R8 | Target file cannot be found | Operational | Medium | Medium | Medium | Multiple resolution methods, prompt instructs Copilot to error | Mitigated |
| R9 | Anchor text changed since doc created | Operational | Medium | Medium | Medium | Verification data helps, Copilot reports failures | Accepted |
| R10 | Large documents cause memory/timeout | Performance | Low | Low | Low | Streaming where possible, configurable timeout | Accepted |
| R11 | Network failures during API calls | Infrastructure | Medium | Medium | Medium | Retry with backoff, clear error messages | Mitigated |
| R12 | User cancels mid-execution | Operational | Low | Medium | Low | Graceful SIGINT handling, branch isolation | Mitigated |
| R13 | PR created with incorrect changes | Quality | High | Low | Medium | Draft PR option, verification checklist in PR body | Mitigated |
| R14 | Multiple BAU instances run simultaneously | Operational | Medium | Low | Low | Lock file or unique branch names | Mitigated |
| R15 | Copilot CLI not trusted for directory | Technical | High | High | High | Check trust, guide user to establish trust first | Mitigated |
| R16 | MCP servers not configured | Technical | Low | Medium | Low | Document recommended setup, warn if missing | Accepted |
| R17 | Token limits exceeded in Copilot | Technical | Medium | Low | Medium | Chunking strategy, warn about large plans | Future |

### Risk Response Strategies

**R1: Copilot CLI Interactive Prompts**
```go
// Solution: Full interactive mode with stdin/stdout passthrough
func (e *CopilotExecutor) ExecuteInteractive() error {
    cmd := exec.Command(e.copilotPath, "--prompt", e.initialPrompt)
    cmd.Dir = e.workingDir
    cmd.Stdin = os.Stdin   // User can type
    cmd.Stdout = os.Stdout // User sees output
    cmd.Stderr = os.Stderr
    return cmd.Run() // Blocks until user exits
}
```

**R4: Credential Security**
```go
// Never log credential paths in production
func loadCredentials(path string) ([]byte, error) {
    slog.Debug("Loading credentials", "source", maskPath(path))
    // ...
}

func maskPath(path string) string {
    if strings.Contains(path, "cred") {
        return "[CREDENTIALS_FILE]"
    }
    return path
}
```

**R5: Git Clean State Check**
```go
func (r *GitRepo) ValidateForBAU() error {
    hasChanges, _ := r.HasUncommittedChanges()
    if hasChanges {
        return &GitDirtyError{
            Message: "Repository has uncommitted changes",
            Suggestion: "Run 'git stash' or commit your changes first",
        }
    }
    return nil
}
```

**R15: Copilot Trust Check**
```go
func checkCopilotTrust(repoPath string) error {
    // Try a simple non-modifying command
    cmd := exec.Command("copilot", "--help")
    cmd.Dir = repoPath
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("Copilot CLI may not trust this directory.\n" +
            "Run 'copilot' interactively once and accept the trust prompt:\n" +
            "  cd %s\n" +
            "  copilot\n" +
            "  # Choose 'Yes, and remember this folder'\n" +
            "  # Then exit with Ctrl+C", repoPath)
    }
    return nil
}
```

---

## Phased Implementation Plan

### Phase 1: CLI Foundation & POC Cleanup

**Objective:** Transform hardcoded POC into configurable CLI tool with proper project structure

**Tasks:**

| Task | Description | From README |
|------|-------------|-------------|
| 1.1 | Add standard `flag` package support | Enhancement Ideas #1 |
| 1.2 | Implement `--doc-id` flag | Remove Hardcoded Config |
| 1.3 | Implement `--credentials` flag (file path only) | Remove Hardcoded Config |
| 1.4 | Implement `--dry-run` flag (skip Copilot/PR) | New |
| 1.5 | Create `internal/config/` package | Proposed File Structure |
| 1.6 | Create `internal/gdocs/` package (refactor) | Proposed File Structure |
| 1.7 | Create `internal/models/` package | Proposed File Structure |
| 1.8 | Add slog-based logging with JSON prettification options | New |
| 1.9 | Unit tests for config and validation | Testing Plan |
| 1.10 | Refactor `buildDocumentStructure`: extract heading extraction into a helper function | New |
| 1.11 | Update `extractMetadataTable`: add check for "metadata" (case-insensitive) in first cell | New |
| 1.12 | Enhance table extraction: extract table title/header (text right above the table) | New |
| 1.13 | Add git repository validation (Deferred to Phase 3) | New |

**Deliverables:**
- [x] flag-based CLI with MVP flags (`--doc-id`, `--credentials`, `--dry-run`)
- [x] Credential loading from file path
- [x] Refactored code in proper package structure
- [x] Unit tests for config package
- [x] Heading extraction helper
- [x] Prettified slog output
- [x] Enhanced table and metadata extraction logic
- [ ] Git repository detection and validation (Deferred to Phase 3)

**Exit Criteria:**
```bash
bau --doc-id "1b9F1Av8tRNG8xkPHgjvtBKrQogXRDaRb0Lw7pEZxr9I" --credentials ./creds.json
bau --help
bau --doc-id "1b9F1Av8..." --credentials ./creds.json --dry-run
```

### Phase 2: Plan Generation & Templates

**Objective:** Generate machine-readable technical implementation plans with embedded templates

**Tasks:**

| Task | Description | From README |
|------|-------------|-------------|
| 2.1 | Create `templates/default.md` | New |
| 2.2 | Create `templates/copilot-prompt.md` | New |
| 2.3 | Create `templates/pr-body.md` | New |
| 2.4 | Implement Go embed for templates | New |
| 2.5 | Create `internal/planner/` package | Proposed File Structure |
| 2.6 | Implement template data model | New |
| 2.7 | Implement template helper functions | New |
| 2.8 | Implement target file resolution | New |
| 2.9 | Add `--template` flag support | Enhancement Ideas |
| 2.10 | Add `--output` flag support | Enhancement Ideas #5 |
| 2.11 | Generate extraction.json output | New |
| 2.12 | Generate technical-plan.md output | New |
| 2.13 | Unit tests for planner package | Testing Plan |

**Deliverables:**
- [ ] Embedded template system
- [ ] Template helper functions (toJSON, truncate, etc.)
- [ ] Target file resolution from metadata
- [ ] Output file generation
- [ ] Custom template support via flag

**Exit Criteria:**
```bash
bau --doc-id "..." # Produces ./output/technical-plan.md
bau --doc-id "..." --template ./custom.md
bau --doc-id "..." --output ./my-dir
```

### Phase 3: Copilot & GitHub Integration

**Objective:** Full automation pipeline with interactive Copilot and PR creation

**Tasks:**

| Task | Description | From README |
|------|-------------|-------------|
| 3.1 | Create `internal/copilot/` package | New |
| 3.2 | Implement Copilot CLI detection | New |
| 3.3 | Implement interactive execution mode | New |
| 3.4 | Create `internal/git/` package | New |
| 3.5 | Implement branch creation | New |
| 3.6 | Implement change detection | New |
| 3.7 | Implement commit and push | New |
| 3.8 | Create `internal/github/` package | New |
| 3.9 | Implement gh CLI detection | New |
| 3.10 | Implement PR creation | New |
| 3.11 | Generate PR title and body | New |
| 3.12 | Add `--skip-copilot` flag | New |
| 3.13 | Add `--skip-pr` flag | New |
| 3.14 | Add `--branch` flag | New |
| 3.15 | Add `--interactive` flag | New |
| 3.16 | Integration tests | Testing Plan |
| 3.17 | End-to-end manual testing | Testing Plan |
| 3.18 | Documentation updates | New |


**Deliverables:**
- [ ] Copilot CLI detection and interactive execution
- [ ] Git branch/commit/push operations
- [ ] PR creation via gh CLI
- [ ] Skip flags for each phase
- [ ] Complete documentation

**Exit Criteria:**
```bash
bau --doc-url "..." # Full pipeline end-to-end
bau --doc-url "..." --skip-copilot # Manual Copilot later
bau --doc-url "..." --skip-pr # Create PR manually
bau --doc-url "..." --dry-run # Preview everything
```

---

## Testing Strategy

### Unit Tests

| Package | Test File | Test Cases |
|---------|-----------|------------|
| `internal/config` | `config_test.go` | Flag parsing, env var precedence, credential resolution, validation |
| `internal/gdocs` | `client_test.go` | Mock auth, error handling |
| `internal/gdocs` | `extractor_test.go` | Document ID extraction, URL parsing |
| `internal/gdocs` | `suggestions_test.go` | All suggestion types, nested structures |
| `internal/gdocs` | `comments_test.go` | Comment parsing, replies |
| `internal/gdocs` | `structure_test.go` | Heading extraction, table detection |
| `internal/planner` | `template_test.go` | Template loading, rendering, helpers |
| `internal/planner` | `target_test.go` | Target file resolution |
| `internal/git` | `repo_test.go` | Repo detection, status parsing, branch names |
| `internal/copilot` | `detector_test.go` | CLI detection, version parsing |
| `internal/github` | `cli_test.go` | gh detection, auth check |
| `internal/github` | `pr_test.go` | PR URL parsing |

### Integration Tests

| Test | Description | Setup |
|------|-------------|-------|
| `TestFullExtractionFlow` | Extract from mock Google API | HTTP mock server |
| `TestTemplateRendering` | All templates render without error | Sample data fixtures |
| `TestGitOperations` | Branch/commit/push in temp repo | `git init` in temp dir |
| `TestErrorHandling` | Graceful failures at each stage | Various error conditions |

### Manual Testing Checklist

- [ ] Run against real Google Doc with insertions
- [ ] Run against real Google Doc with deletions
- [ ] Run against real Google Doc with mixed types
- [ ] Test with domain-wide delegation
- [ ] Test with direct service account access
- [ ] Test Copilot CLI interactive session
- [ ] Verify PR content and formatting
- [ ] Test all flag combinations
- [ ] Test error recovery scenarios
- [ ] Test on macOS, Linux, Windows (if applicable)

---

## Appendices

### Appendix A: Environment Variables Reference

| Variable | Description | Example | Default |
|----------|-------------|---------|---------|
| `BAU_DOC_ID` | Google Doc ID | `1b9F1Av8tRNG8xkPHgjvtBKrQogXRDaRb0Lw7pEZxr9I` | - |
| `GOOGLE_CREDENTIALS_PATH` | Path to credentials JSON | `~/.config/bau/credentials.json` | (resolution chain) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Standard Google SDK path | `/path/to/creds.json` | - |
| `BAU_OUTPUT_DIR` | Output directory | `./output` | `.` |
| `BAU_TEMPLATE_PATH` | Custom template path | `./templates/custom.md` | (embedded) |
| `BAU_TARGET_REPO` | Target repository | `/path/to/repo` | `.` |
| `BAU_BRANCH` | Branch name | `bau/feature-x` | (auto-generated) |
| `BAU_VERBOSE` | Enable verbose logging | `true` | `false` |
| `BAU_INTERACTIVE` | Allow Copilot interaction | `true` | `true` |
| `BAU_ANCHOR_LENGTH` | Anchor text length | `80` | `80` |

### Appendix B: Exit Codes

| Code | Meaning | Recoverable |
|------|---------|-------------|
| `0` | Success | - |
| `1` | General error | Maybe |
| `2` | Invalid arguments | Yes - fix args |
| `3` | Credentials not found | Yes - provide creds |
| `4` | Not a git repository | Yes - run in repo |
| `5` | Git repo has uncommitted changes | Yes - commit/stash |
| `6` | Copilot CLI not found | Yes - install Copilot |
| `7` | gh CLI not found or not authenticated | Yes - install/auth gh |
| `8` | Google API error | Maybe - check permissions |
| `9` | Template error | Yes - fix template |
| `10` | Git operation failed | Maybe |
| `11` | PR creation failed | Maybe - retry |
| `12` | User canceled | Yes - rerun |

### Appendix C: Dependencies

The current implementation relies primarily on the Go standard library and official Google/GitHub API clients.

### Appendix D: Template Files Summary

| File | Purpose | Embedded | Customizable |
|------|---------|----------|--------------|
| `templates/default.md` | Main technical plan | Yes | Yes |
| `templates/copilot-prompt.md` | Copilot CLI prompt | Yes | Yes |
| `templates/pr-body.md` | Pull request body | Yes | Yes |
| `templates/copilot-instructions.md` | Sample for `.github/` | No (docs only) | Yes |
| `templates/extraction-summary.json` | JSON schema reference | No (docs only) | No |

### Appendix E: Copilot CLI Reference

**Commands:**
- `copilot` - Start interactive session
- `copilot --prompt "..."` - Start with initial prompt
- `copilot --continue` - Resume last session
- `copilot --agent=NAME` - Use custom agent

**Slash Commands (in session):**
- `/add-dir PATH` - Trust additional directory
- `/cwd PATH` - Change working directory
- `/mcp add` - Add MCP server
- `/usage` - Show token usage
- `/feedback` - Submit feedback
- `/delegate PROMPT` - Push to Copilot coding agent

**File References:**
- `@path/to/file` - Include file contents in prompt
- Relative to current working directory

**Environment Variables:**
- `XDG_CONFIG_HOME` - Config directory (default: `~/.copilot`)
- `COPILOT_DEBUG` - Enable debug output
- `NO_COLOR` - Disable colored output

### Appendix F: CI/CD Considerations

**GitHub Actions Setup:**

```yaml
# .github/workflows/bau.yml
name: BAU Automation

on:
  workflow_dispatch:
    inputs:
      doc_url:
        description: 'Google Doc URL'
        required: true

jobs:
  apply-feedback:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Install BAU
        run: go install github.com/canonical/bau@latest
      
      - name: Setup credentials
        run: echo '${{ secrets.GOOGLE_CREDENTIALS }}' > /tmp/creds.json
        
      - name: Run BAU (extract and plan only)
        run: |
          bau --doc-url "${{ inputs.doc_url }}" \
              --credentials /tmp/creds.json \
              --skip-copilot \
              --skip-pr \
              --output ./bau-output
      
      # Note: Copilot CLI cannot run in CI (requires interactive session)
      # Users must run Copilot locally or use /delegate for GitHub-hosted agent
      
      - name: Upload plan artifact
        uses: actions/upload-artifact@v4
        with:
          name: technical-plan
          path: ./bau-output/
```

**Security Notes for CI:**
- Store `GOOGLE_CREDENTIALS` as repository secret
- Never log credential contents
- Use short-lived credentials if possible
- Consider using Workload Identity Federation for GCP

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-01-XX | BAU Team | Initial specification |
| 1.1 | 2025-01-XX | BAU Team | Added: Target file resolution, interactive mode details, branch strategy, POC cleanup integration, template specs, Copilot CLI configuration, comprehensive risks, progress reporting |
| 1.2 | 2025-01-XX | BAU Team | Added: Security Considerations section (credential security, API security, Git/GitHub security, process security, data security, security checklist), Learning Resources section (Go fundamentals, Cobra, Google APIs, go-github, GitHub CLI, Copilot CLI, testing, project structure, slog, embed, tools, books, videos) |

---

*End of Technical Specification*