// Package svcmgr is f4's built-in Windows Service Manager plugin (f4#311), in
// the spirit of FAR Manager 3's SvcMgr: a panel listing the services of the
// local machine.
//
// Part 1 is read-only: the panel shows every Win32 service with its name,
// display name, state and process id, and F5 reloads the list. The list comes
// from the Service Control Manager (EnumServicesStatusEx through
// golang.org/x/sys/windows), opened with the enumerate right only, so it works
// without administrator rights. On every other OS the plugin registers
// nothing, the way plugins/proclist leaves a panel quietly absent where it has
// no collector. Starting, stopping and configuring services are later parts.
package svcmgr
