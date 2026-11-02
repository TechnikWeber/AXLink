# SPDX-License-Identifier: GPL-3.0-or-later
<#
.SYNOPSIS
Fetches wintun.dll, the driver library behind AXLink's network adapter on Windows.

.DESCRIPTION
Downloads the official Wintun package from wintun.net, checks its SHA-256
checksum and puts wintun.dll for the chosen architecture, together with its
licence (wintun-LICENSE.txt), into the destination folder. axlinkd.exe loads
wintun.dll from its own folder, so the destination should be that folder.

wintun.dll comes from WireGuard LLC under its own licence, which this script
copies next to it (see LICENSING.md).

.PARAMETER Destination
The folder of axlinkd.exe. Default: the current folder.

.PARAMETER Arch
amd64, arm64, x86 or arm. It must match axlinkd.exe (windows/amd64 needs
amd64, windows/arm64 needs arm64). Default: the architecture of this Windows.

.EXAMPLE
powershell -ExecutionPolicy Bypass -File .\scripts\fetch-wintun.ps1 -Destination C:\AXLink

Windows 10 and 11 run no PowerShell scripts by default; -ExecutionPolicy
Bypass allows this one run only.
#>
[CmdletBinding()]
param(
    [string]$Destination = (Get-Location).Path,
    [ValidateSet('amd64', 'arm64', 'x86', 'arm')]
    [string]$Arch
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # the progress bar slows downloads down in Windows PowerShell

$Version = '0.14.1'
$Url = "https://www.wintun.net/builds/wintun-$Version.zip"
$Sha256 = '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51'

if (-not $Arch) {
    $os = ''
    try { $os = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch { }
    if (-not $os) {
        $os = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    }
    $names = @{ X64 = 'amd64'; AMD64 = 'amd64'; Arm64 = 'arm64'; X86 = 'x86'; Arm = 'arm' }
    if (-not $names.ContainsKey($os)) {
        throw "unknown architecture '$os'; pass -Arch amd64, arm64, x86 or arm"
    }
    $Arch = $names[$os]
}

# Windows PowerShell 5.1 does not offer TLS 1.2 by default everywhere.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$work = Join-Path ([IO.Path]::GetTempPath()) ('axlink-wintun-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
try {
    $zip = Join-Path $work "wintun-$Version.zip"
    Write-Host "Downloading $Url"
    Invoke-WebRequest -Uri $Url -OutFile $zip -UseBasicParsing
    $hash = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLowerInvariant()
    if ($hash -ne $Sha256) {
        throw "checksum mismatch for wintun-$Version.zip: got $hash, want $Sha256"
    }
    Expand-Archive -Path $zip -DestinationPath $work
    New-Item -ItemType Directory -Force -Path $Destination | Out-Null
    Copy-Item -Force -Path (Join-Path $work "wintun\bin\$Arch\wintun.dll") -Destination (Join-Path $Destination 'wintun.dll')
    Copy-Item -Force -Path (Join-Path $work 'wintun\LICENSE.txt') -Destination (Join-Path $Destination 'wintun-LICENSE.txt')
    Write-Host "wintun.dll $Version ($Arch) is now in $Destination"
}
finally {
    Remove-Item -Recurse -Force -Path $work -ErrorAction SilentlyContinue
}
