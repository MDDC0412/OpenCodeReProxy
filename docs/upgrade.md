# One-command upgrades and in-place updates

[Back to README](../README.md) | [简体中文](upgrade.zh-CN.md)

This page covers upgrading to a new version without downloading a full package again and without rewriting `config.json`. Every method replaces only the executable or the image tag; configuration, model caches, logs, and systemd / Compose service definitions stay untouched.

Back up `config.json` and `logs/` first. The service restarts during an upgrade, so in-flight inference requests are not replayed.

## Option 1: Docker Compose (recommended, one command)

Configuration lives in a named volume, so an upgrade only changes the image tag:

```bash
cd /path/to/compose-directory
OPENCODE2API_VERSION=v1.4.0-secdev-diagnostics docker compose pull && docker compose up -d
```

Alternatively set `OPENCODE2API_VERSION` in `.env` to the target tag, then run:

```bash
docker compose pull && docker compose up -d
```

No package download and no `config.json` edit. To roll back, set the version back to the previous tag and repeat.

## Option 2: Update script (Linux / macOS binary deployments)

`scripts/update.sh` performs the whole flow: resolve the latest version, download, verify SHA256, back up the old executable, replace it, and restart the systemd unit.

```bash
# Use the script straight from the repository; no package download needed first
curl -fsSL https://raw.githubusercontent.com/MDDC0412/OpenCodeReProxy/main/scripts/update.sh -o /tmp/oc2api-update.sh
sudo sh /tmp/oc2api-update.sh
```

Common options:

```bash
sudo sh /tmp/oc2api-update.sh -v v1.4.0-secdev-diagnostics   # specific version
sudo sh /tmp/oc2api-update.sh -d /opt/opencodereproxy        # install directory
sudo sh /tmp/oc2api-update.sh -s opencodereproxy             # systemd unit name
sudo sh /tmp/oc2api-update.sh --compose -d /srv/opencodereproxy  # update a Compose deployment
sudo sh /tmp/oc2api-update.sh --no-restart                   # replace only
sh /tmp/oc2api-update.sh -n                                  # check only, no changes
```

What the script does:

- Downloads `opencode2api_<version>_<os>_<arch>.tar.gz` and its `.sha256` from the Release, aborting on a checksum mismatch.
- Backs up the previous executable as `opencode2api.bak-<timestamp>` in the same directory, then replaces it in place.
- Derives the install directory and unit name from the systemd unit's `ExecStart`; prompts for a manual restart if detection fails.
- Touches only the executable. `config.json`, `*.models.*.json`, `*.availability.json`, `*.rotation.json`, and `logs/` are all preserved.

## Option 3: Windows binary deployments

```powershell
cd C:\path\to\opencodereproxy
Invoke-WebRequest https://raw.githubusercontent.com/MDDC0412/OpenCodeReProxy/main/scripts/update.ps1 -OutFile update.ps1
.\update.ps1 -Version v1.4.0-secdev-diagnostics -ServiceName opencodereproxy
```

Windows cannot overwrite a running executable, so the script stops the named service or a matching process, replaces the file, then starts it again, causing a few seconds of downtime. Without `-ServiceName` it only replaces the file and you start the service yourself.

```powershell
.\update.ps1                 # update to the latest Release
.\update.ps1 -Check          # resolve the target version only
.\update.ps1 -NoRestart      # replace only
.\update.ps1 -Compose -Directory C:\path\to\compose-directory
```

## Option 4: Manual replacement (any platform)

1. Download the target release package (`opencode2api_*` CI archives or the `OpenCodeReProxy_*` full packages).
2. Verify it against `SHA256SUMS.txt` or the matching `.sha256` file.
3. Stop the service.
4. Replace only the executable: `opencode2api` on Linux / macOS, `opencode2api.exe` on Windows.
5. Start the service and check `curl -i http://127.0.0.1:8080/healthz` plus one real request.

```bash
# Linux example
tar -xzf opencode2api_v1.4.0-secdev-diagnostics_linux_amd64.tar.gz
sudo install -m 755 opencode2api_v1.4.0-secdev-diagnostics_linux_amd64/opencode2api /opt/opencodereproxy/opencode2api
sudo systemctl restart opencodereproxy
```

## Configuration compatibility

Upgrading from v1.3.7 to v1.4.0 needs no `config.json` changes:

| New field            | Behaviour when omitted                              |
| -------------------- | --------------------------------------------------- |
| `upstream.anonymous` | Defaults to `https://opencode.ai/inference`         |
| `models.free_only`   | Defaults to `false`, matching pre-upgrade behaviour |

The existing `listen`, `webui.listen`, `server_keys`, `zen_keys`, `go_keys`, `anonymous`, `proxies`, `retry`, `models`, `performance`, and `logging` fields keep their meaning. Only add the new fields if you want an explicit anonymous upstream URL or free-models-only routing.

## Verifying the upgrade

```bash
curl -i http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8081/api/status | head -c 400
```

The management overview page also shows the running version. If the new build fails to start, roll back from the backup:

```bash
sudo install -m 755 /opt/opencodereproxy/opencode2api.bak-YYYYMMDDHHMMSS /opt/opencodereproxy/opencode2api
sudo systemctl restart opencodereproxy
```

## About "zero-downtime hot updates"

The program has no built-in self-replacement or zero-downtime switchover. Every method above restarts the process, so this is a sub-second in-place upgrade rather than a true hot update. For zero downtime, run two instances behind a reverse proxy and drain traffic while replacing them one at a time.
