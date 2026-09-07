#pragma once

namespace F4ApplicationIcon
{
// macOS application icons are owned by LaunchServices and the application
// bundle. Other platforms use Qt's runtime window-icon fallback.
bool isBundleManaged() noexcept;
void installRuntimeFallback();
}
