// Package netbrowse is f4's built-in network browser for Windows (f4#1702), in
// the spirit of Far Manager's Network panel: the network's providers and
// domains or workgroups, the servers in them and the shares of each server, as
// the system itself enumerates them (WNetOpenEnum and WNetEnumResource from
// mpr.dll), so what shows is what Explorer's "Network" shows and needs no
// protocol client of its own.
//
// Part 1 is the read-only browser: a panel that descends into every container
// (Enter) and back up (the ".." row) and lists the resources at each level.
// Part 2 makes the same network a drive of the ordinary file panel (Alt+F1,
// "Network", netvfs.go): a share opens as a normal directory there, read and
// written through the file system at its UNC name.
//
// Part 3 does the same off Windows over SMB (smbnet.go): the servers are the
// ones this session has reached (or a host name typed in the Network drive), the
// shares are what each server reports, and a share opens through the smb://
// provider of the NetFox plugin. Without an SMB client (the lite build) the
// plugin registers nothing off Windows.
package netbrowse
