# 接口、配置与运行参考

[返回 README](../README.zh-CN.md) | [English](reference.md)

这里保留详细的接口、配置、模型路由和监控说明。首次运行请先按 README 中对应的启动方式完成配置、启动和请求验证。

## Docker 部署

本项目的镜像为 `ghcr.io/mddc0412/opencodereproxy:v1.4.0-secdev-diagnostics`，支持 Linux amd64 / arm64。直接运行、Compose 和源码构建命令见 [README 快速启动](../README.zh-CN.md#快速启动)。

Compose 从 `config.json` 导入初始配置，将可写状态保存在 `opencode2api-state` 命名卷的 `/var/lib/opencode2api`。首次复制 `.env.example` 为 `.env`，模板显式设置容器监听 `0.0.0.0:8080` / `0.0.0.0:8081`，以便端口映射可达。

| Compose 变量                | 模板值                      | 作用                |
| --------------------------- | --------------------------- | ------------------- |
| `OPENCODE2API_VERSION`      | `v1.4.0-secdev-diagnostics` | 本项目镜像标签      |
| `OPENCODE2API_PORT`         | `8080`                      | 宿主机 API 端口     |
| `OPENCODE2API_WEBUI_PORT`   | `8081`                      | 宿主机管理端口      |
| `OPENCODE2API_LISTEN`       | `0.0.0.0:8080`              | 容器内 API 监听覆盖 |
| `OPENCODE2API_WEBUI_LISTEN` | `0.0.0.0:8081`              | 容器内管理监听覆盖  |

运行时配置以卷内文件为准，宿主机文件只在首次初始化时导入。后续在管理页面修改，或按 [README 运维步骤](../README.zh-CN.md#部署与运维)重新导入、修正文件所有者并重启。

代理文件须挂载到容器内，并在配置中使用容器可访问的路径。容器根文件系统只读，状态卷和 `/tmp` 可写；服务通过入口脚本降为非 root 用户运行。

## API 调用

`server_keys` 用于客户端调用本地网关，与 `zen_keys`、`go_keys` 相互独立，不会作为上游认证凭证发送。

请求可以使用 `Authorization: Bearer YOUR_LOCAL_API_KEY` 或 `x-api-key: YOUR_LOCAL_API_KEY`。健康检查无需认证。

| 方法 | 路径                   | 用途                       |
| ---- | ---------------------- | -------------------------- |
| GET  | `/v1/models`           | 当前配置下可以路由的模型。 |
| POST | `/v1/chat/completions` | Chat Completions。         |
| POST | `/v1/responses`        | Responses。                |
| POST | `/v1/messages`         | Anthropic Messages。       |
| POST | `/v1/systemone`        | System One 结构化决策。    |
| GET  | `/healthz`             | 就绪状态与资源汇总。       |

先获取可用模型：

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY"
```

将以下示例中的 `MODEL_ID` 替换为模型列表返回的 ID。

**Chat Completions**

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Hello"}]}'
```

**Responses**

```bash
curl http://localhost:8080/v1/responses \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","input":"Hello"}'
```

**Anthropic Messages**

```bash
curl http://localhost:8080/v1/messages \
  -H "x-api-key: YOUR_LOCAL_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","max_tokens":512,"messages":[{"role":"user","content":"Hello"}]}'
```

需要流式响应时，在请求体中加入 `"stream": true`，并使用 `curl -N`。响应通过 `x-request-id` 提供请求关联标识。API 请求体上限为 32 MiB。

**System One**

```bash
curl http://localhost:8080/v1/systemone \
  -H "Authorization: Bearer YOUR_LOCAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","state":"1 + 1 = 2","questions":{"arithmetic":{"type":"noul","instructions":"Is 1 + 1 equal to 2?"}}}'
```

System One 将自由文本状态与类型化问题配对，并返回类型化答案，因此其负载与 chat/responses 桥接结构完全不同，只做原样转发，不做任何转换。只有原生协议为 System One 的模型才能在本接口提供服务；在消息接口（`/v1/chat/completions`、`/v1/responses`）上提交的决策负载同样原样转发，而不经过转换。上游的流式回复按原样透传，不做中断恢复。

### 协议兼容边界

客户端协议与上游原生协议相同时，会保留供应商特有字段。跨协议请求经过统一中间结构转换，部分参数没有等价表达。例如，Chat 的 JSON 输出约束与 `seed` 不会被转发到 Anthropic Messages。不支持的内容块或工具类型可能被拒绝。

网关仅实现上表中的接口，不提供 embeddings、文件上传、图像生成、Responses 查询或取消接口。网关不保存会话历史；客户端需要提交历史，或使用实际上游支持的相关能力。

## 路由与模型

### 模型发现

服务按配置周期刷新 Zen / Go 的 `/v1/models` 和 OpenCode [能力目录](https://models.opencode.ai/api.json)，分别记录两个 Tier 的原生协议与模型限制。OpenCode 的 Zen / Go 文档作为协议发现的回退来源。

可通过 `models.protocols` 覆盖自动发现结果：

```json
{
  "models": {
    "refresh_seconds": 300,
    "protocols": {
      "custom-model": "chat"
    }
  }
}
```

协议值仅允许 `chat`、`responses`、`anthropic`。使用不支持的原生协议的模型会从模型列表中隐藏，除非配置了覆盖值。

成本、弃用信息及模型限制来自官方 [OpenCode 元数据地址](https://models.opencode.ai/api.json)，每 24 小时刷新一次。能力目录中的限制优先；元数据中的 `limit.context`、`limit.input` 和 `limit.output` 仅补充缺失字段，通过 `/v1/models` 的 `context_window` / `context_length`、`max_input`、`max_output` 暴露。每个 HTTP 客户端的拉取超时为 30 秒，刷新失败时保留已有数据。升级后继续兼容原有 `.models.dev.json` 缓存文件名，无需删除缓存。

### 匿名通道与回退

启用 `anonymous: true` 后，满足以下任一条件的模型可以进入 Zen 匿名通道：

- 模型 ID 包含 `free`，不区分大小写。
- OpenCode 元数据中的输入、输出成本均为零，且模型未弃用。

这只是网关的路由判断，上游仍可能拒绝或限制请求。匿名请求在上游认证头中使用 `public` 凭证。

路由顺序：

1. 符合条件的模型先尝试匿名通道，每个可用代理最多一次。
2. 按 `prefer` 顺序尝试认证通道，仅包含已配置 Key 且可以提供该模型的 Tier。
3. 每个认证 Tier 分别使用 `retry.max_attempts` 预算。

匿名阶段不受 `retry.max_attempts` 截断，但所有阶段共享请求总超时。网络错误、认证失败、限流和服务端错误可以触发 Key 轮换；其他 4xx 会结束当前 Tier，仍可继续尝试另一个可用 Tier。

每个 Tier 使用自己的原生协议编码请求。启用 `performance.first_event_timeout_seconds` 后，会先等待完整的 SSE data 事件再向客户端交付响应；注释心跳不算首事件。首事件前超时或断开会沿用现有 Key / 代理重试预算，交付后不再重放。默认 `0` 关闭，可对接受请求后无事件的上游尝试设置为 `15` 秒。它限制首事件而非首个文本 token，不适用于 System One；轮转别名使用独立的 `rotation.first_output_seconds`。识别到过期的 Responses reasoning 引用时，可执行一次修复重试；指定 Key 的诊断不会执行该重试。

只有匿名通道可用时，`/v1/models` 仅展示符合匿名条件的模型。

### 免费模型可用性

只探测 ID 含 `free`（不区分大小写），或元数据输入、输出价格均为零且未弃用的模型，按模型原生协议发送简短探测请求。先尝试已启用的匿名通道，再尝试一个配置的 Zen Key；任一通道返回完整有效响应即视为可用，只有所有实际尝试的通道都明确返回模型不存在或已下架时才自动禁用，从模型列表与路由中移除。认证失败、429、超时、网络错误、5xx 和响应格式异常均视为结果不确定，不新增禁用，1 小时后复测。探测不修改生产 Key / 代理的冷却状态，也不计入请求统计，但仍会访问上游并可能消耗免费额度。

System One 使用原生 `noul` 类型询问“1+1 是否等于 2”，不注入聊天或工具字段。模型列表标明 `native_protocol` 和 `supported_endpoints`；仅在配置 Go Key 且有可用模型时展示轮转别名。升级时解除 v1.3.6 旧探测生成的禁用并立即安排复测，避免非法会话 ID 造成的误判继续保留。

周期硬编码：正常模型每 **1 小时**探测，明确失效模型每 **24 小时**复测。复测成功自动恢复；也可在 WebUI「模型与额度」页的免费模型折叠块里立即恢复启用，下一次探测在 1 小时后。手动恢复不会被已经在途的旧探测结果覆盖。没有可用 Zen / 匿名通道，或推理协议不支持探测时跳过，不自动禁用。

状态保存在 `config.json.<上游地址指纹>.availability.json`，重启后保留，按 Zen 上游 URL 隔离。后台每分钟检查到期项，串行探测，每条通道最多等待 60 秒。

管理接口：`GET /api/models/availability` 查看状态；`POST /api/models/restore` 携带 `{"model":"MODEL_ID"}` 恢复。两者均需管理登录，恢复接口还需 CSRF 校验。

### 会话与代理

Key 初始化时均衡分配到代理。真实流量可以触发代理检查、Key 重新绑定和失败冷却。已经标记异常的代理，每 15 分钟通过 Cloudflare trace 复查一次。

服务使用稳定会话哈希选择首选 Key 或匿名代理。建议通过 `x-session-id` 区分独立会话，同时支持 `x-opencode-session`、`x-session-affinity`、`conversation-id`、`conversation_id` 与 `metadata.session_id`。

没有显式会话 ID 时，使用第一条用户消息生成会话标识，因此开场内容相同的会话可能共享亲和性。调整 Key 或代理池成员后，原有会话选择的节点可能变化。

失败冷却按指数增长，最高为 `performance.failure_cooldown_seconds` 的八倍；如果上游的 `Retry-After` 更长，则使用更长时间。所有认证 Key 都处于冷却时，路由仍可能尝试最早结束冷却的 Key；仍在冷却的匿名节点会被跳过。

## 配置参考

完整起始配置见 [config.example.json](../config.example.json)。配置支持 `//` 和 `/* ... */` 注释，未知字段和无效取值会被拒绝。

### Key、监听地址与路由

| 字段                        | 默认值或要求                                                                          |
| --------------------------- | ------------------------------------------------------------------------------------- |
| `listen`                    | `127.0.0.1:8080`。                                                                    |
| `server_keys`               | 至少一个本地 Key。                                                                    |
| `zen_keys`、`go_keys`       | 未启用匿名模式时，至少需要一个上游 Key。                                              |
| `anonymous`                 | `false`。                                                                             |
| `prefer`                    | `go`；可选 `go`、`zen`。                                                              |
| `upstream.zen`              | `https://opencode.ai/zen`。                                                           |
| `upstream.go`               | `https://opencode.ai/zen/go`。                                                        |
| `proxies`                   | 两个代理来源都为空时，使用 `["direct"]`。                                             |
| `proxyfile`                 | 可选；相对路径基于配置文件所在目录解析。                                              |
| `models.refresh_seconds`    | `300`；最小为 1。                                                                     |
| `models.protocols`          | `{}`；按模型 ID 覆盖原生协议。                                                        |
| `reasoning.effort`          | 空（关闭）；可选 `auto`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`、`none`。 |
| `reasoning.effort_by_model` | `{}`；按模型 ID 覆盖 `reasoning.effort`。                                             |

### 强制思考强度

`reasoning.effort` 在请求自身没有指定思考强度时，指定发往上游的强度；
`reasoning.effort_by_model` 按模型 ID 覆盖它。它对三种协议、两个转换方向都生效，
对跨协议请求尤其重要：Anthropic 的 `thinking.budget_tokens` 只能被 Chat 与
Responses 近似表达，而强制指定的强度可以到达单独靠预算表达不出的档位。

```json
{
  "reasoning": {
    "effort": "high",
    "effort_by_model": { "claude-opus-5": "max" }
  }
}
```

客户端显式指定的强度始终优先，因此它只替换无人指定的强度：完全未设置，
或由 `thinking.budget_tokens` 推断出来的强度。设为 `"none"` 可为未请求思考的
请求移除思考配置。

两个方向的强度都会如实保留：显式的 `output_config.effort` 不会再被 `thinking`
块遮蔽，`budget_tokens` 保持其档位（`8192` 为 `high`，`32000` 为 `xhigh`，
两者可区分），发往 Responses 时强度会以 `reasoning.effort` 形式传递而不再丢失。

WebUI 的配置中心也提供默认强度和按模型覆盖。管理 API 的 `GET /api/config`
会返回 `reasoning`，`PUT /api/config` 接受同名对象；省略该字段会保留现有值，
发送空对象则会清除它。

代理支持 `direct`、`http://`、`https://`、`socks5://` 和 `socks5h://`，URL 可包含认证信息。配置内代理先加载，再追加 `proxyfile` 内容，并按首次出现的顺序去重。

代理文件每行一个地址，允许空行和注释：

```text
# 首选代理
http://user:password@127.0.0.1:7890
socks5://127.0.0.1:1080  # 备用代理
direct
```

支持 `#`、`;`、`//` 注释标记，标记需要位于行首或空白字符之后。

### 超时与连接池

| 字段                                    | 默认值 | 含义                                             |
| --------------------------------------- | ------ | ------------------------------------------------ |
| `retry.max_attempts`                    | `3`    | 每个认证 Tier 的尝试次数，包含首次请求。         |
| `retry.timeout_seconds`                 | `300`  | 推理请求总超时，包含流式响应读取。               |
| `performance.attempt_timeout_seconds`   | `0`    | 每次尝试等待响应头的超时；0 表示使用请求总超时。 |
| `performance.connect_timeout_seconds`   | `5`    | 建立连接的超时。                                 |
| `performance.failure_cooldown_seconds`  | `15`   | 失败冷却基数。                                   |
| `performance.max_idle_conns`            | `2048` | 每个代理 Transport 的空闲连接上限。              |
| `performance.max_idle_conns_per_host`   | `256`  | 每个 Transport 对单个主机的空闲连接上限。        |
| `performance.max_conns_per_host`        | `0`    | 单个主机的连接上限；0 表示不限制。               |
| `performance.idle_conn_timeout_seconds` | `120`  | 空闲连接保留时间。                               |

单次响应头超时不会超过请求总超时。希望慢节点仍能留出回退时间时，可以将它设为小于总超时的值。请求上下文过期后会停止后续尝试，不会惩罚尚未实际使用的 Key 或代理。

### 日志与管理端

| 字段                          | 默认值或要求                                    |
| ----------------------------- | ----------------------------------------------- |
| `logging.level`               | `info`；可选 `debug`、`info`、`warn`、`error`。 |
| `logging.ring_size`           | `2000`；范围为 100–50,000。                     |
| `logging.dump_request_bodies` | `false`；配合 `debug` 日志级别输出上游请求体。  |
| `webui.enabled`               | 省略时为 `false`；示例配置设置为 `true`。       |
| `webui.listen`                | `0.0.0.0:8081`。                                |
| `webui.username`              | 启用 WebUI 时必填；示例为 `admin`。             |
| `webui.password`              | 首次初始化密码，最小长度为 10。                 |
| `webui.password_hash`         | 自动生成的 Argon2id 哈希，用于替代明文密码。    |
| `webui.session_ttl_minutes`   | `720`；范围为 5–10,080。                        |

启动时会将初始密码哈希化，并从配置中删除明文；包含初始明文密码的备份也会删除。配置和备份仍包含运行所需的上游 Key、代理凭据，应限制文件访问权限。

请求体日志默认关闭，每份准备好的上游请求体最多输出 64 KiB。已配置的敏感值会脱敏，但对话内容本身仍可能包含敏感信息。

二开版默认同时将运行日志保存到配置文件旁的 `logs/`，无需开启请求体日志。Windows 双击启动时自动监控服务进程：服务异常退出后保留窗口，显示捕获到的错误、退出码及日志位置，按回车后关闭。从已有终端启动时默认不暂停；可用 `-pause-on-error=always` 强制启用，或 `-pause-on-error=never` 禁用。`-log-dir` 可指定日志目录。日志文件和排查方法见[异常退出排查说明](crash-diagnostics.zh-CN.md)。

## WebUI 与诊断

管理监听端口提供 WebUI 和 `/api/*` 接口。认证使用单一管理员账号、服务端 Session、HttpOnly / SameSite Cookie、写操作 CSRF 校验及登录限速。

WebUI 共九个页面：总览、请求台、接入、事件日志、API Key、模型与额度、模型轮转、配置、账号。请求台独立位于 `/#playground`，侧栏列在总览之后。旧地址 `usage`、`guide` 跳转到总览；`diagnostics`、`model-quotas`、`availability` 跳转到模型与额度。

总览默认显示最近请求、模型与通道 Token 用量、请求分布；无请求时显示「暂无请求」，不误判成功率失败。全页支持白天 / 黑夜模式、预设和自定义主题色。浏览器只保存主题偏好；存储被禁用时仍能在当前页面切换主题。

管理登录限速仅信任 loopback 连接来源的代理头。`X-Forwarded-For` 从右向左跳过 loopback 跳数，使用首个非 loopback 地址，前面的客户端伪造前缀不能替换该地址；`X-Forwarded-Proto` 也仅从 loopback 来源信任。本机反向代理应覆盖这些头，或追加实际客户端地址；远程代理来源不在信任范围内。

| 管理接口                                         | 用途                                            |
| ------------------------------------------------ | ----------------------------------------------- |
| `POST /api/auth/login`                           | 登录，获取 Session Cookie 和 CSRF Token。       |
| `GET /api/auth/session`、`POST /api/auth/logout` | 查看或结束当前 Session。                        |
| `GET /api/config`、`PUT /api/config`             | 读取脱敏配置或应用修改。                        |
| `POST /api/config/reload`                        | 从磁盘重载配置。                                |
| `POST /api/config/reveal`                        | 验证密码后查看配置中的敏感值。                  |
| `PUT /api/account`                               | 验证密码后修改账号，修改后使已有 Session 失效。 |
| `GET /api/monitor`                               | 请求、Token、上游和资源统计。                   |
| `GET /api/debug/models`                          | 模型路由、Key 指纹和 metadata 诊断。            |
| `POST /api/debug/inference`                      | 执行 Playground 请求。                          |
| `GET /api/api-keys`                              | 列出本地访问密钥，只返回尾码。                  |
| `POST /api/api-keys`                             | 创建访问密钥。                                  |
| `PATCH /api/api-keys/{id}`                       | 重命名或启停密钥。                              |
| `DELETE /api/api-keys/{id}`                      | 删除密钥。                                      |
| `POST /api/api-keys/{id}/ccswitch-import`        | 生成 CC Switch 导入深链。                       |
| `GET /api/rotation`、`PUT /api/rotation`         | 读取或保存模型轮转分组。                        |
| `POST /api/rotation/current`                     | 手动指定分组的当前模型。                        |
| `GET /api/model-quotas`                          | OpenCode Go 共享套餐额度。                      |
| `GET /api/logs`、`GET /api/logs/stream`          | 最近日志或 SSE 实时订阅。                       |

API Key 页的「导入 CCS」打开独立弹窗，可搜索 Zen / Go 模型；目录含 `big-pickle` 时优先选择它，目录为空时可手填。Claude Code 使用裸 API origin，Codex / OpenCode 使用 `/v1` 后缀。打开操作将 `ccswitch://v1/import` 深链交给系统，无法确认外部客户端已导入；复制失败提供可选文本。密钥明文只在按需导入响应中出现，弹窗关闭时清空链接缓冲，不写入浏览器存储。详见[管理控制台说明](management.zh-CN.md)。

API Key 支持行内重命名。模型轮转的别名、超时和排序编辑显示未保存提示，刷新或手动切换当前模型时保留草稿，保存后生效。

Playground 需要已登录的 Session 和 `X-CSRF-Token`，每个客户端 IP 每分钟最多 12 次：

```json
{
  "protocol": "chat",
  "key": { "mode": "auto" },
  "request": {
    "model": "MODEL_ID",
    "messages": [{ "role": "user", "content": "Hello" }]
  }
}
```

服务端强制设置 `stream: false`。需要指定 Key 时，使用 `{"mode":"selected","tier":"zen","id":"KEY_FINGERPRINT"}`，其中指纹来自 `/api/debug/models`。指定 Key 的诊断最多执行一次上游尝试，不进入匿名通道，不轮换 Key，不切换 Tier，也不重放 reasoning 请求。

**自动模式和指定 Key 模式都不会改变生产 Key 的冷却、失败次数，以及代理的健康状态和绑定关系。** 诊断仍会真实访问上游，可能消耗供应商配额，并会计入监控。

执行后，管理接口返回 HTTP 200，实际结果位于 `ok`、`http_status`、`request_id`、`route` 和 `response`。指定 Key 时还返回 `selected_key` 与 `key_test`，其值包括 `usable`、`rejected`、`rate_limited`、`transport_error`、`upstream_error`、`request_error`、`unavailable`。管理请求格式、认证、CSRF 和限速错误仍返回各自对应的 HTTP 状态。

### 保存与重载

服务先验证候选配置并创建新的 Gateway，然后保存配置并切换实例。验证或保存失败时，当前 Gateway 继续处理请求；已经开始的请求继续使用原有实例。

Key、代理、上游地址、重试、模型、日志和路由偏好立即对新请求生效。`listen`、`webui.listen`、`webui.enabled` 的修改需要重启进程。保存后的 JSON 会规范化，不保留原有注释。

## 监控与持久化

请求结果以完整推理过程为准。即使 HTTP 200 已经发出，SSE 错误事件或异常断流仍会计为失败，并标记 `stream_error`；客户端取消标记为 `client_canceled`。WebUI 会将这些结果与 HTTP 状态一起显示。

上游**尝试记录**统计的是收到响应头为止的 HTTP 交互，其成功状态和耗时与完整 JSON 响应或流的最终完成情况分开计算。

Token 统计仅使用上游报告的 usage。输入 Token 包含缓存读取和写入，缓存 Token 单独统计缓存读取量。缺失 usage 时不估算 Token，覆盖率表示已经建立路由的推理请求中有多少报告了 usage。

| 数据                                            | 保存位置或保留规则                                                            |
| ----------------------------------------------- | ----------------------------------------------------------------------------- |
| Session、指标、Token 累计、最近 Playground 结果 | 进程内存；重启清空。                                                          |
| 请求与尝试明细                                  | 最近一小时，最多 10,000 个请求和 20,000 次尝试；API 分别最多返回 500 条。     |
| 实时日志环形缓冲                                | 进程内存，大小由 `logging.ring_size` 指定。                                   |
| 结构化日志                                      | stdout JSON，同时写入配置旁 `logs/runtime-*.log`，每份 10 MiB，3 份轮转备份。 |
| 崩溃及控制台记录                                | `logs/crash-*.log`；启用窗口保留时另有 `logs/console-*.log`。                 |
| 当前配置与上一版本                              | `config.json`、`config.json.bak`。                                            |
| 模型目录缓存                                    | `config.json.models.catalog.json`。                                           |
| 成本与弃用信息缓存                              | `config.json.models.dev.json`。                                               |

缓存文件名基于实际配置路径生成。lifetime 仅表示当前进程运行期间的累计，各实例之间不共享 Session 或监控状态。

### 健康检查

`/healthz` 不增加监控计数，也不触发网络请求；返回资源数量，不包含 Key 或代理地址。

- HTTP 503：模型目录尚未就绪、当前配置下没有可路由模型，或没有健康代理。
- HTTP 200：服务就绪。可用目录缓存过期时仍返回 200，同时模型状态为 `stale`，整体状态为 `degraded`。

过期阈值为 `models.refresh_seconds` 的两倍，且不低于 60 秒。就绪状态检查目录和资源，不会实际验证某个上游 Key 能否完成下一次推理。

### 自适应思考强度

将 `reasoning.effort` 或按模型覆盖值设置为 `auto`，即可使用免费模型探测实际验证过的最高可用档。已有具体档位语义不变，客户端显式指定的强度始终优先，不维护模型名称白名单。

基础请求成功后，对能力目录标记支持 reasoning 的模型依次验证 `max`、`xhigh`、`high`、`medium`、`low`、`minimal`。只有明确的思考参数不支持错误才尝试下一档；429、超时等临时失败会停止搜索，不能据此判断模型档位上限。全部档位均被拒绝时，不注入思考覆盖。

结果按上游 URL、模型与原生协议持久化，在可用性页展示，最多使用两小时内的验证结果。主动探测仍只覆盖免费 Zen/匿名模型；Go/付费模型、能力未知或尚未验证的模型不猜测档位。开启 `auto` 不扩大探测范围，也不能证明上游一定执行了所有接受的字段；每个模型可能增加数次简短请求并消耗上游额度。
