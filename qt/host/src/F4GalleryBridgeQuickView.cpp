#include "F4GalleryBridge.h"
#include "ViewerCoordinator.h"
#include <ZoinGallery/GallerySession.h>
#include <ZoinGallery/MediaTimingTrace.h>

bool F4GalleryBridge::viewerMounted() const { return m_viewerCoordinator->mounted(); }
int F4GalleryBridge::viewerState() const { return m_viewerCoordinator->state(); }
int F4GalleryBridge::quickViewSide() const { return m_viewerCoordinator->dockSide(); }

QVariantMap F4GalleryBridge::quickView() const
{
    auto view = m_quickView;
    const int side = view.value("sourceSide", -1).toInt();
    if (viewerVisible() || !validSide(side) || view.value("id").toString().isEmpty()
        || view.value("imageRenderer").toString() != "gallery") return view;
    const auto &pending = m_pendingCursors[static_cast<size_t>(side)];
    const auto &catalog = m_panelSessions.catalog(side);
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionForSide(side));
    if (!pending.active || !session || pending.panelId != view.value("sourcePanelId").toString()
        || pending.catalogRevision != catalog.catalogRevision
        || pending.catalogRevision != view.value("catalogRevision").toULongLong()) return view;
    const int index = session->indexForEntryId(pending.entryId);
    if (index < 0 || !session->isImageAt(index)) return view;
    // Native navigation is optimistic until key release. Decode this image
    // from the existing catalog now, without waiting for the Go round trip.
    view["entryId"] = pending.entryId;
    view["previewKind"] = "image";
    return view;
}

void F4GalleryBridge::synchronizeQuickView(const QVariantMap &incomingView)
{
    const bool changed = m_quickView != incomingView;
    m_quickView = incomingView;
    const auto view = quickView();
    const int sourceSide = view.value("sourceSide", -1).toInt();
    const int destinationSide = view.value("side", -1).toInt();
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionForSide(sourceSide));
    const bool nativeImage = view.value("previewKind").toString() == "image"
        && view.value("imageRenderer").toString() == "gallery"
        && m_quickViewPreferences && !m_quickViewPreferences->builtin()
        && available() && validSide(sourceSide) && validSide(destinationSide)
        && sourceSide != destinationSide && session
        && session->indexForEntryId(view.value("entryId").toString()) >= 0
        && view.value("sourcePanelId").toString() == m_panelSessions.catalog(sourceSide).panelId
        && view.value("catalogRevision").toULongLong() == m_panelSessions.catalog(sourceSide).catalogRevision
        && (!viewerVisible() || viewerSide() == sourceSide);
    if (!nativeImage) {
        const int previousSide = viewerSide();
        const bool wasDocked = viewerState() == ViewerCoordinator::Docked;
        m_viewerCoordinator->removeDock();
        if (wasDocked) {
            if (auto *previousSession = qobject_cast<ZoinGallery::GallerySession *>(sessionForSide(previousSide)))
                previousSession->setViewerOpen(false);
        }
    } else {
        if (viewerMounted() && viewerSide() != sourceSide) closeViewer();
        session->setViewerOpen(true);
        m_viewerCoordinator->dock(sourceSide, destinationSide);
    }
    if (changed) emit viewerChanged();
}

void F4GalleryBridge::expandQuickView()
{
    if (viewerState() != ViewerCoordinator::Docked) return;
    // This entry can be a temporary hover preview. Explicit viewer interaction
    // commits it through the ordinary stable-ID cursor route before promotion.
    const QString id = quickView().value("entryId").toString();
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(viewerSession());
    if (!session) return;
    const int index = session->indexForEntryId(id);
    if (index < 0) return;
    requestOpen(viewerSide(), id, session->sourceIndexAt(index), true, session->catalogRevision());
}

void F4GalleryBridge::collapseQuickView()
{
    m_viewerCoordinator->collapse();
}

void F4GalleryBridge::settleViewer()
{
    m_viewerCoordinator->settle();
    ZoinGallery::MediaTimingTrace::event("qt.quickview.presentation",
        {{"state", viewerState()}, {"sourceSide", viewerSide()}, {"dockSide", quickViewSide()}});
}

void F4GalleryBridge::requestViewerCursor(const QString &entryId, int index)
{
    if (!validSide(viewerSide())) return;
    if (viewerState() == ViewerCoordinator::Docked) {
        sendPanelAction(viewerSide(), QStringLiteral("panel.cursor"), entryId, index,
                        m_panelSessions.catalog(viewerSide()).catalogRevision, true, false);
    } else {
        requestCursor(viewerSide(), entryId, index,
                      m_panelSessions.catalog(viewerSide()).catalogRevision);
    }
}
