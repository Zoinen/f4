#include "ExtUiStateStores.h"
#include "NavigationBenchmarkTrace.h"

#include <QMetaType>

#include <functional>

namespace
{
void advanceRevision(qulonglong next, qulonglong *current,
                     const std::function<void()> &changed)
{
    if (next <= *current) {
        return;
    }
    *current = next;
    changed();
}

int integerOrZero(const QVariantMap &state, const QString &key)
{
    bool ok = false;
    const int value = state.value(key).toInt(&ok);
    return ok ? value : 0;
}

QVariantList projectMenuStates(const QVariantList &menus)
{
    QVariantList states;
    states.reserve(menus.size());
    for (const QVariant &menuValue : menus) {
        const QVariantMap menu = menuValue.toMap();
        if (menu.isEmpty()) {
            continue;
        }
        states.push_back(QVariantMap{
            {QStringLiteral("id"), menu.value(QStringLiteral("id"))},
            {QStringLiteral("selected"),
             menu.value(QStringLiteral("selected"))},
            {QStringLiteral("top"), menu.value(QStringLiteral("top"))},
        });
    }
    return states;
}

QVariantMap menuStructure(QVariantMap menu)
{
    menu.remove(QStringLiteral("selected"));
    menu.remove(QStringLiteral("top"));
    return menu;
}

bool menuStructuresEqual(const QVariantList &left,
                         const QVariantList &right)
{
    if (left.size() != right.size()) {
        return false;
    }
    for (qsizetype index = 0; index < left.size(); ++index) {
        if (menuStructure(left.at(index).toMap())
            != menuStructure(right.at(index).toMap())) {
            return false;
        }
    }
    return true;
}
}

ChromeStateStore::ChromeStateStore(QObject *parent)
    : QObject(parent)
{
}

void ChromeStateStore::applyState(const QVariantMap &state,
                                  qulonglong revision)
{
    advanceRevision(revision, &m_revision,
                    [this] { emit revisionChanged(); });
    const QString schema = state.value(QStringLiteral("schema")).toString();
    const int version = integerOrZero(state, QStringLiteral("version"));
    if (schema != m_schema || version != m_version) {
        m_schema = schema;
        m_version = version;
        emit identityChanged();
    }
    const int width = integerOrZero(state, QStringLiteral("width"));
    const int height = integerOrZero(state, QStringLiteral("height"));
    if (width != m_width || height != m_height) {
        m_width = width;
        m_height = height;
        emit geometryChanged();
    }
    const QString presentation = state.value(
        QStringLiteral("presentation")).toString();
    if (presentation != m_presentation) {
        m_presentation = presentation;
        emit presentationChanged();
    }
    const QString iconSet = state.value(
        QStringLiteral("qmlIconSet")).toString();
    if (iconSet != m_qmlIconSet) {
        m_qmlIconSet = iconSet;
        emit qmlIconSetChanged(m_qmlIconSet);
    }
    const QVariantMap keyBar = state.value(
        QStringLiteral("keyBar")).toMap();
    if (keyBar != m_keyBar) {
        m_keyBar = keyBar;
        emit keyBarChanged();
    }
    const QVariantMap toast = state.value(QStringLiteral("toast")).toMap();
    if (toast != m_toast) {
        m_toast = toast;
        emit toastChanged();
    }
}

void ChromeStateStore::reset()
{
    applyState({}, m_revision + 1);
}

WorkspaceStateStore::WorkspaceStateStore(QObject *parent)
    : QObject(parent)
{
}

void WorkspaceStateStore::applyState(const QVariantMap &state,
                                     qulonglong revision)
{
    advanceRevision(revision, &m_revision,
                    [this] { emit revisionChanged(); });
    const int activeScreen = integerOrZero(
        state, QStringLiteral("activeScreen"));
    if (activeScreen != m_activeScreen) {
        m_activeScreen = activeScreen;
        emit activeScreenChanged();
    }
    const int workspaceCount = integerOrZero(
        state, QStringLiteral("workspaceCount"));
    if (workspaceCount != m_workspaceCount) {
        m_workspaceCount = workspaceCount;
        emit workspaceCountChanged();
    }
    const QVariantMap tabs = state.value(
        QStringLiteral("workspaceTabs")).toMap();
    if (tabs != m_tabs) {
        m_tabs = tabs;
        emit tabsChanged();
    }
}

void WorkspaceStateStore::reset()
{
    applyState({}, m_revision + 1);
}

OverlayStateStore::OverlayStateStore(QObject *parent)
    : QObject(parent)
{
}

void OverlayStateStore::applyMenuState(const QVariantMap &state,
                                       qulonglong revision,
                                       bool allowStateOnlyUpdate)
{
    advanceRevision(revision, &m_menuRevision,
                    [this] { emit menuRevisionChanged(); });
    const QVariantMap menuBar = state.value(
        QStringLiteral("menuBar")).toMap();
    if (menuBar != m_menuBar) {
        m_menuBar = menuBar;
        emit menuBarChanged();
    }

    const QVariantList menus = state.value(
        QStringLiteral("menus")).toList();
    const QVariantList states = projectMenuStates(menus);
    if (allowStateOnlyUpdate
        && menuStructuresEqual(m_commandMenus, menus)) {
        if (states != m_commandMenuStates) {
            m_commandMenuStates = states;
            emit commandMenuStatesChanged(m_commandMenuStates);
        }
        return;
    }
    const bool structureChanged = menus != m_commandMenus;
    const bool stateChanged = states != m_commandMenuStates;
    m_commandMenus = menus;
    m_commandMenuStates = states;
    if (structureChanged) {
        emit commandMenusChanged();
    }
    if (stateChanged) {
        emit commandMenuStatesChanged(m_commandMenuStates);
    }
}

void OverlayStateStore::applyDialogsState(const QVariantMap &state,
                                          qulonglong revision)
{
    // Dialog geometry is committed optimistically by QML. A delayed frame
    // from the same stream must not roll that state back after a newer frame
    // has already been accepted.
    if (revision < m_dialogRevision) {
        return;
    }
    advanceRevision(revision, &m_dialogRevision,
                    [this] { emit dialogRevisionChanged(); });
    const QVariantList dialogs = state.value(
        QStringLiteral("dialogs")).toList();
    if (dialogs != m_dialogs) {
        m_dialogs = dialogs;
        emit dialogsChanged();
    }
}

void OverlayStateStore::reset()
{
    applyMenuState({}, m_menuRevision + 1, false);
    applyDialogsState({}, m_dialogRevision + 1);
}

CommandLineStateStore::CommandLineStateStore(QObject *parent)
    : QObject(parent)
{
}

void CommandLineStateStore::applyFrame(const QVariantMap &frame,
                                       qulonglong revision)
{
    advanceRevision(revision, &m_revision,
                    [this] { emit revisionChanged(); });
    if (frame != m_frame) {
        m_frame = frame;
        emit frameChanged();
    }
}

void CommandLineStateStore::reset()
{
    applyFrame({}, m_revision + 1);
}

int DocumentRowsModel::rowCount(const QModelIndex &parent) const
{
    return parent.isValid() ? 0 : m_slots.size();
}

QVariant DocumentRowsModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() < 0 || index.row() >= m_slots.size())
        return {};
    const auto &slot = m_slots.at(index.row());
    if (role == LoadedRole)
        return slot.loaded;
    if (role == RowDataRole)
        return slot.row;
    return {};
}

QHash<int, QByteArray> DocumentRowsModel::roleNames() const
{
    return {{LoadedRole, "loaded"}, {RowDataRole, "rowData"}};
}

QVariantMap DocumentRowsModel::get(int slot) const
{
    if (slot < 0 || slot >= m_slots.size())
        return {};
    return {{QStringLiteral("loaded"), m_slots.at(slot).loaded},
            {QStringLiteral("rowData"), m_slots.at(slot).row}};
}

void DocumentRowsModel::ensureCapacity(int capacity)
{
    if (capacity <= m_slots.size())
        return;
    beginInsertRows({}, m_slots.size(), capacity - 1);
    m_slots.resize(capacity);
    endInsertRows();
    emit countChanged();
}

void DocumentRowsModel::set(int slot, const QVariantMap &value)
{
    if (slot < 0 || slot >= m_slots.size())
        return;
    const bool loaded = value.value(QStringLiteral("loaded")).toBool();
    const auto row = value.value(QStringLiteral("rowData")).toMap();
    auto &target = m_slots[slot];
    if (target.loaded == loaded && target.row == row)
        return;
    if (m_changedFirst < 0 && F4NavigationBenchmarkTrace::enabled())
        m_updateStartedNs = F4NavigationBenchmarkTrace::monotonicNanoseconds();
    target.loaded = loaded;
    target.row = row;
    target.changed = true;
    m_changedFirst = m_changedFirst < 0 ? slot : qMin(m_changedFirst, slot);
    m_changedLast = qMax(m_changedLast, slot);
}

int DocumentRowsModel::replaceWindow(int start, const QVariantList &rows,
                                     int previousStart, int previousEnd,
                                     bool forceReplacement, bool clearOutside)
{
    if (start < 0 || start + rows.size() > m_slots.size())
        return 0;
    const int end = start + rows.size();
    int writes = 0;
    const auto replaceSlot = [this, &writes](int slot, bool loaded,
                                             const QVariantMap &row,
                                             bool force) {
        auto &target = m_slots[slot];
        if (!force && target.loaded == loaded) {
            if (!loaded)
                return;
            // contentKey is a cross-process optimization hint, not the row
            // payload. Never suppress an authoritative nested run/style
            // mutation merely because a producer reused a stale key.
            if (target.row == row)
                return;
        }
        if (m_changedFirst < 0 && F4NavigationBenchmarkTrace::enabled())
            m_updateStartedNs = F4NavigationBenchmarkTrace::monotonicNanoseconds();
        target.loaded = loaded;
        target.row = row;
        target.changed = true;
        m_changedFirst = m_changedFirst < 0 ? slot : qMin(m_changedFirst, slot);
        m_changedLast = qMax(m_changedLast, slot);
        ++writes;
    };
    if (clearOutside) {
        for (int slot = qMax(0, previousStart);
             slot < qMin(previousEnd, m_slots.size()); ++slot) {
            if (slot < start || slot >= end)
                replaceSlot(slot, false, {}, false);
        }
    }
    for (qsizetype index = 0; index < rows.size(); ++index)
        replaceSlot(start + index, true, rows.at(index).toMap(),
                    forceReplacement);
    return writes;
}

void DocumentRowsModel::commit()
{
    if (m_changedFirst < 0)
        return;
    const int first = m_changedFirst, last = m_changedLast;
    m_changedFirst = m_changedLast = -1;
    const bool tracing = F4NavigationBenchmarkTrace::enabled();
    const qint64 beginNs = tracing ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    // Disjoint edge updates must not invalidate unchanged visible rows.
    for (int start = first; start <= last;) {
        if (!m_slots.at(start).changed) {
            ++start;
            continue;
        }
        int end = start;
        while (end < last && m_slots.at(end + 1).changed)
            ++end;
        for (int slot = start; slot <= end; ++slot)
            m_slots[slot].changed = false;
        emit dataChanged(index(start), index(end), {LoadedRole, RowDataRole});
        start = end + 1;
    }
    if (tracing) {
        F4NavigationBenchmarkTrace::event(QStringLiteral("qt.document.rows.committed"), {}, {
            {QStringLiteral("first"), first}, {QStringLiteral("last"), last},
            {QStringLiteral("capacity"), m_slots.size()},
            {QStringLiteral("updateNs"), beginNs - m_updateStartedNs},
            {QStringLiteral("notificationNs"), F4NavigationBenchmarkTrace::monotonicNanoseconds() - beginNs},
        });
    }
}

QObject *SurfaceRegistry::createDocumentRowsModel(QObject *owner)
{
    return new DocumentRowsModel(owner ? owner : this);
}

SurfaceRegistry::SurfaceRegistry(QObject *parent)
    : QObject(parent)
{
}

QVariantMap SurfaceRegistry::withoutCatalogPayload(const QVariantMap &shell)
{
    QVariantMap bounded = shell;
    bounded.remove(QStringLiteral("commandLine"));
    const QVariant panelsValue = bounded.value(QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return bounded;
    }
    QVariantList panels = panelsValue.toList();
    for (qsizetype index = 0; index < panels.size(); ++index) {
        if (panels.at(index).metaType().id() != QMetaType::QVariantMap) {
            continue;
        }
        QVariantMap panel = panels.at(index).toMap();
        panel.remove(QStringLiteral("entries"));
        panel.remove(QStringLiteral("highlightStyles"));
        panels[index] = panel;
    }
    bounded.insert(QStringLiteral("panels"), panels);
    return bounded;
}

void SurfaceRegistry::applyShell(const QVariantMap &shell,
                                 qulonglong revision)
{
    advanceRevision(revision, &m_shellRevision,
                    [this] { emit shellRevisionChanged(); });
    const QVariantMap bounded = withoutCatalogPayload(shell);
    if (bounded != m_shell) {
        m_shell = bounded;
        emit shellChanged();
    }
}

void SurfaceRegistry::adoptShellPanelDescriptor(
    int side, const QVariantMap &panel)
{
    if (side < 0 || side > 1 || panel.isEmpty() || m_shell.isEmpty())
        return;

    QVariantMap descriptor = panel;
    descriptor.remove(QStringLiteral("entries"));
    descriptor.remove(QStringLiteral("highlightStyles"));

    QVariantList panels = m_shell.value(QStringLiteral("panels")).toList();
    bool replaced = false;
    for (qsizetype index = 0; index < panels.size(); ++index) {
        const QVariantMap current = panels.at(index).toMap();
        bool declaredSideOK = false;
        const int declaredSide = current.value(QStringLiteral("side"))
                                     .toInt(&declaredSideOK);
        if ((declaredSideOK ? declaredSide : static_cast<int>(index)) != side)
            continue;
        panels[index] = descriptor;
        replaced = true;
        break;
    }
    if (!replaced)
        panels.push_back(descriptor);
    m_shell.insert(QStringLiteral("panels"), panels);
    // Deliberately no shellChanged(): an accepted catalog publishes its live
    // QML delta through compactPresentationChanged. This backing update exists
    // so a catalog delivered during QML construction is not lost and a later
    // shell projection reset cannot resurrect an obsolete loading descriptor.
}

void SurfaceRegistry::applyDocument(const QVariantMap &document,
                                    qulonglong revision)
{
    advanceRevision(revision, &m_documentRevision,
                    [this] { emit documentRevisionChanged(); });
    if (document != m_document) {
        m_document = document;
        ++m_documentPublication;
        emit documentChanged();
    }
}

QVariantMap SurfaceRegistry::documentMetadata() const
{
    QVariantMap metadata = m_document;
    const QString kind = metadata.value(QStringLiteral("kind")).toString();
    if ((kind == QStringLiteral("viewer") || kind == QStringLiteral("editor"))
        && metadata.contains(QStringLiteral("windowRows"))) {
        // Scalar QML bindings must not repeatedly convert the row-bearing
        // document map. The one current native transaction remains the source
        // of rows, read only when the viewport atomically installs this epoch.
        metadata.remove(QStringLiteral("rows"));
        metadata.remove(QStringLiteral("windowRows"));
        metadata.insert(QStringLiteral("nativeWindowRows"), true);
        metadata.insert(QStringLiteral("nativeWindowRevision"), m_documentPublication);
    }
    return metadata;
}

QVariant SurfaceRegistry::documentWindowRows(const QString &documentKey,
                                             qulonglong publication) const
{
    QString currentKey = m_document.value(QStringLiteral("documentKey")).toString();
    if (currentKey.isEmpty())
        currentKey = m_document.value(QStringLiteral("id")).toString();
    if (publication != m_documentPublication || documentKey != currentKey)
        return {};
    return m_document.value(QStringLiteral("windowRows"));
}

void SurfaceRegistry::applyOperationsQueue(const QVariantMap &queue,
                                           qulonglong revision)
{
    advanceRevision(revision, &m_operationsRevision,
                    [this] { emit operationsRevisionChanged(); });
    if (queue != m_operationsQueue) {
        m_operationsQueue = queue;
        emit operationsQueueChanged();
    }
}

void SurfaceRegistry::applyDocumentState(const QVariantMap &document,
                                        qulonglong revision)
{
    advanceRevision(revision, &m_documentRevision,
                    [this] { emit documentRevisionChanged(); });
    m_document = document;
}

void SurfaceRegistry::reset()
{
    applyShell({}, m_shellRevision + 1);
    applyDocument({}, m_documentRevision + 1);
    applyOperationsQueue({}, m_operationsRevision + 1);
}
