# OpenCodeReProxy · macOS binary package

[English](README.md) | [简体中文](README.zh-CN.md)

This package contains `opencode2api` and the embedded management UI. No Go, Node.js, or database installation is needed.

## 1. Create the configuration

Extract the archive for your OS and CPU architecture, then use `cd` in a terminal to enter the directory containing `opencode2api`.

**The app reads `config.json` by default, not `config.example.json`.** On first use, copy and edit the example below. If `config.json` already exists, skip the copy and edit your existing file.

```bash
cp -n config.example.json config.json
open -e config.json
```

`open -e` opens TextEdit. Save the configuration as plain-text JSON.

## 2. Set keys, password, and upstream mode

Update these fields in your new `config.json`:

| Field            | Value                                                                           |
| ---------------- | ------------------------------------------------------------------------------- |
| `server_keys`    | A local API key you choose, such as `["YOUR_LOCAL_API_KEY"]`, for client access |
| `webui.username` | Management username; defaults to `admin`                                        |
| `webui.password` | Your management password, at least 10 characters                                |

Choose one upstream mode:

| Mode                  | `anonymous` | `zen_keys`         | `go_keys`         |
| --------------------- | ----------- | ------------------ | ----------------- |
| Anonymous free models | `true`      | `[]`               | `[]`              |
| Zen Key               | `false`     | `["YOUR_ZEN_KEY"]` | `[]`              |
| Zen Go Key            | `false`     | `[]`               | `["YOUR_GO_KEY"]` |

Replace `YOUR_...` with actual values. Obtain Zen / Go keys from [OpenCode](https://opencode.ai). Initially leave other settings as supplied. Free model availability depends on the upstream.

Save the file before starting. Use double quotes in JSON and no trailing commas.

## 3. Start the service

Run from the directory containing the executable and configuration:

```bash
chmod +x ./opencode2api
./opencode2api -config ./config.json
```

The API and WebUI `server listening` messages confirm that both listeners started. Keep the process running. Press `Ctrl+C` to stop it. The configuration directory must be writable.

## 4. Log in and test

Open **<http://127.0.0.1:8081>**. Sign in with `webui.username` (default: `admin`) and the `webui.password` you set.

In **Playground**, select an available model and send “Reply OK”. Once you receive a model response, connect a client:

| Setting                       | Value                                                 |
| ----------------------------- | ----------------------------------------------------- |
| OpenAI-compatible base URL    | `http://127.0.0.1:8080/v1`                            |
| Anthropic-compatible base URL | `http://127.0.0.1:8080`                               |
| API key                       | An enabled local key from `server_keys`               |
| Model                         | An available model ID from the UI or `GET /v1/models` |

Readiness URL: `http://127.0.0.1:8080/healthz`. Update these URLs if you change the listen ports. Readiness checks routing resources; use an actual request to verify inference.

Management defaults to `0.0.0.0:8081`; set `webui.listen` to `127.0.0.1:8081` for local access only.

## Troubleshooting

| Error or symptom                                     | Action                                                                                                                         |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `read config.json: ... cannot find the file`         | Copy the example to `config.json`; editing only `config.example.json` has no effect. Check the filename and terminal directory |
| `parse config.json` / `unknown field`                | Check JSON quotes, commas, and field names                                                                                     |
| `webui.password must contain at least 10 characters` | Set a password with at least 10 characters                                                                                     |
| Port occupied / `address already in use`             | Stop the existing instance or change the occupied API / management listen port and restart                                     |
| Management page does not open                        | Keep the service running, enable `webui.enabled`, and use the correct management port                                          |
| Client returns 401                                   | Use a local `server_keys` value, not your management password or an upstream key                                               |
| Free model fails / 502 / 504                         | Check the error in Playground and event logs; inspect model availability, credentials, network, and proxies                    |
| `Permission denied` / `Exec format error`            | Run `chmod +x opencode2api` and check the package OS and architecture                                                          |
| Plaintext password disappears                        | First startup migrates it to `password_hash`; keep using the original password                                                 |

## Logs and documentation

Logs default to `logs/` beside the configuration. For an unexpected exit, retain matching `runtime-*`, `crash-*`, and, when present, `console-*` files.

- [API and full configuration](docs/reference.md)
- [Management UI (Chinese)](docs/management.zh-CN.md)
- [Exit diagnostics (Chinese)](docs/crash-diagnostics.zh-CN.md)
- [Source repository](https://github.com/MDDC0412/OpenCodeReProxy)

License: [GPL-3.0-or-later](LICENSE). Based on [OpenCode2API](https://github.com/jasonxu114514/opencode2api) v1.3.7.
