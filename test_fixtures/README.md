# Test Fixtures

This directory contains static JSON files used for integration testing of the extraction logic.

## Files

### `doc_api_response.json`

A mocked JSON representation of a Google Docs API response (`docs.Document`).
It simulates a document containing:
- A metadata table (Page Title, Page Description).
- A Heading 1 ("Introduction").
- A paragraph with suggested insertion and deletion.
- A standard data table.

This file serves as the input for testing `gdocs.ProcessDocument` logic (mocking the API call).

### `expected_output.json`

The expected `ProcessingResult` JSON output when `doc_api_response.json` is processed by the application.
It verifies:
- Metadata extraction.
- Actionable suggestion generation (anchors, changes, verification).
- Document structure parsing.