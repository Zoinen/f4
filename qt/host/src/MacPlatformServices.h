#pragma once

#include <QVariantMap>

#include <functional>
#include <memory>

// Public macOS integration used by the authenticated extui platform channel.
// All native UI is intentionally owned by the visible Qt host process.
class MacPlatformServices final {
public:
  using SendHandler = std::function<void(const QVariantMap &)>;

  explicit MacPlatformServices(SendHandler sendHandler);
  ~MacPlatformServices();

  MacPlatformServices(const MacPlatformServices &) = delete;
  MacPlatformServices &operator=(const MacPlatformServices &) = delete;

  void handleMessage(const QVariantMap &message);
  void cancelAll();

private:
  struct Impl;
  std::unique_ptr<Impl> m_impl;
};
