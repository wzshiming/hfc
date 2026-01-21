// Package hfc provides a HuggingFace file downloader that is compatible with
// the huggingface_hub Python library's cache system.
package hfc

import (
	"os"
	"path/filepath"
	"strconv"
)

// Constants for file downloads
const (
	DefaultEtagTimeout     = 10 // seconds
	DefaultDownloadTimeout = 10 // seconds
	DefaultRequestTimeout  = 10 // seconds
	DownloadChunkSize      = 10 * 1024 * 1024 // 10 MB
	RepoIDSeparator        = "--"
	DefaultRevision        = "main"
)

// Repository types
const (
	RepoTypeModel   = "model"
	RepoTypeDataset = "dataset"
	RepoTypeSpace   = "space"
)

// RepoTypes is a list of valid repository types
var RepoTypes = []string{RepoTypeModel, RepoTypeDataset, RepoTypeSpace}

// RepoTypesURLPrefixes maps repository types to URL prefixes
var RepoTypesURLPrefixes = map[string]string{
	RepoTypeDataset: "datasets/",
	RepoTypeSpace:   "spaces/",
}

// HTTP headers used by HuggingFace
const (
	HeaderXRepoCommit  = "X-Repo-Commit"
	HeaderXLinkedEtag  = "X-Linked-Etag"
	HeaderXLinkedSize  = "X-Linked-Size"
)

// Default HuggingFace endpoint
const defaultEndpoint = "https://huggingface.co"

// GetEndpoint returns the HuggingFace endpoint, considering environment variables
func GetEndpoint() string {
	if ep := os.Getenv("HF_ENDPOINT"); ep != "" {
		return trimSuffix(ep, "/")
	}
	return defaultEndpoint
}

// GetHFHome returns the HuggingFace home directory
func GetHFHome() string {
	if home := os.Getenv("HF_HOME"); home != "" {
		return os.ExpandEnv(home)
	}
	if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
		return filepath.Join(xdgCache, "huggingface")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".cache", "huggingface")
}

// GetHFHubCache returns the HuggingFace hub cache directory
func GetHFHubCache() string {
	if cache := os.Getenv("HF_HUB_CACHE"); cache != "" {
		return os.ExpandEnv(cache)
	}
	if cache := os.Getenv("HUGGINGFACE_HUB_CACHE"); cache != "" {
		return cache
	}
	return filepath.Join(GetHFHome(), "hub")
}

// GetHFToken returns the HuggingFace token from environment or token file
func GetHFToken() string {
	if token := os.Getenv("HF_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("HUGGING_FACE_HUB_TOKEN"); token != "" {
		return token
	}
	tokenPath := getHFTokenPath()
	if data, err := os.ReadFile(tokenPath); err == nil {
		return string(data)
	}
	return ""
}

func getHFTokenPath() string {
	if path := os.Getenv("HF_TOKEN_PATH"); path != "" {
		return os.ExpandEnv(path)
	}
	return filepath.Join(GetHFHome(), "token")
}

// GetEtagTimeout returns the etag timeout from environment or default
func GetEtagTimeout() int {
	if timeout := os.Getenv("HF_HUB_ETAG_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			return t
		}
	}
	return DefaultEtagTimeout
}

// GetDownloadTimeout returns the download timeout from environment or default
func GetDownloadTimeout() int {
	if timeout := os.Getenv("HF_HUB_DOWNLOAD_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil {
			return t
		}
	}
	return DefaultDownloadTimeout
}

// IsOfflineMode returns whether offline mode is enabled
func IsOfflineMode() bool {
	return isTrueEnv("HF_HUB_OFFLINE") || isTrueEnv("TRANSFORMERS_OFFLINE")
}

func isTrueEnv(key string) bool {
	val := os.Getenv(key)
	switch val {
	case "1", "ON", "YES", "TRUE", "on", "yes", "true":
		return true
	}
	return false
}

func trimSuffix(s, suffix string) string {
	for len(s) > 0 && len(suffix) > 0 && s[len(s)-1] == suffix[len(suffix)-1] {
		s = s[:len(s)-1]
		suffix = suffix[:len(suffix)-1]
	}
	return s
}
