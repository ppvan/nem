# NEM - Anime Downloader App


NEM is a powerful GUI application for anime enthusiasts. It supports searching for anime titles, retrieving detailed information about anime series and episodes, and downloading episodes in various formats including M3U8 playlists.

<img width="1944" height="1290" alt="image" src="https://github.com/user-attachments/assets/60e5da81-74c3-4eec-a74d-aac0ca0e2836" />


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
