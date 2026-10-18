param([switch]$Full)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    & dart run tool/generate_api.dart --check
    if ($LASTEXITCODE -ne 0) { throw 'Generated API is stale.' }
    & docker build -f deploy/qualification/Dockerfile.test -t prisme-qualification-test:local .
    if ($LASTEXITCODE -ne 0) { throw 'Qualification image failed.' }
    # No Docker socket, host paths or external network are available to the drill.
    & docker run --rm --network none --memory 768m --pids-limit 256 prisme-qualification-test:local
    if ($LASTEXITCODE -ne 0) { throw 'Local qualification failed.' }
    if ($Full) { & ./tool/verify-foundation.ps1 -Integration }
} finally { Pop-Location }
