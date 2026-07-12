# Windows release packaging

The Windows release is fail-closed: a bundle is not releasable until its native dependencies, Authenticode signature and WebView2 device prerequisite have been verified.

## Inputs

- Flutter x64 release bundle from `apps/desktop/build/windows/x64/runner/Release`.
- `sqlite3.dll` and `libsodium.dll` built for x64 from approved, pinned sources. Do not download DLLs at installer runtime and do not commit them to this repository.
- A protected code-signing identity available only in the release environment.
- Inno Setup 6 (`ISCC.exe`).

## Release procedure

```powershell
flutter build windows --release --build-name $Version
signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 apps\desktop\build\windows\x64\runner\Release\agent_card_desktop.exe
pwsh packaging\windows\verify-release.ps1 -Bundle apps\desktop\build\windows\x64\runner\Release -RequireSignature
ISCC.exe /DAppVersion=$Version /DBundleDir="$PWD\apps\desktop\build\windows\x64\runner\Release" packaging\windows\agent-card-container.iss
```

Sign the resulting installer and verify its Authenticode signature before publishing. Store the JSON manifest printed by `verify-release.ps1`, installer SHA-256 and signing certificate subject with the release evidence.

## Portable ZIP

CI snapshots and tagged prereleases use the same fail-closed packager:

```powershell
pwsh packaging\windows\build-portable-package.ps1 `
  -Bundle apps\desktop\build\windows\x64\runner\Release `
  -OutputDirectory dist `
  -Version 0.1.0 `
  -Commit (git rev-parse HEAD)
```

The resulting ZIP contains the complete Flutter bundle, approved SQLite and
libsodium DLLs, startup guidance and a per-file SHA-256 manifest. The packager
extracts and verifies its own output before returning success. Add
`-RequireSignature` only after CI has applied a protected Authenticode identity;
unsigned snapshots are runnable but may trigger Windows SmartScreen.

The installer uses a stable AppId for upgrades and a per-user installation. Uninstall removes application binaries and shortcuts. User-generated cards and settings under the application-data directory are deliberately preserved; deletion must be an explicit user action because uninstall must not silently destroy user content.

WebView2 detection follows Microsoft's documented Evergreen Runtime `pv` registry values for per-user and per-machine installations. If absent, the installer opens Microsoft's official WebView2 download page and aborts; it never downloads or executes an unverified bootstrapper itself.
