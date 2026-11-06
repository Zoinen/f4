# Cached pixel grid — проверка и измерения, 2026-10-17

Ветка обоих репозиториев: `zoin_branch/cached-pixel-grid`.
Checkpoint сохранён: parent `dc35c5eef9650fcd74c387f341d1254122ee907a`,
ZoinGallery `2dcba89294d62f00151f4315027a7dcf3f2be9b9`.
Новые изменения не закоммичены и не отправлены.

## Реализация

- Общий `PixelGridContext` в ZoinGallery; host `ScenePixelAlignment` использует
  тот же context. Постоянное дерево зависимостей вместо обхода и сортировки
  всех листьев на каждом кадре. Счётчики включаются только для диагностики.
- `GridMetrics`: физические целочисленные границы, общие границы колонок,
  вместимость с округлением вверх, центрирование на соседний целый пиксель.
  Masonry и остальные layouts кешируют опубликованную геометрию.
- `PixelGridScope` переносит scrolling одним transform. Qt-owned contentItem
  получает attached adapter без замены Flickable/ListView или изменения layout.
- Wheel retarget сохраняет одну motion session. Caption renderType остаётся
  выбранным пользователем; native glyph pixel steps при движении допустимы.
- Общий adapter охватывает controls, dialogs/settings/menus, breadcrumbs,
  tabs/status, document/terminal rows и viewer/video chrome. SVG paths и
  media zoom/rotation/resampling остаются отдельными операциями.
- Подробный контракт: `docs/PIXEL_GRID.md`.

## Измерения

Активное окно, реальный DPR 1.75, экран 160 Hz, viewport 2194×1186 logical,
Downloads, 183 entries, density 152. Wheel: 120 delta каждые 30 ms,
900 ms в каждую сторону. Middle: смещение 296 logical, примерно четверть
высоты экрана. UI Automation observer во время записи не запрашивался.

| Показатель | Checkpoint | Cached grid |
|---|---:|---:|
| Wheel handler median | 15.143 ms | 0.339 ms |
| Wheel handler p95 | 18.530 ms | 0.796 ms |
| Wheel handler max | 24.694 ms | 2.105 ms |
| Wheel events / motion-state changes | 54 / 108 | 59 / 4 |
| Caption measurements во время движения | 0 | 0 |

Это не строгий end-to-end A/B: текущий scroll extent 3710.14 отличается от
6194.22 в старом capture, хотя папка, число entries, density и viewport
совпадают. Сравнение обработчика и количества переходов подтверждает удаление
повторных остановок/финализации; различие layout/cache ограничивает сравнение
полного frame time.

| Кадры Qt | Median | p95 | p99 | Max |
|---|---:|---:|---:|---:|
| Wheel | 6.210 ms | 8.249 ms | 16.064 ms | 31.022 ms |
| Middle, четверть экрана | 6.234 ms | 7.774 ms | 14.080 ms | 18.658 ms |

PresentMon: 267/267 wheel presents и 273/273 middle presents показаны,
без dropped presents в этих двух записях. Это **не** доказательство идеальных
160 кадров каждую секунду: p95/p99 всё ещё превышают бюджет 6.25 ms.
Нельзя приписать оставшиеся выбросы SVG или compositor без отдельного
коррелированного trace. Исторические SVG stalls описаны в checkpoint report;
новая запись сама по себе не доказывает их причиной каждого выброса.

Первую запись при inactive window/DPR 2.25 и запись на portrait 120 Hz
исключили из основного сравнения. Повторная запись выполнена на экране 160 Hz.
Native harness и PresentMon использованы вместо недоступного qmlprofiler CLI;
новый `.qtd` trace не заявляется.

## Регрессии

| Набор | Результат | Локальный лог |
|---|---|---|
| Layout engine, все стратегии и physical edges | 26 passed | `artifacts/pixel-grid-layout-final.txt` |
| Native panel modes, wheel/middle, captions, groups, navigation, leaves | 44 passed | `artifacts/pixel-grid-native-final.txt` |
| Software panel modes, wheel/middle/alignment | 19 passed | `artifacts/pixel-grid-software-regressions.txt` |
| Native grid context, Qt-owned content, renderer stability | 6 passed | `artifacts/pixel-grid-content-rhi-175.txt` |
| Host controls/chrome/path/history/video/import isolation | 17 passed | `artifacts/pixel-grid-host-final.txt` |
| Document/terminal grid, selection, fractional wheel | 8 passed | `artifacts/pixel-grid-document-final.txt` |
| Реальные листья при DPR 1/1.25/1.5/2 | все passed | `artifacts/pixel-grid-leaves-*.txt` |

200 кадров retained motion: **0 alignment visits, 0 scene maps,
0 topology changes**. Проверены Text/TextInput/TextEdit/Image, unit transforms,
reparent, hide/change/show, перемещение предка и возвращение в resting state.
Qt-owned Flickable: ни одного изменения correction у retained текста во время
кинетики; не больше двух boundary visits при завершении.

Captured output `pixel-grid-cached-leaves-175.png` просмотрен: видны маленькая
подпись, TextInput, две строки TextEdit, включая secondary label, и raster image.
Проверки mode/chrome также захватывают настоящее rendered output.

Подтверждённые сбои перед исправлениями:
- cached layout edge: 12.425 physical pixels;
- group separator после ancestor move: 0.350 physical pixels;
- Columns terminal offset: 6308.5 вместо 6308.571;
- Details keyboard reveal использовал requested density вместо physical pitch.
Они закрыты регрессионными проверками. Некоторые старые проверки читали layout
сразу после mutation; они теперь ждут settled frame перед scene-space assertion.
Никакие assertions позиции/единичных векторов не удалены.

Полный suite не объявляется зелёным: checkpoint содержал отдельные Icons reorder
и Details latency failures; здесь запускались перечисленные целевые наборы.
Обычный CMake test link на Windows сохраняет старую проблему длинного rsp;
тесты связаны теми же objects/libraries через response file с одним аргументом
на строку (`artifacts/link-scene-cache-test.py`). Production host link проходит.

## Portable build и запуск

- DLL import audit: только Windows system ABI, static MSVC/Qt.
- `qmlImportsWithoutInstalledQt` и `compiledHostLoadsItsQmlModule`: passed.
- Payload пересоздан, Go собран с `CGO_ENABLED=0`, системным GOCACHE и
  `f4_embedded_qt_host`. Embedded payload/materialization/preference tests passed.
- Запущен `D:/Code/f4-zoin/f4-zoin.exe --gui=qt`, без F4_EXT_UI_PATH и без scale
  overrides. Завершался только process tree этого worktree.
- Runtime branch title проверен: `f4 [zoin_branch/cached-pixel-grid]`.
- Go SHA256: `c4c35bf1b7c2eddd578d04e476edd533c78cc02f8d65a0cac25f52022e96ee97`.
- Built и фактически запущенный extracted Qt host SHA256 совпадают:
  `9e1b5bc8c41221bd52d419b51c3ce40588fe1b65b036d1fafe4b3a9fedfa1278`.
- Cache key: `3d06a166c4e30b20a2d1eb39c1ef5d10a83bd72420e91e894154855f8414ad5b`.
- stderr запуска пуст. Проверка Linux/macOS CI в этой Windows-сессии не заявляется.

## Контекстные gates

Architecture: PASS — изменения принадлежат Qt presentation и ZoinGallery,
Go layers и wire protocol не менялись. Rules: обязательные pixel-grid,
portable-build и worktree rules проверены. Новых настроек/Qt dependencies нет.
`git diff --check` passed в обоих репозиториях.

WARN [roadmap] `.ai-factory/ROADMAP.md` отсутствует, явной milestone linkage нет.
WARN [rules] optional `.ai-factory/RULES.md` отсутствует; применены AGENTS и base rules.
WARN [performance] строгая гарантия 160 Hz по всем кадрам не достигнута;
остаточные выбросы сохранены в измерениях, без выдуманной атрибуции.

```aif-gate-result
{
  "schema_version": 1,
  "gate": "verify",
  "status": "warn",
  "blocking": false,
  "blockers": [],
  "affected_files": ["third_party/ZoinGallery/include/ZoinGallery/PixelGrid.h", "third_party/ZoinGallery/src/pixelgrid/PixelGrid.cpp", "qt/host/src/ScenePixelAlignment.cpp", "docs/PIXEL_GRID.md"],
  "suggested_next": {"command": null, "reason": "Целевые проверки пройдены; остаточные frame-time выбросы требуют отдельного профилирования."}
}
```
