# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of plexcli
- Library management commands: `libraries`, `library`
- Content discovery: `recently-added`, `unwatched`, `search`
- Quality analysis: `quality-check`, `audio-check`, `subtitles-missing`
- Metadata tools: `metadata-missing`, `duplicates`, `file-paths`
- Playlist management: `playlists`, `playlist-create`, `playlist-from-url`
- Episode listing: `episodes`
- Multiple output formats: table, JSON, TSV
- Configuration via file, environment variables, or CLI flags
- Cross-platform support (Linux, macOS, Windows)

### Changed
- Updated to use published plexgo v0.27.0 SDK

### Fixed
- API compatibility with plexgo v0.27.0 type changes
