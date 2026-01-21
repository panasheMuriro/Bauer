package gdocs

import (
	"testing"

	"bauer/internal/models"

	"google.golang.org/api/docs/v1"
)

// Helper to create a pointer to int64
func int64Ptr(i int64) *int64 {
	return &i
}

func TestExtractSuggestions(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: 1,
					EndIndex:   20,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								EndIndex:   10,
								TextRun: &docs.TextRun{
									Content:               "Insertion",
									SuggestedInsertionIds: []string{"ins-1"},
								},
							},
							{
								StartIndex: 10,
								EndIndex:   20,
								TextRun: &docs.TextRun{
									Content:              "Deletion",
									SuggestedDeletionIds: []string{"del-1"},
								},
							},
						},
					},
				},
				{
					StartIndex: 21,
					EndIndex:   30,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 21,
								EndIndex:   30,
								TextRun: &docs.TextRun{
									Content: "StyleChange",
									SuggestedTextStyleChanges: map[string]docs.SuggestedTextStyle{
										"style-1": {},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	suggestions := ExtractSuggestions(doc)

	if len(suggestions) != 3 {
		t.Fatalf("Expected 3 suggestions, got %d", len(suggestions))
	}

	// Verify Insertion
	foundIns := false
	for _, s := range suggestions {
		if s.ID == "ins-1" {
			foundIns = true
			if s.Type != "insertion" {
				t.Errorf("Expected insertion type, got %s", s.Type)
			}
			if s.Content != "Insertion" {
				t.Errorf("Expected content 'Insertion', got %s", s.Content)
			}
			if s.StartIndex != 1 || s.EndIndex != 10 {
				t.Errorf("Incorrect indices for insertion: %d-%d", s.StartIndex, s.EndIndex)
			}
		}
	}
	if !foundIns {
		t.Error("Insertion suggestion not found")
	}

	// Verify Deletion
	foundDel := false
	for _, s := range suggestions {
		if s.ID == "del-1" {
			foundDel = true
			if s.Type != "deletion" {
				t.Errorf("Expected deletion type, got %s", s.Type)
			}
		}
	}
	if !foundDel {
		t.Error("Deletion suggestion not found")
	}

	// Verify Style Change
	foundStyle := false
	for _, s := range suggestions {
		if s.ID == "style-1" {
			foundStyle = true
			if s.Type != "text_style_change" {
				t.Errorf("Expected text_style_change type, got %s", s.Type)
			}
		}
	}
	if !foundStyle {
		t.Error("Style change suggestion not found")
	}
}

func TestExtractMetadataTable(t *testing.T) {
	tests := []struct {
		name       string
		doc        *docs.Document
		wantTitle  string
		wantDesc   string
		wantFields int
		wantNil    bool
	}{
		{
			name: "Valid Metadata Table",
			doc: &docs.Document{
				Body: &docs.Body{
					Content: []*docs.StructuralElement{
						{
							StartIndex: 1,
							EndIndex:   100,
							Table: &docs.Table{
								TableRows: []*docs.TableRow{
									{
										TableCells: []*docs.TableCell{
											{Content: createContent("Metadata")},
											{Content: createContent("")},
										},
									},
									{
										TableCells: []*docs.TableCell{
											{Content: createContent("Page Title")},
											{Content: createContent("My Title")},
										},
									},
									{
										TableCells: []*docs.TableCell{
											{Content: createContent("Page Description")},
											{Content: createContent("My Description")},
										},
									},
									{
										TableCells: []*docs.TableCell{
											{Content: createContent("Custom Field")},
											{Content: createContent("Custom Value")},
										},
									},
								},
							},
						},
					},
				},
			},
			wantTitle:  "My Title",
			wantDesc:   "My Description",
			wantFields: 3,
			wantNil:    false,
		},
		{
			name: "No Table",
			doc: &docs.Document{
				Body: &docs.Body{
					Content: []*docs.StructuralElement{
						{
							Paragraph: &docs.Paragraph{},
						},
					},
				},
			},
			wantNil: true,
		},
		{
			name: "Table without enough columns",
			doc: &docs.Document{
				Body: &docs.Body{
					Content: []*docs.StructuralElement{
						{
							Table: &docs.Table{
								TableRows: []*docs.TableRow{
									{
										TableCells: []*docs.TableCell{
											{Content: createContent("Single Column")},
										},
									},
								},
							},
						},
					},
				},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMetadataTable(tt.doc)
			if tt.wantNil {
				if got != nil {
					t.Error("Expected nil metadata, got struct")
				}
				return
			}

			if got == nil {
				t.Fatal("Expected metadata, got nil")
			}

			if got.PageTitle != tt.wantTitle {
				t.Errorf("PageTitle = %s, want %s", got.PageTitle, tt.wantTitle)
			}
			if got.PageDescription != tt.wantDesc {
				t.Errorf("PageDescription = %s, want %s", got.PageDescription, tt.wantDesc)
			}
			if len(got.Raw) != tt.wantFields {
				t.Errorf("Raw fields count = %d, want %d", len(got.Raw), tt.wantFields)
			}
		})
	}
}

func TestBuildDocumentStructure(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				// Heading 1
				{
					StartIndex: 1,
					EndIndex:   10,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Heading 1"}},
						},
					},
				},
				// Normal Text
				{
					StartIndex: 11,
					EndIndex:   20,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 11,
								EndIndex:   20,
								TextRun:    &docs.TextRun{Content: "Some text"},
							},
						},
					},
				},
				// Table
				{
					StartIndex: 21,
					EndIndex:   50,
					Table: &docs.Table{
						TableRows: []*docs.TableRow{
							{
								StartIndex: 22,
								EndIndex:   30,
								TableCells: []*docs.TableCell{
									{
										StartIndex: 23,
										EndIndex:   29,
										Content:    createContent("Cell 1"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	structure := BuildDocumentStructure(doc)

	// Verify Headings
	if len(structure.Headings) != 1 {
		t.Fatalf("Expected 1 heading, got %d", len(structure.Headings))
	}
	if structure.Headings[0].Text != "Heading 1" {
		t.Errorf("Expected heading 'Heading 1', got '%s'", structure.Headings[0].Text)
	}
	if structure.Headings[0].Level != 1 {
		t.Errorf("Expected heading level 1, got %d", structure.Headings[0].Level)
	}

	// Verify Tables
	if len(structure.Tables) != 1 {
		t.Fatalf("Expected 1 table, got %d", len(structure.Tables))
	}

	// Verify TextElements
	// 1 from Heading, 1 from Normal Text, 1 from Table Cell = 3
	if len(structure.TextElements) != 3 {
		t.Errorf("Expected 3 text elements, got %d", len(structure.TextElements))
	}

	expectedText := "Heading 1Some textCell 1"
	if structure.FullText != expectedText {
		t.Errorf("Expected full text '%s', got '%s'", expectedText, structure.FullText)
	}
}

func TestBuildActionableSuggestions(t *testing.T) {
	// Setup a document structure with text: "Start [INSERT] End"
	// "Start " is indices 0-6
	// "End" is indices 6-9
	// Suggestion is at index 6

	structure := &models.DocumentStructure{
		TextElements: []models.TextElementWithPosition{
			{Text: "Start ", StartIndex: 0, EndIndex: 6},
			{Text: "End", StartIndex: 6, EndIndex: 9},
		},
		Headings: []models.DocumentHeading{
			{Text: "My Heading", Level: 1, StartIndex: 0, EndIndex: 5},
		},
	}

	suggestions := []models.Suggestion{
		{
			ID:         "sugg-1",
			Type:       "insertion",
			Content:    "INSERT ",
			StartIndex: 6,
			EndIndex:   6, // Point insertion
		},
	}

	actionable := BuildActionableSuggestions(suggestions, structure, nil)

	if len(actionable) != 1 {
		t.Fatalf("Expected 1 actionable suggestion, got %d", len(actionable))
	}

	as := actionable[0]

	// Verify Anchor
	if as.Anchor.PrecedingText != "Start " {
		t.Errorf("Expected PrecedingText 'Start ', got '%s'", as.Anchor.PrecedingText)
	}
	if as.Anchor.FollowingText != "End" {
		t.Errorf("Expected FollowingText 'End', got '%s'", as.Anchor.FollowingText)
	}

	// Verify Change
	if as.Change.Type != "insert" {
		t.Errorf("Expected change type 'insert', got '%s'", as.Change.Type)
	}
	if as.Change.NewText != "INSERT " {
		t.Errorf("Expected NewText 'INSERT ', got '%s'", as.Change.NewText)
	}

	// Verify Location context
	if as.Location.ParentHeading != "My Heading" {
		t.Errorf("Expected ParentHeading 'My Heading', got '%s'", as.Location.ParentHeading)
	}
}

// Helper to create basic content structure for tests
func createContent(text string) []*docs.StructuralElement {
	return []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{
							Content: text,
						},
					},
				},
			},
		},
	}
}
