package hfc

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wzshiming/xet/client"
	xetlfs "github.com/wzshiming/xet/lfs"
)

const (
	uploadBatchMaxNumFiles = 256
	lfsContentType         = "application/vnd.git-lfs+json"
	ndjsonContentType      = "application/x-ndjson"
)

type Uploader struct {
	endpoint      string
	token         string
	httpClient    *http.Client
	userAgent     string
	progressFunc  ProgressFunc
	repoID        string
	repoType      string
	revision      string
	localPath     string
	pathInRepo    string
	include       []string
	exclude       []string
	commitMessage string
	maxWorkers    int
}

type UploadOption func(u *Uploader)

type uploadFile struct {
	localPath    string
	pathInRepo   string
	size         int64
	sample       []byte
	sha256       string
	sha1         string
	uploadMode   string
	shouldIgnore bool
	remoteOID    string
}

type preuploadRequest struct {
	Files []preuploadRequestFile `json:"files"`
}

type preuploadRequestFile struct {
	Path   string `json:"path"`
	Sample string `json:"sample"`
	Size   int64  `json:"size"`
}

type preuploadResponse struct {
	Files []preuploadResponseFile `json:"files"`
}

type preuploadResponseFile struct {
	Path         string `json:"path"`
	UploadMode   string `json:"uploadMode"`
	ShouldIgnore bool   `json:"shouldIgnore"`
	OID          string `json:"oid"`
}

type lfsBatchRequest struct {
	Operation string             `json:"operation"`
	Transfers []string           `json:"transfers"`
	Objects   []lfsBatchObject   `json:"objects"`
	HashAlgo  string             `json:"hash_algo"`
	Ref       *lfsBatchReference `json:"ref,omitempty"`
}

type lfsBatchReference struct {
	Name string `json:"name"`
}

type lfsBatchObject struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

type lfsBatchResponse struct {
	Objects  []lfsBatchObjectAction `json:"objects"`
	Errors   []lfsBatchObjectError  `json:"errors"`
	Transfer string                 `json:"transfer"`
}

type lfsBatchObjectAction struct {
	OID     string                       `json:"oid"`
	Size    int64                        `json:"size"`
	Actions *lfsBatchObjectActionDetails `json:"actions,omitempty"`
}

type lfsBatchObjectActionDetails struct {
	Upload *lfsAction `json:"upload,omitempty"`
	Verify *lfsAction `json:"verify,omitempty"`
}

type lfsBatchObjectError struct {
	OID   string `json:"oid"`
	Size  int64  `json:"size"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type lfsAction struct {
	Href   string            `json:"href"`
	Header map[string]string `json:"header"`
}

type multipartCompletionPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type commitResponse struct {
	OID       string `json:"oid"`
	CommitURL string `json:"commitUrl"`
	PRURL     string `json:"prUrl"`
}

func WithUploadToken(token string) UploadOption {
	return func(u *Uploader) {
		u.token = token
	}
}

func WithUploadEndpoint(endpoint string) UploadOption {
	return func(u *Uploader) {
		u.endpoint = endpoint
	}
}

func WithUploadTransport(transport http.RoundTripper) UploadOption {
	return func(u *Uploader) {
		u.httpClient.Transport = transport
	}
}

func WithUploadUserAgent(userAgent string) UploadOption {
	return func(u *Uploader) {
		u.userAgent = userAgent
	}
}

func WithUploadProgressFunc(progressFunc ProgressFunc) UploadOption {
	return func(u *Uploader) {
		u.progressFunc = progressFunc
	}
}

func WithUploadRepoID(repoID string) UploadOption {
	return func(u *Uploader) {
		u.repoID = repoID
	}
}

func WithUploadRepoType(repoType string) UploadOption {
	return func(u *Uploader) {
		u.repoType = repoType
	}
}

func WithUploadRevision(revision string) UploadOption {
	return func(u *Uploader) {
		u.revision = revision
	}
}

func WithUploadLocalPath(localPath string) UploadOption {
	return func(u *Uploader) {
		u.localPath = localPath
	}
}

func WithUploadPathInRepo(pathInRepo string) UploadOption {
	return func(u *Uploader) {
		u.pathInRepo = pathInRepo
	}
}

func WithUploadInclude(include []string) UploadOption {
	return func(u *Uploader) {
		u.include = include
	}
}

func WithUploadExclude(exclude []string) UploadOption {
	return func(u *Uploader) {
		u.exclude = exclude
	}
}

func WithUploadCommitMessage(commitMessage string) UploadOption {
	return func(u *Uploader) {
		u.commitMessage = commitMessage
	}
}

func WithUploadMaxWorkers(maxWorkers int) UploadOption {
	return func(u *Uploader) {
		u.maxWorkers = maxWorkers
	}
}

func NewUploader(opts ...UploadOption) (*Uploader, error) {
	u := &Uploader{
		httpClient:    &http.Client{},
		userAgent:     "hfc/1.0",
		maxWorkers:    4,
		commitMessage: "Upload files with hfc",
	}
	for _, opt := range opts {
		opt(u)
	}

	if u.repoID == "" {
		return nil, fmt.Errorf("repoID is required")
	}
	if u.localPath == "" {
		return nil, fmt.Errorf("localPath is required")
	}
	if u.endpoint == "" {
		u.endpoint = GetEndpoint()
	}
	if u.token == "" {
		u.token = GetToken()
	}
	if u.revision == "" {
		u.revision = DefaultRevision
	}
	if u.repoType == "" {
		u.repoType = repoTypeModel
	}
	if u.maxWorkers <= 0 {
		u.maxWorkers = 1
	}

	return u, nil
}

func (u *Uploader) Upload(ctx context.Context) (string, error) {
	files, err := u.collectFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files to upload after applying filters")
	}

	if err := u.fetchUploadModes(ctx, files); err != nil {
		return "", err
	}

	filtered := files[:0]
	for _, file := range files {
		if file.shouldIgnore {
			continue
		}
		if file.remoteOID != "" && file.remoteOID == file.localOID() {
			continue
		}
		filtered = append(filtered, file)
	}
	files = filtered

	if len(files) == 0 {
		return u.repoURL(), nil
	}

	if err := u.uploadLFS(ctx, files); err != nil {
		return "", err
	}

	commitURL, err := u.createCommit(ctx, files)
	if err != nil {
		return "", err
	}
	return commitURL, nil
}

func (u *Uploader) collectFiles() ([]*uploadFile, error) {
	localPath := expandPath(u.localPath)
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat local path: %w", err)
	}

	if !info.IsDir() {
		pathInRepo := u.cleanRepoPath(u.pathInRepo)
		if pathInRepo == "" {
			pathInRepo = filepath.Base(localPath)
		}
		file, err := newUploadFile(localPath, pathInRepo)
		if err != nil {
			return nil, err
		}
		return []*uploadFile{file}, nil
	}

	basePrefix := u.cleanRepoPath(u.pathInRepo)
	var files []*uploadFile
	err = filepath.WalkDir(localPath, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(localPath, currentPath)
		if err != nil {
			return fmt.Errorf("failed to determine relative path for %s: %w", currentPath, err)
		}
		relPath = filepath.ToSlash(relPath)
		if !u.shouldInclude(relPath) {
			return nil
		}

		pathInRepo := relPath
		if basePrefix != "" {
			pathInRepo = path.Join(basePrefix, relPath)
		}

		file, err := newUploadFile(currentPath, pathInRepo)
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk local path: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].pathInRepo < files[j].pathInRepo
	})

	return files, nil
}

func newUploadFile(localPath, pathInRepo string) (*uploadFile, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", localPath, err)
	}
	defer file.Close()

	sample := make([]byte, 512)
	n, err := file.Read(sample)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read sample from %s: %w", localPath, err)
	}
	sample = sample[:n]

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to reset file pointer for %s: %w", localPath, err)
	}

	sha256Hash := sha256.New()
	sha1Hash := sha1.New()
	size, err := io.Copy(io.MultiWriter(sha256Hash, sha1Hash), file)
	if err != nil {
		return nil, fmt.Errorf("failed to hash %s: %w", localPath, err)
	}

	return &uploadFile{
		localPath:  localPath,
		pathInRepo: pathInRepo,
		size:       size,
		sample:     append([]byte(nil), sample...),
		sha256:     hex.EncodeToString(sha256Hash.Sum(nil)),
		sha1:       hex.EncodeToString(sha1Hash.Sum(nil)),
	}, nil
}

func (u *Uploader) shouldInclude(name string) bool {
	if len(u.include) > 0 {
		matched := false
		for _, pattern := range u.include {
			if matchPattern(name, pattern) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, pattern := range u.exclude {
		if matchPattern(name, pattern) {
			return false
		}
	}

	return true
}

func (u *Uploader) fetchUploadModes(ctx context.Context, files []*uploadFile) error {
	revision := url.PathEscape(u.revision)
	endpoint := fmt.Sprintf("%s/api/%s/%s/preupload/%s", u.endpoint, pluralizeRepoType(u.repoType), u.repoID, revision)

	for start := 0; start < len(files); start += uploadBatchMaxNumFiles {
		end := min(start+uploadBatchMaxNumFiles, len(files))
		chunk := files[start:end]

		payload := preuploadRequest{Files: make([]preuploadRequestFile, 0, len(chunk))}
		for _, file := range chunk {
			payload.Files = append(payload.Files, preuploadRequestFile{
				Path:   file.pathInRepo,
				Sample: base64.StdEncoding.EncodeToString(file.sample),
				Size:   file.size,
			})
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal preupload payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create preupload request: %w", err)
		}
		u.setHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := u.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to get upload modes: %w", err)
		}
		defer resp.Body.Close()

		if err := expectSuccess(resp); err != nil {
			return err
		}

		var result preuploadResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("failed to decode preupload response: %w", err)
		}
		if len(result.Files) != len(chunk) {
			return fmt.Errorf("unexpected preupload response: expected %d files, got %d", len(chunk), len(result.Files))
		}

		resultByPath := make(map[string]preuploadResponseFile, len(result.Files))
		for _, file := range result.Files {
			resultByPath[file.Path] = file
		}

		for _, file := range chunk {
			info, ok := resultByPath[file.pathInRepo]
			if !ok {
				return fmt.Errorf("preupload response missing file %s", file.pathInRepo)
			}
			file.uploadMode = info.UploadMode
			file.shouldIgnore = info.ShouldIgnore
			file.remoteOID = info.OID
		}
	}

	return nil
}

func (u *Uploader) uploadLFS(ctx context.Context, files []*uploadFile) error {
	var lfsFiles []*uploadFile
	for _, file := range files {
		if file.uploadMode == "lfs" {
			lfsFiles = append(lfsFiles, file)
		}
	}
	if len(lfsFiles) == 0 {
		return nil
	}

	for start := 0; start < len(lfsFiles); start += uploadBatchMaxNumFiles {
		end := min(start+uploadBatchMaxNumFiles, len(lfsFiles))
		chunk := lfsFiles[start:end]
		batchResult, err := u.getLFSBatchActions(ctx, chunk)
		if err != nil {
			return err
		}

		// If the server selected xet transfer, upload via CAS.
		if batchResult.Transfer == "xet" {
			if err := u.uploadXetFiles(ctx, chunk); err != nil {
				return err
			}
			continue
		}

		var filtered []lfsBatchObjectAction
		for _, action := range batchResult.Objects {
			if action.Actions == nil || action.Actions.Upload == nil {
				continue
			}
			filtered = append(filtered, action)
		}
		if len(filtered) == 0 {
			continue
		}

		actionsByOID := make(map[string]lfsBatchObjectAction, len(filtered))
		for _, action := range filtered {
			actionsByOID[action.OID] = action
		}

		semaphore := make(chan struct{}, u.maxWorkers)
		errors := make(chan error, len(filtered))
		var wg sync.WaitGroup

		for _, file := range chunk {
			action, ok := actionsByOID[file.sha256]
			if !ok {
				continue
			}

			wg.Add(1)
			go func(file *uploadFile, action lfsBatchObjectAction) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				if err := u.uploadSingleLFS(ctx, file, action); err != nil {
					errors <- err
				}
			}(file, action)
		}

		wg.Wait()
		close(errors)

		var errs []string
		for err := range errors {
			errs = append(errs, err.Error())
		}
		if len(errs) > 0 {
			return fmt.Errorf("failed to upload %d LFS file(s):\n  %s", len(errs), strings.Join(errs, "\n  "))
		}
	}

	return nil
}

func (u *Uploader) getLFSBatchActions(ctx context.Context, files []*uploadFile) (*lfsBatchResponse, error) {
	urlPrefix := repoURLPrefix(u.repoType)
	endpoint := fmt.Sprintf("%s/%s%s.git/info/lfs/objects/batch", u.endpoint, urlPrefix, u.repoID)
	payload := lfsBatchRequest{
		Operation: "upload",
		Transfers: []string{"xet", "basic"},
		Objects:   make([]lfsBatchObject, 0, len(files)),
		HashAlgo:  "sha256",
	}
	if u.revision != "" {
		payload.Ref = &lfsBatchReference{Name: u.revision}
	}
	for _, file := range files {
		payload.Objects = append(payload.Objects, lfsBatchObject{OID: file.sha256, Size: file.size})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LFS batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create LFS batch request: %w", err)
	}
	u.setHeaders(req)
	req.Header.Set("Accept", lfsContentType)
	req.Header.Set("Content-Type", lfsContentType)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request LFS batch actions: %w", err)
	}
	defer resp.Body.Close()

	if err := expectSuccess(resp); err != nil {
		return nil, err
	}

	var result lfsBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode LFS batch response: %w", err)
	}
	if len(result.Errors) > 0 {
		messages := make([]string, 0, len(result.Errors))
		for _, batchErr := range result.Errors {
			messages = append(messages, fmt.Sprintf("%s: %s", batchErr.OID, batchErr.Error.Message))
		}
		return nil, fmt.Errorf("LFS batch API returned errors:\n  %s", strings.Join(messages, "\n  "))
	}

	return &result, nil
}

func (u *Uploader) uploadSingleLFS(ctx context.Context, file *uploadFile, action lfsBatchObjectAction) error {
	if action.Actions == nil || action.Actions.Upload == nil {
		return nil
	}

	u.reportProgress(file.pathInRepo, 0, file.size)

	uploadAction := action.Actions.Upload
	if err := u.uploadLFSSinglePart(ctx, file, uploadAction); err != nil {
		return fmt.Errorf("failed to upload %s via LFS: %w", file.pathInRepo, err)
	}

	if action.Actions.Verify != nil {
		verifyPayload := map[string]any{
			"oid":  file.sha256,
			"size": file.size,
		}
		body, err := json.Marshal(verifyPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal verify payload for %s: %w", file.pathInRepo, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, action.Actions.Verify.Href, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create verify request for %s: %w", file.pathInRepo, err)
		}
		u.setHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := u.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to verify upload for %s: %w", file.pathInRepo, err)
		}
		defer resp.Body.Close()

		if err := expectSuccess(resp); err != nil {
			return fmt.Errorf("failed to verify upload for %s: %w", file.pathInRepo, err)
		}
	}

	return nil
}

func (u *Uploader) uploadLFSSinglePart(ctx context.Context, file *uploadFile, action *lfsAction) error {
	reader, err := os.Open(file.localPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", file.localPath, err)
	}
	defer reader.Close()

	progressReader := &progressReadCloser{
		ReadCloser: reader,
		onProgress: func(downloaded int64) {
			u.reportProgress(file.pathInRepo, downloaded, file.size)
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, action.Href, progressReader)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.ContentLength = file.size
	applyHeaders(req.Header, action.Header)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	return expectSuccess(resp)
}

// uploadXetFiles uploads files through the XET CAS using per-file LFS batch
// negotiation, so the Hub can resolve subsequent lfsFile oid references during commit.
func (u *Uploader) uploadXetFiles(ctx context.Context, files []*uploadFile) error {
	semaphore := make(chan struct{}, u.maxWorkers)
	errs := make(chan error, len(files))
	var wg sync.WaitGroup

	for _, file := range files {
		wg.Add(1)
		go func(f *uploadFile) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			obj := xetlfs.BatchObject{OID: f.sha256, Size: f.size}
			batchResults, err := xetlfs.ResolveOIDUpload(ctx, u.token, xetlfs.Target{
				Endpoint: u.endpoint,
				RepoType: u.repoType,
				RepoID:   u.repoID,
				Revision: u.revision,
			}, obj)
			if err != nil {
				errs <- fmt.Errorf("resolve xet upload for %s: %w", f.pathInRepo, err)
				return
			}

			batchResult := batchResults[0]
			if batchResult.Upload == nil {
				u.reportProgress(f.pathInRepo, f.size, f.size)
				return
			}

			casURL := batchResult.Upload.Header["X-Xet-Cas-Url"]
			casToken := batchResult.Upload.Header["X-Xet-Access-Token"]

			u.reportProgress(f.pathInRepo, 0, f.size)

			file, err := os.Open(f.localPath)
			if err != nil {
				errs <- fmt.Errorf("failed to open %s: %w", f.localPath, err)
				return
			}
			defer file.Close()

			opts := []client.Options{
				client.WithBaseURL(casURL),
				client.WithToken(casToken),
				client.WithNamespace("default"),
				client.WithProgressFunc(func(name string, current, total int64) {
					u.reportProgress(f.pathInRepo, current, total)
				}),
				client.WithConcurrency(u.maxWorkers),
			}

			cli := client.NewClient(opts...)

			_, err = cli.UploadFile(ctx, file)

			if err != nil {
				errs <- fmt.Errorf("xet upload %s: %w", f.pathInRepo, err)
				return
			}

			if batchResult.Verify != nil {
				if err := xetlfs.VerifyObject(ctx, batchResult.Verify, obj); err != nil {
					errs <- fmt.Errorf("verify xet upload for %s: %w", f.pathInRepo, err)
					return
				}
			}

			u.reportProgress(f.pathInRepo, f.size, f.size)
		}(file)
	}

	wg.Wait()
	close(errs)

	var msgs []string
	for e := range errs {
		msgs = append(msgs, e.Error())
	}
	if len(msgs) > 0 {
		return fmt.Errorf("failed to xet-upload %d file(s):\n  %s", len(msgs), strings.Join(msgs, "\n  "))
	}
	return nil
}

func (u *Uploader) createCommit(ctx context.Context, files []*uploadFile) (string, error) {
	payload, err := u.buildCommitPayload(files)
	if err != nil {
		return "", err
	}

	revision := url.PathEscape(u.revision)
	endpoint := fmt.Sprintf("%s/api/%s/%s/commit/%s", u.endpoint, pluralizeRepoType(u.repoType), u.repoID, revision)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create commit request: %w", err)
	}
	u.setHeaders(req)
	req.Header.Set("Content-Type", ndjsonContentType)
	req.Header.Set("Accept", "application/json")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create commit: %w", err)
	}
	defer resp.Body.Close()

	if err := expectSuccess(resp); err != nil {
		return "", err
	}

	var result commitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode commit response: %w", err)
	}
	if result.CommitURL != "" {
		return result.CommitURL, nil
	}
	if result.OID != "" {
		return fmt.Sprintf("%s/%s/commit/%s", u.endpoint, u.repoID, result.OID), nil
	}
	if result.PRURL != "" {
		return result.PRURL, nil
	}
	return u.repoURL(), nil
}

func (u *Uploader) buildCommitPayload(files []*uploadFile) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)

	header := map[string]any{
		"key": "header",
		"value": map[string]any{
			"summary":     u.commitMessage,
			"description": "",
		},
	}
	if err := encoder.Encode(header); err != nil {
		return nil, fmt.Errorf("failed to encode commit header: %w", err)
	}

	for _, file := range files {
		switch file.uploadMode {
		case "regular":
			u.reportProgress(file.pathInRepo, 0, file.size)
			content, err := os.ReadFile(file.localPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", file.localPath, err)
			}
			u.reportProgress(file.pathInRepo, file.size, file.size)
			entry := map[string]any{
				"key": "file",
				"value": map[string]any{
					"content":  base64.StdEncoding.EncodeToString(content),
					"path":     file.pathInRepo,
					"encoding": "base64",
				},
			}
			if err := encoder.Encode(entry); err != nil {
				return nil, fmt.Errorf("failed to encode regular file %s: %w", file.pathInRepo, err)
			}
		case "lfs":
			entry := map[string]any{
				"key": "lfsFile",
				"value": map[string]any{
					"path": file.pathInRepo,
					"algo": "sha256",
					"oid":  file.sha256,
					"size": file.size,
				},
			}
			if err := encoder.Encode(entry); err != nil {
				return nil, fmt.Errorf("failed to encode LFS file %s: %w", file.pathInRepo, err)
			}
		default:
			return nil, fmt.Errorf("unknown upload mode %q for %s", file.uploadMode, file.pathInRepo)
		}
	}

	return buffer.Bytes(), nil
}

func (u *Uploader) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", u.userAgent)
	if u.token != "" {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
}

func (u *Uploader) cleanRepoPath(pathInRepo string) string {
	pathInRepo = strings.TrimSpace(pathInRepo)
	if pathInRepo == "" || pathInRepo == "." {
		return ""
	}
	cleaned := path.Clean(strings.ReplaceAll(pathInRepo, "\\", "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func (u *Uploader) repoURL() string {
	return fmt.Sprintf("%s/%s%s", u.endpoint, repoURLPrefix(u.repoType), u.repoID)
}

func (u *Uploader) reportProgress(name string, uploaded, total int64) {
	if u.progressFunc == nil || total <= 0 {
		return
	}
	u.progressFunc(name, uploaded, total)
}

func repoURLPrefix(repoType string) string {
	switch repoType {
	case repoTypeDataset:
		return "datasets/"
	case repoTypeSpace:
		return "spaces/"
	default:
		return ""
	}
}

func applyHeaders(header http.Header, values map[string]string) {
	for key, value := range values {
		header.Set(key, value)
	}
}

func expectSuccess(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	if message == "" {
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}
	return fmt.Errorf("HTTP error %d: %s: %s", resp.StatusCode, resp.Status, message)
}

type progressReader struct {
	Reader     io.Reader
	onProgress func(downloaded int64)
	read       int64
}

func (p *progressReader) Read(data []byte) (int, error) {
	n, err := p.Reader.Read(data)
	if n > 0 {
		p.read += int64(n)
		if p.onProgress != nil {
			p.onProgress(p.read)
		}
	}
	return n, err
}

type progressReadCloser struct {
	io.ReadCloser
	onProgress func(downloaded int64)
	read       int64
}

func (p *progressReadCloser) Read(data []byte) (int, error) {
	n, err := p.ReadCloser.Read(data)
	if n > 0 {
		p.read += int64(n)
		if p.onProgress != nil {
			p.onProgress(p.read)
		}
	}
	return n, err
}

func (f *uploadFile) localOID() string {
	if f.uploadMode == "lfs" {
		return f.sha256
	}
	return f.sha1
}
