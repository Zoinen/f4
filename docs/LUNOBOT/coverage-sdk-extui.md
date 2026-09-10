# Лунобот-2: покрытие `sdk/extui`

- Claim: кастомная задача «покрыть sdk/extui», часть 1 из 1.
- Основание выбора: отчёт Codecov для `main` `e40b44db248b72deb958a8fe6c70ac6bca31e349`; пакет имел 26.49% покрытия (71/268 строк).
- Изменение: добавлен `sdk/extui/model_coverage_test.go`, покрывающий сериализацию scene, shell, panel, entry, command line, terminal, surface, menu, key bar, dialog, control, row/run и все коллекционные helpers, включая optional и legacy-ветви.
- Локально выполнены только `gofmt` и `git diff --check`; Go build/test не запускались согласно `LUNOBOT.md`.
- Hosted CI: ожидается для PR.
