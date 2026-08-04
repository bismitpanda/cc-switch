# Install or upgrade cc-switch from the latest GitHub release.
#
#   irm https://raw.githubusercontent.com/bismitpanda/cc-switch/main/install.ps1 | iex
#
# Optional env:
#   CC_SWITCH_VERSION      Pin a release tag (e.g. v1.1.1). Default: latest.
#   CC_SWITCH_INSTALL_DIR  Install directory. Default: %LOCALAPPDATA%\Programs\cc-switch

$ErrorActionPreference = 'Stop'

$Repo = 'bismitpanda/cc-switch'
$Binary = 'cc-switch.exe'

function Write-Info([string]$Message) {
    Write-Host $Message
}

function Die([string]$Message) {
    Write-Error "error: $Message"
    exit 1
}

$InstallDir = if ($env:CC_SWITCH_INSTALL_DIR -and $env:CC_SWITCH_INSTALL_DIR.Trim() -ne '') {
    $env:CC_SWITCH_INSTALL_DIR.Trim()
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\cc-switch'
}

$Version = if ($env:CC_SWITCH_VERSION -and $env:CC_SWITCH_VERSION.Trim() -ne '') {
    $env:CC_SWITCH_VERSION.Trim()
} else {
    ''
}

$Arch = $env:PROCESSOR_ARCHITECTURE
switch -Regex ($Arch) {
    '^(AMD64|X64)$' { $Arch = 'amd64' }
    '^(ARM64)$' { $Arch = 'arm64' }
    default { Die "unsupported architecture: $Arch" }
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    Write-Info 'Resolving latest release…'
    try {
        $headers = @{
            Accept                 = 'application/vnd.github+json'
            'User-Agent'           = 'cc-switch-install'
            'X-GitHub-Api-Version' = '2022-11-28'
        }
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
        $Version = [string]$release.tag_name
    } catch {
        Die "could not resolve latest release; set CC_SWITCH_VERSION ($($_.Exception.Message))"
    }
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    Die 'could not resolve latest release; set CC_SWITCH_VERSION'
}

if (-not $Version.StartsWith('v')) {
    $Version = "v$Version"
}

$Asset = "cc-switch-$Version-windows-$Arch.exe"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
$Dest = Join-Path $InstallDir $Binary

Write-Info "Installing cc-switch $Version (windows/$Arch) → $Dest"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("cc-switch-" + [guid]::NewGuid().ToString('N') + '.exe')
try {
    Invoke-WebRequest -Uri $Url -OutFile $tmp -UseBasicParsing
    # Replace in place; retry briefly if the running binary is locked.
    $replaced = $false
    for ($i = 0; $i -lt 5; $i++) {
        try {
            Move-Item -Force -Path $tmp -Destination $Dest
            $replaced = $true
            break
        } catch {
            Start-Sleep -Milliseconds 200
        }
    }
    if (-not $replaced) {
        Die "could not write $Dest (is cc-switch running?)"
    }
} finally {
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -Force -LiteralPath $tmp -ErrorAction SilentlyContinue
    }
}

$onPath = ($env:PATH -split ';' | Where-Object { $_ -and ($_ -ieq $InstallDir) }).Count -gt 0
if (-not $onPath) {
    Write-Info "Note: $InstallDir is not on PATH. Add it for this session:"
    Write-Info "  `$env:PATH = `"$InstallDir;`$env:PATH`""
    Write-Info 'Or permanently (User PATH):'
    Write-Info "  [Environment]::SetEnvironmentVariable('Path', `"$InstallDir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
}

try {
    $ver = & $Dest --version 2>$null
    if ($ver) {
        Write-Info "Installed $ver"
    } else {
        Write-Info "Installed $Dest"
    }
} catch {
    Write-Info "Installed $Dest"
}
