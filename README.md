# Plex CLI

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
git clone https://github.com/user/plexcli.git
cd plexcli
make install
```

### Pre-built Binaries

Download from the releases page for your platform.

## Configuration

The CLI supports configuration through:

1. **Environment Variables**:
   - `PLEX_SERVER` - Your Plex server URL
   - `PLEX_TOKEN` - Your Plex authentication token
   - `PLEX_USERNAME` - Your Plex username (for authentication)
   - `PLEX_PASSWORD` - Your Plex password (for authentication)

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

1. Sign in to your Plex account at https://plex.tv
2. Visit https://plex.tv/claim or check your Plex server settings
3. Copy the token for use with this CLI

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
- `-v, --version` - Show version

### Commands

#### List Unwatched Items

```bash
# Show all unwatched items
plex unwatched

# Show unwatched movies only
plex unwatched --type movie

# Limit results
plex unwatched --limit 20
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
