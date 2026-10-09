package dotnet

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
)

// What the report lists at most: a large assembly has tens of thousands of
// types, and the report is a summary, not a decompiler.
const (
	maxReportRefs      = 200
	maxReportTypes     = 300
	maxReportResources = 100
)

// code wraps s as a Markdown code span, whatever it contains.
func code(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "'") + "`"
}

// Report renders info as Markdown in the interface language.
func Report(info *Info, file string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", code(file))
	if info.Name != "" {
		fmt.Fprintf(&b, "**%s** %s %s", i18n.Msg("DotNet.Assembly"), code(info.Name), code(info.Version.String()))
		if info.Culture != "" {
			fmt.Fprintf(&b, ", %s %s", i18n.Msg("DotNet.Culture"), code(info.Culture))
		}
		b.WriteString("\n\n")
	} else {
		fmt.Fprintf(&b, "%s\n\n", i18n.Msg("DotNet.Module"))
	}
	fmt.Fprintf(&b, "**%s** %s\n\n", i18n.Msg("DotNet.Runtime"), code(info.RuntimeVersion))

	fmt.Fprintf(&b, "## %s (%d)\n\n", i18n.Msg("DotNet.References"), len(info.References))
	for i, ref := range info.References {
		if i == maxReportRefs {
			fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(i18n.Msg("DotNet.More"), len(info.References)-i))
			break
		}
		fmt.Fprintf(&b, "- %s %s\n", code(ref.Name), code(ref.Version.String()))
	}

	fmt.Fprintf(&b, "\n## %s (%d)\n\n", i18n.Msg("DotNet.Types"), info.TypeCount)
	shown := 0
	for _, ns := range info.Namespaces() {
		names := info.Types[ns]
		if shown >= maxReportTypes {
			fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(i18n.Msg("DotNet.More"), info.TypeCount-shown))
			break
		}
		title := ns
		if title == "" {
			title = i18n.Msg("DotNet.GlobalNamespace")
		}
		fmt.Fprintf(&b, "### %s (%d)\n\n", code(title), len(names))
		for _, name := range names {
			if shown >= maxReportTypes {
				break
			}
			fmt.Fprintf(&b, "- %s\n", code(name))
			shown++
		}
		b.WriteString("\n")
	}

	if len(info.Resources) > 0 {
		fmt.Fprintf(&b, "\n## %s (%d)\n\n", i18n.Msg("DotNet.Resources"), len(info.Resources))
		for i, name := range info.Resources {
			if i == maxReportResources {
				fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(i18n.Msg("DotNet.More"), len(info.Resources)-i))
				break
			}
			fmt.Fprintf(&b, "- %s\n", code(name))
		}
	}
	return b.String()
}
