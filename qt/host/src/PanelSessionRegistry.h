#pragma once

#include "PanelCatalogModel.h"

#include <QObject>
#include <QPointer>

#include <array>

// Owns the fixed left/right panel identity boundary for the Qt host.  The
// bridge coordinates protocol and interaction, while this registry is the
// single place that couples a live GallerySession to its typed catalog state.
class PanelSessionRegistry final
{
public:
    static constexpr int PanelCount = 2;

    static bool validSide(int side);

    void setSession(int side, QObject *session);
    QObject *session(int side) const;

    PanelCatalogModel &catalog(int side);
    const PanelCatalogModel &catalog(int side) const;
    const std::array<PanelCatalogModel, PanelCount> &catalogs() const;
    void resetCatalog(int side);

private:
    std::array<QPointer<QObject>, PanelCount> m_sessions;
    std::array<PanelCatalogModel, PanelCount> m_catalogs;
};
