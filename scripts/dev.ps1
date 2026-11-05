param(
    [ValidateSet('run', 'test', 'build')]
    [string]$Mode = 'run',
    [int]$Port = 8082,
    [string]$Compiler = ''
)
$ErrorActionPreference = 'Stop'
$repoDir = Split-Path $PSScriptRoot -Parent
if ($Compiler) { $env:CC = $Compiler }
if (-not $env:CC) {
    $gcc = Get-Command gcc.exe -ErrorAction SilentlyContinue
    $localCompiler = Join-Path $repoDir '..\..\.tools\gcc-2.10.0\w64devkit\bin\gcc.exe'
    if ($gcc) { $env:CC = $gcc.Source }
    elseif (Test-Path -LiteralPath $localCompiler) { $env:CC = (Resolve-Path -LiteralPath $localCompiler).Path }
    else { throw 'Install GCC, set CC, or pass -Compiler with the gcc.exe path.' }
}
$env:CGO_ENABLED = '1'
$env:GOCACHE = Join-Path $env:TEMP 'linkdesk-go-cache'
Push-Location $repoDir
try {
    if ($Mode -eq 'test') {
        go test -race -count=1 ./...
        if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
        go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'Vet failed' }
    } elseif ($Mode -eq 'build') {
        New-Item -ItemType Directory -Force -Path 'dist' | Out-Null
        go build -trimpath -o dist/linkdesk.exe ./cmd
        if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
    } else {
        go run ./cmd --host 127.0.0.1 --port $Port --base-url "http://localhost:$Port"
        if ($LASTEXITCODE -ne 0) { throw 'Server exited with an error' }
    }
} finally { Pop-Location }
