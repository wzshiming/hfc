// Package main provides the hfc command-line interface for downloading files
// from HuggingFace Hub.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/wzshiming/hfc"
)

const usage = `hfc - HuggingFace Cache CLI

Usage:
  hfc download [options] <repo_id> [filenames...]

Commands:
  download    Download files from HuggingFace Hub

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
  hfc download --revision v1.0 gpt2 config.json
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "download":
		if err := runDownload(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		fmt.Print(usage)
	case "version", "-v", "--version":
		fmt.Println("hfc version 0.1.0")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

func runDownload(args []string) error {
	fs := flag.NewFlagSet("download", flag.ExitOnError)

	var (
		repoType      = fs.String("repo-type", "model", "Repository type: model, dataset, or space")
		revision      = fs.String("revision", "", "Git revision (branch, tag, or commit hash)")
		cacheDir      = fs.String("cache-dir", "", "Directory where cached files are stored")
		localDir      = fs.String("local-dir", "", "Download to this local directory instead of cache")
		token         = fs.String("token", "", "HuggingFace authentication token")
		forceDownload = fs.Bool("force", false, "Force re-download even if file is cached")
		quiet         = fs.Bool("quiet", false, "Suppress progress output")
		endpoint      = fs.String("endpoint", "", "HuggingFace endpoint URL")
	)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: hfc download [options] <repo_id> [filenames...]\n\n")
		fmt.Fprintf(os.Stderr, "Download files from HuggingFace Hub.\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  repo_id     Repository ID (e.g., 'gpt2' or 'facebook/opt-350m')\n")
		fmt.Fprintf(os.Stderr, "  filenames   Files to download (if omitted, downloads README.md by default)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("repo_id is required")
	}

	repoID := fs.Arg(0)
	filenames := fs.Args()[1:]

	// Validate repo type
	validRepoTypes := map[string]bool{"model": true, "dataset": true, "space": true}
	if !validRepoTypes[*repoType] {
		return fmt.Errorf("invalid repo-type: %s. Must be one of: model, dataset, space", *repoType)
	}

	// At least one filename is required for single file download
	if len(filenames) == 0 {
		return fmt.Errorf("at least one filename is required")
	}

	ctx := context.Background()

	for _, filename := range filenames {
		opts := hfc.DownloadOptions{
			RepoID:        repoID,
			Filename:      filename,
			RepoType:      *repoType,
			Revision:      *revision,
			CacheDir:      *cacheDir,
			LocalDir:      *localDir,
			Token:         *token,
			ForceDownload: *forceDownload,
			Endpoint:      *endpoint,
		}

		if !*quiet {
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
