#include "F4GalleryBridge.h"

#include "NavigationBenchmarkTrace.h"

#include <ZoinGallery/GallerySession.h>

namespace
{
const QSet<QString> &catalogAppendEntryKeys()
{
    static const QSet<QString> keys = {
        QStringLiteral("index"), QStringLiteral("entryId"),
        QStringLiteral("name"), QStringLiteral("displayBaseName"),
        QStringLiteral("displayExtension"), QStringLiteral("isDir"),
        QStringLiteral("isUp"), QStringLiteral("isImage"),
        QStringLiteral("isHidden"), QStringLiteral("selected"),
        QStringLiteral("highlightStyleId"), QStringLiteral("source"),
    };
    return keys;
}

const QSet<QString> &catalogAppendSourceKeys()
{
    static const QSet<QString> keys = {
        QStringLiteral("resourceId"), QStringLiteral("sourceKey"),
        QStringLiteral("version"), QStringLiteral("versionStrength"),
        QStringLiteral("size"), QStringLiteral("sizeKnown"),
        QStringLiteral("accessProfile"), QStringLiteral("storageClass"),
    };
    return keys;
}

bool catalogAppendInteger(const QVariant &value)
{
    const int type = value.metaType().id();
    const bool integer = type == QMetaType::Char
        || type == QMetaType::SChar || type == QMetaType::UChar
        || type == QMetaType::Short || type == QMetaType::UShort
        || type == QMetaType::Int || type == QMetaType::UInt
        || type == QMetaType::LongLong || type == QMetaType::ULongLong;
    bool ok = false;
    value.toLongLong(&ok);
    return integer && ok;
}

bool validCatalogAppendSource(const QVariant &value)
{
    if (value.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    const QVariantMap source = value.toMap();
    for (auto it = source.cbegin(); it != source.cend(); ++it) {
        if (!catalogAppendSourceKeys().contains(it.key())) {
            return false;
        }
        if (it.key() == QStringLiteral("size")) {
            if (!catalogAppendInteger(it.value())) {
                return false;
            }
        } else if (it.key() == QStringLiteral("sizeKnown")) {
            if (it.value().metaType().id() != QMetaType::Bool) {
                return false;
            }
        } else if (it.value().metaType().id() != QMetaType::QString) {
            return false;
        }
    }
    return true;
}

bool validCatalogAppendEntry(
    const QVariant &value, int expectedRow,
    const QSet<QString> &existingIds, const QSet<QString> &chunkIds,
    QString *entryId)
{
    if (value.metaType().id() != QMetaType::QVariantMap || !entryId) {
        return false;
    }
    const QVariantMap entry = value.toMap();
    for (auto it = entry.cbegin(); it != entry.cend(); ++it) {
        if (!catalogAppendEntryKeys().contains(it.key())) {
            return false;
        }
    }
    bool indexOK = false;
    const int row = entry.value(QStringLiteral("index")).toInt(&indexOK);
    const QVariant idValue = entry.value(QStringLiteral("entryId"));
    *entryId = idValue.toString();
    const bool baseFieldsValid = indexOK && row == expectedRow
        && idValue.metaType().id() == QMetaType::QString
        && !entryId->isEmpty() && !existingIds.contains(*entryId)
        && !chunkIds.contains(*entryId)
        && entry.value(QStringLiteral("name")).metaType().id()
            == QMetaType::QString
        && entry.value(QStringLiteral("isDir")).metaType().id()
            == QMetaType::Bool
        && entry.value(QStringLiteral("isUp")).metaType().id()
            == QMetaType::Bool
        && entry.value(QStringLiteral("isImage")).metaType().id()
            == QMetaType::Bool
        && entry.value(QStringLiteral("selected")).metaType().id()
            == QMetaType::Bool;
    if (!baseFieldsValid) {
        return false;
    }
    if (entry.contains(QStringLiteral("isHidden"))
        && entry.value(QStringLiteral("isHidden")).metaType().id()
            != QMetaType::Bool) {
        return false;
    }
    for (const QString &key : {QStringLiteral("displayBaseName"),
                               QStringLiteral("displayExtension")}) {
        if (entry.contains(key)
            && entry.value(key).metaType().id() != QMetaType::QString) {
            return false;
        }
    }
    if (entry.contains(QStringLiteral("highlightStyleId"))
        && (entry.value(QStringLiteral("highlightStyleId")).metaType().id()
                != QMetaType::QString
            || entry.value(QStringLiteral("highlightStyleId"))
                   .toString().isEmpty())) {
        return false;
    }
    return !entry.contains(QStringLiteral("source"))
        || validCatalogAppendSource(entry.value(QStringLiteral("source")));
}
}

struct F4GalleryBridge::CatalogAppendContext
{
    int side = -1;
    size_t sideIndex = 0;
    SideState *state = nullptr;
    ZoinGallery::GallerySession *session = nullptr;
    QVariantMap snapshot;
    QVariantList rawEntries;
    QVariantList normalizedEntries;
    QString panelId;
    qulonglong catalogRevision = 0;
    int offset = -1;
    int totalCount = -1;
    int currentEntryCount = 0;
    bool final = false;
};

bool F4GalleryBridge::parseCatalogAppend(
    const QVariantMap &append, CatalogAppendContext *context)
{
    if (!context) {
        return false;
    }
    bool sideOK = false;
    context->side = append.value(QStringLiteral("side")).toInt(&sideOK);
    if (!sideOK || !validSide(context->side)) {
        return false;
    }
    context->sideIndex = static_cast<size_t>(context->side);
    context->state = &m_panelSessions.catalog(context->side);
    context->snapshot = m_panelSnapshots[context->sideIndex];
    context->panelId = append.value(QStringLiteral("panelId")).toString();
    context->catalogRevision = revisionValue(
        append, QStringLiteral("catalogRevision"));

    bool offsetOK = false;
    bool totalOK = false;
    context->offset = append.value(QStringLiteral("offset")).toInt(&offsetOK);
    context->totalCount = append.value(
        QStringLiteral("totalCount")).toInt(&totalOK);
    const QVariant entriesValue = append.value(QStringLiteral("entries"));
    const QVariant finalValue = append.value(QStringLiteral("final"));
    if (entriesValue.metaType().id() != QMetaType::QVariantList
        || finalValue.metaType().id() != QMetaType::Bool) {
        return false;
    }
    context->rawEntries = entriesValue.toList();
    context->final = finalValue.toBool();
    context->currentEntryCount = context->state->entries.size();
    const int expectedTotal = context->snapshot.value(
        QStringLiteral("totalCount"), context->currentEntryCount).toInt();
    return context->state->initialized && context->state->catalogProvisional
        && context->state->metadataDeferred && !context->panelId.isEmpty()
        && context->panelId == context->state->panelId
        && context->catalogRevision == context->state->catalogRevision
        && !context->rawEntries.isEmpty() && offsetOK
        && context->offset >= 0 && totalOK && context->totalCount > 0
        && expectedTotal == context->totalCount
        && context->offset == context->currentEntryCount
        && context->offset < context->totalCount
        && context->rawEntries.size()
            <= context->totalCount - context->offset
        && context->final == (context->rawEntries.size()
                              == context->totalCount - context->offset);
}

bool F4GalleryBridge::prepareCatalogAppendIdentityIndex(
    CatalogAppendContext *context)
{
    SideState &state = *context->state;
    if (state.entryIds.size() == context->currentEntryCount) {
        return true;
    }
    state.entryIds.clear();
    for (const QVariant &value : state.entries) {
        const QString entryId = value.toMap().value(
            QStringLiteral("entryId")).toString();
        if (entryId.isEmpty()) {
            return false;
        }
        state.entryIds.insert(entryId);
    }
    return true;
}

bool F4GalleryBridge::validateCatalogAppendRows(
    CatalogAppendContext *context) const
{
    QSet<QString> chunkIds;
    chunkIds.reserve(context->rawEntries.size());
    for (qsizetype index = 0; index < context->rawEntries.size(); ++index) {
        QString entryId;
        if (!validCatalogAppendEntry(
                context->rawEntries.at(index), context->offset + index,
                context->state->entryIds, chunkIds, &entryId)) {
            return false;
        }
        chunkIds.insert(entryId);
    }
    return true;
}

bool F4GalleryBridge::applyCatalogAppendRows(CatalogAppendContext *context)
{
    context->session = qobject_cast<ZoinGallery::GallerySession *>(
        m_panelSessions.session(context->side));
    if (!context->session) {
        return false;
    }
    QVariantMap chunkPanel{
        {QStringLiteral("entries"), context->rawEntries},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("highlightStyles"), context->snapshot.value(
            QStringLiteral("highlightStyles"))},
    };
    context->normalizedEntries = normalizedEntries(chunkPanel);
    return context->normalizedEntries.size() == context->rawEntries.size()
        && context->session->appendExternalCatalog(
            context->normalizedEntries, context->catalogRevision,
            context->offset, context->final);
}

void F4GalleryBridge::commitCatalogAppendRows(CatalogAppendContext *context)
{
    SideState &state = *context->state;
    state.entries.reserve(context->totalCount);
    for (const QVariant &value : context->normalizedEntries) {
        const QVariantMap entry = value.toMap();
        const QString entryId = entry.value(
            QStringLiteral("entryId")).toString();
        state.entries.append(value);
        state.entryIds.insert(entryId);
        state.sourceIndexByEntryId.insert(
            entryId, entry.value(QStringLiteral("index"),
                                 state.sourceIndexByEntryId.size()).toInt());
        if (entry.value(QStringLiteral("selected")).toBool()
            && !state.selectedEntryIds.contains(entryId)) {
            state.selectedEntryIds.insert(entryId);
            state.selectedEntryIdList.push_back(entryId);
        }
    }
    state.catalogProvisional = !context->final;
}

void F4GalleryBridge::finalizeCatalogAppend(CatalogAppendContext *context)
{
    SideState &state = *context->state;
    QVariantMap &snapshot = m_panelSnapshots[context->sideIndex];
    snapshot.insert(QStringLiteral("catalogProvisional"),
                    state.catalogProvisional);
    snapshot.insert(QStringLiteral("totalCount"), context->totalCount);
    if (!context->final) {
        return;
    }
    snapshot.insert(QStringLiteral("entries"), state.entries);
    context->session->applyExternalState(
        state.cursorEntryId, state.cursorIndex,
        state.selectedEntryIdList, state.selectionRevision);
    resetPanelCatalogMetadataPlan(context->side, false);
    m_stateReconciliationPending[context->sideIndex] = false;
    reconcilePendingSelection(context->side);
    reconcilePendingCursor(context->side);
    reconcilePendingPanelOpen(context->side);
    reconcilePendingViewer(context->side);
}

void F4GalleryBridge::traceCatalogAppend(
    const CatalogAppendContext &context) const
{
    if (!F4NavigationBenchmarkTrace::enabled()) {
        return;
    }
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.gallery.bridge.catalog.append"),
        m_lastInputSceneTraceId, {
            {QStringLiteral("side"), context.side},
            {QStringLiteral("offset"), context.offset},
            {QStringLiteral("chunkEntries"), context.rawEntries.size()},
            {QStringLiteral("total"), context.totalCount},
            {QStringLiteral("final"), context.final},
        });
}

void F4GalleryBridge::synchronizePanelCatalogAppend(
    const QVariantMap &append)
{
    CatalogAppendContext context;
    if (!parseCatalogAppend(append, &context)
        || !prepareCatalogAppendIdentityIndex(&context)
        || !validateCatalogAppendRows(&context)
        || !applyCatalogAppendRows(&context)) {
        return;
    }
    commitCatalogAppendRows(&context);
    finalizeCatalogAppend(&context);
    traceCatalogAppend(context);
    schedulePanelCatalogMetadataRequest();
}
