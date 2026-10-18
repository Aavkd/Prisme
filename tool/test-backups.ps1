$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    & docker build -f deploy/backups/Dockerfile.test -t prisme-backup-test:local .
    if ($LASTEXITCODE -ne 0) { throw 'Backup fixture build failed.' }
    # No host volumes, published ports, Docker socket or production credentials.
    & docker run --rm --network none prisme-backup-test:local
    if ($LASTEXITCODE -ne 0) { throw 'Backup and restore verification failed.' }
} finally {
    Pop-Location
}
