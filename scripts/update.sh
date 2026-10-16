#!/bin/sh
# OpenCodeReProxy 一键更新（Linux / macOS）。
#
# 作用：下载指定版本（默认最新 Release）的运行包，校验 SHA256，替换已安装的
# opencode2api 程序，然后重启 systemd 服务；或按 --compose 方式更新 Docker
# Compose 部署。已有的 config.json、模型缓存、日志和服务配置都不会被改动。
#
# 用法：
#   sudo ./update.sh                                   # 自动探测服务与安装目录
#   sudo ./update.sh -v v1.4.0-secdev-diagnostics      # 指定版本
#   sudo ./update.sh -d /opt/opencodereproxy -s opencodereproxy
#   sudo ./update.sh --no-restart                      # 只换程序，稍后自行重启
#   sudo ./update.sh --compose -d /srv/opencodereproxy # 更新 Compose 部署
#   ./update.sh -n                                     # 只检查，不下载不替换
#
# 不落盘使用（直接从仓库取脚本执行）：
#   curl -fsSL https://raw.githubusercontent.com/MDDC0412/OpenCodeReProxy/main/scripts/update.sh | sudo sh -s -- -d /opt/opencodereproxy

set -eu

REPOSITORY=${OPENCODE2API_REPOSITORY:-MDDC0412/OpenCodeReProxy}
VERSION=
DIRECTORY=
SERVICE=
RESTART=1
CHECK_ONLY=0
COMPOSE=0

usage() {
    cat <<'EOF'
用法: update.sh [选项]

  -v, --version <tag>   目标版本，例如 v1.4.0-secdev-diagnostics（默认最新 Release）
  -d, --dir <path>      安装目录：二进制部署为程序所在目录，Compose 部署为 compose 文件所在目录
  -s, --service <unit>  systemd 服务名（默认自动探测 opencodereproxy / opencode2api）
      --compose         更新 Docker Compose 部署：改写 .env 版本并 pull / up -d
      --no-restart      仅二进制部署有效：替换程序后不重启服务
  -n, --check           只查询目标版本，不下载也不替换
  -h, --help            显示本帮助

环境变量:
  OPENCODE2API_REPOSITORY   覆盖仓库，默认 MDDC0412/OpenCodeReProxy
  OPENCODE2API_DIR          默认安装目录
  OPENCODE2API_SERVICE      默认 systemd 服务名
EOF
}

log() { printf '%s\n' "$*"; }
fail() { printf '错误: %s\n' "$*" >&2; exit 1; }

while [ "$#" -gt 0 ]; do
    case "$1" in
        -v|--version) [ "$#" -ge 2 ] || fail "$1 需要一个参数"; VERSION=$2; shift 2 ;;
        -d|--dir) [ "$#" -ge 2 ] || fail "$1 需要一个参数"; DIRECTORY=$2; shift 2 ;;
        -s|--service) [ "$#" -ge 2 ] || fail "$1 需要一个参数"; SERVICE=$2; shift 2 ;;
        --compose) COMPOSE=1; shift ;;
        --no-restart) RESTART=0; shift ;;
        -n|--check) CHECK_ONLY=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) usage; fail "未知参数: $1" ;;
    esac
done

case "$VERSION" in
    '') ;;
    v*) ;;
    *) fail "版本号应以 v 开头: $VERSION" ;;
esac
os=$(uname -s)
case "$os" in
    Linux) goos=linux ;;
    Darwin) goos=darwin ;;
    *) fail "不支持的系统: $os（Windows 请使用 scripts/update.ps1）" ;;
esac

machine=$(uname -m)
case "$machine" in
    x86_64|amd64) goarch=amd64 ;;
    aarch64|arm64) goarch=arm64 ;;
    *) fail "不支持的架构: $machine" ;;
esac

# 自动探测 systemd 服务，用于推导安装目录和重启。
have_systemctl=0
command -v systemctl >/dev/null 2>&1 && have_systemctl=1

if [ "$have_systemctl" -eq 1 ] && [ -z "$SERVICE" ]; then
    SERVICE=${OPENCODE2API_SERVICE:-}
    if [ -z "$SERVICE" ]; then
        for candidate in opencodereproxy opencode2api; do
            if systemctl cat "$candidate.service" >/dev/null 2>&1; then
                SERVICE=$candidate
                break
            fi
        done
    fi
fi

# 从服务单元读取可执行文件路径，推导默认安装目录。
unit_binary() {
    [ -n "$SERVICE" ] || return 1
    [ "$have_systemctl" -eq 1 ] || return 1
    systemctl cat "$SERVICE" 2>/dev/null \
        | awk -F'=' '/^ExecStart=/{print $2; exit}' \
        | awk '{print $1}'
}

if [ "$COMPOSE" -eq 1 ]; then
    [ -n "$DIRECTORY" ] || DIRECTORY=${OPENCODE2API_DIR:-$(pwd)}
    [ -d "$DIRECTORY" ] || fail "目录不存在: $DIRECTORY"
else
    if [ -z "$DIRECTORY" ]; then
        if [ -n "${OPENCODE2API_DIR:-}" ]; then
            DIRECTORY=$OPENCODE2API_DIR
        else
            bin=$(unit_binary || true)
            if [ -n "$bin" ]; then
                DIRECTORY=$(dirname "$bin")
            elif [ -d /opt/opencodereproxy ]; then
                DIRECTORY=/opt/opencodereproxy
            else
                fail '未能确定安装目录，请用 -d /path/to/install 指定'
            fi
        fi
    fi
    [ -d "$DIRECTORY" ] || fail "安装目录不存在: $DIRECTORY"
fi


if [ -z "$VERSION" ]; then
    command -v curl >/dev/null 2>&1 || fail '查询最新版本需要 curl，请用 -v 指定版本或安装 curl'
    log '正在查询最新 Release…'
    VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "https://github.com/$REPOSITORY/releases/latest" | sed 's#.*/tag/##')
    case "$VERSION" in
        v*) ;;
        *) fail '无法确定最新版本，请用 -v 指定，例如 -v v1.4.0-secdev-diagnostics' ;;
    esac
fi

base="https://github.com/$REPOSITORY/releases/download/$VERSION"
log "目标版本: $VERSION"

if [ "$CHECK_ONLY" -eq 1 ]; then
    if [ "$COMPOSE" -eq 1 ]; then
        log "部署方式: Docker Compose（$DIRECTORY）"
    else
        log "部署方式: 二进制（$DIRECTORY）"
        log "目标平台: $goos/$goarch"
        log "下载地址: $base/opencode2api_${VERSION}_${goos}_${goarch}.tar.gz"
    fi
    log '仅检查模式，未做任何改动。'
    exit 0
fi

if [ "$COMPOSE" -eq 1 ]; then
    command -v docker >/dev/null 2>&1 || fail '需要 docker 命令'

    compose_file=
    for candidate in compose.yaml docker-compose.yaml compose.yml docker-compose.yml; do
        if [ -f "$DIRECTORY/$candidate" ]; then
            compose_file=$candidate
            break
        fi
    done
    [ -n "$compose_file" ] || fail "未在 $DIRECTORY 找到 compose.yaml / docker-compose.yaml"

    if docker compose version >/dev/null 2>&1; then
        compose_cmd='docker compose'
    elif command -v docker-compose >/dev/null 2>&1; then
        compose_cmd='docker-compose'
    else
        fail '未检测到 docker compose 插件或 docker-compose 命令'
    fi

    env_file="$DIRECTORY/.env"
    if [ -f "$env_file" ] && grep -q '^OPENCODE2API_VERSION=' "$env_file"; then
        tmp=$(mktemp)
        sed "s|^OPENCODE2API_VERSION=.*|OPENCODE2API_VERSION=$VERSION|" "$env_file" > "$tmp"
        cp -p "$env_file" "$env_file.bak"
        cat "$tmp" > "$env_file"
        rm -f "$tmp"
        log "已更新 $env_file 中的 OPENCODE2API_VERSION（原文件备份为 .env.bak）"
    else
        printf 'OPENCODE2API_VERSION=%s\n' "$VERSION" >> "$env_file"
        log "已写入 $env_file: OPENCODE2API_VERSION=$VERSION"
    fi

    log "正在拉取镜像并重启容器（$compose_file）…"
    ( cd "$DIRECTORY" && $compose_cmd pull && $compose_cmd up -d ) \
        || fail 'docker compose 执行失败，请检查上面的输出'
    log '更新完成。配置文件位于数据卷，未被改动。'
    exit 0
fi

command -v curl >/dev/null 2>&1 || fail '需要 curl，请先安装'

asset="opencode2api_${VERSION}_${goos}_${goarch}.tar.gz"
log "目标平台: $goos/$goarch"
log "下载地址: $base/$asset"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM

log '正在下载…'
curl -fsSL -o "$work/$asset" "$base/$asset" || fail '下载失败'
curl -fsSL -o "$work/$asset.sha256" "$base/$asset.sha256" || fail '校验文件下载失败'

expected=$(awk '{print $1}' "$work/$asset.sha256")
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$work/$asset" | awk '{print $1}')
else
    actual=$(shasum -a 256 "$work/$asset" | awk '{print $1}')
fi
[ -n "$expected" ] || fail '校验文件为空'
[ "$expected" = "$actual" ] || fail "SHA256 校验失败（期望 $expected，实际 $actual）"
log 'SHA256 校验通过'

tar -xzf "$work/$asset" -C "$work" || fail '解压失败'
newbin=$(find "$work" -type f -name opencode2api -print | head -n 1)
[ -n "$newbin" ] || fail '压缩包内未找到 opencode2api'

target="$DIRECTORY/opencode2api"
if [ -f "$target" ]; then
    backup="$target.bak-$(date +%Y%m%d%H%M%S)"
    cp -p "$target" "$backup" 2>/dev/null && log "已备份原程序: $backup" \
        || log "提示: 无法写入 $DIRECTORY，请用 sudo 运行。"
fi

cp -f "$newbin" "$DIRECTORY/.opencode2api.new" || fail "写入失败，请用 sudo 运行或确认 $DIRECTORY 可写"
chmod 755 "$DIRECTORY/.opencode2api.new"
mv -f "$DIRECTORY/.opencode2api.new" "$target" || fail '替换程序失败'
log "已替换程序: $target"

if "$target" -h >/dev/null 2>&1; then
    log '新程序自检通过'
else
    log '警告: 新程序 -h 自检未通过，请手动确认。'
fi

if [ "$RESTART" -eq 0 ]; then
    log '已按 --no-restart 跳过重启，请稍后自行重启服务。'
    exit 0
fi

if [ "$have_systemctl" -eq 0 ]; then
    log '未检测到 systemctl；请手动重启服务（config.json 未被改动）。'
    exit 0
fi

if [ -z "$SERVICE" ]; then
    log '未找到可识别的 systemd 服务；请手动重启（config.json 未被改动）。'
    exit 0
fi

log "正在重启服务: $SERVICE"
if systemctl restart "$SERVICE"; then
    log "服务已重启: $SERVICE"
else
    fail "重启失败，请用 systemctl status $SERVICE 检查；原程序备份在同目录的 opencode2api.bak-*"
fi

log '更新完成。config.json 与日志均未被改动。'