# NEM - Anime Downloader CLI


NEM is a powerful CLI application for anime enthusiasts. It supports searching for anime titles, retrieving detailed information about anime series and episodes, and downloading episodes in various formats including M3U8 playlists.

![preview](https://github.com/user-attachments/assets/b6f81215-7d2c-4074-b02a-9c042ffda835)



## Usage
```sh
 .\nem.exe
NAME:
   nem - Anime downloader CLI

USAGE:
   nem [global options] [command [command options]]

COMMANDS:
   search    Search anime by title
   details   Get anime details
   episodes  List episodes for anime
   download  Download anime episode
   playlist  Get M3U8 playlist
   help, h   Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --source string, -s string  Animevietsub url (default: [https://animevietsub.ee])
   --help, -h                  show help
```

## Installation

### Github release

Download Windows/Linux/MacOS binary from [Release](https://github.com/ppvan/nem/releases) page.

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

```bash
# Build for Windows
go build -v -o nem.exe ./cmd/cli

# Build for Linux/macOS
go build -v -o nem ./cmd/cli

# Run directly
go run ./cmd/cli
```

## Technology Stack

- **Language**: Go 1.24.6+
- **CLI Framework**: [urfave/cli/v3](https://github.com/urfave/cli)
- **Web Scraping**: [PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery)
- **HTTP Client**: Go's standard `net/http` with custom TLS configuration
