package hfc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wzshiming/xet/hf"
)

// fileMetadata represents metadata about a file on the Hub.
type fileMetadata struct {
	CommitHash string
	Etag       string
	Location   string
	Size       int64
	Xet        *hf.DownloadResolved
}

// repoFile represents a file in a repository.
type repoFile struct {
	Type   string   `json:"type"` // "file" or "directory"
	Path   string   `json:"rfilename"`
	Size   int64    `json:"size"`
	BlobID string   `json:"oid"`
	LFS    *lfsInfo `json:"lfs,omitempty"`
}

// lfsInfo contains LFS-specific information.
type lfsInfo struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

// repoInfo represents repository information.
type repoInfo struct {
	ID       string     `json:"_id"`
	ModelID  string     `json:"modelId,omitempty"`
	SHA      string     `json:"sha"`
	Siblings []repoFile `json:"siblings"`
}

// ProgressFunc is a function called to report download progress.
type ProgressFunc func(name string, downloaded, total int64)

// normalizeEtag removes quotes and "W/" prefix from etag.
func normalizeEtag(etag string) string {
	etag = strings.TrimPrefix(etag, "W/")
	etag = strings.Trim(etag, "\"")
	return etag
}

func (d *Downloader) downloadResolveURL(repoType, repoID, revision, filename string) string {
	// Build the repo path with type prefix
	repoPath := repoID
	if repoType != repoTypeModel {
		repoPath = pluralizeRepoType(repoType) + "/" + repoID
	}

	if revision == "" {
		revision = DefaultRevision
	}

	// URL encode the revision and filename
	encodedRevision := url.PathEscape(revision)
	encodedFilename := url.PathEscape(filename)

	return fmt.Sprintf("%s/%s/resolve/%s/%s", d.endpoint, repoPath, encodedRevision, encodedFilename)
}

func (d *Downloader) apiRevisionURL(repoType, repoID, revision string) string {
	if revision == "" {
		revision = DefaultRevision
	}
	return fmt.Sprintf("%s/api/%s/%s/revision/%s", d.endpoint, pluralizeRepoType(repoType), repoID, url.PathEscape(revision))
}

// getFileMetadata retrieves metadata for a file without downloading it.
func (d *Downloader) getFileMetadata(ctx context.Context, repoType, repoID, revision, filename string) (*fileMetadata, error) {
	fileURL := d.downloadResolveURL(repoType, repoID, revision, filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	d.setHeaders(req)

	resp, err := d.httpNoRedirectClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found: %s/%s", repoID, filename)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: authentication required for %s", repoID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("forbidden: access denied to %s", repoID)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	metadata := &fileMetadata{
		Location: fileURL,
	}

	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode <= http.StatusPermanentRedirect {
		if location := resp.Header.Get("Location"); location != "" {
			u, err := url.Parse(fileURL)
			if err == nil {
				u, err = u.Parse(location)
				if err == nil {
					metadata.Location = u.String()
				}
			}
		}
	}

	// Get commit hash from header
	if commitHash := resp.Header.Get(headerXRepoCommit); commitHash != "" {
		metadata.CommitHash = commitHash
	} else {
		if !isCommitHash(revision) {
			return nil, fmt.Errorf("unable to get commit hash for revision: %s", revision)
		}
		metadata.CommitHash = revision
	}

	// Get etag - prefer linked etag for LFS files
	if linkedEtag := resp.Header.Get(headerXLinkedEtag); linkedEtag != "" {
		metadata.Etag = normalizeEtag(linkedEtag)
	} else if etag := resp.Header.Get("ETag"); etag != "" {
		metadata.Etag = normalizeEtag(etag)
	}

	// Get size - prefer linked size for LFS files
	if linkedSize := resp.Header.Get(headerXLinkedSize); linkedSize != "" {
		if size, err := strconv.ParseInt(linkedSize, 10, 64); err == nil {
			metadata.Size = size
		}
	} else if size := resp.ContentLength; size > 0 {
		metadata.Size = size
	}

	dr, err := hf.ResolveResponse(ctx, nil, resp)
	metadata.Xet = dr

	return metadata, nil
}

// getRepoInfo retrieves repository information.
func (d *Downloader) getRepoInfo(ctx context.Context, repoType, repoID, revision string) (*repoInfo, error) {
	apiURL := d.apiRevisionURL(repoType, repoID, revision)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	d.setHeaders(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repo info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository not found: %s", repoID)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	var info repoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// setHeaders sets common headers for API requests.
func (d *Downloader) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", d.userAgent)
	if d.token != "" {
		req.Header.Set("Authorization", "Bearer "+d.token)
	}
}
