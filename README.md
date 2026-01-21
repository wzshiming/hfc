# hfc - HuggingFace Cache for Go

A Go library and CLI for downloading files from [HuggingFace Hub](https://huggingface.co/) with cache compatibility with the Python [`huggingface_hub`](https://github.com/huggingface/huggingface_hub) library.

## Features

- Download files from HuggingFace Hub (models, datasets, spaces)
- Cache compatibility with Python's `huggingface_hub` library
- Support for resume downloads
- Authentication via tokens
- Offline mode support
- Configurable via environment variables
- Command-line interface (`hfc download`)

## Installation

### Library

```bash
go get github.com/wzshiming/hfc
```

### CLI

```bash
go install github.com/wzshiming/hfc/cmd/hfc@latest
```

## CLI Usage

### Download a file

```bash
# Download a single file from a model
hfc download gpt2 config.json

# Download multiple files
hfc download gpt2 config.json tokenizer.json

# Download from a dataset
hfc download --repo-type dataset squad README.md

# Download a specific revision
hfc download --revision v1.0 gpt2 config.json

# Download to a local directory
hfc download --local-dir ./models gpt2 config.json

# Download with authentication
hfc download --token hf_xxx private/model config.json

# Download quietly (only output the path)
hfc download --quiet gpt2 config.json
```

### CLI Options

```
Usage: hfc download [options] <repo_id> [filenames...]

Options:
  --repo-type    Repository type: model, dataset, or space (default: model)
  --revision     Git revision (branch, tag, or commit hash)
  --cache-dir    Directory where cached files are stored
  --local-dir    Download to this local directory instead of cache
  --token        HuggingFace authentication token
  --force        Force re-download even if file is cached
  --quiet        Suppress progress output
  --endpoint     HuggingFace endpoint URL
```

## Library Usage

### Basic Download

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/wzshiming/hfc"
)

func main() {
    ctx := context.Background()
    
    path, err := hfc.Download(ctx, hfc.DownloadOptions{
        RepoID:   "gpt2",
        Filename: "config.json",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Downloaded to: %s\n", path)
}
```

### Library Options

### Download with Options

```go
path, err := hfc.Download(ctx, hfc.DownloadOptions{
    RepoID:    "facebook/opt-350m",
    Filename:  "config.json",
    RepoType:  hfc.RepoTypeModel,
    Revision:  "main",
    Token:     "hf_xxx", // Optional: HuggingFace API token
    CacheDir:  "/path/to/cache",
})
```

### Download Dataset

```go
path, err := hfc.Download(ctx, hfc.DownloadOptions{
    RepoID:   "squad",
    Filename: "README.md",
    RepoType: hfc.RepoTypeDataset,
})
```

### Check Cache

```go
// Try to load from cache without downloading
cachedPath := hfc.TryToLoadFromCache("gpt2", "config.json", "", "", "")
if cachedPath != "" {
    fmt.Printf("Found in cache: %s\n", cachedPath)
}
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `HF_HOME` | HuggingFace home directory | `~/.cache/huggingface` |
| `HF_HUB_CACHE` | Hub cache directory | `$HF_HOME/hub` |
| `HF_TOKEN` | Authentication token | - |
| `HF_ENDPOINT` | HuggingFace endpoint URL | `https://huggingface.co` |
| `HF_HUB_OFFLINE` | Enable offline mode | `false` |
| `HF_HUB_ETAG_TIMEOUT` | Metadata fetch timeout (seconds) | `10` |
| `HF_HUB_DOWNLOAD_TIMEOUT` | Download timeout (seconds) | `10` |

## Cache Structure

The cache follows the same layout as Python's `huggingface_hub`:

```
~/.cache/huggingface/hub/
└── models--gpt2/
    ├── blobs/
    │   └── <etag>              # Actual file content
    ├── refs/
    │   └── main                # Points to commit hash
    └── snapshots/
        └── <commit_hash>/
            └── config.json     # Symlink to blob
```

This means files downloaded with this Go library can be reused by Python's `huggingface_hub` and vice versa.

## API Reference

### Functions

#### `Download(ctx, opts) (string, error)`

Downloads a file from HuggingFace Hub. Returns the path to the downloaded file.

#### `TryToLoadFromCache(repoID, filename, cacheDir, revision, repoType) string`

Checks if a file is already cached. Returns the cached path or empty string.

#### `HfHubURL(opts) (string, error)`

Constructs the URL for a file on HuggingFace Hub.

#### `GetHfFileMetadata(url, token, timeout) (*HfFileMetadata, error)`

Fetches metadata about a file from the Hub.

### Types

#### `DownloadOptions`

```go
type DownloadOptions struct {
    RepoID         string        // Repository ID (required)
    Filename       string        // File name (required)
    Subfolder      string        // Subfolder within repo
    RepoType       string        // "model", "dataset", or "space"
    Revision       string        // Git revision (branch, tag, commit)
    CacheDir       string        // Cache directory
    LocalDir       string        // Download to local dir instead of cache
    Token          string        // Auth token
    ForceDownload  bool          // Force re-download
    LocalFilesOnly bool          // Only use local files
    Endpoint       string        // Custom endpoint URL
    EtagTimeout    time.Duration // Metadata timeout
    DownloadTimeout time.Duration // Download timeout
}
```

## License

MIT License - see [LICENSE](LICENSE) file.
