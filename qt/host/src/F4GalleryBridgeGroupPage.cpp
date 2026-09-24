#include "F4GalleryBridge.h"

#include <ZoinGallery/GalleryGroupIndex.h>

#include <QTimer>

#include <utility>

namespace
{
constexpr int GroupPageSize = 512;

bool integerValue(const QVariantMap &map, const QString &key, int *value)
{
    bool ok = false;
    const int converted = map.value(key).toInt(&ok);
    if (!ok || converted < 0) {
        return false;
    }
    if (value) {
        *value = converted;
    }
    return true;
}
}

int F4GalleryBridge::matchingGroupPageSide(
    const QVariantMap &message) const
{
    int offset = -1;
    qulonglong catalogRevision = 0;
    bool revisionOK = false;
    catalogRevision = message.value(QStringLiteral("catalogRevision"))
                          .toULongLong(&revisionOK);
    if (!revisionOK || !integerValue(message, QStringLiteral("offset"),
                                     &offset)
        || !message.contains(QStringLiteral("panelId"))
        || !message.contains(QStringLiteral("path"))) {
        return -1;
    }

    int match = -1;
    for (int side = 0; side < PanelSessionRegistry::PanelCount; ++side) {
        const SideState &state = m_panelSessions.catalog(side);
        if (!state.initialized || !state.groupsDeferred
            || !state.groupPageRequestInFlight
            || state.panelId != message.value(QStringLiteral("panelId"))
                                      .toString()
            || state.currentPath != message.value(QStringLiteral("path"))
                                      .toString()
            || state.catalogRevision != catalogRevision
            || state.groupPageRequestOffset != offset) {
            continue;
        }
        if (match != -1) {
            return -1;
        }
        match = side;
    }
    return match;
}

void F4GalleryBridge::resetPanelGroupPage(int side)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    state.groupPageRequestInFlight = false;
    state.groupPageRequestOffset = -1;
    state.groupPageRequestLimit = 0;
    state.pendingGroupDescriptors.clear();
    state.pendingGroupKeys.clear();
}

void F4GalleryBridge::requestPanelGroupPage(int side)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    if (!state.initialized || !state.groupsDeferred
        || state.groupCatalogReady || state.groupPageRequestInFlight
        || state.panelId.isEmpty() || state.groupTotal <= 0
        || state.pendingGroupDescriptors.size() >= state.groupTotal) {
        return;
    }
    const int offset = state.pendingGroupDescriptors.size();
    const int limit = qMin(GroupPageSize, state.groupTotal - offset);
    if (limit <= 0) {
        return;
    }
    state.groupPageRequestInFlight = true;
    state.groupPageRequestOffset = offset;
    state.groupPageRequestLimit = limit;
    emit panelGroupPageRequested({
        {QStringLiteral("panelId"), state.panelId},
        {QStringLiteral("path"), state.currentPath},
        {QStringLiteral("catalogRevision"),
         QVariant::fromValue<qulonglong>(state.catalogRevision)},
        {QStringLiteral("offset"), offset},
        {QStringLiteral("limit"), limit},
    });
}

void F4GalleryBridge::schedulePanelGroupPageRequest(int side)
{
    if (!validSide(side)
        || m_groupPageRequestScheduled[static_cast<size_t>(side)]) {
        return;
    }
    m_groupPageRequestScheduled[static_cast<size_t>(side)] = true;
    QTimer::singleShot(0, this, [this, side]() {
        m_groupPageRequestScheduled[static_cast<size_t>(side)] = false;
        requestPanelGroupPage(side);
    });
}

void F4GalleryBridge::commitPanelGroupCatalog(
    int side, const QVariantList &groups, qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    if (!state.initialized || state.catalogRevision != catalogRevision) {
        return;
    }

    ZoinGallery::GalleryGroupIndex validator;
    if (!validator.setDescriptors(groups, state.totalCount)) {
        // A malformed peer response is a recoverable protocol error. Keep
        // the file catalog usable, but never expose an invalid partial index.
        state.groupDescriptors.clear();
        state.groupCatalogReady = true;
        state.groupsDeferred = false;
        state.groupCatalogRejected = true;
        state.groupTotal = 0;
        resetPanelGroupPage(side);
        ++m_groupCatalogEpoch;
        emit groupCatalogEpochChanged();
        emit panelGroupCatalogChanged(side, catalogRevision);
        return;
    }

    state.groupDescriptors = groups;
    state.groupCatalogReady = true;
    state.groupsDeferred = false;
    state.groupCatalogRejected = false;
    state.groupTotal = groups.size();
    resetPanelGroupPage(side);
    ++m_groupCatalogEpoch;
    emit groupCatalogEpochChanged();
    emit panelGroupCatalogChanged(side, catalogRevision);
}

class F4GalleryGroupPageResponseReducer
{
public:
    F4GalleryGroupPageResponseReducer(
        F4GalleryBridge &bridge, const QVariantMap &message)
        : m_bridge(bridge), m_message(message)
    {
    }

    void run();

private:
    bool resolveTarget();
    bool parseEnvelope();
    bool validatePage() const;
    void clearRequest();
    void reject();

    F4GalleryBridge &m_bridge;
    const QVariantMap &m_message;
    QString m_type;
    int m_side = -1;
    F4GalleryBridge::SideState *m_state = nullptr;
    int m_offset = -1;
    int m_limit = 0;
    int m_total = 0;
    QVariantList m_groups;
};

bool F4GalleryGroupPageResponseReducer::resolveTarget()
{
    m_type = m_message.value(QStringLiteral("type")).toString();
    if (m_type != QStringLiteral("panel_group_page")
        && m_type != QStringLiteral("panel_group_page_rejected")) {
        return false;
    }
    m_side = m_bridge.matchingGroupPageSide(m_message);
    if (!F4GalleryBridge::validSide(m_side)) {
        return false;
    }
    m_state = &m_bridge.m_panelSessions.catalog(m_side);
    return true;
}

void F4GalleryGroupPageResponseReducer::clearRequest()
{
    m_state->groupPageRequestInFlight = false;
    m_state->groupPageRequestOffset = -1;
    m_state->groupPageRequestLimit = 0;
}

bool F4GalleryGroupPageResponseReducer::parseEnvelope()
{
    bool limitOK = false;
    bool totalOK = false;
    m_limit = m_message.value(QStringLiteral("limit")).toInt(&limitOK);
    m_total = m_message.value(QStringLiteral("total")).toInt(&totalOK);
    const QVariant groupsValue = m_message.value(QStringLiteral("groups"));
    m_offset = m_state->groupPageRequestOffset;
    if (!limitOK || !totalOK || m_limit <= 0
        || m_limit != m_state->groupPageRequestLimit
        || m_total != m_state->groupTotal
        || groupsValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    m_groups = groupsValue.toList();
    return m_groups.size() == m_limit && m_offset >= 0
        && m_offset + m_limit <= m_total;
}

bool F4GalleryGroupPageResponseReducer::validatePage() const
{
    if (m_offset != m_state->pendingGroupDescriptors.size()
        || m_limit > GroupPageSize) {
        return false;
    }
    int expectedStart = 0;
    QSet<QString> seenKeys = m_state->pendingGroupKeys;
    if (m_offset == 0 && !m_groups.isEmpty()) {
        const QVariantMap first = m_groups.first().toMap();
        bool startOK = false;
        const int firstStart = first.value(QStringLiteral("startIndex"))
                                    .toInt(&startOK);
        // The Go panel keeps .. at source index zero but excludes it from
        // group ranges. No other leading gap is valid.
        if (!startOK || firstStart < 0 || firstStart > 1) {
            return false;
        }
        expectedStart = firstStart;
    }
    if (!m_state->pendingGroupDescriptors.isEmpty()) {
        const QVariantMap previous = m_state->pendingGroupDescriptors.last().toMap();
        bool startOK = false;
        bool countOK = false;
        const int start = previous.value(QStringLiteral("startIndex"))
                               .toInt(&startOK);
        const int count = previous.value(QStringLiteral("count"))
                               .toInt(&countOK);
        if (!startOK || !countOK || count <= 0) {
            return false;
        }
        expectedStart = start + count;
    }

    for (const QVariant &value : m_groups) {
        if (value.metaType().id() != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap group = value.toMap();
        const QString key = group.value(QStringLiteral("key")).toString();
        bool startOK = false;
        bool countOK = false;
        const int start = group.value(QStringLiteral("startIndex"))
                              .toInt(&startOK);
        const int count = group.value(QStringLiteral("count"))
                              .toInt(&countOK);
        if (key.isEmpty() || seenKeys.contains(key)
            || !startOK || !countOK || count <= 0 || start != expectedStart
            || start + count > m_state->totalCount) {
            return false;
        }
        expectedStart = start + count;
        seenKeys.insert(key);
    }
    if (m_offset + m_limit == m_total
        && (m_groups.isEmpty()
            || expectedStart != m_state->totalCount)) {
        return false;
    }
    return true;
}

void F4GalleryGroupPageResponseReducer::reject()
{
    m_state->pendingGroupDescriptors.clear();
    m_state->pendingGroupKeys.clear();
    clearRequest();
    m_state->groupsDeferred = false;
    m_state->groupCatalogReady = true;
    m_state->groupCatalogRejected = true;
    m_state->groupDescriptors.clear();
    // Keep the advertised total as the rejection fence.  The producer will
    // continue sending scalar row-free updates for this catalog revision;
    // changing the total to zero here would make every such update look like
    // a new snapshot and restart the same rejected page request.
}

void F4GalleryGroupPageResponseReducer::run()
{
    if (!resolveTarget()) {
        return;
    }
    if (m_type == QStringLiteral("panel_group_page_rejected")
        || !parseEnvelope() || !validatePage()) {
        reject();
        return;
    }

    if (m_offset == 0) {
        m_state->pendingGroupDescriptors.reserve(m_total);
    }
    for (const QVariant &value : std::as_const(m_groups)) {
        const QString key = value.toMap().value(QStringLiteral("key"))
                                .toString();
        m_state->pendingGroupDescriptors.push_back(value);
        m_state->pendingGroupKeys.insert(key);
    }
    clearRequest();
    if (m_state->pendingGroupDescriptors.size() == m_total) {
        const QVariantList complete = m_state->pendingGroupDescriptors;
        m_bridge.commitPanelGroupCatalog(
            m_side, complete, m_state->catalogRevision);
    } else {
        m_bridge.schedulePanelGroupPageRequest(m_side);
    }
}

void F4GalleryBridge::handlePanelGroupPageMessage(
    const QVariantMap &message)
{
    F4GalleryGroupPageResponseReducer reducer(*this, message);
    reducer.run();
}
