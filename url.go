package hfc

import (
	"fmt"
	"net/url"
	"strings"
)

// HfHubURLOptions contains options for generating a HuggingFace Hub URL
type HfHubURLOptions struct {
	RepoID    string
	Filename  string
	Subfolder string
	RepoType  string
	Revision  string
	Endpoint  string
}

// HfHubURL constructs the URL of a file from the given information.
//
// The resolved address can either be a huggingface.co-hosted url, or a link to
// Cloudfront (a Content Delivery Network, or CDN) for large files.
func HfHubURL(opts HfHubURLOptions) (string, error) {
	if opts.RepoID == "" {
		return "", fmt.Errorf("repo_id is required")
	}
	if opts.Filename == "" {
		return "", fmt.Errorf("filename is required")
	}

	filename := opts.Filename
	if opts.Subfolder != "" {
		filename = opts.Subfolder + "/" + opts.Filename
	}

	repoType := opts.RepoType
	if repoType == "" {
		repoType = RepoTypeModel
	}

	if !isValidRepoType(repoType) {
		return "", fmt.Errorf("invalid repo type: %s", repoType)
	}

	repoID := opts.RepoID
	if prefix, ok := RepoTypesURLPrefixes[repoType]; ok {
		repoID = prefix + repoID
	}

	revision := opts.Revision
	if revision == "" {
		revision = DefaultRevision
	}

	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = GetEndpoint()
	}

	// Build the URL: {endpoint}/{repo_id}/resolve/{revision}/{filename}
	u := fmt.Sprintf("%s/%s/resolve/%s/%s",
		endpoint,
		repoID,
		url.PathEscape(revision),
		urlEncodePath(filename),
	)

	return u, nil
}

func isValidRepoType(repoType string) bool {
	for _, rt := range RepoTypes {
		if rt == repoType {
			return true
		}
	}
	return false
}

// urlEncodePath encodes the path part of a URL, preserving forward slashes
func urlEncodePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// RepoFolderName returns a serialized version of a hf.co repo name and type,
// safe for disk storage as a single non-nested folder.
//
// Example: models--julien-c--EsperBERTo-small
func RepoFolderName(repoID string, repoType string) string {
	if repoType == "" {
		repoType = RepoTypeModel
	}
	// Remove all `/` occurrences to correctly convert repo to directory name
	parts := append([]string{repoType + "s"}, strings.Split(repoID, "/")...)
	return strings.Join(parts, RepoIDSeparator)
}
