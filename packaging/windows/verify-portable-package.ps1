param(
  [Parameter(Mandatory = $true)]
  [string]$Archive,
  [Parameter(Mandatory = $true)]
  [string]$ExpectedVersion,
  [Parameter(Mandatory = $true)]
  [string]$ExpectedCommit,
  [switch]$RequireSignature
)

$ErrorActionPreference = 'Stop'
$archivePath = (Resolve-Path -LiteralPath $Archive).Path
$extractPath = Join-Path ([IO.Path]::GetTempPath()) "agent-card-verify-$([guid]::NewGuid().ToString('N'))"
$required = @(
  'agent_card_desktop.exe',
  'flutter_windows.dll',
  'sqlite3.dll',
  'libsodium.dll',
  'data/flutter_assets/AssetManifest.bin',
  'README.txt',
  'release-manifest.json'
)

function Assert-SafeArchive([string]$Path) {
  Add-Type -AssemblyName System.IO.Compression
  $stream = [IO.File]::OpenRead($Path)
  try {
    $zip = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Read)
    try {
      $paths = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
      [long]$expandedBytes = 0
      foreach ($entry in $zip.Entries) {
        $name = $entry.FullName.Replace('\', '/')
        $segments = $name.Split('/', [StringSplitOptions]::RemoveEmptyEntries)
        if ([string]::IsNullOrWhiteSpace($name) -or
            $name.StartsWith('/') -or
            $name -match '^[A-Za-z]:' -or
            $segments -contains '..' -or
            $name.Contains([char]0) -or
            -not $paths.Add($name)) {
          throw "Unsafe or duplicate archive path: $name"
        }
        $unixType = ($entry.ExternalAttributes -shr 16) -band 0xF000
        $windowsAttributes = $entry.ExternalAttributes -band 0xFFFF
        if ($unixType -eq 0xA000 -or ($windowsAttributes -band 0x400) -ne 0) {
          throw "Archive links are forbidden: $name"
        }
        $expandedBytes += $entry.Length
        if ($expandedBytes -gt 1GB) {
          throw 'Archive expanded size exceeds 1 GiB'
        }
      }
    } finally {
      $zip.Dispose()
    }
  } finally {
    $stream.Dispose()
  }
}

try {
  Assert-SafeArchive $archivePath
  New-Item -ItemType Directory -Path $extractPath -Force | Out-Null
  Expand-Archive -LiteralPath $archivePath -DestinationPath $extractPath

  foreach ($name in $required) {
    if (-not (Test-Path -LiteralPath (Join-Path $extractPath $name) -PathType Leaf)) {
      throw "Missing required portable file: $name"
    }
  }
  $forbidden = Get-ChildItem -LiteralPath $extractPath -Recurse -File -Force | Where-Object {
    $_.Name -match '(^\.env($|\.)|\.(db|sqlite|sqlite3|log|pem|key|pfx|p12)$)' -or
    $_.FullName -match '[\\/](secrets?|private)[\\/]'
  }
  if ($forbidden) {
    throw "Forbidden portable files: $($forbidden.FullName -join ', ')"
  }
  $links = Get-ChildItem -LiteralPath $extractPath -Recurse -Force | Where-Object {
    ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0
  }
  if ($links) {
    throw "Portable package contains links: $($links.FullName -join ', ')"
  }

  $manifestPath = Join-Path $extractPath 'release-manifest.json'
  $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
  if ($manifest.schemaVersion -ne 1 -or
      $manifest.version -ne $ExpectedVersion -or
      $manifest.commit -ne $ExpectedCommit -or
      $manifest.signed -isnot [bool] -or
      $manifest.files -isnot [array]) {
    throw 'Portable release manifest envelope is invalid'
  }

  $actualFiles = Get-ChildItem -LiteralPath $extractPath -Recurse -File -Force |
    Where-Object Name -ne 'release-manifest.json' |
    Sort-Object FullName |
    ForEach-Object {
      [ordered]@{
        path = $_.FullName.Substring($extractPath.Length + 1).Replace('\', '/')
        sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
      }
    }
  $manifestFiles = @($manifest.files | Sort-Object path)
  if ($manifestFiles.Count -ne $actualFiles.Count) {
    throw 'Portable manifest file count does not match archive contents'
  }
  for ($index = 0; $index -lt $actualFiles.Count; $index++) {
    if ($manifestFiles[$index].path -cne $actualFiles[$index].path -or
        $manifestFiles[$index].sha256 -cne $actualFiles[$index].sha256) {
      throw "Portable manifest mismatch: expected=$($manifestFiles[$index].path):$($manifestFiles[$index].sha256) actual=$($actualFiles[$index].path):$($actualFiles[$index].sha256)"
    }
  }

  $signatureStatus = 'NotRequired'
  if ($RequireSignature) {
    if (-not $IsWindows) {
      throw 'Authenticode verification requires Windows'
    }
    $signature = Get-AuthenticodeSignature -FilePath (Join-Path $extractPath 'agent_card_desktop.exe')
    if ($signature.Status -ne 'Valid') {
      throw "Desktop executable signature is not valid: $($signature.Status)"
    }
    if ($manifest.signed -ne $true) {
      throw 'Signed executable is not marked signed in the release manifest'
    }
    $signatureStatus = 'Valid'
  }

  [ordered]@{
    schemaVersion = 1
    archive = $archivePath
    version = $manifest.version
    commit = $manifest.commit
    signatureStatus = $signatureStatus
    fileCount = $actualFiles.Count
    sha256 = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
  } | ConvertTo-Json -Depth 3
} finally {
  Remove-Item -LiteralPath $extractPath -Recurse -Force -ErrorAction SilentlyContinue
}
