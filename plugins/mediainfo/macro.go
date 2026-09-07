package mediainfo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
)

// macroPair is deliberately a pair rather than a map. MediaInfo's macro
// result may contain the same English key once per stream and metadata tags
// may themselves be repeated. Both order and duplicates are observable by
// existing Far macros.
type macroPair struct {
	key   string
	value string
}

type macroArguments struct {
	path       string
	technical  bool
	filter     map[string]struct{}
	filterUsed bool
}

func (plugin *Plugin) callMacro(ctx context.Context, callContext vfs.MacroCallContext, arguments []any) ([]any, error) {
	parsed, err := parseMacroArguments(arguments)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	fs, path, item, err := macroSource(ctx, callContext.Current, parsed.path)
	if err != nil {
		return macroResult(false, nil, parsed), nil
	}
	report, err := plugin.analyzePath(ctx, fs, path, item, ModeDetailed)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return macroResult(false, nil, parsed), nil
	}

	pairs := macroTechnicalPairs(report)
	if !parsed.technical {
		pairs = macroImagePairs(report)
	}
	return macroResult(true, pairs, parsed), nil
}

func parseMacroArguments(arguments []any) (macroArguments, error) {
	parsed := macroArguments{technical: true, filter: make(map[string]struct{})}
	for index, argument := range arguments {
		switch value := argument.(type) {
		case nil:
			continue
		case bool:
			parsed.technical = value
		case int:
			parsed.technical = value != 0
		case int8:
			parsed.technical = value != 0
		case int16:
			parsed.technical = value != 0
		case int32:
			parsed.technical = value != 0
		case int64:
			parsed.technical = value != 0
		case uint:
			parsed.technical = value != 0
		case uint8:
			parsed.technical = value != 0
		case uint16:
			parsed.technical = value != 0
		case uint32:
			parsed.technical = value != 0
		case uint64:
			parsed.technical = value != 0
		case string:
			if value == "" {
				continue
			}
			if isMacroFilter(value) {
				parsed.filterUsed = true
				for _, key := range strings.Split(value[1:len(value)-1], ",") {
					parsed.filter[key] = struct{}{}
				}
				continue
			}
			// The original plugin accepts more than one filename and lets the
			// last non-empty one win. Keep that order-independent convention.
			parsed.path = value
		case []any:
			parsed.filterUsed = true
			if err := addMacroArrayFilter(parsed.filter, value); err != nil {
				return macroArguments{}, fmt.Errorf("MediaInfo macro argument %d: %w", index+1, err)
			}
		case []string:
			parsed.filterUsed = true
			for _, key := range value {
				parsed.filter[key] = struct{}{}
			}
		default:
			return macroArguments{}, fmt.Errorf("MediaInfo macro argument %d has unsupported type %T", index+1, argument)
		}
	}
	return parsed, nil
}

func isMacroFilter(value string) bool {
	return len(value) >= 2 && value[0] == '{' && value[len(value)-1] == '}'
}

func addMacroArrayFilter(filter map[string]struct{}, values []any) error {
	for index, value := range values {
		switch value := value.(type) {
		case nil:
			continue
		case string:
			filter[value] = struct{}{}
		default:
			return fmt.Errorf("filter item %d has unsupported type %T", index+1, value)
		}
	}
	return nil
}

func macroSource(ctx context.Context, current vfs.FileRef, requested string) (vfs.VFS, string, vfs.VFSItem, error) {
	fs := current.VFS
	if fs == nil {
		return nil, "", vfs.VFSItem{}, errors.New("MediaInfo macro has no active VFS")
	}

	path := requested
	if path == "" {
		path = current.Path
		if path == "" && current.Name != "" && current.Name != ".." {
			base := current.Dir
			if base == "" {
				base = fs.GetPath()
			}
			path = fs.Join(base, current.Name)
		}
	} else {
		// Expansion changes only the requested path string; the file remains
		// inside the active VFS and is still read through VFS.Open.
		if _, ok := fs.(*vfs.OSVFS); ok {
			path = expandOSPathEnvironment(path)
		}
		if !fs.IsAbs(path) {
			base := current.Dir
			if base == "" {
				base = fs.GetPath()
			}
			path = fs.Join(base, path)
		}
	}
	if path == "" {
		return nil, "", vfs.VFSItem{}, errors.New("MediaInfo macro has no selected file")
	}
	absolute, err := fs.Abs(path)
	if err != nil {
		return nil, "", vfs.VFSItem{}, err
	}
	item, err := fs.Stat(ctx, absolute)
	if err != nil {
		return nil, "", vfs.VFSItem{}, err
	}
	if item.IsDir {
		return nil, "", vfs.VFSItem{}, errMediaDirectory
	}
	return fs, absolute, item, nil
}

func macroResult(ok bool, pairs []macroPair, arguments macroArguments) []any {
	keys := make([]string, 0, len(pairs))
	values := make([]string, 0, len(pairs))
	if ok {
		for _, pair := range pairs {
			if pair.key == "" || pair.value == "" {
				continue
			}
			if arguments.filterUsed {
				if _, present := arguments.filter[pair.key]; !present {
					continue
				}
			}
			keys = append(keys, pair.key)
			values = append(values, pair.value)
		}
	}
	return []any{ok, int64(len(keys)), keys, values}
}

func macroTechnicalPairs(report Report) []macroPair {
	sections := CanonicalSections(report)
	pairs := make([]macroPair, 0, 64+len(report.Tags))
	for _, section := range sections {
		for _, field := range section.Fields {
			macroAdd(&pairs, field.Name, field.Value)
		}
	}
	return pairs
}

func macroImagePairs(report Report) []macroPair {
	pairs := make([]macroPair, 0, 24+len(report.Tags))
	for _, stream := range report.Streams {
		if stream.Image == nil {
			continue
		}
		image := stream.Image
		macroAdd(&pairs, "Width", plainInt(int64(image.Width)))
		macroAdd(&pairs, "Height", plainInt(int64(image.Height)))
		macroAdd(&pairs, "BitDepth", plainInt(int64(image.BitDepth)))
		macroAdd(&pairs, "ColorSpace", image.ColorModel)
		macroAdd(&pairs, "Compression_Mode", image.Compression)
		macroAdd(&pairs, "FrameCount", plainInt(int64(image.FrameCount)))
		if image.Animated {
			macroAdd(&pairs, "Animation", "Yes")
		}
		macroAdd(&pairs, "Duration", formatDuration(image.AnimationDuration))
		macroAdd(&pairs, "Orientation", plainInt(int64(image.Orientation)))
		macroAdd(&pairs, "DPIX", plainFloat(image.DPIX))
		macroAdd(&pairs, "DPIY", plainFloat(image.DPIY))
		macroAdd(&pairs, "Make", image.CameraMake)
		macroAdd(&pairs, "Model", image.CameraModel)
		macroAdd(&pairs, "LensModel", image.LensModel)
		if image.TakenAt != nil {
			macroAdd(&pairs, "DateTimeOriginal", image.TakenAt.Format("2006-01-02 15:04:05 MST"))
		}
		if image.Latitude != nil {
			macroAdd(&pairs, "GPSLatitude", plainFloat(*image.Latitude))
		}
		if image.Longitude != nil {
			macroAdd(&pairs, "GPSLongitude", plainFloat(*image.Longitude))
		}
		if image.GPSAltitude != nil {
			macroAdd(&pairs, "GPSAltitude", plainFloat(*image.GPSAltitude))
		}
		for _, field := range image.EXIF {
			macroAdd(&pairs, field.Name, field.Value)
		}
		for _, field := range stream.Tags {
			macroAdd(&pairs, field.Name, field.Value)
		}
	}
	for _, field := range report.Tags {
		macroAdd(&pairs, field.Name, field.Value)
	}
	return pairs
}

func macroAdd(pairs *[]macroPair, key, value string) {
	if key != "" && value != "" {
		*pairs = append(*pairs, macroPair{key: key, value: value})
	}
}
