// Package main provides the hfc command-line interface for downloading files
// from HuggingFace Hub.
package main

import (
	"context"
	"fmt"
	"os"

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

	// Download command flags
	downloadCmd.Flags().StringVar(&downloadOpts.repoType, "repo-type", "model", "Repository type: model, dataset, or space")
	downloadCmd.Flags().StringVar(&downloadOpts.revision, "revision", "", "Git revision (branch, tag, or commit hash)")
	downloadCmd.Flags().StringVar(&downloadOpts.cacheDir, "cache-dir", "", "Directory where cached files are stored")
	downloadCmd.Flags().StringVar(&downloadOpts.localDir, "local-dir", "", "Download to this local directory instead of cache")
	downloadCmd.Flags().StringVar(&downloadOpts.token, "token", "", "HuggingFace authentication token")
	downloadCmd.Flags().BoolVar(&downloadOpts.force, "force", false, "Force re-download even if file is cached")
	downloadCmd.Flags().BoolVarP(&downloadOpts.quiet, "quiet", "q", false, "Suppress progress output")
	downloadCmd.Flags().StringVar(&downloadOpts.endpoint, "endpoint", "", "HuggingFace endpoint URL")
}

var downloadOpts struct {
	repoType string
	revision string
	cacheDir string
	localDir string
	token    string
	force    bool
	quiet    bool
	endpoint string
}

var downloadCmd = &cobra.Command{
	Use:   "download <repo_id> <filename> [filenames...]",
	Short: "Download files from HuggingFace Hub",
	Long: `Download files from HuggingFace Hub.

Examples:
  # Download a single file
  hfc download gpt2 config.json

  # Download multiple files
  hfc download gpt2 config.json tokenizer.json

  # Download from a dataset
  hfc download --repo-type dataset squad README.md

  # Download with authentication
  hfc download --token hf_xxx private/model config.json

  # Download to a local directory
  hfc download --local-dir ./models gpt2 config.json

  # Download a specific revision
  hfc download --revision v1.0 gpt2 config.json`,
	Args: cobra.MinimumNArgs(2),
	RunE: runDownload,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hfc version %s\n", version)
	},
}

func runDownload(cmd *cobra.Command, args []string) error {
	repoID := args[0]
	filenames := args[1:]

	// Validate repo type
	validRepoTypes := map[string]bool{"model": true, "dataset": true, "space": true}
	if !validRepoTypes[downloadOpts.repoType] {
		return fmt.Errorf("invalid repo-type: %s. Must be one of: model, dataset, space", downloadOpts.repoType)
	}

	ctx := context.Background()

	for _, filename := range filenames {
		opts := hfc.DownloadOptions{
			RepoID:        repoID,
			Filename:      filename,
			RepoType:      downloadOpts.repoType,
			Revision:      downloadOpts.revision,
			CacheDir:      downloadOpts.cacheDir,
			LocalDir:      downloadOpts.localDir,
			Token:         downloadOpts.token,
			ForceDownload: downloadOpts.force,
			Endpoint:      downloadOpts.endpoint,
		}

		if !downloadOpts.quiet {
			fmt.Printf("Downloading %s from %s...\n", filename, repoID)
		}

		path, err := hfc.Download(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", filename, err)
		}

		fmt.Println(path)
	}

	return nil
}
