# Install AgentLine on Windows.
#
#   irm https://raw.githubusercontent.com/seongyooo/agentline/main/install.ps1 | iex
#
# Downloads the release binary for this machine and puts it on PATH. No Go
# toolchain, no build step. Set $env:VERSION to pin one, and $env:BINDIR to
# choose where it lands.

$ErrorActionPreference = 'Stop'

$repo = 'seongyooo/agentline'
$version = if ($env:VERSION) { $env:VERSION } else { 'latest' }
$bindir = if ($env:BINDIR) { $env:BINDIR } else { "$env:LOCALAPPDATA\Programs\agentline" }

function Get-Architecture {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

# "latest" has to be turned into the tag it points at, since the download URL
# needs the real one.
function Resolve-Tag {
    if ($version -ne 'latest') { return $version }

    $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
    if (-not $release.tag_name) { throw 'could not find the latest release' }
    $release.tag_name
}

$tag = Resolve-Tag
$number = $tag -replace '^v', ''
$arch = Get-Architecture
$url = "https://github.com/$repo/releases/download/$tag/agentline_${number}_windows_$arch.zip"

Write-Host "installing agentline $tag (windows_$arch)"

$temp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid())
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $archive = Join-Path $temp 'agentline.zip'
    Invoke-WebRequest -Uri $url -OutFile $archive

    Expand-Archive -Path $archive -DestinationPath $temp -Force
    New-Item -ItemType Directory -Force -Path $bindir | Out-Null
    Copy-Item (Join-Path $temp 'agentline.exe') (Join-Path $bindir 'agentline.exe') -Force
}
finally {
    Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "installed $bindir\agentline.exe"

# Putting it on PATH for good is the difference between a command that works
# tomorrow and one that only works if you remember where it went.
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($userPath -notlike "*$bindir*") {
    $userPath = if ($userPath) { "$userPath;$bindir" } else { $bindir }
    [Environment]::SetEnvironmentVariable('PATH', $userPath, 'User')
    Write-Host "added $bindir to your PATH"
}

# The stored PATH only reaches processes started from here on. This script is
# run with `iex`, so it is inside the session the user is about to type into,
# and can fix that session too — otherwise the very next command they try is
# the one that fails.
if ($env:PATH -notlike "*$bindir*") {
    $env:PATH = "$env:PATH;$bindir"
}

Write-Host ""
Write-Host "ready - try: agentline --run"
