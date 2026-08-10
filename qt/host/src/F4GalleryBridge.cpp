#include "F4GalleryBridge.h"
#include "F4IconProvider.h"

#include <QGuiApplication>
#include <QQmlEngine>
#include <QSet>
#include <QTimer>
#include <QUrl>

#if F4_WITH_ZOINGALLERY
#include <ZoinGallery/GalleryRuntime.h>
#include <ZoinGallery/GallerySession.h>
#endif

namespace
{
constexpr int GalleryIconLogicalSize = 128;

QVariantMap shellFromScene(const QVariantMap &scene)
{
    const QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    if (!shell.isEmpty()) {
        return shell;
    }

    const QVariantList frames = scene.value(QStringLiteral("frames")).toList();
    for (auto it = frames.crbegin(); it != frames.crend(); ++it) {
        const QVariantMap frame = it->toMap();
        const QString kind = frame.value(QStringLiteral("kind")).toString();
        if (kind == QStringLiteral("shell") || kind == QStringLiteral("panels")) {
            return frame;
        }
    }
    return {};
}

bool isF4BundledLucideIcon(const QString &source)
{
    static const QString normalPrefix =
        QStringLiteral("qrc:/F4QtHost/icons/lucide/");
    static const QString galleryPrefix =
        QStringLiteral("qrc:/F4QtHost/icons/lucide-gallery/");
    return source.startsWith(normalPrefix)
        || source.startsWith(galleryPrefix);
}

bool isZoinGalleryDefaultIcon(const QString &source)
{
    static const QSet<QString> defaults{
        QStringLiteral("qrc:/ZoinGallery/resources/FileIcon.svg"),
        QStringLiteral("qrc:/ZoinGallery/resources/FolderIcon.svg"),
        QStringLiteral("qrc:/ZoinGallery/resources/ImageIcon.svg"),
    };
    return defaults.contains(source);
}

bool isF4SystemFileIcon(const QString &source, const QString &providerId)
{
    const QUrl url(source);
    return url.scheme() == QStringLiteral("image")
        && url.host().compare(providerId, Qt::CaseInsensitive) == 0
        && url.path().startsWith(QStringLiteral("/file/"));
}

qreal availableDevicePixelRatio()
{
    return qGuiApp ? qGuiApp->devicePixelRatio() : qreal(1);
}
}

F4GalleryBridge::F4GalleryBridge(QQmlEngine *engine, QObject *parent,
                                 F4IconSet *iconSet)
    : QObject(parent)
    , m_iconSet(iconSet)
{
    if (m_iconSet) {
        connect(m_iconSet, &F4IconSet::revisionChanged,
                this, &F4GalleryBridge::refreshIconAppearance);
    }
    for (int side = 0; side < 2; ++side) {
        auto *timer = new QTimer(this);
        timer->setSingleShot(true);
        // QML normally commits on key release. This persistent watchdog owns
        // the deferred intent if a transient panel Loader disappears or a
        // focus transition drops that release event.
        timer->setInterval(5000);
        connect(timer, &QTimer::timeout, this,
                [this, side]() { commitPendingCursor(side); });
        m_cursorCommitTimers[static_cast<size_t>(side)] = timer;
    }
#if F4_WITH_ZOINGALLERY
    if (!engine) {
        return;
    }

    ZoinGallery::RuntimeOptions options;
    options.providerPrefix = QStringLiteral("f4-zoingallery");
    options.storageNamespace = QStringLiteral("f4-qt-host");
    // f4 owns one shared runtime for both panels. Preserve ZoinGallery's
    // historical platform-sized decode pool here: the runtime still bounds
    // compressed payloads and viewer frames independently, while a fixed
    // four-worker host pool cannot keep up with held navigation on machines
    // with substantially more decode capacity.
    options.maxDecodeThreads = 0;
    options.persistentCache = true;
    auto *runtime = ZoinGallery::GalleryRuntime::install(engine, options);
    m_runtime = runtime;
    if (!runtime) {
        return;
    }

    m_sessions[0] = runtime->createExternalSession(QStringLiteral("f4-left"), this);
    m_sessions[1] = runtime->createExternalSession(QStringLiteral("f4-right"), this);
#else
    Q_UNUSED(engine);
#endif
}

F4GalleryBridge::~F4GalleryBridge()
{
#if F4_WITH_ZOINGALLERY
    for (const QPointer<QObject> &sessionObject : m_sessions) {
        if (auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionObject.data())) {
            session->shutdown();
        }
    }
    if (auto *runtime = qobject_cast<ZoinGallery::GalleryRuntime *>(m_runtime.data())) {
        // The runtime owns the engine-level providers and shared decode pool.
        // Stop it while the QQmlEngine is still alive; its QObject destructor
        // remains an idempotent fallback for standalone/other embedders.
        runtime->shutdown();
    }
#endif
}

bool F4GalleryBridge::available() const
{
#if F4_WITH_ZOINGALLERY
    return m_runtime && m_sessions[0] && m_sessions[1];
#else
    return false;
#endif
}

QObject *F4GalleryBridge::leftSession() const
{
    return m_sessions[0].data();
}

QObject *F4GalleryBridge::rightSession() const
{
    return m_sessions[1].data();
}

QObject *F4GalleryBridge::viewerSession() const
{
    return validSide(m_viewerSide) ? m_sessions[static_cast<size_t>(m_viewerSide)].data() : nullptr;
}

QUrl F4GalleryBridge::panelComponentUrl() const
{
    return available() ? QUrl(QStringLiteral("qrc:/F4QtHost/qml/GalleryPanelHost.qml")) : QUrl();
}

QUrl F4GalleryBridge::viewerComponentUrl() const
{
    return available() ? QUrl(QStringLiteral("qrc:/F4QtHost/qml/GalleryViewerHost.qml")) : QUrl();
}

QUrl F4GalleryBridge::scrollBarComponentUrl() const
{
    // Load the reusable module's actual control rather than maintaining a
    // visually similar host-side copy. Keeping the URL behind the optional
    // bridge also preserves list-only builds without a ZoinGallery import.
    return available()
        ? QUrl(QStringLiteral("qrc:/ZoinGallery/qml/GalleryScrollBar.qml"))
        : QUrl();
}

QObject *F4GalleryBridge::sessionForSide(int side) const
{
    return validSide(side) ? m_sessions[static_cast<size_t>(side)].data() : nullptr;
}

bool F4GalleryBridge::shouldUseGallery(const QVariantMap &panel) const
{
    return available()
        && panel.value(QStringLiteral("presentation")).toString() == QStringLiteral("gallery")
        && panel.value(QStringLiteral("previewCapable")).toBool()
        && panel.value(QStringLiteral("sourceKind")).toString() == QStringLiteral("local");
}

void F4GalleryBridge::requestActivate(int side)
{
    if (!validSide(side)) {
        return;
    }
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.activate")},
        {QStringLiteral("side"), side},
    });
}

void F4GalleryBridge::requestCursor(int side,
                                    const QString &entryId,
                                    int index,
                                    qulonglong catalogRevision,
                                    bool deferCommit)
{
    if (!validSide(side)) {
        return;
    }
    if (m_pendingViewer.active && m_pendingViewer.side == side
        && m_pendingViewer.entryId != entryId) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side
        && m_pendingPanelOpen.entryId != entryId) {
        clearPendingPanelOpen();
    }
    if (!entryId.isEmpty()) {
        PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
        pending.active = true;
        pending.panelId = m_states[static_cast<size_t>(side)].panelId;
        pending.entryId = entryId;
        pending.index = index;
        pending.catalogRevision = effectiveCatalogRevision(side, catalogRevision);
    } else {
        clearPendingCursor(side);
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = true;
    if (deferCommit) {
        // Gallery already moved optimistically. Keep the latest stable ID in
        // pending state so older scenes cannot snap it backward, but avoid
        // serializing a full semantic catalog for every native key-repeat.
        m_cursorCommitTimers[static_cast<size_t>(side)]->start();
        return;
    }
    m_cursorCommitTimers[static_cast<size_t>(side)]->stop();
    sendPanelAction(side, QStringLiteral("panel.cursor"), entryId, index, catalogRevision);
}

void F4GalleryBridge::requestOpen(int side,
                                  const QString &entryId,
                                  int index,
                                  bool isImage,
                                  qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }

    const SideState &sideState = m_states[static_cast<size_t>(side)];
    if (isImage && available() && sideState.previewCapable
        && sideState.presentation == QStringLiteral("gallery")
        && sideState.sourceKind == QStringLiteral("local")) {
        clearPendingPanelOpen();
        closeViewer();
        m_pendingViewer.active = true;
        m_pendingViewer.side = side;
        m_pendingViewer.panelId = m_states[static_cast<size_t>(side)].panelId;
        m_pendingViewer.entryId = entryId;
        m_pendingViewer.catalogRevision = effectiveCatalogRevision(side, catalogRevision);

        // Enter and the second press of a double-click often target the
        // cursor that Go has already confirmed. Opening that image must not
        // wait for another identical panel.cursor scene: unchanged semantic
        // scenes are deliberately suppressed by the renderer, so no such
        // acknowledgement is guaranteed to arrive.
        if (sideState.active && sideState.cursorEntryId == entryId) {
            // The QML delegate can still carry the preceding scene revision
            // while its stable entry identity already matches the bridge's
            // authoritative state. Use that authoritative revision so a
            // harmless stale binding cannot turn the immediate path back
            // into a pending open with no future scene to reconcile it.
            m_pendingViewer.catalogRevision = sideState.catalogRevision;
            reconcilePendingViewer(side);
            return;
        }
        if (!sideState.active) {
            requestActivate(side);
        }
        requestCursor(side, entryId, index, m_pendingViewer.catalogRevision);
        return;
    }

    clearPendingViewer();
    clearPendingPanelOpen();
    m_pendingPanelOpen.active = true;
    m_pendingPanelOpen.side = side;
    m_pendingPanelOpen.panelId = m_states[static_cast<size_t>(side)].panelId;
    m_pendingPanelOpen.entryId = entryId;

    // A folder double-click is two independent pointer presses. The first
    // press can be acknowledged before QML delivers doubleClicked, leaving
    // the bridge already authoritative at the requested stable identity.
    // Do not wait for the identical cursor action from the second press: f4
    // suppresses unchanged semantic scenes, so that no-op has no guaranteed
    // acknowledgement. panel.open itself activates the owning panel.
    if (sideState.cursorEntryId == entryId) {
        clearPendingCursor(side);
        reconcilePendingPanelOpen(side);
        return;
    }
    requestActivate(side);
    requestCursor(side, entryId, index, catalogRevision);
}

void F4GalleryBridge::requestSelection(int side,
                                       const QString &mode,
                                       const QVariantList &entryIds,
                                       qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }

    static const QSet<QString> validModes = {
        QStringLiteral("replace"), QStringLiteral("add"),
        QStringLiteral("remove"), QStringLiteral("toggle"),
    };
    const QString normalizedMode = validModes.contains(mode) ? mode : QStringLiteral("toggle");
    if (entryIds.isEmpty() && normalizedMode != QStringLiteral("replace")) {
        return;
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_states[sideIndex];
    PendingSelection &pending = m_pendingSelections[sideIndex];
    if (!pending.active || pending.panelId != state.panelId) {
        pending = PendingSelection{};
        pending.active = true;
        pending.panelId = state.panelId;
        pending.catalogRevision = effectiveCatalogRevision(side, catalogRevision);
    }

    QSet<QString> requested;
    for (const QVariant &value : entryIds) {
        const QString id = value.toString();
        if (!id.isEmpty()) {
            requested.insert(id);
        }
    }
    if (normalizedMode == QStringLiteral("replace")) {
        for (const QString &id : state.entryIds) {
            pending.desiredByEntryId.insert(id, requested.contains(id));
        }
    } else {
        for (const QString &id : requested) {
            if (normalizedMode == QStringLiteral("add")) {
                pending.desiredByEntryId.insert(id, true);
            } else if (normalizedMode == QStringLiteral("remove")) {
                pending.desiredByEntryId.insert(id, false);
            } else {
                const bool current = pending.desiredByEntryId.contains(id)
                    ? pending.desiredByEntryId.value(id)
                    : state.selectedEntryIds.contains(id);
                pending.desiredByEntryId.insert(id, !current);
            }
        }
    }

    const qulonglong revision = effectiveCatalogRevision(side, catalogRevision);
    pending.catalogRevision = revision;
    emitSelectionAction(side, normalizedMode, entryIds, revision);
}

void F4GalleryBridge::requestPresentation(int side, const QString &presentation)
{
    if (!validSide(side)) {
        return;
    }
    if (m_pendingViewer.active && m_pendingViewer.side == side) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side) {
        clearPendingPanelOpen();
    }
    // Preserve the gallery's optimistic cursor before replacing its Loader.
    // The cursor action is queued first, so Go applies it before changing the
    // presentation even though the visual panel can disappear immediately.
    commitPendingCursor(side);
    clearPendingCursor(side);
    clearPendingSelection(side);
    const QString normalized = presentation == QStringLiteral("gallery")
        ? QStringLiteral("gallery") : QStringLiteral("list");
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.setPresentation")},
        {QStringLiteral("side"), side},
        {QStringLiteral("presentation"), normalized},
    });
}

void F4GalleryBridge::requestGalleryLayout(int side,
                                           const QString &layoutMode,
                                           int columnCount)
{
    if (!validSide(side)) {
        return;
    }
    static const QSet<QString> supported = {
        QStringLiteral("masonry"), QStringLiteral("columns"),
        QStringLiteral("details"), QStringLiteral("grid"),
        QStringLiteral("icons"),
    };
    const QString normalized = layoutMode.trimmed().toLower();
    if (!supported.contains(normalized)) {
        return;
    }
    QVariantMap action = {
        {QStringLiteral("action"), QStringLiteral("panel.setGalleryLayout")},
        {QStringLiteral("side"), side},
        {QStringLiteral("layoutMode"), normalized},
    };
    if (columnCount > 0) {
        action.insert(QStringLiteral("columnCount"), columnCount);
    }
    emit uiActionRequested(action);
}

void F4GalleryBridge::requestGalleryDensity(int side,
                                            const QString &layoutMode,
                                            int density)
{
    if (!validSide(side)) {
        return;
    }
    static const QSet<QString> supported = {
        QStringLiteral("masonry"), QStringLiteral("columns"),
        QStringLiteral("details"), QStringLiteral("grid"),
        QStringLiteral("icons"),
    };
    const QString normalized = layoutMode.trimmed().toLower();
    if (!supported.contains(normalized)) {
        return;
    }
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.setGalleryDensity")},
        {QStringLiteral("side"), side},
        {QStringLiteral("layoutMode"), normalized},
        {QStringLiteral("density"), density},
    });
}

void F4GalleryBridge::requestSort(int side, const QString &sortMode,
                                  bool contextMenu)
{
    if (!validSide(side)) {
        return;
    }
    const QString normalized = sortMode.trimmed().toLower();
    if (contextMenu) {
        emit uiActionRequested({
            {QStringLiteral("action"), QStringLiteral("panel.sortMenu")},
            {QStringLiteral("side"), side},
        });
        return;
    }
    static const QSet<QString> supported = {
        QStringLiteral("name"), QStringLiteral("extension"),
        QStringLiteral("time"), QStringLiteral("size"),
        QStringLiteral("unsorted"),
    };
    if (!supported.contains(normalized)) {
        return;
    }
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.sort")},
        {QStringLiteral("side"), side},
        {QStringLiteral("mode"), normalized},
    });
}

void F4GalleryBridge::closeViewer()
{
    clearPendingViewer();
    if (!m_viewerVisible) {
        return;
    }
#if F4_WITH_ZOINGALLERY
    if (auto *session = qobject_cast<ZoinGallery::GallerySession *>(viewerSession())) {
        session->setViewerOpen(false);
    }
#endif
    setViewer(-1, false);
}

void F4GalleryBridge::synchronizeScene(const QVariantMap &scene)
{
    const QVariantList panels = panelsFromScene(scene);
    std::array<bool, 2> found = {false, false};
    for (const QVariant &panelValue : panels) {
        const QVariantMap panel = panelValue.toMap();
        const int side = panel.value(QStringLiteral("side")).toInt();
        if (!validSide(side)) {
            continue;
        }
        found[static_cast<size_t>(side)] = true;
        synchronizePanel(side, panel);
    }

    if (m_viewerVisible
        && (!validSide(m_viewerSide) || !found[static_cast<size_t>(m_viewerSide)]
            || !m_states[static_cast<size_t>(m_viewerSide)].previewCapable
            || !m_states[static_cast<size_t>(m_viewerSide)].active
            || m_states[static_cast<size_t>(m_viewerSide)].presentation
                != QStringLiteral("gallery"))) {
        closeViewer();
    }
    if (m_pendingViewer.active
        && (!validSide(m_pendingViewer.side)
            || !found[static_cast<size_t>(m_pendingViewer.side)])) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active
        && (!validSide(m_pendingPanelOpen.side)
            || !found[static_cast<size_t>(m_pendingPanelOpen.side)])) {
        clearPendingPanelOpen();
    }
    for (int side = 0; side < 2; ++side) {
        if (!found[static_cast<size_t>(side)]) {
            m_panelSnapshots[static_cast<size_t>(side)].clear();
            clearPendingCursor(side);
            clearPendingSelection(side);
        }
    }
}

bool F4GalleryBridge::validSide(int side)
{
    return side == 0 || side == 1;
}

QVariantList F4GalleryBridge::panelsFromScene(const QVariantMap &scene)
{
    return shellFromScene(scene).value(QStringLiteral("panels")).toList();
}

QVariantList F4GalleryBridge::normalizedEntries(const QVariantMap &panel)
{
    const QVariantList sourceEntries = panel.value(QStringLiteral("entries")).toList();
    QVariantList entries;
    entries.reserve(sourceEntries.size());
    for (qsizetype row = 0; row < sourceEntries.size(); ++row) {
        const QVariantMap source = sourceEntries[row].toMap();
        QVariantMap entry;
        entry.insert(QStringLiteral("entryId"), source.value(QStringLiteral("entryId")));
        entry.insert(QStringLiteral("index"), source.value(QStringLiteral("index"), row));
        entry.insert(QStringLiteral("name"), source.value(QStringLiteral("name")));
        // Older/legacy semantic producers do not have the preformatted name
        // roles.  Omitting an absent value lets ImageFile derive it from
        // `name`; inserting an invalid QVariant would instead suppress that
        // fallback and leave Columns/Details labels empty.
        if (source.contains(QStringLiteral("displayBaseName"))) {
            entry.insert(QStringLiteral("displayBaseName"),
                         source.value(QStringLiteral("displayBaseName")));
        }
        if (source.contains(QStringLiteral("displayExtension"))) {
            entry.insert(QStringLiteral("displayExtension"),
                         source.value(QStringLiteral("displayExtension")));
        }
        entry.insert(QStringLiteral("localPath"), source.value(QStringLiteral("localPath")));
        entry.insert(QStringLiteral("isDir"), source.value(QStringLiteral("isDir")));
        entry.insert(QStringLiteral("isUp"), source.value(QStringLiteral("isUp")));
        if (source.contains(QStringLiteral("isImage"))) {
            entry.insert(QStringLiteral("isImage"), source.value(QStringLiteral("isImage")));
        }
        entry.insert(QStringLiteral("selected"), source.value(QStringLiteral("selected")));
        entry.insert(QStringLiteral("mtimeNs"), source.value(QStringLiteral("mtimeNanos")));
        entry.insert(QStringLiteral("size"), source.value(QStringLiteral("size")));
        entry.insert(QStringLiteral("sizeText"), source.value(QStringLiteral("sizeText")));
        entry.insert(QStringLiteral("sizeCalculated"),
                     source.value(QStringLiteral("sizeCalculated")));
        entry.insert(QStringLiteral("mtimeText"), source.value(QStringLiteral("mtime")));
        entry.insert(QStringLiteral("modeText"), source.value(QStringLiteral("mode")));
        entry.insert(QStringLiteral("highlightStyleId"),
                     source.value(QStringLiteral("highlightStyleId")));
        entries.push_back(entry);
    }
    return entries;
}

QVariantList F4GalleryBridge::normalizedAppearance(
    const QVariantMap &panel) const
{
    const QVariantList sourceEntries = panel.value(QStringLiteral("entries")).toList();
    const QVariantMap styles = panel.value(QStringLiteral("highlightStyles")).toMap();
    const qreal devicePixelRatio = availableDevicePixelRatio();
    QVariantList appearance;
    appearance.reserve(sourceEntries.size());
    for (const QVariant &value : sourceEntries) {
        const QVariantMap source = value.toMap();
        const QString styleId = source.value(QStringLiteral("highlightStyleId")).toString();
        QVariantMap entry;
        entry.insert(QStringLiteral("entryId"), source.value(QStringLiteral("entryId")));
        QVariantMap style;
        if (!styleId.isEmpty()) {
            style = styles.value(styleId).toMap();
        }

        if (m_iconSet) {
            const QString configuredIcon =
                style.value(QStringLiteral("icon")).toString();
            const bool bundledLucideIcon =
                isF4BundledLucideIcon(configuredIcon);
            const bool replaceableIcon = configuredIcon.isEmpty()
                || isZoinGalleryDefaultIcon(configuredIcon)
                || isF4SystemFileIcon(configuredIcon,
                                      m_iconSet->providerId())
                || (m_iconSet->system() && bundledLucideIcon);
            const bool hasMarker = !style.value(
                QStringLiteral("marker")).toString().isEmpty();

            // User-supplied URLs are appearance overrides and stay intact.
            // An explicitly configured bundled Lucide icon is also an
            // override while Lucide is active. Under System it becomes a
            // replaceable default, so a marker can suppress it; otherwise it
            // is translated to the equivalent native file icon.
            if (replaceableIcon) {
                if (hasMarker) {
                    style.remove(QStringLiteral("icon"));
                } else {
                    const bool isUp = source.value(
                        QStringLiteral("isUp")).toBool();
                    const bool directory = isUp || source.value(
                        QStringLiteral("isDir")).toBool();
                    QString fileName = source.value(
                        QStringLiteral("name")).toString();
                    if (isUp) {
                        fileName = QStringLiteral("..");
                    }
                    const QUrl iconSource = m_iconSet->fileIconSource(
                        source.value(QStringLiteral("localPath")).toString(),
                        fileName,
                        directory,
                        GalleryIconLogicalSize,
                        devicePixelRatio,
                        source.value(QStringLiteral("mtimeNanos"))
                            .toULongLong());
                    style.insert(QStringLiteral("icon"),
                                 iconSource.toString());
                }
            }
        }
        entry.insert(QStringLiteral("highlightStyle"), style);
        appearance.push_back(entry);
    }
    return appearance;
}

QStringList F4GalleryBridge::selectedEntryIds(const QVariantList &entries)
{
    QStringList ids;
    for (const QVariant &entryValue : entries) {
        const QVariantMap entry = entryValue.toMap();
        if (entry.value(QStringLiteral("selected")).toBool()) {
            const QString id = entry.value(QStringLiteral("entryId")).toString();
            if (!id.isEmpty()) {
                ids.push_back(id);
            }
        }
    }
    return ids;
}

int F4GalleryBridge::sourceIndexForEntryId(const QVariantList &entries,
                                           const QString &entryId)
{
    for (qsizetype row = 0; row < entries.size(); ++row) {
        const QVariantMap entry = entries.at(row).toMap();
        if (entry.value(QStringLiteral("entryId")).toString() == entryId) {
            return entry.value(QStringLiteral("index"), row).toInt();
        }
    }
    return -1;
}

qulonglong F4GalleryBridge::revisionValue(const QVariantMap &map, const QString &key)
{
    bool ok = false;
    const qulonglong value = map.value(key).toULongLong(&ok);
    return ok ? value : 0;
}

void F4GalleryBridge::synchronizePanel(int side, const QVariantMap &panel)
{
    m_panelSnapshots[static_cast<size_t>(side)] = panel;
    SideState &state = m_states[static_cast<size_t>(side)];
    const QString panelId = panel.value(QStringLiteral("id")).toString();
    const qulonglong catalogRevision = revisionValue(panel, QStringLiteral("catalogRevision"));
    const qulonglong selectionRevision = revisionValue(panel, QStringLiteral("selectionRevision"));
    const qulonglong highlightRevision = revisionValue(panel, QStringLiteral("highlightRevision"));
    const qulonglong iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    const QString currentPath = panel.value(QStringLiteral("path")).toString();
    const QString cursorEntryId = panel.value(QStringLiteral("cursorEntryId")).toString();
    const int cursorIndex = panel.value(QStringLiteral("cursor"), -1).toInt();
    const QString presentation = panel.value(QStringLiteral("presentation"), QStringLiteral("list")).toString();
    const QString galleryLayoutMode = panel.value(
        QStringLiteral("galleryLayoutMode"), QStringLiteral("masonry")).toString();
    const int galleryColumnCount = panel.value(
        QStringLiteral("galleryColumnCount"), 2).toInt();
    const int galleryDensity = panel.value(QStringLiteral("galleryDensity")).toInt();
    const qulonglong galleryLayoutRevision = revisionValue(
        panel, QStringLiteral("galleryLayoutRevision"));
    const QVariantList galleryColumns = panel.value(
        QStringLiteral("galleryColumns")).toList();
    const QString sourceKind = panel.value(QStringLiteral("sourceKind"), QStringLiteral("vfs")).toString();
    const bool previewCapable = panel.value(QStringLiteral("previewCapable")).toBool()
        && sourceKind == QStringLiteral("local");
    const bool active = panel.value(QStringLiteral("active")).toBool();
    const bool identityChanged = state.initialized && panelId != state.panelId;
    const bool sourceBecameUnavailable = state.initialized
        && state.previewCapable && !previewCapable;
    const bool leavingGallery = state.initialized
        && state.presentation == QStringLiteral("gallery")
        && presentation != QStringLiteral("gallery");

    if (leavingGallery && !identityChanged && !sourceBecameUnavailable) {
        // Commander shortcuts can change presentation without going through
        // requestPresentation(). Flush the stable pending cursor before the
        // transient gallery surface is torn down.
        commitPendingCursor(side);
    }

#if F4_WITH_ZOINGALLERY
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(m_sessions[static_cast<size_t>(side)].data());
    if (session && (identityChanged || sourceBecameUnavailable)) {
        session->resetExternalSource();
    }
#endif
    if (identityChanged) {
        if (m_viewerVisible && m_viewerSide == side) {
            closeViewer();
        }
        if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side) {
            clearPendingPanelOpen();
        }
        clearPendingCursor(side);
        clearPendingSelection(side);
        m_selectionActionPending[static_cast<size_t>(side)] = false;
        state = SideState{};
    }
    if (sourceBecameUnavailable || presentation != QStringLiteral("gallery")) {
        clearPendingCursor(side);
        clearPendingSelection(side);
        if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side) {
            clearPendingPanelOpen();
        }
    }

    const bool catalogChanged = !state.initialized
        || catalogRevision != state.catalogRevision
        || currentPath != state.currentPath
        || (previewCapable && !state.previewCapable);
    const bool selectionChanged = !state.initialized
        || selectionRevision != state.selectionRevision;
    const bool highlightChanged = !state.initialized
        || highlightRevision != state.highlightRevision;
    const bool iconChanged = !state.initialized
        || iconRevision != state.iconRevision;
    // Semantic scenes retain the complete catalog for compatibility, but a
    // cursor acknowledgement does not need to normalize/copy it again. Keep
    // the revision-owned snapshot in the persistent bridge session.
    const QVariantList entries = catalogChanged
        ? normalizedEntries(panel) : state.entries;
    const QStringList selectedIds = catalogChanged || selectionChanged
        ? selectedEntryIds(catalogChanged
              ? entries
              : panel.value(QStringLiteral("entries")).toList())
        : state.selectedEntryIdList;

#if F4_WITH_ZOINGALLERY
    if (session && previewCapable && catalogChanged) {
        session->applyExternalCatalog(entries, catalogRevision, {
            {QStringLiteral("currentPath"), currentPath},
            {QStringLiteral("sourceKind"), sourceKind},
            {QStringLiteral("previewCapable"), true},
        });
    }

    if (session && previewCapable
        && (catalogChanged || highlightChanged || iconChanged)) {
        session->applyExternalAppearance(normalizedAppearance(panel),
                                         highlightRevision);
    }

    if (session && previewCapable
        && (catalogChanged || selectionChanged
            || cursorEntryId != state.cursorEntryId || cursorIndex != state.cursorIndex
            || m_stateReconciliationPending[static_cast<size_t>(side)])) {
        QString appliedCursorEntryId = cursorEntryId;
        int appliedCursorIndex = cursorIndex;
        const PendingCursor &pendingCursor =
            m_pendingCursors[static_cast<size_t>(side)];
        const bool pendingCursorExists = catalogChanged
            ? sourceIndexForEntryId(entries, pendingCursor.entryId) >= 0
            : state.sourceIndexByEntryId.contains(pendingCursor.entryId);
        if (pendingCursor.active && pendingCursor.panelId == panelId
            && pendingCursor.catalogRevision == catalogRevision
            && pendingCursor.entryId != cursorEntryId
            && pendingCursorExists) {
            // Clicking an inactive panel emits activate before cursor. The
            // activation-only scene still carries the old authoritative
            // cursor at the same catalog revision; keep the stable pending
            // cursor visible until its immediately-following action is
            // acknowledged instead of visibly snapping backward.
            appliedCursorEntryId = pendingCursor.entryId;
            appliedCursorIndex = pendingCursor.index;
        }
        session->applyExternalState(appliedCursorEntryId,
                                    appliedCursorIndex,
                                    selectedIds,
                                    selectionRevision);
    }
#endif

    state.initialized = true;
    state.panelId = panelId;
    state.catalogRevision = catalogRevision;
    state.selectionRevision = selectionRevision;
    state.highlightRevision = highlightRevision;
    state.iconRevision = iconRevision;
    state.currentPath = currentPath;
    state.cursorEntryId = cursorEntryId;
    state.cursorIndex = cursorIndex;
    state.presentation = presentation;
    state.galleryLayoutMode = galleryLayoutMode;
    state.galleryColumnCount = galleryColumnCount;
    state.galleryDensity = galleryDensity;
    state.galleryLayoutRevision = galleryLayoutRevision;
    state.galleryColumns = galleryColumns;
    state.sourceKind = sourceKind;
    state.previewCapable = previewCapable;
    state.active = active;
    if (catalogChanged) {
        state.entries = entries;
        state.entryIds.clear();
        state.sourceIndexByEntryId.clear();
        for (qsizetype row = 0; row < entries.size(); ++row) {
            const QVariantMap entry = entries.at(row).toMap();
            const QString entryId =
                entry.value(QStringLiteral("entryId")).toString();
            if (entryId.isEmpty()) {
                continue;
            }
            state.entryIds.insert(entryId);
            state.sourceIndexByEntryId.insert(
                entryId,
                entry.value(QStringLiteral("index"), row).toInt());
        }
    }
    if (catalogChanged || selectionChanged) {
        state.selectedEntryIdList = selectedIds;
        state.selectedEntryIds = QSet<QString>(selectedIds.cbegin(),
                                               selectedIds.cend());
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = false;
    reconcilePendingCursor(side);
    reconcilePendingPanelOpen(side);
    reconcilePendingSelection(side);
    reconcilePendingViewer(side);
}

void F4GalleryBridge::refreshIconAppearance()
{
    for (int side = 0; side < 2; ++side) {
        const QVariantMap panel =
            m_panelSnapshots[static_cast<size_t>(side)];
        if (!panel.isEmpty()) {
            synchronizePanel(side, panel);
        }
    }
}

void F4GalleryBridge::sendPanelAction(int side,
                                      const QString &actionName,
                                      const QString &entryId,
                                      int index,
                                      qulonglong catalogRevision,
                                      bool includeCatalogRevision)
{
    if (!validSide(side)) {
        return;
    }
    QVariantMap action = {
        {QStringLiteral("action"), actionName},
        {QStringLiteral("side"), side},
    };
    if (!entryId.isEmpty()) {
        action.insert(QStringLiteral("entryId"), entryId);
    }
    if (index >= 0) {
        action.insert(QStringLiteral("index"), index);
    }
    if (includeCatalogRevision) {
        const qulonglong revision = effectiveCatalogRevision(side, catalogRevision);
        if (revision != 0) {
            action.insert(QStringLiteral("catalogRevision"), revision);
        }
    }
    emit uiActionRequested(action);
}

qulonglong F4GalleryBridge::effectiveCatalogRevision(int side, qulonglong supplied) const
{
    if (!validSide(side)) {
        return supplied;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (state.initialized && state.catalogRevision != 0) {
        // The bridge is connected to QtShellController::sceneChanged before
        // QML observes that same signal, so its stable-ID catalog snapshot is
        // authoritative. A pointer gesture can nevertheless finish on a
        // delegate created from the preceding Loader binding. Forwarding that
        // stale non-zero revision makes Go reject panel.cursor; because the
        // scene itself did not change, there is then no acknowledgement with
        // which to reconcile a pending double-click/open. Resolve the stable
        // identity against the bridge-owned snapshot and always use its
        // revision. The supplied value remains the bootstrap fallback before
        // the first semantic scene has initialized this side.
        return state.catalogRevision;
    }
    return supplied;
}

void F4GalleryBridge::commitPendingCursor(int side)
{
    if (!validSide(side)) {
        return;
    }
    if (QTimer *timer = m_cursorCommitTimers[static_cast<size_t>(side)]) {
        timer->stop();
    }
    PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
    if (!pending.active || pending.entryId.isEmpty()) {
        return;
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = true;
    sendPanelAction(side, QStringLiteral("panel.cursor"), pending.entryId,
                    pending.index, pending.catalogRevision);
}

void F4GalleryBridge::reconcilePendingCursor(int side)
{
    if (!validSide(side)) {
        return;
    }

    PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
    if (!pending.active) {
        return;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.previewCapable || state.presentation != QStringLiteral("gallery")
        || state.panelId != pending.panelId) {
        clearPendingCursor(side);
        if (m_pendingViewer.active && m_pendingViewer.side == side) {
            clearPendingViewer();
        }
        return;
    }

    if (state.cursorEntryId == pending.entryId) {
        clearPendingCursor(side);
        return;
    }

    const int currentSourceIndex =
        state.sourceIndexByEntryId.value(pending.entryId, -1);
    if (currentSourceIndex < 0) {
        clearPendingCursor(side);
        if (m_pendingViewer.active && m_pendingViewer.side == side) {
            clearPendingViewer();
        }
        return;
    }

    // A local catalog can advance between the scene used to create an intent
    // and Go processing that intent (for example when cached Desktop entries
    // are replaced by the completed async scan). Go correctly rejects the
    // stale revision. Retry the same stable identity against the newer
    // authoritative revision; never retry by the stale row alone.
    if (state.catalogRevision != pending.catalogRevision) {
        pending.catalogRevision = state.catalogRevision;
        pending.index = currentSourceIndex;
        if (m_pendingViewer.active && m_pendingViewer.side == side
            && m_pendingViewer.entryId == pending.entryId) {
            m_pendingViewer.catalogRevision = state.catalogRevision;
        }
        m_stateReconciliationPending[static_cast<size_t>(side)] = true;
        sendPanelAction(side, QStringLiteral("panel.cursor"), pending.entryId,
                        pending.index, pending.catalogRevision);
    }
}

void F4GalleryBridge::clearPendingCursor(int side)
{
    if (validSide(side)) {
        if (QTimer *timer = m_cursorCommitTimers[static_cast<size_t>(side)]) {
            timer->stop();
        }
        m_pendingCursors[static_cast<size_t>(side)] = PendingCursor{};
    }
}

void F4GalleryBridge::reconcilePendingPanelOpen(int side)
{
    if (!m_pendingPanelOpen.active || m_pendingPanelOpen.side != side
        || !validSide(side)) {
        return;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.previewCapable || state.presentation != QStringLiteral("gallery")
        || state.panelId != m_pendingPanelOpen.panelId) {
        clearPendingPanelOpen();
        return;
    }

    const int currentSourceIndex = state.sourceIndexByEntryId.value(
        m_pendingPanelOpen.entryId, -1);
    if (currentSourceIndex < 0) {
        // A successful directory open replaces the catalog and removes the
        // source entry. Removal by a concurrent file operation has the same
        // safe terminal behavior: never open a different row.
        clearPendingPanelOpen();
        return;
    }

    if (state.cursorEntryId != m_pendingPanelOpen.entryId) {
        const PendingCursor &cursor =
            m_pendingCursors[static_cast<size_t>(side)];
        if (!cursor.active || cursor.entryId != m_pendingPanelOpen.entryId) {
            clearPendingPanelOpen();
        }
        return;
    }

    // Cursor confirmation resolves the catalog race. From this point the
    // operation is an exactly-once stable-ID intent: including a catalog
    // revision would only make an unrelated later revision look retryable and
    // could relaunch an external application. Clear before emitting so even a
    // reentrant scene update cannot dispatch it twice.
    const QString entryId = m_pendingPanelOpen.entryId;
    clearPendingPanelOpen();
    sendPanelAction(side, QStringLiteral("panel.open"), entryId,
                    currentSourceIndex, 0, false);
}

void F4GalleryBridge::clearPendingPanelOpen()
{
    m_pendingPanelOpen = PendingPanelOpen{};
}

void F4GalleryBridge::reconcilePendingSelection(int side)
{
    if (!validSide(side)) {
        return;
    }
    PendingSelection &pending = m_pendingSelections[static_cast<size_t>(side)];
    if (!pending.active) {
        m_selectionActionPending[static_cast<size_t>(side)] = false;
        return;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.previewCapable || state.presentation != QStringLiteral("gallery")
        || state.panelId != pending.panelId) {
        clearPendingSelection(side);
        return;
    }

    for (auto it = pending.desiredByEntryId.begin();
         it != pending.desiredByEntryId.end();) {
        if (!state.entryIds.contains(it.key())
            || state.selectedEntryIds.contains(it.key()) == it.value()) {
            it = pending.desiredByEntryId.erase(it);
        } else {
            ++it;
        }
    }
    if (pending.desiredByEntryId.isEmpty()) {
        clearPendingSelection(side);
        return;
    }

    if (state.catalogRevision == pending.catalogRevision) {
        return;
    }

    // The original action raced a newer catalog. Retry only the still-missing
    // target states and convert toggles to idempotent add/remove operations,
    // so an already accepted mutation is never applied twice.
    pending.catalogRevision = state.catalogRevision;
    m_selectionActionPending[static_cast<size_t>(side)] = false;
    QVariantList add;
    QVariantList remove;
    for (auto it = pending.desiredByEntryId.cbegin();
         it != pending.desiredByEntryId.cend(); ++it) {
        (it.value() ? add : remove).push_back(it.key());
    }
    if (!add.isEmpty()) {
        emitSelectionAction(side, QStringLiteral("add"), add,
                            pending.catalogRevision);
    }
    if (!remove.isEmpty()) {
        emitSelectionAction(side, QStringLiteral("remove"), remove,
                            pending.catalogRevision);
    }
}

void F4GalleryBridge::clearPendingSelection(int side)
{
    if (validSide(side)) {
        const size_t sideIndex = static_cast<size_t>(side);
        m_pendingSelections[sideIndex] = PendingSelection{};
        m_selectionActionPending[sideIndex] = false;
    }
}

void F4GalleryBridge::emitSelectionAction(int side, const QString &mode,
                                          const QVariantList &entryIds,
                                          qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }
    QVariantMap action = {
        {QStringLiteral("action"), QStringLiteral("panel.setSelection")},
        {QStringLiteral("side"), side},
        {QStringLiteral("mode"), mode},
        {QStringLiteral("entryIds"), entryIds},
    };
    if (catalogRevision != 0) {
        action.insert(QStringLiteral("catalogRevision"), catalogRevision);
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const qulonglong selectionRevision = m_states[sideIndex].selectionRevision;
    // Multiple Gallery selection gestures can reach Go before the semantic
    // scene acknowledging the first one returns. Only the first action is
    // guarded by the cached selection revision; later actions remain ordered
    // on the same IPC stream and omit that optional guard so they do not all
    // conflict with the first accepted mutation.
    if (selectionRevision != 0 && !m_selectionActionPending[sideIndex]) {
        action.insert(QStringLiteral("selectionRevision"), selectionRevision);
    }
    m_selectionActionPending[sideIndex] = true;
    emit uiActionRequested(action);
}

void F4GalleryBridge::reconcilePendingViewer(int side)
{
    if (!m_pendingViewer.active || m_pendingViewer.side != side || !validSide(side)) {
        return;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.previewCapable || state.presentation != QStringLiteral("gallery")
        || state.panelId != m_pendingViewer.panelId
        || (m_pendingViewer.catalogRevision != 0
            && state.catalogRevision != m_pendingViewer.catalogRevision)) {
        clearPendingViewer();
        return;
    }

    // Opening from an inactive panel first activates that panel. Cursor and
    // activation acknowledgements can arrive in either order; retain the
    // stable viewer intent until both are authoritative.
    if (!state.active) {
        return;
    }

    if (state.cursorEntryId != m_pendingViewer.entryId) {
        // A cursor request can still be in flight or can be retried after a
        // catalog revision advance. Never open a different image, but keep
        // the viewer intent until the stable cursor is confirmed or removed.
        const PendingCursor &pending =
            m_pendingCursors[static_cast<size_t>(side)];
        if (!pending.active || pending.entryId != m_pendingViewer.entryId) {
            clearPendingViewer();
        }
        return;
    }

#if F4_WITH_ZOINGALLERY
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[static_cast<size_t>(side)].data());
    if (!session || !session->isImageAt(session->currentIndex())) {
        clearPendingViewer();
        return;
    }
#endif
    const int confirmedSide = m_pendingViewer.side;
    clearPendingViewer();
#if F4_WITH_ZOINGALLERY
    session->setViewerOpen(true);
#endif
    setViewer(confirmedSide, true);
}

void F4GalleryBridge::clearPendingViewer()
{
    m_pendingViewer = PendingViewer{};
}

void F4GalleryBridge::setViewer(int side, bool visible)
{
    const int normalizedSide = visible && validSide(side) ? side : -1;
    if (m_viewerVisible == visible && m_viewerSide == normalizedSide) {
        return;
    }
    m_viewerVisible = visible;
    m_viewerSide = normalizedSide;
    emit viewerChanged();
}
