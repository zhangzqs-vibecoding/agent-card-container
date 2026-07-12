# Desktop performance baseline evidence

Status: **NOT RUN**  
Commit: `<full commit SHA>`  
Recorded at (UTC): `<ISO-8601>`

## Reference host

- OS/build: Windows 11 x64 `<build>`
- Logical cores: 8
- Memory: 16 GiB
- Storage: NVMe
- GPU: integrated `<model>`
- Display scale: 150%
- WebView2 Runtime: `<version>`
- Flutter: `<version>`

## Required raw scenarios

Store the raw machine-readable samples beside this record and evaluate them with:

```powershell
dart tooling/performance/summarize.dart docs/verification/performance-baseline.json
```

| Scenario | Required population | Budget | Result |
|---|---:|---:|---|
| Cold startup to interactive workspace | at least 20 clean launches | P50 ≤ 3000 ms, P95 ≤ 5000 ms | NOT RUN |
| Cached NativeCard mount to first frame | at least 30 mounts | P50 ≤ 300 ms | NOT RUN |
| Cached CodeCard mount to first frame | at least 20 mounts | P50 ≤ 1500 ms | NOT RUN |
| Steady CPU | 10 NativeCards + 3 CodeCards, 60 seconds | median < 3% | NOT RUN |
| Resident memory | same population | maximum < 1024 MiB | NOT RUN |
| Overlay drag/resize frame time | at least 1000 frames | P95 ≤ 16.7 ms | NOT RUN |

This template is not evidence of a pass. Replace every marker only with output captured on the reference Windows device for the recorded commit.
