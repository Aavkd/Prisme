param(
    [Parameter(Mandatory=$true)][string]$Device,
    [Parameter(Mandatory=$true)][string]$FCMCredentialsFile
)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$credentialPath = (Resolve-Path -LiteralPath $FCMCredentialsFile).Path
$oldDatabase = $env:PRISME_TEST_DATABASE_URL
$oldDevice = $env:PRISME_TEST_ANDROID_DEVICE
$oldFirebase = $env:PRISME_TEST_FCM_FILE
$backend = $null
Push-Location $repoRoot
try {
    & docker compose -f deploy/compose.test.yml up --wait
    if ($LASTEXITCODE -ne 0) { throw 'Test database unavailable.' }
    $password = [IO.File]::ReadAllText((Join-Path $repoRoot '.dev/db-password')).Trim()
    $env:PRISME_TEST_DATABASE_URL = 'postgres://prisme:' + [Uri]::EscapeDataString($password) + '@127.0.0.1:55432/prisme_test?sslmode=disable'
    $env:PRISME_TEST_ANDROID_DEVICE = $Device
    $env:PRISME_TEST_FCM_FILE = $credentialPath
    # Compile first so stopping a failed run only targets the owned test process.
    & go test -c -o .dev/mobile-tests.exe ./internal/api
    if ($LASTEXITCODE -ne 0) { throw 'Native backend test compilation failed.' }
    $backend = Start-Process -FilePath (Join-Path $repoRoot '.dev/mobile-tests.exe') -WorkingDirectory (Join-Path $repoRoot 'internal/api') -ArgumentList '-test.v','-test.run=^TestPhaseFiveLiveAndroid$','-test.timeout=12m' -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $repoRoot '.dev/mobile-backend.log') -RedirectStandardError (Join-Path $repoRoot '.dev/mobile-backend-errors.log')
    Push-Location (Join-Path $repoRoot 'apps/prisme_client')
    try {
        & flutter test integration_test/mobile_backend_test.dart -d $Device
        if ($LASTEXITCODE -ne 0) { throw 'Android/FCM test failed.' }
    } finally { Pop-Location }
    $backend.WaitForExit(30000) | Out-Null
    if (-not $backend.HasExited -or $backend.ExitCode -ne 0) { throw 'Backend did not verify native approval; inspect .dev/mobile-backend.log.' }
    Get-Content -LiteralPath (Join-Path $repoRoot '.dev/mobile-backend.log')
} finally {
    if ($backend -and -not $backend.HasExited) { Stop-Process -Id $backend.Id }
    & adb -s $Device reverse --remove tcp:18443 2>$null
    & adb -s $Device shell run-as dev.mantara.prisme_client rm -f /data/data/dev.mantara.prisme_client/files/phase5/input.json /data/data/dev.mantara.prisme_client/files/phase5/ready /data/data/dev.mantara.prisme_client/files/phase5/complete 2>$null
    $env:PRISME_TEST_DATABASE_URL = $oldDatabase
    $env:PRISME_TEST_ANDROID_DEVICE = $oldDevice
    $env:PRISME_TEST_FCM_FILE = $oldFirebase
    & docker compose -f deploy/compose.test.yml down
    Pop-Location
}
