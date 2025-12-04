# Bauer - Content Review PR Generator

Bauer is a tool that extracts suggestions from Google Docs and automatically creates draft pull requests on the ubuntu.com repository.

## Features

- **Extract Suggestions**: Reads suggestions (insertions, deletions, style changes) from a Google Doc
- **Generate Metadata**: Captures page title, description, and other metadata from the document
- **Create Draft PRs**: Automatically creates draft pull requests on github.com/canonical/ubuntu.com
- **Path Resolution**: Intelligently resolves URL paths to file locations in the repository
  - `/aws` → `templates/aws.html` or `templates/aws/index.html`
  - `/cloud/azure` → `templates/cloud/azure.html` or `templates/cloud/azure/index.html`

## Architecture

### Main Components

1. **main.go** - Entry point and core suggestion extraction logic
   - Google Docs API integration
   - Suggestion extraction and parsing
   - Comment fetching from Google Drive API
   - Metadata table extraction

2. **pr.go** - GitHub PR creation logic
   - `GitHubClient` - Handles all GitHub API interactions
   - `PathResolver` - Converts URL paths to file paths
   - `CreateDraftPR` - Main PR creation orchestration

3. **workflow.go** - High-level workflow functions
   - `ProcessAndCreatePR` - End-to-end workflow
   - `CreatePRFromJSON` - Create PR from existing output.json

## Setup

### Prerequisites

- Go 1.21+
- GitHub Personal Access Token (for PR creation)
- Google Service Account credentials (for document access)

### Google Cloud Setup

1. Create a Google Cloud project
2. Enable Google Docs API and Google Drive API
3. Create a service account
4. Download service account credentials as JSON
5. Save as `bau-test-creds.json` in the Bauer directory

### GitHub Setup

1. Create a GitHub Personal Access Token with `repo` scope
2. Set via environment variable: `export GITHUB_TOKEN=ghp_xxxxx`

## Usage

### Option 1: Generate output.json only

```bash
go run . -help
```

This extracts suggestions from the hardcoded Google Doc and creates `output.json`.

### Option 2: Generate output.json AND create a draft PR

```bash
GITHUB_TOKEN=ghp_xxxxx go run . \
  --create-pr \
  --github-token=$GITHUB_TOKEN \
  --repo-owner=canonical \
  --repo-name=ubuntu.com
```

### Option 3: Create PR from existing output.json

```bash
go run . \
  -create-pr-from-json \
  -github-token=$GITHUB_TOKEN \
  -output-file=output.json
```

## Output Format

The tool generates `output.json` with the following structure:

```json
{
  "document_title": "index.html - \"Ubuntu on AWS\"",
  "document_id": "1vMw8X7Y8yoiubnUEwt6o9f-s0YK5F6J4I1hQ6tevVqg",
  "metadata": {
    "raw": {
      "Page title (60 characters max)": "Ubuntu on AWS",
      "Page description (160 characters max)": "..."
    },
    "page_title": "Ubuntu on AWS",
    "page_description": "...",
    "table_start_index": 19,
    "table_end_index": 762
  },
  "actionable_suggestions": [
    {
      "id": "suggest.o5qxqtvtbdfb",
      "anchor": {
        "preceding_text": "...",
        "following_text": "..."
      },
      "change": {
        "type": "delete",
        "original_text": "Acquia"
      },
      "verification": {
        "text_before_change": "...",
        "text_after_change": "..."
      },
      "location": {
        "section": "Body",
        "parent_heading": "UBUNTU ON AWS POWERS CLOUD LEADERS",
        "heading_level": 5,
        "in_table": false,
        "in_metadata": false
      },
      "position": {
        "start_index": 1245,
        "end_index": 1251
      }
    }
  ],
  "comments": [...]
}
```

## Suggestion Types

### Insertion
```json
{
  "change": {
    "type": "insert",
    "new_text": " forever"
  }
}
```

### Deletion
```json
{
  "change": {
    "type": "delete",
    "original_text": "Acquia"
  }
}
```

### Style Change
```json
{
  "change": {
    "type": "style",
    "original_text": "Optimise",
    "new_text": "Optimise"
  }
}
```

## PR Creation Flow

When `--create-pr` is enabled:

1. **Extract URL** from metadata (`*Current or suggested page URL`)
2. **Resolve file paths** using PathResolver
   - Tries both `.html` and `/index.html` variants
3. **Create branch** named `content/{page-title}-{timestamp}`
4. **Get file content** from the resolved path
5. **Apply suggestions** to the file content
6. **Create commit** with changes
7. **Create draft PR** on the specified repository

## Path Resolution Examples

| URL | Resolved Paths |
|-----|-----------------|
| `/aws` | `templates/aws.html`, `templates/aws/index.html` |
| `/cloud/azure` | `templates/cloud/azure.html`, `templates/cloud/azure/index.html` |
| `/` | `templates/index.html` |

The tool tries each path in order and uses the first one that exists.

## Anchor-based Matching

Suggestions use "anchor" texts (surrounding content) for reliable matching in HTML files:

```go
// Search for this pattern
precedingText + originalText + followingText

// Replace with
precedingText + newText + followingText
```

This ensures suggestions are applied to the correct location even if the file has been modified since the suggestion was created.

## Error Handling

The tool provides detailed logging at each step:

- ERROR: Critical failures (missing credentials, API errors)
- WARN: Non-critical issues (missing comments, file not found, retry next path)
- INFO: Progress information (document fetched, PR created, etc.)
- DEBUG: Detailed internal state

## API Rate Limits

- GitHub API: 5,000 requests per hour (with token)
- Google Docs API: Subject to Google Cloud quota
- Google Drive API: Subject to Google Cloud quota

## Contributing

When adding new suggestion types:

1. Update `Suggestion.Type` in `main.go`
2. Handle in `buildActionableSuggestions()` in `main.go`
3. Implement application logic in `ApplySuggestionsToContent()` in `pr.go`
4. Add tests for the new type

## Troubleshooting

### "No suggestions found"
- Check that the Google Doc has suggestions (Ctrl+Alt+O in Google Docs)
- Ensure service account has access to the document

### "Failed to create branch"
- Token may not have `repo` scope
- Branch may already exist

### "File not found"
- Check the resolved path matches actual file location
- Verify URL path in metadata is correct

### "Failed to authenticate"
- Check service account credentials file exists
- Verify GITHUB_TOKEN environment variable is set
- Ensure token has not expired

## License

Proprietary - Canonical Ltd
