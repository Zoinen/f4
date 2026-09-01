#include "PanelSessionRegistry.h"

#include <QtGlobal>

bool PanelSessionRegistry::validSide(int side)
{
    return side >= 0 && side < PanelCount;
}

void PanelSessionRegistry::setSession(int side, QObject *session)
{
    Q_ASSERT(validSide(side));
    m_sessions[static_cast<size_t>(side)] = session;
}

QObject *PanelSessionRegistry::session(int side) const
{
    return validSide(side)
        ? m_sessions[static_cast<size_t>(side)].data() : nullptr;
}

PanelCatalogModel &PanelSessionRegistry::catalog(int side)
{
    Q_ASSERT(validSide(side));
    return m_catalogs[static_cast<size_t>(side)];
}

const PanelCatalogModel &PanelSessionRegistry::catalog(int side) const
{
    Q_ASSERT(validSide(side));
    return m_catalogs[static_cast<size_t>(side)];
}

const std::array<PanelCatalogModel, PanelSessionRegistry::PanelCount> &
PanelSessionRegistry::catalogs() const
{
    return m_catalogs;
}

void PanelSessionRegistry::resetCatalog(int side)
{
    catalog(side) = PanelCatalogModel{};
}
