param(
    [switch]$BuildWindows,
    [switch]$BuildWeb
)

$ErrorActionPreference = 'Stop'
$packageRoot = Join-Path $PSScriptRoot '../packages/prisme_ui'

function Invoke-Check {
    param([string]$Command, [string[]]$Arguments)
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command $($Arguments -join ' ') failed ($LASTEXITCODE)."
    }
}

Push-Location $packageRoot
try {
    Invoke-Check -Command flutter -Arguments @('pub', 'get')
    Push-Location example
    try { Invoke-Check -Command flutter -Arguments @('pub', 'get') }
    finally { Pop-Location }
    Invoke-Check -Command dart -Arguments @('run', 'tool/sync_tokens.dart', '--check')
    Invoke-Check -Command dart -Arguments @('format', '--output=none', '--set-exit-if-changed', 'lib', 'test', 'tool', 'example/lib', 'example/test')
    Invoke-Check -Command flutter -Arguments @('analyze', '--no-pub')
    Invoke-Check -Command flutter -Arguments @('test', '--no-pub')
    Push-Location example
    try {
        Invoke-Check -Command flutter -Arguments @('test', '--no-pub')
        if ($BuildWindows) {
            Invoke-Check -Command flutter -Arguments @('build', 'windows', '--release', '--no-pub')
        }
        if ($BuildWeb) {
            Invoke-Check -Command flutter -Arguments @('build', 'web', '--release', '--no-pub', '--no-web-resources-cdn', '--pwa-strategy=none')
        }
    } finally { Pop-Location }
} finally { Pop-Location }
