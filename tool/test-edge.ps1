param([switch]$Native)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$previousURL = $env:PRISME_TEST_DATABASE_URL
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
Push-Location $repoRoot
try {
    if (-not (Test-Path -LiteralPath '.dev/db-password')) { throw 'Run go run ./cmd/dev-init first.' }
    & docker compose -f deploy/compose.test.yml up --wait
    if ($LASTEXITCODE -ne 0) { throw 'Test database unavailable.' }
    $edgePassword = [IO.File]::ReadAllText((Join-Path $repoRoot '.dev/db-password')).Trim()
    $env:PRISME_TEST_DATABASE_URL = 'postgres://prisme:' + [Uri]::EscapeDataString($edgePassword) + '@127.0.0.1:55432/prisme_test?sslmode=disable'
    & go test -count=1 ./internal/edge ./internal/api
    if ($LASTEXITCODE -ne 0) { throw 'Edge/PostgreSQL tests failed.' }
    if ($Native) {
        $env:GOOS = 'linux'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        & go test -c -o .dev/edge-tests ./internal/api
        if ($LASTEXITCODE -ne 0) { throw 'Linux test compilation failed.' }
        & docker build -f deploy/edge/Dockerfile.test -t prisme-edge-test:local .
        if ($LASTEXITCODE -ne 0) { throw 'Caddy test image failed.' }
        $env:PRISME_TEST_DATABASE_URL = $env:PRISME_TEST_DATABASE_URL.Replace('@127.0.0.1:', '@host.docker.internal:')
        & docker run --rm -e PRISME_TEST_DATABASE_URL -e PRISME_TEST_CADDY_BINARY=/usr/bin/caddy --mount "type=bind,source=$repoRoot,target=/workspace,readonly" -w /workspace/internal/api prisme-edge-test:local '-test.v' '-test.run=TestPhaseEightLiveCaddySharedDomains' '-test.timeout=3m'
        if ($LASTEXITCODE -ne 0) { throw 'Real Caddy route/rollback tests failed.' }
    }
} finally {
    $env:PRISME_TEST_DATABASE_URL = $previousURL
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
