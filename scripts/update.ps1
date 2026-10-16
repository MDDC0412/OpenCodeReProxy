<#
.SYNOPSIS
    OpenCodeReProxy 一键更新（Windows）。

.DESCRIPTION
    默认按二进制方式更新：下载指定版本（默认最新 Release）的 Windows 运行包，
    校验 SHA256，替换已安装的 opencode2api.exe，然后重启服务。已有的
    config.json、模型缓存和日志都不会被改动。

    使用 -Compose 时改为更新 Docker Compose 部署：改写 .env 中的
    OPENCODE2API_VERSION 并执行 docker compose pull / up -d。

    注意：Windows 不允许覆盖正在运行的 exe，脚本会先停止占用目标文件的服务或
    进程，替换后再启动，因此会有几秒中断。

.EXAMPLE
    .\update.ps1
    更新到最新 Release；脚本与 opencode2api.exe 同目录时使用该目录，
    否则使用脚本所在目录的上一级（仓库 scripts/ 布局）。

.EXAMPLE
    .\update.ps1 -Version v1.4.0-secdev-diagnostics -Directory 'C:\opencodereproxy' -ServiceName opencodereproxy

.EXAMPLE
    .\update.ps1 -Compose -Directory 'C:\opencodereproxy'

.EXAMPLE
    .\update.ps1 -Check
    只查询目标版本，不下载、不替换。
#>
[CmdletBinding()]
param(
    [string]$Version = '',
    [string]$Directory = '',
    [string]$ServiceName = '',
    [string]$Repository = $(if ($env:OPENCODE2API_REPOSITORY) { $env:OPENCODE2API_REPOSITORY } else { 'MDDC0412/OpenCodeReProxy' }),
    [switch]$Compose,
    [switch]$NoRestart,
    [switch]$Check
)

$ErrorActionPreference = 'Stop'

function Write-Log([string]$Message) { Write-Host $Message }
function Fail([string]$Message) { Write-Error $Message; exit 1 }

if (-not $Version) {
    Write-Log '正在查询最新 Release…'
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$Repository/releases/latest" -MaximumRedirection 0 -ErrorAction SilentlyContinue
        $location = $response.Headers.Location
    } catch {
        $location = $null
        if ($_.Exception.Response -and $_.Exception.Response.Headers.Location) {
            $location = $_.Exception.Response.Headers.Location.ToString()
        }
    }
    if (-not $location) { Fail '无法确定最新版本，请用 -Version 指定，例如 -Version v1.4.0-secdev-diagnostics' }
    if ("$location" -match '/tag/(.+)$') { $Version = $Matches[1].Trim() } else { Fail "无法从重定向解析版本: $location" }
}

if ($Version -notmatch '^v') { Fail "版本号应以 v 开头: $Version" }
Write-Log "目标版本: $Version"

if (-not $Directory) {
    if ($env:OPENCODE2API_DIR) {
        $Directory = $env:OPENCODE2API_DIR
    } elseif (Test-Path -LiteralPath (Join-Path $PSScriptRoot 'opencode2api.exe') -PathType Leaf) {
        # 脚本与 opencode2api.exe 放在同一目录时直接使用该目录。
        $Directory = $PSScriptRoot
    } else {
        # 仓库布局（scripts/update.ps1）时安装目录是脚本目录的上一级。
        $Directory = (Split-Path -Parent $PSScriptRoot)
    }
}
if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { Fail "目录不存在: $Directory" }

if ($Check) {
    if ($Compose) { Write-Log "部署方式: Docker Compose（$Directory）" }
    else {
        $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
        Write-Log "部署方式: 二进制（$Directory）"
        Write-Log "下载地址: https://github.com/$Repository/releases/download/$Version/opencode2api_$($Version)_windows_$($arch).zip"
    }
    Write-Log '仅检查模式，未做任何改动。'
    exit 0
}

if ($Compose) {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Fail '需要 docker 命令' }
    $composeFile = Get-ChildItem -LiteralPath $Directory -File |
        Where-Object { $_.Name -in @('compose.yaml', 'docker-compose.yaml', 'compose.yml', 'docker-compose.yml') } |
        Select-Object -First 1
    if (-not $composeFile) { Fail "未在 $Directory 找到 compose.yaml / docker-compose.yaml" }

    $envFile = Join-Path $Directory '.env'
    $line = "OPENCODE2API_VERSION=$Version"
    if (Test-Path -LiteralPath $envFile) {
        $content = Get-Content -LiteralPath $envFile -Encoding UTF8
        Copy-Item -LiteralPath $envFile -Destination "$envFile.bak" -Force
        if ($content -match '^OPENCODE2API_VERSION=') {
            $content = $content -replace '^OPENCODE2API_VERSION=.*$', $line
        } else {
            $content = @($content) + $line
        }
        [IO.File]::WriteAllLines($envFile, [string[]]$content, (New-Object System.Text.UTF8Encoding($false)))
        Write-Log "已更新 $envFile 中的 OPENCODE2API_VERSION（原文件备份为 .env.bak）"
    } else {
        [IO.File]::WriteAllLines($envFile, [string[]]@($line), (New-Object System.Text.UTF8Encoding($false)))
        Write-Log "已写入 ${envFile}: $line"
    }

    Push-Location $Directory
    try {
        Write-Log "正在拉取镜像并重启容器（$($composeFile.Name)）…"
        docker compose pull
        if ($LASTEXITCODE -ne 0) { Fail 'docker compose pull 失败' }
        docker compose up -d
        if ($LASTEXITCODE -ne 0) { Fail 'docker compose up -d 失败' }
    } finally {
        Pop-Location
    }
    Write-Log '更新完成。配置文件位于数据卷，未被改动。'
    exit 0
}

$architecture = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "opencode2api_${Version}_windows_${architecture}.zip"
$base = "https://github.com/$Repository/releases/download/$Version"
$target = Join-Path $Directory 'opencode2api.exe'
Write-Log "目标平台: windows/$architecture"
Write-Log "下载地址: $base/$asset"

$work = Join-Path ([IO.Path]::GetTempPath()) ("opencode2api-update-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null

try {
    Write-Log '正在下载…'
    $zipPath = Join-Path $work $asset
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $zipPath
    $sumPath = Join-Path $work "$asset.sha256"
    Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset.sha256" -OutFile $sumPath

    $expected = ((Get-Content -LiteralPath $sumPath -Raw).Trim() -split '\s+')[0].ToLowerInvariant()
    if (-not $expected) { Fail '校验文件为空' }
    $actual = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expected -ne $actual) { Fail "SHA256 校验失败（期望 $expected，实际 $actual）" }
    Write-Log 'SHA256 校验通过'

    $expanded = Join-Path $work 'expanded'
    Expand-Archive -LiteralPath $zipPath -DestinationPath $expanded -Force
    $newExe = Get-ChildItem -LiteralPath $expanded -Recurse -Filter 'opencode2api.exe' -File | Select-Object -First 1
    if (-not $newExe) { Fail '压缩包内未找到 opencode2api.exe' }

    # 先停止占用目标文件的 systemd 式服务（Windows 服务），再处理同目录进程。
    $stoppedService = $false
    if ($ServiceName) {
        $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        if ($svc) {
            if ($svc.Status -ne 'Stopped') {
                Write-Log "正在停止服务: $ServiceName"
                Stop-Service -Name $ServiceName -Force
                $svc.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
            }
            $stoppedService = $true
        } else {
            Write-Log "提示: 未找到服务 $ServiceName。"
        }
    }

    # 只结束可执行文件路径与目标目录一致的进程，避免误杀其它实例。
    $running = @(Get-Process -Name 'opencode2api' -ErrorAction SilentlyContinue | Where-Object {
        try { $_.Path -and ([IO.Path]::GetFullPath($_.Path) -eq [IO.Path]::GetFullPath($target)) } catch { $false }
    })
    if ($running.Count -gt 0) {
        Write-Log "正在停止进程: opencode2api (PID $($running.Id -join ', '))"
        $running | Stop-Process -Force
        Start-Sleep -Seconds 2
    }

    if (Test-Path -LiteralPath $target) {
        $backup = "$target.bak-" + (Get-Date -Format 'yyyyMMddHHmmss')
        Copy-Item -LiteralPath $target -Destination $backup -Force
        Write-Log "已备份原程序: $backup"
    }

    Copy-Item -LiteralPath $newExe.FullName -Destination $target -Force
    Write-Log "已替换程序: $target"

    if ($NoRestart) {
        Write-Log '已按 -NoRestart 跳过重启，请稍后自行启动。'
        exit 0
    }

    if ($stoppedService) {
        Write-Log "正在启动服务: $ServiceName"
        Start-Service -Name $ServiceName
        Write-Log "服务已启动: $ServiceName"
    } else {
        Write-Log '未指定 -ServiceName，未自动启动；请手动启动，例如:'
        Write-Log "  & '$target' -config '$(Join-Path $Directory 'config.json')'"
    }

    Write-Log '更新完成。config.json 与日志均未被改动。'
} finally {
    if (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue }
}