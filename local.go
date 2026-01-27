package hfc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepoPath returns the path to a repository's cache directory.
// The format is: {cache_dir}/{repo_type}s--{owner}--{repo}
func (d *Downloader) repoPath(repoType, repoID string) string {
	// Replace "/" in repo ID with "--"
	safeName := strings.ReplaceAll(repoID, "/", repoIDSeparator)
	return filepath.Join(d.cacheDir, pluralizeRepoType(repoType)+repoIDSeparator+safeName)
}

// SnapshotsPath returns the path to a repository's snapshots directory.
func (d *Downloader) snapshotsPath(repoType, repoID string) string {
	return filepath.Join(d.repoPath(repoType, repoID), "snapshots")
}

// RefsPath returns the path to a repository's refs directory.
func (d *Downloader) refsPath(repoType, repoID string) string {
	return filepath.Join(d.repoPath(repoType, repoID), "refs")
}

// BlobsPath returns the path to a repository's blobs directory.
func (d *Downloader) blobsPath(repoType, repoID string) string {
	return filepath.Join(d.repoPath(repoType, repoID), "blobs")
}

// saveCommitHash saves a commit hash for a given revision.
func (d *Downloader) saveCommitHash(repoType, repoID, revision, commitHash string) error {
	if isCommitHash(revision) {
		return nil
	}

	if !isCommitHash(commitHash) {
		return fmt.Errorf("invalid commit hash: %s", commitHash)
	}

	refsDir := d.refsPath(repoType, repoID)
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		return fmt.Errorf("failed to create refs directory: %w", err)
	}

	refPath := filepath.Join(refsDir, revision)
	err := os.MkdirAll(filepath.Dir(refPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create ref parent directory: %w", err)
	}
	return os.WriteFile(refPath, []byte(commitHash), 0644)
}

// blobPath returns the path to a blob file.
func (d *Downloader) blobPath(repoType, repoID, etag string) string {
	return filepath.Join(d.blobsPath(repoType, repoID), etag)
}

// snapshotPath returns the path to a snapshot directory.
func (d *Downloader) snapshotPath(repoType, repoID, commitHash string) string {
	return filepath.Join(d.snapshotsPath(repoType, repoID), commitHash)
}

// snapshotFilePath returns the path to a file in a snapshot.
func (d *Downloader) snapshotFilePath(repoType, repoID, commitHash, filename string) string {
	return filepath.Join(d.snapshotPath(repoType, repoID, commitHash), filename)
}

// createHardlink creates a hard link from snapshotFile to blobPath.
func (d *Downloader) createHardlink(blobPath, snapshotFile string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(snapshotFile), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if _, err := os.Stat(snapshotFile); err == nil {
		_ = os.Remove(snapshotFile)
	}

	if err := os.Link(blobPath, snapshotFile); err != nil {
		return fmt.Errorf("failed to create hard link: %w", err)
	}
	return nil
}
