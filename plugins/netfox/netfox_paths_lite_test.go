//go:build lite

package netfox

// Lite retains the saved-connection net:// provider, not SFTP/SCP/SMB.
const netFoxURIRegistrationsForTest = 1
