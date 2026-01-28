package hfc

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/wzshiming/dl"
)

// Downloader handles downloading files from the Hugging Face Hub.
type Downloader struct {
	endpoint             string
	token                string
	httpClient           *http.Client
	httpNoRedirectClient *http.Client
	userAgent            string
	cacheDir             string
	localDir             string
	force                bool
	progressFunc         ProgressFunc
	repoID               string
	repoType             string
	revision             string
	filenames            []string
	include              []string
	exclude              []string
	maxWorkers           int
	maxFileWorkers       int
	chunkSize            int64
	dl                   *dl.Downloader
}

type Option func(d *Downloader)

func WithToken(token string) Option {
	return func(d *Downloader) {
		d.token = token
	}
}

func WithEndpoint(endpoint string) Option {
	return func(d *Downloader) {
		d.endpoint = endpoint
	}
}

func WithTransport(transport http.RoundTripper) Option {
	return func(d *Downloader) {
		d.httpClient.Transport = transport
		d.httpNoRedirectClient.Transport = transport
	}
}

func WithUserAgent(userAgent string) Option {
	return func(d *Downloader) {
		d.userAgent = userAgent
	}
}

func WithCacheDir(cacheDir string) Option {
	return func(d *Downloader) {
		d.cacheDir = cacheDir
	}
}

func WithLocalDir(localDir string) Option {
	return func(d *Downloader) {
		d.localDir = localDir
	}
}

func WithForce(force bool) Option {
	return func(d *Downloader) {
		d.force = force
	}
}

func WithProgressFunc(progressFunc ProgressFunc) Option {
	return func(d *Downloader) {
		d.progressFunc = progressFunc
	}
}

func WithMaxWorkers(maxWorkers int) Option {
	return func(d *Downloader) {
		d.maxWorkers = maxWorkers
	}
}

func WithMaxFileWorkers(maxFileWorkers int) Option {
	return func(d *Downloader) {
		d.maxFileWorkers = maxFileWorkers
	}
}

func WithChunkSize(chunkSize int64) Option {
	return func(d *Downloader) {
		d.chunkSize = chunkSize
	}
}

func WithRepoID(repoID string) Option {
	return func(d *Downloader) {
		d.repoID = repoID
	}
}

func WithRepoType(repoType string) Option {
	return func(d *Downloader) {
		d.repoType = repoType
	}
}

func WithRevision(revision string) Option {
	return func(d *Downloader) {
		d.revision = revision
	}
}

func WithFilenames(filenames []string) Option {
	return func(d *Downloader) {
		d.filenames = filenames
	}
}

func WithInclude(include []string) Option {
	return func(d *Downloader) {
		d.include = include
	}
}

func WithExclude(exclude []string) Option {
	return func(d *Downloader) {
		d.exclude = exclude
	}
}

// NewDownloader creates a new Downloader.
func NewDownloader(opts ...Option) (*Downloader, error) {
	d := &Downloader{
		httpClient: &http.Client{},
		httpNoRedirectClient: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		userAgent:      "hfc/1.0",
		maxWorkers:     8,
		maxFileWorkers: 4,
		chunkSize:      10 * 1024 * 1024, // 10 MB
	}
	for _, opt := range opts {
		opt(d)
	}

	if d.repoID == "" {
		return nil, fmt.Errorf("repoID is required")
	}

	if d.cacheDir == "" {
		d.cacheDir = GetHFHubCache()
	}

	if d.endpoint == "" {
		d.endpoint = GetEndpoint()
	}

	if d.token == "" {
		d.token = GetToken()
	}

	if d.revision == "" {
		d.revision = DefaultRevision
	}

	if d.repoType == "" {
		d.repoType = repoTypeModel
	}

	d.dl = dl.NewDownloader(
		dl.WithHTTPClient(d.httpClient),
		dl.WithConcurrency(d.maxFileWorkers),
		dl.WithChunkSize(d.chunkSize),
		dl.WithProgressFunc(dl.ProgressFunc(d.progressFunc)),
		dl.WithResumeFromOutput(true),
		dl.WithForceTryRange(true),
	)

	return d, nil
}

// downloadFile downloads a single file from the Hub.
func (d *Downloader) downloadFile(ctx context.Context, filename string) (string, error) {
	// Get file metadata
	metadata, err := d.getFileMetadata(ctx, d.repoType, d.repoID, d.revision, filename)
	if err != nil {
		return "", fmt.Errorf("failed to get file metadata: %w", err)
	}

	// Determine blob path and snapshot path
	blobPath := d.blobPath(d.repoType, d.repoID, metadata.Etag)
	snapshotFile := d.snapshotFilePath(d.repoType, d.repoID, metadata.CommitHash, filename)

	// Check if we have a local directory specified
	var finalPath string
	if d.localDir != "" {
		finalPath = filepath.Join(d.localDir, filename)
	} else {
		finalPath = snapshotFile
	}

	// Check if file already exists
	if !d.force {
		if info, err := os.Stat(finalPath); err == nil && info.Size() == metadata.Size {
			return finalPath, nil
		}
	}

	// Check if blob exists in cache
	blobExists := false
	if info, err := os.Stat(blobPath); err == nil && info.Size() == metadata.Size {
		blobExists = true
	}

	if blobExists && !d.force {
		return finalPath, nil
	}

	incomplete := blobPath + ".incomplete"
	err = d.dl.Download(ctx, incomplete, metadata.Location)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}

	err = os.Rename(incomplete, blobPath)
	if err != nil {
		return "", fmt.Errorf("failed to rename incomplete file: %w", err)
	}

	if err := d.createHardlink(blobPath, finalPath); err != nil {
		return "", fmt.Errorf("failed to create hardlink: %w", err)
	}

	return finalPath, nil
}

// Download downloads multiple files from a repository.
func (d *Downloader) Download(ctx context.Context) (string, error) {

	// Get repository info
	repoInfo, err := d.getRepoInfo(ctx, d.repoType, d.repoID, d.revision)
	if err != nil {
		return "", fmt.Errorf("failed to get repository info: %w", err)
	}

	if len(repoInfo.Siblings) == 0 {
		return "", fmt.Errorf("repository is empty")
	}

	// Handle specific filenames
	if len(d.filenames) > 0 {
		if len(d.filenames) == 1 {
			return d.downloadFile(ctx, d.filenames[0])
		}
		repoInfo.Siblings = filterFilesnames(repoInfo.Siblings, d.filenames)
	} else {
		repoInfo.Siblings = filterFiles(repoInfo.Siblings, d.include, d.exclude)
	}

	if len(repoInfo.Siblings) == 0 {
		return "", fmt.Errorf("no files to download after applying filters")
	}

	// Save commit hash
	commitHash := repoInfo.SHA
	if err := d.saveCommitHash(d.repoType, d.repoID, d.revision, commitHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save ref: %v\n", err)
	}

	// Download files concurrently
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, d.maxWorkers)
	errors := make(chan error, len(repoInfo.Siblings))

	for _, file := range repoInfo.Siblings {
		if file.Type == "directory" {
			continue
		}

		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			_, err := d.downloadFile(ctx, filename)
			if err != nil {
				errors <- fmt.Errorf("failed to download %s: %w", filename, err)
			}
		}(file.Path)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, err := range errs {
			errMsgs[i] = err.Error()
		}
		return "", fmt.Errorf("failed to download %d file(s):\n  %s", len(errs), strings.Join(errMsgs, "\n  "))
	}

	// Return the path to the downloaded content
	if d.localDir != "" {
		return d.localDir, nil
	}
	return d.snapshotPath(d.repoType, d.repoID, commitHash), nil
}

func filterFilesnames(files []repoFile, filenames []string) []repoFile {
	filenameSet := make(map[string]struct{})
	for _, name := range filenames {
		filenameSet[name] = struct{}{}
	}

	var result []repoFile
	for _, file := range files {
		if _, ok := filenameSet[file.Path]; ok && file.Type != "directory" {
			result = append(result, file)
		}
	}
	return result
}

// filterFiles filters files based on include and exclude patterns.
func filterFiles(files []repoFile, include, exclude []string) []repoFile {
	var result []repoFile
	for _, file := range files {
		if file.Type == "directory" {
			continue
		}

		// Check include patterns
		if len(include) > 0 {
			matched := false
			for _, pattern := range include {
				if matchPattern(file.Path, pattern) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Check exclude patterns
		excluded := false
		for _, pattern := range exclude {
			if matchPattern(file.Path, pattern) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		result = append(result, file)
	}
	return result
}

// matchPattern matches a filename against a glob pattern.
func matchPattern(name, pattern string) bool {
	// Convert glob pattern to regex
	pattern = strings.ReplaceAll(pattern, ".", "\\.")
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = strings.ReplaceAll(pattern, "?", ".")
	pattern = "^" + pattern + "$"

	matched, err := regexp.MatchString(pattern, name)
	if err != nil {
		return false
	}
	return matched
}
