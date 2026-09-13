# GUI-only settings correction

The earlier provider integration is superseded by the user's correction: GUI
preferences remain wholly in Qt and retain the original graphical configurator.

- [x] Remove GUI Go catalog/provider/transport and generated color controls.
- [x] Restore original configurator as an embeddable native component.
- [x] Add a Qt-only native settings page extension point and icon navigation.
- [x] Verify original editor behavior and every affected leaf at DPR 1.75.
- [x] Rebuild, audit, package and replace only the worktree binary after success.


Verification: native page and original editor behavior, 175% every-leaf scene
origins/unit transforms (including scrolled color list), 67 Qt QuickView tests,
51 operations/settings tests, Go settings/plughost/nativeui/architecture tests
and a Go regression for shared dialog identity/no console GUI category passed.
Static resource import and real compiled-host startup checks passed. The worktree
binary is replaced only after each successful portable build; prior binary saved.

Final packaged startup confirmed: extracted host hash equals the fresh static host.
