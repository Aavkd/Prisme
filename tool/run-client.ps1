param([string]$Device = 'windows')
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$caPath = Join-Path $repoRoot '.dev/pki/ca.pem'
if (-not (Test-Path -LiteralPath $caPath)) { throw 'Run tool/dev.ps1 first.' }
# Only the PUBLIC CA is embedded; credentials are entered in the masked field.
$caBase64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes($caPath))
Push-Location (Join-Path $repoRoot 'apps/prisme_client')
try {
    & flutter run -d $Device --dart-define=PRISME_DEVELOPMENT=true "--dart-define=PRISME_CA_BASE64=$caBase64"
    if ($LASTEXITCODE -ne 0) { throw 'Flutter launch failed.' }
} finally { Pop-Location }
