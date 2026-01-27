package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/wzshiming/hfc/cmd/hfc/download"
)

var rootCmd = &cobra.Command{
	Use:   "hfc",
	Short: "Hugging Face Hub CLI",
	Long: `hfc - Hugging Face Hub CLI

A command-line tool for downloading files from the Hugging Face Hub.
The cache is compatible with the Python huggingface_hub library.

Environment variables:
  HF_HOME          Hugging Face home directory (default: ~/.cache/huggingface)
  HF_HUB_CACHE     Hub cache directory (default: $HF_HOME/hub)
  HF_TOKEN         Authentication token
  HF_ENDPOINT      API endpoint (default: https://huggingface.co)
  HF_HUB_OFFLINE   Enable offline mode (set to 1)`,
}

func init() {
	rootCmd.AddCommand(download.Cmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
