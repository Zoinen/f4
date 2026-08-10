package vtui

// Стандартные идентификаторы команд, общие для всего фреймворка.
const (
	CmValid         = iota // 0
	CmQuit                 // Выход из приложения
	CmOK                   // Подтверждение (ОК)
	CmCancel               // Отмена
	CmYes                  // Да
	CmNo                   // Нет
	CmDefault              // Действие по умолчанию (Enter в диалоге)
	CmClose                // Закрыть окно
	CmZoom                 // Развернуть/свернуть окно (F5)
	CmResize               // Изменить размер
	CmNext                 // Следующее окно
	CmPrev                 // Предыдущее окно
	CmHelp                 // Вызов справки (F1)
	CmReceivedFocus        // Фрейм получил focus
	CmReleasedFocus        // Фрейм потерял focus

	CmMenuLeft  = 302
	CmMenuRight = 303
	CmMenuClose = 304

	// CmApp is the starting offset for application-specific commands.
	CmApp = 1000
)

// CommandSet is a collection of command IDs, used to enable/disable groups of actions.
type CommandSet struct {
	mask map[int]bool
}

func NewCommandSet() CommandSet {
	return CommandSet{mask: make(map[int]bool)}
}

func (cs *CommandSet) Disable(cmd int) {
	if cs.mask == nil {
		cs.mask = make(map[int]bool)
	}
	cs.mask[cmd] = true
}

func (cs *CommandSet) Enable(cmd int) {
	if cs.mask == nil {
		return
	}
	delete(cs.mask, cmd)
}

func (cs *CommandSet) IsDisabled(cmd int) bool {
	if cs.mask == nil {
		return false
	}
	return cs.mask[cmd]
}

func (cs *CommandSet) Clear() {
	cs.mask = make(map[int]bool)
}
