package hfc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DownloadOptions contains options for downloading a file from HuggingFace Hub
type DownloadOptions struct {
	// RepoID is the repository ID (e.g., "facebook/opt-350m")
	RepoID string
	// Filename is the name of the file to download
	Filename string
	// Subfolder is an optional subfolder within the repository
	Subfolder string
	// RepoType is the type of repository ("model", "dataset", "space")
	RepoType string
	// Revision is the git revision (branch, tag, or commit hash)
	Revision string
	// CacheDir is the directory for caching downloaded files
	CacheDir string
	// LocalDir if set, downloads to this directory instead of cache
	LocalDir string
	// Token is the HuggingFace authentication token
	Token string
	// ForceDownload forces re-downloading even if cached
	ForceDownload bool
	// LocalFilesOnly only uses local cached files
	LocalFilesOnly bool
	// Endpoint is the HuggingFace endpoint URL
	Endpoint string
	// EtagTimeout is the timeout for fetching metadata
	EtagTimeout time.Duration
	// DownloadTimeout is the timeout for downloading files
	DownloadTimeout time.Duration
}

// Download downloads a file from HuggingFace Hub.
// Returns the path to the downloaded file.
func Download(ctx context.Context, opts DownloadOptions) (string, error) {
	// Set defaults
	if opts.CacheDir == "" {
		opts.CacheDir = GetHFHubCache()
	}
	if opts.Revision == "" {
		opts.Revision = DefaultRevision
	}
	if opts.RepoType == "" {
		opts.RepoType = RepoTypeModel
	}
	if opts.EtagTimeout == 0 {
		opts.EtagTimeout = time.Duration(GetEtagTimeout()) * time.Second
	}
	if opts.DownloadTimeout == 0 {
		opts.DownloadTimeout = time.Duration(GetDownloadTimeout()) * time.Second
	}
	if opts.Token == "" {
		opts.Token = GetHFToken()
	}

	// Validate repo type
	if !isValidRepoType(opts.RepoType) {
		return "", fmt.Errorf("invalid repo type: %s. Accepted types: %v", opts.RepoType, RepoTypes)
	}

	filename := opts.Filename
	if opts.Subfolder != "" {
		filename = opts.Subfolder + "/" + opts.Filename
	}

	// If LocalDir is provided, download to local directory
	if opts.LocalDir != "" {
		return downloadToLocalDir(ctx, opts, filename)
	}

	return downloadToCacheDir(ctx, opts, filename)
}

func downloadToCacheDir(ctx context.Context, opts DownloadOptions, filename string) (string, error) {
	storageFolder := filepath.Join(opts.CacheDir, RepoFolderName(opts.RepoID, opts.RepoType))
	locksDir := filepath.Join(opts.CacheDir, ".locks")

	// Cross-platform transcription of filename
	relativeFilename := filepath.Join(strings.Split(filename, "/")...)

	// If user provides a commit hash and file exists, shortcut
	if IsCommitHash(opts.Revision) {
		pointerPath := GetPointerPath(storageFolder, opts.Revision, relativeFilename)
		if _, err := os.Stat(pointerPath); err == nil && !opts.ForceDownload {
			return pointerPath, nil
		}
	}

	// Try to get metadata from server
	metadata, headErr := getMetadataOrCatchError(ctx, opts, filename, storageFolder, relativeFilename)

	// If we couldn't get metadata, try to find local file
	if headErr != nil {
		if !opts.ForceDownload {
			cachedPath := tryToFindCachedFile(opts, storageFolder, relativeFilename)
			if cachedPath != "" {
				return cachedPath, nil
			}
		}

		if opts.LocalFilesOnly {
			return "", &LocalEntryNotFoundError{Path: relativeFilename}
		}
		return "", headErr
	}

	// Create blob and pointer paths
	blobPath := GetBlobPath(storageFolder, metadata.Etag)
	pointerPath := GetPointerPath(storageFolder, metadata.CommitHash, relativeFilename)

	// Create directories
	if err := os.MkdirAll(filepath.Dir(blobPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create blob directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pointerPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create pointer directory: %w", err)
	}

	// Cache the commit hash for this revision
	if err := CacheCommitHashForSpecificRevision(storageFolder, opts.Revision, metadata.CommitHash); err != nil {
		// Non-fatal error, just log
	}

	// Pointer already exists -> return immediately
	if !opts.ForceDownload {
		if _, err := os.Stat(pointerPath); err == nil {
			return pointerPath, nil
		}
	}

	// Blob exists but pointer must be created
	if !opts.ForceDownload {
		if _, err := os.Stat(blobPath); err == nil {
			lockPath := filepath.Join(locksDir, RepoFolderName(opts.RepoID, opts.RepoType), metadata.Etag+".lock")
			if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
				return "", err
			}
			if err := CreateSymlink(blobPath, pointerPath, false); err != nil {
				return "", err
			}
			return pointerPath, nil
		}
	}

	// Download the file
	lockPath := filepath.Join(locksDir, RepoFolderName(opts.RepoID, opts.RepoType), metadata.Etag+".lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return "", err
	}

	incompletePath := blobPath + ".incomplete"
	if err := downloadToTmpAndMove(ctx, incompletePath, blobPath, metadata.Location, opts.Token, metadata.Size, opts.ForceDownload); err != nil {
		return "", err
	}

	// Create symlink from blob to pointer
	if err := CreateSymlink(blobPath, pointerPath, true); err != nil {
		return "", err
	}

	return pointerPath, nil
}

func downloadToLocalDir(ctx context.Context, opts DownloadOptions, filename string) (string, error) {
	localPath := filepath.Join(opts.LocalDir, filepath.Join(strings.Split(filename, "/")...))

	// Check if file already exists
	if _, err := os.Stat(localPath); err == nil && !opts.ForceDownload {
		return localPath, nil
	}

	// Get metadata
	metadata, headErr := getMetadataOrCatchError(ctx, opts, filename, "", "")
	if headErr != nil {
		if opts.LocalFilesOnly {
			return "", &LocalEntryNotFoundError{Path: localPath}
		}
		return "", headErr
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	incompletePath := localPath + ".incomplete"
	if err := downloadToTmpAndMove(ctx, incompletePath, localPath, metadata.Location, opts.Token, metadata.Size, opts.ForceDownload); err != nil {
		return "", err
	}

	return localPath, nil
}

func getMetadataOrCatchError(ctx context.Context, opts DownloadOptions, filename, storageFolder, relativeFilename string) (*HfFileMetadata, error) {
	if opts.LocalFilesOnly {
		return nil, fmt.Errorf("local_files_only is set, cannot fetch metadata")
	}

	if IsOfflineMode() {
		return nil, fmt.Errorf("offline mode is enabled")
	}

	fileURL, err := HfHubURL(HfHubURLOptions{
		RepoID:   opts.RepoID,
		Filename: filename,
		RepoType: opts.RepoType,
		Revision: opts.Revision,
		Endpoint: opts.Endpoint,
	})
	if err != nil {
		return nil, err
	}

	metadata, err := GetHfFileMetadata(fileURL, opts.Token, opts.EtagTimeout)
	if err != nil {
		return nil, err
	}

	if metadata.CommitHash == "" {
		return nil, fmt.Errorf("commit hash not found in response headers")
	}
	if metadata.Etag == "" {
		return nil, fmt.Errorf("etag not found in response headers")
	}

	return metadata, nil
}

func tryToFindCachedFile(opts DownloadOptions, storageFolder, relativeFilename string) string {
	var commitHash string

	if IsCommitHash(opts.Revision) {
		commitHash = opts.Revision
	} else {
		refPath := GetRefPath(storageFolder, opts.Revision)
		if data, err := os.ReadFile(refPath); err == nil {
			commitHash = strings.TrimSpace(string(data))
		}
	}

	if commitHash != "" {
		pointerPath := GetPointerPath(storageFolder, commitHash, relativeFilename)
		if _, err := os.Stat(pointerPath); err == nil {
			return pointerPath
		}
	}

	return ""
}

func downloadToTmpAndMove(ctx context.Context, incompletePath, destinationPath, downloadURL, token string, expectedSize int64, forceDownload bool) error {
	// Check if destination already exists
	if _, err := os.Stat(destinationPath); err == nil && !forceDownload {
		return nil
	}

	// Remove incomplete file if force download
	if forceDownload {
		os.Remove(incompletePath)
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(incompletePath), 0755); err != nil {
		return err
	}

	// Open or create incomplete file
	f, err := os.OpenFile(incompletePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open incomplete file: %w", err)
	}
	defer f.Close()

	// Get current file size for resume
	info, err := f.Stat()
	if err != nil {
		return err
	}
	resumeSize := info.Size()

	// Download the file
	if err := httpGet(ctx, downloadURL, f, resumeSize, token, expectedSize); err != nil {
		return err
	}

	// Close file before rename
	f.Close()

	// Move to destination
	if err := os.Rename(incompletePath, destinationPath); err != nil {
		return fmt.Errorf("failed to move file to destination: %w", err)
	}

	return nil
}

func httpGet(ctx context.Context, fileURL string, writer io.Writer, resumeSize int64, token string, expectedSize int64) error {
	// Skip download if already complete
	if expectedSize > 0 && resumeSize >= expectedSize {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}

	// Set headers
	if token != "" && !isExternalURL(fileURL) {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if resumeSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeSize))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// File already complete
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP error %d while downloading %s", resp.StatusCode, fileURL)
	}

	// Copy response body to writer
	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("error while downloading: %w", err)
	}

	// Verify size if expected
	if seeker, ok := writer.(io.Seeker); ok && expectedSize > 0 {
		currentPos, _ := seeker.Seek(0, io.SeekCurrent)
		if currentPos != expectedSize && currentPos != resumeSize+getContentLength(resp) {
			// Size mismatch - this might be okay if we're resuming
		}
	}

	return nil
}

func getContentLength(resp *http.Response) int64 {
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil {
			return size
		}
	}
	return 0
}

func isExternalURL(fileURL string) bool {
	u, err := url.Parse(fileURL)
	if err != nil {
		return false
	}
	endpoint := GetEndpoint()
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	return u.Host != endpointURL.Host
}
