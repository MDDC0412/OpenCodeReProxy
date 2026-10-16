# 一键升级与热更新

[返回 README](../README.zh-CN.md) | [English](upgrade.md)

本页说明如何在不重新下载整包、不重写 `config.json` 的前提下升级到新版本。所有方式都只替换程序本体或镜像标签，配置文件、模型缓存、日志和 systemd / Compose 服务定义保持不动。

升级前建议先备份 `config.json` 与 `logs/`。升级期间服务会短暂中断；进行中的推理请求不会自动重放。

## 方式一：Docker Compose（推荐，一条命令）

配置保存在命名卷里，升级只换镜像标签：

```bash
cd /path/to/compose-directory
OPENCODE2API_VERSION=v1.4.0-secdev-diagnostics docker compose pull && docker compose up -d
```

也可以先把 `.env` 里的 `OPENCODE2API_VERSION` 改成目标版本，再执行：

```bash
docker compose pull && docker compose up -d
```

不需要重新下载运行包，也不需要改 `config.json`。回滚时把版本改回旧标签并重复这两条命令即可。

## 方式二：升级脚本（Linux / macOS 二进制部署）

仓库提供 `scripts/update.sh`，自动完成「查最新版本 → 下载 → 校验 SHA256 → 备份旧程序 → 替换 → 重启 systemd 服务」。

```bash
# 直接使用仓库里的脚本（无需先下载运行包）
curl -fsSL https://raw.githubusercontent.com/MDDC0412/OpenCodeReProxy/main/scripts/update.sh -o /tmp/oc2api-update.sh
sudo sh /tmp/oc2api-update.sh
```

常用参数：

```bash
sudo sh /tmp/oc2api-update.sh -v v1.4.0-secdev-diagnostics   # 指定版本
sudo sh /tmp/oc2api-update.sh -d /opt/opencodereproxy        # 指定安装目录
sudo sh /tmp/oc2api-update.sh -s opencodereproxy             # 指定 systemd 服务名
sudo sh /tmp/oc2api-update.sh --compose -d /srv/opencodereproxy  # 更新 Compose 部署
sudo sh /tmp/oc2api-update.sh --no-restart                   # 只换程序
sh /tmp/oc2api-update.sh -n                                  # 只检查，不改动
```

脚本行为：

- 从 Release 下载 `opencode2api_<版本>_<系统>_<架构>.tar.gz` 及其 `.sha256`，校验不一致时直接中止。
- 把旧程序备份为同目录的 `opencode2api.bak-<时间戳>`，再原地替换。
- 自动从 systemd 单元读取 `ExecStart` 推导安装目录和服务名；探测不到时提示手动重启。
- 只碰程序文件；`config.json`、`*.models.*.json`、`*.availability.json`、`*.rotation.json`、`logs/` 全部保留。

## 方式三：Windows 二进制部署

```powershell
cd C:\path\to\opencodereproxy
Invoke-WebRequest https://raw.githubusercontent.com/MDDC0412/OpenCodeReProxy/main/scripts/update.ps1 -OutFile update.ps1
.\update.ps1 -Version v1.4.0-secdev-diagnostics -ServiceName opencodereproxy
```

Windows 不允许覆盖正在运行的 exe，脚本会先停止指定服务或同路径进程，替换后再启动，因此有几秒中断。不指定 `-ServiceName` 时只替换文件，需要自行启动。

```powershell
.\update.ps1                 # 更新到最新 Release
.\update.ps1 -Check          # 只查询目标版本
.\update.ps1 -NoRestart      # 只换程序
.\update.ps1 -Compose -Directory C:\path\to\compose-directory
```

## 方式四：手工替换（任何平台）

1. 下载目标版本的运行包（`opencode2api_*` 自动产物或 `OpenCodeReProxy_*` 完整包）。
2. 校验 `SHA256SUMS.txt` 或对应 `.sha256` 文件。
3. 停止服务。
4. 只替换程序本体：Linux / macOS 用 `opencode2api`，Windows 用 `opencode2api.exe`。
5. 启动服务，检查 `curl -i http://127.0.0.1:8080/healthz` 和一次真实请求。

```bash
# Linux 示例
tar -xzf opencode2api_v1.4.0-secdev-diagnostics_linux_amd64.tar.gz
sudo install -m 755 opencode2api_v1.4.0-secdev-diagnostics_linux_amd64/opencode2api /opt/opencodereproxy/opencode2api
sudo systemctl restart opencodereproxy
```

## 配置兼容性

从 v1.3.7 升级到 v1.4.0 无需修改 `config.json`：

| 新增字段             | 不填写时的行为                                 |
| -------------------- | ---------------------------------------------- |
| `upstream.anonymous` | 自动使用默认值 `https://opencode.ai/inference` |
| `models.free_only`   | 缺省等价于 `false`，行为与升级前一致           |

原有 `listen`、`webui.listen`、`server_keys`、`zen_keys`、`go_keys`、`anonymous`、`proxies`、`retry`、`models`、`performance`、`logging` 字段语义未变。只有需要显式指定匿名上游地址或启用「仅免费模型」时才需要新增字段。

## 验证升级结果

```bash
curl -i http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8081/api/status | head -c 400
```

管理页面「总览」页也会显示当前版本。若新程序启动失败，用备份文件回滚：

```bash
sudo install -m 755 /opt/opencodereproxy/opencode2api.bak-YYYYMMDDHHMMSS /opt/opencodereproxy/opencode2api
sudo systemctl restart opencodereproxy
```

## 关于「不停机热更新」

本程序没有内置自我替换或零停机切换能力。上面所有方式都会重启进程，属于「秒级中断的原地升级」，不是真正的零停机热更新。需要零停机时，可在反向代理后运行两个实例逐个替换并切换流量。
