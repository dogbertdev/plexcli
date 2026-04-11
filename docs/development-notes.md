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
- `recommend`
- `libraries`
- `library discover` (`similar`, `related`, `matches`)
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
- Related items: `GET /library/metadata/<id>/related`
- Similar items: `GET /library/metadata/<id>/similar?count=<n>`
- Matches: `PUT /library/metadata/<id>/matches`
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

Search/title resolution behavior now lives in shared helpers under `internal/cmd/search_resolver.go`:

- title normalization is case-insensitive, punctuation-insensitive, and whitespace-collapsed
- `search`, `playlist --query`, and `recommend --like` share the same ambiguity handling
- numeric `--like` values are treated as rating keys before falling back to search

Smart playlist filters are now composed as a single library query:

- repeated values inside one category are joined with commas for OR semantics
- different categories are combined in the query string for AND semantics
- year bounds are encoded via `year>=` / `year<=`
- unwatched uses `unwatched=1`

Metadata item edits can add or remove tags by exact rating key:

- `library item edit <ids> --add-tag collection="Martial Arts"` updates `/library/metadata/<ids>` directly.
- `library item bulk-update --filter ...` still sends Plex's raw section filter expression; prefer exact ID edits for curated collections unless the filter expression has been validated against Plex's bulk update endpoint.

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

- `playlist create` and `playlist add` still accept rating keys positionally.
- `playlist create --from-file` and `playlist create --from-stdin` accept whitespace- or comma-separated rating keys.
- `playlist create --dry-run` previews resolved rating keys without creating the playlist.
- `playlist create --query ...` and `playlist add --query ...` resolve titles through the shared search resolver.
- `episodes --keys-only` emits keys space-separated for shell composition.
- `movies --keys-only` also emits keys space-separated. Movie filters OR-match repeated values within a filter type, AND-match across different filter types, and default to GUID dedupe for combined libraries.

## Local Validation Commands

```bash
go build ./...
go test ./...
```

Last verified in this update: `go test ./...` passes.
