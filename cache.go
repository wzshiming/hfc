package hfc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// REGEX_COMMIT_HASH matches a 40-character hex string (git commit hash)
	regexCommitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// TryToLoadFromCache explores the cache to return the latest cached file for a given revision.
// Returns the path to the cached file, or empty string if not found.
func TryToLoadFromCache(repoID, filename string, cacheDir string, revision string, repoType string) string {
	if revision == "" {
		revision = DefaultRevision
	}
	if repoType == "" {
		repoType = RepoTypeModel
	}
	if cacheDir == "" {
		cacheDir = GetHFHubCache()
	}

	objectID := strings.ReplaceAll(repoID, "/", RepoIDSeparator)
	repoCache := filepath.Join(cacheDir, repoType+"s"+RepoIDSeparator+objectID)

	if _, err := os.Stat(repoCache); os.IsNotExist(err) {
		return ""
	}

	refsDir := filepath.Join(repoCache, "refs")
	snapshotsDir := filepath.Join(repoCache, "snapshots")

	// Resolve refs (for instance to convert main to the associated commit sha)
	if info, err := os.Stat(refsDir); err == nil && info.IsDir() {
		revisionFile := filepath.Join(refsDir, revision)
		if data, err := os.ReadFile(revisionFile); err == nil {
			revision = strings.TrimSpace(string(data))
		}
	}

	// Check if revision folder exists in snapshots
	if _, err := os.Stat(snapshotsDir); os.IsNotExist(err) {
		return ""
	}

	cachedShas, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return ""
	}

	found := false
	for _, entry := range cachedShas {
		if entry.Name() == revision {
			found = true
			break
		}
	}
	if !found {
		return ""
	}

	// Check if file exists in cache
	cachedFile := filepath.Join(snapshotsDir, revision, filename)
	if _, err := os.Stat(cachedFile); err == nil {
		return cachedFile
	}

	return ""
}

// GetPointerPath returns the path to the pointer file in the snapshots directory
func GetPointerPath(storageFolder, revision, relativeFilename string) string {
	return filepath.Join(storageFolder, "snapshots", revision, relativeFilename)
}

// GetBlobPath returns the path to the blob file
func GetBlobPath(storageFolder, etag string) string {
	return filepath.Join(storageFolder, "blobs", etag)
}

// GetRefPath returns the path to the ref file
func GetRefPath(storageFolder, revision string) string {
	return filepath.Join(storageFolder, "refs", revision)
}

// IsCommitHash checks if a string is a valid git commit hash
func IsCommitHash(revision string) bool {
	return regexCommitHash.MatchString(revision)
}

// CacheCommitHashForSpecificRevision caches the mapping between a revision and commit hash
func CacheCommitHashForSpecificRevision(storageFolder, revision, commitHash string) error {
	if revision == commitHash {
		return nil
	}

	refPath := GetRefPath(storageFolder, revision)

	// Check if ref already matches
	if data, err := os.ReadFile(refPath); err == nil {
		if strings.TrimSpace(string(data)) == commitHash {
			return nil
		}
	}

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		return err
	}

	// Write the commit hash
	return os.WriteFile(refPath, []byte(commitHash), 0644)
}

// CreateSymlink creates a symbolic link from blob to pointer.
// If symlinks are not supported, the file is copied instead.
func CreateSymlink(src, dst string, newBlob bool) error {
	// Remove existing file/symlink
	os.Remove(dst)

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Try to use relative path for symlink
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	relativeSrc, err := filepath.Rel(filepath.Dir(absDst), absSrc)
	if err != nil {
		relativeSrc = absSrc
	}

	// Try to create symlink
	if err := os.Symlink(relativeSrc, absDst); err == nil {
		return nil
	}

	// Symlink failed - fallback to copy
	if newBlob {
		// Move file instead of copy for new blobs
		return os.Rename(src, dst)
	}

	// Copy file
	return copyFile(src, dst)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
