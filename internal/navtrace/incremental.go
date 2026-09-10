package navtrace

func SemanticIncrementalStageStart() int64 {
	if !NavigationBenchmarkIsEnabled() {
		return 0
	}
	return NavigationBenchmarkMonotonicNs()
}

func SemanticIncrementalStageDone(stage string, started int64, fields ...any) {
	if started == 0 {
		return
	}
	fields = append(fields, "stage", stage,
		"durationNs", NavigationBenchmarkMonotonicNs()-started)
	NavigationBenchmarkUIEvent("scene.incremental.stage", fields...)
}
