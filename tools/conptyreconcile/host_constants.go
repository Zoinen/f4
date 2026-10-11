package main

const (
	pinnedOpenConsoleVersion = "1.12.220408003-release1.12"
	pinnedOpenConsoleSHA256  = "14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e"
	pinnedOpenConsoleCommit  = "e9b4e2e18fb1b9cee6839969d42cd0f95d228926"
)

// pinnedHostOutputPipeSize is the buffer of the pipe the pinned host writes its
// output into. CreatePipe with 0 gives the 4 KiB default, and the host writes
// synchronously, so on a megabyte of output it blocks on every refill until we
// have read the pipe empty -- hundreds of stalls that the host's own paint
// coalescing cannot hide. A megabyte lets it run ahead of the reader (idea 1.3 of
// unxed/f4#1681; the effect is measured with the capture suite, not assumed).
const pinnedHostOutputPipeSize = 1 << 20
