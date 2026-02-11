# Agent Notes for plex-cli

## Bugs
- Add regression test when it fits

## Project Overview
A CLI tool for interacting with Plex Media Server. Uses Kong for CLI parsing and outputs in table/json/tsv formats.

## Key Patterns

### CLI Command Structure
Commands live in `internal/cmd/` and follow this pattern:
```go
type MyCmd struct {
    Arg    string `arg:"" help:"Description"`
    Flag   string `help:"Description" default:"value" enum:"a,b,c"`
    Output string `help:"Output format" default:"table" enum:"table,json,tsv"`
}

func (c *MyCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
    // Use NewClientContext to handle auth/client setup
    cc, err := NewClientContext(cfg)
    if err != nil {
        return err
    }
    defer cc.Cancel()

    // Call API methods using cc.Client and cc.Ctx
    results, err := cc.Client.SomeMethod(cc.Ctx, args...)
    
    // Format and output results
    return c.output(u.Out(), results)
}
```

### ClientContext Helper (internal/cmd/common.go)
The `NewClientContext()` helper consolidates ~20 lines of boilerplate:
- Validates config
- Gets auth token via `auth.GetToken()`
- Creates `plexclient.NewClient()` with timeout
- Returns `ClientContext{Client, Ctx, Cancel, Timeout}`
- **Always call `defer cc.Cancel()`** after creating

### Wiring Up Commands
1. Add command to CLI struct in `cmd/plex/main.go`
2. Add to `applyOutputFormat()` if it has an Output field

### Plex API Notes
- **Search**: `GET /hubs/search?query=<url-encoded>&limit=N`
- **Show episodes**: `GET /library/metadata/<showRatingKey>/allLeaves` - returns all episodes
- **Playlists**: 
  - List: `GET /playlists`
  - Create: `POST /playlists?title=<name>&type=video&smart=0&uri=<uri>`
  - Add items: `PUT /playlists/<id>/items?uri=<uri>`
  - Show items: `GET /playlists/<id>/items`
- **Playlist URI format**: `server://<serverUUID>/com.plexapp.plugins.library/library/metadata/<key1>,<key2>,...`
- **Important**: Creating a playlist requires at least one item (can't create empty playlists)
- **URL encode** all query parameters (query strings, titles, URIs)

### Environment Variables
```
PLEX_SERVER=http://192.168.0.38:32400
PLEX_TOKEN=<token>
```

## Playlist-from-URL Workflow

The main use case: User provides a URL with an episode list, create a Plex playlist.

### Commands
```bash
# Search for content
plex search "Show Name" --type episode --limit 50

# List all episodes of a show
plex episodes "Show Name"

# Filter to specific episodes and get rating keys
plex episodes "Show Name" --filter "S1E1,S2E5,S3E10" --keys-only

# Create playlist with those episodes
plex playlist create "Playlist Name" $(plex episodes "Show" --filter "S1E1,S2E5" --keys-only)
```

### Workflow Steps
1. Fetch URL and parse episode list into S##E## format
2. Run: `plex episodes "Show" --filter "S1E1,S2E3,..." --keys-only`
3. Pipe output to: `plex playlist create "Name" <keys>`

## Testing Commands
```bash
# With env vars from .env file
source .env && ./plex-cli --server "http://$PLEX_SERVER" --token "$PLEX_TOKEN" <command>

# Build binary
go build -o plex-cli ./cmd/plex

# Build all packages
go build ./...

# Run tests
go test ./...

# Run tests verbose
go test ./... -v
```

## Architecture Notes

### Client (internal/plexclient/client.go)
- Uses a **shared `http.Client`** for connection pooling (don't create per-request)
- Timeout configured via `WithTimeout()` option
- All API methods take `context.Context` as first param for cancellation

### Code Style (TigerStyle)
- **Safety > Performance > Developer Experience** (in that order)
- Functions should be <70 lines
- Use stdlib over custom implementations (e.g., `strings.Join` not custom `joinStrings`)
- Remove dead code aggressively
- Pre-allocate slices when size is known: `make([]T, 0, len(source))`

### Common Refactoring Pitfalls
- When extracting helpers, update imports in all affected files
- Remove unused imports after refactoring (Go compiler will catch these)
- Test with real server after refactoring, not just unit tests

## Playlists Created (Examples)
- SG-1 Key Episodes (113 eps) - GateWorld mythology guide
- X-Files Best MOTW (28 eps) - Monster-of-the-week episodes  
- X-Files Essential (103 eps) - Reddit mythology guide
- TNG in 40 Hours (44 eps) - Essential TNG episodes
