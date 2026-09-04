#include "QtShellController.h"

#include "ExtUiSceneReducer.h"
#include "NavigationBenchmarkTrace.h"
#include "QtShellControllerFrameApply.h"

#include <QElapsedTimer>
#include <QMetaType>
#include <QSet>

#include <utility>

using namespace ExtUiSceneReducer;

namespace
{
QVariantMap compactRootPatch(
    const QVariantMap &scene, const QVariantMap &presentationScene,
    const AppliedScenePatch &applied)
{
    QVariantMap patch{{QStringLiteral("type"),
                       QStringLiteral("scene_patch")}};
#if defined(F4_QT_SCENE_TEST_API)
    // Production root state is already published by its typed store. Only
    // the legacy reducer oracle needs this second map projection.
    for (const QString &key : {QStringLiteral("workspaceTabs"),
                               QStringLiteral("menuBar"),
                               QStringLiteral("keyBar"),
                               QStringLiteral("toast")}) {
        if (!applied.rootKeys.contains(key)) {
            continue;
        }
        const QVariant value = scene.value(key);
        patch.insert(key, value.metaType().id() == QMetaType::QVariantMap
                              ? value : QVariant(QVariantMap{}));
    }
#endif
    if (applied.shellKeys.contains(QStringLiteral("activePanel"))) {
        patch.insert(QStringLiteral("activePanel"),
                     scene.value(QStringLiteral("shell")).toMap().value(
                         QStringLiteral("activePanel")));
    }
#if defined(F4_QT_SCENE_TEST_API)
    for (const QString &key : {QStringLiteral("shell"),
                               QStringLiteral("surface")}) {
        if (!applied.rootKeys.contains(key)) {
            continue;
        }
        const bool present = presentationScene.contains(key)
            && presentationScene.value(key).metaType().id()
                == QMetaType::QVariantMap;
        patch.insert(key + QStringLiteral("Present"), present);
        patch.insert(key, present ? presentationScene.value(key)
                                  : QVariant(QVariantMap{}));
    }
#else
    Q_UNUSED(presentationScene);
#endif
    return patch;
}

void addShellReplacement(QVariantMap *patch,
                         const QVariantMap &presentationScene,
                         const AppliedScenePatch &applied)
{
    if (!patch || !applied.rootKeys.contains(QStringLiteral("shell"))) {
        return;
    }
    const QVariantMap shell = presentationScene.value(
        QStringLiteral("shell")).toMap();
    patch->insert(QStringLiteral("replaceShell"), true);
    patch->insert(QStringLiteral("activePanel"),
                  shell.value(QStringLiteral("activePanel")));
}

void addSurfaceState(QVariantMap *patch,
                     const QVariantMap &presentationScene,
                     const AppliedScenePatch &applied)
{
    if (!patch || applied.surfaceKeys.isEmpty()) {
        return;
    }
    const QVariantMap surface = presentationScene.value(
        QStringLiteral("surface")).toMap();
    QVariantMap state{{QStringLiteral("id"),
                       surface.value(QStringLiteral("id"))}};
    for (const QString &key : {QStringLiteral("cursorLine"),
                               QStringLiteral("cursorPos"),
                               QStringLiteral("cursorVisualRow"),
                               QStringLiteral("cursorVisualColumn"),
                               QStringLiteral("cursorVisible"),
                               QStringLiteral("cursorShape"),
                               QStringLiteral("cursorAbsoluteRow"),
                               QStringLiteral("topBarRight")}) {
        state.insert(key, surface.value(key));
    }
    patch->insert(QStringLiteral("surfaceState"), state);
}

bool needsPresentationSignal(const QVariantMap &scene,
                             const AppliedScenePatch &applied)
{
    QSet<QString> rootKeys = applied.rootKeys;
    for (const QString &key : {QStringLiteral("menus"),
                               QStringLiteral("workspaceTabs"),
                               QStringLiteral("menuBar"),
                               QStringLiteral("keyBar"),
                               QStringLiteral("toast"),
                               QStringLiteral("width"),
                               QStringLiteral("height"),
                               QStringLiteral("activeScreen"),
                               QStringLiteral("workspaceCount"),
                               QStringLiteral("qmlIconSet"),
                               QStringLiteral("shell"),
                               QStringLiteral("surface")}) {
        rootKeys.remove(key);
    }

    QSet<QString> shellKeys = applied.shellKeys;
    shellKeys.remove(QStringLiteral("commandLine"));
    shellKeys.remove(QStringLiteral("activePanel"));
    shellKeys.remove(QStringLiteral("title"));
    if (!scene.value(QStringLiteral("shell")).toMap().value(
            QStringLiteral("terminalActive")).toBool()) {
        shellKeys.remove(QStringLiteral("terminal"));
    }
    return !rootKeys.isEmpty() || !shellKeys.isEmpty();
}
}

bool QtShellController::applyHelloFrame(const QVariantMap &message)
{
    bool protocolOK = false;
    const int protocol = message.value(QStringLiteral("protocol"))
                             .toInt(&protocolOK);
    if (!protocolOK || protocol != ExtUiProtocol::Version
        || message.value(QStringLiteral("nonce")).toString() != m_nonce) {
        failProtocol(QStringLiteral("Incompatible f4 Qt protocol handshake"));
        return false;
    }
    m_initialHandshakeComplete = true;
    m_serverHandshakeComplete = true;

    QVariantMap advertisement;
    const int mediaProtocol = message.value(
        QStringLiteral("mediaProtocol")).toInt();
    const QString mediaEndpoint = message.value(
        QStringLiteral("mediaEndpoint")).toString();
    const QString mediaNonce = message.value(
        QStringLiteral("mediaNonce")).toString();
    if (mediaProtocol > 0 && !mediaEndpoint.isEmpty()
        && !mediaNonce.isEmpty()) {
        advertisement.insert(QStringLiteral("protocol"), mediaProtocol);
        advertisement.insert(QStringLiteral("endpoint"), mediaEndpoint);
        advertisement.insert(QStringLiteral("nonce"), mediaNonce);
        advertisement.insert(QStringLiteral("maxChunkSize"),
                             message.value(QStringLiteral("mediaMaxChunkSize")));
    }
    if (advertisement != m_mediaAdvertisement) {
        m_mediaAdvertisement = advertisement;
        if (m_mediaAdvertisementHandler) {
            m_mediaAdvertisementHandler(m_mediaAdvertisement);
        }
    }
    return true;
}

bool QtShellController::applyStreamSnapshotFrame(
    const ExtUiProtocol::Envelope &envelope, const QVariantMap &message)
{
    QVariantMap catalogPanel;
    QString error;
#if defined(F4_QT_SCENE_TEST_API)
    if (!applyStreamSnapshotPayload(envelope.streamId, message, &m_scene,
                                    &catalogPanel, &error)) {
        failProtocol(error);
        return false;
    }
    m_presentationScene = makePresentationScene(m_scene);
    updateCommandMenus(m_scene.value(QStringLiteral("menus")).toList(),
                       false);
    if (!catalogPanel.isEmpty()) {
        const int side = catalogPanel.value(QStringLiteral("side")).toInt();
        if (side >= 0 && side < 2) {
            m_panelCatalogSnapshots[static_cast<size_t>(side)] = catalogPanel;
        }
        emit panelCatalogChanged(catalogPanel);
    }
    emit sceneChanged();
    emit presentationSceneChanged();
#else
    QVariantMap isolatedScene{
        {QStringLiteral("schema"), QStringLiteral("app")},
    };
    if (!applyStreamSnapshotPayload(envelope.streamId, message,
                                    &isolatedScene, &catalogPanel, &error)) {
        failProtocol(error);
        return false;
    }
    const QVariantMap state = message.value(QStringLiteral("state")).toMap();
    const qulonglong revision = envelope.revision;
    if (envelope.streamId == QStringLiteral("chrome")) {
        m_chromeState->applyState(state, revision);
    } else if (envelope.streamId == QStringLiteral("workspaces")) {
        m_workspaceState->applyState(state, revision);
    } else if (envelope.streamId == QStringLiteral("menus")) {
        m_overlayState->applyMenuState(state, revision, false);
    } else if (envelope.streamId == QStringLiteral("dialogs")) {
        m_overlayState->applyDialogsState(state, revision);
    } else if (envelope.streamId == QStringLiteral("operations")) {
        m_surfaceRegistry->applyOperationsQueue(
            state.value(QStringLiteral("operationsQueue")).toMap(),
            revision);
    } else if (envelope.streamId == QStringLiteral("command-line")) {
        m_commandLineState->applyFrame(
            state.value(QStringLiteral("commandLine")).toMap(), revision);
    } else if (envelope.streamId == QStringLiteral("shell")) {
        QVariantMap shell = state.value(QStringLiteral("shell")).toMap();
        shell.insert(QStringLiteral("commandLine"),
                     m_commandLineState->frame());
        m_shellState->applyShell(shell, revision);
        m_surfaceRegistry->applyShell(shell, revision);
    } else if (envelope.streamId.startsWith(
                   QStringLiteral("document/"))) {
        m_surfaceRegistry->applyDocument(
            state.value(QStringLiteral("surface")).toMap(), revision);
    }
    if (!catalogPanel.isEmpty()) {
        const int side = catalogPanel.value(QStringLiteral("side")).toInt();
        if (side >= 0 && side < 2) {
            m_panelCatalogSnapshots[static_cast<size_t>(side)] = catalogPanel;
        }
        emit panelCatalogChanged(catalogPanel);
        // Keep the QML panel descriptor in lockstep with the native catalog
        // model. The snapshot itself may contain thousands of rows, so only
        // send its bounded row-free descriptor through the presentation lane.
        emit compactPresentationChanged(QVariantMap{
            {QStringLiteral("type"), message.value(QStringLiteral("type"))},
            {QStringLiteral("side"), side},
            {QStringLiteral("panel"),
             withoutNativePanelPayload(catalogPanel)},
        });
    }
#endif
    return true;
}

bool QtShellController::applyFullSceneFrame(const QVariantMap &message,
                                            bool hasSemanticEnvelope,
                                            FrameApplyTrace *trace)
{
    if (message.value(QStringLiteral("schema")).toString()
        == QStringLiteral("app")) {
        qulonglong version = 0;
        qulonglong revision = 0;
        if (!nonNegativeInteger(message.value(QStringLiteral("version")),
                                &version)
            || version != 4
            || !nonNegativeInteger(message.value(QStringLiteral("revision")),
                                   &revision)
            || revision == 0
            || (!hasSemanticEnvelope && m_sceneRevision > 0
                && revision != m_sceneRevision + 1)) {
            failProtocol(QStringLiteral("Invalid or out-of-order full app scene"));
            return false;
        }
        m_sceneRevision = hasSemanticEnvelope ? m_sceneRevision + 1
                                              : revision;
    } else if (m_sceneRevision > 0) {
        failProtocol(QStringLiteral("Legacy scene cannot replace an app scene"));
        return false;
    }

    m_scene = message;
    QElapsedTimer stageTimer;
    if (trace->enabled) {
        trace->presentationStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        stageTimer.start();
    }
    m_presentationScene = makePresentationScene(message);
    if (trace->enabled) {
        trace->presentationDurationNs = stageTimer.nsecsElapsed();
        trace->presentationCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
    }
    updateCommandMenus(m_scene.value(QStringLiteral("menus")).toList(),
                       false);
    if (trace->enabled) {
        trace->sceneSignalStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        stageTimer.restart();
    }
#if defined(F4_QT_SCENE_TEST_API)
    emit sceneChanged();
    emit presentationSceneChanged();
#endif
    if (trace->enabled) {
        trace->sceneSignalDurationNs = stageTimer.nsecsElapsed();
        trace->sceneSignalCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
    }
    return true;
}

bool QtShellController::applyScenePatchFrame(const QVariantMap &message,
                                             const ExtUiProtocol::Envelope &envelope,
                                             bool hasSemanticEnvelope,
                                             FrameApplyTrace *trace)
{
    QElapsedTimer stageTimer;
    if (trace->enabled) {
        stageTimer.start();
    }
    QVariantMap reducerMessage = message;
    AppliedScenePatch applied;
    QString error;
#if defined(F4_QT_SCENE_TEST_API)
    if (hasSemanticEnvelope) {
        reducerMessage.insert(QStringLiteral("baseRevision"), m_sceneRevision);
        reducerMessage.insert(QStringLiteral("revision"), m_sceneRevision + 1);
    }
    if (!applyScenePatch(reducerMessage, m_scene, m_presentationScene,
                         m_sceneRevision, &applied, &error)) {
        failProtocol(error.isEmpty() ? QStringLiteral("Invalid app scene patch")
                                     : error);
        return false;
    }

    // The test-only reducer oracle keeps the complete result, while signal
    // projection below still reads the same applied transaction.
    m_scene = applied.scene;
    m_presentationScene = applied.presentationScene;
    m_sceneRevision = applied.revision;
    updateCommandMenus(m_scene.value(QStringLiteral("menus")).toList(), true);
#else
    if (!hasSemanticEnvelope || !envelope.hasBaseRevision) {
        failProtocol(QStringLiteral(
            "ExtUI scene patch has no stream-local base revision"));
        return false;
    }
    reducerMessage.insert(QStringLiteral("baseRevision"),
                          envelope.baseRevision);
    reducerMessage.insert(QStringLiteral("revision"), envelope.revision);
    const QVariantMap reducerScene = streamReducerScene(envelope.streamId);
    if (!applyScenePatch(reducerMessage, reducerScene,
                         makePresentationScene(reducerScene),
                         envelope.baseRevision, &applied, &error)) {
        failProtocol(error.isEmpty() ? QStringLiteral("Invalid app scene patch")
                                     : error);
        return false;
    }
    commitTypedScenePatch(envelope.streamId, applied);
#endif
    if (trace->enabled) {
        trace->scenePatchCoreDurationNs = stageTimer.nsecsElapsed();
    }

    emitScenePatchSignals(message, applied, trace);
    emitScenePatchRootSignals(applied, trace);
    trace->compactProtocolApplied = true;
    return true;
}

void QtShellController::emitScenePatchSignals(
    const QVariantMap &message, const AppliedScenePatch &applied,
    FrameApplyTrace *trace)
{
    QElapsedTimer timer;
    if (trace->enabled) {
        timer.start();
    }
    emit compactMessageApplying(message);
    if (trace->enabled) {
        trace->scenePatchCompactApplyingDurationNs = timer.nsecsElapsed();
        timer.restart();
    }
    for (const QVariantMap &panel : applied.catalogPanels) {
        const int side = panel.value(QStringLiteral("side")).toInt();
        if (side >= 0 && side < 2) {
            m_panelCatalogSnapshots[static_cast<size_t>(side)] = panel;
        }
        emit panelCatalogChanged(panel);
    }
    for (const QVariantMap &append : applied.catalogAppends) {
        emit panelCatalogAppendChanged(append);
    }
    if (trace->enabled) {
        trace->scenePatchPanelCatalogDurationNs = timer.nsecsElapsed();
        timer.restart();
    }
    for (const QVariantMap &panelPatch : applied.panelPatches) {
        emit panelStateChanged(panelPatch);
    }
    if (trace->enabled) {
        trace->scenePatchPanelStateDurationNs = timer.nsecsElapsed();
        timer.restart();
    }
    for (const QVariantMap &patch : applied.compactPatches) {
        emit compactPresentationChanged(patch);
    }
    if (trace->enabled) {
        trace->scenePatchCompactPanelDurationNs = timer.nsecsElapsed();
    }
}

void QtShellController::emitScenePatchRootSignals(
    const AppliedScenePatch &applied, FrameApplyTrace *trace)
{
    QVariantMap rootPatch = compactRootPatch(applied.scene,
                                             applied.presentationScene,
                                             applied);
    addShellReplacement(&rootPatch, applied.presentationScene, applied);
    addSurfaceState(&rootPatch, applied.presentationScene, applied);
    QElapsedTimer timer;
    if (rootPatch.size() > 1) {
        if (trace->enabled) {
            timer.start();
        }
        emit compactPresentationChanged(rootPatch);
        if (trace->enabled) {
            trace->scenePatchCompactRootDurationNs = timer.nsecsElapsed();
        }
    }
    if (!needsPresentationSignal(applied.scene, applied)) {
        return;
    }
    if (trace->enabled) {
        timer.restart();
    }
#if defined(F4_QT_SCENE_TEST_API)
    emit presentationSceneChanged();
#endif
    if (trace->enabled) {
        trace->scenePatchPresentationSignalDurationNs = timer.nsecsElapsed();
    }
}
