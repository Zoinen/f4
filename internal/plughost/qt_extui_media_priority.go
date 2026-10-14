package plughost

import (
	"context"
	"sync"
)

// Admission keeps one of eight transport slots available for the image being
// opened. Focus changes wake queued requests and promote their existing owner.
// Active reads keep their permit until completion; shared flights are not killed.
type extUiMediaAdmission struct {
	mu         sync.Mutex
	changed    chan struct{}
	focus      string
	active     int
	background int
}

func (a *extUiMediaAdmission) notifyLocked() {
	if a.changed != nil {
		close(a.changed)
	}
	a.changed = make(chan struct{})
}

func (a *extUiMediaAdmission) setFocus(resourceID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.focus != resourceID {
		a.focus = resourceID
		a.notifyLocked()
	}
}

func (a *extUiMediaAdmission) acquire(ctx context.Context, resourceID string) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a.mu.Lock()
		foreground := a.focus != "" && a.focus == resourceID
		if a.active < 8 && (foreground || a.background < 7) {
			a.active++
			if !foreground {
				a.background++
			}
			a.mu.Unlock()
			return func() {
				a.mu.Lock()
				a.active--
				if !foreground {
					a.background--
				}
				a.notifyLocked()
				a.mu.Unlock()
			}, nil
		}
		if a.changed == nil {
			a.changed = make(chan struct{})
		}
		changed := a.changed
		a.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
