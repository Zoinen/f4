//go:build (linux && !android) || freebsd || dragonfly || openbsd || netbsd || solaris || hurd

package panel

const desktopAssociationCommand = "xdg-open"
