param([switch]$Integration)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    $unformatted = & gofmt -l cmd internal
    if ($LASTEXITCODE -ne 0 -or $unformatted) { throw 'Run gofmt -w cmd internal.' }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    & go build ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
    & dart run tool/generate_api.dart --check
    if ($LASTEXITCODE -ne 0) { throw 'Generated API is stale.' }
    & docker compose -f deploy/compose.dev.yml config --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Compose configuration is invalid.' }
    if ($Integration) { & ./tool/test-integration.ps1 }
} finally { Pop-Location }
