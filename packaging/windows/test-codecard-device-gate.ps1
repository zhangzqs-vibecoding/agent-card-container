param()

$ErrorActionPreference = 'Stop'
$gate = Join-Path $PSScriptRoot 'run-codecard-device-gate.ps1'
$workspace = Join-Path ([IO.Path]::GetTempPath()) "agent-card-codecard-gate-$([guid]::NewGuid().ToString('N'))"
$commit = 'abcdef0123456789abcdef0123456789abcdef01'

function Write-Evidence([string]$Path, [string]$ExecutableHash) {
  [ordered]@{
    schemaVersion = 1
    platform = 'windows'
    commit = $commit
    durationSeconds = 300
    host = [ordered]@{ os = 'Windows 11 test'; displayScalePercent = 150 }
    runtime = [ordered]@{ flutter = '3.32.8'; webView2 = 'fixture' }
    artifact = [ordered]@{ desktopExeSha256 = $ExecutableHash }
    scenarios = [ordered]@{
      webView2Embedded = 'PASS'
      randomLocalhostOrigin = 'PASS'
      originStorageIsolation = 'PASS'
      localRpc = 'PASS'
      offlineRestart = 'PASS'
      workspaceSurface = 'PASS'
      detachedWindow = 'PASS'
      overlaySurface = 'PASS'
      chineseIme = 'PASS'
      mixedDpi150 = 'PASS'
      webViewCrashIsolation = 'PASS'
      tamperedArtifactRejected = 'PASS'
      privateNetworkRejected = 'PASS'
    }
  } | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $Path -Encoding utf8
}

function Invoke-Gate([string]$Evidence, [string]$Bundle, [string]$Hash, [int]$Timeout = 900) {
  $script:GateOutput = & pwsh -NoProfile -File $gate `
    -Evidence $Evidence `
    -Bundle $Bundle `
    -ExpectedCommit $commit `
    -ExpectedExecutableSHA256 $Hash `
    -TimeoutSeconds $Timeout `
    -ValidateOnly 2>&1 | Out-String
  return $LASTEXITCODE
}

function Assert-Fails([scriptblock]$Mutation) {
  Write-Evidence $evidence $hash
  & $Mutation
  if ((Invoke-Gate $evidence $bundle $hash) -eq 0) {
    throw "Expected CodeCard device gate to reject fixture`n$script:GateOutput"
  }
}

try {
  New-Item -ItemType Directory -Path $workspace -Force | Out-Null
  $bundle = Join-Path $workspace 'bundle'
  New-Item -ItemType Directory -Path $bundle -Force | Out-Null
  $executable = Join-Path $bundle 'agent_card_desktop.exe'
  Set-Content -LiteralPath $executable -Value 'executable fixture' -NoNewline
  $hash = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash.ToLowerInvariant()
  $evidence = Join-Path $workspace 'evidence.json'

  Write-Evidence $evidence $hash
  if ((Invoke-Gate $evidence $bundle $hash) -ne 0) {
    throw "Expected CodeCard device gate fixture to pass`n$script:GateOutput"
  }
  Assert-Fails {
    $document = Get-Content -LiteralPath $evidence -Raw | ConvertFrom-Json
    $document.commit = '0000000000000000000000000000000000000000'
    $document | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $evidence
  }
  Assert-Fails {
    $document = Get-Content -LiteralPath $evidence -Raw | ConvertFrom-Json
    $document.artifact.desktopExeSha256 = '0' * 64
    $document | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $evidence
  }
  Assert-Fails {
    $document = Get-Content -LiteralPath $evidence -Raw | ConvertFrom-Json
    $document.scenarios.offlineRestart = 'NOT RUN'
    $document | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $evidence
  }
  Assert-Fails {
    $document = Get-Content -LiteralPath $evidence -Raw | ConvertFrom-Json
    $document.durationSeconds = 901
    $document | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $evidence
  }
  Assert-Fails {
    $document = Get-Content -LiteralPath $evidence -Raw | ConvertFrom-Json
    $document | Add-Member -NotePropertyName note -NotePropertyValue 'AGENTCARD_MODEL_API_KEY=forbidden'
    $document | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $evidence
  }
  Write-Output 'CodeCard Windows device gate tests passed'
} finally {
  Remove-Item -LiteralPath $workspace -Recurse -Force -ErrorAction SilentlyContinue
}
