package main

import (
	"testing"
)

// Test examples for the PR creation functions

// TestPathResolution tests the path resolver functionality
func TestPathResolution(t *testing.T) {
	resolver := NewPathResolver("")

	tests := []struct {
		urlPath string
		want    []string
	}{
		{
			urlPath: "/aws",
			want:    []string{"templates/aws.html", "templates/aws/index.html"},
		},
		{
			urlPath: "/cloud/azure",
			want:    []string{"templates/cloud/azure.html", "templates/cloud/azure/index.html"},
		},
		{
			urlPath: "/",
			want:    []string{"templates/index.html"},
		},
		{
			urlPath: "aws", // without leading slash
			want:    []string{"templates/aws.html", "templates/aws/index.html"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.urlPath, func(t *testing.T) {
			got := resolver.ResolvePath(tt.urlPath)
			if len(got) != len(tt.want) {
				t.Errorf("ResolvePath(%s) returned %d paths, want %d", tt.urlPath, len(got), len(tt.want))
			}
			for i, path := range got {
				if path != tt.want[i] {
					t.Errorf("ResolvePath(%s)[%d] = %q, want %q", tt.urlPath, i, path, tt.want[i])
				}
			}
		})
	}
}

// TestURLPathExtraction tests URL path extraction
func TestURLPathExtraction(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{
			url:  "https://ubuntu.com/aws",
			want: "/aws",
		},
		{
			url:  "https://ubuntu.com/cloud/azure",
			want: "/cloud/azure",
		},
		{
			url:  "https://ubuntu.com/",
			want: "/",
		},
		{
			url:  "https://ubuntu.com",
			want: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, err := ExtractURLPath(tt.url)
			if err != nil {
				t.Errorf("ExtractURLPath(%s) returned error: %v", tt.url, err)
			}
			if got != tt.want {
				t.Errorf("ExtractURLPath(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestBranchNameGeneration tests branch name generation
func TestBranchNameGeneration(t *testing.T) {
	metadata := &MetadataTable{
		PageTitle: "Ubuntu on AWS",
	}

	branch := generateBranchName(metadata)

	// Should start with "content/"
	if len(branch) < 8 || branch[:8] != "content/" {
		t.Errorf("generateBranchName() = %q, should start with 'content/'", branch)
	}

	// Should contain the title in kebab-case
	if !contains(branch, "ubuntu-on-aws") {
		t.Errorf("generateBranchName() = %q, should contain 'ubuntu-on-aws'", branch)
	}
}

// TestSuggestionApplication tests applying suggestions to content
func TestSuggestionApplication(t *testing.T) {
	content := "Netflix\nPaypal\nHeroku\nAcquia\n\nChoose"

	suggestions := []ActionableSuggestion{
		{
			Change: SuggestionChange{
				Type:         "delete",
				OriginalText: "Acquia",
			},
		},
	}

	result := ApplySuggestionsToContent(content, suggestions)

	// Acquia should be removed
	if contains(result, "Acquia") {
		t.Errorf("ApplySuggestionsToContent() should remove 'Acquia', got: %q", result)
	}

	// Other content should remain
	if !contains(result, "Netflix") || !contains(result, "Heroku") {
		t.Errorf("ApplySuggestionsToContent() removed too much content: %q", result)
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ExamplePathResolver shows how to use the path resolver
func ExamplePathResolver() {
	resolver := NewPathResolver("")

	// Example: /aws URL
	paths := resolver.ResolvePath("/aws")
	for _, path := range paths {
		println("Try:", path)
	}
	// Output:
	// Try: templates/aws.html
	// Try: templates/aws/index.html
}

// ExampleExtractURLPath shows how to extract the path from a URL
func ExampleExtractURLPath() {
	path, _ := ExtractURLPath("https://ubuntu.com/aws")
	println("Path:", path)
	// Output:
	// Path: /aws
}

// ExampleBuildPRMetadata shows how to build PR metadata
func ExampleBuildPRMetadata() {
	metadata := &MetadataTable{
		PageTitle: "Ubuntu on AWS",
	}

	suggestions := []ActionableSuggestion{
		{Change: SuggestionChange{Type: "delete"}},
		{Change: SuggestionChange{Type: "insert"}},
	}

	prMeta := BuildPRMetadata(metadata, suggestions)
	println("Title:", prMeta.Title)
	println("Branch:", prMeta.Branch)
	// Output:
	// Title: chore: update Ubuntu on AWS
	// Branch: content/ubuntu-on-aws-<timestamp>
}
