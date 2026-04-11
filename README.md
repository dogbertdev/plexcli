# Plex CLI

[![CI](https://github.com/dogbertdev/plexcli/actions/workflows/ci.yml/badge.svg)](https://github.com/dogbertdev/plexcli/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/dogbertdev/plexcli)](https://goreportcard.com/report/github.com/dogbertdev/plexcli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A comprehensive command-line interface for managing Plex Media Server libraries using the [plexgo](https://github.com/lukehagar/plexgo) SDK.

## Features

- **Library Management**: List, filter, and analyze your Plex libraries
- **Media Analysis**: Check for duplicates, missing episodes, quality issues
- **Subtitle & Audio**: Detect missing subtitle languages and audio configuration issues
- **Metadata**: Find items with incomplete metadata
- **Flexible Output**: Support for table, JSON, and TSV output formats

## Installation

### From Source

```bash
git clone https://github.com/dogbertdev/plexcli.git
cd plexcli
make install
```

### Using Go Install

```bash
go install github.com/dogbertdev/plexcli/cmd/plex@latest
```

### Pre-built Binaries

Download from the [releases page](https://github.com/dogbertdev/plexcli/releases) for your platform.

## Configuration

The CLI supports configuration through:

1. **Environment Variables**:
   - `PLEX_SERVER` - Your Plex server URL
   - `PLEX_TOKEN` - Your Plex authentication token
   - `PLEX_USERNAME` - Your Plex username (for authentication)
   - `PLEX_PASSWORD` - Your Plex password (for authentication)
   - `PLEX_CACHE_TTL` - Local library cache TTL in seconds (default: `300`, `0` disables cache)
   - `PLEX_NO_CACHE` - Disable local library cache (`true` or `false`)
   - `PLEX_REFRESH_CACHE` - Bypass cache reads and refresh from Plex (`true` or `false`)

2. **Config File** (~/.config/plexcli/config.json):
   ```json
   {
     "server_url": "http://localhost:32400",
     "token": "your-plex-token"
   }
   ```

3. **CLI Flags** (highest priority):
   ```bash
   plex --server http://localhost:32400 --token your-token unwatched
   ```

### Getting a Plex Token

Use the built-in auth flow:

```bash
# Browser-based login (recommended)
plex auth login --browser

# Username/password login
plex auth login --username your-email@example.com --password 'your-password'
```

This fetches a token from Plex and stores it in your config file.

## Usage

### Global Flags

```bash
plex --help
```

- `--json` - Output JSON to stdout
- `--plain` - Output TSV to stdout (for piping)
- `--color auto|always|never` - Color mode (default: auto)
- `--config PATH` - Path to config file
- `--server URL` - Plex server URL
- `--token TOKEN` - Plex authentication token
- `--timeout SECONDS` - Request timeout in seconds (default: `120`)
- `--cache-ttl SECONDS` - Local library cache TTL in seconds (default: `300`, `0` disables cache)
- `--no-cache` - Disable local library cache for this run
- `--refresh-cache` - Bypass cache reads and refresh local cache for this run
- `-v, --version` - Show version

### Commands

#### Manage Cache

```bash
# Clear local library cache
plex cache clear
```

#### Manage Library Maintenance

```bash
# List all library sections
plex library list

# List only movie sections
plex library list --type movie

# Refresh all sections
plex library update

# Refresh one section
plex library update 1

# Clean all sections (empty trash + maintenance tasks)
plex library clean

# Clean one section (plus global maintenance tasks)
plex library clean 1

# Show active activities/background tasks
plex library status

# Show details for a section
plex library section show 1

# Create or edit a section
plex library section create "Movies" --type movie --agent tv.plex.agents.movie --location /srv/media/movies
plex library section edit 1 --language en-AU --pref languageOverride=en-AU

# Cancel one section refresh or stop all refreshes
plex library refresh cancel 1
plex library refresh stop-all --yes

# Update item metadata and preferences
plex library item edit 12345 --set title="New Title" --lock title
plex library item edit 12345,67890 --add-tag collection="Martial Arts"
plex library item edit 12345 --remove-tag collection="Watch Later"
plex library item prefs set 12345 --pref introDetection=1
plex library section prefs defaults --type movie --agent tv.plex.agents.movie

# Run advanced detectors
plex library detect intros 27018 --force
plex library detect status 999

# Use discovery helpers
plex library discover collections 1
plex library discover autocomplete 1 --query sta

# Download binary helpers
plex library artwork get 12345 --element poster --output /tmp/poster.jpg
plex library media file --url /library/metadata/27017/theme/1771256497 --output /tmp/theme.mp3
plex library media part 38540 1704960287 file.mkv --output /tmp/episode.mkv
plex library media part-index 77 sd --interval 1000 --output /tmp/index.bif
plex library media stream get 77 srt --output /tmp/subtitle.srt

# Attach subtitles from a URL or local file
plex library subtitle add 12345 --url https://example.com/subs/en.srt --language eng
plex library subtitle add 12345 --file ./subs/en.srt --language eng --format srt

# Show similar items for a movie/show
plex library discover similar 12345 --compact

# Show related items as JSON
plex library discover related 12345 --json

# Show Plex metadata matches for an item
plex library discover matches 12345 --compact
```

#### Authenticate

```bash
# Login in browser and fetch/store a Plex token
plex auth login --browser

# Login with username/password and fetch/store a Plex token
plex auth login --username your-email@example.com --password 'your-password'

# Logout and clear stored credentials
plex auth logout

# Discover available servers
plex auth servers

# Select server #1 from discovered list and save it to config
plex auth servers --select 1
```

#### List Unwatched Items

```bash
# Show all unwatched items
plex unwatched

# Show unwatched movies only
plex unwatched --type movie

# Limit results
plex unwatched --limit 20
```

#### List Unmatched Items

```bash
# Show all unmatched items
plex unmatched

# Show unmatched movies only
plex unmatched --type movie

# Limit results
plex unmatched --limit 20
```

#### List Recently Watched

```bash
# Show recently watched (last 7 days)
plex recently-watched

# Show last 30 days
plex recently-watched --days 30

# Output as JSON
plex recently-watched --json
```

#### List Recently Added

```bash
# Show recently added items
plex recently-added

# Show last 30 days, limit to 50 items
plex recently-added --days 30 --limit 50
```

#### Find Duplicates

```bash
# Find all duplicate media files
plex duplicates

# Find duplicates in a specific section
plex duplicates --section 1
```

#### Search the Library

```bash
# Fuzzy search
plex search "Ghost in the Shell"

# Exact title + year resolution
plex search "Gattaca" --exact --year 1997

# Return only the top surviving result
plex search "Moon" --exact --first

# Fail for scripting if the result is ambiguous
plex search "Ghost in the Shell" --exact --fail-ambiguous
```

#### List Movies

```bash
# List movies from a section with useful metadata for curation
plex movies --section 14

# OR-match repeated values within a filter, AND-match across filter types
plex movies --section 14 --actor "Jackie Chan" --actor "Bruce Lee" --country "Hong Kong"

# Emit only rating keys for shell composition
plex movies --section 14 --actor "Jackie Chan" --keys-only

# Control duplicate handling for combined libraries
plex movies --section 14 --title "Enter the Dragon" --dedupe=guid
plex movies --section 14 --title "Enter the Dragon" --dedupe=none
```

#### Build Playlists

```bash
# Create a playlist from rating keys
plex playlist create "Sci-Fi Night" 123 456

# Create a playlist from keys emitted by another command
plex movies --section 14 --actor "Jackie Chan" --keys-only | xargs plex playlist create "Jackie Chan"

# Create a playlist from stdin or a file
plex movies --section 14 --actor "Bruce Lee" --keys-only | plex playlist create "Bruce Lee" --from-stdin
plex playlist create "Sci-Fi Night" --from-file ./keys.txt

# Preview resolved playlist items without modifying Plex
plex playlist create "Sci-Fi Night" --from-file ./keys.txt --dry-run

# Create a playlist by resolving titles directly
plex playlist create "Sci-Fi Night" --query "Solaris" --query "On the Silver Globe"

# Add more items by title
plex playlist add 999 --query "Perfect Blue"

# Create a smart playlist with multiple filters
plex playlist smart "Oshii Essentials" --section 1 --director "Mamoru Oshii" --genre Anime --year-from 1985 --year-to 2004 --unwatched
```

#### Recommend from Seeds

```bash
# Build ranked recommendations from multiple seeds
plex recommend --like "Avalon" --like "Stalker"

# Restrict to one library section and save as a playlist
plex recommend --section 1 --like "Avalon" --like "Stalker" --playlist-name "Avalon x Stalker"
```

#### Check File Paths

```bash
# List all file paths
plex file-paths

# Filter by section
plex file-paths --section 1
```

#### Check Subtitles

```bash
# Find items missing English subtitles
plex subtitles-missing --lang en

# Check for multiple languages
plex subtitles-missing --lang en,de,fr
```

#### Check Audio

```bash
# Find items with stereo audio
plex audio-check --min-channels 2

# Check for specific codecs
plex audio-check --codecs aac,ac3
```

#### Find Missing Episodes

```bash
# Find missing episodes in all TV series
plex episodes-missing

# Check specific show
plex episodes-missing --show "Game of Thrones"

# Check specific season
plex episodes-missing --show "Game of Thrones" --season 1
```

#### Check Video Quality

```bash
# Find items below 1080p
plex quality-check --min-resolution 1080p

# Find 4K HDR content
plex quality-check --min-resolution 4k --hdr
```

#### Check Metadata

```bash
# Find items with incomplete metadata
plex metadata-missing
```

#### Manage Audio and Subtitle Streams

```bash
# Set streams for one season
plex streams set "Demon Slayer" --season 1 --audio japanese --subtitle "full english"

# Set streams for all episodes in all seasons
plex streams set "Demon Slayer" --all-seasons --audio japanese --subtitle "full english"

# Preview changes without modifying Plex
plex streams set "Demon Slayer" --all-seasons --audio japanese --subtitle "full english" --dry-run
```

## Development

### Prerequisites

- Go 1.21+
- Make

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run with coverage
make test-coverage
```

### Project Structure

```
.
├── cmd/plex/           # Main entry point
├── internal/
│   ├── auth/           # Authentication logic
│   ├── cmd/            # Command implementations
│   ├── config/         # Configuration management
│   ├── outfmt/         # Output formatting (table/JSON/TSV)
│   ├── plexclient/     # Plex API client wrapper
│   └── ui/             # UI utilities
├── tests/              # Integration tests
├── go.mod
├── Makefile
└── README.md
```

## Output Formats

### Table (default)
Human-readable formatted tables with headers.

### JSON
Machine-readable JSON output for scripting:

```bash
plex unwatched --json | jq '.[] | select(.year > 2020)'
```

### TSV
Tab-separated values for piping to other tools:

```bash
plex unwatched --plain | cut -f1 | sort
```

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## Acknowledgments

- Built with [plexgo](https://github.com/lukehagar/plexgo) SDK
- CLI framework: [Kong](https://github.com/alecthomas/kong)
- Table formatting: [tablewriter](https://github.com/olekukonko/tablewriter)
