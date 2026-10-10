# WebUI 改版与 cc-switch 一键导入 · 设计定稿

日期：2026-10-09。对象目录：`opencode2api-1.3.7`（二次开发副本，该目录当前没有 `.git`，因此本设计文档与后续改动均先落在工作树，未提交）。

用户已确认的四项前提：允许整套重构（功能核心保留即可）；导航压到 8 项；长说明收进问号图标点开才展开；导入形态为"按钮 + 小面板选客户端和模型"，点击后直接唤起 CC Switch，不在导入前额外索要管理密码。

## 1 目标

- 观感从"印刷手册"改为浅色低装饰控制台：去掉衬线大标题、米黄纸底、墨绿侧栏配橙粗边、全大写等宽小标签、卡片阴影。
- 一屏之内只让一到两个决策数字处于最显眼位置，其余降为同行小字。
- 13 个导航项压到 8 个，能力零删除，只改变落位。
- API Key 每行一键生成 `ccswitch://` 深链并交给系统唤起 CC Switch。

## 2 明确不做

- 不做深色主题，不做按 Key 的模型权限、配额或过期时间。
- 不引入任何前端依赖、构建步骤或 CDN（维持三文件内嵌：`index.html` / `app.js` / `styles.css`）。
- 不做 CC Switch 的余额脚本（`usageEnabled` / `usageScript`）。依据：本网关只有 `GET /v1/models`、`POST /v1/chat/completions`、`POST /v1/responses`、`POST /v1/messages`、`POST /v1/systemone`、`GET /healthz`（`internal/gateway/gateway.go:79-84`），没有 `/v1/usage`，带上只会在 CC Switch 里显示"查询失败"。
- 不改任何协议转换、路由、轮转、额度的后端行为。

## 3 复用清单与自研清单

复用（外部已发布规范，均以一手源码为据，不自行发明格式）：

- CC Switch 深链 v1 协议本身：`ccswitch://v1/import?resource=provider&...`。参数名与必填项取自 `farion1231/cc-switch` 的 `src-tauri/src/deeplink/parser.rs`（`parse_provider_deeplink` 读取 `app`、`name`、`homepage`、`endpoint`、`apiKey`、`model`、`configFormat`、`enabled`、`usage*`），URL 形态约束取自同文件对 scheme、host=`v1`、path=`/import` 的校验。
- 三种目标客户端的配置语义，取自 `src-tauri/src/deeplink/provider.rs`：`build_claude_settings`（311-352 行）、`build_codex_settings`（398-452 行）、`build_opencode_settings`（502-526 行）。
- 导入后由 CC Switch 自己弹"确认导入供应商配置"再写入，取值 `deeplink/mod.rs` 中 `enabled` 默认为关闭的注释，以及 `src/components/DeepLinkImportDialog.tsx`。
- sub2api 只借交互形状：每行一个导入按钮、前端拼深链、`ccswitch://v1/import?` + `URLSearchParams`（`Wei-Shaw/sub2api` `frontend/src/utils/ccswitchImport.ts`）。不搬它的 platform 分支与余额脚本。
- 本项目既有资产：Session 认证与 `a.csrf` 中间件、`config.Fingerprint` / `config.MaskValue`、`httpx.WriteJSON`、`writeAdminError`、`a.manager.Redact`、前端 `modal` / `toast` / `codeCard` / `copyText` 组件、`npm run check:web` 与 Prettier 闸门。

自研（与本项目路由绑定，无处可复用）：

- 设计基线 CSS（颜色、字号、间距、组件规格）。
- 深链构造器与"要不要带 `/v1`"的判定，放在 Go 侧而不是 JS 侧。理由：本项目 WebUI 没有 JS 测试运行器（只有 `node --check` 与格式检查），而这条规则恰恰最容易错，必须落在 `go test` 能覆盖的地方。

## 4 设计基线

颜色 token 共 13 个：`--bg #ffffff`、`--surface #fafafa`、`--surface-2 #f4f5f7`、`--border #e3e6ea`、`--border-strong #cfd4da`、`--text #1f2328`、`--text-muted #616b76`、`--accent #2563eb`、`--accent-weak #eef2ff`、`--ok #15803d`、`--warn #b45309`、`--bad #b42318`、`--code-bg #14181d`。其中主强调色只有 `--accent` 一个；三个状态色只用于状态点与错误文本；两个深色只用于代码块与日志区。

字体：定义两个变量 `--font-sans`（`system-ui, -apple-system, "Segoe UI", sans-serif`）与 `--font-mono`（沿用现有等宽栈），除此之外 `styles.css` 不再出现任何具体字体名。等宽只用于密钥尾码、模型 ID、代码块、日志。不出现 `Georgia` 或任何衬线字体名。

字号阶梯五档：12 / 13 / 15 / 18 / 26，行高 1.45，正文 13px，页面标题 18px，板块标题 15px，主指标 26px。

间距：以 4px 为基数。卡片内边距 14px，卡片之间 12px，板块标题到内容 10px，主区左右内边距 `clamp(16px, 3vw, 40px)`，导航项高 32px。

圆角 6px（含按钮、输入框、卡片、弹层），边框一律 1px。阴影只给浮层：`0 6px 18px rgba(16,24,40,.08)`；卡片、表格、按钮一律无阴影。

侧栏：宽 200px，浅色（`--surface`），无上边框装饰条、无 kicker、无 `CONTROL PLANE LIVE` 与版本号大标语；底部只留版本号一行。导航项选中态为 `--accent-weak` 底色 + 左侧 2px `--accent` 竖条 + `--accent` 文字；hover 用 `--surface-2`；导航文本前不加 01/02 编号。

顶栏：一行，左侧页面标题 18px，右侧操作按钮（32px 高）。不显示 `Section 01` 眉标；原有副标题文本迁入问号说明。

按钮：主按钮 32px 高、`--accent` 底、白字；次按钮同高、白底 1px `--border-strong`；表格行内按钮 26px 高、文字 12px；danger 仅文字与边框转 `--bad`；禁用态整体 45% 不透明。删除 `.primary:hover` 变橙色的规则。

表格：表头 12px `--text-muted`、正常大小写、不强制小写；单元格 13px；行 hover `--surface`；行分割线 `--border`；表格外层仍套卡片边框。

状态呈现：6px 圆点 + 2 至 3 字文本，取值为 正常 / 冷却 / 停用 / 启用 / 异常 / 未知。

指标：每页最多两个 26px 大字，其余指标以 13px `--text-muted` 排在同一行，中间用竖线分隔。

登录页：整页 `--surface` 底，居中一张 360px 白卡（1px `--border`、6px 圆角、无阴影），删除左侧半屏的宣传大标题、网格底纹与橙色色块。

## 5 导航与归并（13 → 8）

八个导航项及 hash / 页面 id（沿用现有名字以减少改动面）：总览 `overview`、接入与测试 `access`、模型与额度 `models`（新）、API Key `api-keys`（不改名）、模型轮转 `rotation`、配置 `config`、日志 `logs`、账号 `account`。

原 13 页每一块的新落位，一不删除：

- 运行桌面 → 总览。
- Token 用量 → 总览（趋势图、模型排行、Tier 分布、四个用量指标降为次级小字）。
- 首次运行六步检查 → 总览顶部一条"上线检查 n/6"，点开为折叠清单。
- 路由诊断 · 模型 Metadata → 模型与额度（折叠块）。
- 路由诊断 · 最近 Playground 事实块 → 接入与测试（并入诊断结果区）。
- 路由诊断 · 模型路由表 → 模型与额度（折叠块，沿用现有筛选输入框）。
- 路由诊断 · 请求路由 + 上游尝试两张长表 → 总览的"查看最近请求"折叠区。
- 接入手册 + Playground → 接入与测试（左示例、右请求台）。
- 模型与额度（旧 `model-quotas`）+ 免费模型可用性 → 模型与额度（额度区 → 可用模型表 → 可用性折叠块）。
- API Key 管理、模型轮转、配置中心、事件日志、账号安全 → 各自同名页。

旧 hash 归一表（写在 `app.js` 一处，`openPage` 入口先归一再渲染）：`usage`、`guide`、`playground`、`diagnostics` → 对应新项，`availability`、`model-quotas` → `models`，`api-keys`、`overview`、`access`、`rotation`、`config`、`logs`、`account` 保持原样。

每页首屏顺序（DOM 按此排）：

1. 总览：上线检查条 → 指标条（主：最近一小时请求数、成功率；次：P95、并发与流式；再次：Token 总量、缓存、推理、覆盖率）→ "需要你处理"块（无异常时整块不渲染）→ 每分钟 Token 趋势 → Key 池 + Proxy 节点两栏 → 请求分布 + 模型排行 + Tier 分布 → 最近请求折叠区。
2. 接入与测试：左列 Base URL 只读行 + 三协议 curl + Python/JS 示例 + 接入注意（问号内）；右列入口协议、模型、上游 Key、请求 JSON、发送按钮 → 结果事实块 + 原始响应。
3. 模型与额度：Go Key 选择器 → 三窗口剩余（仅"5 小时窗口"用大字）→ 更新时间与来源一行 → 可用模型表（搜索、复制名称）→ 免费模型可用性折叠 → 模型 Metadata 与模型路由表折叠。
4. API Key：新建密钥（折叠表单）→ 创建成功横幅（一次性明文 + 两个复制按钮）→ 列表（名称 / 尾码 / 状态 / 创建时间 / 操作：停用或启用、导入 CCS、删除）。
5. 模型轮转：两组当前状态（当前模型、下一个、是否暂停）→ 别名与顺序表单 → 首个有效输出等待上限 → 最近请求尝试折叠。
6. 配置：六组两列表单（服务与管理端 / 密钥与匿名通道 / Proxy 节点 / 上游与协议路由 / Thinking 强度 / 性能与日志，组顺序不变）→ 底部固定操作栏（从磁盘重载、验证保存并应用）。
7. 日志：一行过滤条（级别、模块、搜索、暂停）→ 深色日志流区。
8. 账号：一张 520px 窄卡。

## 6 文案规则

- 每个板块标题右侧放一个问号按钮（`aria-label="说明"`、`aria-expanded`），点开后在标题下方原地展开原文，再点收起；悬停不做展开（触屏与键盘不可用）。默认可见的说明只留一句，不超过 40 个汉字。
- 重写只做去重与压缩，规则数字和条件一个不丢，逐项保留：普通错误每模型 3 次、每请求最多 3 个错误模型、额度成功缓存 60 秒、失败缓存 15 秒、尝试与路由各保留最近 100 条、失效模型 24 小时复测、正常模型 1 小时探测、限流等 1 小时重试、自定义 Key 16–256 位、最多 200 个 API Key、至少保留一个启用 Key、密码至少 10 字符、Session TTL 5–10080 分钟、状态文件 0600。
- 删除全部装饰性英文（`Field Manual`、`Operator console`、`Section 01`、`Restricted console`、`Quick index`、`CONTROL PLANE LIVE`、`Sensitive values`）。协议名、字段名、模型 ID、HTTP 状态与错误码保留原文。
- 标题层级最多两级：页面标题 18px、板块标题 15px，不再有 48px 级标题。
- 表格行内按钮文本 2 至 4 字（复制、停用、启用、删除、导入 CCS）。

## 7 API Key 一键导入 CC Switch

交互流程：

- 列表每行一个"导入 CCS"按钮，点击打开小面板。面板含：目标客户端单选（Claude Code / Codex / OpenCode，默认 Claude Code）、模型单选（数据源沿用现有 `/api/debug/models` 的可用 Go 模型清单，默认值沿用现有逻辑：优先 `glm-5.3-flash`，否则第一个可用 Go 模型）、主按钮"打开 CC Switch"、次按钮"复制导入链接"，以及一行小字：导入后需在 CC Switch 内手动启用该供应商。
- 模型清单为空时，模型字段变成可手填输入框；无论单选还是手填，模型为空时两个按钮都禁用（`YOUR_MODEL` 这类占位串绝不进深链）。
- "复制导入链接"与"打开 CC Switch"共用一次接口调用：点击后先 `POST` 取回 `url`，前者写入剪贴板，后者交给系统；两者都在按钮禁用期间显示"生成中"。
- 点"打开 CC Switch"：按钮立即禁用 → `POST /api/api-keys/{id}/ccswitch-import` → 取回 `{url}` → `window.location.assign(url)` → 按钮恢复，面板保留并显示"已交给系统；若没有弹出 CC Switch，说明未安装或协议未注册，可用复制导入链接"。
- 面板不自动关闭、选择保留，便于同一 Key 再导入另一个客户端。
- 创建成功横幅不放导入按钮，统一走列表行（新 Key 创建后就在列表里），避免出现两条实现路径。

深链组成（由服务端生成，参数顺序与命名以 `parser.rs` 为准）：

`ccswitch://v1/import` 加 `resource=provider`、`app`、`name`（Key 名称，URL 编码）、`homepage`（等于 base）、`endpoint`（规则见第 8 节）、`apiKey`（完整明文）、`model`、`configFormat=json`。

不发送的参数：`usage*` 全部（理由见第 2 节）、`enabled`（用 CC Switch 自身默认值，不替他决定是否立即切换）、`config`、`configUrl`、`icon`、`notes`、`haikuModel`、`sonnetModel`、`opusModel`（不把单个模型塞进 Claude 的三个角色位）。

## 8 新增后端接口契约

`POST /api/api-keys/{id}/ccswitch-import`

- 中间件链与现有写操作一致：`a.authenticate(a.csrf(...))`；`id` 是 `config.Fingerprint` 的 10 位指纹。
- 请求体：`{"app":"claude|codex|opencode","model":"<模型 ID>","base":"http://127.0.0.1:8080"}`，`Content-Type: application/json`。
- `app` 不在三值内 → 400 `invalid_app`。
- `base` 校验：`net/url` 可解析、scheme 仅 `http` 或 `https`、host 非空、不含 userinfo、path 为空或 `/`；违反 → 400 `invalid_base`。
- `model` 校验：1–120 字符、不含空白与控制字符；违反 → 400 `invalid_model`。
- 指纹在 `server_keys` 中不存在 → 400 `key_not_found`，文案"API Key 不存在，请刷新列表"。
- `endpoint` 规则（本项目路由事实 + CC Switch 构造器语义）：
  - `claude` → `endpoint = base`（不带 `/v1`）。CC Switch 写 `ANTHROPIC_BASE_URL=endpoint`，Claude Code 自行请求 `/v1/messages`；带上 `/v1` 会变成 `/v1/v1/messages`。
  - `codex` → `endpoint = base + "/v1"`。CC Switch 写进 `config.toml` 的 `base_url`，`wire_api="responses"`，Codex 只在 `base_url` 后拼 `/responses`，本项目的 Responses 入口是 `/v1/responses`。
  - `opencode` → `endpoint = base + "/v1"`。CC Switch 写进 `options.baseURL`（`@ai-sdk/openai-compatible`），该 SDK 在 `baseURL` 后拼 `/chat/completions`。
- 成功：200，`Cache-Control: no-store`，`{"url":"ccswitch://..."}`。
- 日志：一条 `Info`，事件名 `api_key_import_link_issued`，字段 `component=auth`、`client_ip`、`key_id`（指纹），与现有 `secrets_revealed` 同一风格；绝不记录明文。
- 所有错误响应经 `a.manager.Redact`。

安全边界（已知代价，经用户选择"一键就走"而接受）：持有有效管理会话的浏览器可据此取得完整密钥明文。缓解与现状：`GET /api/api-keys` 行为不变仍只回尾码；明文只在按需发起的这一次 POST 响应中出现；响应 `no-store`；POST 走 CSRF；深链仅在交给操作系统时存在一次；不写 localStorage、不进日志、不进监控（`telemetry/http.go` 的中间件仅记录 path、status、duration，path 中只有指纹）。

## 9 错误处理

- 接口返回 4xx / 5xx：面板内一行 `--bad` 文本显示服务端 message，保留已选客户端与模型，按钮恢复可点，面板不关闭。
- 网络失败或超时：同一行显示"网络失败，可稍后重试或手动复制环境变量"。
- 会话失效：沿用 `api()` 现有 401 → 回到登录页，不额外弹提示。
- CC Switch 未安装：不虚构"已导入"成功提示，只保留复制导入链接退路。
- 剪贴板不可用：沿用 `copyText()` 现有 toast 报错分支。

## 10 验证判据

闸门命令须全部通过：`go test ./...`、`go test -race ./...`、`go vet ./...`、`gofmt -l cmd internal webui` 无输出、`npm run check:web`、`npx prettier --check webui/index.html webui/app.js webui/styles.css`、`go build -trimpath -o opencode2api ./cmd/opencode2api`。

静态可核对判据：`webui/styles.css` 中 `grep -c Georgia` 为 0；`grep -c "text-transform"` 为 0；含 `serif` 的行数为 1（即 `--font-sans` 定义那一行）；行数 ≤ 560（现 925）；`webui/index.html` 的 `<nav class="nav">` 内按钮数为 8；`grep -E 'id="page-(usage|guide|diagnostics|availability|playground|model-quotas)"' webui/index.html` 无输出。

新增 Go 测试（断言的字面量按 CC Switch 参数名手写，不与被测构造函数同源）：

- 三种 `app` 的 `endpoint`：`claude` 的结果不含 `/v1`；`codex` 与 `opencode` 的结果恰好以一个 `/v1` 结尾；`base` 以 `/` 结尾时结果不出现 `//`，且不产生 `/v1/v1`。
- 名称含中文与空格时，`name` 与 `apiKey` 的百分号编码可被 `url.QueryUnescape` 还原为原值。
- 非法 `app` / 非法 `base`（带 path、带 userinfo、`javascript:` scheme、空 host）/ 非法 `model`（空、超长、含空格）→ 400，且错误码字面量精确匹配。
- 未登录访问 → 401；缺 CSRF Token 的 POST 被拒；未知 `id` → 400 `key_not_found`。
- 成功响应的 `url` 以 `ccswitch://v1/import?resource=provider` 开头，且串内不含 `usageEnabled` 子串。
- 触发一次导入后，事件日志渲染结果中不出现该 Key 明文。

手工清单（需本机浏览器；CC Switch 已安装才能验证唤起这一项）：八个导航逐页打开且控制台无错误；问号展开与收起各一次；旧 `#usage` 与 `#model-quotas` 链接能落到新页；在配置中心的 Proxy 节点里新增一个不可达地址（`http://127.0.0.1:1`）并发一次请求，确认总览"需要你处理"块出现且不健康节点被点名，删除该代理并刷新后整块消失；新建一个 Key 后从列表行导入，浏览器出现"正在尝试打开 CC Switch"，CC Switch 确认弹窗中的名称、Base URL、模型三项与面板所选一致；未安装或协议未注册时只验证"复制导入链接"，并把唤起一项如实记为未验证，不写成通过。

## 11 文档同步

- `docs/management.zh-CN.md`：API Key 一节补导入流程与新接口，说明"列表仍只回尾码，明文按需在导入动作中读取"及其代价；验证段补新命令。
- `README.zh-CN.md` 与 `README.md`：「WebUI 与诊断」一节同步八个导航项名称；管理接口清单补 `POST /api/api-keys/{id}/ccswitch-import`；文件结构表保持三文件不变。
- `config.example.json`、`compose.yaml`、`Dockerfile`、`docker-entrypoint.sh`：无需改动。

## 12 实现顺序要点

拆成两份实施计划，按序执行，各自可独立验证：计划一为导入链路（第 7、8、9 节的后端深链构造器、接口、Go 测试与 API Key 页的面板），做完即可用；计划二为界面改版（第 4、5、6 节的样式基线、八页归并、文案收纳）加文档同步。

计划二内部顺序：先 `styles.css` 基线；再 `index.html` 的八页归并与文案收纳；再 `app.js` 的 `pages` 表、hash 归一表与渲染函数。每完成一段跑一次第 10 节闸门，不攒到最后一次跑。
