package panel

func viewModeName(mode ViewMode) string {
	switch mode {
	case ViewModeBrief:
		return "brief"
	case ViewModeDetailed:
		return "detailed"
	case ViewModeWide:
		return "wide"
	default:
		return "medium"
	}
}

// SemanticNode экспортирует PanelsFrame в семантическое дерево ShellModel
