# Лунобот-2: покрытие `vfs/sudo_askpass_unix`

- Claim: кастомная задача «покрыть тестами пакет vfs/sudo_askpass_unix», часть 1 из 1.
- Основание выбора: отчёт Codecov для `main` `e40b44db248b72deb958a8fe6c70ac6bca31e349`; файл имел 3.64% покрытия (4/110 строк). Более свежий отчёт для быстро меняющегося `main` ещё не опубликован Codecov.
- Изменение: добавлены Unix-only тесты subprocess askpass helper, Unix socket handshake, missing-parent exit, listen failure, attempt limit и nil FrameManager path.
- Локально выполнены только `gofmt` и `git diff --check`; Go build/test не запускались согласно `LUNOBOT.md`.
- Hosted CI run `34334340207` на exact head `a3065179dc0e81ba5d9682712f40412b532ee154` завершился failure: `Lint (rest)` из-за G204 на запуске текущего test binary, а `Test (linux/amd64)`, `Test (linux/arm64)` и `Race (packages)` зависли в `TestAskpassServerReturnsWhenSocketCannotBeCreated` и завершились по timeout.
- Исправление: путь для listen-failure теперь занят каталогом, а намеренный запуск текущего test binary помечен `#nosec G204`.
- Следующий hosted CI run `34337108768` на exact head `1c407a28b7198542207d30b76fb25238943cb83b` также завершился failure: `Lint (rest)` из-за G702 на запуске test binary, а Linux test/race jobs снова зависли в `TestAskpassServerReturnsWhenSocketCannotBeCreated`; существующий каталог всё ещё был принят Unix listener.
- Исправление: путь listen-failure теперь содержит несуществующий родительский каталог, а запуск test binary помечен `#nosec G204 G702`.
- Финальный hosted CI run `34339306299` на exact head `ad82426bcec7a1196766aa32399ac57b71865d48` завершился `completed/success`; все 26 jobs прошли без ошибок.
