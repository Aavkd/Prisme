$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$previousURL = $env:PRISME_TEST_DATABASE_URL
Push-Location $repoRoot
try {
    if (-not (Test-Path -LiteralPath '.dev/db-password')) { throw 'Run go run ./cmd/dev-init first.' }
    & docker compose -f deploy/compose.test.yml up --wait
    if ($LASTEXITCODE -ne 0) { throw 'Test database failed to start.' }
    $password = [IO.File]::ReadAllText((Join-Path $repoRoot '.dev/db-password')).Trim()
    $env:PRISME_TEST_DATABASE_URL = 'postgres://prisme:' + [Uri]::EscapeDataString($password) + '@127.0.0.1:55432/prisme_test?sslmode=disable'
    & go test -count=1 -v ./internal/api
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL/TLS integration tests failed.' }
} finally {
    $env:PRISME_TEST_DATABASE_URL = $previousURL
    & docker compose -f deploy/compose.test.yml down
    Pop-Location
}
