//go:build windows

package winshell

import (
	"fmt"
	"runtime"

	"github.com/zzl/go-win32api/v2/win32"
)

type staResult struct {
	value any
	err   error
}

type staJob struct {
	fn   func() (any, error)
	done chan staResult
}

type shellApartment struct {
	jobs chan staJob
	done chan struct{}
}

func newShellApartment() (*shellApartment, error) {
	a := &shellApartment{
		jobs: make(chan staJob),
		done: make(chan struct{}),
	}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(a.done)

		hr := win32.CoInitializeEx(nil, win32.COINIT_APARTMENTTHREADED)
		if win32.FAILED(hr) {
			ready <- fmt.Errorf("initialize Windows Shell apartment: %s", win32.HRESULT_ToString(hr))
			return
		}
		defer win32.CoUninitialize()
		ready <- nil

		for job := range a.jobs {
			result := staResult{}
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						result.err = fmt.Errorf("Windows Shell broker panic: %v", recovered)
					}
				}()
				result.value, result.err = job.fn()
			}()
			job.done <- result
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return a, nil
}

func (a *shellApartment) call(fn func() (any, error)) (any, error) {
	if a == nil {
		return nil, fmt.Errorf("Windows Shell apartment is unavailable")
	}
	done := make(chan staResult, 1)
	select {
	case a.jobs <- staJob{fn: fn, done: done}:
	case <-a.done:
		return nil, fmt.Errorf("Windows Shell apartment is closed")
	}
	select {
	case result := <-done:
		return result.value, result.err
	case <-a.done:
		return nil, fmt.Errorf("Windows Shell apartment is closed")
	}
}

func (a *shellApartment) close() {
	if a == nil {
		return
	}
	close(a.jobs)
	<-a.done
}
