# Code generation for the shared proto contract (yjcli builder plugin; repository-owned).
#
# Read tools/builder/GUIDE.md ("Proto contract"), then implement one Invoke-Gen<Language>
# function per language below. Everything else - finding the languages under
# proto\dist\, argument checks - already works.
# This is the entry point `make.ps1 proto-build` calls; builds of platforms that need
# the contract (e.g. backend-msa) call it first. It does not have to stay a single
# file: add helper scripts next to it and call them from here.
#
# Usage: tools/builder/proto/builder.ps1 [language]
param(
    [string]$Language = ""
)
$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
$RootDir = (Resolve-Path (Join-Path $ScriptDir "..\..\..")).Path
$ProtoDir = Join-Path $RootDir "proto"

# ---------------------------------------------------------------- implement me
# Invoke-Gen<Language>
#   Generate code from $ProtoDir\*.proto into $Out (= proto\dist\<language>\, which
#   also holds the library manifest - do not delete it). Remove generated files of
#   .proto files that no longer exist. Check $LASTEXITCODE after each native command.
# Module path comes from $Out\go.mod. The .proto files sit in <pkg>\v1\ and import
# each other relative to proto\ (e.g. "common/v1/status.proto"). Stale *.pb.go are
# removed first so deleted .proto files leave nothing behind.
function Invoke-GenGo {
    $module = ((Get-Content (Join-Path $Out "go.mod")) | Where-Object { $_ -match '^module ' } | Select-Object -First 1) -replace '^module ', ''
    $distRoot = Join-Path $ProtoDir "dist"
    $protos = @(Get-ChildItem $ProtoDir -Recurse -Filter *.proto |
        Where-Object { -not $_.FullName.StartsWith($distRoot) } |
        ForEach-Object { $_.FullName.Substring($ProtoDir.Length + 1).Replace('\', '/') } | Sort-Object)
    Get-ChildItem $Out -Recurse -Filter *.pb.go | Remove-Item -Force
    $env:PATH = (Join-Path (go env GOPATH) "bin") + [IO.Path]::PathSeparator + $env:PATH
    Push-Location $ProtoDir
    try {
        protoc "-I." "--go_out=$Out" "--go_opt=module=$module" `
            "--go-grpc_out=$Out" "--go-grpc_opt=module=$module" @protos
        if ($LASTEXITCODE) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
    Push-Location $Out
    try {
        go mod tidy
        if ($LASTEXITCODE) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
}
# -----------------------------------------------------------------------------

$distDir = Join-Path $ProtoDir "dist"
if ($Language) {
    if (-not (Test-Path (Join-Path $distDir $Language) -PathType Container)) {
        Write-Host "unknown language: $Language (expected $distDir\$Language)"
        exit 1
    }
    $languages = @($Language)
} else {
    $languages = @(Get-ChildItem -Path $distDir -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name | ForEach-Object { $_.Name })
}
if ($languages.Count -eq 0) {
    Write-Host "no languages under $distDir"
    exit 1
}
# Schemas live in <pkg>\v1\ subfolders here, so look recursively (outside dist\).
if (-not (Get-ChildItem -Path $ProtoDir -Recurse -Filter *.proto -File -ErrorAction SilentlyContinue |
        Where-Object { -not $_.FullName.StartsWith((Join-Path $ProtoDir "dist")) })) {
    Write-Host "[proto] no .proto files in proto/ yet - nothing to generate"
    exit 0
}

foreach ($lang in $languages) {
    $Out = Join-Path $distDir $lang
    $fn = "Invoke-Gen" + $lang.Substring(0, 1).ToUpper() + $lang.Substring(1)
    if (-not (Get-Command $fn -CommandType Function -ErrorAction SilentlyContinue)) {
        Write-Host "[proto] $fn is not defined - add it to $PSCommandPath (see tools/builder/GUIDE.md)"
        exit 1
    }
    Write-Host "[proto] $lang -> proto/dist/$lang"
    $global:LASTEXITCODE = 0
    & $fn
    if ($LASTEXITCODE) {
        Write-Host "[proto] $lang failed with exit code $LASTEXITCODE"
        exit $LASTEXITCODE
    }
}
Write-Host "[proto] done: $($languages -join ' ')"
