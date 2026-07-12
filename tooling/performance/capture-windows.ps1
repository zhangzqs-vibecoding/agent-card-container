param(
  [Parameter(Mandatory = $true)]
  [string]$Bundle,
  [string]$Output = "docs/verification/performance-baseline.json"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not $IsWindows) { throw "Windows performance capture requires Windows" }
$repository = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$bundlePath = (Resolve-Path $Bundle).Path
$executable = Join-Path $bundlePath "agent_card_desktop.exe"
if (-not (Test-Path $executable -PathType Leaf)) {
  throw "Release executable is missing: $executable"
}
$commit = (& git -C $repository rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[0-9a-f]{40}$') {
  throw "Unable to resolve the tested commit"
}

$coldStartupMs = @()
for ($sample = 0; $sample -lt 20; $sample++) {
  $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
  $process = Start-Process -FilePath $executable -PassThru
  try {
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while ($true) {
      $process.Refresh()
      if ($process.HasExited) { throw "Desktop exited during cold startup" }
      if ($process.MainWindowHandle -ne 0) { break }
      if ([DateTime]::UtcNow -ge $deadline) {
        throw "Desktop did not expose its main window within 30 seconds"
      }
      Start-Sleep -Milliseconds 25
    }
    $stopwatch.Stop()
    $coldStartupMs += [Math]::Max(0.001, $stopwatch.Elapsed.TotalMilliseconds)
  } finally {
    if (-not $process.HasExited) {
      Stop-Process -Id $process.Id -Force
      $process.WaitForExit()
    }
  }
}

$coldJson = ConvertTo-Json -Compress -InputObject $coldStartupMs
$coldBase64 = [Convert]::ToBase64String(
  [Text.Encoding]::UTF8.GetBytes($coldJson)
)
$desktop = Join-Path $repository "apps/desktop"
$arguments = @(
  "test",
  "integration_test/performance_baseline_test.dart",
  "-d", "windows",
  "--profile",
  "--dart-define=AGENT_CARD_PERFORMANCE_CAPTURE=true",
  "--dart-define=GIT_COMMIT=$commit",
  "--dart-define=COLD_STARTUP_SAMPLES_BASE64=$coldBase64"
)
Push-Location $desktop
try {
  $testOutput = @(& flutter @arguments 2>&1 | ForEach-Object { "$_" })
  $testExitCode = $LASTEXITCODE
} finally {
  Pop-Location
}
if ($testExitCode -ne 0) {
  $testOutput | Write-Error
  throw "Performance integration test failed with exit code $testExitCode"
}
$prefix = "AGENT_CARD_PERFORMANCE_JSON="
$line = $testOutput | Where-Object { $_.Contains($prefix) } | Select-Object -Last 1
if ($null -eq $line) { throw "Performance JSON marker is missing" }
$json = $line.Substring($line.IndexOf($prefix) + $prefix.Length)
$decoded = $json | ConvertFrom-Json
if ($decoded.commit -ne $commit) { throw "Performance commit does not match" }

$outputPath = if ([IO.Path]::IsPathRooted($Output)) {
  $Output
} else {
  Join-Path $repository $Output
}
$outputDirectory = Split-Path -Parent $outputPath
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
[IO.File]::WriteAllText(
  $outputPath,
  ($decoded | ConvertTo-Json -Depth 20),
  [Text.UTF8Encoding]::new($false)
)

& dart (Join-Path $repository "tooling/performance/summarize.dart") $outputPath
if ($LASTEXITCODE -ne 0) { throw "Performance budgets did not pass" }
Write-Host "Performance evidence written to $outputPath"
