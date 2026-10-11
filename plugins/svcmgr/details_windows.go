//go:build windows

package svcmgr

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

type platformDetailer struct{ machine string }

// Details reads the configuration through mgr.Service.Config, which needs only
// the query-configuration right on the handle it is given.
func (d platformDetailer) Details(name string) (serviceDetails, error) {
	var out serviceDetails
	err := withService(d.machine, name, windows.SERVICE_QUERY_CONFIG, func(h windows.Handle) error {
		cfg, err := (&mgr.Service{Name: name, Handle: h}).Config()
		if err != nil {
			return err
		}
		out = serviceDetails{
			StartType:    cfg.StartType,
			Delayed:      cfg.DelayedAutoStart,
			BinaryPath:   cfg.BinaryPathName,
			Account:      cfg.ServiceStartName,
			Description:  cfg.Description,
			DependsOn:    cfg.Dependencies,
			ErrorControl: cfg.ErrorControl,
		}
		// Recovery actions are optional: a service with none configured (or a
		// refused query) simply shows no recovery line.
		if actions, err := (&mgr.Service{Name: name, Handle: h}).RecoveryActions(); err == nil {
			for _, a := range actions {
				out.Recovery = append(out.Recovery, recoveryAction{Type: uint32(a.Type), DelaySec: uint32(a.Delay.Seconds())})
			}
		}
		return nil
	})
	return out, err
}
