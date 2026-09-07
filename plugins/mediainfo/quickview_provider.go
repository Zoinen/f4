package mediainfo

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
)

var quickViewMediaExtensions = map[string]struct{}{
	".3g2": {}, ".3gp": {}, ".aif": {}, ".aifc": {}, ".aiff": {},
	".avi": {}, ".bw64": {}, ".flac": {}, ".m1a": {}, ".m2a": {},
	".m4a": {}, ".m4b": {}, ".m4v": {}, ".mka": {}, ".mks": {}, ".mkv": {},
	".mov": {}, ".mp1": {}, ".mp2": {}, ".mp3": {}, ".mp4": {},
	".oga": {}, ".ogg": {}, ".ogm": {}, ".ogv": {}, ".opus": {},
	".rf64": {}, ".rifx": {}, ".spx": {}, ".wav": {}, ".webm": {},
}

type mediaQuickViewProvider struct {
	plugin *Plugin
}

func (provider *mediaQuickViewProvider) Name() string  { return quickViewID }
func (provider *mediaQuickViewProvider) Priority() int { return 40 }

func (provider *mediaQuickViewProvider) CanPreview(request vfs.QuickViewRequest) bool {
	if provider == nil || provider.plugin == nil || !provider.plugin.settings().EnableQuickView {
		return false
	}
	if request.VFS == nil || request.Item.IsDir {
		return false
	}
	name := request.Item.Name
	if name == "" {
		name = request.Path
	}
	_, supported := quickViewMediaExtensions[strings.ToLower(filepath.Ext(name))]
	return supported
}

func (provider *mediaQuickViewProvider) Preview(ctx context.Context, request vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
	if !provider.CanPreview(request) {
		return vfs.QuickViewResult{}, vfs.ErrQuickViewUnsupported
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	report, err := provider.plugin.analyzePath(ctx, request.VFS, request.Path, request.Item, ModeFast)
	if err != nil {
		var parseError *ParseError
		if errors.Is(err, ErrUnsupported) || errors.As(err, &parseError) {
			return vfs.QuickViewResult{}, vfs.ErrQuickViewUnsupported
		}
		return vfs.QuickViewResult{}, err
	}
	text := provider.plugin.render(report, false)
	lines, _ := splitRenderedTextLines(text, renderedTruncationNotice(provider.plugin.reportLanguage()))
	if len(lines) == 1 && lines[0] == "" {
		return vfs.QuickViewResult{}, vfs.ErrQuickViewUnsupported
	}
	label := report.General.Format
	if label == "" {
		label = provider.plugin.text("MediaInfo.QuickViewLabel", "Media information", "Информация о медиа")
	}
	return vfs.QuickViewResult{Label: label, Lines: lines}, nil
}
