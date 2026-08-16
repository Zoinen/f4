package main

import (
	"os"
	"os/user"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"unicode"

	"github.com/unxed/vtui"
)

// These values are populated by packaged builds with -ldflags -X. Development
// builds fall back to the VCS settings embedded by the Go toolchain and then to
// "(devel)". Version discovery must remain process-local: running Git here used
// to add several hundred milliseconds to every application startup.
var (
	buildVersion  string
	buildRevision string
	buildModified string
	buildTime     string
)

var (
	titleOnce     sync.Once
	cachedHost    string
	cachedUser    string
	cachedAdmin   string
	cachedVersion string
	cachedPlat    string
)

type rawVersionInfo struct {
	version  string
	revision string
	modified string
	time     string
}

type resolvedVersionInfo struct {
	version  string
	revision string
	dirty    string
	time     string
}

var processVersionInfo = sync.OnceValue(func() resolvedVersionInfo {
	var info *debug.BuildInfo
	if current, ok := debug.ReadBuildInfo(); ok {
		info = current
	}
	return resolveVersionInfo(rawVersionInfo{
		version:  buildVersion,
		revision: buildRevision,
		modified: buildModified,
		time:     buildTime,
	}, info)
})

func initTitleCache() {
	h, _ := os.Hostname()
	cachedHost = h

	u, err := user.Current()
	if err == nil && u != nil {
		cachedUser = u.Username
		if idx := strings.LastIndex(cachedUser, "\\"); idx != -1 {
			cachedUser = cachedUser[idx+1:]
		}
	} else {
		cachedUser = "user"
	}

	cachedAdmin = getAdminString()
	cachedVersion = getShortVersionInfo()
	cachedPlat = runtime.GOARCH
}

func isReleaseVersion(v string) bool {
	if !strings.HasPrefix(v, "v") {
		return false
	}
	s := v[1:]
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			return false
		}
	}
	return true
}

func resolveVersionInfo(build rawVersionInfo, info *debug.BuildInfo) resolvedVersionInfo {
	version := strings.TrimSpace(build.version)
	revision := strings.TrimSpace(build.revision)
	modified := strings.TrimSpace(build.modified)
	timeStr := strings.TrimSpace(build.time)

	if info != nil {
		if version == "" {
			candidate := strings.TrimSpace(info.Main.Version)
			if candidate != "(devel)" {
				version = candidate
			}
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if revision == "" {
					revision = strings.TrimSpace(setting.Value)
				}
			case "vcs.modified":
				if modified == "" {
					modified = strings.TrimSpace(setting.Value)
				}
			case "vcs.time":
				if timeStr == "" {
					timeStr = strings.TrimSpace(setting.Value)
				}
			}
		}
	}

	if version == "" {
		version = "(devel)"
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	dirty := ""
	if strings.EqualFold(modified, "true") {
		dirty = "-dirty"
	}
	if len(timeStr) >= 16 {
		timeStr = strings.Replace(timeStr[:16], "T", " ", 1)
	} else {
		timeStr = ""
	}

	return resolvedVersionInfo{
		version:  version,
		revision: revision,
		dirty:    dirty,
		time:     timeStr,
	}
}

func (info resolvedVersionInfo) short() string {
	if isReleaseVersion(info.version) {
		return info.version + info.dirty
	}
	if info.revision != "" {
		return info.revision + info.dirty
	}
	return info.version
}

func (info resolvedVersionInfo) long() string {
	var sb strings.Builder
	if isReleaseVersion(info.version) {
		sb.WriteString(info.version + info.dirty)
	} else if info.revision != "" {
		sb.WriteString(info.revision + info.dirty)
	} else {
		sb.WriteString(info.version)
	}
	if info.time != "" {
		sb.WriteString(" [" + info.time + "]")
	}
	return sb.String()
}

func getShortVersionInfo() string {
	return processVersionInfo().short()
}

func getLongVersionInfo() string {
	return processVersionInfo().long()
}

func UpdateWindowTitle(scr *vtui.ScreenBuf) {
	titleOnce.Do(initTitleCache)

	if vtui.FrameManager == nil {
		return
	}

	state := "Panels"
	if len(vtui.FrameManager.Screens) > 0 {
		state = stableWorkspaceTitle(vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx])
	}

	template := AppConfig.ConsoleTitleTemplate
	if template == "" {
		template = "f4 - %State"
	}

	r := strings.NewReplacer(
		"%State", state,
		"%Ver", cachedVersion,
		"%Platform", cachedPlat,
		"%Backend", getBackendName(),
		"%Host", cachedHost,
		"%User", cachedUser,
		"%Admin", cachedAdmin,
	)

	title := r.Replace(template)
	title = strings.ReplaceAll(title, "  ", " ") // Убираем двойные пробелы, если %Admin пустой
	vtui.SetWindowTitle(title)

	// Macro recording indicator — drawn after MenuBar so it's always on top
	if MacroMgr != nil && MacroMgr.Recording {
		scr.Write(0, 0, vtui.StringToCharInfo(" R ", vtui.SetRGBBoth(0, 0xFFFFFF, 0xFF0000)))
	}
}

func stableWorkspaceTitle(screen *vtui.AppScreen) string {
	// Keep compatibility while the corresponding VTUI API is being reviewed.
	// Once available, the structural assertion starts using it automatically.
	if provider, ok := any(screen).(interface{ GetWorkspaceTitle() string }); ok {
		return provider.GetWorkspaceTitle()
	}
	for i := len(screen.Frames) - 1; i >= 0; i-- {
		if screen.Frames[i].IsModal() {
			continue
		}
		if title := strings.TrimSpace(screen.Frames[i].GetTitle()); title != "" {
			return title
		}
	}
	return screen.GetTitle()
}

func getBackendName() string {
	if vtui.FrameManager == nil {
		return "Console"
	}
	return vtui.FrameManager.GetBackendName()
}
