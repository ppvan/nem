# NEM - Anime Downloader CLI

NEM is a powerful CLI application for anime enthusiasts. It supports searching for anime titles, retrieving detailed information about anime series and episodes, and downloading episodes in various formats including M3U8 playlists.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Building](#building)
- [CLI Commands](#cli-commands)
- [Usage Examples](#usage-examples)
- [Requirements](#requirements)

## Features

- 🔍 **Search**: Search for anime by title with customizable result limits
- 📺 **Details**: Get comprehensive information about anime series (text or JSON output)
- 📋 **Episodes**: List all episodes for a given anime
- ⬇️ **Download**: Download single episodes or episode ranges with customizable output directory
- 🎬 **Playlist**: Generate M3U8 playlists for episodes
- 🔧 **Flexible Output**: Support for multiple output formats (text, JSON)

## Installation

### Prerequisites

- Go 1.24.6 or higher
- `task` (for using Taskfile) - optional but recommended

### From Source

1. Clone the repository:
```bash
git clone https://github.com/ppvan/nem.git
cd nem
```

2. Install dependencies:
```bash
go mod download
go mod tidy
```

## Building

### Using Task (Recommended)

```bash
# Build the binary
task build

# Or build and run immediately
task run

# Run all checks and build
task all
```

### Using Go Directly

```bash
# Build for Windows
go build -v -o nem.exe ./cmd/cli

# Build for Linux/macOS
go build -v -o nem ./cmd/cli

# Run directly
go run ./cmd/cli
```

### Build Tasks Available

- `task build` - Build the binary
- `task run` - Run the application directly
- `task test` - Run unit tests
- `task test-cover` - Run tests with coverage report
- `task bench` - Run benchmarks
- `task clean` - Clean build artifacts
- `task fmt` - Format code
- `task vet` - Vet code for issues
- `task check` - Format and vet code
- `task all` - Clean, install, check, test, and build

## CLI Commands

### Global Flags

- `--verbose, -v` - Enable verbose output

### search

Search for anime by title.

**Usage:**
```
anime search [OPTIONS] <query>
```

**Arguments:**
- `<query>` - The anime title to search for

**Options:**
- `--limit, -l` - Maximum number of results (default: 20)

**Example:**
```bash
anime search "Attack on Titan" --limit 10
anime search -l 5 "Death Note"
```

### details

Get detailed information about a specific anime.

**Usage:**
```
anime details [OPTIONS] <id>
```

**Arguments:**
- `<id>` - The anime ID

**Options:**
- `--format, -f` - Output format: `text` or `json` (default: text)

**Example:**
```bash
anime details 123
anime details 123 --format json
anime details -f json 456
```

### episodes

List all episodes for a specific anime.

**Usage:**
```
anime episodes [OPTIONS] <id>
```

**Arguments:**
- `<id>` - The anime ID

**Options:**
- `--format, -f` - Output format: `text` or `json` (default: text)

**Example:**
```bash
anime episodes 123
anime episodes 123 --format json
```

### download

Download one or more episodes of an anime.

**Usage:**
```
anime download [OPTIONS] <id>
```

**Arguments:**
- `<id>` - The anime ID

**Options:**
- `--episode, -e` - Episode number or range (e.g., `5`, `2-11`, `1-12`) [required]
- `--output, -o` - Output directory path [required]

**Example:**
```bash
anime download 123 --episode 5 --output ./downloads
anime download 123 -e 2-11 -o /path/to/downloads
anime download -e 1-12 -o ./anime/series 456
```

### playlist

Generate an M3U8 playlist for a specific episode.

**Usage:**
```
anime playlist [OPTIONS] <id>
```

**Arguments:**
- `<id>` - The anime ID

**Options:**
- `--episode, -e` - Episode number [required]
- `--output, -o` - Output file path for the playlist [required]

**Example:**
```bash
anime playlist 123 --episode 5 --output playlist.m3u8
anime playlist 123 -e 10 -o /path/to/episode.m3u8
```

## Usage Examples

### Example 1: Search for Anime

```bash
$ anime search "Demon Slayer"
# Returns a list of anime matching "Demon Slayer" with IDs and titles
```

### Example 2: Get Anime Details

```bash
$ anime details 42 --format json
# Returns detailed information about anime with ID 42 in JSON format
```

### Example 3: List Episodes

```bash
$ anime episodes 42
# Lists all episodes for anime ID 42
```

### Example 4: Download a Single Episode

```bash
$ anime download 42 --episode 5 --output ./episodes
# Downloads episode 5 of anime 42 to the ./episodes directory
```

### Example 5: Download Episode Range

```bash
$ anime download 42 --episode 1-12 --output /home/user/anime
# Downloads episodes 1 through 12 of anime 42
```

### Example 6: Get M3U8 Playlist

```bash
$ anime playlist 42 --episode 5 --output playlist.m3u8
# Generates an M3U8 playlist for episode 5 and saves it as playlist.m3u8
```

### Example 7: Using Verbose Mode

```bash
$ anime --verbose search "Jujutsu Kaisen" --limit 5
# Performs search with detailed output
```

## Development

### Running Tests

```bash
# Run all tests
task test

# Run tests with coverage
task test-cover
```

### Code Quality

```bash
# Format code
task fmt

# Vet code
task vet

# Run all checks
task check
```

## Technology Stack

- **Language**: Go 1.24.6+
- **CLI Framework**: [urfave/cli/v3](https://github.com/urfave/cli)
- **Web Scraping**: [PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery)
- **HTTP Client**: Go's standard `net/http` with custom TLS configuration