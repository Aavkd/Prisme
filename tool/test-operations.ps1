$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    & go test ./internal/operations ./internal/coordination
    if ($LASTEXITCODE -ne 0) { throw 'Operations unit tests failed.' }
    & docker build -f deploy/operations/Dockerfile.test -t prisme-operations-test:local .
    if ($LASTEXITCODE -ne 0) { throw 'Isolated SSH fixture build failed.' }
    & docker run --rm --name prisme-phase-six-test prisme-operations-test:local
    if ($LASTEXITCODE -ne 0) { throw 'SSH/PTY/SFTP qualification failed.' }
} finally { Pop-Location }
