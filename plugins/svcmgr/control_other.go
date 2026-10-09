//go:build !windows

package svcmgr

type platformController struct{ machine string }

func (platformController) Start(string) error                      { return errUnsupported }
func (platformController) Stop(string) error                       { return errUnsupported }
func (platformController) Pause(string) error                      { return errUnsupported }
func (platformController) Resume(string) error                     { return errUnsupported }
func (platformController) SetStartType(string, uint32, bool) error { return errUnsupported }
func (platformController) SetConfig(string, serviceConfig) error   { return errUnsupported }
