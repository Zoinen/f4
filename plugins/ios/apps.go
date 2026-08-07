package iosfs

import (
	"context"
	"fmt"
	"sort"
	"strings"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
)

type nativeAppSource struct{}

func (nativeAppSource) ListApps(ctx context.Context, device DeviceInfo) ([]AppInfo, error) {
	entry, err := resolveNativeDevice(ctx, device)
	if err != nil {
		return nil, err
	}
	connection, err := installationproxy.New(entry)
	if err != nil {
		return nil, fmt.Errorf("ios: connect installation proxy: %w", err)
	}
	defer connection.Close()
	apps, err := connection.BrowseUserApps()
	if err != nil {
		return nil, fmt.Errorf("ios: list installed applications: %w", err)
	}

	result := make([]AppInfo, 0, len(apps))
	for _, raw := range apps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bundleID := raw.CFBundleIdentifier()
		if bundleID == "" {
			continue
		}
		displayName := appString(raw, installationproxy.CFBundleDisplayName)
		if displayName == "" {
			displayName = raw.CFBundleName()
		}
		entitlements := appMap(raw, installationproxy.Entitlements)
		groups := collectAppGroups(entitlements, appMap(raw, "GroupContainers"))
		signer := strings.ToLower(appString(raw, "SignerIdentity"))
		developerSigned := mapBool(entitlements, "get-task-allow") ||
			strings.Contains(signer, "development") || strings.Contains(signer, "developer")
		result = append(result, AppInfo{
			BundleID: bundleID, DisplayName: displayName,
			FileSharing: raw.UIFileSharingEnabled(), DeveloperSigned: developerSigned,
			GroupIDs: groups,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DisplayName == result[j].DisplayName {
			return result[i].BundleID < result[j].BundleID
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, nil
}

func appString(app installationproxy.AppInfo, key string) string {
	value, _ := app[key].(string)
	return strings.TrimSpace(value)
}

func appMap(app installationproxy.AppInfo, key string) map[string]interface{} {
	value, _ := app[key].(map[string]interface{})
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

func mapBool(values map[string]interface{}, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func collectAppGroups(entitlements, containers map[string]interface{}) []string {
	groups := make(map[string]struct{})
	for key := range containers {
		if key = strings.TrimSpace(key); key != "" {
			groups[key] = struct{}{}
		}
	}
	add := func(value interface{}) {
		switch values := value.(type) {
		case []interface{}:
			for _, raw := range values {
				if group, ok := raw.(string); ok {
					if group = strings.TrimSpace(group); group != "" {
						groups[group] = struct{}{}
					}
				}
			}
		case []string:
			for _, group := range values {
				if group = strings.TrimSpace(group); group != "" {
					groups[group] = struct{}{}
				}
			}
		}
	}
	add(entitlements["com.apple.security.application-groups"])
	result := make([]string, 0, len(groups))
	for group := range groups {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func resolveAppDevice(ctx context.Context, device DeviceInfo) (goios.DeviceEntry, error) {
	return resolveNativeDevice(ctx, device)
}

var _ AppSource = nativeAppSource{}
