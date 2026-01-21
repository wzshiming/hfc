package hfc_test

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wzshiming/hfc"
)

// getProjectRoot returns the root directory of the project
func getProjectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Dir(filename)
}

// checkNetwork verifies if HuggingFace Hub is accessible
func checkNetwork(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Head("https://huggingface.co")
	if err != nil {
		t.Skipf("Skipping e2e test: network unavailable: %v", err)
	}
	resp.Body.Close()
}

// TestE2EDownloadPublicModel tests downloading a file from a public model
func TestE2EDownloadPublicModel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	tmpDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Download config.json from gpt2 (a small public model)
	opts := hfc.DownloadOptions{
		RepoID:   "gpt2",
		Filename: "config.json",
		CacheDir: tmpDir,
	}

	path, err := hfc.Download(ctx, opts)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Downloaded file does not exist at %s", path)
	}

	// Verify file has content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if len(content) == 0 {
		t.Fatal("Downloaded file is empty")
	}

	// Verify it's valid JSON with expected content
	if !strings.Contains(string(content), "vocab_size") {
		t.Error("Downloaded config.json does not contain expected 'vocab_size' field")
	}

	t.Logf("Successfully downloaded %d bytes to %s", len(content), path)
}

// TestE2EDownloadWithRevision tests downloading a file with a specific revision
func TestE2EDownloadWithRevision(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	tmpDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := hfc.DownloadOptions{
		RepoID:   "gpt2",
		Filename: "config.json",
		Revision: "main",
		CacheDir: tmpDir,
	}

	path, err := hfc.Download(ctx, opts)
	if err != nil {
		t.Fatalf("Download with revision failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Downloaded file does not exist at %s", path)
	}

	t.Logf("Successfully downloaded with revision to %s", path)
}

// TestE2EDownloadToLocalDir tests downloading to a local directory
func TestE2EDownloadToLocalDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	tmpDir := t.TempDir()
	localDir := filepath.Join(tmpDir, "local")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := hfc.DownloadOptions{
		RepoID:   "gpt2",
		Filename: "config.json",
		LocalDir: localDir,
	}

	path, err := hfc.Download(ctx, opts)
	if err != nil {
		t.Fatalf("Download to local dir failed: %v", err)
	}

	expectedPath := filepath.Join(localDir, "config.json")
	if path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, path)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Downloaded file does not exist at %s", path)
	}

	t.Logf("Successfully downloaded to local dir: %s", path)
}

// TestE2ECacheReuse tests that cached files are reused
func TestE2ECacheReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	tmpDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	opts := hfc.DownloadOptions{
		RepoID:   "gpt2",
		Filename: "config.json",
		CacheDir: tmpDir,
	}

	// First download
	path1, err := hfc.Download(ctx, opts)
	if err != nil {
		t.Fatalf("First download failed: %v", err)
	}

	info1, err := os.Stat(path1)
	if err != nil {
		t.Fatalf("Failed to stat first download: %v", err)
	}

	// Second download should use cache
	path2, err := hfc.Download(ctx, opts)
	if err != nil {
		t.Fatalf("Second download failed: %v", err)
	}

	if path1 != path2 {
		t.Errorf("Expected same path for cached download, got %s and %s", path1, path2)
	}

	info2, err := os.Stat(path2)
	if err != nil {
		t.Fatalf("Failed to stat second download: %v", err)
	}

	// Modification time should be the same (file wasn't re-downloaded)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("File was re-downloaded instead of using cache")
	}

	t.Log("Cache reuse verified successfully")
}

// TestE2EDownloadDataset tests downloading from a dataset repository
func TestE2EDownloadDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	tmpDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := hfc.DownloadOptions{
		RepoID:   "squad",
		Filename: "README.md",
		RepoType: hfc.RepoTypeDataset,
		CacheDir: tmpDir,
	}

	path, err := hfc.Download(ctx, opts)
	if err != nil {
		t.Fatalf("Download dataset file failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Downloaded file does not exist at %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if len(content) == 0 {
		t.Fatal("Downloaded file is empty")
	}

	t.Logf("Successfully downloaded dataset file: %s (%d bytes)", path, len(content))
}

// TestE2ECLIDownload tests the CLI download command
func TestE2ECLIDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	// Build the CLI first
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "hfc")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/hfc")
	buildCmd.Dir = getProjectRoot()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	// Create cache directory
	cacheDir := filepath.Join(tmpDir, "cache")

	// Run the CLI
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "download",
		"--cache-dir", cacheDir,
		"--quiet",
		"gpt2", "config.json")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI download failed: %v\nOutput: %s", err, output)
	}

	// Output should contain the path to the downloaded file
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		t.Fatal("CLI produced no output")
	}

	// Verify the file exists at the output path
	if _, err := os.Stat(outputStr); os.IsNotExist(err) {
		t.Fatalf("Downloaded file does not exist at path from CLI output: %s", outputStr)
	}

	t.Logf("CLI download successful: %s", outputStr)
}

// TestE2ECLIDownloadLocalDir tests the CLI with --local-dir option
func TestE2ECLIDownloadLocalDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	checkNetwork(t)

	// Build the CLI first
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "hfc")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/hfc")
	buildCmd.Dir = getProjectRoot()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	// Create local directory
	localDir := filepath.Join(tmpDir, "local")

	// Run the CLI
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "download",
		"--local-dir", localDir,
		"--quiet",
		"gpt2", "config.json")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI download failed: %v\nOutput: %s", err, output)
	}

	expectedPath := filepath.Join(localDir, "config.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("Downloaded file does not exist at expected path: %s", expectedPath)
	}

	t.Logf("CLI download with local-dir successful: %s", expectedPath)
}

// TestE2ECLIHelp tests the CLI help command
func TestE2ECLIHelp(t *testing.T) {
	// Build the CLI first
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "hfc")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/hfc")
	buildCmd.Dir = getProjectRoot()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	// Test help command
	cmd := exec.Command(binaryPath, "help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI help failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "hfc") {
		t.Error("Help output does not contain 'hfc'")
	}
	if !strings.Contains(outputStr, "download") {
		t.Error("Help output does not contain 'download' command")
	}

	t.Log("CLI help command works correctly")
}

// TestE2ECLIVersion tests the CLI version command
func TestE2ECLIVersion(t *testing.T) {
	// Build the CLI first
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "hfc")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/hfc")
	buildCmd.Dir = getProjectRoot()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	// Test version command
	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI version failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "hfc version") {
		t.Error("Version output does not contain 'hfc version'")
	}

	t.Log("CLI version command works correctly")
}
