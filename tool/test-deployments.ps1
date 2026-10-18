$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
Push-Location $repoRoot
try {
    if (-not (Test-Path -LiteralPath '.dev/db-password')) { throw 'Run go run ./cmd/dev-init first.' }
    & docker compose -f deploy/compose.test.yml up --wait
    if ($LASTEXITCODE -ne 0) { throw 'Test database unavailable' }
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    & go test -c -o .dev/deployment-tests ./internal/api
    if ($LASTEXITCODE -ne 0) { throw 'Integration binary compilation failed' }
    & docker build -f deploy/deployment/Dockerfile.test -t prisme-deployment-test:local .
    if ($LASTEXITCODE -ne 0) { throw 'Fixture image build failed' }
    $password = [IO.File]::ReadAllText((Join-Path $repoRoot '.dev/db-password')).Trim()
    $databaseURL = 'PRISME_TEST_DATABASE_URL=postgres://prisme:' + [Uri]::EscapeDataString($password) + '@127.0.0.1:55432/prisme_test?sslmode=disable'
    [IO.File]::WriteAllText((Join-Path $repoRoot '.dev/deployment-test.env'), $databaseURL)
    & docker run --rm --name prisme-deployment-test --network host --env-file .dev/deployment-test.env -e PRISME_TEST_COMPOSE=1 -v "${repoRoot}:/workspace:ro" -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker:/var/lib/docker:ro -w /workspace/internal/api prisme-deployment-test:local /workspace/.dev/deployment-tests '-test.v' '-test.run=TestPhaseFourLiveCompose' '-test.timeout=5m'
    if ($LASTEXITCODE -ne 0) { throw 'Real Compose/Caddy deployment test failed' }
} finally {
    $env:GOOS = $previousGoOS
    $env:GOARCH = $previousGoArch
    $env:CGO_ENABLED = $previousCGO
    & docker compose -f deploy/compose.test.yml down
    Pop-Location
}
