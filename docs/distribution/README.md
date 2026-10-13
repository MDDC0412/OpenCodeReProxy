# 发布包 README 模板

本目录保存 Windows、Linux 和 macOS 运行包的中英文 README。源码根目录的 README 用于源码版。

发布流程按 `GOOS` 选择对应文件，将其复制到运行包根目录并重命名为 `README.md` / `README.zh-CN.md`。模板中的相对链接以运行包根目录为基准。

修改启动方式、配置要求或部署说明时，请同步更新六份平台模板和源码 README。

运行包根目录还应包含 `LICENSE`、`config.example.json`、`compose.yaml`、`.env.example` 和 `CHANGELOG.md`，并保留完整 `docs/` 目录，以保证文档链接、截图和 Compose 部署可用。
