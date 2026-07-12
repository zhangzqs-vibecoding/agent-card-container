param(
  [Parameter(Mandatory = $true)]
  [string]$Bundle,
  [Parameter(Mandatory = $true)]
  [string]$OutputDirectory,
  [Parameter(Mandatory = $true)]
  [string]$Version,
  [Parameter(Mandatory = $true)]
  [string]$Commit,
  [switch]$RequireSignature
)

$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+([+-][0-9A-Za-z.-]+)?$') {
  throw 'Version must be semantic version text'
}
if ($Commit -notmatch '^[0-9a-fA-F]{40}$') {
  throw 'Commit must be a full 40-character SHA'
}
$bundlePath = (Resolve-Path -LiteralPath $Bundle).Path
$outputPath = [IO.Path]::GetFullPath($OutputDirectory)
$staging = Join-Path ([IO.Path]::GetTempPath()) "agent-card-package-$([guid]::NewGuid().ToString('N'))"
$required = @(
  'agent_card_desktop.exe',
  'flutter_windows.dll',
  'sqlite3.dll',
  'libsodium.dll',
  'data/flutter_assets/AssetManifest.bin'
)

try {
  foreach ($name in $required) {
    if (-not (Test-Path -LiteralPath (Join-Path $bundlePath $name) -PathType Leaf)) {
      throw "Missing required bundle file: $name"
    }
  }
  $forbidden = Get-ChildItem -LiteralPath $bundlePath -Recurse -File -Force | Where-Object {
    $_.Name -match '(^\.env($|\.)|\.(db|sqlite|sqlite3|log|pem|key|pfx|p12)$)' -or
    $_.FullName -match '[\\/](secrets?|private)[\\/]'
  }
  if ($forbidden) {
    throw "Forbidden source bundle files: $($forbidden.FullName -join ', ')"
  }

  $signed = $false
  if ($IsWindows) {
    $signature = Get-AuthenticodeSignature -FilePath (Join-Path $bundlePath 'agent_card_desktop.exe')
    $signed = $signature.Status -eq 'Valid'
  }
  if ($RequireSignature -and -not $signed) {
    throw 'A valid Authenticode signature is required for this package'
  }

  New-Item -ItemType Directory -Path $staging, $outputPath -Force | Out-Null
  Get-ChildItem -LiteralPath $bundlePath -Force | Copy-Item -Destination $staging -Recurse -Force
  @"
Agent Card Container $Version
Commit: $Commit

Start: double-click agent_card_desktop.exe.

This portable snapshot may be unsigned and can trigger Windows SmartScreen.
CodeCard requires Microsoft Edge WebView2 Evergreen Runtime. If it is missing,
install the official x64 runtime from:
https://developer.microsoft.com/microsoft-edge/webview2/

The desktop client contains no model-provider API key. Configure cloud access
through process environment as documented in the repository README.
"@ | Set-Content -LiteralPath (Join-Path $staging 'README.txt') -Encoding utf8

  $files = Get-ChildItem -LiteralPath $staging -Recurse -File -Force |
    Sort-Object FullName |
    ForEach-Object {
      [ordered]@{
        path = $_.FullName.Substring($staging.Length + 1).Replace('\', '/')
        sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
      }
    }
  [ordered]@{
    schemaVersion = 1
    version = $Version
    commit = $Commit.ToLowerInvariant()
    signed = $signed
    files = $files
  } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $staging 'release-manifest.json') -Encoding utf8

  $shortCommit = $Commit.Substring(0, 7).ToLowerInvariant()
  $archiveName = "AgentCardContainer-windows-x64-$Version-$shortCommit.zip"
  $archivePath = Join-Path $outputPath $archiveName
  Compress-Archive -Path (Join-Path $staging '*') -DestinationPath $archivePath -CompressionLevel Optimal -Force
  $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
  "$archiveHash  $archiveName" | Set-Content -LiteralPath "$archivePath.sha256" -Encoding ascii

  $verifyArguments = @(
    '-NoProfile', '-File', (Join-Path $PSScriptRoot 'verify-portable-package.ps1'),
    '-Archive', $archivePath,
    '-ExpectedVersion', $Version,
    '-ExpectedCommit', $Commit.ToLowerInvariant()
  )
  if ($RequireSignature) { $verifyArguments += '-RequireSignature' }
  & pwsh @verifyArguments
  if ($LASTEXITCODE -ne 0) {
    throw 'Portable package verification failed'
  }

  [ordered]@{
    schemaVersion = 1
    archive = $archivePath
    checksum = "$archivePath.sha256"
    sha256 = $archiveHash
    signed = $signed
  } | ConvertTo-Json -Depth 3
} finally {
  Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
}
