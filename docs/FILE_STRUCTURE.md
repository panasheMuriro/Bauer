# Bauer PR Creation System - File Structure

This document describes all the files created for the Bauer draft PR creation system.

## Core Implementation Files

### `pr.go`
**Purpose**: GitHub PR creation and path resolution logic

**Key Components**:
- `GitHubClient` - Authenticates with GitHub API and manages requests
- `PathResolver` - Converts URL paths to repository file paths
- `CreateDraftPR()` - Main PR creation orchestration function
- `ExtractURLPath()` - Extracts path from full URLs
- `ApplySuggestionsToContent()` - Applies suggestions to file content
- Utility functions for GitHub API operations

**Key Functions**:
```
CreateGitHubClient(token) → *GitHubClient
ExtractURLPath(fullURL) → (string, error)
BuildPRMetadata(metadata, suggestions) → PRMetadata
CreateDraftPR(ctx, ghClient, owner, repo, output) → error
ApplySuggestionsToContent(content, suggestions) → string
```

**Dependencies**:
- `context` - For API call timeouts
- `net/http` - For GitHub API requests
- `encoding/json` - For JSON marshaling
- Standard library

---

### `workflow.go`
**Purpose**: High-level workflow functions and CLI integration

**Key Components**:
- `ProcessAndCreatePR()` - End-to-end workflow from Google Doc to PR
- `CreatePRFromJSON()` - Create PR from existing output.json
- Command-line flags setup

**Key Functions**:
```
ProcessAndCreatePR(ctx, googleDocURL, githubToken, shouldCreatePR) → error
CreatePRFromJSON(ctx, outputFile, githubToken) → error
```

**Use Cases**:
- One-shot: Google Doc → output.json → PR
- Two-step: Generate JSON, review, then create PR
- Batch processing: Multiple documents

---

### `main.go` (Existing)
**Purpose**: Entry point and Google Docs API integration

**Key Components**:
- Document extraction from Google Docs API
- Suggestion parsing and structuring
- Metadata table extraction
- Comment fetching from Google Drive API
- JSON output generation

**Unchanged**: Core logic remains the same; workflow functions added externally

---

## Documentation Files

### `QUICKSTART.md`
**Purpose**: Get started in 5 minutes

**Contents**:
1. Prerequisites checklist
2. Step-by-step setup (GitHub token, Google credentials)
3. Basic usage examples
4. Verification steps
5. Common workflows
6. Customization options
7. Troubleshooting guide
8. Security best practices

**Target Audience**: New users, quick reference

---

### `PR_CREATION_GUIDE.md`
**Purpose**: Comprehensive reference for PR creation system

**Contents**:
1. Features overview
2. Architecture and components
3. Setup instructions
4. Usage examples (3 scenarios)
5. Output format (full JSON example)
6. Suggestion types (insertion, deletion, style)
7. PR creation flow diagram
8. Path resolution examples
9. Anchor-based matching explanation
10. Error handling strategy
11. API rate limits
12. Contributing guidelines

**Target Audience**: Developers, maintainers

---

### `API_REFERENCE.md`
**Purpose**: Function-level documentation

**Contents**:
1. High-level functions (`ProcessAndCreatePR`, `CreatePRFromJSON`)
2. GitHub API functions (branch, file, commit, PR creation)
3. Path resolution functions
4. Metadata builders
5. GitHub client functions
6. Data structures and types
7. Error handling examples
8. Usage patterns
9. Logging configuration

**Format**: Function signature, parameters, return values, examples

**Target Audience**: Developers integrating the API

---

### `QUICKSTART.md` - Setup Guide
Includes:
- Environment setup instructions
- GitHub token generation (with screenshots)
- Google Cloud service account creation
- Document sharing
- Verification steps

---

## Test Files

### `pr_test.go`
**Purpose**: Unit tests and usage examples

**Contents**:
1. `TestPathResolution` - Tests URL → file path resolution
2. `TestURLPathExtraction` - Tests URL parsing
3. `TestBranchNameGeneration` - Tests branch naming
4. `TestSuggestionApplication` - Tests suggestion application
5. Example functions showing API usage

**Run Tests**:
```bash
go test ./... -v
```

---

## Example Files

### `examples.sh`
**Purpose**: Interactive examples and use cases

**Contents**:
- Interactive menu for different workflows
- Example 1: Generate output.json only
- Example 2: Generate JSON and create PR
- Example 3: Create PR from existing JSON
- Example 4: Custom repository example
- Environment variable setup help
- Colored output for clarity

**Run**:
```bash
bash examples.sh
```

---

## File Organization

```
Bauer/
├── main.go                  (Existing - Google Docs extraction)
├── pr.go                    (NEW - PR creation logic)
├── workflow.go              (NEW - High-level workflows)
├── pr_test.go              (NEW - Tests and examples)
├── QUICKSTART.md           (NEW - 5-minute setup)
├── API_REFERENCE.md        (NEW - Function documentation)
├── PR_CREATION_GUIDE.md    (NEW - Comprehensive guide)
├── examples.sh             (NEW - Interactive examples)
├── output.json             (Generated - Suggestions)
└── bau-test-creds.json     (Config - Service account)
```

---

## Function Call Graph

```
main()
  ↓
ProcessAndCreatePR()
  ├─ extractDocumentID()
  ├─ buildDocsService()
  ├─ buildDriveService()
  ├─ fetchDocumentContent()
  ├─ extractSuggestions()
  ├─ fetchComments()
  ├─ extractMetadataTable()
  ├─ buildDocumentStructure()
  ├─ buildActionableSuggestions()
  ├─ [Write output.json]
  └─ CreateDraftPR() [if shouldCreatePR]
       ├─ ExtractURLPath()
       ├─ ResolvePath()
       ├─ getDefaultBranch()
       ├─ getLatestCommit()
       ├─ createBranch()
       ├─ getFileContent()
       ├─ ApplySuggestionsToContent()
       ├─ createCommit()
       └─ createPR()
```

---

## Data Flow

```
Google Doc
    ↓
[extractDocumentID]
    ↓
[fetchDocumentContent]
    ↓
[extractSuggestions, extractMetadataTable, fetchComments]
    ↓
[buildActionableSuggestions]
    ↓
output.json
    ↓
[CreateDraftPR]
    ├─ [ExtractURLPath from metadata]
    ├─ [ResolvePath to file]
    ├─ [getFileContent]
    ├─ [ApplySuggestionsToContent]
    ├─ [createCommit]
    └─ [createPR]
    ↓
Draft PR on GitHub
```

---

## Key Design Decisions

### 1. Anchor-Based Matching
Suggestions are matched using surrounding text ("anchors") rather than line numbers. This makes them resilient to file changes.

**Benefits**:
- Works even if file has been modified
- Doesn't break on whitespace differences
- LLM-friendly for content matching

### 2. Path Resolution Strategy
Tries multiple file path variants in order:
1. `templates/segment/index.html`
2. `templates/segment.html`

**Benefits**:
- Flexible repository structures
- Graceful fallback
- Works with both index files and direct files

### 3. Draft PR by Default
All PRs are created as drafts.

**Benefits**:
- Safe for review before merging
- Prevents accidental merges
- Allows changes before marking ready

### 4. Separate Workflow Functions
`ProcessAndCreatePR()` handles the full workflow while `CreatePRFromJSON()` allows two-step processing.

**Benefits**:
- Flexibility for different use cases
- Allows reviewing JSON before PR creation
- Supports batch processing

### 5. Structured Logging
Uses `slog` for structured, JSON-compatible logging.

**Benefits**:
- Machine-readable output
- Levels (INFO, WARN, ERROR, DEBUG)
- Easy to parse and monitor

---

## Integration Points

### With GitHub
- Uses GitHub REST API v3
- Requires personal access token with `repo` scope
- Operates on any repository with proper access

### With Google Docs
- Uses Google Docs API v1 for document content
- Uses Google Drive API for comments
- Requires service account credentials

### With ubuntu.com Repository
- Resolves URL paths to file locations
- Applies changes while preserving HTML structure
- Creates PRs with descriptive commit messages

---

## Future Enhancements

### Phase 2
- [ ] Web UI for suggestion review
- [ ] Direct integration with ubuntu.com CI/CD
- [ ] Support for style changes application
- [ ] Batch processing of multiple documents
- [ ] PR status tracking and notifications

### Phase 3
- [ ] AI-powered suggestion validation
- [ ] Automated tests for PR changes
- [ ] Multi-repository support
- [ ] Scheduled document monitoring
- [ ] Review workflow integration

---

## Maintenance Notes

### When Updating
1. Keep `main.go` focused on Google Docs extraction
2. Keep `pr.go` focused on GitHub operations
3. Keep `workflow.go` as the orchestration layer
4. Update tests in `pr_test.go` when adding features
5. Update relevant documentation files

### Adding New Suggestion Types
1. Add case in `buildActionableSuggestions()` (main.go)
2. Implement logic in `ApplySuggestionsToContent()` (pr.go)
3. Add test in `pr_test.go`
4. Document in `API_REFERENCE.md`

### Adding New Configuration
1. Add flag in `workflow.go`
2. Pass through function parameters
3. Document in `QUICKSTART.md`
4. Add example in `examples.sh`
