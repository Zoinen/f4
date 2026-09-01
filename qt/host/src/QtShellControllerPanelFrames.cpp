#include "QtShellController.h"

#include "ExtUiSceneReducer.h"
#include "NavigationBenchmarkTrace.h"
#include "QtShellControllerFrameApply.h"

#include <QElapsedTimer>
#include <QMetaType>

#include <utility>

using namespace ExtUiSceneReducer;

bool QtShellController::applyPanelCatalogFrame(
    const QVariantMap &message, const ExtUiProtocol::Envelope &envelope,
    bool hasSemanticEnvelope, FrameApplyTrace *trace)
{
    int side = -1;
    QVariantMap panel;
    QElapsedTimer timer;
    if (trace->enabled) {
        timer.start();
    }
#if defined(F4_QT_SCENE_TEST_API)
    const QVariantMap &validationScene = m_scene;
#else
    const QVariantMap validationScene = streamReducerScene(envelope.streamId);
#endif
    const bool valid = validPanelCatalogEnvelope(
        message, validationScene, &side, &panel);
    if (trace->enabled) {
        trace->catalogValidationDurationNs = timer.nsecsElapsed();
    }
    if (!valid) {
        if (trace->enabled) {
            traceRejectedPanelCatalog(message, panel, *trace);
        }
        return false;
    }
    if (commitPanelCatalogFrame(message, side, panel, envelope,
                                hasSemanticEnvelope, trace)) {
        trace->compactProtocolApplied = true;
        return true;
    }
    return false;
}

void QtShellController::traceRejectedPanelCatalog(
    const QVariantMap &message, const QVariantMap &panel,
    const FrameApplyTrace &trace) const
{
    const QVariantMap rejectedPanel = panel.isEmpty()
        ? message.value(QStringLiteral("panel")).toMap() : panel;
    const int side = message.value(QStringLiteral("side")).toInt();
#if defined(F4_QT_SCENE_TEST_API)
    const QVariantMap currentScene = m_scene;
#else
    const QVariantMap currentScene = streamReducerScene(
        QStringLiteral("panel/%1").arg(side));
#endif
    QVariantMap currentPanel;
    shellPanelAtSide(currentScene, side, &currentPanel);
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.panel_catalog.rejected"), trace.traceId, {
            {QStringLiteral("side"), side},
            {QStringLiteral("activePanel"),
             message.value(QStringLiteral("activePanel"))},
            {QStringLiteral("currentActivePanel"),
             currentScene.value(QStringLiteral("shell")).toMap().value(
                 QStringLiteral("activePanel"))},
            {QStringLiteral("panelId"),
             rejectedPanel.value(QStringLiteral("id"))},
            {QStringLiteral("currentPanelId"),
             currentPanel.value(QStringLiteral("id"))},
            {QStringLiteral("catalogRevision"),
             rejectedPanel.value(QStringLiteral("catalogRevision"))},
            {QStringLiteral("currentCatalogRevision"),
             currentPanel.value(QStringLiteral("catalogRevision"))},
            {QStringLiteral("metadataDeferred"),
             rejectedPanel.value(QStringLiteral("metadataDeferred"))},
            {QStringLiteral("catalogRowsDeferred"),
             rejectedPanel.value(QStringLiteral("catalogRowsDeferred"))},
            {QStringLiteral("catalogProvisional"),
             rejectedPanel.value(QStringLiteral("catalogProvisional"))},
            {QStringLiteral("entries"),
             rejectedPanel.value(QStringLiteral("entries")).toList().size()},
            {QStringLiteral("totalCount"),
             rejectedPanel.value(QStringLiteral("totalCount"))},
            {QStringLiteral("workspaceTabsType"),
             message.value(QStringLiteral("workspaceTabs")).metaType().id()},
            {QStringLiteral("menusType"),
             message.value(QStringLiteral("menus")).metaType().id()},
        });
}

bool QtShellController::commitPanelCatalogFrame(
    const QVariantMap &message, int side, const QVariantMap &panel,
    const ExtUiProtocol::Envelope &envelope, bool hasSemanticEnvelope,
    FrameApplyTrace *trace)
{
    QElapsedTimer timer;
    if (trace->enabled) {
        timer.start();
    }
    bool activePanelOK = false;
    const int activePanel = message.value(QStringLiteral("activePanel"))
                                .toInt(&activePanelOK);
    if (!activePanelOK) {
        return false;
    }
#if defined(F4_QT_SCENE_TEST_API)
    const QVariantMap presentationPanel = withoutNativePanelPayload(panel);
    const QVariantMap presentationPatch = compactPresentationPatch(
        message, presentationPanel);
    QVariantMap nextScene = m_scene;
    QVariantMap nextPresentationScene = m_presentationScene;
    if (!replaceShellPanel(nextScene, side, panel)
        || !replaceShellPanel(nextPresentationScene, side,
                              presentationPanel)) {
        return false;
    }
    applyPanelCatalogCompactFields(nextScene, message, activePanel);
    applyPanelCatalogCompactFields(nextPresentationScene, message,
                                   activePanel);
    m_scene = std::move(nextScene);
    m_presentationScene = std::move(nextPresentationScene);
#else
    if (!hasSemanticEnvelope) {
        return false;
    }
    applyCompactFieldsToTypedState(message, activePanel,
                                   envelope.revision, true, panel);
#endif
    m_panelCatalogSnapshots[static_cast<size_t>(side)] = panel;
    if (trace->enabled) {
        trace->catalogScenePatchDurationNs = timer.nsecsElapsed();
    }
#if defined(F4_QT_SCENE_TEST_API)
    if (message.contains(QStringLiteral("menus"))) {
        updateCommandMenus(message.value(QStringLiteral("menus")).toList(),
                           true);
    }
#endif

    if (trace->enabled) {
        timer.restart();
    }
    emit compactMessageApplying(message);
    if (trace->enabled) {
        trace->compactApplyingDurationNs = timer.nsecsElapsed();
        timer.restart();
    }
    emit panelCatalogChanged(panel);
    if (trace->enabled) {
        trace->panelCatalogSignalDurationNs = timer.nsecsElapsed();
        timer.restart();
    }
#if defined(F4_QT_SCENE_TEST_API)
    // The test API retains the legacy projected scene so reducer compatibility
    // can be verified in isolation. Production QML consumes typed state stores
    // and panelCatalogChanged directly; emitting this map projection there
    // repeats the same binding work and makes one catalog reset visible as two
    // separate UI transactions.
    emit compactPresentationChanged(presentationPatch);
#endif
    if (trace->enabled) {
        trace->catalogPresentationSignalDurationNs = timer.nsecsElapsed();
    }
    return true;
}

bool QtShellController::applyPanelChromeFrame(
    const QVariantMap &message, const ExtUiProtocol::Envelope &envelope,
    bool hasSemanticEnvelope)
{
    int activePanel = -1;
    if (!validPanelChromeEnvelope(message, &activePanel)) {
        return false;
    }
#if defined(F4_QT_SCENE_TEST_API)
    applyPanelCatalogCompactFields(m_scene, message, activePanel);
    applyPanelCatalogCompactFields(m_presentationScene, message,
                                   activePanel);
    if (message.contains(QStringLiteral("menus"))) {
        updateCommandMenus(message.value(QStringLiteral("menus")).toList(),
                           true);
    }
#else
    if (!hasSemanticEnvelope) {
        return false;
    }
    applyCompactFieldsToTypedState(message, activePanel,
                                   envelope.revision, false);
#endif
#if defined(F4_QT_SCENE_TEST_API)
    emit compactPresentationChanged(compactPresentationPatch(message));
#endif
    return true;
}

bool QtShellController::applyPanelActivationFrame(
    const QVariantMap &message, const ExtUiProtocol::Envelope &envelope,
    bool hasSemanticEnvelope, FrameApplyTrace *trace)
{
    bool sideOK = false;
    bool revisionOK = false;
    const int activePanel = message.value(QStringLiteral("activePanel"))
                                .toInt(&sideOK);
    const qulonglong revision = message.value(QStringLiteral("revision"))
                                    .toULongLong(&revisionOK);
    const bool shellTitleOK = !message.contains(QStringLiteral("shellTitle"))
        || message.value(QStringLiteral("shellTitle")).metaType().id()
            == QMetaType::QString;
    const bool commandLineOK = !message.contains(QStringLiteral("commandLine"))
        || message.value(QStringLiteral("commandLine")).metaType().id()
            == QMetaType::QVariantMap;
    if (!sideOK || activePanel < 0 || activePanel > 1 || !revisionOK
        || !shellTitleOK || !commandLineOK
        || revision <= m_panelActivationRevision) {
        return revisionOK && revision <= m_panelActivationRevision;
    }

    m_panelActivationRevision = revision;
    QVariantMap compactFields;
    if (message.contains(QStringLiteral("shellTitle"))) {
        compactFields.insert(QStringLiteral("shellTitle"),
                             message.value(QStringLiteral("shellTitle")));
    }
    if (message.contains(QStringLiteral("commandLine"))) {
        compactFields.insert(QStringLiteral("commandLine"),
                             message.value(QStringLiteral("commandLine")));
    }
    for (auto it = message.cbegin(); it != message.cend(); ++it) {
        if (it.key().startsWith(QStringLiteral("benchmark"))) {
            compactFields.insert(it.key(), it.value());
        }
    }
#if defined(F4_QT_SCENE_TEST_API)
    m_scene = applyPanelActivationPatch(std::move(m_scene), activePanel);
    m_presentationScene = applyPanelActivationPatch(
        std::move(m_presentationScene), activePanel);
    applyPanelCatalogCompactFields(m_scene, compactFields, activePanel);
    applyPanelCatalogCompactFields(m_presentationScene, compactFields,
                                   activePanel);
#else
    if (!hasSemanticEnvelope) {
        return false;
    }
    for (int side = 0; side < 2; ++side) {
        QVariantMap panel = m_panelCatalogSnapshots[static_cast<size_t>(side)];
        if (panel.isEmpty()) {
            continue;
        }
        panel.insert(QStringLiteral("active"), side == activePanel);
        m_panelCatalogSnapshots[static_cast<size_t>(side)] = panel;
    }
    applyCompactFieldsToTypedState(compactFields, activePanel,
                                   envelope.revision, false);
#endif
    emit compactMessageApplying(message);
    emit panelActivationChanged(activePanel, revision);
    trace->compactProtocolApplied = true;
    return true;
}

bool QtShellController::applyCommandLineFrame(
    const QVariantMap &message, const ExtUiProtocol::Envelope &envelope,
    bool hasSemanticEnvelope)
{
    const QVariant commandLine = message.value(QStringLiteral("commandLine"));
#if defined(F4_QT_SCENE_TEST_API)
    QVariantMap shell = m_scene.value(QStringLiteral("shell")).toMap();
    if (!shell.isEmpty()
        && commandLine.metaType().id() == QMetaType::QVariantMap) {
        const QVariantMap nextCommandLine = commandLine.toMap();
        shell.insert(QStringLiteral("commandLine"), nextCommandLine);
        m_scene.insert(QStringLiteral("shell"), shell);
        QVariantMap presentationShell = m_presentationScene.value(
            QStringLiteral("shell")).toMap();
        if (!presentationShell.isEmpty()) {
            presentationShell.insert(QStringLiteral("commandLine"),
                                     nextCommandLine);
            m_presentationScene.insert(QStringLiteral("shell"),
                                       presentationShell);
        }
    }
    const QVariant menus = message.value(QStringLiteral("menus"));
    if (menus.metaType().id() == QMetaType::QVariantList) {
        const QVariantList nextMenus = menus.toList();
        m_scene.insert(QStringLiteral("menus"), nextMenus);
        m_presentationScene.insert(QStringLiteral("menus"), nextMenus);
        updateCommandMenus(nextMenus, true);
    }
#else
    if (!hasSemanticEnvelope
        || commandLine.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    m_commandLineState->applyFrame(commandLine.toMap(), envelope.revision);
    QVariantMap shell = m_surfaceRegistry->shell();
    shell.insert(QStringLiteral("commandLine"), commandLine);
    m_shellState->applyShell(shell, envelope.revision);
    const QVariant menus = message.value(QStringLiteral("menus"));
    if (menus.isValid()
        && menus.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    if (menus.metaType().id() == QMetaType::QVariantList) {
        QVariantMap state{
            {QStringLiteral("menuBar"), m_overlayState->menuBar()},
            {QStringLiteral("menus"), menus},
        };
        m_overlayState->applyMenuState(state, envelope.revision, true);
    }
#endif
    return true;
}
