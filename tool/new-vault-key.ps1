param([Parameter(Mandatory=$true)][string]$Path)
$ErrorActionPreference = 'Stop'
$keyPath = [IO.Path]::GetFullPath($Path)
if (Test-Path -LiteralPath $keyPath) { throw 'Refusing to overwrite an existing master key.' }
$parentPath = Split-Path -Parent $keyPath
if (-not (Test-Path -LiteralPath $parentPath -PathType Container)) { throw 'Create a private parent directory first.' }
$keyBytes = [byte[]]::new(32)
[Security.Cryptography.RandomNumberGenerator]::Fill($keyBytes)
$stream = [IO.File]::Open($keyPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
try {
    $acl = Get-Acl -LiteralPath $keyPath
    $acl.SetAccessRuleProtection($true, $false)
    $owner = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl.SetOwner($owner)
    $rule = [Security.AccessControl.FileSystemAccessRule]::new($owner, 'FullControl', 'Allow')
    $acl.SetAccessRule($rule)
    Set-Acl -LiteralPath $keyPath -AclObject $acl
    $stream.Write($keyBytes,0,$keyBytes.Length)
    $stream.Flush($true)
} finally { $stream.Dispose(); [Array]::Clear($keyBytes,0,$keyBytes.Length) }
Write-Host 'Master key created in a private file. Keep an independent offline backup before storing secrets.'
