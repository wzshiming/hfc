// Package main provides the hfc command-line interface for downloading files
// from HuggingFace Hub.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wzshiming/hfc"
)

var version = "0.1.0"

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "hfc",
	Short: "HuggingFace Cache CLI",
	Long: `hfc - HuggingFace Cache CLI

A command-line tool for downloading files from HuggingFace Hub.
The cache is compatible with Python's huggingface_hub library.`,
}

func init() {
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(versionCmd)

	// Download command flags - matching Python huggingface-cli
	downloadCmd.Flags().StringVar(&downloadOpts.repoType, "repo-type", "model", "The type of repository (model, dataset, or space)")
	downloadCmd.Flags().StringVar(&downloadOpts.revision, "revision", "", "Git revision id which can be a branch name, a tag, or a commit hash")
	downloadCmd.Flags().StringSliceVar(&downloadOpts.include, "include", nil, "Glob patterns to include from files to download. eg: *.json")
	downloadCmd.Flags().StringSliceVar(&downloadOpts.exclude, "exclude", nil, "Glob patterns to exclude from files to download")
	downloadCmd.Flags().StringVar(&downloadOpts.cacheDir, "cache-dir", "", "Directory where to save files")
	downloadCmd.Flags().StringVar(&downloadOpts.localDir, "local-dir", "", "If set, the downloaded file will be placed under this directory. Check out https://huggingface.co/docs/huggingface_hub/guides/download#download-files-to-a-local-folder for more details.")
	downloadCmd.Flags().BoolVar(&downloadOpts.forceDownload, "force-download", false, "If True, the files will be downloaded even if they are already cached")
	downloadCmd.Flags().BoolVar(&downloadOpts.dryRun, "dry-run", false, "If True, perform a dry run without actually downloading the file")
	downloadCmd.Flags().StringVar(&downloadOpts.token, "token", "", "A User Access Token generated from https://huggingface.co/settings/tokens")
	downloadCmd.Flags().BoolVarP(&downloadOpts.quiet, "quiet", "q", false, "If True, progress bars are disabled and only the path to the download files is printed")
	downloadCmd.Flags().IntVar(&downloadOpts.maxWorkers, "max-workers", 8, "Maximum number of workers to use for downloading files. Default is 8.")
	downloadCmd.Flags().StringVar(&downloadOpts.endpoint, "endpoint", "", "HuggingFace endpoint URL")
}

var downloadOpts struct {
	repoType      string
	revision      string
	include       []string
	exclude       []string
	cacheDir      string
	localDir      string
	forceDownload bool
	dryRun        bool
	token         string
	quiet         bool
	maxWorkers    int
	endpoint      string
}

var downloadCmd = &cobra.Command{
	Use:   "download [OPTIONS] REPO_ID [FILENAMES]...",
	Short: "Download files from the Hub",
	Long: `Download files from the Hub.

Arguments:
  REPO_ID         The ID of the repo (e.g. 'username/repo-name').  [required]
  [FILENAMES]...  Files to download (e.g. 'config.json', 'data/metadata.jsonl').

Examples:
  # Download a single file
  hfc download gpt2 config.json

  # Download multiple files
  hfc download gpt2 config.json tokenizer.json

  # Download from a dataset
  hfc download --repo-type dataset squad README.md

  # Download entire repo with filters (requires explicit filenames for now)
  hfc download gpt2 config.json tokenizer.json vocab.json

  # Download with authentication
  hfc download --token hf_xxx private/model config.json

  # Download to a local directory
  hfc download --local-dir ./models gpt2 config.json

  # Download a specific revision
  hfc download --revision v1.0 gpt2 config.json

  # Dry run to see what would be downloaded
  hfc download --dry-run gpt2 config.json

  # Quiet mode - only print paths
  hfc download --quiet gpt2 config.json`,
	Args: cobra.MinimumNArgs(1),
	RunE: runDownload,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hfc version %s\n", version)
	},
}

// warn prints a warning message to stderr if not in quiet mode
func warn(format string, args ...interface{}) {
	if !downloadOpts.quiet {
		fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
	}
}

func runDownload(cmd *cobra.Command, args []string) error {
	repoID := args[0]
	filenames := args[1:]

	// Validate repo type
	validRepoTypes := map[string]bool{"model": true, "dataset": true, "space": true}
	if !validRepoTypes[downloadOpts.repoType] {
		return fmt.Errorf("invalid repo-type: %s. Must be one of: model, dataset, space", downloadOpts.repoType)
	}

	// Match Python behavior: warn if patterns are ignored when filenames are set
	if len(filenames) > 0 {
		if len(downloadOpts.include) > 0 {
			warn("Ignoring `--include` since filenames have been explicitly set.")
		}
		if len(downloadOpts.exclude) > 0 {
			warn("Ignoring `--exclude` since filenames have been explicitly set.")
		}
	}

	// If no filenames specified, we need at least one filename for now
	// (snapshot_download functionality not yet implemented)
	if len(filenames) == 0 {
		return fmt.Errorf("at least one filename is required. Downloading entire repos with --include/--exclude patterns is not yet supported")
	}

	ctx := context.Background()

	// Download files
	for _, filename := range filenames {
		opts := hfc.DownloadOptions{
			RepoID:        repoID,
			Filename:      filename,
			RepoType:      downloadOpts.repoType,
			Revision:      downloadOpts.revision,
			CacheDir:      downloadOpts.cacheDir,
			LocalDir:      downloadOpts.localDir,
			Token:         downloadOpts.token,
			ForceDownload: downloadOpts.forceDownload,
			Endpoint:      downloadOpts.endpoint,
		}

		if downloadOpts.dryRun {
			// Match Python dry-run output format
			fmt.Printf("[dry-run] Would download %s from %s\n", filename, repoID)
			continue
		}

		if !downloadOpts.quiet {
			fmt.Fprintf(os.Stderr, "Downloading %s from %s...\n", filename, repoID)
		}

		path, err := hfc.Download(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", filename, err)
		}

		// Print only the path (consistent with Python quiet behavior)
		fmt.Println(path)
	}

	return nil
}

// filterByPatterns filters filenames by excluding those matching any of the exclude patterns
func filterByPatterns(filenames []string, excludePatterns []string) []string {
	var result []string
	for _, filename := range filenames {
		excluded := false
		for _, pattern := range excludePatterns {
			if matched, _ := filepath.Match(pattern, filename); matched {
				excluded = true
				break
			}
			// Also check if pattern matches basename
			if matched, _ := filepath.Match(pattern, filepath.Base(filename)); matched {
				excluded = true
				break
			}
			// Check for simple substring match for patterns like "*.json"
			if strings.HasPrefix(pattern, "*") {
				suffix := strings.TrimPrefix(pattern, "*")
				if strings.HasSuffix(filename, suffix) {
					excluded = true
					break
				}
			}
		}
		if !excluded {
			result = append(result, filename)
		}
	}
	return result
}
