package hfc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Environment variable names
const (
	EnvHFHome       = "HF_HOME"
	EnvHFHubCache   = "HF_HUB_CACHE"
	EnvXDGCacheHome = "XDG_CACHE_HOME"
	EnvHFEndpoint   = "HF_ENDPOINT"
	EnvHFToken      = "HF_TOKEN"
)

// Defaults
const (
	DefaultEndpoint = "https://huggingface.co"
	DefaultRevision = "main"
)

// Repository types
const (
	repoTypeModel   = "model"
	repoTypeDataset = "dataset"
	repoTypeSpace   = "space"
)

// Separator used in cache directory names (not allowed in repo IDs on hf.co)
const repoIDSeparator = "--"

// HTTP headers from Hugging Face Hub
const (
	headerXRepoCommit = "X-Repo-Commit"
	headerXLinkedEtag = "X-Linked-Etag"
	headerXLinkedSize = "X-Linked-Size"
)

// GetHFHome returns the Hugging Face home directory.
func GetHFHome() string {
	if home := os.Getenv(EnvHFHome); home != "" {
		return expandPath(home)
	}
	cacheDir := os.Getenv(EnvXDGCacheHome)
	if cacheDir == "" {
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	return filepath.Join(cacheDir, "huggingface")
}

// GetHFHubCache returns the Hugging Face Hub cache directory.
func GetHFHubCache() string {
	if cache := os.Getenv(EnvHFHubCache); cache != "" {
		return expandPath(cache)
	}
	return filepath.Join(GetHFHome(), "hub")
}

// GetEndpoint returns the Hugging Face endpoint URL.
func GetEndpoint() string {
	if endpoint := os.Getenv(EnvHFEndpoint); endpoint != "" {
		return strings.TrimSuffix(endpoint, "/")
	}
	return DefaultEndpoint
}

// GetToken returns the Hugging Face token.
func GetToken() string {
	if token := os.Getenv(EnvHFToken); token != "" {
		return token
	}
	// Try reading from token file
	tokenPath := filepath.Join(GetHFHome(), "token")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// expandPath expands environment variables and ~ in a path.
func expandPath(path string) string {
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, path[1:])
	}
	return path
}

func pluralizeRepoType(repoType string) string {
	switch repoType {
	case "", repoTypeModel:
		return "models"
	case repoTypeDataset:
		return "datasets"
	case repoTypeSpace:
		return "spaces"
	default:
		return repoType + "s"
	}
}

// IsCommitHash checks if a string is a valid git commit hash (40 hex characters).
var commitHashRegex = regexp.MustCompile(`^[0-9a-f]{40}$`)

func isCommitHash(s string) bool {
	return commitHashRegex.MatchString(s)
}
