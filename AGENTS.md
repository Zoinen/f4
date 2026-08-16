# Local workspace instructions

## Mandatory portable-build policy

- Before changing, running, debugging, or reviewing any build, packaging,
  release, deployment, or CI workflow, read
  `docs/PORTABLE_BUILD_POLICY.md` completely.
- The platform contracts and verification gates in that document are required;
  do not replace them with a directory bundle on Linux or Windows, and do not
  replace the signed application bundle with runtime extraction on macOS.

## Mandatory build-and-run cycle

- After every change to workspace files, always produce a fresh canonical build of the affected application, including the Go core and QML/Qt frontend when either participates in the result.
- Only after the fresh build succeeds, close every already-running `f4` instance that belongs to this workspace together with its Qt host child, then launch the newly built canonical QML frontend.
- Never leave the previous application instance running after a successful rebuild. Verify that the old processes exited and that the new Go and Qt host processes are alive.
- If the build fails, keep the last working application instance alive, fix the build, and repeat the cycle before handing the task back to the user.

## Canonical f4 binary

- Always build the user-facing Go application binary only as `/Users/zoin/Documents/f4/f4` in the project root.
- Never create, build, copy, or launch alternate `f4` application binaries in subdirectories, temporary directories, or under any other filename.
- Supporting Qt/QML build artifacts may remain in their one canonical configured CMake build directory, but the application must always be launched through the root `/Users/zoin/Documents/f4/f4` binary.
