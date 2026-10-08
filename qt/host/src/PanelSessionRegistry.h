#pragma once

#include "PanelCatalogModel.h"

#include <QObject>
#include <QPointer>

#include <array>
#include <functional>
#include <QMap>
#include <QStringList>

// Couples each retained panel catalog to its live session. The visible slots
// select a bounded LRU of panel identities, rather than replacing their models.
class PanelSessionRegistry final
{
public:
    static constexpr int PanelCount = 2;

    static bool validSide(int side);

    void setSession(int side, QObject *session);
    QObject *session(int side) const;
    QObject *sessionForPanel(int side, const QString &panelId) const;

    PanelCatalogModel &catalog(int side);
    const PanelCatalogModel &catalog(int side) const;
    const std::array<PanelCatalogModel, PanelCount> &catalogs() const;
    void resetCatalog(int side);
    void activatePanel(int side, const QString &panelId,
                       const std::function<QObject *()> &create,
                       const std::function<void(QObject *)> &release);

private:
    struct RetainedPanel {
        QPointer<QObject> session;
        PanelCatalogModel catalog;
    };
    static constexpr int RetainedPanelsPerSide = 7;
    std::array<QMap<QString, RetainedPanel>, PanelCount> m_retained;
    std::array<QStringList, PanelCount> m_recency;
    std::array<QString, PanelCount> m_panelIds;
    std::array<QPointer<QObject>, PanelCount> m_sessions;
    std::array<PanelCatalogModel, PanelCount> m_catalogs;
};
