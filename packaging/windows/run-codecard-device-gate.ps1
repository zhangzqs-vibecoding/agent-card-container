param(
  [Parameter(Mandatory = $true)]
  [string]$Evidence,
  [Parameter(Mandatory = $true)]
  [string]$Bundle,
  [Parameter(Mandatory = $true)]
  [string]$ExpectedCommit,
  [Parameter(Mandatory = $true)]
  [string]$ExpectedExecutableSHA256,
  [ValidateRange(60, 3600)]
  [int]$TimeoutSeconds = 900,
  [switch]$RequireSignature,
  [switch]$ValidateOnly
)

$ErrorActionPreference = 'Stop'
$requiredScenarios = @(
  'webView2Embedded',
  'randomLocalhostOrigin',
  'originStorageIsolation',
  'localRpc',
  'offlineRestart',
  'workspaceSurface',
  'detachedWindow',
  'overlaySurface',
  'chineseIme',
  'mixedDpi150',
  'webViewCrashIsolation',
  'tamperedArtifactRejected',
  'privateNetworkRejected'
)

if ($ExpectedCommit -cnotmatch '^[0-9a-f]{40}$') {
  throw 'ExpectedCommit must be a lowercase full 40-character SHA'
}
if ($ExpectedExecutableSHA256 -cnotmatch '^[0-9a-f]{64}$') {
  throw 'ExpectedExecutableSHA256 must be a lowercase SHA-256 digest'
}
$bundlePath = (Resolve-Path -LiteralPath $Bundle).Path
$evidencePath = (Resolve-Path -LiteralPath $Evidence).Path
$executable = Join-Path $bundlePath 'agent_card_desktop.exe'
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
  throw 'Desktop executable is missing from the bundle'
}
$actualHash = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualHash -cne $ExpectedExecutableSHA256) {
  throw 'Desktop executable SHA-256 does not match the expected digest'
}

if (-not $ValidateOnly) {
  if (-not $IsWindows) {
    throw 'CodeCard device execution requires Windows'
  }
  $verifyArguments = @(
    '-NoProfile',
    '-File', (Join-Path $PSScriptRoot 'verify-release.ps1'),
    '-Bundle', $bundlePath
  )
  if ($RequireSignature) { $verifyArguments += '-RequireSignature' }
  & pwsh @verifyArguments
  if ($LASTEXITCODE -ne 0) {
    throw 'Windows release verification failed'
  }
}

$evidenceText = Get-Content -LiteralPath $evidencePath -Raw
if ($evidenceText -match '(?i)(sk-[a-z0-9]{20,}|authorization|api[_-]?key|bearer\s)') {
  throw 'Device evidence contains forbidden credential-like material'
}
$document = $evidenceText | ConvertFrom-Json
if ($document.schemaVersion -ne 1 -or
    $document.platform -cne 'windows' -or
    $document.commit -cne $ExpectedCommit -or
    $document.durationSeconds -isnot [long] -and $document.durationSeconds -isnot [int]) {
  throw 'CodeCard device evidence envelope is invalid'
}
if ($document.durationSeconds -lt 1 -or $document.durationSeconds -gt $TimeoutSeconds) {
  throw 'CodeCard device evidence duration is outside the allowed timeout'
}
if ($document.host -isnot [pscustomobject] -or
    [string]::IsNullOrWhiteSpace($document.host.os) -or
    $document.host.displayScalePercent -isnot [long] -and
    $document.host.displayScalePercent -isnot [int] -and
    $document.host.displayScalePercent -isnot [double]) {
  throw 'CodeCard device host metadata is invalid'
}
if ($document.runtime -isnot [pscustomobject] -or
    [string]::IsNullOrWhiteSpace($document.runtime.flutter) -or
    [string]::IsNullOrWhiteSpace($document.runtime.webView2)) {
  throw 'CodeCard device runtime metadata is invalid'
}
if ($document.artifact -isnot [pscustomobject] -or
    $document.artifact.desktopExeSha256 -cne $ExpectedExecutableSHA256) {
  throw 'CodeCard device artifact metadata is invalid'
}
if ($document.scenarios -isnot [pscustomobject]) {
  throw 'CodeCard device scenarios are missing'
}
$failed = @($requiredScenarios | Where-Object {
  $property = $document.scenarios.PSObject.Properties[$_]
  $null -eq $property -or $property.Value -cne 'PASS'
})
if ($failed.Count -gt 0) {
  throw "Mandatory CodeCard device scenarios are missing or not PASS: $($failed -join ', ')"
}

[ordered]@{
  schemaVersion = 1
  status = 'PASS'
  commit = $ExpectedCommit
  desktopExeSha256 = $ExpectedExecutableSHA256
  webView2Version = $document.runtime.webView2
  durationSeconds = $document.durationSeconds
  scenarioCount = $requiredScenarios.Count
} | ConvertTo-Json -Depth 3
