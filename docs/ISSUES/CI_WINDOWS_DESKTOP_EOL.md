# CI: TestDesktopEntryMatchesThePackagedOne on Windows

Run 36864825515 (HEAD d9f9a85): `internal/install` failed on `windows/amd64 A-D`
and `windows/arm64`; every line of the two launchers matched once `Exec=`,
`TryExec=` and `X-F4-Managed=` were stripped, so the difference was invisible:
the Windows checkout (core.autocrlf) gave `packaging/linux/org.unxed.f4.desktop`
CRLF endings while `DesktopEntry` emits LF.

Fix: `*.desktop text eol=lf` in `.gitattributes`. The test is the regression test.

## Open, not fixed here

`windows/arm64` also failed `internal/terminal`
`TestConPTYPackageKeepsLongLinesWhole/powershell`: `powershell.exe` did not
finish in 90s (`pty_windows_test.go:85`, the only real error). The "left behind,
still loaded: ... conpty.dll: Access is denied" line next to it is just a
`t.Logf` from the test's cleanup, which expects the loaded DLL to stay.
Looks like a slow or hung PowerShell on the arm64 runner, not tied to this
change; it was not seen on amd64. It did not repeat in the next run on main
(run 36919996210, 57ed196, green), so it is one red out of two so far: if it
fails again, look at it for real.
