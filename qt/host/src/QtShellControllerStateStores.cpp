#include "QtShellController.h"

#include "ExtUiSceneReducer.h"

#include <QMetaType>
#include <QSet>

using namespace ExtUiSceneReducer;

namespace
{
QVariantMap rootState(const QVariantMap &scene,
                      std::initializer_list<const char *> keys)
{
    QVariantMap state;
    for (const char *key : keys) {
        const QString name = QString::fromLatin1(key);
        if (scene.contains(name)) {
            state.insert(name, scene.value(name));
        }
    }
    return state;
}

QVariantMap projectChromeState(const QVariantMap &scene)
{
    return rootState(scene, {"schema", "version", "width", "height",
                             "presentation", "qmlIconSet", "keyBar",
                             "toast"});
}

QVariantMap projectWorkspaceState(const QVariantMap &scene)
{
    return rootState(scene, {"activeScreen", "workspaceCount",
                             "workspaceTabs"});
}

QVariantMap menuState(const QVariantMap &scene)
{
    return rootState(scene, {"menuBar", "menus"});
}

QVariantMap dialogState(const QVariantMap &scene)
{
    return rootState(scene, {"dialogs"});
}

QSet<QString> mapPatchKeys(const QVariant &patchValue)
{
    QSet<QString> keys;
    if (patchValue.metaType().id() != QMetaType::QVariantMap) {
        return keys;
    }
    const QVariantMap patch = patchValue.toMap();
    const QVariantMap set = patch.value(QStringLiteral("set")).toMap();
    for (auto it = set.cbegin(); it != set.cend(); ++it) {
        keys.insert(it.key());
    }
    for (const QVariant &key : patch.value(
             QStringLiteral("clear")).toList()) {
        keys.insert(key.toString());
    }
    return keys;
}

bool intersects(const QSet<QString> &left,
                std::initializer_list<const char *> right)
{
    for (const char *key : right) {
        if (left.contains(QString::fromLatin1(key))) {
            return true;
        }
    }
    return false;
}

QVariantMap chromeStoreState(const ChromeStateStore *store)
{
    return {
        {QStringLiteral("schema"), store->schema()},
        {QStringLiteral("version"), store->version()},
        {QStringLiteral("width"), store->width()},
        {QStringLiteral("height"), store->height()},
        {QStringLiteral("presentation"), store->presentation()},
        {QStringLiteral("qmlIconSet"), store->qmlIconSet()},
        {QStringLiteral("keyBar"), store->keyBar()},
        {QStringLiteral("toast"), store->toast()},
    };
}

QVariantMap workspaceStoreState(const WorkspaceStateStore *store)
{
    return {
        {QStringLiteral("activeScreen"), store->activeScreen()},
        {QStringLiteral("workspaceCount"), store->workspaceCount()},
        {QStringLiteral("workspaceTabs"), store->tabs()},
    };
}

QVariantMap menuStoreState(const OverlayStateStore *store)
{
    return {
        {QStringLiteral("menuBar"), store->menuBar()},
        {QStringLiteral("menus"), store->commandMenus()},
    };
}

void mergeMap(QVariantMap *target, const QVariantMap &source)
{
    for (auto it = source.cbegin(); it != source.cend(); ++it) {
        target->insert(it.key(), it.value());
    }
}

void replaceOrAppendPanel(QVariantMap *shell, int side,
                          const QVariantMap &panel)
{
    QVariantList panels = shell->value(QStringLiteral("panels")).toList();
    for (qsizetype index = 0; index < panels.size(); ++index) {
        const QVariantMap current = panels.at(index).toMap();
        bool declaredSideOK = false;
        const int declaredSide = current.value(QStringLiteral("side"))
                                     .toInt(&declaredSideOK);
        if ((declaredSideOK ? declaredSide : static_cast<int>(index)) == side) {
            panels[index] = panel;
            shell->insert(QStringLiteral("panels"), panels);
            return;
        }
    }
    panels.push_back(panel);
    shell->insert(QStringLiteral("panels"), panels);
}
}

QVariantMap QtShellController::streamReducerScene(
    const QString &streamId) const
{
    QVariantMap scene = chromeStoreState(m_chromeState);
    mergeMap(&scene, workspaceStoreState(m_workspaceState));
    mergeMap(&scene, menuStoreState(m_overlayState));
    scene.insert(QStringLiteral("dialogs"), m_overlayState->dialogs());
    if (streamId == QStringLiteral("operations")) {
        scene.insert(QStringLiteral("operationsQueue"),
                     m_surfaceRegistry->operationsQueue());
    }
    if (streamId.startsWith(QStringLiteral("document/"))) {
        scene.insert(QStringLiteral("surface"), m_surfaceRegistry->document());
    }

    QVariantMap shell = m_surfaceRegistry->shell();
    if (shell.isEmpty()) {
        shell = {
            {QStringLiteral("id"), m_shellState->id()},
            {QStringLiteral("title"), m_shellState->title()},
            {QStringLiteral("mode"), m_shellState->mode()},
            {QStringLiteral("activePanel"), m_shellState->activePanel()},
            {QStringLiteral("showPanels"), m_shellState->showPanels()},
            {QStringLiteral("showLeftPanel"),
             m_shellState->showLeftPanel()},
            {QStringLiteral("showRightPanel"),
             m_shellState->showRightPanel()},
            {QStringLiteral("wide"), m_shellState->wide()},
            {QStringLiteral("widePanel"), m_shellState->widePanel()},
            {QStringLiteral("terminalActive"),
             m_shellState->terminalActive()},
            {QStringLiteral("terminalBusy"),
             m_shellState->terminalBusy()},
            {QStringLiteral("fallback"), m_shellState->fallback()},
            {QStringLiteral("reason"), m_shellState->fallbackReason()},
        };
    }
    shell.insert(QStringLiteral("commandLine"),
                 m_commandLineState->frame());
    if (streamId.startsWith(QStringLiteral("panel/"))
        || streamId.startsWith(QStringLiteral("panel-id/"))) {
        for (int side = 0; side < 2; ++side) {
            const QVariantMap panel = panelCatalogSnapshot(side);
            if (!panel.isEmpty()) {
                replaceOrAppendPanel(&shell, side, panel);
            }
        }
    }
    scene.insert(QStringLiteral("shell"), shell);
    scene.insert(QStringLiteral("schema"), QStringLiteral("app"));
    scene.insert(QStringLiteral("version"), 4);
    return scene;
}

void QtShellController::commitTypedScenePatch(
    const QString &streamId, const AppliedScenePatch &applied)
{
    const qulonglong revision = applied.revision;
    if (intersects(applied.rootKeys,
                   {"schema", "version", "width", "height",
                    "presentation", "qmlIconSet", "keyBar", "toast"})) {
        m_chromeState->applyState(projectChromeState(applied.scene), revision);
    }
    if (intersects(applied.rootKeys,
                   {"activeScreen", "workspaceCount", "workspaceTabs"})) {
        m_workspaceState->applyState(projectWorkspaceState(applied.scene),
                                     revision);
    }
    if (intersects(applied.rootKeys, {"menuBar", "menus"})) {
        m_overlayState->applyMenuState(menuState(applied.scene), revision,
                                       true);
    }
    if (applied.rootKeys.contains(QStringLiteral("dialogs"))) {
        m_overlayState->applyDialogsState(dialogState(applied.scene),
                                          revision);
    }
    if (applied.rootKeys.contains(QStringLiteral("operationsQueue"))) {
        m_surfaceRegistry->applyOperationsQueue(
            applied.scene.value(QStringLiteral("operationsQueue")).toMap(),
            revision);
    }

    const bool rootShell = applied.rootKeys.contains(
        QStringLiteral("shell"));
    if (rootShell || !applied.shellKeys.isEmpty()) {
        const QVariantMap shell = applied.scene.value(
            QStringLiteral("shell")).toMap();
        m_shellState->applyShell(shell, revision);
        if (rootShell || streamId == QStringLiteral("shell")) {
            m_surfaceRegistry->applyShell(shell, revision);
        }
        if (rootShell
            || applied.shellKeys.contains(QStringLiteral("commandLine"))) {
            m_commandLineState->applyFrame(
                shell.value(QStringLiteral("commandLine")).toMap(),
                revision);
        }
    }
    if (applied.rootKeys.contains(QStringLiteral("surface"))
        || !applied.surfaceKeys.isEmpty()) {
        m_surfaceRegistry->applyDocument(
            applied.scene.value(QStringLiteral("surface")).toMap(),
            revision);
    }

    QSet<int> changedPanelSides;
    for (const QVariantMap &panel : applied.catalogPanels) {
        changedPanelSides.insert(panel.value(QStringLiteral("side")).toInt());
    }
    for (const QVariantMap &append : applied.catalogAppends) {
        changedPanelSides.insert(append.value(QStringLiteral("side")).toInt());
    }
    for (const QVariantMap &patch : applied.panelPatches) {
        changedPanelSides.insert(patch.value(QStringLiteral("side")).toInt());
    }
    for (int side : changedPanelSides) {
        QVariantMap panel;
        if (side >= 0 && side < 2
            && shellPanelAtSide(applied.scene, side, &panel)) {
            m_panelCatalogSnapshots[static_cast<size_t>(side)] = panel;
        }
    }
}

void QtShellController::applyCompactFieldsToTypedState(
    const QVariantMap &message, int activePanel, qulonglong revision,
    bool replacePanelDescriptor, const QVariantMap &panel)
{
    QVariantMap shell = m_surfaceRegistry->shell();
    shell.insert(QStringLiteral("activePanel"), activePanel);
    if (message.contains(QStringLiteral("shellTitle"))) {
        shell.insert(QStringLiteral("title"),
                     message.value(QStringLiteral("shellTitle")));
    }
    if (message.contains(QStringLiteral("commandLine"))) {
        shell.insert(QStringLiteral("commandLine"),
                     message.value(QStringLiteral("commandLine")));
        m_commandLineState->applyFrame(
            message.value(QStringLiteral("commandLine")).toMap(), revision);
    }
    if (replacePanelDescriptor && !panel.isEmpty()) {
        const int side = panel.value(QStringLiteral("side")).toInt();
        replaceOrAppendPanel(&shell, side, withoutNativePanelPayload(panel));
    }
    for (int side = 0; side < 2; ++side) {
        const QVariantMap &catalog =
            m_panelCatalogSnapshots[static_cast<size_t>(side)];
        if (!catalog.isEmpty()) {
            replaceOrAppendPanel(&shell, side,
                                 withoutNativePanelPayload(catalog));
        }
    }
    QVariantList descriptors = shell.value(QStringLiteral("panels")).toList();
    for (qsizetype index = 0; index < descriptors.size(); ++index) {
        QVariantMap descriptor = descriptors.at(index).toMap();
        bool sideOK = false;
        const int side = descriptor.value(QStringLiteral("side"))
                             .toInt(&sideOK);
        descriptor.insert(QStringLiteral("active"),
                          (sideOK ? side : static_cast<int>(index))
                              == activePanel);
        descriptors[index] = descriptor;
    }
    shell.insert(QStringLiteral("panels"), descriptors);
    m_shellState->applyShell(shell, revision);
    if (!replacePanelDescriptor) {
        m_surfaceRegistry->applyShell(shell, revision);
    }

    if (message.contains(QStringLiteral("workspaceTabs"))) {
        QVariantMap workspaces = workspaceStoreState(m_workspaceState);
        workspaces.insert(QStringLiteral("workspaceTabs"),
                          message.value(QStringLiteral("workspaceTabs")));
        m_workspaceState->applyState(workspaces, revision);
    }
    if (message.contains(QStringLiteral("menus"))) {
        QVariantMap menus = menuStoreState(m_overlayState);
        menus.insert(QStringLiteral("menus"),
                     message.value(QStringLiteral("menus")));
        m_overlayState->applyMenuState(menus, revision, true);
    }
}

void QtShellController::synchronizeAllTypedState(
    qulonglong revision, bool allowMenuStateOnlyUpdate)
{
    m_chromeState->applyState(projectChromeState(m_scene), revision);
    m_workspaceState->applyState(projectWorkspaceState(m_scene), revision);
    m_overlayState->applyMenuState(menuState(m_scene), revision,
                                   allowMenuStateOnlyUpdate);
    m_overlayState->applyDialogsState(dialogState(m_scene), revision);
    const QVariantMap shell = m_scene.value(
        QStringLiteral("shell")).toMap();
    m_shellState->applyShell(shell, revision);
    m_commandLineState->applyFrame(shell.value(
        QStringLiteral("commandLine")).toMap(), revision);
    m_surfaceRegistry->applyShell(shell, revision);
    m_surfaceRegistry->applyDocument(m_scene.value(
        QStringLiteral("surface")).toMap(), revision);
    m_surfaceRegistry->applyOperationsQueue(m_scene.value(
        QStringLiteral("operationsQueue")).toMap(), revision);
}

void QtShellController::synchronizeTypedState(
    const QString &messageType, const QVariantMap &message,
    const ExtUiProtocol::Envelope &envelope, bool hasSemanticEnvelope)
{
    if (!hasSemanticEnvelope) {
        synchronizeLegacyTypedState(
            messageType, message,
            qMax(m_sceneRevision, m_panelActivationRevision));
        return;
    }
    synchronizeTypedStreamOwnedState(messageType, message, envelope);
    if (messageType == QStringLiteral("scene_patch")) {
        synchronizeTypedCrossStreamPatch(message, envelope);
    }
}

void QtShellController::synchronizeLegacyTypedState(
    const QString &messageType, const QVariantMap &message,
    qulonglong revision)
{
    if (messageType == QStringLiteral("scene")
        || messageType == QStringLiteral("scene_patch")) {
        synchronizeAllTypedState(
            revision, messageType == QStringLiteral("scene_patch"));
        return;
    }
    const QVariantMap shell = m_scene.value(
        QStringLiteral("shell")).toMap();
    if (messageType == QStringLiteral("command_line")) {
        m_commandLineState->applyFrame(shell.value(
            QStringLiteral("commandLine")).toMap(), revision);
        m_overlayState->applyMenuState(menuState(m_scene), revision, true);
        return;
    }
    if (messageType != QStringLiteral("panel_chrome")
        && messageType != QStringLiteral("panel_activation")
        && messageType != QStringLiteral("panel_catalog")) {
        return;
    }
    m_shellState->applyShell(shell, revision);
    if (messageType != QStringLiteral("panel_catalog")) {
        m_surfaceRegistry->applyShell(shell, revision);
    }
    if (message.contains(QStringLiteral("commandLine"))) {
        m_commandLineState->applyFrame(shell.value(
            QStringLiteral("commandLine")).toMap(), revision);
    }
}

void QtShellController::synchronizeTypedStreamOwnedState(
    const QString &messageType, const QVariantMap &message,
    const ExtUiProtocol::Envelope &envelope)
{
    const qulonglong revision = envelope.revision;
    const QString &stream = envelope.streamId;
    if (stream == QStringLiteral("chrome")) {
        m_chromeState->applyState(projectChromeState(m_scene), revision);
    } else if (stream == QStringLiteral("workspaces")) {
        m_workspaceState->applyState(projectWorkspaceState(m_scene), revision);
    } else if (stream == QStringLiteral("menus")) {
        m_overlayState->applyMenuState(menuState(m_scene), revision,
                                       messageType
                                           == QStringLiteral("scene_patch"));
    } else if (stream == QStringLiteral("dialogs")) {
        m_overlayState->applyDialogsState(dialogState(m_scene), revision);
    } else if (stream == QStringLiteral("operations")) {
        m_surfaceRegistry->applyOperationsQueue(m_scene.value(
            QStringLiteral("operationsQueue")).toMap(), revision);
    } else if (stream == QStringLiteral("command-line")) {
        const QVariantMap shell = m_scene.value(
            QStringLiteral("shell")).toMap();
        m_commandLineState->applyFrame(shell.value(
            QStringLiteral("commandLine")).toMap(), revision);
        if (message.contains(QStringLiteral("menus"))) {
            m_overlayState->applyMenuState(menuState(m_scene), revision,
                                           true);
        }
    } else if (stream == QStringLiteral("shell")) {
        const QVariantMap shell = m_scene.value(
            QStringLiteral("shell")).toMap();
        m_shellState->applyShell(shell, revision);
        m_surfaceRegistry->applyShell(shell, revision);
        if (message.contains(QStringLiteral("commandLine"))) {
            m_commandLineState->applyFrame(shell.value(
                QStringLiteral("commandLine")).toMap(), revision);
        }
    } else if (stream.startsWith(QStringLiteral("document/"))
               && messageType != QStringLiteral("scene_patch")) {
        m_surfaceRegistry->applyDocument(m_scene.value(
            QStringLiteral("surface")).toMap(), revision);
    } else if ((stream.startsWith(QStringLiteral("panel/"))
                || stream.startsWith(QStringLiteral("panel-id/")))
               && messageType == QStringLiteral("scene_patch")
               && !mapPatchKeys(message.value(
                       QStringLiteral("shell"))).isEmpty()) {
        // Panel operations can carry a tiny shell title/layout delta. Fixed
        // shell roles may consume it, but the surface registry deliberately
        // keeps its previous row-free panel descriptors until the shell
        // stream commits, avoiding panel-wide QML invalidation per cursor.
        m_shellState->applyShell(m_scene.value(
            QStringLiteral("shell")).toMap(), revision);
    } else if ((stream.startsWith(QStringLiteral("panel/"))
                || stream.startsWith(QStringLiteral("panel-id/")))
               && message.contains(QStringLiteral("commandLine"))) {
        m_commandLineState->applyFrame(m_scene.value(
            QStringLiteral("shell")).toMap().value(
                QStringLiteral("commandLine")).toMap(), revision);
    }
}

void QtShellController::synchronizeTypedCrossStreamPatch(
    const QVariantMap &message, const ExtUiProtocol::Envelope &envelope)
{
    const qulonglong revision = envelope.revision;
    const QString &stream = envelope.streamId;
    const QSet<QString> rootKeys = mapPatchKeys(message.value(
        QStringLiteral("root")));
    if (intersects(rootKeys, {"schema", "version", "width", "height",
                              "presentation", "qmlIconSet", "keyBar",
                              "toast"})
        && stream != QStringLiteral("chrome")) {
        m_chromeState->applyState(projectChromeState(m_scene), revision);
    }
    if (intersects(rootKeys, {"activeScreen", "workspaceCount",
                              "workspaceTabs"})
        && stream != QStringLiteral("workspaces")) {
        m_workspaceState->applyState(projectWorkspaceState(m_scene), revision);
    }
    if (intersects(rootKeys, {"menuBar", "menus"})
        && stream != QStringLiteral("menus")) {
        m_overlayState->applyMenuState(menuState(m_scene), revision, true);
    }
    if (rootKeys.contains(QStringLiteral("dialogs"))
        && stream != QStringLiteral("dialogs")) {
        m_overlayState->applyDialogsState(dialogState(m_scene), revision);
    }
    if (rootKeys.contains(QStringLiteral("operationsQueue"))
        && stream != QStringLiteral("operations")) {
        m_surfaceRegistry->applyOperationsQueue(m_scene.value(
            QStringLiteral("operationsQueue")).toMap(), revision);
    }
}
