# OpenCodeReProxy · macOS 运行版

[简体中文](README.zh-CN.md) | [English](README.md)

## 项目概述

OpenCodeReProxy 是一个使用 Go 编写的 OpenCode Zen / Zen Go API 网关。它将上游模型接入统一服务，为支持 OpenAI 或 Anthropic 接口的客户端提供本地访问入口，并通过内嵌管理页面完成配置、密钥管理、模型查看和请求排查。

本项目基于 [OpenCode2API](https://github.com/jasonxu114514/opencode2api) v1.4.0 二次开发，增加管理页面改进、CC Switch 导入和持久化异常退出诊断。程序名称沿用 `opencode2api.exe` / `opencode2api`。

适用于个人本地使用、自托管 API 服务和继续二次开发。上游模型、额度和访问条件由 OpenCode 决定；免费匿名通道只支持当前开放的免费模型。

## 目录

1. [项目概述](#项目概述)
2. [功能特性](#功能特性)
3. [界面预览](#界面预览)
4. [运行环境](#运行环境)
5. [下载与安装](#下载与安装)
6. [快速启动](#快速启动)
7. [配置说明](#配置说明)
8. [使用方法与客户端接入](#使用方法与客户端接入)
9. [部署与运维](#部署与运维)
10. [项目目录结构](#项目目录结构)
11. [开发编译与测试](#开发编译与测试)
12. [常见问题与故障排查](#常见问题与故障排查)
13. [贡献与问题反馈](#贡献与问题反馈)
14. [版本与更新记录](#版本与更新记录)
15. [许可证与致谢](#许可证与致谢)

## 功能特性

| 功能         | 说明                                                                            |
| ------------ | ------------------------------------------------------------------------------- |
| 多协议接口   | OpenAI Chat Completions、Responses、Anthropic Messages；部分模型使用 System One |
| 流式与非流式 | 支持 JSON 和 SSE，按模型原生协议转换请求与响应                                  |
| 上游通道     | Zen、Zen Go、免费匿名通道；支持多个上游 Key、重试和路由偏好                     |
| 代理         | 直连、HTTP、HTTPS、SOCKS5、SOCKS5H，以及代理列表文件                            |
| 管理页面     | 总览、请求台、接入、事件日志、API Key、模型与额度、模型轮转、配置、账号         |
| 本地密钥管理 | 创建、命名、启停和删除客户端密钥；支持 CC Switch 导入                           |
| 模型与监控   | 模型发现、可用性检查、Go 额度、请求与 Token 统计                                |
| 运行诊断     | 持久化运行日志、Go 崩溃堆栈；Windows 双击异常退出后保留窗口                     |
| 配置更新     | 控制台保存或重载配置后，支持将可热更新的字段应用到新请求                        |
| 内嵌前端     | 管理页面直接包含在程序中，运行包无需 Node.js 或数据库                           |

协议转换遵循上游实际能力；不提供 embeddings、文件上传或图像生成接口。

## 界面预览

截图使用本地演示数据，展示总览和主题效果。

![总览：浅色主题](docs/screenshots/overview-light.png)

<details>
<summary>查看暗色主题</summary>

![总览：暗色主题](docs/screenshots/overview-dark.png)

</details>

## 运行环境

| 使用方式       | 环境要求                                                                  |
| -------------- | ------------------------------------------------------------------------- |
| Windows 运行包 | Windows amd64 / arm64，选择与系统架构一致的 ZIP                           |
| Linux 运行包   | Linux amd64 / arm64，解压后程序需要执行权限                               |
| macOS 运行包   | Intel 选择 amd64，Apple Silicon 选择 arm64                                |
| 源码运行或编译 | Go 1.24 或更新版本；首次构建需要下载 Go 模块                              |
| Docker         | 可运行 Linux 容器的 Docker Engine / Docker Desktop；镜像支持 amd64、arm64 |
| Docker Compose | Docker Engine 和 Compose 插件，使用 `docker compose` 命令                 |
| 前端开发检查   | Node.js 22+；仅格式检查和浏览器回归需要                                   |

服务需要能连接 OpenCode 上游及模型目录。配置和数据目录必须可写，以保存密码哈希、配置、缓存和日志。

## 下载与安装

[源码仓库](https://github.com/MDDC0412/OpenCodeReProxy) · [Release 下载](https://github.com/MDDC0412/OpenCodeReProxy/releases) · [当前版本](https://github.com/MDDC0412/OpenCodeReProxy/releases/tag/v1.4.0-secdev-diagnostics)

| 文件或渠道                                                   | 用途                                      |
| ------------------------------------------------------------ | ----------------------------------------- |
| `OpenCodeReProxy_*_windows_amd64.zip`                        | Windows x64 直接运行                      |
| `OpenCodeReProxy_*_linux_amd64.tar.gz`                       | Linux x64 直接运行                        |
| `opencode2api_*_<系统>_<架构>.zip / .tar.gz`                 | 自动发布的 amd64 / arm64 / macOS 运行包   |
| `OpenCodeReProxy_*_source.zip`                               | 干净源码；目录名为 `OpenCodeReProxySouce` |
| GitHub 的 `Source code`                                      | 对应 Release 标签的源码快照，需要自行编译 |
| `ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics` | 本二开版 Docker 镜像                      |
| `SHA256SUMS.txt` / `*.sha256`                                | 校验下载包完整性                          |

下载后先解压整个运行包。已有配置的用户保留自己的 `config.json`，通过 `-config` 指定路径即可。

## 快速启动

### 首次配置

以下命令均在运行包或源码的根目录执行；目录中应能看到 `config.example.json`。首次使用先复制并编辑配置，已有 `config.json` 则跳过复制。

Windows PowerShell：

```powershell
Copy-Item .\config.example.json .\config.json
notepad .\config.json
```

Linux / macOS：

```bash
cp -n config.example.json config.json
vi config.json
```

按[配置说明](#配置说明)填写本地密钥、管理密码和上游方式。免费匿名模式：`anonymous: true`，`zen_keys: []`，`go_keys: []`。在 Windows 资源管理器开启文件扩展名显示，避免保存成 `config.json.txt`。

使用 `vi` 时按 `i` 编辑，完成后按 `Esc`，输入 `:wq` 并回车保存；也可以使用其他纯文本编辑器。

### macOS 二进制

Intel Mac 下载 darwin amd64 包，Apple Silicon 下载 darwin arm64 包。解压、配置后执行：

```bash
chmod +x ./opencode2api
./opencode2api -config ./config.json
```

保持终端运行，按 `Ctrl+C` 停止服务。

### 源码直接运行

安装 Go 1.24+，下载源码包或克隆仓库：

```bash
git clone https://github.com/MDDC0412/OpenCodeReProxy.git
cd OpenCodeReProxy
```

在源码目录完成首次配置，再直接运行：

```bash
go run ./cmd/opencode2api -config ./config.json
```

以上 `go run` 命令适用于 PowerShell、Linux 和 macOS。源码包本身没有预编译程序；需要生成独立运行文件时按[开发编译与测试](#开发编译与测试)操作。

### Docker 镜像运行

先准备并填写 `config.json`，确保 Docker 正在运行且使用 Linux 容器。

Linux / macOS 终端：

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

Windows PowerShell：

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

初次启动会将宿主机配置导入 `opencodereproxy-state` 数据卷。运行中的实际配置、密码哈希、缓存与日志保存在卷内的 `/var/lib/opencode2api`。之后修改宿主机配置不会自动覆盖卷内配置，处理方法见[部署与运维](#部署与运维)。

### Docker Compose

目录中需有 `compose.yaml`、`.env.example` 和已填写的 `config.json`。首次使用复制环境变量模板，已有 `.env` 则保留现有文件。

Windows PowerShell：

```powershell
Copy-Item .\.env.example .\.env
docker compose up -d
docker compose logs -f
```

Linux / macOS：

```bash
cp -n .env.example .env
docker compose up -d
docker compose logs -f
```

Compose 使用本项目的已发布镜像，配置和日志保存在 `opencode2api-state` 命名卷中。`.env` 指定宿主机端口和容器监听地址；容器内监听保持 `0.0.0.0:8080` / `0.0.0.0:8081`，以便通过映射端口访问。

### 从源码构建 Docker 镜像

在源码根目录执行：

```bash
docker build --build-arg VERSION=local -t opencodereproxy:local .
```

之后使用上面的 `docker run` 命令，将最后的镜像名称替换为 `opencodereproxy:local`。镜像会从当前源码构建，并内嵌当前 `webui/` 页面。

### 登录与验证

服务启动后，在服务所在机器打开 **<http://127.0.0.1:8081>**。默认示例用户名为 `admin`，密码为自己填写的 `webui.password`。

登录后进入“请求台”，选择当前模型列表里的模型，发送“请回复 OK”。收到实际模型回复后再接入客户端。API 默认位于 `http://127.0.0.1:8080`。

健康检查：

```bash
curl -i http://127.0.0.1:8080/healthz
```

Windows PowerShell 可用 `curl.exe`。健康接口返回 200 且 `ready: true` 表示目录和路由资源就绪；推理是否成功仍以实际请求为准。若修改过端口，请使用修改后的地址。

## 配置说明

### 配置文件与最小示例

**默认文件名为 `config.json`。程序不会自动读取 `config.example.json`。** 相对路径以终端当前目录为基准；从其他目录启动时使用配置的完整路径。

下面是可以加载的免费匿名模式最小配置。使用前替换 `YOUR_LOCAL_API_KEY` 和 `YOUR_ADMIN_PASSWORD`，管理密码至少 10 个字符。也可以从包内完整的 [config.example.json](config.example.json) 创建配置，再按需要修改。

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

### 主要字段

下表区分程序默认值和示例文件中主动启用的选项。

| 字段                                           | 程序默认 / 要求                                          | 用途                                        |
| ---------------------------------------------- | -------------------------------------------------------- | ------------------------------------------- |
| `listen`                                       | `127.0.0.1:8080`                                         | API 监听地址                                |
| `server_keys`                                  | 必填，至少一个启用的本地密钥                             | 客户端访问网关的 API Key                    |
| `zen_keys` / `go_keys`                         | 默认空；非匿名模式至少配置一种                           | OpenCode 上游凭据                           |
| `anonymous`                                    | `false`                                                  | 开启免费匿名 Zen 通道                       |
| `prefer`                                       | `"go"`，可选 `"zen"`                                     | 两种付费通道的优先顺序                      |
| `proxies`                                      | 没有代理配置时使用直连                                   | `["direct"]` 或代理 URL 列表                |
| `proxyfile`                                    | 空                                                       | 每行一个代理地址；相对路径基于配置目录      |
| `upstream.zen` / `upstream.go`                 | `https://opencode.ai/zen` / `https://opencode.ai/zen/go` | 上游地址                                    |
| `webui.enabled`                                | 默认 `false`；示例为 `true`                              | 启用管理页面                                |
| `webui.listen`                                 | `0.0.0.0:8081`                                           | 管理监听；仅本机访问可改为 `127.0.0.1:8081` |
| `webui.username`                               | 启用管理时必填；示例为 `admin`                           | 管理员用户名                                |
| `webui.password`                               | 首次启用管理时必填，至少 10 个字符                       | 初始密码；启动后迁移为 `password_hash`      |
| `webui.session_ttl_minutes`                    | `720`                                                    | 登录有效期，单位分钟                        |
| `retry.max_attempts` / `retry.timeout_seconds` | `3` / `300`                                              | 重试次数与请求总超时，单位秒                |
| `models.refresh_seconds`                       | `300`                                                    | 模型目录刷新周期，单位秒                    |
| `models.protocols`                             | `{}`                                                     | 自定义模型原生协议覆盖                      |
| `performance`                                  | 默认值见完整示例                                         | 连接池、连接超时和首事件超时                |
| `logging.level` / `logging.ring_size`          | `"info"` / `2000`                                        | 日志级别与内存日志条数                      |
| `logging.dump_request_bodies`                  | `false`                                                  | 是否记录请求体                              |
| `reasoning.effort` / `effort_by_model`         | 默认关闭；完整示例设为 `"high"`                          | 请求未指定时的思考强度                      |
| `rotation`                                     | 内置分组默认值                                           | `sota` / `sweet` 别名、排序与首输出超时     |

本地 API 密钥、管理密码、上游 Key 用于三个不同入口：客户端填写 `server_keys`；管理页面填写管理员密码；`zen_keys` / `go_keys` 用于访问 OpenCode。

### Compose 环境变量

Compose 通过同目录的 `.env` 读取以下变量。首次从 [.env.example](.env.example) 复制，更改后执行 `docker compose up -d` 应用。

| 变量                        | 模板值                      | 用途                |
| --------------------------- | --------------------------- | ------------------- |
| `OPENCODE2API_VERSION`      | `v1.4.0-secdev-diagnostics` | 本项目镜像标签      |
| `OPENCODE2API_PORT`         | `8080`                      | 宿主机 API 端口     |
| `OPENCODE2API_WEBUI_PORT`   | `8081`                      | 宿主机管理端口      |
| `OPENCODE2API_LISTEN`       | `0.0.0.0:8080`              | 容器内 API 监听覆盖 |
| `OPENCODE2API_WEBUI_LISTEN` | `0.0.0.0:8081`              | 容器内管理监听覆盖  |

更改宿主机端口时，容器监听保持模板值。例如设置 `OPENCODE2API_PORT=9090` 后，客户端地址改为 `http://127.0.0.1:9090/v1`。

### 选择上游方式

| 方式     | `anonymous` | `zen_keys`         | `go_keys`         |
| -------- | ----------- | ------------------ | ----------------- |
| 免费匿名 | `true`      | `[]`               | `[]`              |
| Zen      | `false`     | `["YOUR_ZEN_KEY"]` | `[]`              |
| Zen Go   | `false`     | `[]`               | `["YOUR_GO_KEY"]` |

Zen / Go Key 从 [OpenCode](https://opencode.ai) 获取；免费模型的实际可用性以请求结果为准。付费通道和匿名通道也可组合，详见[路由与配置参考](docs/reference.zh-CN.md)。

### 修改配置如何生效

- 在管理页面保存或执行配置重载后，Key、代理、上游、重试、模型与日志等设置应用到新请求。
- `listen`、`webui.listen`、`webui.enabled` 修改后需要重启进程。
- 手动修改磁盘文件后，需要在配置页重载或重启；已有请求继续使用原运行实例。
- 首次启动会保存密码哈希并移除明文密码，之后仍用原密码登录。

## 使用方法与客户端接入

### 管理页面与客户端参数

管理页面为 `http://127.0.0.1:8081`，支持查看请求结果、管理本地 API Key、编辑配置和验证模型。“API Key”页的“导入 CCS”可生成 CC Switch 导入链接。

| 客户端设置              | 内容                                    |
| ----------------------- | --------------------------------------- |
| OpenAI 兼容 Base URL    | `http://127.0.0.1:8080/v1`              |
| Anthropic 兼容 Base URL | `http://127.0.0.1:8080`                 |
| API Key                 | `server_keys` 中启用的本地密钥          |
| 模型                    | 当前 `GET /v1/models` 返回的可用模型 ID |

| 方法 | 路径                   | 用途                             |
| ---- | ---------------------- | -------------------------------- |
| GET  | `/v1/models`           | 查询模型及 `supported_endpoints` |
| POST | `/v1/chat/completions` | OpenAI Chat Completions          |
| POST | `/v1/responses`        | OpenAI Responses                 |
| POST | `/v1/messages`         | Anthropic Messages               |
| POST | `/v1/systemone`        | 仅支持该协议的模型               |
| GET  | `/healthz`             | 无需鉴权的就绪检查               |

### API 请求示例

以下 curl 命令用于 Linux / macOS 终端。先查询模型，再将 `MODEL_ID` 换成支持目标接口的模型 ID，并将 `YOUR_LOCAL_API_KEY` 换成自己的本地密钥。

```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY"
```

Chat Completions：

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Reply OK"}],"stream":false}'
```

Responses：

```bash
curl http://127.0.0.1:8080/v1/responses \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","input":"Reply OK"}'
```

Anthropic Messages：

```bash
curl http://127.0.0.1:8080/v1/messages \
  -H "x-api-key: YOUR_LOCAL_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","max_tokens":128,"messages":[{"role":"user","content":"Reply OK"}]}'
```

需要流式响应时，给请求体加入 `"stream": true`，并使用 `curl -N`。

Windows PowerShell 示例：

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

请求结果包含 `x-request-id`，可用于关联日志。System One 请求和协议转换边界见[接口参考](docs/reference.zh-CN.md)。

## 部署与运维

### 端口与命令行参数

远程访问二进制服务时，将对应的 `listen` / `webui.listen` 改为 `0.0.0.0:端口`，客户端使用服务器 IP。Docker 和 Compose 的示例映射宿主机 8080 / 8081；如需只允许本机访问，可将端口映射改为 `127.0.0.1:8080:8080` / `127.0.0.1:8081:8081`。公网部署可通过 HTTPS 反向代理接入，并限制管理端访问。

| 参数              | 默认值           | 作用                                              |
| ----------------- | ---------------- | ------------------------------------------------- |
| `-config`         | `config.json`    | 配置路径                                          |
| `-listen`         | 使用配置值       | 覆盖本次进程的 API 监听                           |
| `-web-listen`     | 使用配置值       | 覆盖本次进程的管理监听                            |
| `-log-dir`        | 配置旁的 `logs/` | 诊断目录                                          |
| `-pause-on-error` | `auto`           | `auto`、`always`、`never`，控制异常退出后等待回车 |
| `-h`              | —                | 查看命令行帮助                                    |

### Linux 后台与服务管理

在运行目录执行：

```bash
nohup ./opencode2api -config ./config.json > startup.log 2>&1 &
echo $! > opencodereproxy.pid
tail -f startup.log
```

停止本次后台进程：

```bash
kill "$(cat opencodereproxy.pid)"
```

长期部署可使用 systemd，实现开机启动和异常重启；完整用户、目录、服务文件及启停命令见[服务部署指南](docs/deployment.zh-CN.md)。

### Docker 与 Compose 运维

```bash
docker compose ps
docker compose logs -f
docker compose restart
docker compose stop
docker compose up -d
```

直接 `docker run` 部署时，使用 `docker logs -f opencodereproxy`、`docker restart opencodereproxy` 和 `docker stop opencodereproxy`。

容器实际使用数据卷中的配置。优先在管理页面修改；需要重新导入宿主机配置时执行：

```bash
docker compose cp config.json opencode2api:/var/lib/opencode2api/config.json
docker compose exec -u root opencode2api sh -c 'chown opencode2api:opencode2api /var/lib/opencode2api/config.json && chmod 600 /var/lib/opencode2api/config.json'
docker compose restart
```

上面的所有者修正确保非 root 服务仍可读取配置和保存密码哈希。直接 Docker 部署时，将 `docker compose cp / exec / restart` 换成 `docker cp / exec / restart`，容器名使用 `opencodereproxy`。

升级 Compose（一条命令，配置保留在数据卷中）：

```bash
OPENCODE2API_VERSION=v1.4.0-secdev-diagnostics docker compose pull && docker compose up -d
```

二进制升级可用 `scripts/update.sh` 或 `scripts/update.ps1` 一键完成，不需要重新下载整包，也不会改动 `config.json`。完整说明与回滚方式见[一键升级与热更新](docs/upgrade.zh-CN.md)。

`docker compose down` 保留命名卷；`docker compose down -v` 会删除持久数据。当前镜像健康检查使用容器内 8080 端口，修改容器内部端口时需要同步调整端口映射与健康检查。

### 日志与持久化

| 内容               | 位置 / 保留方式                                                      |
| ------------------ | -------------------------------------------------------------------- |
| 配置与备份         | `config.json`、`config.json.bak`                                     |
| 模型缓存与状态     | 配置旁的 `*.models.*.json`、`*.availability.json`、`*.rotation.json` |
| 运行日志           | 配置旁的 `logs/runtime-*.log`                                        |
| Go 崩溃堆栈        | `logs/crash-*.log`                                                   |
| Windows 监控控制台 | 启用窗口保留时的 `logs/console-*.log`                                |
| Docker 持久数据    | 数据卷挂载到 `/var/lib/opencode2api`                                 |
| Session 与监控累计 | 进程内存；重启清空                                                   |

运行和控制台日志按 10 MiB 轮转并保留 3 份备份。旧启动日志不会自动清理；排查后可自行清理。更多保留规则见[日志排查说明](docs/crash-diagnostics.zh-CN.md)。

## 项目目录结构

本运行包结构：

```text
OpenCodeReProxy_<version>_darwin_<arch>/
├── opencode2api       程序（内嵌管理页面）
├── config.example.json   配置模板
├── compose.yaml          Compose 部署文件
├── .env.example          Compose 环境变量模板
├── README.md             英文自述文档
├── README.zh-CN.md       中文自述文档
├── CHANGELOG.md          更新记录
├── docs/                 使用指南、参考文档与截图
├── scripts/              一键升级脚本
└── LICENSE               GPL 许可证
```

`config.json`、日志与缓存在首次配置或运行后生成，不随运行包提供。完整源码结构见[源码仓库](https://github.com/MDDC0412/OpenCodeReProxy)。

## 开发编译与测试

源码开发需要 Go 1.24+。在源码根目录执行：

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

发布构建可以通过 `-ldflags "-X main.version=自己的版本号"` 注入版本。修改 `webui/` 后必须重新编译并重启，管理页面资源由 Go embed 嵌入程序。

前端开发与检查需要 Node.js 22+：

```bash
npm ci --ignore-scripts
npm run check:web
npm run format:check
npx playwright-cli install-browser chrome
npm run test:web
```

浏览器回归使用隔离配置和本地模拟上游，报告保存在 `output/playwright/`。源码中不包含运行配置、凭据、日志或预编译程序。更多说明见[二开指南](docs/source-distribution.zh-CN.md)和[浏览器回归](docs/webui-regression.zh-CN.md)。

## 常见问题与故障排查

| 现象                                  | 检查与处理                                                                            |
| ------------------------------------- | ------------------------------------------------------------------------------------- |
| 找不到 `config.json`                  | 在运行目录创建该文件；确认没有保存成 `config.json.txt`，或使用 `-config` 指定完整路径 |
| 修改示例后仍报错                      | 程序读取 `config.json`，先从 `config.example.json` 复制再编辑                         |
| `parse config.json` / `unknown field` | 检查英文双引号、逗号和字段名；对照配置示例                                            |
| 管理密码过短 / 缺少密码               | 首次启用管理页面需设置至少 10 个字符的密码                                            |
| 端口占用                              | 停止自己的旧实例，或修改被占用的 API / 管理端口后重启                                 |
| 管理页面打不开                        | 检查程序是否仍在运行、`webui.enabled` 与实际管理端口                                  |
| Docker 提示无法连接 daemon            | 启动 Docker Engine / Docker Desktop，确认使用 Linux 容器                              |
| 容器运行但访问失败                    | 检查端口映射和卷内实际配置；容器监听使用 `0.0.0.0`；Compose 按示例创建 `.env`         |
| 修改宿主机配置不生效                  | 容器使用命名卷中的配置；通过管理页面修改或重新导入后重启                              |
| API 返回 401                          | 客户端使用启用的本地 `server_keys`，不要填写管理密码或上游 Key                        |
| 免费模型失败 / 502 / 504              | 查看请求台的具体错误及请求 ID，检查模型是否开放、上游凭据、网络与代理                 |
| 健康接口 503                          | 首次目录可能尚未加载；持续失败时查看目录刷新日志及网络错误                            |
| HTTP 200 但回答失败                   | 流式响应可能包含错误或异常断流；查看最终请求结果与日志                                |
| Windows 等待回车                      | 服务已异常停止；保留窗口错误和对应日志，修正后重新启动                                |
| 明文密码消失                          | 正常迁移为 `password_hash`；仍使用原密码登录                                          |
| 重启后累计监控消失                    | Session 和监控在内存中；持久运行日志位于 `logs/`                                      |

异常退出时保留同一次运行的 `runtime-*`、`crash-*`、存在时的 `console-*` 及轮转备份。诊断机制用于收集原因；此前报告的原版运行中偶发退出尚未复现，不能据此断言根因已经修复。

补充文档：[接口与配置](docs/reference.zh-CN.md) · [管理控制台](docs/management.zh-CN.md) · [异常退出排查](docs/crash-diagnostics.zh-CN.md)。

## 贡献与问题反馈

欢迎通过 [Issues](https://github.com/MDDC0412/OpenCodeReProxy/issues) 反馈问题或提出改进，通过 Pull Request 贡献代码。

反馈故障请包含：程序版本、操作系统和架构、启动方式、复现步骤、错误时间、模型 ID、是否流式，以及请求 ID 和对应日志。提交前移除密钥、密码、代理凭据和私人对话内容。

贡献代码时从 Fork 创建分支，说明问题、修改效果与验证结果；Go 修改运行 `go test ./...` 和 `go vet ./...`，前端修改执行对应格式、语法和浏览器检查。

## 版本与更新记录

当前程序版本为 `v1.4.0-secdev-diagnostics`，基线为上游 OpenCode2API v1.4.0（本次已同步 v1.3.8 – v1.4.0）。

| 类别       | 主要变化                                                                                                       |
| ---------- | -------------------------------------------------------------------------------------------------------------- |
| 代理错误   | 所有尝试都失败在代理链路（代理拨号、CONNECT/SOCKS 握手或隧道 TLS）时返回 503 `proxy_error`，不再误报为上游 502 |
| 免费通道   | 匿名通道改用独立上游主机 `upstream.anonymous`；新增 `models.free_only`，只暴露并路由免费模型                   |
| 用量统计   | 按服务端密钥与会话统计用量，API Key 列表显示每把密钥的用量                                                     |
| 模型接口   | `/v1/models` 增加 `cost` 与可选推理等级，并提供 `auto` 档探测                                                  |
| 兼容性修复 | 跨协议保留工具结果图片、`max_output_tokens` 下限 16、整次尝试统一超时、请求头阶段断连返回 499                  |
| 管理页面   | 九页导航、请求台、API Key 管理、CC Switch 导入、亮暗主题                                                       |
| 总览检查   | 检查摘要与异常数量一致，每项状态有独立边框和处理入口                                                           |
| 运行诊断   | Windows 异常窗口保留，持久化运行、控制台和 Go 崩溃日志                                                         |

版本下载见 [Releases](https://github.com/MDDC0412/OpenCodeReProxy/releases)，后续变化见 [CHANGELOG](CHANGELOG.md)。

## 许可证与致谢

采用 [GNU GPL-3.0-or-later](LICENSE)。修改和重新分发时请遵守许可证、保留上游归属，并按许可证要求提供对应源码。

感谢上游 [jasonxu114514/opencode2api](https://github.com/jasonxu114514/opencode2api)、[OpenCode](https://opencode.ai) 以及相关开源贡献者。
