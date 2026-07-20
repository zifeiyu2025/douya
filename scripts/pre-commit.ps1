# pre-commit.ps1
# Git pre-commit hook：提交前自动运行 lint，拦截低质量代码
#
# 用法（接入说明）：
#   1. 复制本文件为 .git/hooks/pre-commit（无扩展名）
#   2. 或在 .git/hooks/pre-commit 中调用本脚本
#   3. 首次需允许脚本执行：Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
#
# 接入命令（在项目根目录运行）：
#   pwsh -c "if (Test-Path .git\hooks\pre-commit) { Remove-Item .git\hooks\pre-commit }; New-Item -ItemType SymbolicLink -Path .git\hooks\pre-commit -Target scripts\pre-commit.ps1"
#
# 生活类比：像快递站点发货前的质检员——包裹（commit）必须通过基础检查才能发出，
# 避免把破损件（lint 错误）发给收件人（代码仓库）。
#
# 设计原则：
# - 快速反馈：只跑 lint，不跑测试（测试由 CI 完成）
# - 只检查暂存区文件：未 add 的修改不触发
# - 失败即中止：lint 不通过则 commit 被拒绝

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $ProjectRoot

$exitCode = 0

# 检查是否有 Go 文件变更
$stagedGoFiles = git diff --cached --name-only --diff-filter=ACM | Where-Object { $_ -match '\.go$' }
if ($stagedGoFiles) {
    Write-Host "[pre-commit] 检测到 Go 文件变更，运行 golangci-lint..." -ForegroundColor Cyan
    golangci-lint run ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[pre-commit] golangci-lint 失败，请修复后再提交" -ForegroundColor Red
        $exitCode = 1
    } else {
        Write-Host "[pre-commit] golangci-lint 通过" -ForegroundColor Green
    }
}

# 检查是否有前端文件变更
$stagedFrontendFiles = git diff --cached --name-only --diff-filter=ACM | Where-Object { $_ -match '\.(ts|tsx|vue|js|jsx)$' }
if ($stagedFrontendFiles) {
    Write-Host "[pre-commit] 检测到前端文件变更，运行 ESLint..." -ForegroundColor Cyan
    Push-Location frontend
    npm run lint
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[pre-commit] ESLint 失败，请修复后再提交（可尝试 npm run lint:fix）" -ForegroundColor Red
        $exitCode = 1
    } else {
        Write-Host "[pre-commit] ESLint 通过" -ForegroundColor Green
    }
    Pop-Location
}

if ($exitCode -eq 0) {
    Write-Host "[pre-commit] 所有检查通过" -ForegroundColor Green
} else {
    Write-Host "[pre-commit] 检查未通过，提交已中止。如需跳过，使用 git commit --no-verify" -ForegroundColor Yellow
}

exit $exitCode
