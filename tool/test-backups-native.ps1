param(
    [Parameter(Mandatory=$true)][string]$PostgresBin,
    [Parameter(Mandatory=$true)][string]$ResticBin,
    [string]$TestPattern = 'TestPhaseSeven|TestPhaseFour'
)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$pgBin = (Resolve-Path -LiteralPath $PostgresBin).Path
$resticBinPath = (Resolve-Path -LiteralPath $ResticBin).Path
$fixtureRoot = Join-Path $repoRoot ('.dev/backup-fixture-' + [Guid]::NewGuid().ToString('N'))
$oldPath = $env:PATH
$oldURL = $env:PRISME_TEST_DATABASE_URL
$oldLive = $env:PRISME_TEST_BACKUP
$started = $false
Push-Location $repoRoot
try {
    foreach ($binary in @('initdb.exe','pg_ctl.exe','pg_dump.exe','pg_restore.exe','psql.exe','createdb.exe','dropdb.exe')) {
        if (-not (Test-Path -LiteralPath (Join-Path $pgBin $binary))) { throw "Missing PostgreSQL tool: $binary" }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $resticBinPath 'restic.exe'))) { throw 'ResticBin must contain restic.exe.' }
    New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
    $data = Join-Path $fixtureRoot 'data'
    $log = Join-Path $fixtureRoot 'postgres.log'
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()
    & (Join-Path $pgBin 'initdb.exe') -D $data -U postgres -A trust --encoding=UTF8 --locale=C
    if ($LASTEXITCODE -ne 0) { throw 'Fixture initialization failed.' }
    $process = Start-Process -FilePath (Join-Path $pgBin 'pg_ctl.exe') -ArgumentList @('-D',('"'+$data+'"'),'-l',('"'+$log+'"'),'-o',('"-h 127.0.0.1 -p '+$port+'"'),'-w','start') -WindowStyle Hidden -PassThru
    # Wait only for pg_ctl, not the PostgreSQL descendant that it starts.
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw 'Fixture startup failed.' }
    $started = $true
    $env:PATH = $pgBin + ';' + $resticBinPath + ';' + $oldPath
    $env:PRISME_TEST_DATABASE_URL = 'postgres://postgres@127.0.0.1:'+$port+'/postgres?sslmode=disable'
    $env:PRISME_TEST_BACKUP = '1'
    & go test -count=1 -v ./internal/api -run $TestPattern -timeout 8m
    if ($LASTEXITCODE -ne 0) { throw 'Native backup fixture failed.' }
} finally {
    if ($started) {
        $process = Start-Process -FilePath (Join-Path $pgBin 'pg_ctl.exe') -ArgumentList @('-D',('"'+$data+'"'),'-m','fast','-w','stop') -WindowStyle Hidden -PassThru
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) { Write-Warning "Fixture could not stop; inspect $fixtureRoot" }
    }
    $env:PATH = $oldPath
    $env:PRISME_TEST_DATABASE_URL = $oldURL
    $env:PRISME_TEST_BACKUP = $oldLive
    # Logs/data are retained in this new .dev directory for diagnosis. No user
    # database or installation is modified, and no recursive deletion is needed.
    Pop-Location
}
