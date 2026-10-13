# Changelog

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
