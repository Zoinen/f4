#include "QtShellController.h"

#include "ExtUiProtocol.h"
#include "ExtUiSceneReducer.h"
#include "NavigationBenchmarkTrace.h"
#include "QtShellControllerFrameApply.h"

#include <QElapsedTimer>
#include <QMetaType>

using namespace ExtUiSceneReducer;

void QtShellController::applyFrameDecoded(quint64 epoch, quint64 sequence,
                                          const QVariant &decoded)
{
    if (!m_acceptDecodedFrames || epoch != m_decodeEpoch) {
        return;
    }
    if (sequence != m_nextApplySequence) {
        failProtocol(QStringLiteral("Out-of-order IPC decode result"));
        return;
    }
    if (m_queuedFrames.isEmpty()
        || m_queuedFrames.head().sequence != sequence) {
        failProtocol(QStringLiteral("Missing IPC decode queue metadata"));
        return;
    }

    ++m_nextApplySequence;
    const QueuedFrameMetadata frame = m_queuedFrames.dequeue();
    m_queuedPayloadBytes -= frame.payloadBytes;
    m_applyingPayloadBytes = frame.payloadBytes;
    processBuffer();
    if (!m_acceptDecodedFrames
        || decoded.metaType().id() != QMetaType::QVariantMap) {
        return;
    }

    const QVariantMap wireMessage = decoded.toMap();
    QVariantMap message = wireMessage;
    ExtUiProtocol::Envelope envelope;
    bool hasSemanticEnvelope = false;
    if (ExtUiProtocol::isEnvelope(wireMessage)) {
        const ExtUiProtocol::Inspection inspection =
            m_streamRegistry.inspect(wireMessage);
        if (inspection.disposition == ExtUiProtocol::Disposition::Fatal) {
            failProtocol(inspection.error);
            return;
        }
        if (inspection.disposition
            == ExtUiProtocol::Disposition::RequestSnapshot) {
            if (!inspection.snapshotRequest.isEmpty()) {
                sendMessage(inspection.snapshotRequest);
            }
            return;
        }
        if (inspection.disposition
            == ExtUiProtocol::Disposition::IgnoreStale) {
            return;
        }
        envelope = inspection.envelope;
        hasSemanticEnvelope = true;
        message = envelope.payload;
    }
#if !defined(F4_EXTUI_REDUCER_TEST_HARNESS)
    else if (ExtUiProtocol::isSemanticPayloadType(
                 wireMessage.value(QStringLiteral("type")).toString())) {
        failProtocol(QStringLiteral(
            "Semantic payload is missing the ExtUI v4 envelope"));
        return;
    }
#endif

    const QString messageType = message.value(QStringLiteral("type"))
                                    .toString();
    FrameApplyTrace trace;
    trace.enabled = F4NavigationBenchmarkTrace::enabled();
    if (trace.enabled) {
        trace.traceId = F4NavigationBenchmarkTrace::benchmarkTraceId(message);
        trace.applyStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        trace.applyTimer.start();
    }
    if (!dispatchDecodedFrame(messageType, message, envelope,
                              hasSemanticEnvelope, &trace)) {
        return;
    }
    finalizeDecodedFrame(messageType, message, envelope,
                         hasSemanticEnvelope, sequence, frame, &trace);
}

bool QtShellController::dispatchDecodedFrame(
    const QString &messageType, const QVariantMap &message,
    const ExtUiProtocol::Envelope &envelope, bool hasSemanticEnvelope,
    FrameApplyTrace *trace)
{
    if (messageType == QStringLiteral("hello")) {
        return applyHelloFrame(message);
    }
    if (messageType == QStringLiteral("platform_request")
        || messageType == QStringLiteral("platform_cancel")) {
        if (m_platformRequestHandler) {
            m_platformRequestHandler(message);
        }
        return true;
    }
    if (hasSemanticEnvelope && envelope.kind == QStringLiteral("snapshot")
        && messageType.endsWith(QStringLiteral("_snapshot"))) {
        return applyStreamSnapshotFrame(envelope, message);
    }
    if (messageType == QStringLiteral("scene")) {
#if defined(F4_QT_SCENE_TEST_API)
        return applyFullSceneFrame(message, hasSemanticEnvelope, trace);
#else
        failProtocol(QStringLiteral(
            "Full application scenes are not valid in ExtUI v4"));
        return false;
#endif
    }
    if (messageType == QStringLiteral("scene_patch")) {
        return applyScenePatchFrame(message, envelope,
                                    hasSemanticEnvelope, trace);
    }
    if (messageType == QStringLiteral("panel_catalog")) {
        return applyPanelCatalogFrame(message, envelope,
                                      hasSemanticEnvelope, trace);
    } else if (messageType == QStringLiteral("panel_chrome")) {
        return applyPanelChromeFrame(message, envelope,
                                     hasSemanticEnvelope);
    } else if (messageType == QStringLiteral("panel_activation")) {
        return applyPanelActivationFrame(message, envelope,
                                         hasSemanticEnvelope, trace);
    } else if (messageType == QStringLiteral("command_line")) {
        return applyCommandLineFrame(message, envelope,
                                     hasSemanticEnvelope);
    }
    return true;
}

void QtShellController::finalizeDecodedFrame(
    const QString &messageType, const QVariantMap &message,
    const ExtUiProtocol::Envelope &envelope, bool hasSemanticEnvelope,
    quint64 sequence, const QueuedFrameMetadata &frame,
    FrameApplyTrace *trace)
{
#if defined(F4_QT_SCENE_TEST_API)
    synchronizeTypedState(messageType, message, envelope,
                          hasSemanticEnvelope);
#endif
    if (hasSemanticEnvelope) {
        m_streamRegistry.commit(envelope);
    }

    if (messageType == QStringLiteral("panel_catalog_rows")
        || messageType == QStringLiteral("panel_catalog_rows_rejected")) {
        emit panelCatalogRowsReceived(message);
    } else if (messageType == QStringLiteral("panel_catalog_metadata")
               || messageType
                   == QStringLiteral("panel_catalog_metadata_rejected")) {
        emit panelCatalogMetadataReceived(message);
    }
    if (trace->compactProtocolApplied) {
        emit compactMessageApplied(message);
    }

    trace->publicMessageEmitted =
        !ExtUiProtocol::isSemanticPayloadType(messageType);
    QElapsedTimer signalTimer;
    if (trace->enabled) {
        trace->messageSignalStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        signalTimer.start();
    }
    if (trace->publicMessageEmitted) {
#if defined(F4_QT_SCENE_TEST_API)
        emit messageReceived(makePresentationMessage(message,
                                                     m_presentationScene));
#else
        emit messageReceived(makePresentationMessage(message, {}));
#endif
    }
    if (trace->enabled) {
        trace->messageSignalDurationNs = signalTimer.nsecsElapsed();
        trace->messageSignalCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        trace->applyDurationNs = trace->applyTimer.nsecsElapsed();
        traceDecodedFrame(messageType, sequence, frame, *trace);
    }
}
