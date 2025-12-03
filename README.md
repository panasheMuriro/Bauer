# Google Docs Content & Feedback Extraction POC

A proof-of-concept Go application that extracts document content, suggestions (proposed edits), and comments from Google Docs using the Google Docs API and Google Drive API.

## Table of Contents

- [Purpose](#purpose)
- [Architecture Overview](#architecture-overview)
- [API Endpoints Used](#api-endpoints-used)
- [Authentication](#authentication)
- [Data Flow](#data-flow)
- [Code Structure](#code-structure)
- [Data Models](#data-models)
- [Key Implementation Details](#key-implementation-details)
- [Configuration](#configuration)
- [Usage](#usage)
- [Output Format](#output-format)
- [Limitations & Known Issues](#limitations--known-issues)
- [Future Improvements](#future-improvements)

---

## Purpose

This POC demonstrates how to:

1. **Extract document content** from Google Docs including text within nested structures (tables, lists, headers, footers)
2. **Extract suggestions** (pending insertions, deletions, and style changes) with their exact positions in the document
3. **Extract comments** and their replies, including the exact text each comment references
4. **Map feedback to content** by correlating suggestion positions and comment anchors with document text

### Use Case

This tool is designed for workflows where document feedback (suggestions and comments) needs to be programmatically extracted and analyzed, such as:
- Content review pipelines
- Document approval workflows
- Feedback aggregation and reporting

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        main.go                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │ Auth Layer   │    │ Docs API     │    │ Drive API    │       │
│  │              │    │ Client       │    │ Client       │       │
│  │ - JWT Config │    │              │    │              │       │
│  │ - Delegation │    │ - Fetch Doc  │    │ - Fetch      │       │
│  └──────┬───────┘    │ - Inline     │    │   Comments   │       │
│         │            │   Suggestions│    │ - Pagination │       │
│         │            └──────┬───────┘    └──────┬───────┘       │
│         │                   │                   │                │
│         └───────────────────┴───────────────────┘                │
│                             │                                    │
│                             ▼                                    │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Extraction Layer                       │   │
│  │                                                           │   │
│  │  ┌─────────────────┐  ┌─────────────────┐                │   │
│  │  │ extractSuggest- │  │ fetchComments   │                │   │
│  │  │ ions()          │  │ ()              │                │   │
│  │  │                 │  │                 │                │   │
│  │  │ Recursive tree  │  │ Paginated fetch │                │   │
│  │  │ traversal of:   │  │ with fields     │                │   │
│  │  │ - Body          │  │ selection       │                │   │
│  │  │ - Tables        │  │                 │                │   │
│  │  │ - Headers       │  │                 │                │   │
│  │  │ - Footers       │  │                 │                │   │
│  │  └─────────────────┘  └─────────────────┘                │   │
│  └──────────────────────────────────────────────────────────┘   │
│                             │                                    │
│                             ▼                                    │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Output (JSON)                          │   │
│  │  - Document metadata                                      │   │
│  │  - Suggestions with positions                             │   │
│  │  - Comments with quoted content                           │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## API Endpoints Used

### Google Docs API v1

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `documents.get` | GET | Fetch document content with suggestions inline |

**Request Parameters:**
- `documentId`: The ID of the document to retrieve
- `suggestionsViewMode`: Set to `SUGGESTIONS_INLINE` to include suggestion metadata in the response

**API Reference:** https://developers.google.com/docs/api/reference/rest/v1/documents/get

**Why this endpoint?**
- The Docs API is the only way to get the full document structure with suggestion IDs
- `SUGGESTIONS_INLINE` mode marks text with `suggestedInsertionIds` and `suggestedDeletionIds` arrays
- Provides `startIndex` and `endIndex` for exact positioning of each element

### Google Drive API v3

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `comments.list` | GET | Fetch all comments and replies on a file |

**Request Parameters:**
- `fileId`: The document ID (same as Docs API)
- `fields`: Selective field mask for response optimization
- `pageToken`: For pagination through large comment sets

**Fields Requested:**
```
nextPageToken,
comments(
  id,
  author(displayName, emailAddress),
  content,
  quotedFileContent,
  createdTime,
  modifiedTime,
  resolved,
  replies(id, author(displayName, emailAddress), content, createdTime),
  anchor
)
```

**API Reference:** https://developers.google.com/drive/api/reference/rest/v3/comments/list

**Why this endpoint?**
- The Docs API does not expose comments; they are stored as Drive file metadata
- `quotedFileContent.value` provides the exact text a comment is attached to
- `anchor` contains JSON with positional information (alternative to quoted content)

---

## Authentication

### Service Account Authentication

The application uses Google Cloud service account credentials with JWT (JSON Web Token) authentication.

**Credential Flow:**
```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ Credentials JSON│────▶│ JWT Config      │────▶│ OAuth2 Client   │
│ (service acct)  │     │ (with scopes)   │     │ (HTTP client)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### OAuth2 Scopes Required

| Scope | Purpose |
|-------|---------|
| `https://www.googleapis.com/auth/documents.readonly` | Read document content and structure |
| `https://www.googleapis.com/auth/drive.readonly` | Read comments and file metadata |

### Authentication Modes

The application supports two authentication modes controlled by `useDelegation`:

#### 1. Direct Service Account Access (`useDelegation = false`)

- Service account accesses documents directly
- Document must be explicitly shared with the service account email
- Simpler setup, no admin involvement required
- **Use when:** Testing, personal documents, or documents you can share

#### 2. Domain-Wide Delegation (`useDelegation = true`)

- Service account impersonates a user via `config.Subject`
- Requires Google Workspace admin to authorize scopes
- Can access any document the impersonated user can access
- **Use when:** Production environments accessing organizational documents

**Delegation Setup (for mode 2):**
1. Google Workspace Admin Console → Security → API Controls → Domain-wide Delegation
2. Add the service account client ID
3. Authorize the required scopes
4. Set `delegationEmail` to a user who has document access

---

## Data Flow

### Step-by-Step Execution Flow

```
1. INITIALIZATION
   │
   ├─▶ Parse document ID from URL (regex extraction)
   │
   ├─▶ Build Docs API service (JWT auth)
   │
   └─▶ Build Drive API service (JWT auth)

2. DOCUMENT FETCH
   │
   ├─▶ Call Docs API: documents.get with SUGGESTIONS_INLINE
   │
   └─▶ Receive full document structure with suggestion markers

3. SUGGESTION EXTRACTION
   │
   ├─▶ Recursively traverse document tree:
   │   │
   │   ├─▶ Body → StructuralElements
   │   │         ├─▶ Paragraphs → ParagraphElements → TextRuns
   │   │         └─▶ Tables → TableRows → TableCells → (recurse)
   │   │
   │   ├─▶ Headers → StructuralElements → (same as body)
   │   │
   │   └─▶ Footers → StructuralElements → (same as body)
   │
   └─▶ Collect all TextRuns with suggestedInsertionIds/suggestedDeletionIds

4. COMMENT FETCH
   │
   ├─▶ Call Drive API: comments.list with pagination
   │
   ├─▶ For each comment:
   │   ├─▶ Extract author, content, timestamps
   │   ├─▶ Extract quotedFileContent (the referenced text)
   │   └─▶ Extract replies with their metadata
   │
   └─▶ Continue until no more pageTokens

5. OUTPUT
   │
   ├─▶ Print full API responses (debug)
   │
   ├─▶ Print extracted suggestions with positions
   │
   ├─▶ Print comments with quoted content
   │
   └─▶ Print consolidated summary JSON
```

---

## Code Structure

### Current File: `main.go`

| Section | Lines | Description |
|---------|-------|-------------|
| **Constants** | 19-30 | Configuration: doc URL, delegation email, delegation flag |
| **Data Types** | 32-63 | `Suggestion`, `Comment`, `Reply` structs |
| **Auth Functions** | 65-140 | `buildDocsService()`, `buildDriveService()` |
| **URL Parsing** | 142-151 | `extractDocumentID()` |
| **Docs API** | 153-164 | `fetchDocumentContent()` |
| **Suggestion Extraction** | 166-301 | `extractSuggestions()` with recursive traversal |
| **Debug Utilities** | 303-336 | `findAllSuggestionsRaw()` |
| **Text Extraction** | 338-355 | `extractPlainText()` |
| **Comments Extraction** | 357-422 | `fetchComments()` with pagination |
| **Main Entry Point** | 424-612 | Orchestration and output |

### Key Functions

#### `buildDocsService(ctx, email, delegate) (*docs.Service, error)`

Creates an authenticated Google Docs API client.

**Parameters:**
- `ctx`: Context for cancellation/timeout
- `email`: Email to impersonate (if delegation enabled)
- `delegate`: Whether to use domain-wide delegation

**Returns:** Configured Docs API service or error

---

#### `buildDriveService(ctx, email, delegate) (*drive.Service, error)`

Creates an authenticated Google Drive API client.

**Parameters:** Same as `buildDocsService`

**Returns:** Configured Drive API service or error

---

#### `extractDocumentID(url) (string, error)`

Extracts the document ID from a Google Docs URL.

**Input:** `https://docs.google.com/document/d/{ID}/edit`

**Output:** `{ID}`

**Regex Pattern:** `/document/d/([a-zA-Z0-9_-]+)`

---

#### `fetchDocumentContent(ctx, service, docID) (*docs.Document, error)`

Fetches document with suggestions inline.

**API Call:**
```go
service.Documents.Get(docID).
    SuggestionsViewMode("SUGGESTIONS_INLINE").
    Context(ctx).
    Do()
```

**Why `SUGGESTIONS_INLINE`?**
- Default mode hides suggestions
- `PREVIEW_ACCEPT_ALL` shows document as if all suggestions were accepted
- `PREVIEW_REJECT_ALL` shows document as if all suggestions were rejected
- `SUGGESTIONS_INLINE` includes suggestion markers in the content

---

#### `extractSuggestions(doc) []Suggestion`

Recursively traverses the document structure to find all suggestions.

**Traversal Pattern:**
```
Document
├── Body.Content[]
│   └── StructuralElement
│       ├── Paragraph
│       │   └── Elements[]
│       │       └── TextRun ← CHECK FOR SUGGESTIONS HERE
│       └── Table
│           └── TableRows[]
│               └── TableCells[]
│                   └── Content[] ← RECURSE
├── Headers{}
│   └── Content[] ← SAME PATTERN
└── Footers{}
    └── Content[] ← SAME PATTERN
```

**What's Extracted:**
- `suggestedInsertionIds[]` → Type: "insertion"
- `suggestedDeletionIds[]` → Type: "deletion"
- `suggestedTextStyleChanges{}` → Type: "text_style_change"

---

#### `fetchComments(ctx, service, docID) ([]Comment, error)`

Fetches all comments with pagination.

**Pagination Logic:**
```go
for {
    resp := service.Comments.List(docID)...Do()
    // Process comments
    if resp.NextPageToken == "" {
        break
    }
    pageToken = resp.NextPageToken
}
```

**Key Field:** `quotedFileContent.Value` contains the exact text the comment references.

---

## Data Models

### Suggestion

```go
type Suggestion struct {
    ID           string `json:"id"`            // e.g., "suggest.kh2mgxr2w2dl"
    Type         string `json:"type"`          // "insertion", "deletion", "text_style_change"
    Content      string `json:"content"`       // The suggested text
    StartIndex   int64  `json:"start_index"`   // Character position (0-based)
    EndIndex     int64  `json:"end_index"`     // End position (exclusive)
    SurroundText string `json:"surround_text"` // Context (truncated to 100 chars)
}
```

### Comment

```go
type Comment struct {
    ID              string   `json:"id"`              // Drive comment ID
    Author          string   `json:"author"`          // Display name
    AuthorEmail     string   `json:"author_email"`    // Email address
    Content         string   `json:"content"`         // Comment text
    QuotedContent   string   `json:"quoted_content"`  // Text being commented on
    CreatedTime     string   `json:"created_time"`    // ISO 8601 timestamp
    ModifiedTime    string   `json:"modified_time"`   // Last modified
    Resolved        bool     `json:"resolved"`        // Resolution status
    Replies         []Reply  `json:"replies"`         // Thread replies
    MentionedEmails []string `json:"mentioned_emails"` // @mentions
}
```

### Reply

```go
type Reply struct {
    ID          string `json:"id"`
    Author      string `json:"author"`
    AuthorEmail string `json:"author_email"`
    Content     string `json:"content"`
    CreatedTime string `json:"created_time"`
}
```

---

## Key Implementation Details

### Why Two APIs?

| Data | API | Reason |
|------|-----|--------|
| Document content | Docs API | Only source for document structure |
| Suggestions | Docs API | Embedded in document content with `SUGGESTIONS_INLINE` |
| Comments | Drive API | Comments are Drive file metadata, not part of doc content |

### Document Structure Traversal

Google Docs documents have a tree structure:

```
Document
└── Body
    └── Content[] (StructuralElement)
        ├── Paragraph
        │   ├── Elements[] (ParagraphElement)
        │   │   ├── TextRun (text + formatting + suggestions)
        │   │   ├── InlineObjectElement
        │   │   └── ...
        │   └── ParagraphStyle
        ├── Table
        │   └── TableRows[]
        │       └── TableCells[]
        │           └── Content[] (StructuralElement) ← NESTED!
        ├── SectionBreak
        └── TableOfContents
            └── Content[] (StructuralElement) ← NESTED!
```

**Important:** Tables and TOCs contain nested `StructuralElement` arrays, requiring recursive traversal.

### Suggestion Position Mapping

Each `ParagraphElement` has `startIndex` and `endIndex`:

```json
{
  "startIndex": 1435,
  "endIndex": 1443,
  "textRun": {
    "content": " forever",
    "suggestedInsertionIds": ["suggest.kh2mgxr2w2dl"]
  }
}
```

These indices are **character positions** in the document (0-based), allowing precise mapping to document content.

### Comment Anchor Resolution

Comments reference document text via `quotedFileContent`:

```json
{
  "id": "AAABw4lOGC8",
  "content": "Justin who?",
  "quotedFileContent": {
    "value": "Justin Rigling"
  }
}
```

The `anchor` field contains JSON with more detailed position info but `quotedFileContent.value` is simpler for most use cases.

---

## Configuration

### Constants in `main.go`

| Constant | Description | Example |
|----------|-------------|---------|
| `googleDocURL` | Target document URL | `https://docs.google.com/document/d/.../edit` |
| `delegationEmail` | Email for impersonation | `user@domain.com` |
| `useDelegation` | Enable domain-wide delegation | `false` |

### Credentials File

**Path:** `bau-test-creds.json` (same directory as `main.go`)

**Format:** Google Cloud service account JSON key

**Required Fields:**
- `type`: "service_account"
- `project_id`: GCP project ID
- `private_key`: RSA private key
- `client_email`: Service account email

---

## Usage

### Prerequisites

1. Go 1.21+ installed
2. Service account credentials JSON
3. Document shared with service account (if `useDelegation = false`)
4. APIs enabled in GCP: Google Docs API, Google Drive API

### Running

```bash
cd projects/bau
go mod tidy
go run main.go
```

### Output

The program outputs:
1. JSON-formatted log messages (via `slog`)
2. Full raw API responses (for debugging)
3. Extracted suggestions array
4. Extracted comments array
5. Consolidated summary JSON

---

## Output Format

### Actionable Suggestions (Machine-Readable)

The primary output is a JSON array of **actionable suggestions** designed for LLM consumption. The format is optimized for an LLM to read this output and apply the changes to an HTML file.

#### How Anchor Text is Computed (Not from API)

**Important**: The `anchor.preceding_text` and `anchor.following_text` fields are **NOT provided by the Google APIs**. They are computed by this application through the following process:

1. **API provides positions**: The Google Docs API provides `startIndex` and `endIndex` for each `ParagraphElement` (character positions in the document).

2. **We build a text position map**: The `buildDocumentStructure()` function traverses the entire document and collects all text elements with their positions into `TextElementWithPosition` structs.

3. **We compute surrounding text**: The `getTextAround()` function:
   - Iterates through all collected text elements
   - Finds elements that END before the suggestion's `startIndex` → concatenates them for `preceding_text`
   - Finds elements that START after the suggestion's `endIndex` → concatenates them for `following_text`
   - Takes the last/first 80 characters closest to the suggestion point

```
Document Text Elements (from API):
[elem1: "Hello "][elem2: "world"][elem3: " how"][elem4: " are you"]
  0-6            6-11           11-15          15-23

Suggestion at position 11-15 (suggesting change to " how"):
  preceding_text = "Hello world"  (text from elements ending before 11)
  following_text = " are you"     (text from elements starting after 15)
```

This approach ensures the anchor text is:
- **Exact**: No truncation markers like "..."
- **Contextual**: Enough surrounding text (80 chars) to uniquely identify the location
- **Format-agnostic**: Works whether applying to Google Docs, HTML, or plain text

#### Why This Format is Machine-Readable

| Aspect | Design Choice | Why It Matters |
|--------|---------------|----------------|
| **No truncation** | Anchor texts are exact (80 chars), not truncated with "..." | LLM can do exact string matching |
| **Explicit operations** | `change.type` is "insert", "delete", or "style" | Clear instruction, no ambiguity |
| **Verification data** | `text_before_change` and `text_after_change` | LLM can verify the change was applied correctly |
| **Anchor-based location** | `preceding_text` + `following_text` | Works across document formats (Google Docs → HTML) |
| **Separate concerns** | `anchor` (for finding), `change` (the edit), `location` (metadata) | Each field has one purpose |

#### How an LLM Should Apply Suggestions

1. **Find the location**: Search for `anchor.preceding_text` + `change.original_text` + `anchor.following_text`
2. **Apply the change**: Replace `change.original_text` with `change.new_text`
3. **Verify**: Confirm the result matches `verification.text_after_change`

### ActionableSuggestion JSON Structure

```json
{
  "id": "suggest.kh2mgxr2w2dl",
  "anchor": {
    "preceding_text": "h includes expanded security, compliance and AWS integration:\nUbuntu\nFree to use",
    "following_text": "\n\nAWS-optimised kernel and userspace components built from the latest releases\nB"
  },
  "change": {
    "type": "insert",
    "original_text": "",
    "new_text": " forever"
  },
  "verification": {
    "text_before_change": "...Ubuntu\nFree to use\n\nAWS-optimised...",
    "text_after_change": "...Ubuntu\nFree to use forever\n\nAWS-optimised..."
  },
  "location": {
    "section": "Body",
    "parent_heading": "Choose the right Ubuntu for you",
    "heading_level": 2,
    "in_table": true,
    "table": {
      "table_index": 3,
      "row_index": 1,
      "column_index": 1,
      "column_header": "Ubuntu",
      "row_header": "Ubuntu"
    },
    "in_metadata": false
  },
  "position": {
    "start_index": 1435,
    "end_index": 1443
  }
}
```

### Field Reference

#### Anchor Fields (for finding location)

| Field | Description |
|-------|-------------|
| `anchor.preceding_text` | Exact text (80 chars) immediately before the change point |
| `anchor.following_text` | Exact text (80 chars) immediately after the change point |

#### Change Fields (the edit to make)

| Field | Description |
|-------|-------------|
| `change.type` | Operation: `insert`, `delete`, or `style` |
| `change.original_text` | Text to find/remove (empty for insertions) |
| `change.new_text` | Text to insert (empty for deletions) |

#### Verification Fields (for validation)

| Field | Description |
|-------|-------------|
| `verification.text_before_change` | What the text looks like before applying |
| `verification.text_after_change` | What the text should look like after applying |

#### Location Fields (metadata, not for finding)

| Field | Description |
|-------|-------------|
| `location.section` | Document section: "Body", "Header", or "Footer" |
| `location.parent_heading` | Nearest heading above the suggestion |
| `location.in_table` | Whether suggestion is inside a table |
| `location.table.*` | Table position details (index, row, column) |
| `location.in_metadata` | Whether in the metadata table |

#### Position Fields (reference only)

| Field | Description |
|-------|-------------|
| `position.start_index` | Character position in Google Doc (0-based) |
| `position.end_index` | End character position |

### Summary JSON Structure

```json
{
  "document_title": "Document Title",
  "document_id": "1vMw8X7Y8...",
  "content_length": 279,
  "metadata": {
    "raw": {"Page title": "Ubuntu on AWS", ...},
    "page_title": "Ubuntu on AWS",
    "page_description": "...",
    "table_start_index": 19,
    "table_end_index": 762
  },
  "suggestion_count": 2,
  "comment_count": 1,
  "suggestions": [...],
  "comments": [...]
}
```

---

## Limitations & Known Issues

1. **No inline object extraction**: Images, drawings, and embedded objects are not extracted
2. **No formatting extraction**: Text styles (bold, italic, etc.) are not included in output
3. **Hardcoded credentials path**: `bau-test-creds.json` must be in working directory
4. **No error retry logic**: API failures are not retried
5. **Full document in memory**: Large documents may cause memory issues
6. **Comment mentioned emails**: The `mentionedEmailAddresses` field is not populated by the Drive API for comments (only available in some contexts)

---

## How Actionable Suggestions Work

### Document Structure Analysis

The POC builds a complete map of the document structure:

1. **Headings**: All H1-H6 headings with their positions
2. **Tables**: All tables with row/column/cell positions
3. **Text Elements**: All text with character positions

### Context Resolution

For each suggestion found, the system:

1. **Finds parent heading**: Looks for the nearest heading *before* the suggestion position
2. **Detects table location**: Checks if position falls within any table's range, then finds row/column
3. **Extracts surrounding text**: Gets ~50 characters before and after from adjacent text elements
4. **Generates action description**: Creates human-readable instruction based on suggestion type

### Suggestion Types

| Type | Marker | Description |
|------|--------|-------------|
| `insertion` | `[INSERT: X]` | New text to add |
| `deletion` | `[DELETE: X]` | Existing text to remove |
| `text_style_change` | `[STYLE CHANGE: X]` | Formatting change |

---

## Future Improvements

### Testing Plan

A comprehensive testing strategy should be implemented:

#### Unit Tests

| Function | Test Cases |
|----------|------------|
| `extractDocumentID()` | Valid URLs, invalid URLs, edge cases (no ID, malformed) |
| `extractSuggestions()` | Documents with: no suggestions, insertions only, deletions only, mixed, nested in tables |
| `extractMetadataTable()` | Documents with: no table, valid metadata table, malformed table |
| `buildDocumentStructure()` | Documents with: headings at all levels, nested tables, empty sections |
| `findParentHeading()` | Position before any heading, between headings, after all headings |
| `findTableLocation()` | Position outside tables, in various cells, in nested tables |
| `getTextAround()` | Normal case, edge of document, empty surrounding text |
| `buildActionableSuggestions()` | All suggestion types, metadata table suggestions, table suggestions |

#### Integration Tests

| Test Scenario | Description |
|---------------|-------------|
| Full extraction flow | End-to-end test with a known test document |
| API error handling | Mock API failures, rate limits, auth errors |
| Large documents | Performance testing with documents >100 pages |
| Complex structures | Documents with nested tables, multiple heading levels |

#### Test Documents

Create a set of test Google Docs with known content:

1. **`test-simple.gdoc`**: Plain text with 2-3 simple suggestions
2. **`test-tables.gdoc`**: Suggestions inside tables at various positions
3. **`test-metadata.gdoc`**: Document with metadata table and suggestions in it
4. **`test-complex.gdoc`**: All suggestion types, nested structures, many headings
5. **`test-empty.gdoc`**: Document with no suggestions (verify graceful handling)

#### Verification Tests

For each suggestion type, verify:

```go
// Pseudo-test structure
func TestInsertionSuggestion(t *testing.T) {
    // 1. Apply suggestion to original HTML
    original := loadHTML("test.html")
    result := applyChange(original, suggestion.Anchor, suggestion.Change)
    
    // 2. Verify result matches expected
    assert.Contains(t, result, suggestion.Verification.TextAfterChange)
    
    // 3. Verify anchor text was found exactly once
    assert.Equal(t, 1, countOccurrences(original, suggestion.Anchor.PrecedingText))
}
```

#### CI/CD Integration

```yaml
# .github/workflows/test.yml
jobs:
  test:
    steps:
      - run: go test ./... -v -cover
      - run: go test -race ./...  # Race condition detection
      - run: golangci-lint run    # Linting
```

### Remove Hardcoded Configuration

The following values are currently hardcoded and should be externalized:

| Hardcoded Value | Current Location | Recommended Solution |
|-----------------|------------------|---------------------|
| `googleDocURL` | `main.go:21` | CLI flag: `--doc-url` or env var `DOC_URL` |
| `delegationEmail` | `main.go:26` | CLI flag: `--email` or env var `DELEGATION_EMAIL` |
| `useDelegation` | `main.go:30` | CLI flag: `--use-delegation` (boolean) |
| Credentials path | `buildDocsService()`, `buildDriveService()` | CLI flag: `--credentials` or env var `GOOGLE_CREDENTIALS_PATH` |
| Anchor length (80) | `buildActionableSuggestions()` | Config file or CLI flag `--anchor-length` |

**Recommended Implementation:**

```go
// config.go
type Config struct {
    DocumentURL     string `env:"DOC_URL" flag:"doc-url"`
    DelegationEmail string `env:"DELEGATION_EMAIL" flag:"email"`
    UseDelegation   bool   `env:"USE_DELEGATION" flag:"use-delegation"`
    CredentialsPath string `env:"GOOGLE_CREDENTIALS_PATH" flag:"credentials"`
    AnchorLength    int    `env:"ANCHOR_LENGTH" flag:"anchor-length" default:"80"`
}
```

### Proposed File Structure

```
projects/bau/
├── main.go              # Entry point only
├── config/
│   └── config.go        # Configuration loading (flags, env vars)
├── auth/
│   └── auth.go          # Service account authentication
├── docs/
│   ├── client.go        # Docs API client wrapper
│   ├── suggestions.go   # Suggestion extraction
│   ├── structure.go     # Document structure analysis
│   └── content.go       # Content/text extraction
├── drive/
│   ├── client.go        # Drive API client wrapper
│   └── comments.go      # Comment extraction
├── models/
│   └── models.go        # Data types (Suggestion, Comment, ActionableSuggestion, etc.)
├── output/
│   └── output.go        # JSON formatting
└── README.md
```

### Enhancement Ideas

1. **CLI interface**: Use `cobra` or `flag` for argument parsing
2. **Environment variables**: Support 12-factor app configuration
3. **Config file**: Support YAML/JSON config for complex setups
4. **Batch processing**: Accept multiple document URLs from stdin or file
5. **Output to file**: `--output` flag to write JSON to file instead of stdout
6. **Filter options**: `--type=insertion,deletion` to filter suggestion types
7. **Rate limiting**: Handle API quota with exponential backoff
8. **Caching**: Cache document content to reduce API calls
9. **Comment context**: Apply same anchor-based location to comments
10. **HTML output mode**: Generate suggestions pre-formatted for HTML application
11. **Suggestion deduplication**: Group duplicate suggestion IDs that span multiple text runs
12. **Diff output**: Generate unified diff format for easier review