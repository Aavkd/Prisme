param([switch]$Stop)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $repoRoot
try {
    if ($Stop) {
        & docker compose -f deploy/compose.dev.yml --profile agent down
        if ($LASTEXITCODE -ne 0) { throw 'Development stack stop failed.' }
        return
    }
    if (-not (Test-Path -LiteralPath '.dev')) {
        & go run ./cmd/dev-init
        if ($LASTEXITCODE -ne 0) { throw 'Development initialization failed.' }
    }
    if ($env:OS -eq 'Windows_NT') {
        $principal = [Security.Principal.WindowsIdentity]::GetCurrent().Name
        & icacls (Join-Path $repoRoot '.dev') /inheritance:r /grant:r "${principal}:(OI)(CI)F" 'SYSTEM:(OI)(CI)F' /Q
        if ($LASTEXITCODE -ne 0) { throw 'Could not restrict development secret permissions.' }
    }
    & docker compose -f deploy/compose.dev.yml up --build --wait api
    if ($LASTEXITCODE -ne 0) { throw 'Development stack failed.' }
    Write-Host 'Development API ready at https://localhost:8443. Run tool/run-client.ps1.'
} finally { Pop-Location }
