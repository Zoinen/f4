# Android device list on parent navigation

The Android manager did not implement DirectoryCacheKeyProvider. Device mounts
already opted into the panel cache, but navigating up to their manager discarded
the previous device rows until ADB discovery returned.

ManagerVFS now opts into the existing panel cache with its own instance identity.
The same manager is retained as the mounted device's parent. Independent managers
remain isolated, matching their independent device-row lookup maps.

Regression: TestAndroidParentNavigationShowsCachedDevicesDuringDiscovery creates
a real Android manager with a controllable discovery source, visits a mounted
child, and navigates up while discovery is blocked. Before the fix the selected
row was empty. After the fix the cached device and loading state are immediately
available; releasing discovery replaces it with a fresh device and clears loading.

The existing remote directory cache regression and Android package tests pass.
