# Native gate audit

Статус на 2026-08-31: полный native gate пройден на pinned OpenConsole.
Все активные разделы A–D закрыты; ниже перечислены измерения и ограничения
области применимости.

### Актуальная сводка (последняя сверка)

Последующие прогоны закрыли транспорт и побайтовую историю на реальных
командах (включая `dir /s /b` при host-width 256–512), consumer reflow,
произвольное chunking, scrollback eviction, control-поведение tabs/OSC 8,
`Clear-Host`, пустой кадр, progress-bar и 300 deterministic seed-сессий.
Полная C4-матрица (первый prompt, EOF-хвост, broken pipe, cancel, timeout и
варианты порядка закрытия), объединённый partial payload и полный gate теперь
выполнены.

### C4: низкоуровневый lifecycle-прогон

Добавлен отдельный `-lifecycle-probe`, который не запускает reader-горутины
для обычных EOF/timeout-кейсов и отдельно дожидается реального prompt:
он напрямую наблюдает `WaitForSingleObject` у дочернего процесса, проверяет
принудительное завершение при timeout, закрытие output pipe для broken-pipe,
два порядка закрытия (`host-first` и `pipes-first`), завершение pinned host и
освобождение всех нативных handles. Интерактивный кейс дополнительно ждёт
явный prompt-маркер до bounded cancellation.

```text
native lifecycle probe complete: artifacts/pinned-conpty-lifecycle-group.json cases=6

first-prompt   host-first  timeout child=true host=true handles=true prompt=true bytes=164
eof-tail       host-first  exit    child=true host=true handles=true tail=true bytes=125
startup-eof    host-first  exit    child=true host=true handles=true
empty-eof      pipes-first exit    child=true host=true handles=true
cancel-timeout host-first  timeout child=true host=true handles=true
broken-pipe    pipes-first timeout child=true host=true handles=true
```

Проверка выполнена на pinned OpenConsole
`1.12.220408003-release1.12`, SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`.
Актуальный отчёт содержит 6 кейсов; процессы `OpenConsole` и пробника после
прогона отсутствуют. `eof-tail` проверяет, что после последнего маркера
`printableStream` не оставляет байтового хвоста (`tail_clean=true`). Это
закрывает C4 lifecycle-матрицу и пункт 12.2.

### Единый partial payload: 10.2, 12.3, 14.6 и 16

Один native-сеанс на `80x25` подаёт пары строк с ровно 8 и 9 хвостовыми
пробелами до и на нижней строке viewport, включает и выключает мигание вокруг
текста, затем отключает DECAWM на строке из 257 `W` и возвращает его обратно.
Проба проверяет результат рендеринга, а не восстанавливает строки из рядов:

```text
native edge probe complete: artifacts/pinned-conpty-edge-group.json spaces=4 blink=true nowrap_tail_lost=true
spaces_eight_top_elided=true  spaces_nine_top_elided=true
spaces_eight_bottom_advanced=true  spaces_nine_bottom_advanced=true
blink_rendered=true  blink_sequence_in_stream=true  blink_in_rendered_history=false
auto_wrap_host_sequences=0  auto_wrap_tail_lost=true
```

Поля `spaces_*_top_elided` — наблюдательные: они фиксируют отсутствие ровно
заданного хвоста в сыром рендере верхних строк и не объявляют срабатывание ECH.
Порог хоста проверен отдельным static-контролем
`pinned-conpty-probe-static-c2-final.json`: `spaces-eight` пришла с восемью
пробелами, а `spaces-nine` — как `ESC[9X`, что соответствует
`numSpaces > ERASE_CHARACTER_STRING_LENGTH`. В edge-сеансе хвост верхних строк
элиминируется при viewport paint, а на нижнем ряду представлен явным `CSI C`,
что соответствует ветке `_newBottomLine`. Хост действительно может передать
управляющие `CSI ?12` в отдельном участке потока, но они не попадают в
rendered history. Потеря хвоста при отключённом DECAWM ожидаема и зафиксирована
как условие интеграции, не как ошибка накопителя.

### Диагностическое сравнение `--resizeQuirk`

Один и тот же медленный payload запущен дважды при одинаковой серии сужений
и расширений host-resize. Это не возвращает host-resize в рабочий B1-0 путь;
ветка остаётся диагностической. При включённом quirk ветка полной
инвалидизации при сужении не выполняется; число отдельных repaint-фреймов
зависит от тайминга:

```text
native resizeQuirk probe complete: artifacts/pinned-conpty-quirk-group.json without_repaints=5 with_repaints=1
without_quirk: repaint_bytes=572  with_quirk: repaint_bytes=15
```

Обе сессии использовали именно проверенный pinned OpenConsole; child, host и
handles закрылись. Это закрывает пункты 4 и 13.4 как диагностическое
сравнение, не меняя неактивность host-resize в B1-0. Число кадров не является
инвариантом: при повторных запусках оно меняется из-за границ таймерных кадров
(например, `without_repaints=4, with_repaints=6`), поэтому фиксируется факт
сравнения при сужении, а не случайный счётчик.

### Итоговый полный native gate

Команда `artifacts/pinned-probe.exe -gate -report
artifacts/pinned-conpty-gate-final.json` завершилась с кодом 0 на pinned
OpenConsole `1.12.220408003-release1.12`, SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`.
Все активные стадии A–D прошли: static, consumer reflow, command suite и
`dir /s /b` (`mismatches=0`, 23 906 строк), clear, scrollback 37/37, empty,
lifecycle 6/6, edge payload, диагностический quirk, semantic tabs/link/
progress/unicode и `seed_count=300`, `sessions=300`, `consumer_checks=1500`,
`failures=0`. После прогона pinned host и probe завершены; raw-артефакты
проверены побайтово и по SHA при записи.

## Что подтверждено командой

После синхронизации `git fetch && git checkout main && git pull` дерево было
на `4d535ac8` плюс рабочие изменения этого шага. Команда

```text
go test ./...
```

в `tools/conptyreconcile` прошла после удаления старого mock/grid-пути. Кэш
использован системный (`go env GOCACHE`), без task-local замены.

Новые unit-тесты проверяют, что поток режется только по явному `LF`, что
`CRLF` сохраняется как терминатор, что одинаковые строки не сливаются, и что
перевыкладка целой строки по ширине не изменяет её байты. Это не native
доказательство поведения хоста.

Реальный static-прогон после исправления точного ProductVersion:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static.json
native OpenConsole probe complete: ../../artifacts/pinned-conpty-probe-static.json
```

Проверка артефакта командой дала `raw_output=1324`, `.raw=1324`,
`equal=True`, SHA-256 `e03681a92f5c386d6ec8083c58e588dc02e7eb8c15c1d646877dac2813818780`.
В отчёте зафиксированы оба маркера ровно по одному и в правильном порядке.
Это закрывает только транспортную запись static-прогона; логическая история
и reflow ещё не сверены.

Dynamic-прогон командой

```text
go run . -probe -report ../../artifacts/pinned-conpty-probe.json
```

показал в сыром потоке повторную отрисовку begin-маркера при resize (в
первой сессии было 2 вхождения) и перестановку raw end-маркера в сессии
`121x40`. Это repaint-байты ConPTY; строгая проверка ровно одного маркера и
порядка должна выполняться после восстановления logical history, а не на
сыром потоке. Raw-слой сохраняет оба факта как предупреждения.

После перевода raw reorder в предупреждения dynamic-прогон завершился:
`80x25` — 1351 байт, 4 resize, 1 repaint warning; `1x1` — 1361 байт, 4
resize, 1 repaint warning; `121x40` — 1381 байт, 4 resize, без warning. Для
всех трёх сессий проверка `len(base64decode(raw_output)) == len(.raw)` и
побайтовое равенство дала `True`. Это подтверждает только native transport,
resize и запись артефактов; logical history/reflow пока не доказаны.

Эксперимент с control-stream parser остановлен и удалён: попытка обработать
`CSI`-перемещение как продолжение записи потребовала бы выводить границу из
экранной позиции. Это прямо запрещённая эвристика. Поэтому A/B остаются
открытыми до появления источника логических событий, где граница задана самим
потоком `LF`, а не восстановлена из repaint.

Команда `go run . -gate` сейчас намеренно завершается ошибкой после native
static/dynamic стадий с сообщением `native gate incomplete ...`: это защитный
гейт от ложной зелени, пока не реализованы проверки A–D.

## Наблюдение: поток пиннутого хоста в нынешнем f4 (не критерий гейта)

Измерение выполнено разово тестом, который читал артефакт пробника и подавал
его через `PanelsFrame.consumeLocalOutput` и `AnsiParser` в `TerminalView`.
Тест удалён: код гейта в `cmd/f4` не живёт (см. запрет в начале
`CONPTY_GATE_REQUIREMENTS.md`). Он появился потому, что пункт D1 требовал
прогона «через настоящий путь f4» — формулировка от старой постановки,
противоречившая запрету; D1 переписан.

Результат сохранён как наблюдение о **текущем fallback**, а не как результат
гейта.

Команда на Windows:

```text
F4_NATIVE_CONPTY_REPLAY=<absolute path to pinned-conpty-probe-static-v6.json> go test ./cmd/f4 -run '^TestNativeConPTYReplay$' -count=1 -v
```

Прогон получил `raw=2852`, `f4_log=1293`, `begin=1`, `end=1`, но завершился
первым строгим несоответствием payload: `actual=1236`, `expected=1326`,
`first_diff=1`. В фактическом логе присутствуют две лишние начальные пустые
строки, а строка cursor/rewrite также имеет изменённое содержимое. Это
первое прямое измерение потери/перестановки на пути f4; оно оставлено
красным и не маскируется удалением control-последовательностей.

## Текущая реализация

- модуль инструмента самостоятельный: `github.com/unxed/pinned-conpty-probe`;
- маркеры: `__PINNED_CONPTY_PROBE_BEGIN__` и
  `__PINNED_CONPTY_PROBE_END__`;
- OSC-title: `pinned-conpty-probe`;
- кэш: `%LOCALAPPDATA%\pinned-conpty\1.12.10983.0\`;
- проверка личности требует именно пиннутый `OpenConsole.exe` из
  `docs/PINNED_CONSOLE.md` и проверяет живой процесс;
- старый mock/grid-код и неиспользуемый сценарный runner удалены/исключены;
  текущий Windows fallback в основном проекте не менялся.

## Ограничения области применимости

Проверка не распространяется на полноэкранные программы, которым нужен
реальный host-resize, и на дочерний процесс, отключивший автоперенос. В рабочем
B1-0 размер ConPTY не меняется ради reflow; отображение перевыкладывает целые
логические строки самостоятельно. Строки при отключённом ребёнком DECAWM могут
терять хвост — это ограничение режима, не дефект накопителя.

## Сохранённое расхождение артефактов

Для ранее созданного `native-openconsole-probe-static.json` проверка показала:
`base64decode(raw_output)` — 957 байт с 49 `CR`, файл `.raw` — 908 байт с нулём
`CR`; поэтому прежняя запись о совпадении была ошибочной. Хэш совпадал с
`raw_output`, но не с файлом на диске. Критерий закрытия остаётся строгим:
`sha256(file) == raw_sha256` и побайтовое равенство `file` и
`base64decode(raw_output)`, проверенные командой.

Прежние наблюдения о 46/211 и зависании статического `1x1` сохранены как
диагностические факты удалённого пути; они не являются доказательством
текущего native-гейта.

## Новый host-stream прогон

### Частичная строка при resize

После проверки исходника пиннутого хоста (`state.cpp:248-264`,
`renderer.cpp:107-157,668-745`, `XtermEngine.cpp:410-445`) выполнен отдельный
прогон:

```text
go run . -partial -report ../../artifacts/pinned-conpty-partial-current.json
```

Сессия завершилась с `exit_code=0`; проверка отчёта получила `raw=293` байта,
`logical_lines=1`, `frames=0`, один явный `CRLF`, а собственные границы
resize — `output_offset=0,0,8,162`. Полный `expected_input` найден в сыром
потоке побайтово (`expected=131`, `exact_raw=True`), включая хвост строки
после resize. Это подтверждает транспортный случай «половина строки → resize
→ хвост», но пока не доказывает, что отдельный накопитель истории не примет
repaint за новую строку: такой накопитель в standalone-гейте ещё не реализован.

Свежий static-прогон после синхронизации `main` выполнен командой:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static-current.json
```

Он завершился успешно. Скриптовая проверка отчёта и `.raw` получила:
`decoded=2891`, `raw=2891`, `bytes_equal=True`, `decoded_cr=92`,
`raw_cr=92`, `sha_equal=True`; SHA файла и `raw_sha256` равны
`7dc18add97587c6349c7887a86f7ac104548cc5b278d1ba4dabab12d75a71d3f`.
Это повторно закрывает только целостность транспортного артефакта D4 для
этой сессии; проверки A--C и путь f4 этим прогоном не закрыты.

### Строгая A-проверка static-потока

В static-прогоне с новым assertion layer (`-probe-static`, отчёт
`pinned-conpty-probe-static-assertions.json`) только `exact-n-minus-1`,
`spaces` и оба маркера получили `passed`. `ascii`, `exact-n`, `exact-n-plus-1`,
`exact-2n-plus-1`, `width-edge`, `repeat-char`, `alternating`, `empty`,
`unicode`, `tabs`, все три `repeat: SAME` и `long` получили `failed` с
`observed_count=2` (для повторов ожидается 3, наблюдается 6); строки с SGR,
erase, cursor и alternate screen получили явный `deferred`. Raw static был
`2891` байт.

Это не ошибка счётчика. В сыром потоке после `alternate-begin` host повторно
выдаёт primary-содержимое, но управляющая последовательность `?1049l` наружу
не попадает; после repaint те же plain-строки встречаются второй раз, а
`long:` представлен физическими фрагментами. Источник по пиновке описывает
рендеринг dirty-area (`Renderer::PaintFrame`, `renderer.cpp:107-157,668-745`),
но не предоставляет в ConPTY pipe границу окончания такого кадра. Поэтому
дедупликация по содержимому, склейка по ширине или удаление повторной строки
были бы запрещёнными эвристиками.

**Законного события не существует** — см. исправленный пункт 6 и новый пункт 8
в `PINNED_HOST_FACTS.md`. Ранее здесь утверждалось обратное: движок сам
обрамляет кадр парой `ESC[?25l` … `ESC[?25h` (`XtermEngine::EndPaint` вставляет
открывающую скобку в начало буфера кадра). На этом же артефакте все три
повторные выдачи primary лежат внутри скобок, а вне скобок ни одна строка не
повторяется. A1/A2/A3 остаются открыты до реализации накопителя, который
игнорирует внутрискобочный поток, и до подтверждения краевых случаев скобки на
живом хосте.

### Произвольные границы чтения (C2)

В native static-прогоне добавлена проверка captured stream тремя
детерминированными разбиениями: `one-byte`, `fixed-7` и PRNG-размеры 1..31.
Все три получили `status=passed`; explicit host-CRLF lines и диагностические
frames побайтово совпали с цельным чтением. Unit-тест отдельно включает разрыв
UTF-8, CSI и CRLF. Это закрывает только транспортную устойчивость chunking;
границы repaint, история, прокрутка и reflow по-прежнему открыты.

### Повтор отдельного seed (D3)

Добавлен явный `-seed` (также `PINNED_CONPTY_SEED`) для повторения одной
сессии с тем же проверенным cache host. Повтор командой
`go run . -seed 21 -report ../../artifacts/pinned-conpty-seed-21-current.json`
завершился с `exit=0`: width `1`, raw `654` байта, два маркера, три
chunking-проверки `passed`. Seed 115 повторён той же командой с raw `921`
байтом, width `121`, двумя маркерами и тремя `passed` chunking-проверками.
Эти повторы не закрывают D3: предыдущий полный прогон 300 seed записал ошибки
21 и 115, а полные assertions истории, экрана и reflow пока отсутствуют.

В том же повторе seed 21 lifecycle-поля получили `child_exited=true`,
`host_exited=true`, `handles_closed=true` (`child_pid=11820`, `host_pid=2888`,
`exit_code=0`). Это закрывает только штатный cleanup одной сессии; отмена,
таймаут, broken pipe, обратные порядки закрытия и отсутствие утечек во всех
300 сессиях ещё не проверены.

После чтения `docs/PINNED_HOST_FACTS.md` и исходника `e9b4e2e` добавлен
разбор, который режет поток только по host-emitted `CRLF`; resize-frame
отмечается собственным output-offset при вызове `ResizePseudoConsole`.
Ни экранная сетка, ни флаг wrap, ни хвостовые пробелы в этом разборе не
используются. Unit-тесты проходят с системным кэшем
`C:\Users\Windows\AppData\Local\go-build`.

Команда:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static-v6.json
native OpenConsole probe complete: ../../artifacts/pinned-conpty-probe-static-v6.json
```

Проверка дала `base64decode(raw_output) == .raw` побайтово (`2852 == 2852`),
`sha256=.raw` совпадает с `raw_sha256`. Static-сессия содержит 89 host-CRLF,
89 записанных logical-line объектов и не содержит resize-frame, что ожидаемо
для режима без resize. Динамический прогон тем же payload завершился для
80x25/1x1/121x40 (2890/1956/3279 байт); на 1x1 сохранено предупреждение о
двух begin-маркерах как о repaint.

Важное наблюдение: в этих бинарных прогонах не обнаружена последовательность
`ESC[8;H;Wt`; вместо неё видны CUP-перемещения (`ESC[...H`). После чтения
`src/host/PtySignalInputThread.cpp:133-146` это ожидаемо: PTY-resize сразу
вызывает `s_SuppressResizeRepaint`, чтобы не эхо-отправлять размер терминалу.
Границы repaint теперь должны отмечаться собственными output-offset в момент
`ResizePseudoConsole`, а содержимое таких кадров не участвует в истории.

Повторный seed-прогон `go run . -seeds -report ../../artifacts/pinned-conpty-seeds-v7.json`
выполнил все 300 сессий и записал 300 `.raw`; две семантические ошибки маркеров
остались в отчёте: seed 21 (width 1, begin) и seed 115 (width 121, end).
Это открытые случаи устойчивости и не доказательство прохождения D3.

Отдельный прогон незавершённой строки:

```text
go run . -partial -report ../../artifacts/pinned-conpty-partial-v1.json
```

завершился успешно. Результат содержит одну host-логическую строку и один
явный `CRLF`; четыре собственных resize-offset записаны (`0,0,8,116`), а
артефакт проверен побайтово и по SHA. Это подтверждает только транспортную
последовательность «половина строки → resize → хвост»; проверка того, что
история не принимает repaint как новую строку, ещё не реализована.

### Разделение primary и alternate static-фаз

После этого наблюдения static-прогон был разделён на две независимые сессии,
чтобы repaint primary при выходе из alternate buffer не смешивался с payload
основного теста:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static-split3.json
```

Проверка выполнена на pinned OpenConsole из отчёта (`version
1.12.220408003-release1.12`, SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`). В
primary-сессии assertions дали `passed=17`, `deferred=5`, `failed=1`; все
три проверки произвольного chunking (`one-byte`, `fixed-7`, `prng`) получили
`passed`. В отдельной alternate-сессии assertions дали `passed=4`,
`deferred=2`, `failed=0`, chunking также `passed` во всех трёх режимах.

Единственный failure primary — `long`: ожидаемая строка не найдена как одна
логическая запись. Это подтверждено побайтовым разбором `raw_output`, а не
визуальным выводом: при ширине 80 внутри 257 символов `C` записаны пары
`CRLF` на смещениях `1375/1376`, `1475/1476`, `1566/1567` и `1600/1601`,
между ними находятся `ESC[24;80H`/`ESC[25;80H`. Внешняя идентичность host и
целостность `.raw` проверены; это не системный conhost и не потеря CR при
записи.

**Механизм объяснён исходником** — см. пункт 11 в `PINNED_HOST_FACTS.md`:
`XtermEngine::PaintCursor` не пропускает отрисовку курсора, когда не выполнены
все три его условия, `_MoveCursor` уходит в абсолютный CUP последней колонки и
сбрасывает `_wrappedRow`/`_delayedEolWrap` (`XtermEngine.cpp:329-330`), после
чего следующий переход выдаёт `CRLF` без знания о переносе. Такой `CRLF` —
потеря состояния хостом, а не граница строки. Первый шаг проверки: payload,
обрамлённый `ESC[?25l`/`ESC[?25h`, при спрятанном курсоре `PaintCursor` не
вызывается вовсе.

Наблюдение пока не превращается в правило склейки: `PINNED_HOST_FACTS.md`
определяет host-emitted `CRLF` как единственный законный источник границ
(`src/renderer/vt/XtermEngine.cpp:236-276`), а repaint относится к dirty-area
рендерингу (`src/renderer/base/renderer.cpp:107-157,668-745`) и не должен
восстанавливаться из рядов. Поэтому `long` остаётся открытым до объяснения
этого конкретного вывода исходником и подтверждения прогоном; дедупликация,
объединение по ширине или удаление повторов были бы запрещёнными эвристиками.

### Текущее состояние: host-emitted repaint brackets

В накопителе `tools/conptyreconcile` добавлено распознавание пар, которые
сам pinned-host вставляет вокруг repaint-кадра: `ESC[?25l` ... `ESC[?25h`.
Содержимое такой пары намеренно полностью исключается из логической истории;
строки вне пары принимаются только по явному host-emitted `CRLF`. Маркеры
обрабатываются и при произвольном разбиении входного потока. Unit-тесты
проверяют пропуск содержимого кадра, разбиение маркеров по байтам и сохранение
frame offsets. Это не дедупликация и не восстановление строк из рядов.

Последний успешный native static-прогон выполнялся на pinned OpenConsole
(`1.12.220408003-release1.12`, SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`) командой:

```text
go run . -probe-static -report ../../artifacts/pinned-conpty-probe-static-phases3.json
```

Для primary-сессии после разделения фаз получены `passed=19`, `failed=0`;
для control-сессии — `passed=4`, `deferred=4`, `failed=0`, один repaint-frame
(`start=266`, `end=682`). Байтовая целостность артефактов проверена отдельно:
`base64decode(raw_output) == .raw` и SHA файла совпадает с `raw_sha256`.
Ранее запущенная alternate-сессия ещё содержала ошибки из-за того, что её
маркеры находились внутри repaint; после переноса маркеров за выход из
alternate buffer повторный native-прогон ещё не выполнен.

Открытые сложности на момент фиксации:

* это только транспортная и фазовая проверка; полная сверка истории, экрана
  и cursor-позиции внутри standalone-инструмента ещё не закончена;
* static-пробник временно использует высоту `80`, чтобы не смешивать
  bottom-row circling с проверяемой primary-фазой; это диагностическая
  настройка, а не доказательство dynamic reflow. **Обход, по-видимому, не
  нужен**: пункт 7 в `PINNED_HOST_FACTS.md` показывает по исходнику, что
  прокрутка не перевыдаёт уехавшее содержимое и при курсоре на нижней строке
  не даёт скобок вовсе. Требуется прогон с заведомой прокруткой, чтобы снять
  высоту как часть методики;
* не подтверждены отдельными native-прогонами краевые случаи скрытого
  курсора (`_lastCursorIsVisible`), `StartPaint`/`_quickReturn` и влияние
  `PaintCursor`/`_nextCursorIsVisible` на закрывающую скобку;
* dynamic reflow, resize во время вывода, реальные команды, экстремальные
  условия, fuzzing и 300 seed-сессий остаются открытыми; gate по-прежнему
  fail-closed.

Сгенерированные probe-артефакты из исследовательских прогонов в этот коммит
не входят и перед фиксацией удаляются; сохраняются только исходники, тесты и
этот проверяемый аудит.

### Native-проверка разделения фаз и alternate history

После чтения `XtermEngine::StartPaint`/`EndPaint` по пиннутому исходнику
проверено, что первый paint начинается с невидимым курсором, а последующие
абсолютные перемещения могут обрамляться host-bracket. Поэтому control и
alternate workload передаются цельной записью: это не правило восстановления
истории, а способ не разрывать маркер самим дочерним эмиттером. Произвольные
границы чтения по-прежнему проверяются отдельно на уже захваченном потоке.

Команда последнего прогона:

```text
go run . -probe-static -report artifacts/pinned-conpty-probe-static-phases7.json
```

На pinned OpenConsole из отчёта (`1.12.220408003-release1.12`, SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`) получено:

* primary: `passed=19`, `failed=0`, 17 logical lines;
* control: `passed=5`, `deferred=4`, `failed=0`, один repaint-frame;
* alternate: `passed=4`, `failed=0`; записи `alt-screen` и `alternate-end`
  проверены на отсутствие в primary history.

Control assertions с SGR/erase/cursor/tabs/OSC пока явно `deferred`: текущий
накопитель сохраняет host-emitted строки, но ещё не предоставляет
побайтовую историю после применения управляющих последовательностей. Это
открытая часть A7, а не пропуск проверки. Для всех трёх сессий артефакт
проверен байтово и по SHA; generated reports этого исследовательского прогона
в репозиторий не добавляются.

### Проверка пункта 11: hidden-cursor primary

Primary payload обрамлён дочерним `ESC[?25l` ... `ESC[?25h`, чтобы
`PaintCursor` не вызывался во время проверки длинной строки. Последний
успешный native-прогон:

```text
go run . -probe-static -report artifacts/pinned-conpty-probe-static-hidden-cursor3.json
```

На том же pinned OpenConsole (`1.12.220408003-release1.12`, SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`)
primary-сессия дала `passed=19`, `failed=0`, `repaint_frames=0`.
В частности, `long` (257 `C` после префикса), `exact-n` и
`exact-2n-plus-1` прошли как отдельные точные logical lines. Это подтверждает
по прогону механизм длинной строки и объяснение пункта 11; распознавание
разрыва по CUP не понадобилось и не добавлялось.

Ограничение остаётся явным: у реальных команд курсором управляет ребёнок, и
пробник не может скрыть его вокруг `dir` или сборки. При совпадении условий
из пункта 11 pinned-host может сам потерять wrap-состояние и выдать внутренний
`CRLF`; это ограничение OpenConsole 1.12, а не дефект накопителя. Control и
alternate static-сессии в этом же отчёте не являются закрытием A7: control
имеет межстрочный repaint, для которого пункт 8 определяет историю как
`deferred`, а полная A/B/C/D-проверка ещё не выполнена.

### Partial-строка через resize и граница кадра

После чтения pinned-исходника по `_invalidMap` и `_quickReturn` выполнен новый
прицельный native-прогон:

```text
go run . -partial -report artifacts/pinned-conpty-partial-next.json
```

Хост в отчёте проверен как `OpenConsole.exe` версии
`1.12.220408003-release1.12` с SHA
`14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`.
Скриптовая проверка отчёта получила `raw=246`, `expected=131`, один явный
`CRLF`, resize-offset `0,0,8,180`. Единственная запись
`rendered_lines` имеет `129` байт текста и `2` байта терминатора; их
конкатенация (`131` байт) побайтово равна `expected_input`. Завершение также
подтверждено полями `child_exited=true`, `host_exited=true`,
`handles_closed=true`; все три проверки произвольного chunking — `passed`.
Это подтверждает конкретную последовательность «половина строки → resize →
хвост» на живом хосте и не разрешает склеивать строки по ширине.

В этом же изменении resize-offset стал записываться до вызова
`WriteFile(PTY_SIGNAL_RESIZE_WINDOW)`, то есть на начало собственной операции,
а не после неё. Это устраняет гонку измерения, но не создаёт несуществующий
конец кадра.

Исходник pinned-коммита уточняет открытый вопрос. `VtEngine::StartPaint`
(`src/renderer/vt/paint.cpp:21-42`) возвращает `S_FALSE` при пустых
`_invalidMap`/`_scrollDelta`/`_cursorMoved`/`_titleChanged`; это внутренний
ранний возврат и наружу не сериализуется. `VtEngine::EndPaint`
(`paint.cpp:54-65`) только сбрасывает состояние, а
`Renderer::_PaintFrameForEngine` (`src/renderer/base/renderer.cpp:107-181`)
обходит dirty-area и завершает paint. В pipe нет байтового события «repaint
закончился». Поэтому после собственного resize нельзя отбрасывать весь поток
до следующего наблюдаемого события: ребёнок может уже продолжать новый
дописывающий вывод. Надёжный внешний протокол границы пока не найден; это
открытый blocker динамической истории и C1, а не основание для эвристической
дедупликации.

Повтор после фиксации offset перед `WriteFile` выполнен командой:

```text
go run . -partial -report artifacts/pinned-conpty-partial-next2.json
```

Проверка отчёта и приложенного `.raw` вывела:

```text
raw=247 expected=131 rendered=131 exact=True
raw_sha=98e2eb0fb54046ca482375dec30a1f5a8ca71c422054d6b3803ef59a3faf586a
file_sha=98e2eb0fb54046ca482375dec30a1f5a8ca71c422054d6b3803ef59a3faf586a
offsets=0,0,8,116 child=True host=True handles=True
```

Размер сырого потока меняется из-за асинхронного порядка paint, но логическая
строка после «половина → resize → хвост» остаётся ровно 131 байтом; это
повторяемое подтверждение транспорта, а не разрешение на вывод границы из
репейнта.

### Измерение ограничения на реальной `dir /s`

Чтобы оценить практическую частоту ограничения из пункта 11 до интеграции,
добавлен отдельный диагностический режим (он не является assertion истории):

```text
go run . -command-probe -report artifacts/pinned-conpty-command-next.json
```

Он запускает на pinned `OpenConsole.exe` настоящую команду
`cmd.exe /d /q /c "set DIRCMD= & dir /s /b C:\Windows\System32 & echo ..."`
в двух размерах (`80x25` и `20x10`) и сохраняет `.raw`. Режим считает только
наблюдаемые факты потока — число `CRLF`, число последовательностей
`ESC[<row>;<col>H\r\n`, `RenderedLines.CrossRow`, marker и lifecycle; эти числа
не используются для восстановления истории.

Проверка отчёта получила:

```text
width=80  raw=1835978  raw_crlf=32676  cup_before_crlf=1  rendered_cross_row=9384  marker_count=1  exit=0 child=true host=true handles=true
width=20  raw=1835931  raw_crlf=32677  cup_before_crlf=3  rendered_cross_row=9379  marker_count=1  exit=0 child=true host=true handles=true
```

То есть в этом конкретном дереве наблюдалось `1/32676` и `3/32677`
непосредственных CUP→CRLF-кандидата соответственно; содержимое примеров
показывает длинные пути `CatRoot`/`DriverStore`, разорванные между рядами.
Большинство записей пришло без такого соседства, поэтому ограничение в этом
прогоне редкое, но не нулевое. `RenderedLines.CrossRow` существенно больше и
не является счётчиком разрывов: он включает абсолютные перемещения и
репейнт. Полного вывода «целая логическая строка против разорванной» из одного
неразмеченного потока делать нельзя; для этого нужен отдельный законный
источник границы, которого pinned API не предоставляет. Это измерение закрывает
вопрос частоты как наблюдение и оставляет C1/динамическую историю открытыми.

### B1-0: reflow без изменения размера ConPTY

Требования обновлены подразделом B1-0: ширина меняется только у потребителя,
а pinned host остаётся в исходном размере. Старый путь `-probe`, который
вызывает `ResizePseudoConsole`, сохранён как неактивная диагностическая
альтернатива; gate его не вызывает.

Первый native-прогон новой схемы:

```text
go run . -reflow-probe -report artifacts/pinned-conpty-reflow-next.json
```

Сессия статического pinned host `80x80` дала `initial_a`: 4 строки и 2
маркера, все `passed`; `resizes=0`, `child_exited=true`, `host_exited=true`,
`handles_closed=true`. После получения этих целых строк потребительская матрица
ширин/высот (`1x1`, `79x24`, `80x25`, `81x26`, `121x40`, `20x10`, `121x10`,
`1x25`, возврат `80x25`) дала `layout_status=passed` во всех 9 контрольных
точках. В каждой точке `stored_lines=4` и SHA единой истории
`be2e3c7da3fd039fc458c5b7477efb11a8fd92647325cd07a4b7d98c1ecde365` не менялся;
число display rows было `406,8,7,7,6,22,6,406,7` соответственно.

Это закрывает для текущего consumer-only слоя сохранность целых строк и
перевыкладку по ширине без host repaint; это не выдаётся за полный B2: отдельные
снимки screen/history/cursor теперь входят в каждый checkpoint consumer-модели;
интеграция с f4 по-прежнему вне standalone gate. Нативная исходная
история при этом получена живым pinned host, а не второй моделью консоли.

Повторный прогон с расширенными B2-снимками:

```text
go run . -reflow-probe -report artifacts/pinned-conpty-reflow-b2.json
width=1   height=1  display_rows=406 cursor=(406,0) layout_status=passed
width=79  height=24 display_rows=8   cursor=(8,0)   layout_status=passed
width=80  height=25 display_rows=7   cursor=(7,0)   layout_status=passed
width=81  height=26 display_rows=7   cursor=(7,0)   layout_status=passed
width=121 height=40 display_rows=6   cursor=(6,0)   layout_status=passed
width=20  height=10 display_rows=22  cursor=(22,0)  layout_status=passed
width=121 height=10 display_rows=6   cursor=(6,0)   layout_status=passed
width=1   height=25 display_rows=406 cursor=(406,0) layout_status=passed
width=80  height=25 display_rows=7   cursor=(7,0)   layout_status=passed
```

Каждый checkpoint содержит `screen_sha256`, `stored_history_sha256` и cursor
координаты; история осталась неизменной при всех изменениях отображения.

### Набор реальных команд (A4)

Отдельный native suite прогнал обязательные ограниченные команды на pinned
host с уникальными begin/end-маркерами и проверкой exit/lifecycle:

```text
native command suite complete: artifacts/pinned-conpty-command-suite-v2.json cases=4
echo       exact=true exit=0 child_exited=true host_exited=true handles_closed=true
type       exact=true exit=0 child_exited=true host_exited=true handles_closed=true
findstr    exact=true exit=0 child_exited=true host_exited=true handles_closed=true
powershell exact=true exit=0 child_exited=true host_exited=true handles_closed=true
```

`type` читает временный UTF-8 fixture, PowerShell запускается с
`-NoProfile -NonInteractive`; каждый raw-файл сохранён и проверен побайтово.

### Изолированные табы и OSC 8 (10.3, 14.3)

После того как общий control-сеанс показал repaint-смешение, оба поведения
проверены отдельными статическими сессиями pinned host 512x25:

```text
native semantic probe complete: artifacts/pinned-conpty-tabs.json kind=tabs exact=true
native semantic probe complete: artifacts/pinned-conpty-link.json kind=link exact=true
```

Для `tabs` наблюдалась ровно строка `tabs:   X       Y`: табуляций в потоке
нет, хост выдал перемещения по табостопам. Для OSC 8 текст `link` совпал,
а последовательность завершилась ST (`ESC\\`); оба raw-файла и SHA проверены.

Прогресс-бар проверен отдельно тем же способом:

```text
native semantic probe complete: artifacts/pinned-conpty-progress.json kind=progress exact=true
```

Промежуточные состояния `0%` и `50%` не стали историческими строками;
финальное `progress: 100%` совпало побайтово.

Изолированная Unicode-проверка (10.5) также прошла:

```text
native semantic probe complete: artifacts/pinned-conpty-unicode.json kind=unicode exact=true
```

Ожидаемая и наблюдаемая строка совпали побайтово для CJK, combining mark,
emoji, ZWJ `👩‍💻`, иврита и арабского; ширины дисплея в assertion не входят.

### Завершение native output после child exit

Пиннутый renderer может дописывать финальный paint уже после завершения child.
Адаптер теперь ждёт bounded quiescence output (300 мс без новых байт, максимум
10 секунд) перед закрытием pseudoconsole. Это предотвращает усечение хвоста и
не использует содержимое/форму потока как границу логической строки. После
изменения повторные native empty, command-suite и static-прогоны завершились
успешно с закрытыми child/host/handles.

### D3: 300 детерминированных seed-сессий

Полный запуск после фиксации host-width=512 завершён без ошибок:

```text
native seed gate: 300/300 (100%) width=121
seed_count=300 failures=0 sessions=300
```

Каждая сессия сохранила raw-артефакт с проверенным SHA и закрытыми child/host/
handles; после прогона активных процессов pinned probe/OpenConsole не осталось.
Разнообразие `width=1,79,80,81,121` теперь относится к payload и consumer
reflow, а не к размеру ConPTY, поэтому известное зависание host 1x1 не искажает
этот этап. Это закрывает воспроизводимый seed-stage; полная C4-матрица
cancel/timeout/close-order всё ещё отдельный незакрытый пункт.

Повтор D3 после добавления consumer-проверок к каждой сессии:

```text
seed_count=300 sessions=300 consumer_checks=1500 failures=0
```

Каждый seed дополнительно прошёл пять consumer checkpoints (1x1, 79x24,
80x25, 121x40, 512x25): SHA единой истории не изменился после scroll/reflow,
screen SHA зафиксирован, piece-table spill проверен. Это не заменяет C4
жизненного цикла, но закрывает требование запускать B/C consumer assertions на
каждом из 300 native seed.

### Независимая сверка `dir /s /b` через redirected-файл

По пункту 11 выполнена одна и та же команда двумя путями: напрямую в файл
(`cmd.exe /d /q /c "set DIRCMD= & dir /s /b C:\\Windows\\System32"`) и через
pinned ConPTY с маркерами. Файл — независимый источник настоящих границ строк;
переводы `CRLF` нормализованы к `LF`, содержимое строк не изменялось.

Первый прогон до source-derived CUP-правила дал `28525` несовпадений при
`23906` ожидаемых и `32679` наблюдаемых строках. После включения правила для
абсолютного CUP в последний столбец и повторной проверки:

```text
go run . -command-compare -report artifacts/pinned-conpty-command-compare-cup2.json
pinned-conpty-probe: native command comparison found 19686 line mismatches (report artifacts/pinned-conpty-command-compare-cup2.json)
expected_lines=23906 observed_lines=23471 mismatch_count=19686 cup_before_crlf=1
```

Классификация последнего отчёта: `content_mismatch_count=19679`,
`trailing_padding_only=7`, `cross_row_mismatch=57`. Правило уменьшило число
расхождений, но не свело его к нулю; следовательно, оно пока не может быть
критерием закрытия A и не включается в общий gate как доказанное решение.
В частности, один наблюдённый фрагмент содержит
`...proxys\r\nESC[999;80Hstub.dll`, что после механического продолжения даёт
`proxysstub.dll`, тогда как redirected-файл содержит `proxystub.dll`.
Это отдельное открытое расхождение, требующее чтения исходника и нового
прицельного прогона; дедупликация по совпадающим символам запрещена.

Дополнительный прогон с более узким source-derived правилом (только CUP,
непосредственно предшествующий следующему `CRLF`; без попытки угадывать по
`CRLF` перед CUP) дал:

```text
go run . -command-compare -report artifacts/pinned-conpty-command-compare-cup5.json
pinned-conpty-probe: native command comparison found 28519 line mismatches (report artifacts/pinned-conpty-command-compare-cup5.json)
expected_lines=23906 observed_lines=32680 mismatch_count=28519 cup_before_crlf=1
```

Это почти не меняет результат и подтверждает, что одно CUP→CRLF правило не
объясняет основную массу расхождений. Вариант, который трактовал `CRLF` перед
последне-колоночным CUP как продолжение, был отброшен: он ошибочно склеивал
законные соседние строки (`observed_lines=23471`) и потому был бы эвристикой.

### Контрольный payload после изоляции от отрисовки курсора

Контрольная фаза также выполнена со скрытым курсором, чтобы отделить SGR,
стирание, CUP и табуляции от известного cursor-wrap ограничения:

```text
go run . -probe-static -report artifacts/pinned-conpty-probe-static-control-hidden.json
```

`red`, `rewritten`, `cursor`, SGR и alternate-screen assertions прошли.
`tabs` остался `deferred`: живой поток содержит
`tabs:\x1b[3CX\x1b[7CY\x1b[8;1H__PINNED_CONPTY_PROBE_CONTROL_END__` без `CRLF`
между табличной строкой и следующим маркером. Текущий накопитель не угадывает
границу по одному CUP и не превращает этот результат в проход; пункт 10.3
остаётся открытым до source-derived решения и отдельного прогона.

В том же прогоне добавлены отдельные атрибуты и OSC 8. `bold`, `under`,
`reverse`, все шесть пар SGR-последовательностей и OSC 8 (`id=<pid>-<n>` плюс
ST `ESC\\`) прошли exact-count проверку. Текст `link` пока `deferred`, как и
`tabs`: хост вывел его после абсолютного CUP без явного `CRLF`, а байтовая
проверка OSC 8 уже подтверждает правильный ST и отсутствие зависимости от
детерминированного id.

### Проверка состоятельности метрики `dir`

Позиционный счётчик дополнен первым содержательным расхождением (с hex),
контекстом двух строк с каждой стороны и разреженным LCS по нормализованным
только для этой диагностики хвостовым пробелам. Повторный прогон:

```text
go run . -command-compare -report artifacts/pinned-conpty-command-compare-lcs.json
pinned-conpty-probe: native command comparison found 28517 line mismatches (report artifacts/pinned-conpty-command-compare-lcs.json)
expected_lines=23906 observed_lines=32671 mismatch_count=28517 normalized_mismatch_count=28513
lcs_length=15805 lcs_insertions=8765 lcs_deletions=8101 lcs_replacements=8101 cup_before_crlf=0
```

Первое строгое несовпадение — только renderer padding: ожидаемая строка
`...NearShareExperience.dll` (hex заканчивается `646c6c`), наблюдаемая та же
строка с четырьмя байтами `20 20 20 20`. Первое содержательное несовпадение
в контексте индекса `4158`:

```text
expected: C:\\Windows\\System32\\windows.applicationmodel.conversationalagent.internal.proxystub.dll
observed: C:\\Windows\\System32\\windows.applicationmodel.conversationalagent.internal.proxys
next observed: 104 пробела + stub.dll (cross_row=true)
```

Redirected-файл принудительно получен в UTF-8 (`chcp 65001 >nul`), не содержит
ошибочных UTF-8 replacement-символов; его хвост — ровно `0d 0a`, после
нормализации это `23906` непустых строк. Заголовка или итоговой строки `dir`
в файл не попало. Поэтому OEM/UTF-8 и лишний финальный перевод строки не
объясняют результат. LCS показывает не «один сдвиг»: после нормализации есть
`8765` чистых вставок, `8101` удалений и `8101` замен. Это измеряет смесь
повторных render-записей и разорванных строк, а не число независимых дефектов;
причину нужно разбирать по кадрам, не по позиционному счётчику.

### Сырые байты первого содержательного расхождения

По требованию проверен именно приложенный поток
`artifacts/pinned-conpty-command-compare-lcs.json.sessions/80x1000.raw`.
Фрагмент 200 байт до и после найденного `...proxys` (смещения потока):

```text
000256B0  67 65 6E 74 2E 69 6E 74 65 72 6E 61 6C 2E 70 72  gent.internal.pr
000256C0  6F 78 79 73 0D 0A 1B 5B 39 39 39 3B 38 30 48 73  oxys...[999;80Hs
000256D0  74 75 62 2E 64 6C 6C 0D 0A 43 3A 5C 57 69 6E 64  tub.dll..C:\Wind
000256E0  6F 77 73 5C 53 79 73 74 65 6D 33 32 5C 77 69 6E  ows\System32\win
```

Между `proxys` и `stub.dll` в сыром потоке **нет 104 пробелов**: там ровно
`0d 0a 1b 5b 39 39 39 3b 38 30 48`. 104 пробела из предыдущего отчёта
синтезированы самим диагностическим накопителем при обработке CUP в колонку
80 (`put` заполнял промежуток до `h.column`); это не байты host и не основание
для правила склейки. Последовательность также не является CUP→CRLF-кандидатом:
она имеет `CRLF → ESC[999;80H → stub.dll → CRLF`. Тем самым измерение
подтверждает пункт 11 (`cup_before_crlf=0` в этом прогоне), а основная масса
расхождений объясняется пунктом 15 `PINNED_HOST_FACTS.md`: `_wrappedRow`
сбрасывается безусловно на границе кадра, поэтому видимый курсор может
превратить физический перенос в обычный `CRLF` без CUP. Накопитель по этому
результату не менялся и не получает эвристику склейки.

### Проверка ширины захвата по пункту 15.2

После чтения пункта 15 повторена одна и та же сверка `dir /s /b` с
redirected-файлом, менялась только ширина pinned-host сессии через новый
параметр `-command-compare-width`. Максимальная строка независимого файла в
этом дереве — 199 символов; все строки ASCII, поэтому это также 199 колонок.
Результаты (это проверка рабочего правила, а не
поиск минимального порога):

```text
width=512  mismatches=0  expected=23906 observed=23906 cross_row=0
width=384  mismatches=0  expected=23906 observed=23906 cross_row=0
width=320  mismatches=0  expected=23906 observed=23906 cross_row=0
width=256  mismatches=0  expected=23906 observed=23906 cross_row=0
width=224  mismatches=0  expected=23906 observed=23906 cross_row=0
width=208  mismatches=0  expected=23906 observed=23906 cross_row=0
width=207  strict=1 normalized=0 content=0 trailing_padding=1
width=206  strict=2 normalized=0 content=0 trailing_padding=2
width=205  strict=2 normalized=0 content=0 trailing_padding=2
width=204  strict=2 normalized=0 content=0 trailing_padding=2
width=201  mismatches=8
width=200  mismatches=16685 (LCS: one deletion, далее позиционный сдвиг)
width=199  mismatches=16685 (LCS: two deletions, далее позиционный сдвиг)
width=198  mismatches=11
```

Команда первого прогона:

```text
go run . -command-compare -command-compare-width 512 -report artifacts/pinned-conpty-command-compare-512.json
native command comparison complete: artifacts/pinned-conpty-command-compare-512.json mismatches=0 expected_lines=23906 observed_lines=23906 cup_before_crlf=0
```

При ширине 512 доказан строгий байтовый результат; тот же ноль получен на
256, 320 и 384 (а также на 224 и 208). Ширина 512 принята как рабочее
правило: она заведомо превышает все строки этого корпуса и подтверждена
повторяемым `mismatch=0`. Значения 198–207 и 199/200 показывают не
характеристику хоста и не минимальный порог, а плавающий результат в области,
где строки начинают переноситься и границы кадров случайно совпадают:
единичное удаление может дать тысячи позиционных последствий. В самом
redirected-файле строки на 208 колонок не найдено: все строки ASCII и максимум
равен 199. Поэтому 208 не записывается как свойство данных или хоста, а
результаты узких ширин остаются диагностикой плавающих границ кадра и
padding/переноса. Правило CUP
из пункта 11 оставлено без изменений; на широком захвате `cup_before_crlf=0`.

### Native scrollback и вытеснение (C3)

Добавлен отдельный `-scroll-probe`: он получает статический поток pinned host
на `512x8` со спрятанным курсором, сверяет 37 строк с authored payload, затем
передаёт их в consumer-модель. Модель хранит хвост из 8 целых логических
строк, старшие записи складывает в piece-table spill, а отображение получает
только через reflow этих записей.

```text
go run . -scroll-probe -report artifacts/pinned-conpty-scroll.json
native scrollback probe complete: artifacts/pinned-conpty-scroll.json lines=37 spilled=29 history_sha256=bce1f058e4006c14feeacfdd5a7f0f09886ad48f4785b800242d2282998f8906
```

Отчёт: `expected_lines=37`, `observed_lines=37`, `spilled_pieces=29`,
`spilled_bytes=1254`, `eviction_boundary_preserved=true`. Все семь
контрольных точек прокрутки до/после consumer-only изменения ширины/высоты
имеют `status=passed`; SHA истории не менялся. Длинная строка на границе
вытеснения сохранилась одним piece байт-в-байт. Сырой native поток и его SHA
проверены тем же `writeAndVerifyRawArtifact`, что и остальные прогоны.

### `Clear-Host` и `ESC[3J` (14.4)

Отдельный native-прогон PowerShell проверил семантическое очищение
scrollback:

```text
go run . -clear-probe -report artifacts/pinned-conpty-clear.json
native clear probe complete: artifacts/pinned-conpty-clear.json esc3j=1 begin=0 end=1
```

Хост выдал ровно один `ESC[3J`; маркер до `Clear-Host` отсутствует в
накопленной истории, маркер после него присутствует один раз. Процесс ребёнка,
host и native handles закрылись, raw-файл проверен побайтово и по SHA.

### Пустой ребёнок и quick-return (10.8)

Запуск ребёнка без вывода показал 56 сырых байт стартовой внеполосной
инициализации (`ESC[?9001h`, очистка, SGR/home, title и hide-cursor), но ни
одной отрисованной логической строки:

```text
go run . -empty-probe -report artifacts/pinned-conpty-empty.json
native empty probe complete: artifacts/pinned-conpty-empty.json raw_bytes=56 rendered_lines=0 child=true host=true handles=true
```

Критерий 10.8 проверяет именно `rendered_lines=0`, а не отсутствие startup
controls. Raw-файл и SHA сохранены; процессы и handles закрыты.

### Заголовок окна (10.6)

В control-прогоне assertion считает OSC-title по всему raw-потоку, поскольку
хост выдаёт его после `controlEnd`, вне логического payload:

```text
go run . -probe-static -report artifacts/pinned-conpty-probe-static-title.json
title-osc: passed observed_count=1
```

Последовательность ровно `ESC]0;pinned-conpty-probe BEL`; в `RenderedLines` она
не попадает.

### Consumer-only resize во время реального `dir` (C1)

Для B1-0 host оставался фиксированным (`512x1000`), а captured output
подавался в накопитель incremental chunks. На контрольных offsets менялась
только ширина consumer display:

```text
artifacts/pinned-conpty-command-compare-c1.json
expected_lines=23906 observed_lines=23906 mismatch_count=0
width=1   offset=287392  completed_lines=5478  passed
width=79  offset=574758  completed_lines=7260  passed
width=80  offset=862127  completed_lines=11778 passed
width=121 offset=1149514 completed_lines=14679 passed
width=20  offset=1436871 completed_lines=19658 passed
width=512 offset=1724241 completed_lines=23906 passed
```

SHA истории на каждой контрольной точке записан в отчёте и не изменяет
канонический результат. Это native подтверждение C1 для consumer-only resize;
host-resize ветка остаётся неактивной по B1-0.

### Усиление проверки произвольных границ чтения (C2)

Static native-прогон теперь проверяет три расписания chunking не только для
диагностического host-stream состояния, но и для основного накопителя целых
логических строк:

```text
go run . -probe-static -report artifacts/pinned-conpty-probe-static-c2-final.json
one-byte         passed
fixed-7          passed
prng             passed
logical-one-byte passed
logical-fixed-7  passed
logical-prng     passed
```

Сравниваются байты записей и их явные терминаторы; в расписаниях присутствуют
разрывы UTF-8, CSI/OSC и CRLF. Все шесть проверок прошли на raw-потоке живого
pinned host.
