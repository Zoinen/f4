#include "ViewerCoordinator.h"

ViewerCoordinator::ViewerCoordinator(QObject *parent)
    : QObject(parent)
{
}

void ViewerCoordinator::beginPending(int side, const QString &panelId,
                                     const QString &entryId,
                                     qulonglong catalogRevision)
{
    m_pendingIntent = PendingIntent{
        true, side, panelId, entryId, catalogRevision,
    };
}

void ViewerCoordinator::clearPending()
{
    m_pendingIntent = PendingIntent{};
}

void ViewerCoordinator::show(int side)
{
    if (side < 0 || side > 1) { hide(); return; }
    setState(m_state == Docked && side == m_side ? Expanding : Full, side);
}
void ViewerCoordinator::dock(int sourceSide, int destinationSide)
{
    if (sourceSide < 0 || sourceSide > 1 || destinationSide < 0 || destinationSide > 1) return;
    const bool changedDock = m_dockSide != destinationSide;
    m_dockSide = destinationSide;
    if (!visible()) {
        if (m_state == Docked && m_side == sourceSide && changedDock) emit changed();
        else setState(Docked, sourceSide);
    }
    else if (changedDock) emit changed();
}
void ViewerCoordinator::removeDock()
{
    if (m_dockSide < 0) return;
    m_dockSide = -1;
    if (m_state == Docked) hide();
    else if (m_state == Collapsing) setState(Full, m_side);
    else emit changed();
}
void ViewerCoordinator::collapse()
{
    if (m_dockSide >= 0 && visible()) setState(Collapsing, m_side);
}
void ViewerCoordinator::settle()
{
    if (m_state == Expanding) setState(Full, m_side);
    else if (m_state == Collapsing) setState(Docked, m_side);
}
void ViewerCoordinator::hide()
{
    m_dockSide = -1;
    setState(Closed, -1);
}
void ViewerCoordinator::setState(State state, int side)
{
    if (m_state == state && m_side == side) return;
    m_state = state;
    m_side = side;
    emit changed();
}
