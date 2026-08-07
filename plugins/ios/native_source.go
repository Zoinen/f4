package iosfs

import (
	"context"
	"fmt"
	"strings"

	goios "github.com/danielpaulus/go-ios/ios"
)

// nativeDeviceSource discovers devices through the platform usbmuxd service.
// It intentionally treats Lockdown failures as per-device state: one phone
// waiting for Trust must not hide another phone that is already usable.
type nativeDeviceSource struct{}

func (nativeDeviceSource) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list, err := goios.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("ios: list usbmuxd devices: %w", err)
	}

	// usbmuxd can report the same UDID through USB and Wi-Fi. Keep one row and
	// prefer USB because it has lower latency and can establish CoreDevice.
	entries := make(map[string]goios.DeviceEntry, len(list.DeviceList))
	for _, entry := range list.DeviceList {
		udid := strings.TrimSpace(entry.Properties.SerialNumber)
		if udid == "" {
			continue
		}
		previous, ok := entries[udid]
		if !ok || preferNativeEntry(entry, previous) {
			entries[udid] = entry
		}
	}

	result := make([]DeviceInfo, 0, len(entries))
	for udid, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info := DeviceInfo{
			UDID:           udid,
			ConnectionType: entry.ConnectionTypeLabel(),
			State:          DeviceStateReady,
			Paired:         true,
		}
		values, valuesErr := goios.GetValuesPlist(entry)
		if valuesErr != nil {
			info.State, info.Paired = lockdownFailureState(valuesErr)
			result = append(result, info)
			continue
		}
		info.Name = plistString(values, "DeviceName")
		info.Model = firstNonEmpty(plistString(values, "HardwareModel"), plistString(values, "ModelNumber"))
		info.ProductType = plistString(values, "ProductType")
		info.OSVersion = plistString(values, "ProductVersion")
		info.BuildVersion = plistString(values, "BuildVersion")
		info.DeveloperMode = plistBool(values, "DeveloperModeStatus")
		result = append(result, info)
	}
	return result, nil
}

func preferNativeEntry(candidate, current goios.DeviceEntry) bool {
	candidateUSB := strings.EqualFold(candidate.ConnectionTypeLabel(), "USB")
	currentUSB := strings.EqualFold(current.ConnectionTypeLabel(), "USB")
	if candidateUSB != currentUSB {
		return candidateUSB
	}
	return candidate.DeviceID < current.DeviceID
}

func lockdownFailureState(err error) (string, bool) {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "pair"), strings.Contains(message, "hostid"), strings.Contains(message, "trust"):
		return DeviceStateUnpaired, false
	case strings.Contains(message, "locked"), strings.Contains(message, "password protected"), strings.Contains(message, "passcode"):
		return DeviceStateLocked, true
	default:
		return DeviceStateOffline, true
	}
}

func plistString(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func plistBool(values map[string]interface{}, key string) bool {
	switch value := values[key].(type) {
	case bool:
		return value
	case uint64:
		return value != 0
	case int64:
		return value != 0
	case string:
		return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "enabled")
	default:
		return false
	}
}

func resolveNativeDevice(ctx context.Context, device DeviceInfo) (goios.DeviceEntry, error) {
	if err := ctx.Err(); err != nil {
		return goios.DeviceEntry{}, err
	}
	list, err := goios.ListDevices()
	if err != nil {
		return goios.DeviceEntry{}, fmt.Errorf("ios: refresh device %q: %w", device.UDID, err)
	}
	var selected goios.DeviceEntry
	found := false
	for _, candidate := range list.DeviceList {
		if candidate.Properties.SerialNumber != device.UDID {
			continue
		}
		if !found || preferNativeEntry(candidate, selected) {
			selected = candidate
			found = true
		}
	}
	if !found {
		return goios.DeviceEntry{}, fmt.Errorf("ios: device %q: %w", device.UDID, ErrDeviceUnavailable)
	}
	return selected, nil
}

var _ DeviceSource = nativeDeviceSource{}
