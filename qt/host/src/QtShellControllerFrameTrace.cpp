#include "QtShellController.h"

#include "NavigationBenchmarkTrace.h"
#include "QtShellControllerFrameApply.h"

void QtShellController::traceDecodedFrame(
    const QString &messageType, quint64 sequence,
    const QueuedFrameMetadata &frame, const FrameApplyTrace &trace) const
{
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.socket.frame.received"), frame.receivedNs,
        trace.traceId, {
            {QStringLiteral("sequence"), sequence},
            {QStringLiteral("payloadBytes"), frame.payloadBytes},
            {QStringLiteral("receiveDurationNs"), frame.receiveDurationNs},
        });
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.message.apply.begin"), trace.applyStartedNs,
        trace.traceId, {
            {QStringLiteral("sequence"), sequence},
            {QStringLiteral("payloadBytes"), frame.payloadBytes},
            {QStringLiteral("messageType"), messageType},
        });
    traceFullSceneFrame(messageType, sequence, frame, trace);
    traceFrameCompletion(messageType, sequence, frame, trace);
}

void QtShellController::traceFullSceneFrame(
    const QString &messageType, quint64 sequence,
    const QueuedFrameMetadata &frame, const FrameApplyTrace &trace) const
{
    if (messageType != QStringLiteral("scene")) {
        return;
    }
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.scene.presentation.begin"),
        trace.presentationStartedNs, trace.traceId, {
            {QStringLiteral("sequence"), sequence},
            {QStringLiteral("payloadBytes"), frame.payloadBytes},
        });
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.scene.presentation.created"),
        trace.presentationCompletedNs, trace.traceId, {
            {QStringLiteral("sequence"), sequence},
            {QStringLiteral("payloadBytes"), frame.payloadBytes},
            {QStringLiteral("durationNs"), trace.presentationDurationNs},
        });
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.scene.sceneChanged.begin"),
        trace.sceneSignalStartedNs, trace.traceId, {
            {QStringLiteral("sequence"), sequence},
            {QStringLiteral("payloadBytes"), frame.payloadBytes},
        });
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.scene.sceneChanged.emitted"),
        trace.sceneSignalCompletedNs, trace.traceId, {
            {QStringLiteral("sequence"), sequence},
            {QStringLiteral("payloadBytes"), frame.payloadBytes},
            {QStringLiteral("durationNs"), trace.sceneSignalDurationNs},
        });
}

void QtShellController::traceFrameCompletion(
    const QString &messageType, quint64 sequence,
    const QueuedFrameMetadata &frame, const FrameApplyTrace &trace) const
{
    const QVariantMap identity{
        {QStringLiteral("sequence"), sequence},
        {QStringLiteral("payloadBytes"), frame.payloadBytes},
        {QStringLiteral("messageType"), messageType},
    };
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.message.messageReceived.begin"),
        trace.messageSignalStartedNs, trace.traceId, identity);
    QVariantMap emitted = identity;
    emitted.insert(QStringLiteral("durationNs"),
                   trace.messageSignalDurationNs);
    emitted.insert(QStringLiteral("emitted"), trace.publicMessageEmitted);
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.message.messageReceived.emitted"),
        trace.messageSignalCompletedNs, trace.traceId, emitted);

    QVariantMap metrics = identity;
    metrics.insert(QStringLiteral("presentationDurationNs"),
                   trace.presentationDurationNs);
    metrics.insert(QStringLiteral("sceneChangedDurationNs"),
                   trace.sceneSignalDurationNs);
    metrics.insert(QStringLiteral("messageReceivedDurationNs"),
                   trace.messageSignalDurationNs);
    metrics.insert(QStringLiteral("catalogValidationDurationNs"),
                   trace.catalogValidationDurationNs);
    metrics.insert(QStringLiteral("catalogScenePatchDurationNs"),
                   trace.catalogScenePatchDurationNs);
    metrics.insert(QStringLiteral("compactApplyingDurationNs"),
                   trace.compactApplyingDurationNs);
    metrics.insert(QStringLiteral("panelCatalogSignalDurationNs"),
                   trace.panelCatalogSignalDurationNs);
    metrics.insert(QStringLiteral("catalogPresentationSignalDurationNs"),
                   trace.catalogPresentationSignalDurationNs);
    metrics.insert(QStringLiteral("scenePatchCoreDurationNs"),
                   trace.scenePatchCoreDurationNs);
    metrics.insert(QStringLiteral("scenePatchCompactApplyingDurationNs"),
                   trace.scenePatchCompactApplyingDurationNs);
    metrics.insert(QStringLiteral("scenePatchPanelCatalogDurationNs"),
                   trace.scenePatchPanelCatalogDurationNs);
    metrics.insert(QStringLiteral("scenePatchPanelStateDurationNs"),
                   trace.scenePatchPanelStateDurationNs);
    metrics.insert(QStringLiteral("scenePatchCompactPanelDurationNs"),
                   trace.scenePatchCompactPanelDurationNs);
    metrics.insert(QStringLiteral("scenePatchCompactRootDurationNs"),
                   trace.scenePatchCompactRootDurationNs);
    metrics.insert(QStringLiteral("scenePatchPresentationSignalDurationNs"),
                   trace.scenePatchPresentationSignalDurationNs);
    metrics.insert(QStringLiteral("durationNs"), trace.applyDurationNs);
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.message.apply.end"),
        trace.messageSignalCompletedNs, trace.traceId, metrics);
}
