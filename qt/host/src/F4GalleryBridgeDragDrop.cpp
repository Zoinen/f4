#include "F4GalleryBridge.h"
#include "F4NativeDragVisuals.h"
#include <ZoinGallery/GallerySession.h>
#include <QAbstractItemModel>
#include <QQmlEngine>
#include <QQuickImageProvider>
#include <QQuickItemGrabResult>
#include <QIcon>

#include <QCoreApplication>
#include <QDir>
#include <QDrag>
#include <QDropEvent>
#include <QGuiApplication>
#include <QMimeData>
#include <QMouseEvent>
#include <QKeyEvent>
#include <QQuickItem>
#include <QQuickWindow>
#include <QStyleHints>
#include <QTimer>
#include <QPainter>
#include <QUuid>
#include <QCursor>
#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif

namespace {
const char *sessionMime = "application/x-f4-drag-session";
}

void F4GalleryBridge::registerDragPanel(int side, QQuickItem *item)
{
    if (!validSide(side)) return;
    m_dragPanels[side] = item;
    qApp->installEventFilter(this);
}

void F4GalleryBridge::registerDragWorkspaceBar(QQuickItem *item)
{
    m_dragWorkspaceBar = item;
    qApp->installEventFilter(this);
}

QVariantMap F4GalleryBridge::dragWorkspaceHit(QObject *window, const QPointF &position) const
{
    auto *bar = m_dragWorkspaceBar.data();
    if (!bar || bar->window() != window || !bar->isVisible() || !bar->isEnabled()
        || QGuiApplication::modalWindow()) return {};
    const auto point = bar->mapFromScene(position);
    if (!bar->contains(point)) return {};
    QVariant hit;
    if (!QMetaObject::invokeMethod(bar, "dragWorkspaceHit", Q_RETURN_ARG(QVariant, hit),
            Q_ARG(QVariant, point.x()), Q_ARG(QVariant, point.y()))) return {};
    return hit.toMap();
}

QVariantMap F4GalleryBridge::dragEndpoint(int side, int sourceIndex) const
{
    if (!validSide(side)) return {};
    const auto &state = m_panelSessions.catalog(side);
    if (!state.initialized || state.loading) return {};
    QVariantMap result{{"side", side}, {"panelId", state.panelId},
        {"path", state.currentPath}, {"catalogRevision", state.catalogRevision}};
    if (sourceIndex >= 0) {
        // sourceIndex is the Go index returned by GalleryPanelController.
        for (const auto &value : state.entries) {
            const auto row = value.toMap();
            if (row.value("index").toInt() != sourceIndex) continue;
            result.insert("entryId", row.value("entryId"));
            result.insert("index", sourceIndex);
            result.insert("name", row.value("name"));
            result.insert("isDir", row.value("isDir"));
            return result;
        }
        return {}; // A not-yet-loaded page is not an empty-area drop.
    }
    return result;
}

QVariantMap F4GalleryBridge::dragHit(QObject *window, const QPointF &pos, int *side) const
{
    *side = -1;
    if (QGuiApplication::modalWindow()) return {};
    for (int i = 0; i < 2; ++i) {
        auto *item = m_dragPanels[i].data();
        if (!item || item->window() != window || !item->isVisible() || !item->isEnabled()
            || !item->property("dropInputEnabled").toBool()) continue;
        const auto point = item->mapFromScene(pos);
        if (!item->contains(point)) continue;
        QVariant hit;
        if (!QMetaObject::invokeMethod(item, "dragHit", Q_RETURN_ARG(QVariant, hit),
                Q_ARG(QVariant, point.x()), Q_ARG(QVariant, point.y()))) continue;
        const auto data = hit.toMap();
        if (!data.value("valid").toBool()) continue;
        auto endpoint = dragEndpoint(i, data.value("index", -1).toInt());
        if (endpoint.isEmpty()) continue;
        *side = i;
        return endpoint;
    }
    return {};
}

Qt::DropAction F4GalleryBridge::acceptNativeDrop(const QMimeData *mime,
        Qt::DropActions allowed, Qt::KeyboardModifiers mods) const
{
    if (!mime) return Qt::IgnoreAction;
    const bool internal = !m_dragToken.isEmpty()
        && mime->data(sessionMime) == m_dragToken.toUtf8();
    if (internal && (mods & Qt::ShiftModifier) && !(mods & Qt::ControlModifier)
        && allowed.testFlag(Qt::MoveAction)) return Qt::MoveAction;
    if (!allowed.testFlag(Qt::CopyAction)) return Qt::IgnoreAction;
    if (internal) return Qt::CopyAction;
    if (!mime->hasUrls() || mime->urls().isEmpty()) return Qt::IgnoreAction;
    for (const auto &url : mime->urls()) {
        if (!url.isLocalFile() || !QDir::isAbsolutePath(url.toLocalFile()))
            return Qt::IgnoreAction;
    }
    return Qt::CopyAction;
}

void F4GalleryBridge::clearDropHighlight()
{
    for (auto &item : m_dragPanels) if (item) item->setProperty("dropHoverIndex", -2);
}

bool F4GalleryBridge::finishInternalDrop(QObject *window, const QPointF &position, Qt::KeyboardModifiers modifiers)
{
    if (m_nativeDragCancelled || m_dragSource.isEmpty()) {
        if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish cancelled/empty" << m_nativeDragCancelled << m_dragSource.isEmpty();
        return false;
    }
    const auto source = dragEndpoint(m_dragSource.value("side", -1).toInt());
    // After switching workspaces the visible side belongs to a different panel.
    // Go revalidates the captured source against its owning workspace.
    if (source.value("panelId") == m_dragSource.value("panelId"))
    for (const auto *key : {"panelId", "path", "catalogRevision"})
        if (source.value(key) != m_dragSource.value(key)) {
            if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish stale" << key << source.value(key) << m_dragSource.value(key);
            return false;
        }
    int targetSide = -1;
    auto target = dragWorkspaceHit(window, position);
    const bool workspace = !target.isEmpty();
    if (!workspace) target = dragHit(window, position, &targetSide);
    if (target.isEmpty() || (!workspace && !m_panelSessions.catalog(targetSide).dropAllowed)) {
        if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish invalid target" << position << targetSide << QGuiApplication::modalWindow();
        return false;
    }
    target.insert("action", workspace ? "workspace.dropFiles" : "panel.dropFiles");
    target.insert("source", m_dragSource);
    target.insert("operation", (modifiers & Qt::ShiftModifier)
        && !(modifiers & Qt::ControlModifier) ? "move" : "copy");
    emit uiActionRequested(target);
    return true;
}

bool F4GalleryBridge::eventFilter(QObject *object, QEvent *event)
{
    if (!qobject_cast<QQuickWindow *>(object)) return QObject::eventFilter(object, event);
    bool ownsWindow = m_dragWorkspaceBar && m_dragWorkspaceBar->window() == object;
    for (const auto &item : m_dragPanels)
        ownsWindow = ownsWindow || (item && item->window() == object);
    if (!ownsWindow) return QObject::eventFilter(object, event);
    if (m_nativeDragActive) {
        if (event->type() == QEvent::DragEnter) m_nativeDragEntered = true;
        if (event->type() == QEvent::MouseButtonRelease
            && static_cast<QMouseEvent *>(event)->button() == Qt::LeftButton)
            m_nativeDragReleased = true;
        if (event->type() == QEvent::KeyPress
            && static_cast<QKeyEvent *>(event)->key() == Qt::Key_Escape)
            m_nativeDragCancelled = true;
    }
    if (event->type() == QEvent::DragLeave) { clearDropHighlight(); m_dragHoveredWorkspace.clear(); return false; }
    if (event->type() == QEvent::DragEnter || event->type() == QEvent::DragMove
        || event->type() == QEvent::Drop) {
        auto *drop = static_cast<QDropEvent *>(event);
        const auto workspace = dragWorkspaceHit(object, drop->position());
        const auto action = acceptNativeDrop(drop->mimeData(), drop->possibleActions(), drop->modifiers());
        if (!workspace.isEmpty() && action != Qt::IgnoreAction) {
            clearDropHighlight();
            const QString id = workspace.value("target").toString();
            if (event->type() == QEvent::Drop) {
                auto request = workspace;
                request.insert("action", "workspace.dropFiles");
                request.insert("operation", "copy");
                if (!m_dragToken.isEmpty() && drop->mimeData()->data(sessionMime) == m_dragToken.toUtf8()) {
                    request.insert("source", m_dragSource);
                    if ((drop->modifiers() & Qt::ShiftModifier) && !(drop->modifiers() & Qt::ControlModifier))
                        request.insert("operation", "move");
                } else {
                    QStringList paths;
                    for (const auto &url : drop->mimeData()->urls()) paths.append(url.toLocalFile());
                    request.insert("paths", paths);
                }
                emit uiActionRequested(request);
                m_dragHoveredWorkspace.clear();
            } else if (id != m_dragHoveredWorkspace) {
                m_dragHoveredWorkspace = id;
                if (!workspace.value("active").toBool())
                    emit uiActionRequested({{"action", "workspace.dragActivate"}, {"target", id}});
            }
            drop->setDropAction(action);
            drop->accept();
            return true;
        }
        m_dragHoveredWorkspace.clear();
        int side;
        auto target = dragHit(object, drop->position(), &side);
        if (qEnvironmentVariableIsSet("VTUI_DEBUG") && event->type() != QEvent::DragMove)
            qInfo() << "QT_DND: native event" << event->type() << drop->position()
                    << "target" << side << "action" << action << "identity" << target.value("panelId");
        clearDropHighlight();
        if (target.isEmpty() || action == Qt::IgnoreAction
            || !m_panelSessions.catalog(side).dropAllowed) { drop->ignore(); return true; }
        if (event->type() != QEvent::Drop) {
            m_dragPanels[side]->setProperty("dropHoverIndex", target.value("isDir").toBool()
                && target.value("name").toString() != ".." ? target.value("index").toInt() : -1);
        } else {
            target.insert("action", "panel.dropFiles");
            target.insert("operation", "copy");
            if (!m_dragToken.isEmpty() && drop->mimeData()->data(sessionMime) == m_dragToken.toUtf8()) {
                target.insert("source", m_dragSource);
                if ((drop->modifiers() & Qt::ShiftModifier) && !(drop->modifiers() & Qt::ControlModifier))
                    target.insert("operation", "move");
            } else {
                QStringList paths;
                for (const auto &url : drop->mimeData()->urls()) paths.append(url.toLocalFile());
                target.insert("paths", paths);
            }
            emit uiActionRequested(target);
        }
        drop->setDropAction(action);
        drop->accept();
        return true;
    }
    if (m_nativeDragActive) return false;
    if (m_nativeDragStartupPending && event->type() == QEvent::KeyPress
        && static_cast<QKeyEvent *>(event)->key() == Qt::Key_Escape) {
        m_nativeDragStartupPending = false;
        m_dragArmedSide = -1;
        m_dragSource.clear();
        m_dragRequestId.clear();
        return true;
    }
    if (event->type() == QEvent::MouseButtonRelease || event->type() == QEvent::WindowDeactivate) {
        if (m_nativeDragStartupPending && event->type() == QEvent::MouseButtonRelease) {
            const auto *mouse = static_cast<QMouseEvent *>(event);
            if (mouse->button() == Qt::LeftButton
                && QGuiApplication::topLevelAt(mouse->globalPosition().toPoint()) == object)
                finishInternalDrop(object, mouse->position(), mouse->modifiers());
        }
        m_nativeDragStartupPending = false;
        m_dragArmedSide = -1;
        m_dragSource.clear();
        m_dragRequestId.clear();
    }
    if (event->type() == QEvent::MouseButtonPress) {
        auto *mouse = static_cast<QMouseEvent *>(event);
        m_dragArmedSide = -1;
        m_nativeDragStartupPending = false;
        m_dragSource.clear();
        m_dragPrepared = false;
        m_dragThresholdPassed = false;
        m_preparedDragUrls.clear();
        m_dragRequestId.clear();
        if (mouse->button() != Qt::LeftButton || mouse->modifiers() != Qt::NoModifier) return false;
        int side;
        auto source = dragHit(object, mouse->position(), &side);
        const auto id = source.value("entryId").toString();
        if (id.isEmpty() || source.value("name").toString() == "..") return false;
        const auto &state = m_panelSessions.catalog(side);
        QStringList ids;
        if (state.selectedEntryIds.contains(id)) ids = state.selectedEntryIdList;
        else if (state.selectedEntryIds.isEmpty()) ids.append(id);
        else return false;
        source.insert("entryIds", ids);
        m_dragSource = source;
        m_dragArmedSide = side;
        m_dragPress = mouse->position();
        // A complete local selection is already in the authoritative catalog.
        // Use it immediately for quick gestures; Go resolves off-page entries.
        if (state.sourceKind == "local") {
            for (const auto &id : ids) {
                for (const auto &v : state.entries) {
                    const auto row = v.toMap();
                    if (row.value("entryId").toString() != id) continue;
                    m_preparedDragUrls.append(QUrl::fromLocalFile(
                        QDir(state.currentPath).filePath(row.value("name").toString())));
                    break;
                }
            }
            m_dragPrepared = m_preparedDragUrls.size() == ids.size();
        }
        m_dragRequestId = QUuid::createUuid().toString(QUuid::WithoutBraces);
        prepareDragPreview(m_dragPanels[side]);
        auto request = source;
        request.insert("action", "panel.prepareDrag");
        request.insert("requestId", m_dragRequestId);
        emit uiActionRequested(request);
    }
    if (event->type() == QEvent::MouseMove && m_dragArmedSide >= 0) {
        auto *mouse = static_cast<QMouseEvent *>(event);
        if (!(mouse->buttons() & Qt::LeftButton)) { m_dragArmedSide = -1; return false; }
        if ((mouse->position() - m_dragPress).manhattanLength() < QGuiApplication::styleHints()->startDragDistance()) return false;
        m_dragThresholdPassed = true;
        startPreparedDrag();
        return true;
    }
    return QObject::eventFilter(object, event);
}

void F4GalleryBridge::handleDragPrepared(const QVariantMap &message)
{
    if (message.value("type") != "drag_prepared" || m_dragRequestId.isEmpty()
        || message.value("requestId") != m_dragRequestId || m_dragArmedSide < 0) return;
    if (!message.value("ok").toBool()) { m_dragArmedSide = -1; return; }
    m_preparedDragUrls.clear();
    for (const auto &value : message.value("paths").toList())
        m_preparedDragUrls.append(QUrl::fromLocalFile(value.toString()));
    m_dragPrepared = true;
    QTimer::singleShot(0, this, &F4GalleryBridge::startPreparedDrag);
}

void F4GalleryBridge::prepareDragPreview(QQuickItem *host)
{
    m_dragPreviewPixmap = {};
    m_dragPreviewPending = false;
    m_dragPreviewHotSpot = QPoint(18,18);
    if (!host || !host->window()) return;
    const int side = m_dragSource.value("side").toInt();
    const auto ids = m_dragSource.value("entryIds").toStringList();
    const qreal dpr = host->window()->devicePixelRatio();
    QList<QImage> images;
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionForSide(side));
    auto *model = session ? session->model() : nullptr;
    auto *engine = qmlEngine(host);
    int visualRole = -1;
    if (model) {
        const auto roles = model->roleNames();
        for (auto it=roles.begin(); it!=roles.end(); ++it)
            if (it.value()=="visualSnapshot") visualRole=it.key();
    }

    for (const auto &id : ids.mid(0,5)) {
        QVariantMap visual;
        if (model && visualRole>=0) {
            const int row=session->indexForEntryId(id);
            if (row>=0) visual=model->data(model->index(row,0),visualRole).toMap();
        }
        QImage image;
        const QUrl url(visual.value("imageIdUrl").toString());
        if (engine && url.scheme()=="image") {
            auto *provider=dynamic_cast<QQuickImageProvider *>(engine->imageProvider(url.host()));
            if (provider && provider->imageType()==QQmlImageProviderBase::Image) {
                QSize size;
                image=provider->requestImage(url.path().mid(1),&size,QSize(qCeil(40*dpr),qCeil(40*dpr)));
                if (!image.isNull()) { // upstream thumbnails use PreserveAspectCrop
                    image=image.scaled(qCeil(40*dpr),qCeil(40*dpr),Qt::KeepAspectRatioByExpanding,Qt::SmoothTransformation);
                    image=image.copy((image.width()-qCeil(40*dpr))/2,(image.height()-qCeil(40*dpr))/2,qCeil(40*dpr),qCeil(40*dpr));
                }
            }
        }
        if (image.isNull()) {
            QString path=visual.value("iconPath").toString();
            if (path.startsWith("qrc:/")) path=":"+path.mid(4);
            QIcon icon(path);
            if (icon.isNull()) icon=QIcon(visual.value("isFolder").toBool()
                ? ":/F4QtHost/icons/lucide-gallery/folder.svg" : ":/F4QtHost/icons/lucide-gallery/file.svg");
            image=icon.pixmap(QSize(qCeil(40*dpr),qCeil(40*dpr))).toImage();
        }
        images.append(image);
    }
    m_dragPreviewPixmap=F4NativeDragVisuals::compactPreview(images,ids.size(),dpr,
        QGuiApplication::styleHints()->colorScheme()==Qt::ColorScheme::Dark);
    // Standalone single-item drags use the actual rendered image/icon, with
    // the original pointer hotspot; preserve that in every gallery layout.
    if (ids.size()!=1) return;
    QList<QQuickItem *> pending{host};
    while (!pending.isEmpty()) {
        auto *item=pending.takeLast();
        pending.append(item->childItems());
        if (item->property("entryId").toString()!=ids.first()) continue;
        auto *preview=item->property("previewContainerItem").value<QQuickItem *>();
        if (!preview || !preview->isVisible() || preview->width()<=0 || preview->height()<=0) continue;
        auto grab=preview->grabToImage();
        if (!grab) break;
        m_dragPreviewHotSpot=preview->mapFromScene(m_dragPress).toPoint();
        m_dragPreviewPending=true;
        const QString request=m_dragRequestId;
        connect(grab.data(),&QQuickItemGrabResult::ready,this,[this,grab,request,dpr] {
            // Break the connection/captured shared-pointer ownership cycle.
            disconnect(grab.data(), nullptr, this, nullptr);
            if (request!=m_dragRequestId) return;
            if (!grab->image().isNull()) {
                m_dragPreviewPixmap=QPixmap::fromImage(grab->image());
                m_dragPreviewPixmap.setDevicePixelRatio(dpr);
            }
            m_dragPreviewPending=false;
            startPreparedDrag();
        });
        break;
    }
}

void F4GalleryBridge::startPreparedDrag()
{
        if (!m_dragPrepared || !m_dragThresholdPassed || m_dragArmedSide < 0
            || m_nativeDragActive || m_dragPreviewPending || !(QGuiApplication::mouseButtons() & Qt::LeftButton)) return;
        const int side = m_dragArmedSide;
        m_dragArmedSide = -1;
        auto *host = m_dragPanels[side].data();
        if (!host || !host->window() || !host->property("dropInputEnabled").toBool()) return;
        const auto current = dragEndpoint(side);
        if (current.value("catalogRevision") != m_dragSource.value("catalogRevision")
            || current.value("panelId") != m_dragSource.value("panelId")
            || current.value("path") != m_dragSource.value("path")) return;
        auto *mime = new QMimeData;
        m_dragToken = QUuid::createUuid().toString(QUuid::WithoutBraces);
        mime->setData(sessionMime, m_dragToken.toUtf8());
        const auto ids = m_dragSource.value("entryIds").toStringList();
        if (m_preparedDragUrls.size() == ids.size()) mime->setUrls(m_preparedDragUrls);
        if (m_dragWorkspaceBar)
            QMetaObject::invokeMethod(m_dragWorkspaceBar, "beginWorkspaceDrag");
        m_nativeDragActive = true;
        m_nativeDragEntered = false;
        m_nativeDragReleased = false;
        m_nativeDragCancelled = false;
        m_nativeDragStartupPending = false;
        if (qEnvironmentVariableIsSet("VTUI_DEBUG"))
            qInfo() << "QT_DND: native drag starting, files" << ids.size();
        auto *window = host->window();
        QMetaObject::invokeMethod(m_dragPanels[side], "endNativeDragPointer");
        QDrag drag(window);
        drag.setMimeData(mime);
        const qreal dpr = window->devicePixelRatio();
        drag.setPixmap(m_dragPreviewPixmap);
        drag.setHotSpot(m_dragPreviewHotSpot);
#ifdef Q_OS_WIN
        const auto previewSize=drag.pixmap().deviceIndependentSize();
        const auto copyCursor=F4NativeDragVisuals::windowsDragCursorPixmap(dpr,previewSize,drag.hotSpot(),Qt::CopyAction);
        const auto moveCursor=F4NativeDragVisuals::windowsDragCursorPixmap(dpr,previewSize,drag.hotSpot(),Qt::MoveAction);
        drag.setDragCursor(copyCursor,Qt::CopyAction);
        drag.setDragCursor(moveCursor,Qt::MoveAction);
        drag.setDragCursor(F4NativeDragVisuals::windowsDragCursorPixmap(dpr,previewSize,drag.hotSpot(),Qt::IgnoreAction),Qt::IgnoreAction);
        // Internal Shift-move is Go-owned. Keep external OLE Copy-only, but
        // replace its cursor artwork when the internal receiver will move.
        // Qt Windows GiveFeedback checks the pixmap cache key on every poll.
        QTimer actionFeedback;
        actionFeedback.setInterval(16);
        auto updateFeedback=[&] {
            const auto mods=QGuiApplication::queryKeyboardModifiers();
            bool internalTarget=false;
            if (QGuiApplication::topLevelAt(QCursor::pos())==window) {
                const QPointF point=window->mapFromGlobal(QCursor::pos());
                int targetSide=-1;
                internalTarget=!dragWorkspaceHit(window,point).isEmpty();
                if (!internalTarget) {
                    const auto target=dragHit(window,point,&targetSide);
                    internalTarget=!target.isEmpty() && m_panelSessions.catalog(targetSide).dropAllowed;
                }
            }
            const bool move=internalTarget && (mods&Qt::ShiftModifier) && !(mods&Qt::ControlModifier);
            drag.setDragCursor(move?moveCursor:copyCursor,Qt::CopyAction);
        };
        connect(&actionFeedback,&QTimer::timeout,&drag,updateFeedback);
        updateFeedback();
        actionFeedback.start();
#endif
        // The desktop may only copy. Internal Shift-move is selected by our
        // receiver; a native move is never advertised to external receivers.
        Qt::DropAction finished = Qt::IgnoreAction;
#ifdef Q_OS_WIN
        bool missedRelease = false;
        int releasedPolls = 0;
        QTimer releaseWatchdog;
        releaseWatchdog.setInterval(75);
        connect(&releaseWatchdog, &QTimer::timeout, &drag, [&] {
            if (GetAsyncKeyState(VK_LBUTTON) & 0x8000) {
                releasedPolls = 0;
            } else if (++releasedPolls >= 2) {
                // Normal OLE completion returns before this grace period.
                missedRelease = true;
                releaseWatchdog.stop();
                QDrag::cancel();
            }
        });
        // Windows' OLE source learns its initial button state on its first
        // poll. If the physical release already happened while processing
        // queued input / preparing the preview, DoDragDrop never sees a
        // held button and waits for a second click. Complete only an internal
        // drop in that case; there is no desktop transaction to acknowledge.
        auto finishReleasedInternalDrop = [&] {
            if (m_nativeDragCancelled || (GetAsyncKeyState(VK_ESCAPE) & 0x8000)
                || QGuiApplication::topLevelAt(QCursor::pos()) != window) {
                if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish wrong window/cancel" << m_nativeDragCancelled << QGuiApplication::topLevelAt(QCursor::pos()) << window;
                return;
            }
            if (finishInternalDrop(window, window->mapFromGlobal(QCursor::pos()), QGuiApplication::queryKeyboardModifiers())) {
                finished = Qt::CopyAction;
            }
        };
        if (!(GetAsyncKeyState(VK_LBUTTON) & 0x8000)) {
            finishReleasedInternalDrop();
        } else
#endif
        {
#ifdef Q_OS_WIN
            releaseWatchdog.start();
#endif
            finished = drag.exec(Qt::CopyAction, Qt::CopyAction);
#ifdef Q_OS_WIN
            releaseWatchdog.stop();
            if (!(GetAsyncKeyState(VK_LBUTTON) & 0x8000))
                m_nativeDragReleased = true;
            // Qt's Windows startup waits for a further WM_MOUSEMOVE. If the
            // async preparation consumed the last held-button move, it can
            // process the release and return E_FAIL before OLE ever starts.
            // Recover that observed release, never an Escape or rejected OLE drop.
            if ((missedRelease || nativeDragStartupWasReleased()) && finished == Qt::IgnoreAction)
                finishReleasedInternalDrop();
#endif
        }
        if (auto *grabber = window->mouseGrabberItem()) grabber->ungrabMouse();
        if (qEnvironmentVariableIsSet("VTUI_DEBUG"))
            qInfo() << "QT_DND: native drag finished" << finished << "cursor" << window->mapFromGlobal(QCursor::pos())
                    << "entered/released/cancelled" << m_nativeDragEntered << m_nativeDragReleased << m_nativeDragCancelled;
        if (m_dragWorkspaceBar)
            m_dragWorkspaceBar->setProperty("dragSourceWorkspace", QString());
        m_nativeDragActive = false;
        m_dragHoveredWorkspace.clear();
        m_dragToken.clear();
        clearDropHighlight();
#ifdef Q_OS_WIN
        // A stale no-button WM_MOUSEMOVE can also fail Qt startup while the
        // actual button is still held. Keep this gesture for the next move or
        // its release, instead of discarding an otherwise valid internal drop.
        if (finished == Qt::IgnoreAction && !m_nativeDragEntered && !m_nativeDragCancelled
            && (GetAsyncKeyState(VK_LBUTTON) & 0x8000) && !(GetAsyncKeyState(VK_ESCAPE) & 0x8000)) {
            m_nativeDragStartupPending = true;
            m_dragArmedSide = side;
            return;
        }
#endif
        m_dragSource.clear();
}
