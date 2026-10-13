# OpenCodeReProxy · macOS 运行版

[简体中文](README.zh-CN.md) | [English](README.md)

本包包含 `opencode2api` 和内嵌管理页面。无需安装 Go、Node.js 或数据库。

## 1. 创建配置文件

解压对应系统和 CPU 架构的压缩包，在终端中用 `cd` 进入包含 `opencode2api` 的目录。

**程序默认读取 `config.json`，不会自动读取 `config.example.json`。** 首次使用执行下面的复制和编辑命令；已有 `config.json` 时跳过复制，直接编辑已有文件。

```bash
cp -n config.example.json config.json
open -e config.json
```

`open -e` 会打开文本编辑器，保存时保持 JSON 的纯文本格式。

## 2. 填写密钥、密码和上游方式

在刚创建的 `config.json` 中修改：

| 字段             | 填写内容                                                               |
| ---------------- | ---------------------------------------------------------------------- |
| `server_keys`    | 你自己设置的本地 API 密钥，例如 `["YOUR_LOCAL_API_KEY"]`，供客户端接入 |
| `webui.username` | 管理页面用户名，默认 `admin`                                           |
| `webui.password` | 你自己设置的管理密码，至少 10 个字符                                   |

再从下面选择一种上游方式：

| 使用方式     | `anonymous` | `zen_keys`         | `go_keys`         |
| ------------ | ----------- | ------------------ | ----------------- |
| 免费匿名模型 | `true`      | `[]`               | `[]`              |
| Zen Key      | `false`     | `["YOUR_ZEN_KEY"]` | `[]`              |
| Zen Go Key   | `false`     | `[]`               | `["YOUR_GO_KEY"]` |

将 `YOUR_...` 替换为实际值。Zen / Go Key 从 [OpenCode](https://opencode.ai) 获取。其他配置先保留示例值；免费模型能否使用取决于上游当前开放情况。

保存文件后再启动。JSON 字符串使用英文双引号，最后一项后不要加逗号。

## 3. 启动服务

在包含程序和配置的目录执行：

```bash
chmod +x ./opencode2api
./opencode2api -config ./config.json
```

看到 API 和 WebUI 的 `server listening` 日志，表示监听已经启动。保持程序运行；按 `Ctrl+C` 停止服务。配置目录需要可写。

## 4. 登录并测试

浏览器打开 **<http://127.0.0.1:8081>**，用户名为 `webui.username`（默认 `admin`），密码为你刚填写的 `webui.password`。

进入“请求台”，选择当前可用模型，发送“请回复 OK”。收到模型回复后即可接入客户端：

| 设置                    | 填写内容                                    |
| ----------------------- | ------------------------------------------- |
| OpenAI 兼容 Base URL    | `http://127.0.0.1:8080/v1`                  |
| Anthropic 兼容 Base URL | `http://127.0.0.1:8080`                     |
| API Key                 | `server_keys` 中启用的本地密钥              |
| 模型                    | 管理页面或 `GET /v1/models` 中的可用模型 ID |

就绪检查地址：`http://127.0.0.1:8080/healthz`。若修改了监听端口，上面的地址也要同步修改。健康检查通过表示路由资源就绪；真实推理以请求结果为准。

管理默认监听 `0.0.0.0:8081`；仅本机使用时可将 `webui.listen` 改为 `127.0.0.1:8081`。

## 常见报错

| 报错或现象                                           | 处理方法                                                                                    |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `read config.json: ... cannot find the file`         | 把示例复制为 `config.json`；只编辑 `config.example.json` 不会生效。检查文件名和终端所在目录 |
| `parse config.json` / `unknown field`                | 检查 JSON 双引号、逗号和字段名                                                              |
| `webui.password must contain at least 10 characters` | 修改为至少 10 个字符的管理密码                                                              |
| 端口占用 / `address already in use`                  | 停止已运行的实例，或修改被占用的 API / 管理监听端口后重启                                   |
| 管理页面打不开                                       | 确认服务仍在运行、`webui.enabled` 为 `true`，并使用正确的管理端口                           |
| 客户端 401                                           | 使用本地 `server_keys`，不是管理密码或上游 Key                                              |
| 免费模型失败 / 502 / 504                             | 查看“请求台”和“事件日志”的具体错误，检查模型、上游密钥、网络与代理                          |
| `Permission denied` / `Exec format error`            | 执行 `chmod +x opencode2api`，并确认下载包的系统和架构正确                                  |
| 明文密码在配置中消失                                 | 首次启动会迁移为 `password_hash`；继续用原密码登录                                          |

## 日志与更多说明

运行日志默认位于配置文件旁的 `logs/`。异常退出时保留对应的 `runtime-*`、`crash-*`，以及存在时的 `console-*` 文件。

- [接口与完整配置](docs/reference.zh-CN.md)
- [管理页面与请求台](docs/management.zh-CN.md)
- [异常退出排查](docs/crash-diagnostics.zh-CN.md)
- [源码仓库](https://github.com/MDDC0412/OpenCodeReProxy)

许可证：[GPL-3.0-or-later](LICENSE)。基于 [OpenCode2API](https://github.com/jasonxu114514/opencode2api) v1.3.7 二次开发。
