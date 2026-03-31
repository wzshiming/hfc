package upload

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/wzshiming/hfc"
	"github.com/wzshiming/hfc/internal/util"
)

var (
	uploadRepoType   string
	uploadRevision   string
	uploadInclude    []string
	uploadExclude    []string
	uploadToken      string
	uploadMessage    string
	uploadMaxWorkers int
	uploadQuiet      bool
)

var Cmd = &cobra.Command{
	Use:   "upload <repo_id> [local_path] [path_in_repo]",
	Short: "Upload files to the Hugging Face Hub",
	Long: `Upload a file or a folder to the Hugging Face Hub.

Examples:
  # Upload a single file
  hfc upload username/repo ./README.md

  # Upload a single file to a custom repo path
  hfc upload username/repo ./README.md docs/README.md

  # Upload a folder recursively
  hfc upload username/repo ./dist

  # Upload selected files only
  hfc upload username/repo ./dist assets --include="*.json" --include="*.md"

  # Upload to a dataset repo
  hfc upload --repo-type=dataset username/my-dataset ./data`,
	Args: cobra.RangeArgs(1, 3),
	RunE: runUpload,
}

func init() {
	Cmd.Flags().StringVar(&uploadRepoType, "repo-type", "model", "Repository type: model, dataset, or space")
	Cmd.Flags().StringVar(&uploadRevision, "revision", "main", "Git revision (branch, tag, or commit hash)")
	Cmd.Flags().StringSliceVar(&uploadInclude, "include", nil, "Glob patterns to include")
	Cmd.Flags().StringSliceVar(&uploadExclude, "exclude", nil, "Glob patterns to exclude")
	Cmd.Flags().StringVar(&uploadToken, "token", "", "Hugging Face token for authentication")
	Cmd.Flags().StringVar(&uploadMessage, "message", "Upload files with hfc", "Commit message")
	Cmd.Flags().IntVar(&uploadMaxWorkers, "max-workers", 4, "Maximum number of concurrent LFS uploads")
	Cmd.Flags().BoolVar(&uploadQuiet, "quiet", false, "Suppress progress output")
}

func runUpload(cmd *cobra.Command, args []string) error {
	repoID := args[0]
	localPath := "."
	pathInRepo := ""

	if len(args) >= 2 {
		localPath = args[1]
	}
	if len(args) >= 3 {
		pathInRepo = args[2]
	}

	opts := []hfc.UploadOption{
		hfc.WithUploadToken(uploadToken),
		hfc.WithUploadRepoID(repoID),
		hfc.WithUploadRepoType(uploadRepoType),
		hfc.WithUploadRevision(uploadRevision),
		hfc.WithUploadLocalPath(localPath),
		hfc.WithUploadPathInRepo(pathInRepo),
		hfc.WithUploadInclude(uploadInclude),
		hfc.WithUploadExclude(uploadExclude),
		hfc.WithUploadCommitMessage(uploadMessage),
		hfc.WithUploadMaxWorkers(uploadMaxWorkers),
	}

	if !uploadQuiet {
		var lastUploaded int64
		lastTime := time.Now()
		var speed float64
		var mut sync.Mutex

		type prog struct {
			total    int64
			uploaded int64
		}

		progresses := map[string]prog{}
		getProgresses := func() (uploaded, total int64) {
			mut.Lock()
			defer mut.Unlock()
			for _, progress := range progresses {
				uploaded += progress.uploaded
				total += progress.total
			}
			return uploaded, total
		}

		printCh := make(chan struct{}, 1)
		defer close(printCh)
		go func() {
			for range printCh {
				now := time.Now()
				elapsed := now.Sub(lastTime).Seconds()

				uploaded, total := getProgresses()
				if total <= 0 {
					continue
				}
				if lastUploaded == 0 {
					lastUploaded = uploaded
				}
				if elapsed <= 1 && uploaded != total {
					continue
				}

				speed = float64(uploaded-lastUploaded) / elapsed
				lastUploaded = uploaded
				lastTime = now

				percent := float64(uploaded) / float64(total) * 100
				fmt.Printf("Uploading: %.1f%% (%s / %s) - Speed: %s/s\t\r", percent, util.FormatBytes(uploaded), util.FormatBytes(total), util.FormatBytes(int64(speed)))
			}
		}()

		progressFunc := func(name string, uploaded, total int64) {
			mut.Lock()
			progresses[name] = prog{total: total, uploaded: uploaded}
			mut.Unlock()

			select {
			case printCh <- struct{}{}:
			default:
			}
		}

		opts = append(opts, hfc.WithUploadProgressFunc(progressFunc))
	}

	uploader, err := hfc.NewUploader(opts...)
	if err != nil {
		return err
	}

	result, err := uploader.Upload(cmd.Context())
	if err != nil {
		return err
	}
	if !uploadQuiet {
		fmt.Print("\n")
	}

	fmt.Println(result)
	return nil
}
