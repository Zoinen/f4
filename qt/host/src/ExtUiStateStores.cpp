#include "ExtUiStateStores.h"
#include "NavigationBenchmarkTrace.h"

#include <QMetaType>
#include <QCryptographicHash>
#include <QDataStream>

#include <functional>
#include <algorithm>

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
    // Update the payload before publishing its revision.  QML consumers use
    // menuRevision as the dependency for overlayFrames(); emitting it first
    // lets that binding observe the previous menu list and cache a stale
    // popup until some unrelated property changes.
    const QVariantMap menuBar = state.value(
        QStringLiteral("menuBar")).toMap();
    // Menu-bar actions address the active bar by category/item index. Its Go
    // object ID changes between workspaces without changing that presentation.
    // Keep delegates alive when only this routing identity changes.
    QVariantMap previousBarPresentation = m_menuBar;
    QVariantMap nextBarPresentation = menuBar;
    previousBarPresentation.remove(QStringLiteral("id"));
    nextBarPresentation.remove(QStringLiteral("id"));
    const bool barPresentationChanged =
        previousBarPresentation != nextBarPresentation;
    m_menuBar = menuBar;
    if (barPresentationChanged)
        emit menuBarChanged();

    const QVariantList menus = state.value(
        QStringLiteral("menus")).toList();
    const QVariantList states = projectMenuStates(menus);
    if (allowStateOnlyUpdate
        && menuStructuresEqual(m_commandMenus, menus)) {
        const bool stateChanged = states != m_commandMenuStates;
        if (stateChanged) {
            m_commandMenuStates = states;
            emit commandMenuStatesChanged(m_commandMenuStates);
        }
        if (revision > m_menuRevision) {
            advanceRevision(revision, &m_menuRevision,
                            [this] { emit menuRevisionChanged(); });
        } else if (stateChanged) {
            // A state-only update can share the current stream revision
            // during a cross-stream reconciliation. The menu payload still
            // changed and overlayFrames() needs an invalidation notification.
            emit menuRevisionChanged();
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
    if (revision > m_menuRevision) {
        advanceRevision(revision, &m_menuRevision,
                        [this] { emit menuRevisionChanged(); });
    } else if (structureChanged || stateChanged) {
        // Keep the helper-function binding live even when an accepted payload
        // does not advance the wire revision.
        emit menuRevisionChanged();
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
    // Dialogs and their revision form one observable state transition.  The
    // revision is used by QML as the invalidation key for overlayFrames(); it
    // must not be signalled while m_dialogs still contains the prior frame.
    const QVariantList dialogs = state.value(
        QStringLiteral("dialogs")).toList();
    const bool payloadChanged = dialogs != m_dialogs;
    if (payloadChanged) {
        m_dialogs = dialogs;
        emit dialogsChanged();
    }
    if (revision > m_dialogRevision) {
        advanceRevision(revision, &m_dialogRevision,
                        [this] { emit dialogRevisionChanged(); });
    } else if (payloadChanged) {
        // Revisions identify the wire stream, while the payload is the QML
        // invalidation source. A same-revision accepted replacement must
        // still force overlayFrames() to reevaluate.
        emit dialogRevisionChanged();
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

namespace {
bool sameDocumentRowPresentation(QVariantMap first, QVariantMap second)
{
    if (first == second)
        return true;
    // The producer's index is relative to its moving window. Neither it nor
    // its content hash is painted; compare the authoritative payload itself.
    first.remove(QStringLiteral("index"));
    second.remove(QStringLiteral("index"));
    first.remove(QStringLiteral("contentKey"));
    second.remove(QStringLiteral("contentKey"));
    return first == second;
}

QString inlineEditorMarkup(const QVariantMap &row, const QString &defaultBackground)
{
    const auto runs = row.value(QStringLiteral("runs")).toList();
    if (runs.isEmpty())
        return {};
    const auto flag = [](const QVariantMap &run, const char *name) {
        const auto value = run.value(QLatin1String(name));
        return value.metaType().id() == QMetaType::Bool && value.toBool();
    };
    const auto spacesOnly = [](const QString &text) {
        return std::all_of(text.cbegin(), text.cend(), [](QChar ch) { return ch == QLatin1Char(' '); });
    };
    QList<QVariantMap> values;
    values.reserve(runs.size());
    for (const auto &value : runs) {
        const auto run = value.toMap();
        const auto text = run.value(QStringLiteral("text")).toString();
        const auto background = run.value(QStringLiteral("background")).toString().toLower();
        if (run.value(QStringLiteral("foreground")).toString().isEmpty()
                || (!background.isEmpty() && background != defaultBackground
                    && background != QLatin1String("transparent")
                    && background != QLatin1String("#000000")
                    && background != QLatin1String("#ff000000")
                    && background != QLatin1String("black")))
            return {};
        for (const QChar ch : text) {
            if (ch.unicode() < 0x20 || ch.unicode() > 0x7e)
                return {};
        }
        values.append(run);
    }
    qsizetype last = values.size() - 1;
    while (last >= 0 && !flag(values.at(last), "underline")
            && !flag(values.at(last), "strikeout")
            && spacesOnly(values.at(last).value(QStringLiteral("text")).toString()))
        --last;
    QString result;
    for (qsizetype i = 0; i <= last; ++i) {
        const auto &run = values.at(i);
        QString text = run.value(QStringLiteral("text")).toString();
        if (i == last && !flag(run, "underline") && !flag(run, "strikeout")) {
            while (text.endsWith(QLatin1Char(' '))) text.chop(1);
        }
        text = text.toHtmlEscaped().replace(QLatin1Char(' '), QStringLiteral("&nbsp;"));
        if (flag(run, "bold")) text = QStringLiteral("<b>") + text + QStringLiteral("</b>");
        if (flag(run, "underline")) text = QStringLiteral("<u>") + text + QStringLiteral("</u>");
        if (flag(run, "strikeout")) text = QStringLiteral("<s>") + text + QStringLiteral("</s>");
        result += QStringLiteral("<font color=\"")
            + run.value(QStringLiteral("foreground")).toString().toHtmlEscaped()
            + QStringLiteral("\">") + text + QStringLiteral("</font>");
    }
    return result.isEmpty() ? QStringLiteral("<font></font>") : result;
}
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
    target.changedRoles |= (target.loaded != loaded ? 1 : 0)
                         | (target.row != row ? 2 : 0);
    target.loaded = loaded;
    target.row = row;
    m_changedFirst = m_changedFirst < 0 ? slot : qMin(m_changedFirst, slot);
    m_changedLast = qMax(m_changedLast, slot);
}

int DocumentRowsModel::replaceWindow(int start, const QVariantList &rows,
                                     int previousStart, int previousEnd,
                                     bool forceReplacement, bool clearOutside,
                                     bool retainInactiveRows)
{
    if (start < 0 || start + rows.size() > m_slots.size())
        return 0;
    const int end = start + rows.size();
    int writes = 0;
    const auto replaceSlot = [this, &writes, retainInactiveRows](int slot, bool loaded,
                                             const QVariantMap &row,
                                             bool force) {
        auto &target = m_slots[slot];
        if (!force && target.loaded == loaded) {
            if (!loaded && (retainInactiveRows || target.row.isEmpty()))
                return;
            // contentKey is a cross-process optimization hint, not the row
            // payload. Never suppress an authoritative nested run/style
            // mutation merely because a producer reused a stale key.
            if (sameDocumentRowPresentation(target.row, row)) {
                target.row = row;
                return;
            }
        }
        if (m_changedFirst < 0 && F4NavigationBenchmarkTrace::enabled())
            m_updateStartedNs = F4NavigationBenchmarkTrace::monotonicNanoseconds();
        const bool replaceRow = loaded || !retainInactiveRows;
        const bool rowChanged = replaceRow && !sameDocumentRowPresentation(target.row, row);
        target.changedRoles |= (force || target.loaded != loaded ? 1 : 0)
                             | (force || rowChanged ? 2 : 0);
        target.loaded = loaded;
        if (replaceRow)
            target.row = row;
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
    int rowDataChanges = 0;
    // Disjoint edge updates must not invalidate unchanged visible rows.
    for (int start = first; start <= last;) {
        if (!m_slots.at(start).changedRoles) {
            ++start;
            continue;
        }
        int end = start;
        const int roles = m_slots.at(start).changedRoles;
        while (end < last && m_slots.at(end + 1).changedRoles == roles)
            ++end;
        if (roles & 2) rowDataChanges += end - start + 1;
        for (int slot = start; slot <= end; ++slot)
            m_slots[slot].changedRoles = 0;
        QList<int> changedRoles;
        if (roles & 1) changedRoles.append(LoadedRole);
        if (roles & 2) changedRoles.append(RowDataRole);
        emit dataChanged(index(start), index(end), changedRoles);
        start = end + 1;
    }
    if (tracing) {
        F4NavigationBenchmarkTrace::event(QStringLiteral("qt.document.rows.committed"), {}, {
            {QStringLiteral("first"), first}, {QStringLiteral("last"), last},
            {QStringLiteral("capacity"), m_slots.size()},
            {QStringLiteral("rowDataChanges"), rowDataChanges},
            {QStringLiteral("updateNs"), beginNs - m_updateStartedNs},
            {QStringLiteral("notificationNs"), F4NavigationBenchmarkTrace::monotonicNanoseconds() - beginNs},
        });
    }
}

bool DocumentRowsModel::moveWindow(int sourceStart, int count, int destinationStart)
{
    if (count <= 0 || sourceStart < 0 || destinationStart < 0
            || sourceStart + count > m_slots.size()
            || destinationStart + count > m_slots.size())
        return false;
    if (sourceStart == destinationStart)
        return true;
    commit();
    const int destinationChild = destinationStart > sourceStart
        ? destinationStart + count : destinationStart;
    if (!beginMoveRows({}, sourceStart, sourceStart + count - 1, {}, destinationChild))
        return false;
    if (destinationStart < sourceStart)
        std::rotate(m_slots.begin() + destinationStart, m_slots.begin() + sourceStart,
                    m_slots.begin() + sourceStart + count);
    else
        std::rotate(m_slots.begin() + sourceStart, m_slots.begin() + sourceStart + count,
                    m_slots.begin() + destinationStart + count);
    endMoveRows();
    return true;
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
        if (metadata.value(QStringLiteral("windowContentKey")).toString().isEmpty()) {
            QByteArray content;
            QDataStream stream(&content, QIODevice::WriteOnly);
            stream << m_document.value(QStringLiteral("windowRows"));
            metadata.insert(QStringLiteral("nativeWindowContentKey"),
                QString::fromLatin1(QCryptographicHash::hash(content, QCryptographicHash::Sha256).toHex()));
        }
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

QVariant SurfaceRegistry::documentWindowCoordinates(const QString &documentKey,
                                                    qulonglong publication) const
{
    const QVariant rows = documentWindowRows(documentKey, publication);
    if (!rows.isValid())
        return {};
    QVariantList coordinates;
    for (const auto &value : rows.toList()) {
        const auto row = value.toMap();
        coordinates.append(row.value(QStringLiteral("visualRow")));
    }
    return coordinates;
}

int SurfaceRegistry::replaceDocumentWindow(QObject *model, const QString &documentKey,
                                           qulonglong publication, int start,
                                           int previousStart, int previousEnd,
                                           bool forceReplacement, bool clearOutside) const
{
    auto *rowsModel = qobject_cast<DocumentRowsModel *>(model);
    const QVariant rows = documentWindowRows(documentKey, publication);
    if (!rowsModel || !rows.isValid())
        return -1;
    auto presentationRows = rows.toList();
    if (m_document.value(QStringLiteral("kind")) == QStringLiteral("editor")
            && m_document.value(QStringLiteral("viewportColumns")).toInt() > 0) {
        const auto background = m_document.value(QStringLiteral("defaultBackground")).toString().toLower();
        for (auto &value : presentationRows) {
            auto row = value.toMap();
            row.insert(QStringLiteral("_inlineMarkup"), inlineEditorMarkup(row, background));
            value = row;
        }
    }
    return rowsModel->replaceWindow(start, presentationRows, previousStart, previousEnd,
                                    forceReplacement, clearOutside,
                                    m_document.value(QStringLiteral("kind")) == QStringLiteral("editor"));
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
