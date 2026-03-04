# Development Notes

This document captures implementation details that are easy to forget and are validated against the current codebase.

## Current CLI Surface

Root command is defined in `cmd/plex/main.go` and currently includes:

- `unwatched`
- `unmatched`
- `recently-watched`
- `recently-added`
- `duplicates`
- `file-paths`
- `subtitles-missing`
- `audio-check`
- `episodes-missing`
- `quality-check`
- `metadata-missing`
- `search`
- `libraries`
- `server-info`
- `playlist` (`list`, `create`, `smart`, `add`, `show`, `delete`)
- `episodes`
- `movies`
- `directors`
- `match`
- `editions`
- `streams`
- `watch` (`now`, `history`, `stats`)

Global output flags:

- `--json` sets all command `Output` fields to `json`
- `--plain` sets all command `Output` fields to `tsv`

When adding a new command with an `Output` field, update `applyOutputFormat()` in `cmd/plex/main.go`.

## Command Pattern

Commands in `internal/cmd/` should use `NewClientContext(cfg)` to centralize config validation, auth, and client creation.

Canonical pattern:

```go
cc, err := NewClientContext(cfg)
if err != nil {
    return err
}
defer cc.Cancel()

// use cc.Client with cc.Ctx
```

`NewClientContext` behavior (`internal/cmd/common.go`):

- Validates config (`cfg.Validate()`)
- Authenticates via `auth.GetToken()`
- Creates `plexclient.Client` with configured timeout
- Returns cancelable request context

## Config + Auth Behavior

From `internal/config/config.go` and `internal/auth/auth.go`:

- Config path: `~/.config/plexcli/config.json`
- Env overrides: `PLEX_SERVER`, `PLEX_TOKEN`, `PLEX_USERNAME`, `PLEX_PASSWORD`
- URL normalization: bare host values are normalized to `http://...`
- Validation requires server URL and either:
  - token, or
  - username/password
- Auth flow prefers token validation, then falls back to username/password if present

## Plex API Notes (Current Implementation)

Search and playlists are direct HTTP calls in `internal/plexclient/client.go`:

- Search: `GET /hubs/search?query=<...>&limit=<n>[&sectionId=<id>]`
- List playlists: `GET /playlists`
- Create playlist: `POST /playlists?title=<name>&type=<type>&smart=0&uri=<...>`
- Create smart playlist: `POST /playlists?...&smart=1&uri=<library://...>`
- Add to playlist: `PUT /playlists/<id>/items?uri=<...>`
- Show playlist items: `GET /playlists/<id>/items`
- Delete playlist: `DELETE /playlists/<id>`
- Server UUID: `GET /identity`

Important URI formats:

- Standard playlist item URI:
  - `server://<serverUUID>/com.plexapp.plugins.library/library/metadata/<k1>,<k2>,...`
- Smart playlist URI wrapper:
  - `library://x/directory/<url-escaped-/library/sections/...>`

Always URL-escape query params and URI payloads.

## Playlist from URL Workflow (Current Best Path)

Use this flow when an LLM or script has parsed a watchlist into show + episode IDs:

1. Resolve episodes and rating keys:

```bash
plex episodes "Show Name" --filter "S1E1,S1E3,S2E5" --keys-only
```

2. Create playlist with those keys:

```bash
plex playlist create "Playlist Name" <space-separated-rating-keys>
```

3. Add more later if needed:

```bash
plex playlist add <playlist-id> <ratingKey1> <ratingKey2>
```

Notes:

- `playlist create` currently requires rating keys at CLI argument level.
- `episodes --keys-only` emits keys space-separated for shell composition.

## Local Validation Commands

```bash
go build ./...
go test ./...
```

Last verified in this update: `go test ./...` passes.
