param()

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$verifier = Join-Path $PSScriptRoot 'verify-portable-package.ps1'
$workspace = Join-Path ([IO.Path]::GetTempPath()) "agent-card-portable-$([guid]::NewGuid().ToString('N'))"

function New-TestPayload([string]$Path) {
  New-Item -ItemType Directory -Path (Join-Path $Path 'data/flutter_assets') -Force | Out-Null
  @{
    'agent_card_desktop.exe' = 'exe fixture'
    'flutter_windows.dll' = 'flutter fixture'
    'sqlite3.dll' = 'sqlite fixture'
    'libsodium.dll' = 'sodium fixture'
    'data/flutter_assets/AssetManifest.bin' = 'asset fixture'
    'README.txt' = 'Unsigned test package'
  }.GetEnumerator() | ForEach-Object {
    $target = Join-Path $Path $_.Key
    Set-Content -LiteralPath $target -Value $_.Value -NoNewline
  }
  $files = Get-ChildItem -LiteralPath $Path -Recurse -File |
    Sort-Object FullName |
    ForEach-Object {
      [ordered]@{
        path = $_.FullName.Substring($Path.Length + 1).Replace('\', '/')
        sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
      }
    }
  [ordered]@{
    schemaVersion = 1
    version = '1.2.3'
    commit = 'abcdef0123456789abcdef0123456789abcdef01'
    signed = $false
    files = $files
  } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $Path 'release-manifest.json')
}

function New-TestArchive([string]$Name, [scriptblock]$Mutate) {
  $payload = Join-Path $workspace "$Name-payload"
  $archive = Join-Path $workspace "$Name.zip"
  New-Item -ItemType Directory -Path $payload -Force | Out-Null
  New-TestPayload $payload
  & $Mutate $payload
  Compress-Archive -Path (Join-Path $payload '*') -DestinationPath $archive
  return $archive
}

function Add-ArchiveEntry([string]$Archive, [string]$Name, [string]$Content) {
  Add-Type -AssemblyName System.IO.Compression
  $stream = [IO.File]::Open($Archive, [IO.FileMode]::Open)
  try {
    $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Update)
    try {
      $entry = $zip.CreateEntry($Name)
      $writer = [IO.StreamWriter]::new($entry.Open())
      try { $writer.Write($Content) } finally { $writer.Dispose() }
    } finally { $zip.Dispose() }
  } finally { $stream.Dispose() }
}

function Invoke-Verifier([string]$Archive) {
  $script:LastVerifierOutput = & pwsh -NoProfile -File $verifier `
    -Archive $Archive `
    -ExpectedVersion '1.2.3' `
    -ExpectedCommit 'abcdef0123456789abcdef0123456789abcdef01' 2>&1 | Out-String
  return $LASTEXITCODE
}

function Assert-Passes([string]$Archive) {
  $exit = Invoke-Verifier $Archive
  if ($exit -ne 0) {
    throw "Expected archive to pass: $Archive`n$script:LastVerifierOutput"
  }
}

function Assert-Fails([string]$Archive) {
  $exit = Invoke-Verifier $Archive
  if ($exit -eq 0) {
    throw "Expected archive to fail: $Archive"
  }
}

try {
  New-Item -ItemType Directory -Path $workspace -Force | Out-Null
  Assert-Passes (New-TestArchive 'valid' {})
  Assert-Fails (New-TestArchive 'missing-dll' {
    param($payload)
    Remove-Item -LiteralPath (Join-Path $payload 'sqlite3.dll')
  })
  $forbidden = New-TestArchive 'forbidden-env' {}
  Add-ArchiveEntry $forbidden '.env' 'fixture=true'
  Assert-Fails $forbidden
  Assert-Fails (New-TestArchive 'missing-readme' {
    param($payload)
    Remove-Item -LiteralPath (Join-Path $payload 'README.txt')
  })

  $traversal = New-TestArchive 'traversal' {}
  Add-ArchiveEntry $traversal '../escape.txt' 'escape'
  Assert-Fails $traversal
  Write-Output 'portable package verifier tests passed'
} finally {
  Remove-Item -LiteralPath $workspace -Recurse -Force -ErrorAction SilentlyContinue
}
