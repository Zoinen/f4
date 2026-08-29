#pragma once

#include <functional>
#include <memory>

class MacApplicationMenu final
{
public:
    using SettingsHandler = std::function<void()>;

    explicit MacApplicationMenu(SettingsHandler settingsHandler);
    ~MacApplicationMenu();

    MacApplicationMenu(const MacApplicationMenu &) = delete;
    MacApplicationMenu &operator=(const MacApplicationMenu &) = delete;

    bool install();
    bool installed() const;

private:
    struct Impl;
    std::unique_ptr<Impl> m_impl;
};
