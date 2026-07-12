param(
  [Parameter(Mandatory = $true)]
  [string]$Bundle,
  [switch]$RequireSignature
)

$ErrorActionPreference = 'Stop'
$bundlePath = (Resolve-Path $Bundle).Path
$required = @(
  'agent_card_desktop.exe',
  'flutter_windows.dll',
  'sqlite3.dll',
  'libsodium.dll'
)

foreach ($name in $required) {
  $path = Join-Path $bundlePath $name
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    throw "Missing required release file: $name"
  }
}

$forbidden = Get-ChildItem -LiteralPath $bundlePath -Recurse -File | Where-Object {
  $_.Name -match '(^\.env($|\.)|\.(db|sqlite|sqlite3|log|pem|key)$)' -or
  $_.FullName -match '[\\/](secrets?|private)[\\/]'
}
if ($forbidden) {
  throw "Forbidden release files: $($forbidden.FullName -join ', ')"
}

$executable = Join-Path $bundlePath 'agent_card_desktop.exe'
$signature = Get-AuthenticodeSignature -FilePath $executable
if ($RequireSignature -and $signature.Status -ne 'Valid') {
  throw "Desktop executable signature is not valid: $($signature.Status)"
}

$client = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
$registryPaths = @(
  "HKCU:\Software\Microsoft\EdgeUpdate\Clients\$client",
  "HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\$client",
  "HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\$client"
)
$webViewVersion = $null
foreach ($path in $registryPaths) {
  if (Test-Path $path) {
    $candidate = (Get-ItemProperty -Path $path -Name pv -ErrorAction SilentlyContinue).pv
    if ($candidate -and $candidate -ne '0.0.0.0') {
      $webViewVersion = $candidate
      break
    }
  }
}

$hashes = Get-ChildItem -LiteralPath $bundlePath -Recurse -File |
  Sort-Object FullName |
  ForEach-Object {
    [ordered]@{
      path = $_.FullName.Substring($bundlePath.Length + 1).Replace('\', '/')
      sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    }
  }

[ordered]@{
  schemaVersion = 1
  bundle = $bundlePath
  signatureStatus = $signature.Status.ToString()
  webView2Version = $webViewVersion
  files = $hashes
} | ConvertTo-Json -Depth 4

if (-not $webViewVersion) {
  throw 'WebView2 Evergreen Runtime was not detected in an official registry location'
}
