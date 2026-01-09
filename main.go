package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Hardcoded Google Doc URL for POC
// Format: https://docs.google.com/document/d/{documentId}/edit
const googleDocURL = ""

// Hardcoded email for credentials delegation
// This email is used to delegate service account credentials to access the document
const delegationEmail = ""

// Set to false to use direct service account access (document must be shared with service account)
// Set to true to use domain-wide delegation (requires admin setup)
const useDelegation = false

// Suggestion represents a suggestion (insertion or deletion) in the document.
// This is the raw suggestion data extracted from the Google Docs API.
// The API provides suggestions as part of TextRun elements with suggestedInsertionIds
// or suggestedDeletionIds arrays. The StartIndex and EndIndex are character positions
// provided by the API for each ParagraphElement.
type Suggestion struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // "insertion", "deletion", or "text_style_change"
	Content    string `json:"content"`
	StartIndex int64  `json:"start_index"`
	EndIndex   int64  `json:"end_index"`
}

// DocumentHeading represents a heading in the document with its position.
// Used to determine which section a suggestion belongs to.
type DocumentHeading struct {
	Text       string `json:"text"`
	Level      int    `json:"level"` // 1-6 for HEADING_1 through HEADING_6
	StartIndex int64  `json:"start_index"`
	EndIndex   int64  `json:"end_index"`
}

// TableLocation describes where within a table a suggestion is located
type TableLocation struct {
	TableIndex   int    `json:"table_index"`   // Which table (1-based)
	RowIndex     int    `json:"row_index"`     // Row number (1-based)
	ColumnIndex  int    `json:"column_index"`  // Column number (1-based)
	ColumnHeader string `json:"column_header"` // Header of this column if available
	RowHeader    string `json:"row_header"`    // First cell of this row if available
}

// SuggestionLocation provides context about where in the document a suggestion is located.
// This is metadata for verification, not for finding the text.
type SuggestionLocation struct {
	Section       string         `json:"section"`                  // "Body", "Header", "Footer"
	ParentHeading string         `json:"parent_heading,omitempty"` // Nearest heading above
	HeadingLevel  int            `json:"heading_level,omitempty"`  // Level of parent heading (1-6)
	InTable       bool           `json:"in_table"`
	Table         *TableLocation `json:"table,omitempty"` // Table details if in a table
	InMetadata    bool           `json:"in_metadata"`     // True if in the metadata table
}

// SuggestionAnchor contains the exact text before and after a suggestion.
// Used by LLMs to locate where to apply the change in HTML/text content.
// These are NOT truncated - they contain enough context to uniquely identify the location.
type SuggestionAnchor struct {
	// PrecedingText is the exact text immediately before the suggestion point.
	// For insertions: text before where new content should be inserted.
	// For deletions: text before the content to be deleted.
	PrecedingText string `json:"preceding_text"`

	// FollowingText is the exact text immediately after the suggestion point.
	// For insertions: text after where new content should be inserted.
	// For deletions: text after the content to be deleted.
	FollowingText string `json:"following_text"`
}

// SuggestionChange describes exactly what text change should be made.
type SuggestionChange struct {
	// Type is the operation: "insert", "delete", or "replace"
	Type string `json:"type"`

	// OriginalText is the text currently in the document (empty for pure insertions)
	OriginalText string `json:"original_text,omitempty"`

	// NewText is the text that should replace/be inserted (empty for pure deletions)
	NewText string `json:"new_text,omitempty"`
}

// SuggestionVerification shows the before/after state for validation.
type SuggestionVerification struct {
	// TextBeforeChange shows what the text looks like before applying the suggestion
	TextBeforeChange string `json:"text_before_change"`

	// TextAfterChange shows what the text should look like after applying the suggestion
	TextAfterChange string `json:"text_after_change"`
}

// ActionableSuggestion provides all context needed for an LLM to find and apply a suggestion.
//
// Design principles for machine-readability:
// 1. Anchor texts are NOT truncated - they contain exact strings for matching
// 2. Change is explicit about the operation (insert/delete/replace)
// 3. Verification allows confirming the change was applied correctly
// 4. Location is metadata for context, not for finding text
//
// To apply a suggestion to HTML:
// 1. Search for anchor.preceding_text + change.original_text + anchor.following_text
// 2. Replace change.original_text with change.new_text
// 3. Verify result matches verification.text_after_change
type ActionableSuggestion struct {
	// ID is the unique suggestion identifier from Google Docs
	ID string `json:"id"`

	// Anchor contains exact text before/after for locating where to apply the change
	Anchor SuggestionAnchor `json:"anchor"`

	// Change describes exactly what modification to make
	Change SuggestionChange `json:"change"`

	// Verification provides before/after text for validating the change
	Verification SuggestionVerification `json:"verification"`

	// Location provides contextual metadata (section, table, etc.) for human verification
	Location SuggestionLocation `json:"location"`

	// Position contains character indices in the original Google Doc (for reference only)
	Position struct {
		StartIndex int64 `json:"start_index"`
		EndIndex   int64 `json:"end_index"`
	} `json:"position"`
}

// DocumentStructure holds the parsed structure of the document for context lookups
type DocumentStructure struct {
	Headings     []DocumentHeading         `json:"headings"`
	Tables       []TableRange              `json:"tables"`
	FullText     string                    `json:"full_text"`     // Complete document text
	TextElements []TextElementWithPosition `json:"text_elements"` // All text with positions
}

// TableRange represents a table's position in the document
type TableRange struct {
	StartIndex    int64      `json:"start_index"`
	EndIndex      int64      `json:"end_index"`
	RowRanges     []RowRange `json:"row_ranges"`
	ColumnHeaders []string   `json:"column_headers"` // Headers from first row if available
}

// RowRange represents a row's position within a table
type RowRange struct {
	StartIndex int64       `json:"start_index"`
	EndIndex   int64       `json:"end_index"`
	CellRanges []CellRange `json:"cell_ranges"`
}

// CellRange represents a cell's position within a row
type CellRange struct {
	StartIndex int64  `json:"start_index"`
	EndIndex   int64  `json:"end_index"`
	Text       string `json:"text"`       // Full text of cell
	FirstLine  string `json:"first_line"` // First line only (for display)
}

// TextElementWithPosition stores text content with its document position
type TextElementWithPosition struct {
	Text       string `json:"text"`
	StartIndex int64  `json:"start_index"`
	EndIndex   int64  `json:"end_index"`
}

// Comment represents a comment on the document (from Drive API)
type Comment struct {
	ID              string   `json:"id"`
	Author          string   `json:"author"`
	AuthorEmail     string   `json:"author_email"`
	Content         string   `json:"content"`
	QuotedContent   string   `json:"quoted_content"` // The text this comment refers to
	CreatedTime     string   `json:"created_time"`
	ModifiedTime    string   `json:"modified_time"`
	Resolved        bool     `json:"resolved"`
	Replies         []Reply  `json:"replies,omitempty"`
	MentionedEmails []string `json:"mentioned_emails,omitempty"`
}

// Reply represents a reply to a comment
type Reply struct {
	ID          string `json:"id"`
	Author      string `json:"author"`
	AuthorEmail string `json:"author_email"`
	Content     string `json:"content"`
	CreatedTime string `json:"created_time"`
}

// MetadataTable represents the metadata table at the beginning of a document.
// This table contains key-value pairs with document metadata such as title,
// description, and other custom fields defined by the document template.
//
// Structure expected:
//
//	| Metadata                          |                                    |
//	| Page title (60 characters max)    | Ubuntu on AWS                      |
//	| Page description (160 chars max)  | Ubuntu is the operating system...  |
//	| ...                               | ...                                |
type MetadataTable struct {
	// Raw contains all key-value pairs from the metadata table
	Raw map[string]string `json:"raw"`

	// Commonly used fields extracted for convenience
	PageTitle       string `json:"page_title,omitempty"`
	PageDescription string `json:"page_description,omitempty"`

	// TableStartIndex is the character position where the metadata table starts
	TableStartIndex int64 `json:"table_start_index"`
	// TableEndIndex is the character position where the metadata table ends
	TableEndIndex int64 `json:"table_end_index"`
}

// buildDocsService creates a Google Docs API service using service account credentials.
// If useDelegation is true and an email is provided, it delegates the credentials to that email.
// If useDelegation is false, the service account accesses documents directly (must be shared with it).
// This follows the same pattern as DriveServiceBuild in go_extract_gsuite_data/pipeline/drive.go
func buildDocsService(ctx context.Context, email *string, delegate bool) (*docs.Service, error) {
	scopes := []string{
		"https://www.googleapis.com/auth/documents.readonly",
	}

	// Read service account credentials - using local credentials file
	credentials, err := os.ReadFile("bau-test-creds.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read service account file: %v", err)
	}

	config, err := google.JWTConfigFromJSON(credentials, scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT config: %v", err)
	}

	// Only set Subject for domain-wide delegation
	if delegate {
		if email != nil {
			config.Subject = *email
		} else {
			config.Subject = delegationEmail // default email
		}
	}
	// If delegate is false, don't set Subject - use service account directly

	client := config.Client(ctx)
	service, err := docs.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create docs service: %v", err)
	}

	return service, nil
}

// buildDriveService creates a Google Drive API service using service account credentials.
// If useDelegation is true and an email is provided, it delegates the credentials to that email.
// If useDelegation is false, the service account accesses files directly (must be shared with it).
// This follows the same pattern as DriveServiceBuild in go_extract_gsuite_data/pipeline/drive.go
func buildDriveService(ctx context.Context, email *string, delegate bool) (*drive.Service, error) {
	scopes := []string{
		"https://www.googleapis.com/auth/drive.readonly",
	}

	credentials, err := os.ReadFile("bau-test-creds.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read service account file: %v", err)
	}

	config, err := google.JWTConfigFromJSON(credentials, scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT config: %v", err)
	}

	// Only set Subject for domain-wide delegation
	if delegate {
		if email != nil {
			config.Subject = *email
		} else {
			config.Subject = delegationEmail // default email
		}
	}
	// If delegate is false, don't set Subject - use service account directly

	client := config.Client(ctx)
	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create drive service: %v", err)
	}

	return service, nil
}

// extractDocumentID extracts the document ID from a Google Docs URL
func extractDocumentID(url string) (string, error) {
	// Pattern: https://docs.google.com/document/d/{documentId}/...
	re := regexp.MustCompile(`/document/d/([a-zA-Z0-9_-]+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract document ID from URL: %s", url)
	}
	return matches[1], nil
}

// fetchDocumentContent fetches the document with suggestions inline
func fetchDocumentContent(ctx context.Context, docsService *docs.Service, docID string) (*docs.Document, error) {
	// Use SUGGESTIONS_INLINE to see suggestions marked in the content
	doc, err := docsService.Documents.Get(docID).
		SuggestionsViewMode("SUGGESTIONS_INLINE").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document: %v", err)
	}
	return doc, nil
}

// extractSuggestions walks through the document content and extracts all suggestions
// This function traverses all nested elements including tables, lists, etc.
func extractSuggestions(doc *docs.Document) []Suggestion {
	var suggestions []Suggestion

	// Declare all functions first so they can reference each other
	var processParagraphElement func(paraElem *docs.ParagraphElement)
	var processParagraph func(para *docs.Paragraph)
	var processTable func(table *docs.Table)
	var processStructuralElement func(elem *docs.StructuralElement)

	// Process a single paragraph element for suggestions
	processParagraphElement = func(paraElem *docs.ParagraphElement) {
		if paraElem.TextRun != nil {
			tr := paraElem.TextRun

			// Check for suggested insertions
			if len(tr.SuggestedInsertionIds) > 0 {
				for _, suggID := range tr.SuggestedInsertionIds {
					suggestions = append(suggestions, Suggestion{
						ID:         suggID,
						Type:       "insertion",
						Content:    tr.Content,
						StartIndex: paraElem.StartIndex,
						EndIndex:   paraElem.EndIndex,
					})
				}
			}

			// Check for suggested deletions
			if len(tr.SuggestedDeletionIds) > 0 {
				for _, suggID := range tr.SuggestedDeletionIds {
					suggestions = append(suggestions, Suggestion{
						ID:         suggID,
						Type:       "deletion",
						Content:    tr.Content,
						StartIndex: paraElem.StartIndex,
						EndIndex:   paraElem.EndIndex,
					})
				}
			}

			// Check for suggested text style changes
			if tr.SuggestedTextStyleChanges != nil {
				for suggID := range tr.SuggestedTextStyleChanges {
					suggestions = append(suggestions, Suggestion{
						ID:         suggID,
						Type:       "text_style_change",
						Content:    tr.Content,
						StartIndex: paraElem.StartIndex,
						EndIndex:   paraElem.EndIndex,
					})
				}
			}
		}
	}

	// Process a paragraph for suggestions
	processParagraph = func(para *docs.Paragraph) {
		if para == nil {
			return
		}
		for _, paraElem := range para.Elements {
			processParagraphElement(paraElem)
		}
	}

	// Process table cells recursively
	processTable = func(table *docs.Table) {
		if table == nil {
			return
		}
		for _, row := range table.TableRows {
			for _, cell := range row.TableCells {
				for _, cellContent := range cell.Content {
					processStructuralElement(cellContent)
				}
			}
		}
	}

	// Process a structural element (can contain paragraphs, tables, etc.)
	processStructuralElement = func(elem *docs.StructuralElement) {
		if elem == nil {
			return
		}
		if elem.Paragraph != nil {
			processParagraph(elem.Paragraph)
		}
		if elem.Table != nil {
			processTable(elem.Table)
		}
		if elem.TableOfContents != nil && elem.TableOfContents.Content != nil {
			for _, tocElem := range elem.TableOfContents.Content {
				processStructuralElement(tocElem)
			}
		}
	}

	// Process body content
	if doc.Body != nil {
		for _, elem := range doc.Body.Content {
			processStructuralElement(elem)
		}
	}

	// Also check headers and footers
	for _, header := range doc.Headers {
		if header.Content != nil {
			for _, elem := range header.Content {
				processStructuralElement(elem)
			}
		}
	}

	for _, footer := range doc.Footers {
		if footer.Content != nil {
			for _, elem := range footer.Content {
				processStructuralElement(elem)
			}
		}
	}

	return suggestions
}

// buildDocumentStructure builds a comprehensive structure of the document
// including all headings, tables, and text positions for context lookups.
func buildDocumentStructure(doc *docs.Document) *DocumentStructure {
	structure := &DocumentStructure{
		Headings:     []DocumentHeading{},
		Tables:       []TableRange{},
		TextElements: []TextElementWithPosition{},
	}

	var fullTextBuilder strings.Builder

	if doc.Body == nil || doc.Body.Content == nil {
		return structure
	}

	tableIndex := 0

	for _, elem := range doc.Body.Content {
		// Extract headings
		if elem.Paragraph != nil {
			para := elem.Paragraph

			// Check if this is a heading
			if para.ParagraphStyle != nil {
				namedStyle := para.ParagraphStyle.NamedStyleType
				headingLevel := 0
				switch namedStyle {
				case "HEADING_1":
					headingLevel = 1
				case "HEADING_2":
					headingLevel = 2
				case "HEADING_3":
					headingLevel = 3
				case "HEADING_4":
					headingLevel = 4
				case "HEADING_5":
					headingLevel = 5
				case "HEADING_6":
					headingLevel = 6
				}

				if headingLevel > 0 {
					// Extract heading text
					var headingText strings.Builder
					for _, paraElem := range para.Elements {
						if paraElem.TextRun != nil {
							headingText.WriteString(paraElem.TextRun.Content)
						}
					}
					structure.Headings = append(structure.Headings, DocumentHeading{
						Text:       strings.TrimSpace(headingText.String()),
						Level:      headingLevel,
						StartIndex: elem.StartIndex,
						EndIndex:   elem.EndIndex,
					})
				}
			}

			// Extract all text elements with positions
			for _, paraElem := range para.Elements {
				if paraElem.TextRun != nil {
					structure.TextElements = append(structure.TextElements, TextElementWithPosition{
						Text:       paraElem.TextRun.Content,
						StartIndex: paraElem.StartIndex,
						EndIndex:   paraElem.EndIndex,
					})
					fullTextBuilder.WriteString(paraElem.TextRun.Content)
				}
			}
		}

		// Extract table structure
		if elem.Table != nil {
			tableIndex++
			tableRange := TableRange{
				StartIndex:    elem.StartIndex,
				EndIndex:      elem.EndIndex,
				RowRanges:     []RowRange{},
				ColumnHeaders: []string{},
			}

			for rowIdx, row := range elem.Table.TableRows {
				rowRange := RowRange{
					StartIndex: row.StartIndex,
					EndIndex:   row.EndIndex,
					CellRanges: []CellRange{},
				}

				for _, cell := range row.TableCells {
					cellText := extractCellText(cell)
					// Get first line only for display purposes
					firstLine := cellText
					if idx := strings.Index(cellText, "\n"); idx != -1 {
						firstLine = cellText[:idx]
					}
					// Truncate to 50 chars for display
					if len(firstLine) > 50 {
						firstLine = firstLine[:50] + "..."
					}

					cellRange := CellRange{
						StartIndex: cell.StartIndex,
						EndIndex:   cell.EndIndex,
						Text:       cellText,
						FirstLine:  firstLine,
					}
					rowRange.CellRanges = append(rowRange.CellRanges, cellRange)

					// First row cells are column headers (use first line only)
					if rowIdx == 0 {
						tableRange.ColumnHeaders = append(tableRange.ColumnHeaders, firstLine)
					}

					// Also add cell text to text elements
					for _, cellContent := range cell.Content {
						if cellContent.Paragraph != nil {
							for _, paraElem := range cellContent.Paragraph.Elements {
								if paraElem.TextRun != nil {
									structure.TextElements = append(structure.TextElements, TextElementWithPosition{
										Text:       paraElem.TextRun.Content,
										StartIndex: paraElem.StartIndex,
										EndIndex:   paraElem.EndIndex,
									})
								}
							}
						}
					}
				}

				tableRange.RowRanges = append(tableRange.RowRanges, rowRange)
			}

			structure.Tables = append(structure.Tables, tableRange)
		}
	}

	structure.FullText = fullTextBuilder.String()
	return structure
}

// findParentHeading finds the nearest heading that comes before the given position
func findParentHeading(structure *DocumentStructure, position int64) (string, int) {
	var parentHeading string
	var headingLevel int

	for _, heading := range structure.Headings {
		if heading.StartIndex < position {
			parentHeading = heading.Text
			headingLevel = heading.Level
		} else {
			break
		}
	}

	return parentHeading, headingLevel
}

// findTableLocation determines if a position is within a table and returns its location
func findTableLocation(structure *DocumentStructure, position int64) *TableLocation {
	for tableIdx, table := range structure.Tables {
		if position >= table.StartIndex && position <= table.EndIndex {
			loc := &TableLocation{
				TableIndex: tableIdx + 1,
			}

			// Find the row and column
			for rowIdx, row := range table.RowRanges {
				if position >= row.StartIndex && position <= row.EndIndex {
					loc.RowIndex = rowIdx + 1

					// Get row header (first line of first cell of this row)
					if len(row.CellRanges) > 0 {
						loc.RowHeader = row.CellRanges[0].FirstLine
					}

					// Find the column
					for colIdx, cell := range row.CellRanges {
						if position >= cell.StartIndex && position <= cell.EndIndex {
							loc.ColumnIndex = colIdx + 1

							// Get column header
							if colIdx < len(table.ColumnHeaders) {
								loc.ColumnHeader = table.ColumnHeaders[colIdx]
							}
							break
						}
					}
					break
				}
			}

			return loc
		}
	}

	return nil
}

// getTextAround extracts text before and after a given position.
// Returns exact text (not truncated) for machine-readable anchoring.
// The anchorLength parameter controls how much context to include (default 80 chars).
func getTextAround(structure *DocumentStructure, startIndex, endIndex int64, anchorLength int) (before, after string) {
	var beforeBuilder strings.Builder
	var afterBuilder strings.Builder

	for _, elem := range structure.TextElements {
		// Text before the suggestion
		if elem.EndIndex <= startIndex {
			beforeBuilder.WriteString(elem.Text)
		}
		// Text after the suggestion
		if elem.StartIndex >= endIndex {
			afterBuilder.WriteString(elem.Text)
		}
	}

	beforeText := beforeBuilder.String()
	afterText := afterBuilder.String()

	// Take the last N characters of before text (closest to the suggestion)
	if len(beforeText) > anchorLength {
		before = beforeText[len(beforeText)-anchorLength:]
	} else {
		before = beforeText
	}

	// Take the first N characters of after text (closest to the suggestion)
	if len(afterText) > anchorLength {
		after = afterText[:anchorLength]
	} else {
		after = afterText
	}

	// Normalize whitespace but preserve for exact matching
	// Only trim leading/trailing, keep internal whitespace intact
	before = strings.TrimLeft(before, "\n\t ")
	after = strings.TrimRight(after, "\n\t ")

	return before, after
}

// buildActionableSuggestions converts raw suggestions into actionable suggestions with full context.
// The output is designed for machine consumption (LLM) to apply changes to HTML/text content.
func buildActionableSuggestions(suggestions []Suggestion, structure *DocumentStructure, metadata *MetadataTable) []ActionableSuggestion {
	actionable := make([]ActionableSuggestion, 0, len(suggestions))

	// Use 80 characters for anchor context - enough to uniquely identify location
	const anchorLength = 80

	for _, sugg := range suggestions {
		as := ActionableSuggestion{
			ID: sugg.ID,
		}

		// Set position
		as.Position.StartIndex = sugg.StartIndex
		as.Position.EndIndex = sugg.EndIndex

		// Determine location (metadata for context, not for finding)
		as.Location = SuggestionLocation{
			Section: "Body",
		}

		if metadata != nil && sugg.StartIndex >= metadata.TableStartIndex && sugg.EndIndex <= metadata.TableEndIndex {
			as.Location.InMetadata = true
		}

		parentHeading, headingLevel := findParentHeading(structure, sugg.StartIndex)
		as.Location.ParentHeading = parentHeading
		as.Location.HeadingLevel = headingLevel

		tableLoc := findTableLocation(structure, sugg.StartIndex)
		if tableLoc != nil {
			as.Location.InTable = true
			as.Location.Table = tableLoc
		}

		// Get anchor texts (not truncated - exact for matching)
		precedingText, followingText := getTextAround(structure, sugg.StartIndex, sugg.EndIndex, anchorLength)
		as.Anchor = SuggestionAnchor{
			PrecedingText: precedingText,
			FollowingText: followingText,
		}

		// Build change and verification based on suggestion type
		switch sugg.Type {
		case "insertion":
			as.Change = SuggestionChange{
				Type:         "insert",
				OriginalText: "",
				NewText:      sugg.Content,
			}
			as.Verification = SuggestionVerification{
				TextBeforeChange: precedingText + followingText,
				TextAfterChange:  precedingText + sugg.Content + followingText,
			}

		case "deletion":
			as.Change = SuggestionChange{
				Type:         "delete",
				OriginalText: sugg.Content,
				NewText:      "",
			}
			as.Verification = SuggestionVerification{
				TextBeforeChange: precedingText + sugg.Content + followingText,
				TextAfterChange:  precedingText + followingText,
			}

		case "text_style_change":
			// Style changes don't modify text content, only formatting
			as.Change = SuggestionChange{
				Type:         "style",
				OriginalText: sugg.Content,
				NewText:      sugg.Content, // Text unchanged, only style
			}
			as.Verification = SuggestionVerification{
				TextBeforeChange: precedingText + sugg.Content + followingText,
				TextAfterChange:  precedingText + sugg.Content + followingText,
			}
		}

		actionable = append(actionable, as)
	}

	return actionable
}

// extractCellText extracts all text content from a table cell.
// It traverses all paragraphs and text runs within the cell and concatenates their content.
// Newlines are trimmed from the final result.
func extractCellText(cell *docs.TableCell) string {
	var builder strings.Builder

	if cell == nil || cell.Content == nil {
		return ""
	}

	for _, elem := range cell.Content {
		if elem.Paragraph != nil {
			for _, paraElem := range elem.Paragraph.Elements {
				if paraElem.TextRun != nil {
					builder.WriteString(paraElem.TextRun.Content)
				}
			}
		}
	}

	return strings.TrimSpace(builder.String())
}

// extractMetadataTable extracts the metadata table from the beginning of the document.
//
// The metadata table is expected to be the FIRST table in the document body,
// typically following a header like "METADATA TEMPLATE". It contains key-value
// pairs where:
//   - Column 0 (left): Key/field name (e.g., "Page title (60 characters max)")
//   - Column 1 (right): Value (e.g., "Ubuntu on AWS")
//
// The function:
//  1. Finds the first table in doc.Body.Content
//  2. Iterates through each row (skipping header rows with empty values)
//  3. Extracts key-value pairs from 2-column rows
//  4. Maps common field names to structured fields (PageTitle, PageDescription)
//
// Returns nil if no table is found in the document.
//
// API Context:
//   - Uses the document structure from Google Docs API v1
//   - Table structure: Table -> TableRows -> TableCells -> Content -> Paragraphs
//   - Each TableCell contains StructuralElements (same as Body.Content)
func extractMetadataTable(doc *docs.Document) *MetadataTable {
	if doc.Body == nil || doc.Body.Content == nil {
		return nil
	}

	// Find the first table in the document
	var firstTable *docs.Table
	var tableStartIndex, tableEndIndex int64

	for _, elem := range doc.Body.Content {
		if elem.Table != nil {
			firstTable = elem.Table
			tableStartIndex = elem.StartIndex
			tableEndIndex = elem.EndIndex
			break
		}
	}

	if firstTable == nil {
		return nil
	}

	metadata := &MetadataTable{
		Raw:             make(map[string]string),
		TableStartIndex: tableStartIndex,
		TableEndIndex:   tableEndIndex,
	}

	// Process each row of the table
	for _, row := range firstTable.TableRows {
		if row.TableCells == nil || len(row.TableCells) < 2 {
			continue
		}

		// Extract key from first column, value from second column
		key := extractCellText(row.TableCells[0])
		value := extractCellText(row.TableCells[1])

		// Skip header row (usually "Metadata" with empty value) or empty rows
		if key == "" || key == "Metadata" {
			continue
		}

		// Store in raw map
		metadata.Raw[key] = value

		// Map to common fields based on key patterns
		keyLower := strings.ToLower(key)
		if strings.Contains(keyLower, "page title") || strings.Contains(keyLower, "title") && !strings.Contains(keyLower, "description") {
			metadata.PageTitle = value
		} else if strings.Contains(keyLower, "page description") || strings.Contains(keyLower, "description") {
			metadata.PageDescription = value
		}
	}

	// Only return if we found some metadata
	if len(metadata.Raw) == 0 {
		return nil
	}

	return metadata
}

// fetchComments fetches all comments from the document using Drive API
func fetchComments(ctx context.Context, driveService *drive.Service, docID string) ([]Comment, error) {
	var comments []Comment
	pageToken := ""

	for {
		req := driveService.Comments.List(docID).
			Fields("nextPageToken, comments(id, author(displayName, emailAddress), content, quotedFileContent, createdTime, modifiedTime, resolved, replies(id, author(displayName, emailAddress), content, createdTime), mentionedEmailAddresses, anchor)").
			Context(ctx)

		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch comments: %v", err)
		}

		for _, c := range resp.Comments {
			comment := Comment{
				ID:           c.Id,
				Content:      c.Content,
				CreatedTime:  c.CreatedTime,
				ModifiedTime: c.ModifiedTime,
				Resolved:     c.Resolved,
			}

			if c.Author != nil {
				comment.Author = c.Author.DisplayName
				comment.AuthorEmail = c.Author.EmailAddress
			}

			// This is the key field - it shows exactly what text the comment is attached to
			if c.QuotedFileContent != nil {
				comment.QuotedContent = c.QuotedFileContent.Value
			}

			// Note: MentionedEmailAddresses is only available in replies, not in comments
			// The Drive API Comment struct doesn't expose this field directly

			// Process replies
			for _, r := range c.Replies {
				reply := Reply{
					ID:          r.Id,
					Content:     r.Content,
					CreatedTime: r.CreatedTime,
				}
				if r.Author != nil {
					reply.Author = r.Author.DisplayName
					reply.AuthorEmail = r.Author.EmailAddress
				}
				comment.Replies = append(comment.Replies, reply)
			}

			comments = append(comments, comment)
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return comments, nil
}

func main() {
	// Setup structured logging
	logFile, err := os.OpenFile("log.json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()

	slog.Info("Starting Google Docs POC",
		slog.String("doc_url", googleDocURL),
	)

	// Extract document ID from URL
	docID, err := extractDocumentID(googleDocURL)
	if err != nil {
		slog.Error("Failed to extract document ID", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("Extracted document ID", slog.String("doc_id", docID))

	// Build services
	// If useDelegation is true, will impersonate delegationEmail via domain-wide delegation
	// If useDelegation is false, service account accesses document directly (must be shared with it)
	email := delegationEmail
	docsService, err := buildDocsService(ctx, &email, useDelegation)
	if err != nil {
		slog.Error("Failed to build Docs service", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if useDelegation {
		slog.Info("Docs service created successfully", slog.String("delegated_to", email))
	} else {
		slog.Info("Docs service created successfully (direct service account access)")
	}

	driveService, err := buildDriveService(ctx, &email, useDelegation)
	if err != nil {
		slog.Error("Failed to build Drive service", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if useDelegation {
		slog.Info("Drive service created successfully", slog.String("delegated_to", email))
	} else {
		slog.Info("Drive service created successfully (direct service account access)")
	}

	// Fetch document content with suggestions inline
	slog.Info("Fetching document content with suggestions inline...")
	doc, err := fetchDocumentContent(ctx, docsService, docID)
	if err != nil {
		slog.Error("Failed to fetch document", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("Document fetched successfully",
		slog.String("title", doc.Title),
		slog.String("document_id", doc.DocumentId),
	)

	// Extract suggestions from document
	suggestions := extractSuggestions(doc)
	slog.Info("Suggestions extracted",
		slog.Int("count", len(suggestions)),
	)

	// Fetch comments from Drive API
	slog.Info("Fetching comments from Drive API...")
	comments, err := fetchComments(ctx, driveService, docID)
	if err != nil {
		slog.Error("Failed to fetch comments", slog.String("error", err.Error()))
		// Don't exit - comments might not be accessible but we still have suggestions
	} else {
		slog.Info("Comments fetched", slog.Int("count", len(comments)))
	}

	// Extract metadata table from document
	metadata := extractMetadataTable(doc)
	if metadata != nil {
		slog.Info("Metadata table extracted", slog.Int("field_count", len(metadata.Raw)))
	}

	// Build document structure for context lookups
	docStructure := buildDocumentStructure(doc)
	slog.Info("Document structure built",
		slog.Int("headings", len(docStructure.Headings)),
		slog.Int("tables", len(docStructure.Tables)),
	)

	// Build actionable suggestions with full context
	actionableSuggestions := buildActionableSuggestions(suggestions, docStructure, metadata)

	// Output machine-readable JSON
	// This is the primary output - designed for LLM consumption
	output := struct {
		DocumentTitle         string                 `json:"document_title"`
		DocumentID            string                 `json:"document_id"`
		Metadata              *MetadataTable         `json:"metadata,omitempty"`
		ActionableSuggestions []ActionableSuggestion `json:"actionable_suggestions"`
		Comments              []Comment              `json:"comments"`
	}{
		DocumentTitle:         doc.Title,
		DocumentID:            doc.DocumentId,
		Metadata:              metadata,
		ActionableSuggestions: actionableSuggestions,
		Comments:              comments,
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	slog.Info("Actionable suggestions JSON generated")
	err = os.WriteFile("output.json", outputJSON, 0644)
	if err != nil {
		slog.Error("Failed to write output file", slog.String("error", err.Error()))
	}
	fmt.Println(string(outputJSON))
}
