# OpenCodeReProxy

[English](README.md) | [简体中文](README.zh-CN.md)

A gateway for OpenCode Zen / Zen Go with OpenAI Chat Completions, Responses, and Anthropic Messages endpoints. Includes a management UI, Playground, key management, and exit diagnostics. Based on [OpenCode2API](https://github.com/jasonxu114514/opencode2api) v1.3.7.

## Quick start

Binary packages include the executable and WebUI. No Go, Node.js, or database installation is needed to run them. Follow these four steps.

### 1. Download and extract

Open [Releases](https://github.com/MDDC0412/OpenCodeReProxy/releases) and choose:

| Purpose              | Download                                                | Next step                                     |
| -------------------- | ------------------------------------------------------- | --------------------------------------------- |
| Run on Windows x64   | Binary archive ending in `windows_amd64.zip`            | Configure and start below                     |
| Run on Linux x64     | Binary archive ending in `linux_amd64.tar.gz`           | Configure and start below                     |
| Run on ARM64 / macOS | Archive matching your OS and architecture               | Follow its packaged README                    |
| Modify the source    | `OpenCodeReProxy_*_source.zip` or clone this repository | Install Go 1.24+, then follow the steps below |

`source.zip` and GitHub's automatic `Source code` downloads contain source files. Choose a binary archive to run the app directly. Run the commands below from the extracted directory.

### 2. Create and edit config.json

**The app reads `config.json` by default. It does not automatically load `config.example.json`. Editing only the example still causes a missing-config error.**

Copy the example on first use. If you already have `config.json`, skip the copy and edit your existing file.

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

On Windows, enable “View → File name extensions” in Explorer. The file must be named `config.json`, not `config.json.txt`.

Update these fields in `config.json`; leave the other fields as supplied initially:

| Field                              | Value                                                                                 |
| ---------------------------------- | ------------------------------------------------------------------------------------- |
| `server_keys`                      | A local API key you choose, such as `["YOUR_LOCAL_API_KEY"]`; use this key in clients |
| `webui.username`                   | Management username; defaults to `admin`                                              |
| `webui.password`                   | Your management password, at least 10 characters; replace the example                 |
| `anonymous`, `zen_keys`, `go_keys` | Choose one upstream mode below                                                        |

| Upstream mode         | `anonymous` | `zen_keys`         | `go_keys`         |
| --------------------- | ----------- | ------------------ | ----------------- |
| Anonymous free models | `true`      | `[]`               | `[]`              |
| Zen Key               | `false`     | `["YOUR_ZEN_KEY"]` | `[]`              |
| Zen Go Key            | `false`     | `[]`               | `["YOUR_GO_KEY"]` |

Replace each `YOUR_...` value. You choose the local API key; obtain upstream Zen / Go keys from [OpenCode](https://opencode.ai). Anonymous access covers only models currently offered free by the upstream.

Use valid JSON: double quotes for strings and no trailing commas. Keep the configuration directory writable for password migration, configuration saves, caches, and logs.

### 3. Start

Windows binary package:

```powershell
.\opencode2api.exe -config .\config.json
```

After configuring it, you can also double-click `opencode2api.exe` in the same directory. Extract the entire archive first.

Linux / macOS binary package:

```bash
chmod +x ./opencode2api
./opencode2api -config ./config.json
```

Source distribution (Go 1.24+ required; works in PowerShell, Linux, and macOS):

```bash
go run ./cmd/opencode2api -config ./config.json
```

The API and WebUI `server listening` messages confirm that both listeners started. Keep the process running. Press `Ctrl+C` to stop it.

### 4. Log in and test a request

Open **<http://127.0.0.1:8081>** and sign in with the `webui.username` / `webui.password` you set.

Open **Playground**, select a model from the current list, and send “Reply OK”. A model response confirms successful inference for that model.

| Address                         | Purpose                                |
| ------------------------------- | -------------------------------------- |
| `http://127.0.0.1:8081`         | Management UI                          |
| `http://127.0.0.1:8080/healthz` | Readiness check; no API key required   |
| `http://127.0.0.1:8080/v1`      | Base URL for OpenAI-compatible clients |

These ports come from the example configuration. Use your configured addresses if you changed `listen` or `webui.listen`. The example binds management to `0.0.0.0:8081`; set `webui.listen` to `127.0.0.1:8081` for local access only.

Readiness checks inspect the model catalog and routing resources. Use an actual request to verify upstream inference.

## Connect a client

| Client setting                | Value                                                 |
| ----------------------------- | ----------------------------------------------------- |
| OpenAI-compatible base URL    | `http://127.0.0.1:8080/v1`                            |
| Anthropic-compatible base URL | `http://127.0.0.1:8080`                               |
| API key                       | An enabled local key from `server_keys`               |
| Model                         | An available model ID from the UI or `GET /v1/models` |

Endpoints: `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, and `/v1/models`. Some models support only `/v1/systemone`; check their `supported_endpoints`. See the [API and configuration reference](docs/reference.md) for request examples.

You can also create local keys on the **API Keys** page and use **Import CCS** to configure CC Switch.

## Source development and builds

The source ZIP uses the directory name `OpenCodeReProxySouce`. It contains source files without compiled binaries or a runtime configuration. Build when needed:

```powershell
# Windows
go build -trimpath -o opencode2api.exe ./cmd/opencode2api
```

```bash
# Linux / macOS
go build -trimpath -o opencode2api ./cmd/opencode2api
```

Rebuild and restart after changing `webui/`. Go tests and checks:

```bash
go test ./...
go vet ./...
```

Node.js 22+ is needed only for frontend formatting and browser regression tests. See the [development guide (Chinese)](docs/source-distribution.zh-CN.md) for the directory layout and frontend commands.

## Troubleshooting

| Symptom                                                            | Action                                                                                                                                       |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `read config.json: ... cannot find the file`                       | Create `config.json` beside the executable; check extensions and your terminal directory, or pass the full configuration path with `-config` |
| `parse config.json` / `unknown field`                              | Check JSON quotes, commas, and field names against the example                                                                               |
| `webui.password must contain at least 10 characters`               | Set a management password with at least 10 characters                                                                                        |
| `address already in use` / `Only one usage of each socket address` | Stop the existing instance or change the occupied API / management port and restart                                                          |
| Management page does not open                                      | Keep the process running, enable `webui.enabled`, and use the port in `webui.listen`                                                         |
| API returns 401                                                    | Use a local `server_keys` value in the client                                                                                                |
| Free model fails / 502 / 504                                       | Read the error in Playground and event logs; check model availability, upstream credentials, network, and proxies                            |
| Readiness temporarily returns 503                                  | Initial model discovery may be pending; check discovery and network logs if it persists                                                      |
| Windows waits for Enter after a failed exit                        | The service stopped; read the displayed reason and corresponding `logs/` files, fix the cause, and start again                               |
| `webui.password` disappears after startup                          | Expected: the first startup migrates it to `password_hash`; keep using the original password to log in                                       |

Logs default to `logs/` beside the configuration. For an unexpected exit, retain matching `runtime-*`, `crash-*`, and `console-*` files. Include the time, model, and error when reporting an issue; keep credentials out of the report.

## Documentation

- [API, configuration, routing, and monitoring reference](docs/reference.md)
- [Management UI and Playground (Chinese)](docs/management.zh-CN.md)
- [Exit diagnostics (Chinese)](docs/crash-diagnostics.zh-CN.md)
- [Source distribution and development (Chinese)](docs/source-distribution.zh-CN.md)
- [WebUI browser regression (Chinese)](docs/webui-regression.zh-CN.md)

## License

[GPL-3.0-or-later](LICENSE). Preserve the license and upstream attribution when modifying or redistributing.
