package hfc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoFolderName(t *testing.T) {
	tests := []struct {
		repoID   string
		repoType string
		expected string
	}{
		{"gpt2", "model", "models--gpt2"},
		{"facebook/opt-350m", "model", "models--facebook--opt-350m"},
		{"squad", "dataset", "datasets--squad"},
		{"gradio/hello_world", "space", "spaces--gradio--hello_world"},
		{"bert-base-uncased", "", "models--bert-base-uncased"}, // default to model
	}

	for _, tt := range tests {
		t.Run(tt.repoID, func(t *testing.T) {
			result := RepoFolderName(tt.repoID, tt.repoType)
			if result != tt.expected {
				t.Errorf("RepoFolderName(%q, %q) = %q, want %q", tt.repoID, tt.repoType, result, tt.expected)
			}
		})
	}
}

func TestHfHubURL(t *testing.T) {
	tests := []struct {
		name     string
		opts     HfHubURLOptions
		expected string
		wantErr  bool
	}{
		{
			name: "basic model",
			opts: HfHubURLOptions{
				RepoID:   "gpt2",
				Filename: "config.json",
				Endpoint: "https://huggingface.co",
			},
			expected: "https://huggingface.co/gpt2/resolve/main/config.json",
			wantErr:  false,
		},
		{
			name: "model with namespace",
			opts: HfHubURLOptions{
				RepoID:   "facebook/opt-350m",
				Filename: "config.json",
				Endpoint: "https://huggingface.co",
			},
			expected: "https://huggingface.co/facebook/opt-350m/resolve/main/config.json",
			wantErr:  false,
		},
		{
			name: "dataset",
			opts: HfHubURLOptions{
				RepoID:   "squad",
				Filename: "README.md",
				RepoType: "dataset",
				Endpoint: "https://huggingface.co",
			},
			expected: "https://huggingface.co/datasets/squad/resolve/main/README.md",
			wantErr:  false,
		},
		{
			name: "with revision",
			opts: HfHubURLOptions{
				RepoID:   "gpt2",
				Filename: "config.json",
				Revision: "v1.0",
				Endpoint: "https://huggingface.co",
			},
			expected: "https://huggingface.co/gpt2/resolve/v1.0/config.json",
			wantErr:  false,
		},
		{
			name: "with subfolder",
			opts: HfHubURLOptions{
				RepoID:    "gpt2",
				Filename:  "model.safetensors",
				Subfolder: "weights",
				Endpoint:  "https://huggingface.co",
			},
			expected: "https://huggingface.co/gpt2/resolve/main/weights/model.safetensors",
			wantErr:  false,
		},
		{
			name: "missing repo_id",
			opts: HfHubURLOptions{
				Filename: "config.json",
			},
			wantErr: true,
		},
		{
			name: "missing filename",
			opts: HfHubURLOptions{
				RepoID: "gpt2",
			},
			wantErr: true,
		},
		{
			name: "invalid repo type",
			opts: HfHubURLOptions{
				RepoID:   "gpt2",
				Filename: "config.json",
				RepoType: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HfHubURL(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("HfHubURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("HfHubURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNormalizeEtag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"abc123"`, "abc123"},
		{`W/"abc123"`, "abc123"},
		{"abc123", "abc123"},
		{"", ""},
		{`"quoted"`, "quoted"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeEtag(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeEtag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsCommitHash(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", true},  // 40 hex chars
		{"main", false},
		{"v1.0", false},
		{"abc", false},
		{"0123456789abcdef0123456789abcdef01234567", true}, // valid 40 char hex
		{"ABCDEF", false}, // uppercase not valid
		{"0123456789abcdef0123456789abcdef0123456", false}, // 39 chars
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsCommitHash(tt.input)
			if result != tt.expected {
				t.Errorf("IsCommitHash(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetHFHome(t *testing.T) {
	// Save original env
	origHFHome := os.Getenv("HF_HOME")
	origXDGCache := os.Getenv("XDG_CACHE_HOME")
	defer func() {
		os.Setenv("HF_HOME", origHFHome)
		os.Setenv("XDG_CACHE_HOME", origXDGCache)
	}()

	// Test with HF_HOME set
	os.Setenv("HF_HOME", "/custom/hf/home")
	os.Setenv("XDG_CACHE_HOME", "")
	if got := GetHFHome(); got != "/custom/hf/home" {
		t.Errorf("GetHFHome() = %q, want %q", got, "/custom/hf/home")
	}

	// Test with XDG_CACHE_HOME set
	os.Setenv("HF_HOME", "")
	os.Setenv("XDG_CACHE_HOME", "/custom/xdg")
	if got := GetHFHome(); got != "/custom/xdg/huggingface" {
		t.Errorf("GetHFHome() = %q, want %q", got, "/custom/xdg/huggingface")
	}
}

func TestGetHFHubCache(t *testing.T) {
	// Save original env
	origCache := os.Getenv("HF_HUB_CACHE")
	defer func() {
		os.Setenv("HF_HUB_CACHE", origCache)
	}()

	os.Setenv("HF_HUB_CACHE", "/custom/cache")
	if got := GetHFHubCache(); got != "/custom/cache" {
		t.Errorf("GetHFHubCache() = %q, want %q", got, "/custom/cache")
	}
}

func TestTryToLoadFromCache(t *testing.T) {
	// Create a temporary cache directory
	tmpDir := t.TempDir()
	
	// Create cache structure
	repoCache := filepath.Join(tmpDir, "models--gpt2")
	snapshotsDir := filepath.Join(repoCache, "snapshots")
	commitDir := filepath.Join(snapshotsDir, "abc123def456789012345678901234567890abcd")
	
	if err := os.MkdirAll(commitDir, 0755); err != nil {
		t.Fatal(err)
	}
	
	// Create a cached file
	cachedFile := filepath.Join(commitDir, "config.json")
	if err := os.WriteFile(cachedFile, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	
	// Create refs directory and main ref
	refsDir := filepath.Join(repoCache, "refs")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refsDir, "main"), []byte("abc123def456789012345678901234567890abcd"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test finding cached file
	result := TryToLoadFromCache("gpt2", "config.json", tmpDir, "main", "model")
	if result != cachedFile {
		t.Errorf("TryToLoadFromCache() = %q, want %q", result, cachedFile)
	}

	// Test non-existent file
	result = TryToLoadFromCache("gpt2", "nonexistent.json", tmpDir, "main", "model")
	if result != "" {
		t.Errorf("TryToLoadFromCache() = %q, want empty string", result)
	}

	// Test non-existent repo
	result = TryToLoadFromCache("nonexistent-repo", "config.json", tmpDir, "main", "model")
	if result != "" {
		t.Errorf("TryToLoadFromCache() = %q, want empty string", result)
	}
}

func TestCacheCommitHashForSpecificRevision(t *testing.T) {
	tmpDir := t.TempDir()
	
	err := CacheCommitHashForSpecificRevision(tmpDir, "main", "abc123")
	if err != nil {
		t.Fatalf("CacheCommitHashForSpecificRevision() error = %v", err)
	}
	
	// Check that ref file was created
	refPath := filepath.Join(tmpDir, "refs", "main")
	data, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("Failed to read ref file: %v", err)
	}
	if string(data) != "abc123" {
		t.Errorf("Ref file content = %q, want %q", string(data), "abc123")
	}
	
	// Test that same commit hash doesn't error
	err = CacheCommitHashForSpecificRevision(tmpDir, "main", "abc123")
	if err != nil {
		t.Fatalf("CacheCommitHashForSpecificRevision() error on same hash = %v", err)
	}
}

func TestGetEndpoint(t *testing.T) {
	// Save original env
	origEndpoint := os.Getenv("HF_ENDPOINT")
	defer func() {
		os.Setenv("HF_ENDPOINT", origEndpoint)
	}()

	os.Setenv("HF_ENDPOINT", "")
	if got := GetEndpoint(); got != "https://huggingface.co" {
		t.Errorf("GetEndpoint() = %q, want %q", got, "https://huggingface.co")
	}

	os.Setenv("HF_ENDPOINT", "https://custom.endpoint.co/")
	if got := GetEndpoint(); got != "https://custom.endpoint.co" {
		t.Errorf("GetEndpoint() = %q, want %q", got, "https://custom.endpoint.co")
	}
}

func TestIsOfflineMode(t *testing.T) {
	// Save original env
	origOffline := os.Getenv("HF_HUB_OFFLINE")
	defer func() {
		os.Setenv("HF_HUB_OFFLINE", origOffline)
	}()

	os.Setenv("HF_HUB_OFFLINE", "")
	if IsOfflineMode() {
		t.Error("IsOfflineMode() = true, want false")
	}

	os.Setenv("HF_HUB_OFFLINE", "1")
	if !IsOfflineMode() {
		t.Error("IsOfflineMode() = false, want true")
	}
}

func TestGetEndpointTrimsTrailingSlash(t *testing.T) {
	// Save original env
	origEndpoint := os.Getenv("HF_ENDPOINT")
	defer func() {
		os.Setenv("HF_ENDPOINT", origEndpoint)
	}()

	// Test with trailing slash
	os.Setenv("HF_ENDPOINT", "https://huggingface.co/")
	if got := GetEndpoint(); got != "https://huggingface.co" {
		t.Errorf("GetEndpoint() = %q, want %q", got, "https://huggingface.co")
	}

	// Test without trailing slash
	os.Setenv("HF_ENDPOINT", "https://huggingface.co")
	if got := GetEndpoint(); got != "https://huggingface.co" {
		t.Errorf("GetEndpoint() = %q, want %q", got, "https://huggingface.co")
	}
}
