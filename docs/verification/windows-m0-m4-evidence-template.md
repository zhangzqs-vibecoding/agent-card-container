# Windows M0/M4 device evidence

Status: **NOT RUN**  
Commit: `<full commit SHA>`

Run `tooling/device-gates/windows-m0.ps1` on Windows 11 x64. The JSON evidence must contain the exact commit, host OS/build, CPU, memory, GPU, display configuration, Flutter and WebView2 versions, plus `PASS` for every scenario enforced by `validate-evidence.dart`.

Manual observations, screenshots and raw performance samples should be stored with the JSON. This template and a CI build alone do not constitute a pass.

Authoritative WebView2 deployment and detection guidance: <https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution>
