# 管理控制台：模型轮转、Go 额度与 API Key

本文说明管理控制台的模型轮转、Go 额度、API Key、主题和错误处理。

## 模型与额度

管理页面 `/#model-quotas`，登录后读取 `GET /api/model-quotas`。

- 模型来自网关同步的 OpenCode Go 目录，支持搜索和复制模型 ID。
- 额度由网关使用已配置的 Go Key 查询 `https://opencode.ai/zen/go/v1/usage`。浏览器不会收到上游 Key。
- 显示 5 小时、每周、每月窗口的剩余百分比和重置时间。接口只提供共享套餐窗口，不提供独立的逐模型余额，不推算金额或可用次数。多个 Key 的额度不相加。
- 成功响应缓存 60 秒，失败缓存 15 秒；失败时保留上次成功结果并明确标记历史数据。没有成功数据时显示未知。
- 页面打开期间每 60 秒查询一次；离开页面或隐藏浏览器时停止周期查询。手动刷新也受服务端缓存约束。

依据：[OpenCode 官方额度接口源码](https://github.com/anomalyco/opencode/blob/dev/packages/console/app/src/routes/zen/go/v1/usage.ts)。该接口未列入正式 API 文档，若响应格式变化，会显示查询异常而非虚构额度。

## API Key 管理

管理页面 API Key 页支持列表、生成、自定义、重命名、启用、停用、删除和创建后复制环境变量。

- 管理的是网关的 `server_keys`，不修改 OpenCode Go 的上游 Key。
- 自动生成 192 位随机密钥；自定义密钥为 16–256 位英文字母、数字、`-`、`_` 或 `.`。
- 列表只显示名称、尾码、状态和创建时间。创建后可复制完整密钥及环境变量。
- 变更热应用；停用和删除后，新请求返回 401。在途请求可以完成。至少保留一个启用的 Key。
- 所有管理接口需要登录，写操作需要 CSRF Token。
- 元数据以 `server_key_metadata` 保存在原配置中，按 Key 指纹关联；已有 `server_keys` 自动兼容。
- 当前未实现按 Key 单独分配模型权限、用量配额或过期时间。

### 一键导入 CC Switch

API Key 页面每行提供「导入 CCS」。点击后打开独立弹窗，选择目标客户端与模型，再复制导入链接或交给系统打开 CC Switch。弹窗不改变表格行高。

- 默认优先 `big-pickle`，候选合并当前可用 Zen / Go 及匿名模型；不会因存在 Go 模型而隐藏 Zen 的 `big-pickle`。没有该模型时选其他可用模型，目录为空时可手动填写。
- 搜索框聚焦显示全部候选，输入后过滤；支持方向键和 Enter 选择，Escape 先关闭候选列表，再关闭弹窗。弹窗支持 Tab / Shift+Tab，并在关闭后恢复触发按钮焦点。

- `POST /api/api-keys/{id}/ccswitch-import` 需要登录和 CSRF。请求体为 `{"app":"claude","model":"big-pickle","base":"http://127.0.0.1:8080"}`，三个字段都必填。
- 响应为 `{"url":"ccswitch://v1/import?…"}`，`Cache-Control: no-store`。深链含 `resource=provider`、`app`、`name`、`homepage`、`endpoint`、`apiKey`、`model` 与 `configFormat=json`。
- `app` 仅支持 `claude`、`codex`、`opencode`；其他值返回 400 `invalid_app`。`endpoint` 规则：`claude` 取 `base` 本身（Claude Code 自行拼接 `/v1/messages`，带 `/v1` 会变成 `/v1/v1/messages`）；`codex` 与 `opencode` 取 `base + "/v1"`，且不会重复追加。
- `base` 只接受 `http` / `https` 的裸 origin：不能带 userinfo、路径、查询或片段，否则返回 400 `invalid_base`。`model` 需为 1–120 个字符且不含空白与控制字符，否则返回 400 `invalid_model`。
- 指纹不存在时返回 400 `key_not_found`；错误文案经 `a.manager.Redact` 处理。
- 弹窗不自动关闭，便于同一 Key 再导入另一个客户端。打开只表示已交给系统，不表示客户端已导入；未安装 CC Switch 或协议未注册时可改用「复制导入链接」。剪贴板被拒绝时明确提示复制失败，并显示可选中手动复制的链接。
- 不发送 `usage*`、`enabled`、`config`、`configUrl`、`icon`、`notes` 以及 Claude 的 `sonnetModel` / `opusModel` / `haikuModel`：本网关没有 `/v1/usage`，带上只会让余额查询失败；角色位也不应由网关替客户端决定。
- 明文密钥不会由列表接口返回，只在按需导入响应中出现。链接会短暂存在页面内存，关闭或退出登录时清空；不写入浏览器存储或应用日志、监控。复制或打开后，剪贴板和操作系统如何保留链接取决于系统。日志仅记录 `api_key_import_link_issued` 事件、`client_ip` 与 Key 指纹。

## 界面结构

管理页面共九项：总览、请求台、接入、事件日志、API Key、模型与额度、模型轮转、配置、账号。侧栏分为运营、密钥与模型、系统三组，请求台为第二项。`/#playground` 是独立请求台；旧地址 `usage`、`guide` 跳转到总览，`diagnostics`、`model-quotas`、`availability` 跳转到模型与额度。

总览默认显示最近请求、模型与通道 Token 用量和请求分布；无请求时显示「配置就绪 · 暂无请求」。状态检查展开后列出管理登录、本地 Key、上游通道、代理节点、模型目录、请求成功率六项，各项名称、状态与当前数据置于同一边框内。顶部状态、检查摘要和异常列表共用判断；近一小时成功率低于 90%、目录过期、代理不可用或监控更新失败时，不再提示全部通过。异常列表直接显示原因与处理入口，成功率异常显示请求及失败数量，「查看失败请求」跳转最近请求并勾选「仅失败」，可取消筛选。无请求的成功率检查为中性状态，未取得新监控数据时保留历史数据并明确告警。

顶部支持模式切换及主题弹窗：白天 / 黑夜、六个预设色、颜色选择器、`#RRGGBB` 十六进制输入。主题在样式表加载前初始化，按钮文字和选中浅底随主题色派生；禁用浏览器存储时保留当前页面内存状态。主题偏好不含密钥。

管理登录的代理信任规则：只信任 loopback socket peer。`X-Forwarded-For` 从右向左解析，跳过 loopback 后取首个非 loopback IP，遇到非法项则回退 socket peer；不信任更靠左的客户端前缀。`X-Forwarded-Proto` 同样只信任 loopback 来源。本机代理需覆盖代理头或追加真实客户端地址，不能原样透传客户端伪造的代理头。远程反向代理按 socket peer 限速，未提供任意可信 CIDR 配置。

## 验证和构建

```bash
go test ./...
go test -race ./...
go vet ./...
npm run check:web
npm run format:check
npm run test:web
go build -trimpath -o opencode2api ./cmd/opencode2api
```

升级前请备份程序、配置及轮转状态，再停止服务并替换可执行文件。旧版本不认识新增的 `server_key_metadata` 和 `rotation` 字段；回退时应同时恢复旧程序和旧配置。

浏览器回归使用独立端口及模拟上游，具体前置条件、范围和限制见[浏览器回归说明](webui-regression.zh-CN.md)。修改嵌入前端后必须重新构建并重启服务，刷新页面不会更新旧二进制。

Windows 双击新版 exe 会监控服务，异常退出后保留控制台并显示退出码和日志位置。日志默认写入配置文件旁的 `logs/`；自定义配置路径时，日志目录也跟随该配置。记录内容、启动选项和提交故障证据的方法见[异常退出排查说明](crash-diagnostics.zh-CN.md)。

## SOTA / 甜点模型轮转

管理页：`/#rotation`。调用时继续使用 网关的 Base URL（默认
`http://127.0.0.1:8080/v1`）和已有 API Key，将 `model` 设为 `sota` 或
`sweet`。两个别名列入 `/v1/models`，支持 Chat Completions、Responses 和
Messages，以及这三种协议的流式输出。真实模型 ID 的原有调用路径保持不变。

- SOTA 固定成员：`deepseek-v4.1-flash`、`glm-5.3`、`grok-4.6`、`kimi-k3`、
  `qwen3.8-max`，默认按上述顺序。
- 甜点组包含其余 Go 可用模型，初次以 `glm-5.3-flash` 为首，随后按名称排序。
  新模型追加在末尾；下架或协议不受支持的模型保留在列表，但不参与调用。
- 每组共享一个当前模型。成功后保持位置；失败切换后，下次从新位置开始。
  并发请求通过位置版本号避免旧请求把游标移回去；手动切换不打断在途回答。
- 普通错误每模型最多 3 次（包含首次），每请求最多消耗 3 个普通错误模型预算。
  模型耗尽重试后暂停 15 秒。分组请求不使用原有 Key 重试、跨 Tier 降级、
  自动重定向或 stale-reasoning 补发，也不因单个模型故障暂停整个 Go Key。
- 429 或明确额度错误直接跳到下一模型，不新增普通错误模型计数；已经发生的
  普通错误仍计数。允许遍历整组，同一请求不重复模型。暂停时间采用 Retry-After，
  未提供时为 60 秒。共享套餐额度只用于展示，不能保证换模型后就有余额。
- 整体超时沿用 `retry.timeout_seconds`。`rotation.first_output_seconds` 默认
  30 秒，限制每次等待首个有效输出的时间，不限制已经开始的正常流式回答。
- 流式 start/role/usage 前导事件会等到有效输出或正常结束后再提交。提交前错误
  可重试；提交后中断会保留已输出内容、追加协议对应的 SSE 错误并结束，随后
  推进当前位置，下一次请求使用下一个模型。不会在同一响应里拼接替代答案。
  调用方取消不会触发暂停或切换。
- 响应 JSON / SSE 的 `model` 保持请求别名，`X-Resolved-Model` 标记实际模型。
  用量按实际上游模型记录。轮转日志与管理页面仅保留分类后的错误原因，
  不记录上游错误正文、请求正文或密钥。最近保留 100 条尝试和每组 30 条切换。

配置保存于 `config.json` 的 `rotation` 字段：`sota` / `sweet` 各有 `alias`
和 `order`；状态单独原子写入 `config.json.rotation.json`（0600），包括当前
位置、版本、动态追加的顺序、暂停期限和最近记录。修改 Key 或其他配置时
沿用同一个轮转状态管理器。页面展示状态写入失败提示，首次无法写入则拒绝启动。

管理接口均需要现有管理 Session，写入另需 CSRF：

页面中的别名、首个输出超时和顺序编辑会标记「未保存」；上移 / 下移、刷新、手动设为当前模型都保留草稿，保存时才提交配置。手动切换只更新运行状态，并显示最新当前位置。

- `GET /api/rotation`：分组配置、可用状态、暂停期限、切换和尝试记录。
- `PUT /api/rotation`：提交 `sota` / `sweet` 的 `alias`、完整 `order`，以及
  `first_output_seconds`；别名不得重复或与真实模型 ID 冲突，成员不得跨组。
- `POST /api/rotation/current`：`{"group":"sota","model":"glm-5.3"}`；立即
  修改当前模型并解除该模型的临时暂停。

验证：`go test -race ./...`、`go vet ./...`、`npm run check:web`。
模拟上游测试覆盖 9 次普通错误上限、超过 3 个额度跳过、混合错误预算、
Retry-After、单个模型故障不影响 Key、三种入口协议的跨协议转换、成功保持、
别名发现与用量归属、畸形响应、输出前错误、首个输出超时、输出后故障、
调用方取消、在途回答不受手动切换影响、并发版本保护、下架和重启恢复。
上线前建议使用独立配置及端口验证管理登录、CSRF、页面排序与保存、别名冲突、
Key 配置热更新和真实 Go 请求。测试与正式配置、凭证、日志及运行状态不应提交到仓库。
