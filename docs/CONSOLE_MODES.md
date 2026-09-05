# f4: альтернативный режим Ctrl+O («консоль хоста», как в Far и mc)

Диздок и план работ. Исполнителю: читать целиком до написания первой строки кода.
Раздел 9 — список файлов, которые нужно иметь под рукой.

## 1. Что есть сейчас

f4 — Go-приложение (`package main` в корне), UI и ввод вынесены в `vtui`/`vtinput`.

* Панели рисуются в **alternate screen**: `vtui.PrepareTerminal()` → `Resume()` шлёт
  `\x1b[?1049h`, выключает autowrap, включает продвинутые протоколы ввода
  (`vtui/terminal_env.go`).
* Встроенный терминал: `PanelsFrame.initPTY()` запускает шелл в своём PTY; байты из PTY
  идут в `AnsiParser.Process()` → сетка `TerminalView` → `vtui.ScreenBuf`. То есть
  **f4 сам является эмулятором терминала**, хостовый терминал видит только готовый кадр.
* `Ctrl+O` = action `Panel.Toggle` (`action_registry.go`,
  `DefaultKeys: ["CtrlO:NoAltScreenApp", ...]`), переключает `pf.showPanels`, то есть
  показывает **внутреннюю** сетку, а не исходную консоль.
* Команда из командной строки (`panels_frame.go`, обработка `VK_RETURN`) заворачивается в
  OSC 133 (`\033]133;C\007 … \033]133;D\007`), выставляет `pf.executing = true`,
  `pf.returnToPanels = pf.showPanels`, `pf.showPanels = false` и пишется в PTY через
  `writePTY`. По приходу `OSC 133;D` (`TerminalView.HandleOSC133` → `OnBusyChange`,
  подписка в `NewPanelsFrame`) панели возвращаются.

**Цель:** опциональный режим, где вывод команд идёт напрямую в **настоящий** терминал
(в его primary screen), а `Ctrl+O` переключает alt screen (панели) ↔ primary screen
(реальная консоль со скроллбэком, историей и мышью хоста).

## 2. Ограничения среды (понять до кода)

### 2.1. На Unix UI живёт в демоне

`session_unix.go`: `ManageSessions()` → `startNewSession()` порождает `f4 --server <sock>`
с `Setsid: true`; клиент (`runClient`) передаёт демону свои `fd 0/1` и notify-pipe через
`SCM_RIGHTS` и блокируется на чтении пайпа. UI (`FrameManager.Run`) крутится **в демоне**.

Следствия:

* демон не входит в сессию управляющего терминала → `tcsetpgrp()` для его детей вернёт
  `ENOTTY` → **job control для детей на хостовом tty не работает**;
* если вернуть терминал в cooked с `ISIG`, `Ctrl+C` уйдёт в foreground-группу tty, то есть
  **в процесс-клиент**, а не в команду: клиент умрёт, шелл получит промпт обратно, а демон
  продолжит рисовать в тот же tty. Это уже латентная проблема существующего
  `runExternalEditor()` (`actions.go`: `vtui.Suspend(); cmd.Run(); vtui.Resume()`).

**Вывод: «suspend + fork с наследованием stdio» не может быть основой нового режима на
Unix.** Основной режим обязан сохранять PTY.

На Windows демона нет: `session_windows.go: ManageSessions()` = `InitCore()` +
`PrepareTerminal()` + `FrameManager.Run()` в том же процессе.

### 2.2. GUI-бэкенды

`gui_unix.go`/`gui_windows.go` → `vtui.RunInGUIWindow(...)`; GUI-хосты вызывают
`vtui.SetActiveBackend("x11"|"wayland"|"gogpu"|"ebiten"|"win32")`, в чистом терминале
`vtui.ActiveBackend() == ""`. Плюс `checkAndDetach()` переоткрывает процесс с
`stdin/stdout/stderr → /dev/null`. Хостового терминала физически нет → **в GUI всегда
откат на «свой терминал»**.

### 2.3. Wine

`vtui.IsWine()` уже есть и используется в `main.go`. Под Wine ConPTY сырой → PTY считаем
недоступным и **не пытаемся его создавать**.

## 3. Три режима исполнения команд

| Режим | Имя в коде | Суть |
|---|---|---|
| Свой терминал (по умолчанию) | `ShellModeOwn` | как сейчас: PTY → парсер → сетка → vtui |
| Консоль хоста | `ShellModeHost` | PTY сохраняется, его байты транслируются в реальный stdout; панели = alt screen, консоль = primary screen |
| Простая выполнялка | `ShellModeSimpleInline` / `ShellModeSimpleCaptured` | PTY нет: либо `Suspend → exec с наследованием stdio → Resume`, либо запуск с пайпами и вывод в окно f4 |

### 3.1. Почему «консоль хоста» сохраняет PTY (главное решение)

В `ShellModeHost` шелл по-прежнему живёт в PTY, но вывод PTY **не парсится в кадр, а
пишется байт-в-байт в хостовый терминал**, пока панели скрыты. Это архитектура mc
(`feed_subshell()` читает из `subshell_pty` и пишет в `stdout`). Что это даёт:

* рабочий job control (`Ctrl+C`, `Ctrl+Z`, `^\`): шелл — лидер сессии своего PTY, сигналы
  генерирует line discipline самого PTY;
* нативный скроллбэк, выделение мышью и поиск средствами хостового терминала — ровно то,
  чего ждут от «как в Far/mc»;
* нативную sixel/kitty-графику, если её умеет хостовый терминал; встроенный
  терминал (`ShellModeOwn`) принимает и то и другое сам — см. раздел 8
  `TERMINAL.md`, — так что графика работает в обоих режимах, но по разным
  путям: в `ShellModeHost` картинку рисует хостовый терминал, в `ShellModeOwn`
  её декодирует f4 и кладёт в слой размещений;
* совместимость с демон-архитектурой Unix: демон уже владеет `os.Stdout` клиента;
* минимальную дельту: ввод (`TranslateInput`), запуск команд (OSC 133), возврат к панелям —
  переиспользуются без изменений.

### 3.2. Зеркалирование (mirror) — всегда включено

Даже в `ShellModeHost` поток PTY продолжает скармливаться `AnsiParser` в «немом» режиме
(4.4). Причины:

* сохраняется бесконечный лог в `PieceTable` → `F3` по логу, `terminal_log_vfs.go`,
  выделение, `GetAllLogBytes()`;
* сессию можно отсоединить (`SupportsBackgrounding()`), тогда `os.Stdout` уходит в
  `/dev/null` — без зеркала вывод фонового шелла терялся бы;
* после re-attach primary screen нового терминала пуст, из зеркала можно перерисовать хвост;
* f4 продолжает знать состояние (alt screen приложения, bracketed paste, mouse tracking,
  `Win32InputMode`, `KittyFlags`) и корректно восстанавливает экран при возврате к панелям.

Опция отключения зеркала ради производительности — **не сейчас**, при необходимости позже.

## 4. Дизайн режима `ShellModeHost`

### 4.1. Разрешение режима (единая точка)

```go
type ShellMode int
const (
    ShellModeOwn ShellMode = iota
    ShellModeHost
    ShellModeSimpleInline
    ShellModeSimpleCaptured
)

// Все проверки — через переменные-функции, чтобы тесты могли их подменять.
var (
    probeGUIBackend = func() string { return vtui.ActiveBackend() }
    probeHostTTY    = func() bool   { return term.IsTerminal(int(os.Stdout.Fd())) }
    probePTYUsable  = func() bool   { return !vtui.IsWine() && lastPTYAllocOK }
)

func resolveShellMode(cfg ShellModeConfig) ShellMode
```

Правила, порядок важен:

1. `!probePTYUsable()` → `ShellModeSimpleInline`, если есть хостовый tty **и**
   `runtime.GOOS == "windows"`; иначе `ShellModeSimpleCaptured`.
   *Почему:* inline безопасен только там, где нет демона (2.1).
2. `cfg.Preferred == ShellModeOwn` → `ShellModeOwn`.
3. `probeGUIBackend() != ""` → `ShellModeOwn`.
4. `!probeHostTTY()` → `ShellModeOwn`.
5. иначе → `ShellModeHost`.

| Среда | конфиг `own` | конфиг `host` |
|---|---|---|
| TTY + PTY ок | own | **host** |
| TTY, PTY нет (Wine в консоли) | simple-inline | simple-inline |
| GUI + PTY ок | own | own |
| GUI, PTY нет (дефолт Wine) | simple-captured | simple-captured |

Режим вычисляется **один раз при создании `PanelsFrame`** и хранится в `pf.shellMode`.
Переключение на лету не поддерживается: `TERM` и прочее окружение шелла формируются при
его старте (4.7). В диалоге настроек — явная подпись «применится к новым сессиям».

### 4.2. Вид консоли: mc по умолчанию, Far по галке

Решено:

* **По умолчанию (mc-стиль):** при скрытых панелях экран целиком принадлежит ребёнку, f4 не
  рисует ничего. Пользователь работает с промптом самого шелла.
* **По галке `ConsoleOverlayUI` (Far-стиль):** внизу primary screen поверх консоли
  держатся командная строка f4 и (если включён) keybar.

Реализация Far-стиля — **без участия `ScreenBuf`**, иначе обычный рендер перерисует весь
экран из теневого буфера и затрёт консоль. Вместо этого:

* `n` = 1 (командная строка) + 1, если `pf.showKeyBar`;
* при входе в консоль выставить scroll region `\x1b[1;<h-n>r`, чтобы вывод ребёнка не
  затекал в зарезервированные строки; при выходе — `\x1b[r`;
* размер PTY = `(w, h-n)` вместо `(w, h)`;
* оверлей рисуется вручную коротким ANSI-снипетом через `vtui.WritePassthrough`:
  `\x1b7` (save cursor) + `\x1b[<row>;1H` + SGR + текст + `\x1b8` (restore);
* перерисовка оверлея: после каждого обработанного чанка PTY (троттлинг ~30 мс) и на каждое
  изменение содержимого командной строки.

В mc-стиле `n = 0`, scroll region не трогаем, оверлей не рисуем.

### 4.3. Состояния и переходы

Два визуальных состояния (уже отражены в `pf.showPanels`):

* `showPanels == true` → alt screen, панели, f4 владеет клавиатурой и экраном;
* `showPanels == false` → primary screen, экраном владеет ребёнок.

`enterHostConsole()`:
1. `pf.SetBusy(true)` (поле `BaseFrame.Busy`; `FrameManager` пропускает Draw+Flush — см.
   проверку `!topFrame.IsBusy()` в `framemanager.go`);
2. `vtui.SetAltScreen(false)`; **raw-режим и протоколы ввода не трогаем** — f4 продолжает
   читать клавиатуру сам;
3. если `ConsoleOverlayUI` — выставить scroll region и нарисовать оверлей;
4. включить роутер вывода: байты PTY → `vtui.WritePassthrough(...)` + зеркало в парсер.

`leaveHostConsole()`:
1. выключить роутер, дождаться слива текущего чанка;
2. защитный сброс того, что мог оставить ребёнок: если по зеркалу `UseAltScreen` — шлём
   `\x1b[?1049l`; далее `\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l` (мышь),
   `\x1b[?2004l` (bracketed paste), `\x1b[r` (scroll region), `\x1b[0m`, `\x1b[?25h`;
3. `vtui.SetAltScreen(true)`, `scr.HardReset()`, `pf.SetBusy(false)`,
   `FrameManager.Redraw()`.

**Primary screen не очищается ни при входе, ни при выходе** — в этом весь смысл: там копится
история, как в Far.

### 4.4. Немой PTY для ответов

`AnsiParser` и `TerminalView` отвечают в PTY на запросы: DA (`\x1b[?1;2c`), DSR/CPR, OSC 52,
far2l APC (`\x1b_far2lok\x07`), kitty. В host-режиме на них отвечает **хостовый терминал**,
поэтому зеркало обязано молчать, иначе ребёнок получит два ответа.

```go
type mutedPTY struct{ PtyBackend }
func (m mutedPTY) Write(p []byte) (int, error) { return len(p), nil }
```

В host-режиме `pf.parser.pty = mutedPTY{...}` и `pf.termView.pty = mutedPTY{...}`.
Обёртка **не должна** реализовывать `PtyPixelSizer` (иначе `TerminalView.syncPtyPixelSize`
пойдёт не туда). Пользовательский ввод по-прежнему идёт через
`pf.writePTY(pf.getActivePTY(), ...)` с настоящим бэкендом.

### 4.5. Ввод

`PanelsFrame.ProcessKey` при скрытых панелях уже транслирует событие в байты через
`TranslateInput(e, win32InputMode, kittyFlags, appCursorKeys)` и пишет в PTY. Сейчас сырой
форвардинг включается только при `!pf.showPanels && pf.isPtyBusy()`; в host-режиме форвардить
надо **всегда** при `!pf.showPanels` (интерактивный шелл — обычное состояние, а не занятость).

Исключения:
* `Ctrl+O` (`Panel.Toggle`) перехватывается f4, но только когда шелл простаивает — берём
  существующее условие `terminalquiet` (`hotkeys.go`), чтобы `Ctrl+O` внутри `vim`/`htop`
  уходил приложению;
* в Far-стиле клавиши редактирования командной строки f4 обрабатываются f4, а не шеллом,
  когда фокус на командной строке; переключение фокуса — та же логика, что и на панелях.

Флаги `Win32InputMode` / `KittyFlags` / `ApplicationCursorKeys` берутся из зеркала.

### 4.6. Размеры и resize

В host-режиме ребёнок владеет экраном, поэтому PTY получает размер **всего терминала**
`(w, h-n)` (см. 4.2), а не `(w, termH)` как сейчас в `ResizeConsole()` (там вычитаются
keybar/menubar). Зеркало `termView.Resize()` — тем же размером. Правка локализована одной
веткой по `pf.shellMode`.

Правило из `TERMINAL.md`: **resize 0×0 игнорировать**.

### 4.7. Окружение ребёнка

`child_env.go: buildChildEnv(env, graphics, kittyTerm)` в host-режиме вызывать с
`graphics=false, kittyTerm=false`: терминал теперь настоящий, `TERM` и `KITTY_WINDOW_ID`
хоста должны дойти до ребёнка нетронутыми, иначе f4 соврёт про свои возможности вместо
реальных. `F4_NESTED=1` и `TERM_PROGRAM=f4` оставить.

### 4.8. Запуск команды из командной строки f4

Не меняется: OSC 133-обёртка, `pf.executing`, `pf.returnToPanels`, автоматический возврат к
панелям по `OSC 133;D`. Разница только в том, куда идут байты. Получается поведение Far:
команда выполнилась → панели вернулись → `Ctrl+O` показывает консоль с её выводом.

## 5. Простые режимы (graceful degradation)

### 5.1. `ShellModeSimpleInline` (есть хостовый tty, PTY нет)

Резидентного шелла нет. Каждая команда: `vtui.Suspend()` →
`exec.Command(shell, "-c"/"/c", cmd)` с `Stdin/Stdout/Stderr = os.Stdin/os.Stdout/os.Stderr`
и `cmd.Dir = <путь активной панели>` → `Wait()` → «Press any key» → `vtui.Resume()`.
Прототип уже есть — `runExternalEditor()` в `actions.go`. `cd` и смена диска перехватываются
самим f4 до отправки в шелл, так что отсутствие резидентного шелла почти не заметно.

`Ctrl+O` здесь показывает primary screen (`vtui.SetAltScreen(false)`) в режиме «только
просмотр»: клавиатуру обрабатывает f4, любая клавиша возвращает панели. Живого промпта нет —
это честная деградация.

### 5.2. `ShellModeSimpleCaptured` (GUI / нет tty, PTY нет)

Код почти весь готов: `showRemoteCommandOutput(pf, NewLocalCommandRunner(), dir, cmd)`
(`remote_command.go` + `command_runner.go`) — окно со стриминговым выводом, скроллом и
отменой по закрытию. Плюс существующие `view:<<` / `edit:<<` / `clip:<<`
(`executeCapturedCommand`). Интерактивные программы не поддерживаются — тост
«terminal is not available in this environment».

## 6. Новое API в vtui

Нужна только возможность писать сырые байты в тот же поток, что и рендерер, не ломая теневой
буфер:

```go
// screenbuf.go
// WritePassthrough writes bytes straight to the terminal, bypassing the
// shadow buffer. Takes writeMu so it can never interleave with a frame.
func (s *ScreenBuf) WritePassthrough(p []byte)
```

Реализация: взять `s.writeMu`, писать в `s.Writer` (или `os.Stdout`, если nil) чанками по
8 КБ, как `AnsiRenderer.write`. Плюс тонкая обёртка `vtui.WritePassthrough(p []byte)` через
`FrameManager.scr`. Больше ничего добавлять не нужно: `SetAltScreen`, `HardReset`, `IsBusy`
уже есть.

Тест: `NewSilentScreenBuf()` с `Writer = &bytes.Buffer{}` — байты дошли дословно, параллельный
`Flush()` не перемешался.

## 7. План работ (Все этапы реализованы)

Каждый этап — отдельный коммит, компилируется, `go test ./...` зелёный.

* [x] **Этап 1.** Конфиг и разрешение режима (`ShellMode`, `resolveShellMode()`, `ConsoleMode`, `ConsoleOverlayUI`).
* [x] **Этап 2.** `vtui`: `WritePassthrough` в `ScreenBuf` и пакет `vtui`.
* [x] **Этап 3.** Роутер вывода: `console_passthrough.go`, `mutedPTY`, `enterHostConsole()` / `leaveHostConsole()` (базовый mc-стиль), read-loop в `initPTY()`.
* [x] **Этап 4.** Переключение и ввод (`Panel.Toggle`, `ProcessKey`, автовозврат по `OSC 133;D`, гарантированный `leaveHostConsole()` при `Close()`).
* [x] **Этап 5.** Far-стиль (`ConsoleOverlayUI`, scroll region, ручная отрисовка оверлея, троттлинг перерисовки ~30 мс, роутинг командной строки).
* [x] **Этап 6.** Простые режимы деградации (`ShellModeSimpleInline` и `ShellModeSimpleCaptured`).
* [x] **Этап 7.** Настройки и локализация (радиогруппа и чекбокс в `Panel Settings`, строки в `en.lng`/`ru.lng`, автолейаут-тест).
* [x] **Этап 8.** Документация и граничные случаи (интеграция с сессиями Unix daemon/detach, раздел в `TERMINAL.md`).

**Этап 0. Чтение.** `TERMINAL.md` (и `CONPTY_NATIVE_TEST.md` для Windows-проверки),
`UX_GUIDELINES.md`, `I18N.md`, `vtui/UI_TESTING.md`. Кода не писать.

**Этап 1. Конфиг и разрешение режима.** Новый `shell_mode.go` + `shell_mode_test.go`.
* `ShellMode`, `resolveShellMode()`, probe-переменные (4.1).
* В `config.go`: `F4Config.ConsoleMode string` (`"own"`|`"host"`, дефолт `"own"`) и
  `F4Config.ConsoleOverlayUI bool` (дефолт `false`) — поле, дефолт, чтение в `LoadConfig`
  (`ini.GetString("Panel", "ConsoleMode", "own")`), запись в `SaveConfig`; по образцу
  соседнего `KeepTerminalCursor`.
* Тесты: вся матрица из 4.1 через подмену probe-функций. Поведение приложения не меняется.

**Этап 2. vtui: `WritePassthrough`.** Отдельный PR в vtui (на время разработки — локальный
`replace` в `go.mod`). Тесты по разделу 6.

**Этап 3. Роутер вывода.** Новый `console_passthrough.go`.
* Поля `PanelsFrame`: `shellMode ShellMode`, `hostConsoleActive bool`.
* `mutedPTY` (4.4), `enterHostConsole()` / `leaveHostConsole()` (4.3), пока без оверлея.
* В read-loop внутри `initPTY()` — развилка: в host-режиме при активной консоли сначала
  `vtui.WritePassthrough(buf[:n])`, затем зеркало `pf.parser.Process(buf[:n])` **без**
  `FrameManager.Redraw()`.
* Ветка в `ResizeConsole()` (4.6) и вызов `buildChildEnv(..., false, false)` (4.7).
* Переключения по клавише ещё нет; проверяется юнит-тестами на фейковом PTY.

**Этап 4. Переключение и ввод.**
* Handler `Panel.Toggle` в `action_registry.go`: при `pf.shellMode == ShellModeHost`
  дополнительно зовёт `enterHostConsole()` / `leaveHostConsole()`.
* `ProcessKey`: сырой форвардинг при `pf.shellMode == ShellModeHost && !pf.showPanels`.
* Возврат панелей по `OSC 133;D` (`OnBusyChange`) зовёт `leaveHostConsole()`.
* `PanelsFrame.Show()`: при `hostConsoleActive` не рисовать ничего.
* `Close()` и выход из f4: гарантированно `leaveHostConsole()` **до** `vtui.Suspend()`,
  иначе терминал останется в кривом состоянии.

**Этап 5. Far-стиль (`ConsoleOverlayUI`).** Scroll region, `n`, ручная отрисовка оверлея,
троттлинг перерисовки, маршрутизация клавиш при фокусе на командной строке (4.2, 4.5).

**Этап 6. Простые режимы.** `simple_exec.go`: inline (5.1) и captured (5.2, обёртка вокруг
существующего). Точка входа — та же ветка обработки Enter в `panels_frame.go`, куда
добавляется `switch` по `pf.shellMode`.

**Этап 7. Настройки и локализация.**
* Радиогруппа «свой терминал / консоль хоста» + зависимый чекбокс «показывать командную
  строку поверх консоли» в `actionPanelSettings()` (`actions.go`).
* Строки `PanelSettings.ConsoleMode*` в `lang/en.lng` (базовый) и `lang/ru.lng`; остальные
  языки подхватят fallback (`I18N.md`).
* Подпись «применится к новым сессиям»; disabled-состояние с пояснением, если текущая среда
  всё равно откатится на «свой терминал» (GUI / нет tty).
* Тест разметки через `vtui.AssertLayout` — обязателен по правилам vtui.
* Раздел в `help.hlf`.

**Этап 8. Документация и края.** Раздел в `TERMINAL.md` (или этот файл в репозиторий);
перерисовка хвоста из зеркала после re-attach; при detach сессии сначала
`leaveHostConsole()`, потом отдача терминала.

## 8. Правила и запреты

1. **Не копировать код из mc и far2l.** f4 под BSD-3-Clause, mc под GPL. Исходники mc
   (`src/subshell.c`, `src/execute.c`) читать можно только как описание идеи.
2. Не менять поведение по умолчанию: без `ConsoleMode = host` всё должно работать
   байт-в-байт как раньше. Существующие тесты обязаны проходить без правок.
3. Не пытаться делать `tcsetpgrp` / `TIOCSCTTY` из демона (2.1) — это тупик.
4. Не восстанавливать cooked-режим терминала в host-режиме: f4 продолжает читать клавиатуру.
5. Соблюдать правила `TERMINAL.md`: порядок закрытия ConPTY (сначала хендл ConPTY, потом
   пайпы), игнор resize 0×0, отсутствие reflow живой сетки.
6. OS-специфичный код — только в файлах с build-тегами (`*_unix.go`, `*_windows.go`). CI
   (`.github/workflows/build.yml`) кросс-собирает под 9 GOOS, включая solaris/illumos/
   dragonfly, — сборка не должна ломаться нигде.
7. Каждый этап — с тестами; UI-диалоги — с `vtui.AssertLayout`.

## 9. Какие файлы нужны исполнителю

**f4, обязательно:** `panels_frame.go`, `terminal_view.go`, `ansi_parser.go`,
`input_translation.go`, `pty_interface.go`, `pty_unix.go`, `pty_windows.go`, `child_env.go`,
`process_environment_shell.go`, `config.go`, `action_registry.go`, `hotkeys.go`, `actions.go`,
`main.go`, `session_unix.go`, `session_windows.go`, `gui_unix.go`, `gui_windows.go`,
`detach_unix.go`, `command_runner.go`, `remote_command.go`, `terminal_log_vfs.go`,
`commands.go`.

**f4, документация и i18n:** `TERMINAL.md`, `CONPTY_NATIVE_TEST.md`, `UX_GUIDELINES.md`,
`I18N.md`, `README.md`, `lang/en.lng`, `lang/ru.lng`, `help.hlf`.

**f4, образцы стиля тестов:** `panels_frame_test.go`, `ansi_parser_test.go`, `pty_test.go`,
`config_test.go`.

**vtui:** `terminal_env.go`, `screenbuf.go`, `framemanager.go`, `baseframe.go`, `frame.go`,
`backend_info.go`, `win32_console_common.go`, `ansi_writer.go`, `README.md`, `UI_TESTING.md`,
`ARCHITECTURE.md`.

**vtinput:** `README.md` и файл с `Enable()` и ридером (только для понимания, править не надо).

**Внешнее, как справка по концепции (не для копирования):** mc — `src/subshell.c`,
`src/execute.c`, `lib/tty/tty.c`; far2l — `far2l/src/VT/VTShell.cpp` (для контраста: как в
этом режиме делать **не** надо).

## 10. Приёмка (host-режим в обычном терминале)

1. `Ctrl+O` из панелей → видна консоль с историей прошлых команд; повторный `Ctrl+O` →
   панели, содержимое консоли не потеряно.
2. Команда из командной строки f4 → панели уходят, вывод идёт живьём, по завершении панели
   возвращаются; `Ctrl+O` показывает этот вывод.
3. Колесо мыши в консоли скроллит **нативный** скроллбэк терминала; выделение мышью и
   `Ctrl+Shift+C` хоста работают.
4. `ping` / `yes` → `Ctrl+C` прерывает; `sleep 100` → `Ctrl+Z` + `fg` работают.
5. `vim`, `htop`, `mc` внутри: запускаются, `Ctrl+O` уходит им, при выходе экран
   восстанавливается без мусора.
6. Resize окна во время работы команды и в панелях — без артефактов.
7. `F10` из панелей и `exit` в консоли — терминал остаётся нормальным (не raw, не alt screen,
   курсор виден).
8. `F3` по логу терминала по-прежнему открывает накопленный лог (зеркало живо).
9. С `ConsoleOverlayUI` — командная строка и keybar держатся внизу, вывод не затекает под них,
   при выходе scroll region сброшен.
10. GUI (`--gui=x11`) с `ConsoleMode = host` → тихий откат на «свой терминал».
11. Wine → простая выполнялка, f4 не падает и не пытается создать ConPTY.
