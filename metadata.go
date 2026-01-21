package hfc

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HfFileMetadata contains information about a file versioned on the Hub.
type HfFileMetadata struct {
	// CommitHash is the commit hash related to the file
	CommitHash string
	// Etag is the etag of the file on the server
	Etag string
	// Location is the URL where to download the file
	Location string
	// Size is the size of the file in bytes
	Size int64
}

// GetHfFileMetadata fetches metadata of a file versioned on the Hub.
func GetHfFileMetadata(fileURL string, token string, timeout time.Duration) (*HfFileMetadata, error) {
	req, err := http.NewRequest(http.MethodHead, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept-Encoding", "identity")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Only follow relative redirects, not CDN redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// For HEAD requests, we handle redirects manually to capture Location header
			return http.ErrUseLastResponse
		},
	}

	// Follow only relative redirects
	resp, err := followRelativeRedirects(client, req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: invalid or missing token")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, &RemoteEntryNotFoundError{URL: fileURL}
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("forbidden: access denied to %s", fileURL)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d for %s", resp.StatusCode, fileURL)
	}

	metadata := &HfFileMetadata{
		Location: fileURL,
	}

	// Get commit hash from custom header
	metadata.CommitHash = resp.Header.Get(HeaderXRepoCommit)

	// Get etag - prefer X-Linked-Etag, fall back to ETag
	etag := resp.Header.Get(HeaderXLinkedEtag)
	if etag == "" {
		etag = resp.Header.Get("ETag")
	}
	metadata.Etag = normalizeEtag(etag)

	// Get size - prefer X-Linked-Size, fall back to Content-Length
	sizeStr := resp.Header.Get(HeaderXLinkedSize)
	if sizeStr == "" {
		sizeStr = resp.Header.Get("Content-Length")
	}
	if sizeStr != "" {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			metadata.Size = size
		}
	}

	// Get location from header if redirected
	if loc := resp.Header.Get("Location"); loc != "" {
		metadata.Location = loc
	} else {
		metadata.Location = resp.Request.URL.String()
	}

	return metadata, nil
}

// followRelativeRedirects follows only relative redirects
func followRelativeRedirects(client *http.Client, req *http.Request) (*http.Response, error) {
	for i := 0; i < 10; i++ {
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			if location == "" {
				return resp, nil
			}

			// Parse the location to check if it's relative
			if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
				// Relative redirect - follow it
				parsedLocation, err := url.Parse(location)
				if err != nil {
					return resp, nil
				}
				newURL := req.URL.ResolveReference(parsedLocation)
				req.URL = newURL
				resp.Body.Close()
				continue
			}

			// Absolute redirect - return response with Location header intact
			return resp, nil
		}

		return resp, nil
	}
	return nil, fmt.Errorf("too many redirects")
}

// normalizeEtag normalizes the ETag HTTP header for use as a filename.
// The HTTP spec allows two forms of ETag:
//
//	ETag: W/"<etag_value>"
//	ETag: "<etag_value>"
func normalizeEtag(etag string) string {
	if etag == "" {
		return ""
	}
	// Remove weak validator prefix
	etag = strings.TrimPrefix(etag, "W/")
	// Remove quotes
	etag = strings.Trim(etag, "\"")
	return etag
}

// RemoteEntryNotFoundError is returned when a file is not found on the Hub
type RemoteEntryNotFoundError struct {
	URL string
}

func (e *RemoteEntryNotFoundError) Error() string {
	return fmt.Sprintf("remote entry not found: %s", e.URL)
}

// LocalEntryNotFoundError is returned when a file is not found in the local cache
type LocalEntryNotFoundError struct {
	Path string
}

func (e *LocalEntryNotFoundError) Error() string {
	return fmt.Sprintf("local entry not found: %s", e.Path)
}


