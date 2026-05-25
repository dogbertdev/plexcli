# Project Notes

## Plex API References

- When researching Plex endpoint behavior or response shapes, use only the Go API reference generated for the Plex client dependency or official Plex documentation as authoritative sources.
- Do not cite or rely on third-party, scraped, generated, or unofficial Plex API pages for implementation decisions. If official documentation is missing or unclear, verify behavior through local code, sanitized real-response fixtures, or live CLI checks, and describe it as empirical verification rather than documentation-backed behavior.

## Real-Response Test Fixtures

- `internal/auth/testdata/plex_tv_user.json` and `internal/auth/testdata/plex_tv_resources.json` are sanitized fixtures derived from real Plex.tv responses captured on March 6, 2026.
- These fixtures were shaped from local `.env` credentials during development and must not contain live tokens, private IPs, real email addresses, or other secrets.
- When changing Plex auth/resource normalization in `internal/auth/plextv.go`, update the fixtures and the fixture-backed tests in `internal/auth/auth_test.go` to reflect real response shapes.
- Prefer extending this fixture-based approach before adding broader mocks when validating Plex.tv schema changes.
- Apply the same pattern anywhere the CLI depends on external Plex response shapes or other schema-sensitive integrations.
- For relevant changes, prefer adding sanitized real-response fixtures plus normalization/output tests for:
  - Plex.tv account, token, and resource payloads
  - Plex server library, metadata, history, and stream payloads
  - Command output that flattens or reformats external API responses
- When a live response reveals a field shape the current tests do not cover, add or refresh fixtures in the nearest `testdata` directory as part of the change.
