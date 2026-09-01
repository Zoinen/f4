#pragma once

#include "ExtUiStateStores.h"
#include "ShellStateStore.h"

#include <QObject>
#include <QVariantMap>

// Shared typed-state test double for QML surface tests. Production QML must
// exercise the same stream-local contract in tests; exposing a synthetic
// presentationScene here would silently reintroduce the removed dependency.
class TestExtUiStateController : public QObject
{
    Q_OBJECT
    Q_PROPERTY(ShellStateStore* shellState READ shellState CONSTANT)
    Q_PROPERTY(ChromeStateStore* chromeState READ chromeState CONSTANT)
    Q_PROPERTY(WorkspaceStateStore* workspaceState READ workspaceState CONSTANT)
    Q_PROPERTY(OverlayStateStore* overlayState READ overlayState CONSTANT)
    Q_PROPERTY(CommandLineStateStore* commandLineState READ commandLineState CONSTANT)
    Q_PROPERTY(SurfaceRegistry* surfaceRegistry READ surfaceRegistry CONSTANT)

public:
    explicit TestExtUiStateController(QObject *parent = nullptr);

    ShellStateStore *shellState() const { return m_shellState; }
    ChromeStateStore *chromeState() const { return m_chromeState; }
    WorkspaceStateStore *workspaceState() const { return m_workspaceState; }
    OverlayStateStore *overlayState() const { return m_overlayState; }
    CommandLineStateStore *commandLineState() const
    {
        return m_commandLineState;
    }
    SurfaceRegistry *surfaceRegistry() const { return m_surfaceRegistry; }

    void applyScene(const QVariantMap &scene);
    void applyCommandLine(const QVariantMap &commandLine);
    void applyCommandMenus(const QVariantList &menus,
                           bool allowStateOnlyUpdate = false);

signals:
    void sceneChanged();
    void panelActivationChanged(int activePanel, qulonglong revision);
    void compactPresentationChanged(const QVariantMap &patch);
    void commandMenuStatesChanged(const QVariantList &states);
    void messageReceived(const QVariantMap &message);

private:
    qulonglong m_revision = 0;
    ShellStateStore *m_shellState = nullptr;
    ChromeStateStore *m_chromeState = nullptr;
    WorkspaceStateStore *m_workspaceState = nullptr;
    OverlayStateStore *m_overlayState = nullptr;
    CommandLineStateStore *m_commandLineState = nullptr;
    SurfaceRegistry *m_surfaceRegistry = nullptr;
};
