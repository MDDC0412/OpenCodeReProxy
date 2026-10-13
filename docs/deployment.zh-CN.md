# Linux 服务部署与运维

[返回 README](../README.zh-CN.md) | [English](deployment.md)

适用于使用 systemd 的 Linux 发行版。先按 README 解压 Linux 运行包、完成配置，并确认前台运行能够处理请求。下面以安装目录 `/opt/opencodereproxy` 和专用用户 `opencodereproxy` 为例。

## 安装运行文件

在包含已配置的 `config.json` 和 Linux 程序 `opencode2api` 的目录执行。首次安装使用这些复制命令；升级时保留已有配置。

```bash
id -u opencodereproxy >/dev/null 2>&1 || sudo useradd --system --home-dir /opt/opencodereproxy --shell /usr/sbin/nologin opencodereproxy
sudo mkdir -p /opt/opencodereproxy
sudo install -m 755 ./opencode2api /opt/opencodereproxy/opencode2api
sudo install -m 600 ./config.json /opt/opencodereproxy/config.json
sudo chown -R opencodereproxy:opencodereproxy /opt/opencodereproxy
```

安装目录由服务用户拥有，以便保存配置、密码哈希、缓存和日志。

## 创建服务文件

使用 `sudo vi /etc/systemd/system/opencodereproxy.service` 创建文件，填写：

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

`Restart=on-failure` 由 systemd 在进程失败时重启；程序本身的诊断功能负责记录退出原因。被中断的推理请求不会自动重放。

## 启动与管理

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now opencodereproxy
sudo systemctl status opencodereproxy
sudo journalctl -u opencodereproxy -f
```

日常启停：

```bash
sudo systemctl stop opencodereproxy
sudo systemctl start opencodereproxy
sudo systemctl restart opencodereproxy
```

检查 API：`curl -i http://127.0.0.1:8080/healthz`。管理页面：`http://127.0.0.1:8081`；使用配置中的管理员账号登录。

## 升级与数据

1. 备份 `/opt/opencodereproxy/` 中的配置、模型状态和需要保留的日志。
2. 执行 `sudo systemctl stop opencodereproxy`。
3. 使用 `sudo install -m 755 新程序路径 /opt/opencodereproxy/opencode2api` 替换程序。
4. 执行 `sudo systemctl start opencodereproxy`，检查服务状态和一次实际请求。

服务的诊断日志在 `/opt/opencodereproxy/logs/`，systemd 输出通过 `journalctl` 查看。更改监听地址或端口后需要重启服务。远程访问时使用服务器 IP，并将相应监听从 `127.0.0.1` 调整为可访问的接口。
