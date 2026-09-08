# CI

В этом файле фиксируются запущенные, но ещё не проверенные прогоны CI. Перед
коммитом в `main` список незавершённых прогонов проверяется.

## Незавершённые прогоны

Нет.

## Последний проверенный прогон

- `34194464508` — commit `561da80e197d14522af3093e08fadc1e8916eafe`; завершён
  с ошибкой в двух независимых shuffled-проверках: `Race (cmd/f4 rest)` —
  `TestMacKeysSkipsCommandRulesWithoutTheChannelSplit`, `Test (linux/amd64)` —
  `TestMainMenuFilePath_HasExpectedSuffix`. Clipboard race из предыдущего
  прогона этим результатом не подтверждена; follow-up исправляет оставшийся
  такой же асинхронный helper `waitForMarkedClipboard`.
