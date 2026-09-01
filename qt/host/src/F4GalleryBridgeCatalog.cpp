#include "F4GalleryBridge.h"
#include "F4IconProvider.h"
#include "NavigationBenchmarkTrace.h"
#include "ViewerCoordinator.h"

#include <QGuiApplication>
#include <QSet>
#include <QTimer>
#include <QUrl>

#include <ZoinGallery/GallerySession.h>
#include <ZoinGallery/MediaTimingTrace.h>

#include <algorithm>
#include <utility>

namespace
{
constexpr int GalleryIconLogicalSize = 128;
constexpr int CatalogMetadataChunkLimit = 8;
constexpr int CatalogMetadataCursorWindowChunks = 8;
constexpr int MaximumDeferredCatalogPageRows = 256;

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

bool usefulLocalCatalogPreview(const QString &sourceKind,
                               bool previewCapable,
                               const QVariantList &entries)
{
    if (sourceKind != QStringLiteral("local") || !previewCapable) {
        return false;
    }
    return std::any_of(entries.cbegin(), entries.cend(),
                       [](const QVariant &value) {
        const QString name = value.toMap().value(
            QStringLiteral("name")).toString();
        return !name.isEmpty() && name != QStringLiteral("..");
    });
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

bool catalogIntegerValue(const QVariant &value, qlonglong *result = nullptr)
{
    const int type = value.metaType().id();
    const bool integer = type == QMetaType::Char
        || type == QMetaType::SChar || type == QMetaType::UChar
        || type == QMetaType::Short || type == QMetaType::UShort
        || type == QMetaType::Int || type == QMetaType::UInt
        || type == QMetaType::LongLong || type == QMetaType::ULongLong;
    if (!integer) {
        return false;
    }
    bool ok = false;
    const qlonglong converted = value.toLongLong(&ok);
    if (!ok) {
        return false;
    }
    if (result) {
        *result = converted;
    }
    return true;
}

QVariant descriptorValue(const QVariantMap &source,
                         const QVariantMap &descriptor,
                         const QString &key,
                         const QString &legacyKey = QString())
{
    if (descriptor.contains(key)) {
        return descriptor.value(key);
    }
    return source.value(legacyKey.isEmpty() ? key : legacyKey);
}

void insertSourceDescriptorFields(QVariantMap *entry,
                                  const QVariantMap &source)
{
    const QVariantMap descriptor = source.value(
        QStringLiteral("source")).toMap();
    for (const QString &key : {QStringLiteral("resourceId"),
                               QStringLiteral("sourceKey")}) {
        const QVariant value = descriptorValue(source, descriptor, key);
        if (value.isValid()) {
            entry->insert(key, value);
        }
    }
    QVariant contentVersion = descriptorValue(
        source, descriptor, QStringLiteral("version"));
    if (!contentVersion.isValid()) {
        contentVersion = descriptorValue(
            source, descriptor, QStringLiteral("contentVersion"));
    }
    if (contentVersion.isValid()) {
        entry->insert(QStringLiteral("contentVersion"), contentVersion);
        entry->insert(QStringLiteral("version"), contentVersion);
    }
    for (const QString &key : {
             QStringLiteral("versionStrength"),
             QStringLiteral("storageClass"),
             QStringLiteral("accessProfile"),
             QStringLiteral("mimeType"),
             QStringLiteral("sizeKnown")}) {
        const QVariant value = descriptorValue(source, descriptor, key);
        if (value.isValid()) {
            entry->insert(key, value);
        }
    }
    const QVariant size = descriptorValue(
        source, descriptor, QStringLiteral("size"));
    if (size.isValid()) {
        entry->insert(QStringLiteral("size"), size);
    }
}

void insertCatalogDisplayFields(QVariantMap *entry,
                                const QVariantMap &source,
                                bool metadataDeferred)
{
    for (const QString &key : {QStringLiteral("displayBaseName"),
                               QStringLiteral("displayExtension")}) {
        if (source.contains(key)) {
            entry->insert(key, source.value(key));
        }
    }
    if (source.contains(QStringLiteral("localPath"))) {
        entry->insert(QStringLiteral("localPath"),
                      source.value(QStringLiteral("localPath")));
    } else if (!metadataDeferred
               && source.contains(QStringLiteral("path"))) {
        entry->insert(QStringLiteral("localPath"),
                      source.value(QStringLiteral("path")));
    }
    for (const QString &key : {QStringLiteral("isHidden"),
                               QStringLiteral("isImage"),
                               QStringLiteral("sizeText"),
                               QStringLiteral("sizeCalculated"),
                               QStringLiteral("highlightStyleId")}) {
        if (source.contains(key)) {
            entry->insert(key, source.value(key));
        }
    }
    if (source.contains(QStringLiteral("mtimeNanos"))) {
        entry->insert(QStringLiteral("mtimeNs"),
                      source.value(QStringLiteral("mtimeNanos")));
    }
    if (source.contains(QStringLiteral("mtime"))) {
        entry->insert(QStringLiteral("mtimeText"),
                      source.value(QStringLiteral("mtime")));
    }
    if (source.contains(QStringLiteral("mode"))) {
        entry->insert(QStringLiteral("modeText"),
                      source.value(QStringLiteral("mode")));
    }
}

QVariantMap resolveFileIconStyle(F4IconSet *iconSet, QVariantMap style,
                                 const QVariantMap &context,
                                 bool metadataDeferred,
                                 qreal devicePixelRatio,
                                 qulonglong mtimeNanos)
{
    if (!iconSet) {
        return style;
    }
    const QString configuredIcon =
        style.value(QStringLiteral("icon")).toString();
    const bool replaceableIcon = configuredIcon.isEmpty()
        || isZoinGalleryDefaultIcon(configuredIcon)
        || isF4SystemFileIcon(configuredIcon, iconSet->providerId())
        || (iconSet->system() && isF4BundledLucideIcon(configuredIcon));
    if (!replaceableIcon) {
        return style;
    }
    if (iconSet->system()) {
        style.remove(QStringLiteral("iconKey"));
    }
    const bool hasMarker = !style.value(
        QStringLiteral("marker")).toString().isEmpty();
    if (hasMarker && iconSet->system()) {
        style.remove(QStringLiteral("icon"));
        return style;
    }
    const bool isUp = context.value(QStringLiteral("isUp")).toBool();
    const bool directory =
        isUp || context.value(QStringLiteral("isDir")).toBool();
    const QString fileName = isUp
        ? QStringLiteral("..")
        : context.value(QStringLiteral("name")).toString();
    const bool genericSystemIcon = metadataDeferred && iconSet->system();
    const QUrl iconSource = iconSet->fileIconSource(
        genericSystemIcon
            ? QString()
            : context.value(QStringLiteral("localPath")).toString(),
        genericSystemIcon ? QString() : fileName,
        directory, GalleryIconLogicalSize, devicePixelRatio,
        genericSystemIcon ? 0 : mtimeNanos);
    style.insert(QStringLiteral("icon"), iconSource.toString());
    return style;
}

QVariantMap normalizedCatalogEntry(
    const QVariantMap &source, const QVariantMap &styles,
    bool metadataDeferred, qsizetype fallbackRow,
    F4IconSet *iconSet, qreal devicePixelRatio)
{
    QVariantMap entry{
        {QStringLiteral("entryId"),
         source.value(QStringLiteral("entryId"))},
        {QStringLiteral("index"),
         source.value(QStringLiteral("index"), fallbackRow)},
        {QStringLiteral("name"), source.value(QStringLiteral("name"))},
        {QStringLiteral("isDir"), source.value(QStringLiteral("isDir"))},
        {QStringLiteral("isUp"), source.value(QStringLiteral("isUp"))},
        {QStringLiteral("selected"),
         source.value(QStringLiteral("selected"))},
    };
    insertCatalogDisplayFields(&entry, source, metadataDeferred);
    insertSourceDescriptorFields(&entry, source);
    const QString styleId = source.value(
        QStringLiteral("highlightStyleId")).toString();
    QVariantMap style = styleId.isEmpty()
        ? QVariantMap() : styles.value(styleId).toMap();
    style = resolveFileIconStyle(
        iconSet, std::move(style), entry, metadataDeferred,
        devicePixelRatio,
        source.value(QStringLiteral("mtimeNanos")).toULongLong());
    entry.insert(QStringLiteral("highlightStyle"), style);
    return entry;
}

qreal availableDevicePixelRatio()
{
    return qGuiApp ? qGuiApp->devicePixelRatio() : qreal(1);
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

QVariantList F4GalleryBridge::normalizedEntries(
    const QVariantMap &panel) const
{
    const QVariantList sourceEntries = panel.value(
        QStringLiteral("entries")).toList();
    const QVariantMap styles = panel.value(
        QStringLiteral("highlightStyles")).toMap();
    const bool metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred")).toBool();
    const qreal devicePixelRatio = availableDevicePixelRatio();
    QVariantList entries;
    entries.reserve(sourceEntries.size());
    for (qsizetype row = 0; row < sourceEntries.size(); ++row) {
        entries.push_back(normalizedCatalogEntry(
            sourceEntries.at(row).toMap(), styles, metadataDeferred,
            row, m_iconSet, devicePixelRatio));
    }
    return entries;
}

QVariantList F4GalleryBridge::normalizedMetadataEntries(
    int side, int offset, const QVariantList &sourceEntries,
    const QVariantMap &highlightStyles) const
{
    QVariantList entries;
    if (!validSide(side)) {
        return entries;
    }
    const SideState &state = m_panelSessions.catalog(side);
    entries.reserve(sourceEntries.size());
    const qreal devicePixelRatio = availableDevicePixelRatio();
    for (qsizetype index = 0; index < sourceEntries.size(); ++index) {
        const QVariantMap source = sourceEntries.at(index).toMap();
        const QVariantMap base = catalogEntryAt(state, offset + index);
        QVariantMap entry = source;
        if (source.contains(QStringLiteral("mtimeNanos"))) {
            entry.insert(QStringLiteral("mtimeNs"),
                         source.value(QStringLiteral("mtimeNanos")));
        }
        if (source.contains(QStringLiteral("mtime"))) {
            entry.insert(QStringLiteral("mtimeText"),
                         source.value(QStringLiteral("mtime")));
        }
        if (source.contains(QStringLiteral("mode"))) {
            entry.insert(QStringLiteral("modeText"),
                         source.value(QStringLiteral("mode")));
        }

        const QString styleId = source.value(
            QStringLiteral("highlightStyleId")).toString();
        QVariantMap style = styleId.isEmpty()
            ? QVariantMap() : highlightStyles.value(styleId).toMap();
        QVariantMap iconContext = base;
        iconContext.insert(QStringLiteral("localPath"),
                           entry.value(QStringLiteral("localPath")));
        style = resolveFileIconStyle(
            m_iconSet, std::move(style), iconContext, false,
            devicePixelRatio,
            source.value(QStringLiteral("mtimeNanos")).toULongLong());
        entry.insert(QStringLiteral("highlightStyle"), style);
        entries.push_back(entry);
    }
    return entries;
}

void F4GalleryBridge::refreshDeferredIconAppearance(int side)
{
    if (!validSide(side) || !m_iconSet) {
        return;
    }
    const SideState &state = m_panelSessions.catalog(side);
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_panelSessions.session(side));
    if (!session || !state.initialized || !state.metadataDeferred
        || !state.metadataComplete) {
        return;
    }

    const qreal devicePixelRatio = availableDevicePixelRatio();
    QVariantList appearance;
    appearance.reserve(state.entries.size());
    for (const QVariant &entryValue : state.entries) {
        const QVariantMap base = entryValue.toMap();
        bool rowOK = false;
        const int row = base.value(QStringLiteral("index")).toInt(&rowOK);
        if (!rowOK || !catalogRowLoaded(state, row)) {
            continue;
        }
        QVariantMap style = session->highlightStyleAt(row);
        QVariantMap iconContext = base;
        iconContext.insert(QStringLiteral("isDir"),
                           session->isDirectoryAt(row));
        iconContext.insert(QStringLiteral("name"),
                           session->entryNameAt(row));
        iconContext.insert(QStringLiteral("localPath"),
                           session->localPathAt(row));
        style = resolveFileIconStyle(
            m_iconSet, std::move(style), iconContext, false,
            devicePixelRatio, 0);
        appearance.push_back(QVariantMap{
            {QStringLiteral("entryId"), session->entryIdAt(row)},
            {QStringLiteral("highlightStyle"), style},
        });
    }
    session->applyExternalAppearance(appearance, state.highlightRevision);
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

bool F4GalleryBridge::activatePanelSession(int side,
                                           const QString &panelId)
{
    if (!validSide(side) || panelId.isEmpty()) {
        return false;
    }
    const size_t index = static_cast<size_t>(side);
    if (m_panelSessions.catalog(static_cast<int>(index)).initialized
        && m_panelSessions.catalog(static_cast<int>(index)).panelId == panelId) {
        return true;
    }
    // Each visible side owns one virtual model. Changing panel identity clears
    // only its bounded materialized rows; there is no retained panel/session
    // lookup and therefore no hidden full-directory model to swap back in.
    m_panelSessions.resetCatalog(static_cast<int>(index));
    m_panelSnapshots[index].clear();
    return true;
}

bool F4GalleryBridge::canSkipUnchangedInactivePanel(
    int side, const QVariantMap &panel) const
{
    if (!validSide(side)) {
        return false;
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_panelSessions.catalog(static_cast<int>(sideIndex));
    if (!state.initialized || state.active
        || panel.value(QStringLiteral("active")).toBool()
        || m_stateReconciliationPending[sideIndex]
        || m_selectionActionPending[sideIndex]
        || m_pendingCursors[sideIndex].active
        || m_pendingSelections[sideIndex].active
        || (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side)
        || (m_viewerCoordinator->pendingIntent().active && m_viewerCoordinator->pendingIntent().side == side)) {
        return false;
    }

    const QString sourceKind = panel.value(
        QStringLiteral("sourceKind"), QStringLiteral("vfs")).toString();
    const bool previewCapable = panel.value(
        QStringLiteral("previewCapable")).toBool()
        && sourceKind == QStringLiteral("local");
    const bool metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred")).toBool();
    const qulonglong iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    return panel.value(QStringLiteral("id")).toString() == state.panelId
        && revisionValue(panel, QStringLiteral("catalogRevision"))
            == state.catalogRevision
        && revisionValue(panel, QStringLiteral("selectionRevision"))
            == state.selectionRevision
        && metadataDeferred == state.metadataDeferred
        && (!metadataDeferred
            ? revisionValue(panel, QStringLiteral("highlightRevision"))
                == state.highlightRevision
            : revisionValue(panel, QStringLiteral("metadataRevision"))
                == state.metadataRevision)
        && iconRevision == state.iconRevision
        && panel.value(QStringLiteral("path")).toString()
            == state.currentPath
        && sourceKind == state.sourceKind
        && previewCapable == state.previewCapable
        && panel.value(QStringLiteral("cursorEntryId")).toString()
            == state.cursorEntryId
        && panel.value(QStringLiteral("cursor"), -1).toInt()
            == state.cursorIndex
        && panel.value(QStringLiteral("loading")).toBool()
            == state.loading
        && panel.value(QStringLiteral("catalogProvisional")).toBool()
            == state.catalogProvisional
        && panel.value(QStringLiteral("catalogRowsDeferred")).toBool()
            == state.catalogRowsDeferred
        && panel.value(QStringLiteral("totalCount"),
                       panel.value(QStringLiteral("entries"))
                           .toList().size()).toInt()
            == state.totalCount
        && panel.value(QStringLiteral("galleryLayoutMode")).toString()
            == state.galleryLayoutMode;
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
