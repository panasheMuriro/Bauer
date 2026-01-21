package gdocs

import (
	"sort"
	"strings"
)

// GroupActionableSuggestions groups related atomic suggestions into logical units.
// Suggestions are grouped by their ID and must be contiguous in position.
// Returns a slice of grouped suggestions ready for LLM consumption.
func GroupActionableSuggestions(suggestions []ActionableSuggestion, structure *DocumentStructure) []GroupedActionableSuggestion {
	if len(suggestions) == 0 {
		return []GroupedActionableSuggestion{}
	}

	// Group suggestions by ID
	groupsByID := make(map[string][]ActionableSuggestion)
	for _, sugg := range suggestions {
		groupsByID[sugg.ID] = append(groupsByID[sugg.ID], sugg)
	}

	// Process each group
	var grouped []GroupedActionableSuggestion
	for id, group := range groupsByID {
		// Sort by start position to ensure correct ordering
		sort.Slice(group, func(i, j int) bool {
			return group[i].Position.StartIndex < group[j].Position.StartIndex
		})

		// Verify contiguity (atomic operations should be adjacent or overlapping)
		if !areContiguous(group) {
			// If not contiguous, treat each as separate (shouldn't happen, but defensive)
			for _, sugg := range group {
				grouped = append(grouped, convertSingleSuggestion(sugg))
			}
			continue
		}

		// Group is valid - merge the suggestions
		merged := mergeSuggestions(id, group, structure)
		grouped = append(grouped, merged)
	}

	// Sort final result by position for consistent output
	sort.Slice(grouped, func(i, j int) bool {
		return grouped[i].Position.StartIndex < grouped[j].Position.StartIndex
	})

	return grouped
}

// areContiguous checks if suggestions are adjacent or overlapping in position.
// This validates that they're truly part of the same logical change.
func areContiguous(suggestions []ActionableSuggestion) bool {
	if len(suggestions) <= 1 {
		return true
	}

	for i := 0; i < len(suggestions)-1; i++ {
		current := suggestions[i]
		next := suggestions[i+1]

		// Next suggestion should start at or before current ends (allowing for overlap/adjacency)
		// We allow a small gap (1 char) for edge cases
		if next.Position.StartIndex > current.Position.EndIndex+1 {
			return false
		}
	}

	return true
}

// convertSingleSuggestion converts a single ActionableSuggestion to GroupedActionableSuggestion.
// Used for suggestions that don't need grouping.
func convertSingleSuggestion(sugg ActionableSuggestion) GroupedActionableSuggestion {
	return GroupedActionableSuggestion{
		ID:     sugg.ID,
		Anchor: sugg.Anchor,
		Change: sugg.Change,
		Verification: SuggestionVerification{
			TextBeforeChange: sugg.Verification.TextBeforeChange,
			TextAfterChange:  sugg.Verification.TextAfterChange,
		},
		Location: sugg.Location,
		Position: struct {
			StartIndex int64 `json:"start_index"`
			EndIndex   int64 `json:"end_index"`
		}{
			StartIndex: sugg.Position.StartIndex,
			EndIndex:   sugg.Position.EndIndex,
		},
		AtomicChanges: []SuggestionChange{sugg.Change},
		AtomicCount:   1,
	}
}

// mergeSuggestions combines multiple atomic suggestions into a single grouped suggestion.
func mergeSuggestions(id string, suggestions []ActionableSuggestion, structure *DocumentStructure) GroupedActionableSuggestion {
	if len(suggestions) == 1 {
		return convertSingleSuggestion(suggestions[0])
	}

	first := suggestions[0]
	last := suggestions[len(suggestions)-1]

	// Extract anchors with increased length (120 chars) for better context
	const groupedAnchorLength = 120
	precedingText, followingText := getTextAround(structure, first.Position.StartIndex, last.Position.EndIndex, groupedAnchorLength)

	// Collect atomic changes
	atomicChanges := make([]SuggestionChange, len(suggestions))
	for i, sugg := range suggestions {
		atomicChanges[i] = sugg.Change
	}

	// Merge the changes to compute the net effect
	mergedChange := mergeChanges(suggestions)

	// Build verification texts
	var originalText, newText string
	if mergedChange.Type == "insert" {
		originalText = ""
		newText = mergedChange.NewText
	} else if mergedChange.Type == "delete" {
		originalText = mergedChange.OriginalText
		newText = ""
	} else { // "replace"
		originalText = mergedChange.OriginalText
		newText = mergedChange.NewText
	}

	verification := SuggestionVerification{
		TextBeforeChange: precedingText + originalText + followingText,
		TextAfterChange:  precedingText + newText + followingText,
	}

	return GroupedActionableSuggestion{
		ID: id,
		Anchor: SuggestionAnchor{
			PrecedingText: precedingText,
			FollowingText: followingText,
		},
		Change:       mergedChange,
		Verification: verification,
		Location:     first.Location, // All should have same location
		Position: struct {
			StartIndex int64 `json:"start_index"`
			EndIndex   int64 `json:"end_index"`
		}{
			StartIndex: first.Position.StartIndex,
			EndIndex:   last.Position.EndIndex,
		},
		AtomicChanges: atomicChanges,
		AtomicCount:   len(suggestions),
	}
}

// mergeChanges combines multiple atomic changes into a single net change.
// Handles sequences like: insert "Build " + delete "Y" + insert "y" -> replace "Y" with "Build y"
func mergeChanges(suggestions []ActionableSuggestion) SuggestionChange {
	var originalParts []string
	var newParts []string
	hasInsertions := false
	hasDeletions := false

	// Process each atomic change in order
	for _, sugg := range suggestions {
		switch sugg.Change.Type {
		case "insert":
			hasInsertions = true
			newParts = append(newParts, sugg.Change.NewText)
		case "delete":
			hasDeletions = true
			originalParts = append(originalParts, sugg.Change.OriginalText)
		case "style":
			// Style changes don't affect text content
			// Keep the text in both original and new
			if sugg.Change.OriginalText != "" {
				originalParts = append(originalParts, sugg.Change.OriginalText)
				newParts = append(newParts, sugg.Change.OriginalText)
			}
		}
	}

	originalText := strings.Join(originalParts, "")
	newText := strings.Join(newParts, "")

	// Determine the type of the merged change
	changeType := "replace"
	if !hasDeletions && hasInsertions {
		changeType = "insert"
	} else if hasDeletions && !hasInsertions {
		changeType = "delete"
	} else if !hasDeletions && !hasInsertions {
		changeType = "style"
	}

	return SuggestionChange{
		Type:         changeType,
		OriginalText: originalText,
		NewText:      newText,
	}
}
