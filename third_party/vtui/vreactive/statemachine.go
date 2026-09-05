package vreactive

// StateMachine helps defining declarative UI states.
type StateMachine struct {
	State Property[string]
	rules map[string][]func()
}

func NewStateMachine(initialState string) *StateMachine {
	sm := &StateMachine{
		State: NewProperty(initialState),
		rules: make(map[string][]func()),
	}
	sm.State.OnChange(func(newState string) {
		if setters, ok := sm.rules[newState]; ok {
			for _, s := range setters {
				s()
			}
		}
	})
	return sm
}

func (sm *StateMachine) AddState(name string, setters ...func()) {
	sm.rules[name] = setters
}

// SetProp is a convenient helper for AddState to declare target property values.
func SetProp[T any](p Property[T], val T) func() {
	return func() { p.Set(val) }
}
