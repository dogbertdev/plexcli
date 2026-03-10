# plexgo CLI Coverage

This document compares the reference SDK surface in [references/plexgo](/Users/paulmansfield/projects/plex-cli/references/plexgo) with the current CLI command surface in [internal/cmd](/Users/paulmansfield/projects/plex-cli/internal/cmd).

It is intentionally grouped by capability rather than listing every generated SDK method in one flat table. The goal is to show:

- what the CLI already exposes
- what is only partially exposed
- what has no CLI coverage yet

## Current CLI Surface

The current CLI exposes commands in these files:

- [internal/cmd/auth.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/auth.go)
- [internal/cmd/library.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/library.go)
- [internal/cmd/libraries.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/libraries.go)
- [internal/cmd/server_info.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/server_info.go)
- [internal/cmd/search.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/search.go)
- [internal/cmd/playlist.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/playlist.go)
- [internal/cmd/watch.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/watch.go)
- [internal/cmd/match.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/match.go)
- [internal/cmd/streams.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/streams.go)
- [internal/cmd/directors.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/directors.go)
- [internal/cmd/movies.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/movies.go)
- [internal/cmd/recently_added.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/recently_added.go)
- [internal/cmd/recently_watched.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/recently_watched.go)
- [internal/cmd/unwatched.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/unwatched.go)
- [internal/cmd/unmatched.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/unmatched.go)
- [internal/cmd/duplicates.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/duplicates.go)
- [internal/cmd/episodes.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/episodes.go)
- [internal/cmd/episodes_list.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/episodes_list.go)
- [internal/cmd/file_paths.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/file_paths.go)
- [internal/cmd/audio.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/audio.go)
- [internal/cmd/subtitles.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/subtitles.go)
- [internal/cmd/quality.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/quality.go)
- [internal/cmd/metadata.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/metadata.go)
- [internal/cmd/editions.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/editions.go)
- [internal/cmd/cache.go](/Users/paulmansfield/projects/plex-cli/internal/cmd/cache.go)

## Coverage Summary

## Operational Notes

- `library detect intros` is exposed generically, but live Plex testing showed it behaving as a season-scoped operation on this server. In practice, season metadata IDs are the safest target.
- `library media file` supports both bundle-style URLs and direct Plex library asset paths such as `/library/metadata/<id>/theme/<timestamp>`.

### Covered Or Mostly Covered

These SDK areas have meaningful CLI coverage already, even if the CLI does not expose every SDK method.

| SDK area | Reference file | Current CLI coverage |
| --- | --- | --- |
| Plex.tv auth and server discovery | [references/plexgo/authentication.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/authentication.go), [references/plexgo/plex.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/plex.go) | `auth login`, `auth logout`, `auth servers`, `auth token-info` |
| Basic server/library inspection | [references/plexgo/general.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/general.go), [references/plexgo/library.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/library.go) | `server-info`, `library list`, `library update`, `library clean`, `library status` |
| Search | [references/plexgo/search.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/search.go), [references/plexgo/library.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/library.go) | `search` |
| Watch/session visibility | [references/plexgo/status.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/status.go) | `watch now`, `watch history`, `watch stats`, `recently-watched` |
| Playlist basics | [references/plexgo/playlist.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/playlist.go), [references/plexgo/libraryplaylists.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/libraryplaylists.go) | `playlist list`, `playlist create`, `playlist smart`, `playlist add`, `playlist show`, `playlist delete` |
| Match and stream selection | [references/plexgo/library.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/library.go) | `match search`, `match apply`, `streams list`, `streams set` |
| Audit/report workflows built on library content | [references/plexgo/library.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/library.go), [references/plexgo/content.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/content.go) | `unwatched`, `unmatched`, `duplicates`, `episodes-missing`, `episodes-list`, `recently-added`, `file-paths`, `audio-check`, `subtitles-missing`, `quality-check`, `metadata-missing`, `editions`, `directors`, `movies` |

### Partially Covered

These SDK areas have some CLI coverage, but significant methods are still not exposed directly.

| SDK area | Reference file | Covered in CLI | Missing notable methods |
| --- | --- | --- | --- |
| General | [references/plexgo/general.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/general.go) | `server-info` approximates basic server info | `GetIdentity`, `GetSourceConnectionInformation`, `GetTransientToken` |
| Status | [references/plexgo/status.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/status.go) | Session list, background tasks, playback history reads | `TerminateSession`, `DeleteHistory`, `GetHistoryItem` |
| Plex.tv | [references/plexgo/users.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/users.go) | Token inspection and resource discovery | `GetUsers` |
| Library maintenance | [references/plexgo/library.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/library.go) | `RefreshSection`, `CancelRefresh`, `EmptyTrash`, `CleanBundles`, `DeleteCaches`, `OptimizeDatabase`, `AnalyzeMetadata`, `GenerateThumbs`, `SetStreamSelection`, match/list sections helpers | Many library and metadata operations still missing |
| Playlists | [references/plexgo/libraryplaylists.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/libraryplaylists.go) | Create, add, list/show, delete | `UpdatePlaylist`, `UploadPlaylist`, `ClearPlaylistItems`, `DeletePlaylistItem`, `MovePlaylistItem`, `RefreshPlaylist`, generator methods |

## Missing SDK Areas

The following SDK areas currently have no corresponding CLI command family.

### Preferences

Reference: [references/plexgo/preferences.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/preferences.go)

Missing methods:

- `GetAllPreferences`
- `GetPreference`
- `SetPreferences`

Potential CLI family:

- `preferences list`
- `preferences get`
- `preferences set`

### Updater

Reference: [references/plexgo/updater.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/updater.go)

Missing methods:

- `CheckUpdates`
- `GetUpdatesStatus`
- `ApplyUpdates`

Potential CLI family:

- `updates check`
- `updates status`
- `updates apply`

### Collections

Reference:

- [references/plexgo/collections.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/collections.go)
- [references/plexgo/librarycollections.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/librarycollections.go)

Missing methods:

- `CreateCollection`
- `AddCollectionItems`
- `DeleteCollectionItem`
- `MoveCollectionItem`
- `DeleteCollection`

Potential CLI family:

- `collection create`
- `collection add-items`
- `collection remove-item`
- `collection move-item`
- `collection delete`

### Ratings And Timeline Actions

Reference:

- [references/plexgo/rate.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/rate.go)
- [references/plexgo/timeline.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/timeline.go)

Missing methods:

- `SetRating`
- `MarkPlayed`
- `Report`
- `Unscrobble`

Potential CLI family:

- `rate set`
- `timeline mark-played`
- `timeline unscrobble`

### Providers And Hubs

Reference:

- [references/plexgo/provider.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/provider.go)
- [references/plexgo/hubs.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/hubs.go)

Missing methods:

- Provider management: `ListProviders`, `AddProvider`, `RefreshProviders`, `DeleteMediaProvider`
- Hub operations: `GetAllHubs`, `GetContinueWatching`, `GetHubItems`, `GetPromotedHubs`, `GetMetadataHubs`, `GetPostplayHubs`, `GetRelatedHubs`, `GetSectionHubs`, `ResetSectionDefaults`, `ListHubs`, `CreateCustomHub`, `MoveHub`, `DeleteCustomHub`, `UpdateHubVisibility`

Potential CLI family:

- `provider list`
- `provider add`
- `provider refresh`
- `provider delete`
- `hub list`
- `hub show`
- `hub create`
- `hub move`
- `hub delete`

### Play Queue

Reference: [references/plexgo/playqueue.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/playqueue.go)

Missing methods:

- `CreatePlayQueue`
- `GetPlayQueue`
- `AddToPlayQueue`
- `ClearPlayQueue`
- `ResetPlayQueue`
- `Shuffle`
- `Unshuffle`
- `DeletePlayQueueItem`
- `MovePlayQueueItem`

Potential CLI family:

- `queue create`
- `queue show`
- `queue add`
- `queue clear`
- `queue shuffle`
- `queue move-item`

### Download Queue

Reference: [references/plexgo/downloadqueue.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/downloadqueue.go)

Missing methods:

- `CreateDownloadQueue`
- `GetDownloadQueue`
- `AddDownloadQueueItems`
- `ListDownloadQueueItems`
- `GetItemDecision`
- `GetDownloadQueueMedia`
- `RemoveDownloadQueueItems`
- `GetDownloadQueueItems`
- `RestartProcessingDownloadQueueItems`

Potential CLI family:

- `download-queue create`
- `download-queue list`
- `download-queue add`
- `download-queue remove`
- `download-queue restart`

### Events, Butler, Log, UltraBlur

Reference:

- [references/plexgo/events.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/events.go)
- [references/plexgo/butler.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/butler.go)
- [references/plexgo/log.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/log.go)
- [references/plexgo/ultrablur.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/ultrablur.go)

Missing methods:

- Events: `GetNotifications`, `ConnectWebSocket`
- Butler: `GetTasks`, `StartTasks`, `StopTasks`, `StartTask`, `StopTask`
- Log: `WriteLog`, `WriteMessage`, `EnablePapertrail`
- UltraBlur: `GetColors`, `GetImage`

Potential CLI family:

- `events listen`
- `butler list`
- `butler start`
- `butler stop`

### Live TV, DVR, EPG, Devices, Subscriptions

Reference:

- [references/plexgo/dvrs.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/dvrs.go)
- [references/plexgo/epg.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/epg.go)
- [references/plexgo/devices.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/devices.go)
- [references/plexgo/livetv.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/livetv.go)
- [references/plexgo/subscriptions.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/subscriptions.go)

Missing methods include:

- DVR management and lineup control
- Guide/EPG browsing and lineup discovery
- Device discovery, scan, add/remove, and tuning
- Live TV session inspection
- Recording/subscription management

Potential CLI family:

- `dvr ...`
- `epg ...`
- `device ...`
- `livetv ...`
- `recording ...`

## High-Volume Missing Library Methods

The largest gap used to be the advanced library API surface in [references/plexgo/library.go](/Users/paulmansfield/projects/plex-cli/references/plexgo/library.go). The CLI now exposes nested `library` families for section/item/detect/discover/person/media workflows, including section create/edit/delete, refresh cancellation, item mutation, discovery helpers, artwork downloads, marker operations, stream helpers, and several binary media endpoints.

The biggest remaining holes are the less-common or less-polished helpers below.

### Section Management

- richer custom formatting for `GetSectionsPrefs`

### Item And Metadata Mutation

- richer local-file subtitle upload beyond URL-based subtitle attachment
- broader artwork/body-upload variants beyond URL-driven set/update flows

### Analysis And Detection

- `RefreshSectionsMetadata`

### Collections, People, Similarity, Discovery

- section- or query-specific flag coverage is still shallow for some discovery endpoints

### Extras, Files, Parts, Streams, Markers, Artwork

- `AddExtras`

### Miscellaneous Library Methods

- `IngestTransientItem`
- `GetLibraryItems`

## High-Value Next Candidates

If the goal is to expand the CLI in the most useful order, these are the best next command families:

1. Preferences: highly actionable and relatively contained.
2. Status actions: terminate sessions and delete history.
3. Collections: create and manage collection membership.
4. Ratings and timeline actions: rating, mark-played, unscrobble.
5. Library item mutation: unmatch, split, merge, refresh-item, edit metadata.
6. Updater: check/apply server updates.

## Notes

- This comparison is based on user-facing CLI commands, not just helper methods in [internal/plexclient/client.go](/Users/paulmansfield/projects/plex-cli/internal/plexclient/client.go).
- Some current commands are higher-level workflows that combine multiple SDK calls, so “coverage” here means practical command availability, not strict one-command-per-method parity.
