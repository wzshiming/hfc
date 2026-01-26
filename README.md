# hfc - Hugging Face CLI

A command-line tool for downloading files from the [Hugging Face Hub](https://huggingface.co/) written in Go. The cache is compatible with the Python [huggingface_hub](https://github.com/huggingface/huggingface_hub) library.

## Features

- Download models, datasets, and spaces from Hugging Face Hub
- Compatible cache layout with `huggingface_hub` Python library
- Resume interrupted downloads
- Support for include/exclude patterns
- Parallel downloads for faster repository downloads
- Authentication with HF tokens

## Installation

```bash
go install github.com/wzshiming/hfc/cmd/hfc@latest
```

## Usage

### Download a single file

```bash
hfc download gpt2 config.json
```

### Download entire repository

```bash
hfc download gpt2
```

### Download with specific revision

```bash
hfc download gpt2 --revision=refs/pr/78
```

### Download dataset

```bash
hfc download --repo-type=dataset bigscience/P3
```

### Download space

```bash
hfc download --repo-type=space gradio/hello
```

### Download with patterns

```bash
# Include only JSON files
hfc download gpt2 --include="*.json"

# Exclude binary files
hfc download gpt2 --exclude="*.bin"
```

### Download to local directory

```bash
hfc download gpt2 --local-dir=./models/gpt2
```

### Download with authentication

```bash
# Using command line token
hfc download meta-llama/Llama-2-7b --token=hf_***

# Or set environment variable
export HF_TOKEN=hf_***
hfc download meta-llama/Llama-2-7b
```

### Quiet mode (no progress bar)

```bash
hfc download gpt2 config.json --quiet
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `HF_HOME` | Hugging Face home directory | `~/.cache/huggingface` |
| `HF_HUB_CACHE` | Hub cache directory | `$HF_HOME/hub` |
| `HF_TOKEN` | Authentication token | (none) |
| `HF_ENDPOINT` | API endpoint | `https://huggingface.co` |

## License

MIT License - see [LICENSE](LICENSE) for details.
