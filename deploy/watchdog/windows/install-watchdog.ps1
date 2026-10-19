<#
.SYNOPSIS
  Installe (ou met a jour) le veilleur Prisme sur un PC Windows : tache planifiee
  sous le compte integre SERVICE LOCAL, demarrage au boot, relance automatique.

.DESCRIPTION
  A lancer dans un PowerShell **administrateur**. Ne lit et n'affiche aucun secret :
  la cle FCM est copiee telle quelle, les jetons du telephone sont lus sur le VPS
  par SSH et ecrits directement dans la configuration, sans etre affiches.

  Dossier (par defaut %ProgramData%\PrismeWatchdog, -Root pour un autre disque) :
    bin\watchdog.exe                 lecture/execution pour SERVICE LOCAL
    config\watchdog.json             lecture seule pour SERVICE LOCAL
    config\fcm-service-account.json  lecture seule pour SERVICE LOCAL
    state\                           ecriture pour SERVICE LOCAL (etat du veilleur)
  Seuls SYSTEM, les Administrateurs et SERVICE LOCAL ont acces (heritage coupe).
  Le veilleur n'ecoute aucun port ; il interroge l'URL toutes les 30 s.

.EXAMPLE
  .\install-watchdog.ps1 -Instance tour -Root D:\PrismeWatchdog -Binary .\watchdog.exe -CredentialsFile C:\Users\me\Downloads\cle-tour.json
.EXAMPLE
  .\install-watchdog.ps1 -RefreshTokens   # apres un changement de telephone ou de jeton
.EXAMPLE
  .\install-watchdog.ps1 -Status
#>
[CmdletBinding(DefaultParameterSetName = 'Install')]
param(
    [Parameter(ParameterSetName = 'Install', Mandatory = $true)]
    [ValidatePattern('^[a-z0-9][a-z0-9-]{0,15}$')]
    [string]$Instance,

    [string]$Root = (Join-Path $env:ProgramData 'PrismeWatchdog'),

    [Parameter(ParameterSetName = 'Install', Mandatory = $true)]
    [string]$Binary,

    # Cle JSON d'un compte de service limite a l'envoi FCM (jamais le compte firebase-adminsdk).
    [Parameter(ParameterSetName = 'Install')]
    [string]$CredentialsFile,

    [Parameter(ParameterSetName = 'Install')]
    [Parameter(ParameterSetName = 'Refresh')]
    [string]$SshHost = 'ovh-mantara',

    [Parameter(ParameterSetName = 'Install')]
    [string]$Url = 'https://prisme.mantara.fr/readyz',

    [Parameter(ParameterSetName = 'Refresh', Mandatory = $true)]
    [switch]$RefreshTokens,

    [Parameter(ParameterSetName = 'Status', Mandatory = $true)]
    [switch]$Status,

    [Parameter(ParameterSetName = 'Uninstall', Mandatory = $true)]
    [switch]$Uninstall,

    # Construit et verifie tout sans rien installer (ni dossier, ni tache, ni SSH).
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$TaskPath = '\Prisme\'
$TaskName = 'Prisme watchdog'
# SID plutot que noms : Windows francais dit "AUTORITE NT\SERVICE LOCAL", "Administrateurs".
$LocalService = 'S-1-5-19'
$System = 'S-1-5-18'
$Admins = 'S-1-5-32-544'
$Bin = Join-Path $Root 'bin'
$Config = Join-Path $Root 'config'
$State = Join-Path $Root 'state'
$ConfigFile = Join-Path $Config 'watchdog.json'
$KeyFile = Join-Path $Config 'fcm-service-account.json'
$StateFile = Join-Path $State 'state.json'

function Assert-Admin {
    $p = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    if (-not $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Lancez ce script dans un PowerShell administrateur (clic droit > Executer en tant qu''administrateur).'
    }
}

function Invoke-Icacls([string[]]$Arguments) {
    & icacls.exe @Arguments | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "icacls a echoue ($LASTEXITCODE) : $($Arguments -join ' ')" }
}

function Write-Utf8NoBom([string]$Path, [string]$Text) {
    # Go refuse un BOM en tete de JSON ; Set-Content -Encoding UTF8 en ajoute un sous PowerShell 5.1.
    [IO.File]::WriteAllText($Path, $Text, (New-Object Text.UTF8Encoding $false))
}

function Test-ServiceAccountKey([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "Cle FCM introuvable : $Path" }
    $j = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if ($j.type -ne 'service_account' -or $j.client_email -notmatch '^[a-z0-9-]+@[a-z0-9-]+\.iam\.gserviceaccount\.com$' -or -not $j.private_key) {
        throw 'Ce fichier n''est pas une cle JSON de compte de service Google.'
    }
    if ($j.client_email -like 'firebase-adminsdk-*') {
        throw 'Refus : cle du compte complet firebase-adminsdk. Utilisez la cle du compte limite a l''envoi FCM.'
    }
    return [pscustomobject]@{ Project = $j.project_id; Email = $j.client_email; KeyId = $j.private_key_id }
}

function Get-PhoneTokens {
    # Jetons FCM des appareils actifs, lus dans la base du control plane ; jamais affiches.
    $sql = 'SELECT n.token FROM notification_devices n JOIN devices d ON d.id = n.device_id WHERE n.enabled AND NOT d.revoked ORDER BY n.updated_at DESC LIMIT 20'
    # Guillemets simples seulement : PowerShell 5.1 transmet mal les guillemets doubles a ssh.exe.
    $remote = "cd /opt/projects/prisme && sudo docker compose --env-file deploy/production/ovh-mantara.env -f deploy/production/compose.yml exec -T postgres psql -U prisme -d prisme -At -c '$sql' < /dev/null"
    # Sous PowerShell 5.1, une ligne sur stderr d'un programme natif devient une erreur fatale avec 'Stop'.
    $ErrorActionPreference = 'Continue'
    $out = & ssh.exe -o BatchMode=yes $SshHost $remote 2>$null
    $code = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($code -ne 0) { throw "Lecture des jetons impossible par SSH ($SshHost, code $code)." }
    $tokens = @($out | ForEach-Object { $_.Trim() } | Where-Object { $_ -match '^[A-Za-z0-9_:\-]{100,4096}$' })
    if ($tokens.Count -eq 0) {
        throw 'Aucun telephone abonne aux notifications : activez-les d''abord dans Prisme sur le S24.'
    }
    return , $tokens
}

function New-WatchdogConfig([string[]]$Tokens) {
    $cfg = [ordered]@{
        url              = $Url
        state_file       = $StateFile
        credentials_file = $KeyFile
        tokens           = $Tokens
        threshold        = 3
        urgent           = $true
        instance         = $Instance
        # Horaires silencieux 22 h - 7 h Paris ; urgent les traverse.
        preferences      = [ordered]@{ enabled = $true; timezone = 'Europe/Paris'; quiet_start = 1320; quiet_end = 420 }
    }
    return ($cfg | ConvertTo-Json -Depth 4)
}

function New-WatchdogTask {
    $exe = Join-Path $Bin 'watchdog.exe'
    $action = New-ScheduledTaskAction -Execute $exe -Argument ('-config "{0}"' -f $ConfigFile) -WorkingDirectory $State
    # Au demarrage, puis toutes les 5 minutes : relance le veilleur s'il s'est arrete
    # (une seule instance a la fois).
    $boot = New-ScheduledTaskTrigger -AtStartup
    $keepAlive = New-ScheduledTaskTrigger -Once -At (Get-Date).Date -RepetitionInterval (New-TimeSpan -Minutes 5)
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable `
        -MultipleInstances IgnoreNew -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
    $principal = New-ScheduledTaskPrincipal -UserId $LocalService -LogonType ServiceAccount -RunLevel Limited
    return New-ScheduledTask -Action $action -Trigger @($boot, $keepAlive) -Settings $settings -Principal $principal `
        -Description 'Veilleur Prisme : previent le S24 si https://prisme.mantara.fr ne repond plus. Aucune ecoute reseau.'
}

function Show-Status {
    $t = Get-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction SilentlyContinue
    if (-not $t) { Write-Host 'Tache absente.'; return }
    $i = $t | Get-ScheduledTaskInfo
    Write-Host ("Tache : {0} ; derniere execution {1} (code {2}) ; compte {3}" -f $t.State, $i.LastRunTime, $i.LastTaskResult, $t.Principal.UserId)
    $p = Get-Process -Name watchdog -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq (Join-Path $Bin 'watchdog.exe') }
    Write-Host ('Processus : ' + $(if ($p) { "en marche (PID $($p.Id))" } else { 'arrete' }))
    if (Test-Path -LiteralPath $StateFile) {
        $s = Get-Content -LiteralPath $StateFile -Raw | ConvertFrom-Json
        # Go ecrit 9 decimales, .NET n'en lit que 7.
        $probe = [DateTimeOffset]::Parse(($s.last_probe -replace '(\.\d{7})\d+', '$1'), [Globalization.CultureInfo]::InvariantCulture)
        $age = [int]([DateTimeOffset]::UtcNow - $probe).TotalSeconds
        $alerted = if ($s.alerted) { 'OUI' } else { 'non' }
        Write-Host ("Dernier controle il y a {0} s ; echecs consecutifs {1} ; panne signalee : {2}" -f $age, $s.failures, $alerted)
    } else {
        Write-Host 'Aucun etat encore (premier controle dans les 30 s).'
    }
    if (Test-Path -LiteralPath $ConfigFile) {
        $c = Get-Content -LiteralPath $ConfigFile -Raw | ConvertFrom-Json
        Write-Host ("Configuration : {0}, instance {1}, urgent {2}, {3} telephone(s)" -f $c.url, $c.instance, $c.urgent, @($c.tokens).Count)
    }
}

switch ($PSCmdlet.ParameterSetName) {
    'Status' {
        Assert-Admin
        Show-Status
        return
    }
    'Uninstall' {
        Assert-Admin
        if ($DryRun) { Write-Host "[essai] suppression de la tache $TaskPath$TaskName"; return }
        Stop-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction SilentlyContinue
        Unregister-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
        Write-Host "Tache supprimee. Le dossier $Root est conserve (cle FCM incluse) : supprimez-le vous-meme si besoin."
        return
    }
    'Refresh' {
        if (-not $DryRun) { Assert-Admin }
        if (-not (Test-Path -LiteralPath $ConfigFile)) { throw "Veilleur non installe ($ConfigFile absent)." }
        if ($DryRun) { Write-Host "[essai] relecture des jetons par SSH ($SshHost) puis reecriture de $ConfigFile"; return }
        $tokens = Get-PhoneTokens
        # Seuls les jetons changent ; URL, instance et horaires sont conserves.
        $cfg = Get-Content -LiteralPath $ConfigFile -Raw | ConvertFrom-Json
        $cfg.tokens = $tokens
        Write-Utf8NoBom $ConfigFile ($cfg | ConvertTo-Json -Depth 4)
        Stop-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction SilentlyContinue
        Start-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName
        Write-Host ("Jetons mis a jour : {0} telephone(s). Veilleur relance." -f $tokens.Count)
        return
    }
}

# --- Installation ---
# ValidatePattern ignore la casse ; le veilleur, lui, refuse les majuscules.
if ($Instance -cnotmatch '^[a-z0-9][a-z0-9-]{0,15}$') { throw "Libelle d'instance invalide : $Instance (minuscules, chiffres et tirets, 16 caracteres au plus)." }
if (-not $DryRun) { Assert-Admin }
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) { throw "Programme introuvable : $Binary" }
$hash = (Get-FileHash -LiteralPath $Binary -Algorithm SHA256).Hash
if (-not $CredentialsFile -and -not (Test-Path -LiteralPath $KeyFile)) { throw 'Indiquez -CredentialsFile (cle FCM du veilleur) pour une premiere installation.' }
if ($CredentialsFile) {
    $key = Test-ServiceAccountKey $CredentialsFile
    Write-Host ("Cle FCM : compte {0}, projet {1}, cle {2}" -f $key.Email, $key.Project, $key.KeyId)
}
$task = New-WatchdogTask
if ($DryRun) {
    Write-Host "[essai] programme $Binary (SHA-256 $hash)"
    Write-Host "[essai] dossier $Root : heritage coupe ; SYSTEM et Administrateurs controle total ; SERVICE LOCAL lecture, ecriture dans state seulement"
    Write-Host ("[essai] tache {0}{1} sous {2} ({3}), {4} declencheurs, relance {5} fois/1 min, duree illimitee" -f $TaskPath, $TaskName, $task.Principal.UserId, $task.Principal.LogonType, @($task.Triggers).Count, $task.Settings.RestartCount)
    Write-Host '[essai] configuration (jetons remplaces par un exemple) :'
    Write-Host (New-WatchdogConfig @('EXEMPLE-JETON-NON-REEL'))
    return
}

$tokens = Get-PhoneTokens
foreach ($d in @($Root, $Bin, $Config, $State)) { New-Item -ItemType Directory -Force -Path $d | Out-Null }
# ACL avant d'y copier la cle : heritage coupe, trois comptes seulement.
Invoke-Icacls @($Root, '/inheritance:r', '/grant:r', "*${System}:(OI)(CI)F", "*${Admins}:(OI)(CI)F", "*${LocalService}:(OI)(CI)RX")
Invoke-Icacls @($State, '/grant:r', "*${LocalService}:(OI)(CI)M")
Stop-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -ErrorAction SilentlyContinue
Get-Process -Name watchdog -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq (Join-Path $Bin 'watchdog.exe') } | Stop-Process -Force
Copy-Item -LiteralPath $Binary -Destination (Join-Path $Bin 'watchdog.exe') -Force
# Un fichier cree par copie herite des ACL du dossier de destination, pas de celles de l'original.
if ($CredentialsFile) { Copy-Item -LiteralPath $CredentialsFile -Destination $KeyFile -Force }
Write-Utf8NoBom $ConfigFile (New-WatchdogConfig $tokens)
Register-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName -InputObject $task -Force | Out-Null
Start-ScheduledTask -TaskPath $TaskPath -TaskName $TaskName
Write-Host ("Veilleur '{0}' installe dans {1} : {2} telephone(s), programme SHA-256 {3}." -f $Instance, $Root, $tokens.Count, $hash)
if ($CredentialsFile) { Write-Host "Supprimez maintenant l'original de la cle : $CredentialsFile" }
Write-Host 'Etat dans une minute : .\install-watchdog.ps1 -Status'
