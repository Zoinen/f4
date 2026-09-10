# Coverage: plugins/ios

- Baseline: f4 `main` after the wincon coverage merge, Codecov project
  coverage 60.86%.
- Package baseline: `plugins/ios` at 37.09% (2203 lines, 11 files).
- This change adds tests for application metadata/group normalization and
  native device metadata/error classification without requiring an iOS device.
- Local Go builds and tests are intentionally not run; CI is authoritative.
