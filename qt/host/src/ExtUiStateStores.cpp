#include "ExtUiStateStores.h"

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

void SurfaceRegistry::applyDocument(const QVariantMap &document,
                                    qulonglong revision)
{
    advanceRevision(revision, &m_documentRevision,
                    [this] { emit documentRevisionChanged(); });
    if (document != m_document) {
        m_document = document;
        emit documentChanged();
    }
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

void SurfaceRegistry::reset()
{
    applyShell({}, m_shellRevision + 1);
    applyDocument({}, m_documentRevision + 1);
    applyOperationsQueue({}, m_operationsRevision + 1);
}
