# OpenCodeReProxy

[简体中文](README.zh-CN.md) | [English](README.md)

将 OpenCode Zen / Zen Go 转为 OpenAI Chat Completions、Responses 和 Anthropic Messages 接口，附带管理页面、请求台、密钥管理与异常退出日志。基于 [OpenCode2API](https://github.com/jasonxu114514/opencode2api) v1.3.7 二次开发。

## 快速开始

运行包已包含程序和管理页面，运行时无需安装 Go、Node.js 或数据库。按下面四步操作。

### 1. 下载并解压

前往 [Releases](https://github.com/MDDC0412/OpenCodeReProxy/releases)，选择对应文件：

| 你的用途           | 下载文件                                  | 下一步                          |
| ------------------ | ----------------------------------------- | ------------------------------- |
| Windows x64 运行   | 名称以 `windows_amd64.zip` 结尾的运行包   | 按下面步骤配置并启动            |
| Linux x64 运行     | 名称以 `linux_amd64.tar.gz` 结尾的运行包  | 按下面步骤配置并启动            |
| ARM64 / macOS 运行 | 对应系统与架构的运行包                    | 按包内 README 操作              |
| 修改源码、二次开发 | `OpenCodeReProxy_*_source.zip` 或克隆仓库 | 安装 Go 1.24+，再按下面步骤操作 |

`source.zip` 和 GitHub 自动生成的 `Source code` 下载项都是源码。直接运行请选系统运行包。以下命令均在解压后的目录中执行。

### 2. 创建并修改 config.json

**程序默认读取 `config.json`，不会自动读取 `config.example.json`。只修改示例文件仍然会报“找不到 config.json”。**

首次使用时复制一份。已有 `config.json` 则跳过复制，直接编辑已有文件。

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

Windows 资源管理器请开启“显示 → 文件扩展名”，确认文件名是 `config.json`，而不是 `config.json.txt`。

在 `config.json` 中修改以下字段，其他字段先保留：

| 字段                               | 怎么填写                                                                         |
| ---------------------------------- | -------------------------------------------------------------------------------- |
| `server_keys`                      | 你自己设置的本地 API 密钥，例如 `["YOUR_LOCAL_API_KEY"]`；以后客户端填写这个密钥 |
| `webui.username`                   | 管理页面用户名，默认 `admin`                                                     |
| `webui.password`                   | 你自己设置的管理密码，至少 10 个字符；不要沿用示例值                             |
| `anonymous`、`zen_keys`、`go_keys` | 从下面三种上游方式中选一种                                                       |

| 上游方式        | `anonymous` | `zen_keys`         | `go_keys`         |
| --------------- | ----------- | ------------------ | ----------------- |
| 免费匿名模型    | `true`      | `[]`               | `[]`              |
| 使用 Zen Key    | `false`     | `["YOUR_ZEN_KEY"]` | `[]`              |
| 使用 Zen Go Key | `false`     | `[]`               | `["YOUR_GO_KEY"]` |

将表中的 `YOUR_...` 替换为实际值。本地 API 密钥由你设置；Zen / Go Key 从 [OpenCode](https://opencode.ai) 获取。匿名方式只适用于上游当前开放的免费模型。

配置是 JSON，字符串使用英文双引号，最后一项后不要加逗号。配置目录需要可写，供密码哈希、配置保存、缓存和日志使用。

### 3. 启动

Windows 运行包：

```powershell
.\opencode2api.exe -config .\config.json
```

也可以在配置完成后双击同目录中的 `opencode2api.exe`。请先解压整个运行包。

Linux / macOS 运行包：

```bash
chmod +x ./opencode2api
./opencode2api -config ./config.json
```

源码版（需要 Go 1.24+，命令适用于 PowerShell、Linux 和 macOS）：

```bash
go run ./cmd/opencode2api -config ./config.json
```

看到 API 和 WebUI 的 `server listening` 日志，表示两项监听已启动。保持程序运行；按 `Ctrl+C` 停止服务。

### 4. 登录并验证请求

浏览器打开 **<http://127.0.0.1:8081>**，使用刚填写的 `webui.username` / `webui.password` 登录。

进入“请求台”，从当前模型列表选择模型，发送“请回复 OK”。收到模型回复，才说明该模型的上游请求成功。

| 地址                            | 用途                         |
| ------------------------------- | ---------------------------- |
| `http://127.0.0.1:8081`         | 管理页面                     |
| `http://127.0.0.1:8080/healthz` | 服务就绪检查，无需密钥       |
| `http://127.0.0.1:8080/v1`      | OpenAI 兼容客户端的 Base URL |

以上为示例配置的端口。若你修改了 `listen` 或 `webui.listen`，请使用修改后的地址。管理页面的默认监听是 `0.0.0.0:8081`；仅本机使用时可将 `webui.listen` 改为 `127.0.0.1:8081`。

健康检查只检查目录和路由资源；真实推理是否成功以请求结果为准。

## 接入客户端

| 客户端设置              | 填写内容                                      |
| ----------------------- | --------------------------------------------- |
| OpenAI 兼容 Base URL    | `http://127.0.0.1:8080/v1`                    |
| Anthropic 兼容 Base URL | `http://127.0.0.1:8080`                       |
| API Key                 | `server_keys` 中启用的本地密钥                |
| 模型                    | 从管理页面或 `GET /v1/models` 获取可用模型 ID |

可用接口：`/v1/chat/completions`、`/v1/responses`、`/v1/messages`、`/v1/models`。部分模型只支持 `/v1/systemone`；以模型返回的 `supported_endpoints` 为准。请求样例见[接口与配置参考](docs/reference.zh-CN.md)。

也可以在管理页面“API Key”中创建本地密钥，并通过“导入 CCS”配置 CC Switch。

## 源码开发与编译

源码包目录名为 `OpenCodeReProxySouce`。源码不带预编译程序或运行配置；开发者按需要自行编译：

```powershell
# Windows
go build -trimpath -o opencode2api.exe ./cmd/opencode2api
```

```bash
# Linux / macOS
go build -trimpath -o opencode2api ./cmd/opencode2api
```

修改 `webui/` 后重新编译并重启。Go 测试与检查：

```bash
go test ./...
go vet ./...
```

Node.js 22+ 仅用于前端格式检查和浏览器回归。目录结构、前端命令和交付说明见[二开指南](docs/source-distribution.zh-CN.md)。

## 常见问题

| 现象                                                               | 处理方法                                                                                    |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| `read config.json: ... cannot find the file`                       | 在程序同目录创建 `config.json`；检查扩展名和终端当前目录，或用 `-config` 指定配置的完整路径 |
| `parse config.json` / `unknown field`                              | 检查 JSON 引号、逗号和字段名，对照示例配置                                                  |
| `webui.password must contain at least 10 characters`               | 将管理密码改为至少 10 个字符                                                                |
| `address already in use` / `Only one usage of each socket address` | 端口已被使用；停止已运行的实例，或修改对应的 API / 管理监听端口后重启                       |
| 管理页面打不开                                                     | 确认程序仍在运行、`webui.enabled` 为 `true`，并使用 `webui.listen` 对应端口                 |
| API 返回 401                                                       | 客户端填写 `server_keys` 中的本地密钥                                                       |
| 免费模型请求失败 / 502 / 504                                       | 在“请求台”和“事件日志”查看具体错误；检查模型是否仍开放、上游密钥、网络和代理                |
| 健康检查暂时返回 503                                               | 首次模型目录可能尚未加载；持续失败时查看日志中的目录刷新和网络错误                          |
| Windows 显示“服务异常退出”并等待回车                               | 服务已经停止；查看窗口给出的原因与 `logs/` 中对应日志，再修正问题并重新启动                 |
| 启动后 `webui.password` 消失                                       | 正常现象：初次启动会将明文迁移为 `password_hash`，登录仍使用原密码                          |

默认日志位于配置文件旁的 `logs/`。遇到异常退出，请保留对应的 `runtime-*`、`crash-*`、`console-*` 日志；提交问题时附上时间、模型和错误文字，勿提交含密钥的配置文件。

## 更多文档

- [接口、配置、路由和监控参考](docs/reference.zh-CN.md)
- [管理页面与请求台](docs/management.zh-CN.md)
- [异常退出与日志排查](docs/crash-diagnostics.zh-CN.md)
- [源码目录与二次开发](docs/source-distribution.zh-CN.md)
- [WebUI 浏览器回归](docs/webui-regression.zh-CN.md)

## 许可证

[GPL-3.0-or-later](LICENSE)。二次开发与分发请遵守许可证并保留上游归属。
