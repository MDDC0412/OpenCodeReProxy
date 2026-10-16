# Linux service deployment

[Back to README](../README.md) | [简体中文](deployment.zh-CN.md)

For Linux distributions using systemd. Extract the Linux package, configure it, and verify foreground operation first. This guide uses `/opt/opencodereproxy` and a dedicated `opencodereproxy` account.

## Install runtime files

Run from the directory containing the Linux `opencode2api` executable and your configured `config.json`. These copy commands are for initial installation; retain your existing configuration during upgrades.

```bash
id -u opencodereproxy >/dev/null 2>&1 || sudo useradd --system --home-dir /opt/opencodereproxy --shell /usr/sbin/nologin opencodereproxy
sudo mkdir -p /opt/opencodereproxy
sudo install -m 755 ./opencode2api /opt/opencodereproxy/opencode2api
sudo install -m 600 ./config.json /opt/opencodereproxy/config.json
sudo chown -R opencodereproxy:opencodereproxy /opt/opencodereproxy
```

The service account owns the directory so it can persist configuration, password hashes, caches, and logs.

## Create a service unit

Create `/etc/systemd/system/opencodereproxy.service` with `sudo vi` and paste:

```ini
[Unit]
Description=OpenCodeReProxy API gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=opencodereproxy
Group=opencodereproxy
WorkingDirectory=/opt/opencodereproxy
ExecStart=/opt/opencodereproxy/opencode2api -config /opt/opencodereproxy/config.json -pause-on-error=never
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`Restart=on-failure` lets systemd restart the process after failure. The application's diagnostics record the exit reason; interrupted inference requests are not replayed.

## Start and manage

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now opencodereproxy
sudo systemctl status opencodereproxy
sudo journalctl -u opencodereproxy -f
```

Daily lifecycle commands:

```bash
sudo systemctl stop opencodereproxy
sudo systemctl start opencodereproxy
sudo systemctl restart opencodereproxy
```

Check readiness with `curl -i http://127.0.0.1:8080/healthz`. Open management at `http://127.0.0.1:8081` and use the configured administrator account.

## Upgrade and preserve state

Upgrading does not require re-downloading the runtime package or rewriting `config.json`. The recommended path uses the one-command script (download → verify SHA256 → back up → replace in place → restart); see [One-command upgrade and in-place update](upgrade.md) for all options and rollback:

```bash
curl -fsSL https://raw.githubusercontent.com/MDDC0412/OpenCodeReProxy/main/scripts/update.sh | sudo sh
```

Manual steps:

1. Back up configuration, model state, and required logs in `/opt/opencodereproxy/`.
2. Run `sudo systemctl stop opencodereproxy`.
3. Replace the executable with `sudo install -m 755 NEW_BINARY_PATH /opt/opencodereproxy/opencode2api`.
4. Run `sudo systemctl start opencodereproxy`, check status, and verify an actual request.

Upgrading from v1.3.7 to v1.4.0 needs no `config.json` changes; `upstream.anonymous` and `models.free_only` default to the previous behaviour. The process restarts, so the service is briefly interrupted and in-flight requests are not replayed.

Diagnostics live in `/opt/opencodereproxy/logs/`; read systemd output with `journalctl`. Restart after listener or port changes. For remote access use the server IP and adjust the relevant listener from `127.0.0.1` to a reachable interface.
