# Bauer Quick Start Guide

Get started creating draft PRs from Google Docs in 5 minutes.

## 1. Prerequisites

- Go 1.21+
- GitHub Personal Access Token
- Google Cloud service account credentials

## 2. Setup

### Step 1: Get GitHub Token

1. Go to https://github.com/settings/tokens
2. Click "Generate new token (classic)"
3. Set name: "Bauer PR Creator"
4. Select scopes: `repo` (full control of private repositories)
5. Click "Generate token"
6. Copy the token

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxx
```

### Step 2: Get Google Service Account Credentials

1. Go to Google Cloud Console: https://console.cloud.google.com
2. Create a new project or select existing
3. Enable these APIs:
   - Google Docs API
   - Google Drive API
4. Create a service account:
   - Navigate to "Service Accounts"
   - Click "Create Service Account"
   - Fill in the details
5. Create a key:
   - Click on the service account
   - Go to "Keys" tab
   - Click "Add Key" → "Create new key"
   - Select JSON
   - Save the file
6. Rename to `bau-test-creds.json` and place in the Bauer directory

### Step 3: Share Google Doc

1. Open the Google Doc you want to review
2. Click "Share"
3. Add the service account email as an editor
4. Copy the document ID from the URL

Update the `googleDocURL` constant in `main.go`:

```go
const googleDocURL = "https://docs.google.com/document/d/YOUR_DOC_ID/edit"
```

## 3. Basic Usage

### Generate JSON Only

Extract suggestions without creating a PR:

```bash
go run .
```

This creates `output.json` with all suggestions.

### Create a Draft PR

Generate JSON AND create a draft PR:

```bash
GITHUB_TOKEN=$GITHUB_TOKEN go run . \
  --create-pr \
  --github-token=$GITHUB_TOKEN \
  --repo-owner=canonical \
  --repo-name=ubuntu.com
```

This will:
1. Extract suggestions from the Google Doc
2. Create a new branch
3. Apply changes to the file
4. Create a draft PR on github.com/canonical/ubuntu.com
5. Print the PR URL

## 4. Verify It Worked

After running with `--create-pr`, you should see:

```
INFO: Starting draft PR creation
INFO: Extracted URL path path=/aws
INFO: Resolved potential file paths paths=[templates/aws.html templates/aws/index.html]
INFO: Built PR metadata title="chore: update Ubuntu on AWS"
INFO: Created branch branch=content/ubuntu-on-aws-1733328872
INFO: Got latest commit sha=abc123def456
INFO: Created commit for file path=templates/aws/index.html
INFO: Draft PR created successfully url=https://github.com/canonical/ubuntu.com/pull/1234
```

Visit the PR URL to review changes before merging.

## 5. Common Workflows

### Workflow 1: Review → Suggest → Create PR

1. **Add suggestions to Google Doc**
   - Open the doc in Google Docs
   - Make suggestions (Ctrl+Alt+O)

2. **Run Bauer**
   ```bash
   GITHUB_TOKEN=$GITHUB_TOKEN go run . --create-pr
   ```

3. **Review PR on GitHub**
   - Visit the PR URL
   - Review changes
   - Request reviewers
   - Merge when approved

### Workflow 2: Batch Multiple Docs

Create a script to process multiple documents:

```bash
#!/bin/bash

DOCS=(
  "https://docs.google.com/document/d/DOC_ID_1/edit"
  "https://docs.google.com/document/d/DOC_ID_2/edit"
  "https://docs.google.com/document/d/DOC_ID_3/edit"
)

for doc in "${DOCS[@]}"; do
  echo "Processing: $doc"
  # Modify main.go googleDocURL and run
  go run .
  # Create PR from output.json
  GITHUB_TOKEN=$GITHUB_TOKEN go run . --create-pr-from-json
done
```

### Workflow 3: Dry Run Before PR

1. Generate JSON without creating PR
   ```bash
   go run .
   ```

2. Review `output.json` to verify suggestions

3. Create PR when satisfied
   ```bash
   GITHUB_TOKEN=$GITHUB_TOKEN go run . --create-pr-from-json
   ```

## 6. Customization

### Change Target Repository

To create PRs on a different repository:

```bash
GITHUB_TOKEN=$GITHUB_TOKEN go run . \
  --create-pr \
  --github-token=$GITHUB_TOKEN \
  --repo-owner=my-org \
  --repo-name=my-repo
```

### Change Base Branch

Currently hardcoded to `main`. To modify, edit `pr.go`:

```go
prMetadata.BaseBranch = "develop" // or your branch
```

### Filter Suggestions

To apply only certain suggestion types, edit `ApplySuggestionsToContent()` in `pr.go`:

```go
func ApplySuggestionsToContent(content string, suggestions []ActionableSuggestion) string {
  for _, sugg := range suggestions {
    // Skip style changes
    if sugg.Change.Type == "style" {
      continue
    }
    // ... apply other changes
  }
  return content
}
```

## 7. Troubleshooting

### "Invalid GitHub Token"

**Error**: `failed to create branch: status 401`

**Solution**: 
- Check token is set: `echo $GITHUB_TOKEN`
- Verify token hasn't expired
- Regenerate token at https://github.com/settings/tokens

### "Service Account Credentials Not Found"

**Error**: `Service account credentials not found: bau-test-creds.json`

**Solution**:
- Download JSON from Google Cloud Console
- Save in Bauer directory as `bau-test-creds.json`
- Ensure service account email is added to Google Doc

### "File Not Found"

**Error**: `Failed to get file content, skipping this path`

**Solution**:
- Check URL path in metadata is correct
- Verify file exists in repository
- Try both `.html` and `/index.html` variants

### "No Suggestions Found"

**Solution**:
- Open Google Doc
- Add suggestions (Ctrl+Alt+O)
- Make sure they're not resolved
- Re-run Bauer

## 8. Next Steps

- Read [API_REFERENCE.md](API_REFERENCE.md) for detailed function documentation
- Read [PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md) for architecture details
- Check `pr_test.go` for more examples

## 9. Support

For issues or questions:
1. Check the troubleshooting section above
2. Review the logs (use `-debug` flag for more output)
3. Check `output.json` for actual suggestions being extracted
4. Verify GitHub and Google credentials have proper access

## 10. Security Notes

- **Never commit GitHub tokens** to git
- Use environment variables: `export GITHUB_TOKEN=...`
- Keep `bau-test-creds.json` out of version control
- Add to `.gitignore`:
  ```
  bau-test-creds.json
  .env
  *.token
  ```
- Rotate tokens regularly
- Use service accounts with minimal necessary permissions
