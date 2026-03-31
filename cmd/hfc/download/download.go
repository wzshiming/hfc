package download

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/wzshiming/hfc"
	"github.com/wzshiming/hfc/internal/util"
)

var (
	repoType       string
	revision       string
	include        []string
	exclude        []string
	cacheDir       string
	localDir       string
	force          bool
	token          string
	quiet          bool
	maxWorkers     int
	maxFileWorkers int
	chunkSize      int64
)

var Cmd = &cobra.Command{
	Use:   "download <repo_id> [files...]",
	Short: "Download files from the Hugging Face Hub",
	Long: `Download files from the Hugging Face Hub.

Examples:
  # Download a single file
  hfc download gpt2 config.json

  # Download entire repository
  hfc download gpt2

  # Download with type and revision
  hfc download --repo-type=dataset bigscience/P3 --revision=refs/pr/78

  # Download with patterns
  hfc download gpt2 --include="*.json"

  # Download to local directory
  hfc download gpt2 --local-dir=./models/gpt2

  # Download with authentication token
  hfc download meta-llama/Llama-2-7b --token=hf_***`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDownload,
}

func init() {
	Cmd.Flags().StringVar(&repoType, "repo-type", "model", "Repository type: model, dataset, or space")
	Cmd.Flags().StringVar(&revision, "revision", "main", "Git revision (branch, tag, or commit hash)")
	Cmd.Flags().StringSliceVar(&include, "include", nil, "Glob patterns to include")
	Cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "Glob patterns to exclude")
	Cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Directory to store cached files")
	Cmd.Flags().StringVar(&localDir, "local-dir", "", "Download to local directory instead of cache")
	Cmd.Flags().BoolVar(&force, "force", false, "Force re-download even if cached")
	Cmd.Flags().StringVar(&token, "token", "", "Hugging Face token for authentication")
	Cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress progress output")
	Cmd.Flags().IntVar(&maxWorkers, "max-workers", 8, "Maximum number of concurrent downloads files")
	Cmd.Flags().IntVar(&maxFileWorkers, "max-file-workers", 4, "Maximum number of concurrent downloads per file")
	Cmd.Flags().Int64Var(&chunkSize, "chunk-size", 10*1024*1024, "Chunk size in bytes for downloading files")
}

func runDownload(cmd *cobra.Command, args []string) error {
	repoID := args[0]
	filenames := args[1:]

	opts := []hfc.Option{
		hfc.WithToken(token),
		hfc.WithCacheDir(cacheDir),
		hfc.WithLocalDir(localDir),
		hfc.WithForce(force),
		hfc.WithRepoID(repoID),
		hfc.WithRepoType(repoType),
		hfc.WithRevision(revision),
		hfc.WithInclude(include),
		hfc.WithExclude(exclude),
		hfc.WithFilenames(filenames),
		hfc.WithMaxWorkers(maxWorkers),
		hfc.WithMaxFileWorkers(maxFileWorkers),
		hfc.WithChunkSize(chunkSize),
	}

	if !quiet {
		var lastDownloaded int64
		var lastTime = time.Now()
		var speed float64
		var mut sync.Mutex

		type prog struct {
			total      int64
			downloaded int64
		}

		progresses := map[string]prog{}

		getProgresses := func() (downloaded, total int64) {
			downloaded = 0
			total = 0

			mut.Lock()
			defer mut.Unlock()
			for _, p := range progresses {
				downloaded += p.downloaded
				total += p.total
			}
			return
		}

		printCh := make(chan struct{}, 1)
		defer close(printCh)
		go func() {
			for range printCh {
				now := time.Now()
				elapsed := now.Sub(lastTime).Seconds()

				downloaded, total := getProgresses()
				if lastDownloaded == 0 {
					lastDownloaded = downloaded
				}

				if elapsed <= 5 && downloaded != total {
					continue
				}

				speed = float64(downloaded-lastDownloaded) / elapsed
				lastDownloaded = downloaded
				lastTime = now

				percent := float64(downloaded) / float64(total) * 100
				fmt.Printf("Downloading: %.1f%% (%s / %s) - Speed: %s/s\t\r", percent, util.FormatBytes(downloaded), util.FormatBytes(total), util.FormatBytes(int64(speed)))
			}
		}()

		progressFunc := func(name string, downloaded, total int64) {
			if total <= 0 {
				return
			}

			mut.Lock()
			progresses[name] = prog{total: total, downloaded: downloaded}
			mut.Unlock()

			select {
			case printCh <- struct{}{}:
			default:
			}
		}

		opts = append(opts, hfc.WithProgressFunc(progressFunc))
	}

	downloader, err := hfc.NewDownloader(opts...)
	if err != nil {
		return err
	}

	path, err := downloader.Download(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Println(path)
	return nil
}
