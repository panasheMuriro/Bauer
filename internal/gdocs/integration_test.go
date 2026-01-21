package gdocs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestFullExtractionIntegration(t *testing.T) {
	// 1. Load the mock API response fixture
	fixturePath := filepath.Join("..", "..", "test_fixtures", "doc_api_response.json")
	docJSON, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("Failed to read fixture file %s: %v", fixturePath, err)
	}

	var doc docs.Document
	if err := json.Unmarshal(docJSON, &doc); err != nil {
		t.Fatalf("Failed to unmarshal fixture into docs.Document: %v", err)
	}

	// 2. Run the extraction pipeline (mimicking ProcessDocument without network calls)

	// Step A: Extract Suggestions
	suggestions := ExtractSuggestions(&doc)

	// Step B: Extract Metadata
	metadata := ExtractMetadataTable(&doc)

	// Step C: Build Document Structure
	docStructure := BuildDocumentStructure(&doc)

	// Step D: Build Actionable Suggestions
	actionableSuggestions := BuildActionableSuggestions(suggestions, docStructure, metadata)

	// Construct the result object
	// Note: We are mocking comments as empty since the fixture is only for the Docs API response
	actualResult := ProcessingResult{
		DocumentTitle:         doc.Title,
		DocumentID:            doc.DocumentId,
		Metadata:              metadata,
		ActionableSuggestions: actionableSuggestions,
		Comments:              []Comment{},
	}

	// 3. Load the expected output fixture
	expectedPath := filepath.Join("..", "..", "test_fixtures", "expected_output.json")
	expectedJSON, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read fixture file %s: %v", expectedPath, err)
	}

	var expectedResult ProcessingResult
	if err := json.Unmarshal(expectedJSON, &expectedResult); err != nil {
		t.Fatalf("Failed to unmarshal expected output: %v", err)
	}

	// 4. Compare Actual vs Expected
	// We'll verify sections individually to provide better failure messages

	// Verify Metadata
	if !reflect.DeepEqual(actualResult.Metadata, expectedResult.Metadata) {
		t.Errorf("Metadata mismatch.\nGot: %+v\nWant: %+v", actualResult.Metadata, expectedResult.Metadata)
	}

	// Verify Actionable Suggestions
	if len(actualResult.ActionableSuggestions) != len(expectedResult.ActionableSuggestions) {
		t.Fatalf("Actionable suggestions count mismatch. Got %d, want %d",
			len(actualResult.ActionableSuggestions), len(expectedResult.ActionableSuggestions))
	}

	for i := range actualResult.ActionableSuggestions {
		got := actualResult.ActionableSuggestions[i]
		want := expectedResult.ActionableSuggestions[i]

		// Check IDs
		if got.ID != want.ID {
			t.Errorf("Suggestion[%d] ID mismatch. Got %s, want %s", i, got.ID, want.ID)
		}

		// Check Change
		if !reflect.DeepEqual(got.Change, want.Change) {
			t.Errorf("Suggestion[%d] Change mismatch.\nGot: %+v\nWant: %+v", i, got.Change, want.Change)
		}

		// Check Anchor
		if got.Anchor.PrecedingText != want.Anchor.PrecedingText {
			t.Errorf("Suggestion[%d] PrecedingText mismatch.\nGot: %q\nWant: %q", i, got.Anchor.PrecedingText, want.Anchor.PrecedingText)
		}
		if got.Anchor.FollowingText != want.Anchor.FollowingText {
			t.Errorf("Suggestion[%d] FollowingText mismatch.\nGot: %q\nWant: %q", i, got.Anchor.FollowingText, want.Anchor.FollowingText)
		}

		// Check Verification
		if !reflect.DeepEqual(got.Verification, want.Verification) {
			t.Errorf("Suggestion[%d] Verification mismatch.\nGot: %+v\nWant: %+v", i, got.Verification, want.Verification)
		}

		// Check Location
		// We explicitly ignore the table pointer address in comparison, checking content
		if got.Location.Section != want.Location.Section {
			t.Errorf("Suggestion[%d] Location.Section mismatch", i)
		}
		if got.Location.ParentHeading != want.Location.ParentHeading {
			t.Errorf("Suggestion[%d] Location.ParentHeading mismatch", i)
		}
		if got.Location.HeadingLevel != want.Location.HeadingLevel {
			t.Errorf("Suggestion[%d] Location.HeadingLevel mismatch", i)
		}
	}

	// Verify Titles/IDs
	if actualResult.DocumentTitle != expectedResult.DocumentTitle {
		t.Errorf("Title mismatch. Got %s, want %s", actualResult.DocumentTitle, expectedResult.DocumentTitle)
	}
	if actualResult.DocumentID != expectedResult.DocumentID {
		t.Errorf("ID mismatch. Got %s, want %s", actualResult.DocumentID, expectedResult.DocumentID)
	}
}
