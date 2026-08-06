package androidfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	deviceInfoCacheTTL   = 10 * time.Second
	deviceInfoTimeout    = 2 * time.Second
	deviceInfoMaxOutput  = int64(256 << 10)
	deviceInfoPropMarker = "__F4_ANDROID_INFO_PROP__"
	deviceInfoDFMarker   = "__F4_ANDROID_INFO_DF__"
	deviceInfoMemMarker  = "__F4_ANDROID_INFO_MEM__"
	deviceInfoUpMarker   = "__F4_ANDROID_INFO_UPTIME__"
	deviceInfoBatMarker  = "__F4_ANDROID_INFO_BATTERY__"
	deviceInfoKernelMark = "__F4_ANDROID_INFO_KERNEL__"
	deviceInfoStorage    = "/sdcard"
)

type deviceInfoCommandFunc func(context.Context, string, string, int64) (shellResult, error)

type deviceInfoFacts struct {
	manufacturer  string
	brand         string
	model         string
	android       string
	api           string
	securityPatch string
	build         string
	abis          string
	hardware      string
	platform      string
	bootloader    string
	baseband      string
	kernel        string

	battery      string
	memoryTotal  uint64
	memoryAvail  uint64
	uptime       string
	storageTotal uint64
	storageAvail uint64
	storageMount string
}

type deviceInfoCache struct {
	mu sync.Mutex

	device     DeviceInfo
	facts      deviceInfoFacts
	generation uint64

	fetchedAt   time.Time
	refreshing  bool
	refreshDone chan struct{}

	run     deviceInfoCommandFunc
	now     func() time.Time
	ttl     time.Duration
	timeout time.Duration
}

type deviceInfoService struct {
	mu     sync.Mutex
	caches map[string]*deviceInfoCache
	run    deviceInfoCommandFunc
}

type devicePanelInfoProvider struct {
	cache         *deviceInfoCache
	backend       string
	backendDetail string
}

func newDeviceInfoService(server *Server) *deviceInfoService {
	service := &deviceInfoService{caches: make(map[string]*deviceInfoCache)}
	if server != nil {
		service.run = func(ctx context.Context, serial, command string, maxOutput int64) (shellResult, error) {
			stdout, stderr, exitCode, err := server.RunShellLimited(ctx, serial, command, maxOutput)
			return shellResult{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}, err
		}
	}
	return service
}

func (s *deviceInfoService) provider(device DeviceInfo, backend, backendDetail string) *devicePanelInfoProvider {
	if s == nil || strings.TrimSpace(device.Serial) == "" {
		return nil
	}
	serial := strings.TrimSpace(device.Serial)
	s.mu.Lock()
	if s.caches == nil {
		s.caches = make(map[string]*deviceInfoCache)
	}
	cache := s.caches[serial]
	if cache == nil {
		cache = &deviceInfoCache{
			device:     device,
			generation: 1,
			run:        s.run,
			now:        time.Now,
			ttl:        deviceInfoCacheTTL,
			timeout:    deviceInfoTimeout,
		}
		s.caches[serial] = cache
	} else {
		cache.mu.Lock()
		if cache.device != device {
			// ADB serials are normally stable hardware identifiers, but emulator
			// slots and network endpoints can be reused. Transport/model changes
			// describe a new connection identity: never present the previous
			// device's getprop facts as a fresh snapshot for it.
			cache.device = device
			cache.facts = deviceInfoFacts{}
			cache.fetchedAt = time.Time{}
			cache.generation++
		} else {
			cache.device = device
		}
		cache.mu.Unlock()
	}
	s.mu.Unlock()
	return &devicePanelInfoProvider{cache: cache, backend: backend, backendDetail: backendDetail}
}

func normalizeDeviceInfoPath(p string) string {
	if !path.IsAbs(p) {
		return "/"
	}
	return path.Clean(p)
}

func buildDeviceInfoCommand() string {
	// The root path is commonly a small read-only system partition, not the
	// capacity users recognise as device storage. Prefer Android's primary
	// shared-storage view, with vendor/legacy fallbacks to its canonical target
	// and the userdata volume.
	storageProbe := strings.Join([]string{
		"df -k " + quoteShellArg(deviceInfoStorage) + " 2>/dev/null",
		"df -k '/storage/emulated/0' 2>/dev/null",
		"df -k '/data' 2>/dev/null",
	}, " || ")
	return strings.Join([]string{
		"printf '" + deviceInfoPropMarker + "\\n'", "getprop 2>/dev/null",
		"printf '" + deviceInfoDFMarker + "\\n'", storageProbe,
		"printf '" + deviceInfoMemMarker + "\\n'", "cat /proc/meminfo 2>/dev/null",
		"printf '" + deviceInfoUpMarker + "\\n'", "cat /proc/uptime 2>/dev/null",
		"printf '" + deviceInfoBatMarker + "\\n'", "dumpsys battery 2>/dev/null",
		"printf '" + deviceInfoKernelMark + "\\n'", "uname -a 2>/dev/null",
	}, "; ")
}

func (c *deviceInfoCache) isFreshLocked(now time.Time) bool {
	return !c.fetchedAt.IsZero() && now.Sub(c.fetchedAt) >= 0 && now.Sub(c.fetchedAt) < c.ttl
}

func (c *deviceInfoCache) snapshotState() (DeviceInfo, deviceInfoFacts, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	return c.device, c.facts, c.fetchedAt, c.isFreshLocked(now)
}

func (c *deviceInfoCache) refresh(ctx context.Context) error {
	if c == nil || c.run == nil {
		return errors.New("android: device information command runner is unavailable")
	}
	for {
		c.mu.Lock()
		now := time.Now()
		if c.now != nil {
			now = c.now()
		}
		if c.isFreshLocked(now) {
			c.mu.Unlock()
			return nil
		}
		if c.refreshing {
			done := c.refreshDone
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				continue
			}
		}
		c.refreshing = true
		c.refreshDone = make(chan struct{})
		serial := c.device.Serial
		generation := c.generation
		timeout := c.timeout
		c.mu.Unlock()

		probeCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			probeCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		result, err := c.run(probeCtx, serial, buildDeviceInfoCommand(), deviceInfoMaxOutput)
		if cancel != nil {
			cancel()
		}
		var parsed deviceInfoFacts
		if err == nil {
			if result.ExitCode > 0 {
				err = fmt.Errorf("android: device information command exited with %d: %s", result.ExitCode, strings.TrimSpace(string(result.Stderr)))
			} else {
				parsed, err = parseDeviceInfoOutput(result.Stdout)
			}
		}

		c.mu.Lock()
		if err == nil && c.generation != generation {
			err = errors.New("android: device identity changed during information refresh")
		}
		if err == nil {
			c.facts = mergeDeviceInfoFacts(c.facts, parsed)
			if c.now != nil {
				c.fetchedAt = c.now()
			} else {
				c.fetchedAt = time.Now()
			}
		}
		c.refreshing = false
		close(c.refreshDone)
		c.refreshDone = nil
		c.mu.Unlock()
		return err
	}
}

func mergeDeviceInfoFacts(old, fresh deviceInfoFacts) deviceInfoFacts {
	mergeText := func(dst *string, src string) {
		if src != "" {
			*dst = src
		}
	}
	mergeText(&old.manufacturer, fresh.manufacturer)
	mergeText(&old.brand, fresh.brand)
	mergeText(&old.model, fresh.model)
	mergeText(&old.android, fresh.android)
	mergeText(&old.api, fresh.api)
	mergeText(&old.securityPatch, fresh.securityPatch)
	mergeText(&old.build, fresh.build)
	mergeText(&old.abis, fresh.abis)
	mergeText(&old.hardware, fresh.hardware)
	mergeText(&old.platform, fresh.platform)
	mergeText(&old.bootloader, fresh.bootloader)
	mergeText(&old.baseband, fresh.baseband)
	mergeText(&old.kernel, fresh.kernel)
	mergeText(&old.battery, fresh.battery)
	mergeText(&old.uptime, fresh.uptime)
	if fresh.memoryTotal != 0 {
		old.memoryTotal = fresh.memoryTotal
	}
	if fresh.memoryAvail != 0 {
		old.memoryAvail = fresh.memoryAvail
	}
	// Storage always describes the primary user-visible volume. Clear it if a
	// later probe cannot find any of the supported mount points rather than
	// retaining a plausible-looking stale capacity.
	old.storageTotal = fresh.storageTotal
	old.storageAvail = fresh.storageAvail
	old.storageMount = fresh.storageMount
	return old
}

func parseDeviceInfoOutput(output []byte) (deviceInfoFacts, error) {
	sections := make(map[string][]string)
	current := ""
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 4096), int(deviceInfoMaxOutput))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		switch line {
		case deviceInfoPropMarker, deviceInfoDFMarker, deviceInfoMemMarker, deviceInfoUpMarker, deviceInfoBatMarker, deviceInfoKernelMark:
			current = line
		default:
			if current != "" {
				sections[current] = append(sections[current], line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return deviceInfoFacts{}, err
	}
	if _, ok := sections[deviceInfoPropMarker]; !ok {
		return deviceInfoFacts{}, errors.New("android: device information output has no property section")
	}

	props := parseGetprop(sections[deviceInfoPropMarker])
	facts := deviceInfoFacts{
		manufacturer:  props["ro.product.manufacturer"],
		brand:         props["ro.product.brand"],
		model:         props["ro.product.model"],
		android:       props["ro.build.version.release"],
		api:           props["ro.build.version.sdk"],
		securityPatch: props["ro.build.version.security_patch"],
		build:         firstNonEmpty(props["ro.build.display.id"], props["ro.build.id"]),
		abis:          firstNonEmpty(props["ro.product.cpu.abilist"], props["ro.product.cpu.abi"]),
		hardware:      props["ro.hardware"],
		platform:      props["ro.board.platform"],
		bootloader:    props["ro.bootloader"],
		baseband:      firstNonEmpty(props["gsm.version.baseband"], props["ro.baseband"]),
	}
	if kernel := firstNonBlankLine(sections[deviceInfoKernelMark]); kernel != "" {
		facts.kernel = kernel
	}
	facts.memoryTotal, facts.memoryAvail = parseMemInfo(sections[deviceInfoMemMarker])
	facts.uptime = parseUptime(sections[deviceInfoUpMarker])
	facts.battery = parseBattery(sections[deviceInfoBatMarker])
	facts.storageTotal, facts.storageAvail, facts.storageMount = parseDF(sections[deviceInfoDFMarker])
	return facts, nil
}

func parseGetprop(lines []string) map[string]string {
	props := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			if split := strings.Index(line, "]: ["); split > 1 && strings.HasSuffix(line, "]") {
				props[line[1:split]] = line[split+4 : len(line)-1]
				continue
			}
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			props[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return props
}

func parseMemInfo(lines []string) (total, available uint64) {
	values := make(map[string]uint64)
	for _, line := range lines {
		key, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil || value > math.MaxUint64/1024 {
			continue
		}
		values[strings.TrimSpace(key)] = value * 1024
	}
	total = values["MemTotal"]
	available = values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	return total, available
}

func parseUptime(lines []string) string {
	line := firstNonBlankLine(lines)
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return ""
	}
	totalMinutes := int64(seconds) / 60
	days := totalMinutes / (24 * 60)
	hours := totalMinutes / (60) % 24
	minutes := totalMinutes % 60
	if days > 0 {
		return fmt.Sprintf("%dd %02dh %02dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func parseBattery(lines []string) string {
	values := make(map[string]string)
	for _, line := range lines {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok {
			values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	var parts []string
	if level, err := strconv.Atoi(values["level"]); err == nil && level >= 0 && level <= 100 {
		parts = append(parts, fmt.Sprintf("%d%%", level))
	}
	for _, power := range []struct{ key, label string }{
		{"ac powered", "AC"}, {"usb powered", "USB"}, {"wireless powered", "wireless"},
	} {
		if strings.EqualFold(values[power.key], "true") {
			parts = append(parts, power.label)
			break
		}
	}
	if raw, err := strconv.Atoi(values["temperature"]); err == nil && raw > -1000 && raw < 2000 {
		parts = append(parts, fmt.Sprintf("%.1f °C", float64(raw)/10))
	}
	return strings.Join(parts, " · ")
}

func parseDF(lines []string) (total, available uint64, mount string) {
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) < 6 || strings.EqualFold(fields[0], "Filesystem") {
			continue
		}
		totalKB, totalErr := strconv.ParseUint(fields[1], 10, 64)
		availKB, availErr := strconv.ParseUint(fields[3], 10, 64)
		if totalErr != nil || availErr != nil || totalKB > math.MaxUint64/1024 || availKB > math.MaxUint64/1024 {
			continue
		}
		return totalKB * 1024, availKB * 1024, fields[len(fields)-1]
	}
	return 0, 0, ""
}

func firstNonBlankLine(lines []string) string {
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func panelInfoText(id, labelKey, label, value string) vfs.PanelInfoField {
	return vfs.PanelInfoField{ID: id, LabelKey: labelKey, Label: label, Value: value, Kind: vfs.PanelInfoText}
}

func panelInfoUsage(id, labelKey, label string, total, available uint64) vfs.PanelInfoField {
	return vfs.PanelInfoField{
		ID: id, LabelKey: labelKey, Label: label, Kind: vfs.PanelInfoUsage,
		TotalBytes: total, AvailableBytes: available,
	}
}

func appendTextField(fields []vfs.PanelInfoField, id, key, label, value string) []vfs.PanelInfoField {
	if strings.TrimSpace(value) == "" {
		return fields
	}
	return append(fields, panelInfoText(id, key, label, value))
}

func deviceBaselineSnapshot(device DeviceInfo) vfs.PanelInfoSnapshot {
	fields := make([]vfs.PanelInfoField, 0, 6)
	fields = appendTextField(fields, "model", "AndroidInfo.Model", "Model", device.Model)
	fields = appendTextField(fields, "serial", "AndroidInfo.Serial", "Serial", device.Serial)
	fields = appendTextField(fields, "state", "AndroidInfo.State", "State", device.State)
	fields = appendTextField(fields, "product", "AndroidInfo.Product", "Product", device.Product)
	fields = appendTextField(fields, "device", "AndroidInfo.Device", "Device", device.Device)
	fields = appendTextField(fields, "transport", "AndroidInfo.TransportID", "Transport ID", device.TransportID)
	return vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID: "android.device", TitleKey: "AndroidInfo.DeviceTitle", Title: "Android device", Fields: fields,
		}},
	}
}

func buildDevicePanelInfoSnapshot(device DeviceInfo, facts deviceInfoFacts, fetchedAt time.Time, backend, backendDetail string) vfs.PanelInfoSnapshot {
	model := firstNonEmpty(facts.model, device.Model)
	if facts.manufacturer != "" && model != "" && !strings.Contains(strings.ToLower(model), strings.ToLower(facts.manufacturer)) {
		model = facts.manufacturer + " " + model
	}
	androidVersion := facts.android
	if facts.api != "" {
		if androidVersion != "" {
			androidVersion += " (API " + facts.api + ")"
		} else {
			androidVersion = "API " + facts.api
		}
	}

	deviceFields := make([]vfs.PanelInfoField, 0, 8)
	deviceFields = appendTextField(deviceFields, "model", "AndroidInfo.Model", "Model", model)
	deviceFields = appendTextField(deviceFields, "serial", "AndroidInfo.Serial", "Serial", device.Serial)
	deviceFields = appendTextField(deviceFields, "backend", "AndroidInfo.Backend", "Backend", backend)
	deviceFields = appendTextField(deviceFields, "protocol", "AndroidInfo.Protocol", "Protocol", backendDetail)
	deviceFields = appendTextField(deviceFields, "android", "AndroidInfo.AndroidVersion", "Android", androidVersion)
	deviceFields = appendTextField(deviceFields, "build", "AndroidInfo.Build", "Build", facts.build)
	deviceFields = appendTextField(deviceFields, "security_patch", "AndroidInfo.SecurityPatch", "Security patch", facts.securityPatch)
	deviceFields = appendTextField(deviceFields, "abi", "AndroidInfo.ABI", "ABI", facts.abis)

	statusFields := make([]vfs.PanelInfoField, 0, 4)
	statusFields = appendTextField(statusFields, "battery", "AndroidInfo.Battery", "Battery", facts.battery)
	if facts.memoryTotal != 0 {
		statusFields = append(statusFields, panelInfoUsage(
			"memory", "AndroidInfo.Memory", "Memory", facts.memoryTotal, facts.memoryAvail))
	}
	statusFields = appendTextField(statusFields, "uptime", "AndroidInfo.Uptime", "Uptime", facts.uptime)
	if facts.storageTotal != 0 {
		statusFields = append(statusFields, panelInfoUsage(
			"storage", "AndroidInfo.Storage", "Storage", facts.storageTotal, facts.storageAvail))
	}

	sections := make([]vfs.PanelInfoSection, 0, 2)
	sections = append(sections, vfs.PanelInfoSection{ID: "android.device", TitleKey: "AndroidInfo.DeviceTitle", Title: "Android device", Fields: deviceFields})
	if len(statusFields) != 0 {
		sections = append(sections, vfs.PanelInfoSection{ID: "android.status", TitleKey: "AndroidInfo.StatusTitle", Title: "Device status", Fields: statusFields})
	}
	return vfs.PanelInfoSnapshot{Authoritative: true, Sections: sections, RefreshedAt: fetchedAt}
}

func (p *devicePanelInfoProvider) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if p == nil || p.cache == nil {
		return ""
	}
	device, _, _, _ := p.cache.snapshotState()
	return fmt.Sprintf("android:%q:%q:%q:%q:%q:%q:%s",
		device.Serial, device.TransportID, device.Model, device.Product, device.Device,
		p.backend, normalizeDeviceInfoPath(req.Path))
}

func (p *devicePanelInfoProvider) CachedPanelInfo(_ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	if p == nil || p.cache == nil {
		return vfs.PanelInfoSnapshot{}, true
	}
	device, facts, fetchedAt, fresh := p.cache.snapshotState()
	return buildDevicePanelInfoSnapshot(device, facts, fetchedAt, p.backend, p.backendDetail), fresh
}

func (p *devicePanelInfoProvider) RefreshPanelInfo(ctx context.Context, _ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if p == nil || p.cache == nil {
		return vfs.PanelInfoSnapshot{}, errors.New("android: device information provider is unavailable")
	}
	err := p.cache.refresh(ctx)
	device, facts, fetchedAt, _ := p.cache.snapshotState()
	return buildDevicePanelInfoSnapshot(device, facts, fetchedAt, p.backend, p.backendDetail), err
}

func syncBackendDetail(features map[string]bool) string {
	wanted := []string{"shell_v2", "stat_v2", "ls_v2", "sendrecv_v2"}
	var enabled []string
	for _, feature := range wanted {
		if features[feature] {
			enabled = append(enabled, feature)
		}
	}
	sort.Strings(enabled)
	return strings.Join(enabled, ", ")
}

var _ vfs.PanelInfoProvider = (*devicePanelInfoProvider)(nil)
