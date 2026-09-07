package vtui

// Spacer represents an expanding layout spacer element.
type Spacer struct {
	ScreenObject
}

func NewSpacer() *Spacer {
	s := &Spacer{}
	s.SetGrowMode(GrowAll)
	return s
}

func (s *Spacer) Show(scr *ScreenBuf)          {}
func (s *Spacer) DisplayObject(scr *ScreenBuf) {}
