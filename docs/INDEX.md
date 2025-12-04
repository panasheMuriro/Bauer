# Bauer - Draft PR Creator Documentation Index

Welcome! This is your guide to the Bauer system for creating GitHub draft PRs from Google Docs suggestions.

## 📚 Documentation Files

### Getting Started
- **[QUICKSTART.md](QUICKSTART.md)** ⭐ **START HERE**
  - 5-minute setup guide
  - Basic usage examples
  - Troubleshooting
  - Common workflows

### Understanding the System
- **[IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)**
  - High-level overview
  - Architecture summary
  - Key features and capabilities
  - Design decisions

- **[PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md)**
  - Comprehensive reference
  - Complete architecture details
  - Path resolution examples
  - Anchor-based matching explanation

### Developer Reference
- **[API_REFERENCE.md](API_REFERENCE.md)**
  - Function signatures
  - Parameter documentation
  - Return values and errors
  - Usage patterns and examples

- **[FILE_STRUCTURE.md](FILE_STRUCTURE.md)**
  - File organization
  - Function call graphs
  - Data flow diagrams
  - Maintenance notes

### Code Examples
- **[examples.sh](examples.sh)**
  - Interactive example script
  - 4 different use cases
  - Command templates

- **[pr_test.go](pr_test.go)**
  - Unit tests
  - Test examples
  - Usage patterns

---

## 🚀 Quick Start (2 minutes)

### 1. Set GitHub Token
```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxx
```

### 2. Verify Credentials
```bash
# Check service account credentials exist
ls -la bau-test-creds.json
```

### 3. Run Bauer
```bash
# Generate output.json AND create draft PR
GITHUB_TOKEN=$GITHUB_TOKEN go run . \
  --create-pr \
  --github-token=$GITHUB_TOKEN
```

### 4. Check GitHub
Visit the URL printed in the output to see your draft PR!

---

## 📖 How to Use This Documentation

### I want to...

**Get started immediately**
→ Read [QUICKSTART.md](QUICKSTART.md)

**Understand how it works**
→ Read [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)

**Learn all the details**
→ Read [PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md)

**Write code that uses it**
→ Read [API_REFERENCE.md](API_REFERENCE.md)

**See examples**
→ Run [examples.sh](examples.sh) or check [pr_test.go](pr_test.go)

**Understand file organization**
→ Read [FILE_STRUCTURE.md](FILE_STRUCTURE.md)

**Deploy to production**
→ Read QUICKSTART.md + PR_CREATION_GUIDE.md

**Troubleshoot an issue**
→ Check QUICKSTART.md section 7

---

## 🎯 Main Concepts

### Suggestion Types
- **Insert**: Add new content
- **Delete**: Remove content
- **Style**: Change formatting (not content)

### How It Works
1. Extract suggestions from Google Doc
2. Parse metadata (page title, URL, etc.)
3. Resolve file path in repository
4. Apply changes to file
5. Create draft PR on GitHub

### Path Resolution
```
URL: https://ubuntu.com/aws
  ↓
Extract: /aws
  ↓
Try: templates/aws.html
   or templates/aws/index.html
  ↓
Found: templates/aws/index.html
  ↓
Apply suggestions and commit
```

### Anchor-Based Matching
Suggestions use surrounding text to find exact locations:
```
Before: ...Heroku\nAcquia\nChoose...
Change: Delete "Acquia"
After:  ...Heroku\nChoose...
```

---

## 💻 Main Functions

### High-Level (Use These)
```go
// Generate JSON and optionally create PR
ProcessAndCreatePR(ctx, googleDocURL, githubToken, shouldCreatePR)

// Create PR from existing JSON
CreatePRFromJSON(ctx, outputFile, githubToken)
```

### Low-Level (Developers)
```go
// GitHub operations
CreateGitHubClient(token)
CreateDraftPR(ctx, ghClient, owner, repo, output)

// Utilities
ExtractURLPath(fullURL)
ResolvePath(urlPath)
ApplySuggestionsToContent(content, suggestions)
```

---

## 📋 File Locations

```
Bauer/
├── CODE FILES
│   ├── main.go              (Google Docs extraction)
│   ├── pr.go                (GitHub PR creation) ← NEW
│   ├── workflow.go          (High-level workflows) ← NEW
│   ├── pr_test.go           (Tests) ← NEW
│   └── examples.sh          (Interactive examples) ← NEW
│
├── DOCUMENTATION
│   ├── README.md            (This file)
│   ├── QUICKSTART.md        (5-minute setup) ← START HERE
│   ├── IMPLEMENTATION_SUMMARY.md (Overview)
│   ├── PR_CREATION_GUIDE.md (Full details)
│   ├── API_REFERENCE.md     (Function docs)
│   ├── FILE_STRUCTURE.md    (File organization)
│   └── INDEX.md             (This file)
│
├── CONFIG
│   ├── bau-test-creds.json  (Service account credentials)
│   └── output.json          (Generated suggestions)
│
└── DEPENDENCIES
    └── Go standard library + Google/GitHub APIs
```

---

## ✅ Checklist Before Using

- [ ] Have GitHub Personal Access Token (`ghp_...`)
- [ ] Have Google Service Account credentials (`bau-test-creds.json`)
- [ ] Credentials are NOT in version control
- [ ] Go 1.21+ installed
- [ ] Can access both GitHub and Google Docs

---

## 🔍 Common Tasks

### Task: Generate output.json
```bash
go run .
```
→ Check [QUICKSTART.md](QUICKSTART.md) - Step 3

### Task: Create a draft PR
```bash
GITHUB_TOKEN=$GITHUB_TOKEN go run . --create-pr
```
→ Check [QUICKSTART.md](QUICKSTART.md) - Basic Usage section

### Task: Review before creating PR
```bash
go run .          # Generate JSON
# Review output.json
GITHUB_TOKEN=$GITHUB_TOKEN go run . --create-pr-from-json
```
→ Check [QUICKSTART.md](QUICKSTART.md) - Workflow 2

### Task: Custom repository
```bash
GITHUB_TOKEN=$GITHUB_TOKEN go run . \
  --create-pr \
  --repo-owner=your-org \
  --repo-name=your-repo
```
→ Check [QUICKSTART.md](QUICKSTART.md) - Customization

### Task: Debug an issue
1. Check logs output by Bauer
2. Review [QUICKSTART.md](QUICKSTART.md) - Troubleshooting
3. Check [API_REFERENCE.md](API_REFERENCE.md) for error handling

---

## 🆘 Need Help?

### Setup Issues
→ See [QUICKSTART.md](QUICKSTART.md) Section 7: Troubleshooting

### How something works
→ See [API_REFERENCE.md](API_REFERENCE.md)

### Architecture questions
→ See [PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md)

### Code examples
→ See [pr_test.go](pr_test.go) or run [examples.sh](examples.sh)

### I can't find something
→ Check [FILE_STRUCTURE.md](FILE_STRUCTURE.md) for file organization

---

## 📊 Documentation at a Glance

| Document | Length | Audience | Purpose |
|----------|--------|----------|---------|
| [QUICKSTART.md](QUICKSTART.md) | 3 pages | Everyone | Get running in 5 min |
| [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md) | 2 pages | Decision makers | Understand what it does |
| [PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md) | 6 pages | Tech leads | Full details |
| [API_REFERENCE.md](API_REFERENCE.md) | 5 pages | Developers | Function docs |
| [FILE_STRUCTURE.md](FILE_STRUCTURE.md) | 4 pages | Maintainers | Code organization |

---

## 🎓 Learning Path

### Path 1: I just want to use it (5 minutes)
1. [QUICKSTART.md](QUICKSTART.md) - Sections 1-3
2. Run: `GITHUB_TOKEN=$GITHUB_TOKEN go run . --create-pr`
3. Done! ✓

### Path 2: I want to understand it (20 minutes)
1. [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)
2. [QUICKSTART.md](QUICKSTART.md)
3. [API_REFERENCE.md](API_REFERENCE.md) - Just the function names
4. Done! ✓

### Path 3: I want to integrate it (45 minutes)
1. [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)
2. [API_REFERENCE.md](API_REFERENCE.md) - Full functions
3. [pr_test.go](pr_test.go) - Examples
4. [PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md) - Architecture
5. Done! ✓

### Path 4: I need to maintain it (1 hour)
1. All of Path 3
2. [FILE_STRUCTURE.md](FILE_STRUCTURE.md)
3. [QUICKSTART.md](QUICKSTART.md) - Troubleshooting
4. Code review of `pr.go` and `workflow.go`
5. Done! ✓

---

## 🔗 External Resources

- [GitHub REST API Docs](https://docs.github.com/en/rest)
- [Google Docs API Docs](https://developers.google.com/docs/api)
- [Go Documentation](https://golang.org/doc/)

---

## 📝 Quick Reference Card

```go
// Main workflow
ProcessAndCreatePR(ctx, docURL, token, true)

// Components
GoogleDoc → (extract) → output.json
output.json → (process) → Draft PR

// Path examples
/aws → templates/aws.html | templates/aws/index.html
/cloud/azure → templates/cloud/azure.html | templates/cloud/azure/index.html

// Suggestion types
insert, delete, style

// Branch naming
content/{page-title}-{timestamp}

// PR creation
Draft PR (not ready for merge)
```

---

## 📅 Version History

**v1.0** (December 4, 2025)
- ✅ PR creation from Google Docs
- ✅ Path resolution system
- ✅ GitHub integration
- ✅ Complete documentation
- ✅ Tests and examples

---

## 📞 Questions?

Refer to the appropriate documentation:
- **Setup**: [QUICKSTART.md](QUICKSTART.md)
- **How it works**: [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)
- **Full details**: [PR_CREATION_GUIDE.md](PR_CREATION_GUIDE.md)
- **API usage**: [API_REFERENCE.md](API_REFERENCE.md)
- **Troubleshooting**: [QUICKSTART.md](QUICKSTART.md) Section 7

---

**Happy coding!** 🚀

Created: December 4, 2025
Last updated: December 4, 2025
