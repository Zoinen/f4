#include "ShellStateStore.h"

namespace
{
bool boolWithDefault(const QVariantMap &map, const QString &key,
                     bool defaultValue)
{
    return map.contains(key) ? map.value(key).toBool() : defaultValue;
}
}

ShellStateStore::ShellStateStore(QObject *parent)
    : QObject(parent)
{
}

void ShellStateStore::applyShell(const QVariantMap &shell,
                                 qulonglong revision)
{
    if (revision > m_revision) {
        m_revision = revision;
        emit revisionChanged();
    }

    const QString nextId = shell.value(QStringLiteral("id")).toString();
    if (nextId != m_id) {
        m_id = nextId;
        emit identityChanged();
    }
    const QString nextTitle = shell.value(QStringLiteral("title")).toString();
    if (nextTitle != m_title) {
        m_title = nextTitle;
        emit titleChanged();
    }
    const QString nextMode = shell.value(QStringLiteral("mode")).toString();
    if (nextMode != m_mode) {
        m_mode = nextMode;
        emit modeChanged();
    }

    bool activePanelOk = false;
    const int parsedActivePanel = shell.value(
        QStringLiteral("activePanel")).toInt(&activePanelOk);
    const int nextActivePanel = activePanelOk ? parsedActivePanel : -1;
    if (nextActivePanel != m_activePanel) {
        m_activePanel = nextActivePanel;
        emit activePanelChanged();
    }

    const bool nextShowPanels = boolWithDefault(
        shell, QStringLiteral("showPanels"), true);
    const bool nextShowLeftPanel = boolWithDefault(
        shell, QStringLiteral("showLeftPanel"), true);
    const bool nextShowRightPanel = boolWithDefault(
        shell, QStringLiteral("showRightPanel"), true);
    const bool nextWide = shell.value(QStringLiteral("wide")).toBool();
    bool widePanelOk = false;
    const int parsedWidePanel = shell.value(
        QStringLiteral("widePanel")).toInt(&widePanelOk);
    const int nextWidePanel = widePanelOk ? parsedWidePanel : -1;
    if (nextShowPanels != m_showPanels
        || nextShowLeftPanel != m_showLeftPanel
        || nextShowRightPanel != m_showRightPanel
        || nextWide != m_wide || nextWidePanel != m_widePanel) {
        m_showPanels = nextShowPanels;
        m_showLeftPanel = nextShowLeftPanel;
        m_showRightPanel = nextShowRightPanel;
        m_wide = nextWide;
        m_widePanel = nextWidePanel;
        emit layoutChanged();
    }

    const bool nextTerminalActive = shell.value(
        QStringLiteral("terminalActive")).toBool();
    const bool nextTerminalBusy = shell.value(
        QStringLiteral("terminalBusy")).toBool();
    if (nextTerminalActive != m_terminalActive
        || nextTerminalBusy != m_terminalBusy) {
        m_terminalActive = nextTerminalActive;
        m_terminalBusy = nextTerminalBusy;
        emit terminalChanged();
    }

    const bool nextFallback = shell.value(QStringLiteral("fallback")).toBool();
    const QString nextFallbackReason = shell.value(
        QStringLiteral("reason")).toString();
    if (nextFallback != m_fallback
        || nextFallbackReason != m_fallbackReason) {
        m_fallback = nextFallback;
        m_fallbackReason = nextFallbackReason;
        emit fallbackChanged();
    }

    const QVariantMap nextCommandLine = shell.value(
        QStringLiteral("commandLine")).toMap();
    if (nextCommandLine != m_commandLine) {
        m_commandLine = nextCommandLine;
        emit commandLineChanged();
    }
}

void ShellStateStore::reset()
{
    m_revision = 0;
    m_id.clear();
    m_title.clear();
    m_mode.clear();
    m_activePanel = -1;
    m_showPanels = true;
    m_showLeftPanel = true;
    m_showRightPanel = true;
    m_wide = false;
    m_widePanel = -1;
    m_terminalActive = false;
    m_terminalBusy = false;
    m_fallback = false;
    m_fallbackReason.clear();
    m_commandLine.clear();
    emit revisionChanged();
    emit identityChanged();
    emit titleChanged();
    emit modeChanged();
    emit activePanelChanged();
    emit layoutChanged();
    emit terminalChanged();
    emit fallbackChanged();
    emit commandLineChanged();
}
