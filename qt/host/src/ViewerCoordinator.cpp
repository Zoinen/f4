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
    setVisibleState(side == 0 || side == 1, side);
}

void ViewerCoordinator::hide()
{
    setVisibleState(false, -1);
}

void ViewerCoordinator::setVisibleState(bool visible, int side)
{
    const int normalizedSide = visible && (side == 0 || side == 1)
        ? side : -1;
    const bool normalizedVisible = normalizedSide >= 0;
    if (m_visible == normalizedVisible && m_side == normalizedSide) {
        return;
    }
    m_visible = normalizedVisible;
    m_side = normalizedSide;
    emit changed();
}
