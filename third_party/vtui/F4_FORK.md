# f4 vtui fork

This directory is based on `github.com/unxed/vtui` v0.1.129.

f4 carries one integration patch: `PeriodicRedrawRenderer` allows a native
renderer to opt out of vtui's terminal-oriented 250 ms redraw heartbeat. The
default remains enabled, so terminal and other existing renderers retain their
cursor blinking and animation behavior. f4's external Qt renderer returns
`false` because Qt owns cursor blinking and all model changes already request
redraws through vtui's event, task, resize, and explicit redraw paths.

Remove the local `replace` from f4's `go.mod` once this capability is available
in an upstream vtui release.
