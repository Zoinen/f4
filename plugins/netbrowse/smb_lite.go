//go:build lite

package netbrowse

// smbBuilt is false in the lite build: no SMB client, so off Windows there is
// no network to browse.
const smbBuilt = false
