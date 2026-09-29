# Installs dwf on Windows into %LOCALAPPDATA%\Programs\discord-warp-fix.
#
#   irm https://raw.githubusercontent.com/rockswe/erisim/main/install.ps1 | iex
#
# UNTESTED on real Windows so far. See CONTRIBUTING.md.
$ErrorActionPreference = 'Stop'
$repo = 'rockswe/erisim'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$dir = Join-Path $env:LOCALAPPDATA 'Programs\discord-warp-fix'
$exe = Join-Path $dir 'dwf.exe'
$url = "https://github.com/$repo/releases/latest/download/dwf-windows-$arch.exe"

New-Item -ItemType Directory -Force -Path $dir | Out-Null
# A running dwf locks the file.
Get-Process dwf -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host "downloading $url"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $exe

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $userPath) { $userPath = '' }
if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir).TrimStart(';'), 'User')
    Write-Host "added $dir to your PATH"
}
$env:Path = "$env:Path;$dir"
Write-Host "installed $exe ($(& $exe version))"

# Already set up: restart the background service with the new binary.
if (Test-Path (Join-Path $env:APPDATA 'discord-warp-fix\config')) { & $exe install }

Write-Host ''
Write-Host 'Next: dwf setup   (open a new terminal first if "dwf" is not found)'
