//go:build !lite

package netbrowse

// smbBuilt reports that the build links the SMB client (NetFox), so off Windows
// the network is browsed over SMB (f4#1702, part 3).
const smbBuilt = true
