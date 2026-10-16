# OpenCodeReProxy · Linux binary package

[English](README.md) | [简体中文](README.zh-CN.md)

## Project overview

OpenCodeReProxy is a Go gateway for OpenCode Zen / Zen Go. It provides a single endpoint for clients that support OpenAI or Anthropic APIs, with an embedded management UI for configuration, local keys, models, and request diagnostics.

This fork is based on [OpenCode2API](https://github.com/jasonxu114514/opencode2api) v1.4.0 and adds management UI improvements, CC Switch imports, and persistent exit diagnostics. Executable names remain `opencode2api.exe` / `opencode2api`.

Use it locally, deploy it as a self-hosted API service, or modify the source. OpenCode controls upstream model availability, quotas, and access requirements. Anonymous access covers only models currently offered free.

## Contents

1. [Project overview](#project-overview)
2. [Features](#features)
3. [Screenshots](#screenshots)
4. [Requirements](#requirements)
5. [Download and installation](#download-and-installation)
6. [Quick start](#quick-start)
7. [Configuration](#configuration)
8. [Usage and client integration](#usage-and-client-integration)
9. [Deployment and operations](#deployment-and-operations)
10. [Directory structure](#directory-structure)
11. [Development builds and tests](#development-builds-and-tests)
12. [Troubleshooting](#troubleshooting)
13. [Contributing and support](#contributing-and-support)
14. [Versions and changes](#versions-and-changes)
15. [License and acknowledgements](#license-and-acknowledgements)

## Features

| Feature                  | Description                                                                                          |
| ------------------------ | ---------------------------------------------------------------------------------------------------- |
| API protocols            | OpenAI Chat Completions, Responses, Anthropic Messages; System One for applicable models             |
| Response modes           | JSON and SSE, with conversion to the model's native protocol                                         |
| Upstream lanes           | Zen, Zen Go, and anonymous free access; multiple upstream keys, retries, and tier preference         |
| Proxies                  | Direct, HTTP, HTTPS, SOCKS5, SOCKS5H, and proxy list files                                           |
| Management UI            | Overview, Playground, access, event logs, API keys, models and quotas, rotation, config, account     |
| Local keys               | Create, name, enable, disable, and delete client keys; CC Switch import                              |
| Discovery and monitoring | Model discovery, availability checks, Go quotas, request and token metrics                           |
| Exit diagnostics         | Persistent logs and Go crash traces; preserve the Windows console after a failed double-click launch |
| Configuration updates    | Apply supported settings to new requests after saving or reloading through the UI                    |
| Embedded frontend        | Binary packages need no Node.js runtime or database                                                  |

Protocol conversion follows upstream capabilities. Embeddings, file uploads, and image generation endpoints are not implemented.

## Screenshots

Screenshots use local demonstration data to show the overview and themes.

![Overview in light mode](docs/screenshots/overview-light.png)

<details>
<summary>View dark mode</summary>

![Overview in dark mode](docs/screenshots/overview-dark.png)

</details>

## Requirements

| Method              | Requirements                                                                        |
| ------------------- | ----------------------------------------------------------------------------------- |
| Windows binaries    | Windows amd64 / arm64; choose the ZIP matching your architecture                    |
| Linux binaries      | Linux amd64 / arm64; the executable needs execute permission                        |
| macOS binaries      | amd64 for Intel, arm64 for Apple Silicon                                            |
| Source run or build | Go 1.24+; the first build downloads Go modules                                      |
| Docker              | Docker Engine / Desktop capable of Linux containers; image supports amd64 and arm64 |
| Docker Compose      | Docker Engine and the Compose plugin, using `docker compose`                        |
| Frontend checks     | Node.js 22+; needed only for formatting and browser regression                      |

The service needs access to OpenCode and its model catalog. Configuration and state directories must be writable for password migration, configuration saves, caches, and logs.

## Download and installation

[Repository](https://github.com/MDDC0412/OpenCodeReProxy) · [Downloads](https://github.com/MDDC0412/OpenCodeReProxy/releases) · [Current release](https://github.com/MDDC0412/OpenCodeReProxy/releases/tag/v1.4.0-secdev-diagnostics)

| File or channel                                              | Purpose                                           |
| ------------------------------------------------------------ | ------------------------------------------------- |
| `OpenCodeReProxy_*_windows_amd64.zip`                        | Windows x64 binary package                        |
| `OpenCodeReProxy_*_linux_amd64.tar.gz`                       | Linux x64 binary package                          |
| `opencode2api_*_<os>_<arch>.zip / .tar.gz`                   | Automated amd64, arm64, and macOS binary packages |
| `OpenCodeReProxy_*_source.zip`                               | Clean source in `OpenCodeReProxySouce`            |
| GitHub's `Source code`                                       | Source at the release tag; compile it yourself    |
| `ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics` | This fork's Docker image                          |
| `SHA256SUMS.txt` / `*.sha256`                                | Download integrity checks                         |

Extract the entire binary archive. If you already have a configuration, retain it and select its path with `-config`.

## Quick start

### First-time configuration

Run the commands from the binary package or source root, where `config.example.json` is located. Copy and edit the example on first use. Skip the copy if `config.json` already exists.

Windows PowerShell:

```powershell
Copy-Item .\config.example.json .\config.json
notepad .\config.json
```

Linux / macOS:

```bash
cp -n config.example.json config.json
vi config.json
```

Set your local key, administrator password, and upstream mode as described in [Configuration](#configuration). For anonymous free access use `anonymous: true`, `zen_keys: []`, and `go_keys: []`. Enable filename extensions in Windows Explorer to avoid `config.json.txt`.

In `vi`, press `i` to edit, then `Esc`, type `:wq`, and press Enter. Any plain-text editor works.

### Linux binary

From the extracted Linux package:

```bash
chmod +x ./opencode2api
./opencode2api -config ./config.json
```

Keep the terminal running. Press `Ctrl+C` to stop. See [Deployment and operations](#deployment-and-operations) for background and systemd service options.

### Run from source

Install Go 1.24+, download the source ZIP, or clone:

```bash
git clone https://github.com/MDDC0412/OpenCodeReProxy.git
cd OpenCodeReProxy
```

Complete first-time configuration in the source directory, then run:

```bash
go run ./cmd/opencode2api -config ./config.json
```

The `go run` command works in PowerShell, Linux, and macOS. Source ZIPs contain no precompiled executable. See [Development builds and tests](#development-builds-and-tests) to produce standalone binaries.

### Run the Docker image

Prepare and edit `config.json` first. Start Docker and select Linux containers.

Linux / macOS terminal:

```bash
docker pull ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics
docker run -d --name opencodereproxy --restart unless-stopped \
  -p 8080:8080 -p 8081:8081 \
  -e CONFIG_SEED_PATH=/run/config/config.json \
  -e LISTEN_ADDRESS=0.0.0.0:8080 \
  -e WEBUI_LISTEN_ADDRESS=0.0.0.0:8081 \
  --mount "type=bind,source=$(pwd)/config.json,target=/run/config/config.json,readonly" \
  --mount type=volume,source=opencodereproxy-state,target=/var/lib/opencode2api \
  ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics
docker logs -f opencodereproxy
```

Windows PowerShell:

```powershell
$configFile = (Resolve-Path .\config.json).Path
docker pull ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics
docker run -d --name opencodereproxy --restart unless-stopped `
  -p 8080:8080 -p 8081:8081 `
  -e CONFIG_SEED_PATH=/run/config/config.json `
  -e LISTEN_ADDRESS=0.0.0.0:8080 `
  -e WEBUI_LISTEN_ADDRESS=0.0.0.0:8081 `
  --mount "type=bind,source=$configFile,target=/run/config/config.json,readonly" `
  --mount type=volume,source=opencodereproxy-state,target=/var/lib/opencode2api `
  ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics
docker logs -f opencodereproxy
```

First startup imports the host configuration into the `opencodereproxy-state` volume. The active configuration, password hash, caches, and logs live under `/var/lib/opencode2api` inside that volume. Later host edits do not automatically overwrite it; see [Deployment and operations](#deployment-and-operations).

### Docker Compose

The directory must contain `compose.yaml`, `.env.example`, and your configured `config.json`. Copy the environment template on first use; retain an existing `.env`.

Windows PowerShell:

```powershell
Copy-Item .\.env.example .\.env
docker compose up -d
docker compose logs -f
```

Linux / macOS:

```bash
cp -n .env.example .env
docker compose up -d
docker compose logs -f
```

Compose uses this fork's published image and keeps state in the `opencode2api-state` named volume. `.env` sets host ports and container listeners. Keep the container listeners at `0.0.0.0:8080` / `0.0.0.0:8081` so port mappings can reach them.

### Build a Docker image from source

Run from the source root:

```bash
docker build --build-arg VERSION=local -t opencodereproxy:local .
```

Use the `docker run` command above with its final image name changed to `opencodereproxy:local`. The image builds from the current source and embeds the current `webui/` assets.

### Log in and verify

On the server machine, open **<http://127.0.0.1:8081>**. The example username is `admin`; use the `webui.password` you set.

In **Playground**, choose a model from the current list and send “Reply OK”. Verify an actual model response before connecting clients. The default API origin is `http://127.0.0.1:8080`.

Readiness check:

```bash
curl -i http://127.0.0.1:8080/healthz
```

Use `curl.exe` in Windows PowerShell. HTTP 200 with `ready: true` means the catalog and routing resources are ready; verify inference with an actual request. Use your selected ports if you changed them.

## Configuration

### Configuration file and minimal example

**The default filename is `config.json`. The app does not automatically load `config.example.json`.** Relative paths resolve from your terminal's current directory; use an absolute configuration path when starting elsewhere.

This is a loadable minimal configuration for anonymous free access. Replace `YOUR_LOCAL_API_KEY` and `YOUR_ADMIN_PASSWORD`; the management password must have at least 10 characters. Alternatively, copy the complete [config.example.json](config.example.json) and edit it.

```json
{
  "listen": "127.0.0.1:8080",
  "server_keys": ["YOUR_LOCAL_API_KEY"],
  "zen_keys": [],
  "go_keys": [],
  "anonymous": true,
  "proxies": ["direct"],
  "webui": {
    "enabled": true,
    "listen": "127.0.0.1:8081",
    "username": "admin",
    "password": "YOUR_ADMIN_PASSWORD"
  }
}
```

### Main settings

The table distinguishes built-in defaults from options explicitly enabled in the example file.

| Setting                                        | Default / requirement                                         | Purpose                                                     |
| ---------------------------------------------- | ------------------------------------------------------------- | ----------------------------------------------------------- |
| `listen`                                       | `127.0.0.1:8080`                                              | API listener                                                |
| `server_keys`                                  | Required; at least one enabled local key                      | Client API authentication                                   |
| `zen_keys` / `go_keys`                         | Empty; at least one upstream lane required unless anonymous   | OpenCode credentials                                        |
| `anonymous`                                    | `false`                                                       | Enable anonymous free Zen access                            |
| `prefer`                                       | `"go"`; also accepts `"zen"`                                  | Preferred authenticated tier                                |
| `proxies`                                      | Direct if none configured                                     | `["direct"]` or proxy URL list                              |
| `proxyfile`                                    | Empty                                                         | One proxy per line; relative to the configuration directory |
| `upstream.zen` / `upstream.go`                 | `https://opencode.ai/zen` / `https://opencode.ai/zen/go`      | Upstream endpoints                                          |
| `webui.enabled`                                | `false`; example enables it                                   | Enable management UI                                        |
| `webui.listen`                                 | `0.0.0.0:8081`                                                | Management listener; use `127.0.0.1:8081` for local access  |
| `webui.username`                               | Required when management is enabled; example uses `admin`     | Administrator username                                      |
| `webui.password`                               | Required for initial management setup; at least 10 characters | Migrated to `password_hash` on startup                      |
| `webui.session_ttl_minutes`                    | `720`                                                         | Session lifetime in minutes                                 |
| `retry.max_attempts` / `retry.timeout_seconds` | `3` / `300`                                                   | Retry count and total request timeout in seconds            |
| `models.refresh_seconds`                       | `300`                                                         | Model discovery refresh in seconds                          |
| `models.protocols`                             | `{}`                                                          | Native protocol overrides                                   |
| `performance`                                  | See the complete example                                      | Connection pools, connection and first-event timeouts       |
| `logging.level` / `logging.ring_size`          | `"info"` / `2000`                                             | Log level and in-memory log capacity                        |
| `logging.dump_request_bodies`                  | `false`                                                       | Request body logging                                        |
| `reasoning.effort` / `effort_by_model`         | Disabled; complete example uses `"high"`                      | Thinking override when absent from a request                |
| `rotation`                                     | Built-in group defaults                                       | `sota` / `sweet` aliases, ordering, first-output timeout    |

Local keys, administrator passwords, and upstream keys serve separate entry points: clients use `server_keys`, management uses the administrator password, and `zen_keys` / `go_keys` authenticate to OpenCode.

### Compose environment variables

Compose reads the following variables from `.env` in the same directory. Copy [.env.example](.env.example) on first use and run `docker compose up -d` after changes.

| Variable                    | Template value              | Purpose                                |
| --------------------------- | --------------------------- | -------------------------------------- |
| `OPENCODE2API_VERSION`      | `v1.4.0-secdev-diagnostics` | This project's image tag               |
| `OPENCODE2API_PORT`         | `8080`                      | Host API port                          |
| `OPENCODE2API_WEBUI_PORT`   | `8081`                      | Host management port                   |
| `OPENCODE2API_LISTEN`       | `0.0.0.0:8080`              | Container API listener override        |
| `OPENCODE2API_WEBUI_LISTEN` | `0.0.0.0:8081`              | Container management listener override |

Keep container listeners at the template values when changing host ports. For example, `OPENCODE2API_PORT=9090` changes the client URL to `http://127.0.0.1:9090/v1`.

### Choose an upstream mode

| Mode                  | `anonymous` | `zen_keys`         | `go_keys`         |
| --------------------- | ----------- | ------------------ | ----------------- |
| Anonymous free access | `true`      | `[]`               | `[]`              |
| Zen                   | `false`     | `["YOUR_ZEN_KEY"]` | `[]`              |
| Zen Go                | `false`     | `[]`               | `["YOUR_GO_KEY"]` |

Obtain upstream keys from [OpenCode](https://opencode.ai). Actual request results determine free model availability. Anonymous and authenticated lanes may also be combined; see the [routing and configuration reference](docs/reference.md).

### Applying changes

- Saving or reloading through management applies keys, proxies, upstreams, retries, models, and logging settings to new requests.
- Changing `listen`, `webui.listen`, or `webui.enabled` requires a process restart.
- After editing the file on disk, reload it on the config page or restart. In-flight requests continue on their original runtime.
- First startup persists a password hash and removes the plaintext password. Keep using the original password to sign in.

## Usage and client integration

### Management and client settings

Management is at `http://127.0.0.1:8081`. Inspect requests, manage local keys, edit configuration, and test models. **Import CCS** on the API Keys page generates CC Switch import links.

| Client setting                | Value                                 |
| ----------------------------- | ------------------------------------- |
| OpenAI-compatible base URL    | `http://127.0.0.1:8080/v1`            |
| Anthropic-compatible base URL | `http://127.0.0.1:8080`               |
| API key                       | An enabled local key in `server_keys` |
| Model                         | An available ID from `GET /v1/models` |

| Method | Path                   | Purpose                                |
| ------ | ---------------------- | -------------------------------------- |
| GET    | `/v1/models`           | Model list and `supported_endpoints`   |
| POST   | `/v1/chat/completions` | OpenAI Chat Completions                |
| POST   | `/v1/responses`        | OpenAI Responses                       |
| POST   | `/v1/messages`         | Anthropic Messages                     |
| POST   | `/v1/systemone`        | Models using that native protocol only |
| GET    | `/healthz`             | Unauthenticated readiness check        |

### API examples

Use these curl commands in Linux / macOS terminals. List models first, replace `MODEL_ID` with one supporting your endpoint, and replace `YOUR_LOCAL_API_KEY` with your local key.

```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY"
```

Chat Completions:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Reply OK"}],"stream":false}'
```

Responses:

```bash
curl http://127.0.0.1:8080/v1/responses \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","input":"Reply OK"}'
```

Anthropic Messages:

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "x-api-key: YOUR_LOCAL_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","max_tokens":128,"messages":[{"role":"user","content":"Reply OK"}]}'
```

For streaming, add `"stream": true` to the body and use `curl -N`.

Windows PowerShell:

```powershell
$headers = @{ Authorization = "Bearer YOUR_LOCAL_API_KEY" }
Invoke-RestMethod -Uri "http://127.0.0.1:8080/v1/models" -Headers $headers
$body = @{
  model = "MODEL_ID"
  messages = @(@{ role = "user"; content = "Reply OK" })
  stream = $false
} | ConvertTo-Json -Depth 8
Invoke-RestMethod -Uri "http://127.0.0.1:8080/v1/chat/completions" `
  -Method Post -Headers $headers -ContentType "application/json" -Body $body
```

Responses include `x-request-id` for log correlation. See the [API reference](docs/reference.md) for System One and conversion boundaries.

## Deployment and operations

### Listeners and CLI options

For remote binary access, set the applicable `listen` / `webui.listen` to `0.0.0.0:PORT` and use the server IP in clients. Docker examples publish host ports 8080 / 8081. For local-only access, use mappings `127.0.0.1:8080:8080` / `127.0.0.1:8081:8081`. For public access, use an HTTPS reverse proxy and restrict management access.

| Option            | Default               | Purpose                                                      |
| ----------------- | --------------------- | ------------------------------------------------------------ |
| `-config`         | `config.json`         | Configuration path                                           |
| `-listen`         | Configuration value   | API listener override for this process                       |
| `-web-listen`     | Configuration value   | Management listener override                                 |
| `-log-dir`        | `logs/` beside config | Diagnostics directory                                        |
| `-pause-on-error` | `auto`                | `auto`, `always`, or `never`; preserve console after failure |
| `-h`              | —                     | CLI help                                                     |

### Linux background process and service

From the runtime directory:

```bash
nohup ./opencode2api -config ./config.json > startup.log 2>&1 &
echo $! > opencodereproxy.pid
tail -f startup.log
```

Stop that background process:

```bash
kill "$(cat opencodereproxy.pid)"
```

For long-term deployments, use systemd for boot startup and restart on failure. See the [service deployment guide](docs/deployment.md) for account, directory, unit, and lifecycle commands.

### Docker and Compose operations

```bash
docker compose ps
docker compose logs -f
docker compose restart
docker compose stop
docker compose up -d
```

For a direct Docker deployment, use `docker logs -f opencodereproxy`, `docker restart opencodereproxy`, and `docker stop opencodereproxy`.

The volume contains the active configuration. Prefer management UI edits. To import an edited host configuration again:

```bash
docker compose cp config.json opencode2api:/var/lib/opencode2api/config.json
docker compose exec -u root opencode2api sh -c 'chown opencode2api:opencode2api /var/lib/opencode2api/config.json && chmod 600 /var/lib/opencode2api/config.json'
docker compose restart
```

The ownership step lets the unprivileged service read configuration and persist password migration. For direct Docker, use `docker cp / exec / restart` with container name `opencodereproxy` instead.

To upgrade Compose (one command; configuration stays in the volume):

```bash
OPENCODE2API_VERSION=v1.4.0-secdev-diagnostics docker compose pull && docker compose up -d
```

Binary deployments can use `scripts/update.sh` or `scripts/update.ps1`, which do not re-download the runtime package or touch `config.json`. See [One-command upgrade and in-place update](docs/upgrade.md) for options and rollback.

`docker compose down` retains named volumes; `docker compose down -v` removes persistent data. The image health check uses container port 8080. If changing internal ports, update mappings and the health check.

### Logs and state

| Data                            | Location / retention                                                      |
| ------------------------------- | ------------------------------------------------------------------------- |
| Configuration and backup        | `config.json`, `config.json.bak`                                          |
| Model caches and state          | `*.models.*.json`, `*.availability.json`, `*.rotation.json` beside config |
| Runtime logs                    | `logs/runtime-*.log` beside config                                        |
| Go crash traces                 | `logs/crash-*.log`                                                        |
| Windows supervisor output       | `logs/console-*.log` when console supervision is enabled                  |
| Docker persistent state         | Volume mounted at `/var/lib/opencode2api`                                 |
| Sessions and cumulative metrics | Process memory; cleared on restart                                        |

Runtime and console logs rotate at 10 MiB with 3 backups. Old runs are not cleaned automatically; remove them after investigation when no longer needed. See [exit diagnostics (Chinese)](docs/crash-diagnostics.zh-CN.md).

## Directory structure

Binary package layout:

```text
OpenCodeReProxy_<version>_linux_<arch>/
├── opencode2api       Executable with embedded WebUI
├── config.example.json   Configuration template
├── compose.yaml          Compose deployment
├── .env.example          Compose environment template
├── README.md             English README
├── README.zh-CN.md       Chinese README
├── CHANGELOG.md          Change history
├── docs/                 Guides, references and screenshots
├── scripts/              One-command update scripts
└── LICENSE               GPL license
```

`config.json`, logs, and caches are created during configuration or runtime; they are not shipped. See the [repository](https://github.com/MDDC0412/OpenCodeReProxy) for source layout.

## Development builds and tests

Source development requires Go 1.24+. Run from the source root:

```powershell
# Windows
go build -trimpath -o opencode2api.exe ./cmd/opencode2api
```

```bash
# Linux / macOS
go build -trimpath -o opencode2api ./cmd/opencode2api

go test ./...
go vet ./...
```

Inject release versions with `-ldflags "-X main.version=YOUR_VERSION"`. Rebuild and restart after editing `webui/`; Go embed includes the management assets in the binary.

Frontend development and checks require Node.js 22+:

```bash
npm ci --ignore-scripts
npm run check:web
npm run format:check
npx playwright-cli install-browser chrome
npm run test:web
```

Browser regression uses isolated configuration and a local fixture. Reports go to `output/playwright/`. Source distributions omit runtime configuration, credentials, logs, and precompiled binaries. See [development](docs/source-distribution.zh-CN.md) and [browser regression](docs/webui-regression.zh-CN.md) for details in Chinese.

## Troubleshooting

| Symptom                                 | Check and action                                                                                                   |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Missing `config.json`                   | Create it in the runtime directory, check for `config.json.txt`, or use an absolute `-config` path                 |
| Editing the example has no effect       | The app loads `config.json`; copy `config.example.json` first                                                      |
| `parse config.json` / `unknown field`   | Check quotes, commas, and names against the example                                                                |
| Missing or short administrator password | Initial management setup requires at least 10 characters                                                           |
| Port already in use                     | Stop your older instance or change the occupied API / management port and restart                                  |
| Management page does not open           | Check the running process, `webui.enabled`, and actual management port                                             |
| Docker cannot connect to daemon         | Start Docker Engine / Desktop and select Linux containers                                                          |
| Container runs but cannot be reached    | Check published ports and volume configuration; listen on `0.0.0.0` inside the container and create Compose `.env` |
| Host config edits have no effect        | The volume holds active configuration; edit in management or reimport and restart                                  |
| API returns 401                         | Use an enabled local `server_keys` value, not an admin password or upstream key                                    |
| Free model fails / 502 / 504            | Read the Playground error and request ID; inspect availability, upstream keys, network, and proxies                |
| Health check returns 503                | Initial discovery may be pending; inspect discovery and network logs if it persists                                |
| HTTP 200 but generation fails           | SSE can report an error or disconnect; check the final request outcome and logs                                    |
| Windows waits for Enter                 | Service stopped abnormally; retain its reason and logs, fix the cause, and restart                                 |
| Plaintext password disappears           | Expected migration to `password_hash`; use the original password to sign in                                        |
| Metrics disappear on restart            | Sessions and metrics are in memory; persistent logs live in `logs/`                                                |

For unexpected exits, retain the same run's `runtime-*`, `crash-*`, `console-*` when present, and rotation backups. Diagnostics collect evidence. The earlier intermittent exit in the original app has not been reproduced, so its root cause has not been confirmed fixed.

More: [API and config](docs/reference.md) · [management (Chinese)](docs/management.zh-CN.md) · [exit diagnostics (Chinese)](docs/crash-diagnostics.zh-CN.md).

## Contributing and support

Report bugs or improvements through [Issues](https://github.com/MDDC0412/OpenCodeReProxy/issues), and contribute changes through pull requests.

Include version, OS and architecture, launch method, reproduction steps, time, model ID, streaming mode, request ID, and relevant logs. Remove keys, passwords, proxy credentials, and private conversation content before posting.

Create a branch in your fork and describe the problem, resulting behavior, and validation. For Go changes run `go test ./...` and `go vet ./...`; run the appropriate frontend checks for UI changes.

## Versions and changes

The current executable release is `v1.4.0-secdev-diagnostics`, based on upstream OpenCode2API v1.4.0 (synced from v1.3.8 through v1.4.0).

| Area            | Changes                                                                                                                                                                       |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Proxy errors    | When every attempt fails on the proxy route (proxy dial, CONNECT/SOCKS handshake, tunnelled TLS) the API answers 503 `proxy_error` instead of a generic upstream 502          |
| Free tier       | The anonymous lane moved to its own host `upstream.anonymous`; `models.free_only` exposes and routes only free models                                                         |
| Usage           | Usage is tracked per server key and per session; the API key list shows per-key usage                                                                                         |
| Models API      | `/v1/models` reports `cost` and available reasoning levels, with an `auto` effort probe                                                                                       |
| Compatibility   | Tool-result images survive cross-protocol conversion, `max_output_tokens` is floored at 16, attempt timeouts cover the whole attempt, and header-phase disconnects return 499 |
| Management      | Nine-page navigation, Playground, local keys, CC Switch import, light/dark themes                                                                                             |
| Overview checks | Consistent summary and issue counts, bordered status items, action links                                                                                                      |
| Diagnostics     | Preserved Windows failure console and persistent runtime, console, and Go crash logs                                                                                          |

Download versions from [Releases](https://github.com/MDDC0412/OpenCodeReProxy/releases). See [CHANGELOG](CHANGELOG.md) for subsequent changes.

## License and acknowledgements

Licensed under [GNU GPL-3.0-or-later](LICENSE). Preserve upstream attribution and provide corresponding source as required by the license when modifying or redistributing.

Thanks to [jasonxu114514/opencode2api](https://github.com/jasonxu114514/opencode2api), [OpenCode](https://opencode.ai), and the open-source contributors.
