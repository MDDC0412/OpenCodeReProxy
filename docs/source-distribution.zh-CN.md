# 干净源码交付与二开

`OpenCodeReProxySouce` 是本二开版的完整源码目录，保留此交付名称的拼写。目录中不包含预编译程序、Git 历史、本机运行配置、账号数据、日志、模型缓存、测试报告或依赖缓存。Windows 和 Linux 发布包独立交付。

## 文件与配置

- `cmd/`、`internal/`、`webui/`：Go 服务与内嵌管理页面。
- `go.mod`、`go.sum`：Go 模块和依赖校验。
- `config.example.json`：只有占位值的标准配置示例；不提供可直接使用的运行账号或上游 Key。
- `*_test.go`、`tests/`：供二开使用的自动化测试源码。浏览器测试会在运行时生成隔离的演示配置，演示凭据只用于本地模拟上游。
- `package.json`、`package-lock.json`：前端检查和浏览器测试的开发依赖，不是服务运行依赖。
- `README.md`、`README.zh-CN.md`、`docs/`、`LICENSE`：使用说明、开发说明及 GPL-3.0-or-later 许可证。
- `.github/`、Docker 文件和格式配置：通用开发与发布工具配置。

使用时自行从 `config.example.json` 创建 `config.json`，替换本地 API Key、管理密码并配置上游通道。源码默认没有 `config.json`。免费匿名通道可设置 `anonymous: true` 并清空 `zen_keys`、`go_keys`；模型是否可用由上游实际返回决定。

## 本地开发

需要 Go 1.24 或更新版本。以下命令在源码根目录执行；构建产物仅由开发者在需要运行时生成。

```bash
go test ./...
go vet ./...
go build -trimpath -o opencode2api ./cmd/opencode2api
```

Windows 构建时将输出名称改为 `opencode2api.exe`。发布构建可通过 `-ldflags "-X main.version=自己的版本号"` 注入版本。

管理页面使用原生 HTML、CSS 和 JavaScript，通过 Go embed 嵌入程序。修改 `webui/` 后须重新构建并重启才能看到更新。只有前端格式检查和浏览器回归需要 Node.js 22 或更新版本：

```bash
npm ci --ignore-scripts
npm run check:web
npm run format:check
npm run test:web
```

浏览器测试的前提及覆盖范围见[WebUI 浏览器回归](webui-regression.zh-CN.md)。测试生成的 `.local/`、`output/` 和依赖目录由 `.gitignore` 排除。

## 发布归属

本项目基于上游 OpenCode2API v1.4.0。二开发布请保留上游归属与 GPL-3.0-or-later 许可证，并使用自己的仓库和发布地址。本项目的 Docker 镜像为 `ghcr.io/mddc0412/opencodereproxy`，Compose 默认使用 `v1.4.0-secdev-diagnostics` 标签；上游预编译文件不包含本二开版改动。
