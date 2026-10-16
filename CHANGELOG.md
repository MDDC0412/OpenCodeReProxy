# Changelog

## v1.4.0-secdev-diagnostics

First fork release rebased on upstream OpenCode2API v1.4.0. It merges upstream
v1.3.8 through v1.4.0 plus the post-release `proxy_error` fix while keeping the
fork's WebUI, documentation, and npm / Prettier toolchain.

### Upstream sync — 2026-10-16

Merge upstream `jasonxu114514/opencode2api` v1.3.8 through v1.4.0 plus the
post-release `proxy_error` fix (upstream `main` @ `a6601da`) into the fork while
keeping the fork's rewritten WebUI, documentation, and npm/Prettier toolchain.

### Fixed

- **Proxy errors are no longer reported as upstream failures.** Attempts that
  fail before a connection to the upstream exists (proxy dial, CONNECT/SOCKS
  handshake, tunnelled TLS) are tagged as proxy failures; the anonymous lane
  reports a proxy failure when no proxy is healthy. When the final error is one
  of these, the inference and System One handlers now answer `503` with type
  `proxy_error` instead of a generic `502`. Real upstream HTTP responses are
  still forwarded unchanged, so genuine upstream errors are not mislabelled.
- Free-tier quota and network switching no longer leave the anonymous lane
  stuck on `401`; the keyless lane moved to its own host
  (`upstream.anonymous`, default `https://opencode.ai/inference`).
- Cross-protocol tool-result images are preserved, Responses requests clamp
  `max_output_tokens` to a floor of 16, attempt timeouts now cover the whole
  attempt, and a client disconnect during the header phase returns `499`.

### Added

- `models.free_only`: expose and route only free models even when Zen / Go keys
  are configured; paid models disappear from `/v1/models`, direct requests are
  rejected, and rotation aliases keep only their free members.
- Embedded official tool definitions (`internal/gateway/opencode_tools.json`),
  verified byte-for-byte upstream.
- Usage split per server key and per-session token accounting; the API key list
  in the fork's WebUI now shows per-key usage.
- `/v1/models` now reports `cost` and available reasoning levels, with an `auto`
  effort probe.
- WebUI: the availability panel gained manual control (restore / switch to
  manual / back to automatic) and the config form gained the free-only toggle
  and the anonymous upstream URL.

### Kept from the fork

- The nine-page WebUI, its browser regression suite, and the npm / Prettier
  toolchain that upstream removed.

## v1.3.7-secdev-diagnostics

### Documentation and deployment update — 2026-10-13

- Reorganize source and platform READMEs into a complete project guide with a table of contents, configuration tables, API examples, and deployment instructions.
- Document Windows, Linux, macOS, source, Docker, and Compose startup methods.
- Point Compose to this fork's published GHCR image and add an environment template for container listeners.
- Include Compose files, screenshots, and change history in binary distributions.

### Initial fork release — 2026-10-10

- Improve the management UI, Playground, local API key management, CC Switch imports, and theme controls.
- Make overview checks and issue counts consistent and show a separate border for each status item.
- Preserve the Windows console after a failed double-click launch and write runtime, console, and Go crash diagnostics to disk.
- Publish clean source and Windows / Linux runtime packages.
