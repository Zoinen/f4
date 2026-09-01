#include "TestExtUiStateController.h"

namespace
{
QVariantMap rootProjection(const QVariantMap &scene,
                           std::initializer_list<const char *> keys)
{
    QVariantMap result;
    for (const char *key : keys) {
        const QString name = QString::fromLatin1(key);
        if (scene.contains(name)) {
            result.insert(name, scene.value(name));
        }
    }
    return result;
}
}

TestExtUiStateController::TestExtUiStateController(QObject *parent)
    : QObject(parent)
    , m_shellState(new ShellStateStore(this))
    , m_chromeState(new ChromeStateStore(this))
    , m_workspaceState(new WorkspaceStateStore(this))
    , m_overlayState(new OverlayStateStore(this))
    , m_commandLineState(new CommandLineStateStore(this))
    , m_surfaceRegistry(new SurfaceRegistry(this))
{
}

void TestExtUiStateController::applyScene(const QVariantMap &scene)
{
    ++m_revision;
    m_chromeState->applyState(rootProjection(
        scene, {"schema", "version", "width", "height", "presentation",
                "qmlIconSet", "keyBar", "toast"}), m_revision);
    m_workspaceState->applyState(rootProjection(
        scene, {"activeScreen", "workspaceCount", "workspaceTabs"}),
        m_revision);
    m_overlayState->applyMenuState(rootProjection(
        scene, {"menuBar", "menus"}), m_revision, false);
    m_overlayState->applyDialogsState(rootProjection(
        scene, {"dialogs"}), m_revision);

    const QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    m_shellState->applyShell(shell, m_revision);
    m_commandLineState->applyFrame(shell.value(
        QStringLiteral("commandLine")).toMap(), m_revision);
    m_surfaceRegistry->applyShell(shell, m_revision);
    m_surfaceRegistry->applyDocument(scene.value(
        QStringLiteral("surface")).toMap(), m_revision);
    m_surfaceRegistry->applyOperationsQueue(scene.value(
        QStringLiteral("operationsQueue")).toMap(), m_revision);
    emit sceneChanged();
}

void TestExtUiStateController::applyCommandLine(
    const QVariantMap &commandLine)
{
    m_commandLineState->applyFrame(commandLine, ++m_revision);
}

void TestExtUiStateController::applyCommandMenus(
    const QVariantList &menus, bool allowStateOnlyUpdate)
{
    m_overlayState->applyMenuState(QVariantMap{
        {QStringLiteral("menuBar"), m_overlayState->menuBar()},
        {QStringLiteral("menus"), menus},
    }, ++m_revision, allowStateOnlyUpdate);
}
