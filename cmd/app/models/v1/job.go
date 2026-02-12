package models

type JobPost struct {
	// DocID is the Google Doc ID to extract feedback from.
	DocID string `json:"doc_id"`

	// ChunkSize is the total number of chunks to create from all locations.
	// Default is 1 if not specified, or 5 if PageRefresh is true.
	ChunkSize int `json:"chunk_size"`

	// PageRefresh indicates if the page refresh mode should be used.
	// When true, uses page-refresh-instructions.md template and defaults ChunkSize to 5.
	PageRefresh bool `json:"page_refresh"`
}
