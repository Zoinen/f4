package svcmgr

// controller starts, stops, pauses and resumes services by name. The Windows
// implementation talks to the Service Control Manager, opening each service
// with only the right its action needs, so a user allowed to start a service
// but not to stop it gets exactly that; every other OS reports
// errUnsupported.
type controller interface {
	Start(name string) error
	Stop(name string) error
	Pause(name string) error
	Resume(name string) error
	// SetStartType changes how the service starts: one of the startAuto,
	// startManual and startDisabled values; delayed asks for a delayed
	// automatic start and only means something with startAuto.
	SetStartType(name string, startType uint32, delayed bool) error
	// SetConfig changes the fields of the service properties dialog at once.
	SetConfig(name string, c serviceConfig) error
}

// serviceConfig is what the service properties dialog edits.
type serviceConfig struct {
	DisplayName  string
	BinaryPath   string
	StartType    uint32
	Delayed      bool
	ErrorControl uint32
}

// serviceController makes the controller for a computer (empty: this one); a
// test replaces it.
var serviceController = func(machine string) controller { return platformController{machine: machine} }
