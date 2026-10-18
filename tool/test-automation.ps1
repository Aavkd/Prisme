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
    & docker compose -f deploy/compose.test.yml up --wait --pull never
    if ($LASTEXITCODE -ne 0) { throw 'Test database unavailable.' }
    $automationPassword = [IO.File]::ReadAllText((Join-Path $repoRoot '.dev/db-password')).Trim()
    $env:PRISME_TEST_DATABASE_URL = 'postgres://prisme:' + [Uri]::EscapeDataString($automationPassword) + '@127.0.0.1:55432/prisme_test?sslmode=disable'
    & go test -count=1 ./internal/automation
    if ($LASTEXITCODE -ne 0) { throw 'Automation model tests failed.' }
    & go test -count=1 ./internal/api -run 'TestPhaseNine'
    if ($LASTEXITCODE -ne 0) { throw 'Automation/PostgreSQL tests failed.' }
    if ($Native) {
        $env:GOOS = 'linux'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        & go test -c -o .dev/automation-tests ./internal/api
        if ($LASTEXITCODE -ne 0) { throw 'Linux test compilation failed.' }
        & docker build -f deploy/deployment/Dockerfile.test -t prisme-deployment-test:local .
        if ($LASTEXITCODE -ne 0) { throw 'Compose fixture image failed.' }
        & docker run --rm --network host -e PRISME_TEST_DATABASE_URL -e PRISME_TEST_COMPOSE=1 --mount "type=bind,source=$repoRoot,target=/workspace,readonly" -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker:/var/lib/docker:ro -w /workspace/internal/api prisme-deployment-test:local /workspace/.dev/automation-tests '-test.v' '-test.run=TestPhaseNineLiveComposeRemediation' '-test.timeout=5m'
        if ($LASTEXITCODE -ne 0) { throw 'Real Compose remediation failed.' }
    }
} finally {
    $env:PRISME_TEST_DATABASE_URL = $previousURL
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
