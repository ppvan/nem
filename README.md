# NEM - Anime Downloader App


NEM is a powerful GUI application for anime enthusiasts. It supports searching for anime titles, retrieving detailed information about anime series and episodes, and downloading episodes in various formats including M3U8 playlists.

<img width="1944" height="1200" alt="image" src="https://github.com/user-attachments/assets/8aeaa630-b223-4a97-829c-b3c8366d47d3" />


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
go build -trimpath -ldflags "-s -w -H=windowsgui" -o nem.exe ./app

./nem.exe
```

## Technology Stack

- **Language**: Go 1.24.6+
- **GUI Framework**: [rodrigocfd/windigo]([https://github.com/urfave/cli](https://github.com/rodrigocfd/windigo))
- **Web Scraping**: [PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery)
- **HTTP Client**: [bogdanfinn/tls-client](https://github.com/bogdanfinn/tls-client) with 
