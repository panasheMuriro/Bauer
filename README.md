# Bauer

A proof-of-concept Go application that extracts document content, suggestions (proposed edits), and comments from Google Docs using the Google Docs API and Google Drive API.


## Installation

### [Snap](https://snapcraft.io/bauer)
```
sudo snap install bauer
```

### Homebrew
```
brew install britneywwc/bauer
```

## Configuration
1. Install [Copilot CLI](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli)
2. Get credentials from Google Cloud service or Bitwarden (internally)
3. Fill up `credentials.json` with Google Cloud credentials.
   Required fields: 
- `type`: "service_account"
- `project_id`: GCP project ID
- `private_key`: RSA private key
- `client_email`: Service account email
4. Share copy document with service account (if `useDelegation = false`)
5. If you are not using Bitwarden credentials, make sure to enable APIs in GCP: Google Docs API, Google Drive API

## Usage
1. Clone this repository
2. Check that `bauer` is installed
```
bauer
```
3. Get document ID from Google Document
4. Run Bauer
```bash
bauer --doc-id <your-document-id> --credentials ./credentials.json
```

6. Optional parameters

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--chunk-size` | int | `30` | Number of suggestions per chunk sent to Copilot |
| `--dry-run` | bool | `false` | Run extraction and planning only; skip Copilot execution and PR creation |
| `--output-dir` | string | `bauer-output` | Output directory for generated files |
| `--model` | string | `gpt-4` | Copilot model to use for code generation |

### Examples

#### Basic run
```bash
bauer --doc-id <your-document-id> --credentials ./credentials.json
```

#### Dry run (test without executing changes)
```bash
bauer --doc-id <your-document-id> \
        --credentials ./credentials.json \
        --dry-run
```

#### Custom chunk size and output directory
```bash
bauer --doc-id <your-document-id> \
        --credentials ./credentials.json \
        --chunk-size 5 \
        --output-dir ./results
```

#### Specify model
```bash
bauer --doc-id <your-document-id> \
        --credentials ./credentials.json \
        --model "gpt-4-turbo"
```


## Local development
### Prerequisites
1. Install [`go`](https://golang.org/dl/)
2. Install [`task`](https://taskfile.dev/docs/installation)
3. Install [Copilot CLI](https://docs.github.com/en/copilot/how-tos/set-up/install-copilot-cli)

## Steps
1. Build the project with task
```
task build
```
2. Run the project with `./bauer` command


## Documentation
For more information refer to [`ARCHITECTURE.md`](/docs/ARCHITECTURE.md)

Full details of the specification can be found in [`FULL_SPEC.md`](/docs/FULL_SPEC.md)
