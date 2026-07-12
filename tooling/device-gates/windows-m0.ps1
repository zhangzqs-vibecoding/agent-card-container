param(
  [Parameter(Mandatory = $true)]
  [string]$Evidence,
  [Parameter(Mandatory = $true)]
  [string]$Bundle,
  [switch]$RequireSignature
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$commit = (git -C $root rev-parse HEAD).Trim()

Push-Location (Join-Path $root 'apps\desktop')
try {
  flutter analyze
  flutter test
  flutter build windows --release
} finally {
  Pop-Location
}

$verifyArgs = @(
  '-File', (Join-Path $root 'packaging\windows\verify-release.ps1'),
  '-Bundle', $Bundle
)
if ($RequireSignature) { $verifyArgs += '-RequireSignature' }
& pwsh @verifyArgs
if ($LASTEXITCODE -ne 0) { throw 'release verification failed' }

dart (Join-Path $root 'tooling\device-gates\validate-evidence.dart') `
  --platform windows `
  --commit $commit `
  --evidence $Evidence
if ($LASTEXITCODE -ne 0) { throw 'Windows device evidence is incomplete' }
