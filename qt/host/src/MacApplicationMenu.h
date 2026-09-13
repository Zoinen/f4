#pragma once

#include <functional>
#include <memory>
#include <QVariantMap>
#include <QByteArray>

class MacApplicationMenu final
{
public:
    using SettingsHandler = std::function<void()>;
    using ActionHandler = std::function<void(const QVariantMap &)>;
    using IconRenderer = std::function<QByteArray(const QString &)>;

    explicit MacApplicationMenu(SettingsHandler settingsHandler, ActionHandler actionHandler = {}, IconRenderer iconRenderer = {});
    ~MacApplicationMenu();

    MacApplicationMenu(const MacApplicationMenu &) = delete;
    MacApplicationMenu &operator=(const MacApplicationMenu &) = delete;

    bool install();
    bool installed() const;
    void synchronize(const QVariantMap &menuBar);

private:
    struct Impl;
    std::unique_ptr<Impl> m_impl;
};
