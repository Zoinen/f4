//go:build !windows

package svcmgr

type platformDetailer struct{ machine string }

func (platformDetailer) Details(string) (serviceDetails, error) {
	return serviceDetails{}, errUnsupported
}
