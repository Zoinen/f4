#pragma once

#include "QtShellController.h"

#include <QElapsedTimer>
#include <QVariant>

// Per-frame instrumentation is intentionally kept out of QtShellController's
// public header. Reducers update only the stage that they own; the compact
// orchestrator emits the completed trace after all state lanes are committed.
struct QtShellController::FrameApplyTrace
{
    bool enabled = false;
    bool compactProtocolApplied = false;
    QVariant traceId;
    QElapsedTimer applyTimer;
    qint64 applyStartedNs = 0;
    qint64 presentationDurationNs = 0;
    qint64 sceneSignalDurationNs = 0;
    qint64 presentationStartedNs = 0;
    qint64 sceneSignalStartedNs = 0;
    qint64 presentationCompletedNs = 0;
    qint64 sceneSignalCompletedNs = 0;
    qint64 catalogValidationDurationNs = 0;
    qint64 catalogScenePatchDurationNs = 0;
    qint64 compactApplyingDurationNs = 0;
    qint64 panelCatalogSignalDurationNs = 0;
    qint64 catalogPresentationSignalDurationNs = 0;
    qint64 scenePatchCoreDurationNs = 0;
    qint64 scenePatchCompactApplyingDurationNs = 0;
    qint64 scenePatchPanelCatalogDurationNs = 0;
    qint64 scenePatchPanelStateDurationNs = 0;
    qint64 scenePatchCompactPanelDurationNs = 0;
    qint64 scenePatchCompactRootDurationNs = 0;
    qint64 scenePatchPresentationSignalDurationNs = 0;
    qint64 messageSignalStartedNs = 0;
    qint64 messageSignalCompletedNs = 0;
    qint64 messageSignalDurationNs = 0;
    qint64 applyDurationNs = 0;
    bool publicMessageEmitted = false;
};
