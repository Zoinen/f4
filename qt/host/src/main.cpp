#include "F4GalleryBridge.h"
#include "F4IconProvider.h"
#include "NavigationBenchmarkTrace.h"
#include "QtShellController.h"
#include "WindowGeometryPersistence.h"

#include <QCommandLineOption>
#include <QCommandLineParser>
#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QFontDatabase>
#include <QGuiApplication>
#include <QIcon>
#include <QProcess>
#include <QPointer>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>
#include <QQuickStyle>
#include <QStringList>
#include <QTimer>
#include <QUrl>

#include <memory>

#if defined(Q_OS_WIN)
#include <qt_windows.h>
#endif

#if defined(__USE_QWK)
#include <QWKQuick/qwkquickglobal.h>
#else
#include "DummyQWK.h"
#endif

namespace
{
int launchCoreShortcut(int argc, char *argv[])
{
    // Resolving the executable directory through QCoreApplication keeps this
    // path reliable when the host was found through PATH or started from a
    // different working directory. In particular, do not construct the much
    // heavier QGuiApplication in this short-lived launcher process.
    QCoreApplication launcher(argc, argv);
    QCoreApplication::setApplicationName(QStringLiteral("f4 Qt Host"));
    QCoreApplication::setOrganizationName(QStringLiteral("f4"));

#if defined(Q_OS_WIN)
    const QString coreFileName = QStringLiteral("f4.exe");
#else
    const QString coreFileName = QStringLiteral("f4");
#endif
    QDir applicationDir(QCoreApplication::applicationDirPath());
    QString corePath = applicationDir.filePath(coreFileName);
#if defined(Q_OS_MACOS)
    if (!QFileInfo::exists(corePath)) {
        // Installed macOS trees place f4 beside f4-qt-host.app, not inside
        // Contents/MacOS. Walk from MacOS -> Contents -> .app -> bin.
        QDir bundleParent(applicationDir);
        if (bundleParent.cdUp() && bundleParent.cdUp()
            && bundleParent.cdUp()) {
            const QString siblingCore = bundleParent.filePath(coreFileName);
            if (QFileInfo::exists(siblingCore)) {
                applicationDir = bundleParent;
                corePath = siblingCore;
            }
        }
    }
#endif
    if (!QFileInfo::exists(corePath)) {
        qCritical().noquote()
            << "Unable to find the f4 core next to the Qt host:" << corePath;
        return 2;
    }

    QProcess coreProcess;
    coreProcess.setProgram(corePath);
    coreProcess.setArguments({QStringLiteral("--gui=qt"),
                              QStringLiteral("--attached")});
    coreProcess.setWorkingDirectory(applicationDir.absolutePath());
#if defined(Q_OS_WIN)
    // --attached deliberately prevents f4 from spawning its own hidden
    // replacement process. Preserve the GUI launcher's no-console behaviour
    // while avoiding that second Go process.
    coreProcess.setCreateProcessArgumentsModifier(
        [](QProcess::CreateProcessArguments *arguments) {
            arguments->flags |= CREATE_NO_WINDOW;
        });
#endif
    if (!coreProcess.startDetached()) {
        qCritical().noquote() << "Unable to start the f4 core:" << corePath;
        return 2;
    }
    return 0;
}

void applyMacInitialShowWorkaround(QQuickWindow *window)
{
#if defined(__USE_QWK) && defined(Q_OS_MACOS)
    // Match ZoinGallery's initial-show workaround for transparent QWK windows.
    // The native NSWindow/title-bar controls can otherwise keep their stale
    // startup layout until the first resize. Re-applying the geometry over
    // queued event-loop turns makes macOS refresh the title bar immediately.
    if (window && window->visibility() == QWindow::Windowed) {
        const QRect initialGeometry = window->geometry();
        window->setGeometry(QRect(0, 0, 0, 0));
        QTimer::singleShot(0, window, [window, initialGeometry]() {
            window->setGeometry(initialGeometry.adjusted(1, 0, 0, 0));
            window->setGeometry(initialGeometry);
            window->requestUpdate();
        });
    }
#else
    Q_UNUSED(window);
#endif
}
}

int main(int argc, char *argv[])
{
    // A protocol-free launch is a user-facing shortcut. Start the sibling f4
    // core in Qt mode; the core will then start this executable again with the
    // ExtUI connection arguments below. This branch must precede all GUI/Qt
    // Quick initialization because the launcher exits immediately.
    if (argc == 1) {
        return launchCoreShortcut(argc, argv);
    }

    QGuiApplication app(argc, argv);
    QGuiApplication::setApplicationName(QStringLiteral("f4 Qt Host"));
    QGuiApplication::setOrganizationName(QStringLiteral("f4"));
    const QIcon applicationIcon(
        QStringLiteral(":/F4QtHost/icons/app/f4.svg"));
    if (!applicationIcon.isNull()) {
        QGuiApplication::setWindowIcon(applicationIcon);
    }

    QCommandLineParser parser;
    parser.setApplicationDescription(QStringLiteral("Qt/QML sidecar renderer for f4"));
    parser.addHelpOption();
    const QCommandLineOption connectOption(QStringLiteral("f4-ext-connect"), QStringLiteral("Host:port to connect back to."), QStringLiteral("address"));
    const QCommandLineOption nonceOption(QStringLiteral("f4-ext-nonce"), QStringLiteral("One-time handshake nonce."), QStringLiteral("nonce"));
    const QCommandLineOption colsOption(QStringLiteral("f4-ext-cols"), QStringLiteral("Initial grid columns."), QStringLiteral("cols"), QStringLiteral("100"));
    const QCommandLineOption rowsOption(QStringLiteral("f4-ext-rows"), QStringLiteral("Initial grid rows."), QStringLiteral("rows"), QStringLiteral("30"));
    const QCommandLineOption iconSetOption(QStringLiteral("f4-icon-set"), QStringLiteral("QML icon set: lucide or system."), QStringLiteral("icon-set"), QStringLiteral("lucide"));
    const QCommandLineOption fontFamilyOption(QStringLiteral("f4-font-family"), QStringLiteral("GUI monospace font family or font-file path."), QStringLiteral("family"));
#if defined(Q_OS_MACOS)
    const QString defaultFontSize = QStringLiteral("17");
#else
    const QString defaultFontSize = QStringLiteral("16");
#endif
    const QCommandLineOption fontSizeOption(QStringLiteral("f4-font-size"), QStringLiteral("GUI monospace font pixel size."), QStringLiteral("size"), defaultFontSize);
    const QCommandLineOption windowGeometryFileOption(
        QStringLiteral("f4-window-geometry-file"),
        QStringLiteral("INI file used to persist the main-window geometry."),
        QStringLiteral("path"));
    const QCommandLineOption legacyConnectOption(QStringLiteral("f4-qt-connect"), QStringLiteral("Legacy host:port option."), QStringLiteral("address"));
    const QCommandLineOption legacyNonceOption(QStringLiteral("f4-qt-nonce"), QStringLiteral("Legacy nonce option."), QStringLiteral("nonce"));
    const QCommandLineOption legacyColsOption(QStringLiteral("f4-qt-cols"), QStringLiteral("Legacy initial grid columns."), QStringLiteral("cols"));
    const QCommandLineOption legacyRowsOption(QStringLiteral("f4-qt-rows"), QStringLiteral("Legacy initial grid rows."), QStringLiteral("rows"));
    parser.addOptions({connectOption,
                       nonceOption,
                       colsOption,
                       rowsOption,
                       iconSetOption,
                       fontFamilyOption,
                       fontSizeOption,
                       windowGeometryFileOption,
                       legacyConnectOption,
                       legacyNonceOption,
                       legacyColsOption,
                       legacyRowsOption});
    parser.process(app);

    const auto optionValue = [&parser](const QCommandLineOption &primary, const QCommandLineOption &fallback) {
        const QString primaryValue = parser.value(primary);
        return primaryValue.isEmpty() ? parser.value(fallback) : primaryValue;
    };

    const QString connectAddress = optionValue(connectOption, legacyConnectOption);
    const QString nonce = optionValue(nonceOption, legacyNonceOption);
    if (connectAddress.isEmpty() || nonce.isEmpty()) {
        qCritical("f4-qt-host requires --f4-ext-connect and --f4-ext-nonce");
        return 2;
    }

    // Portable Windows CI validates the complete native/QML graph separately
    // with the test suite. Keep its disconnected smoke probe before QML/window
    // creation: headless Windows can terminate inside the platform plugin
    // before the expected handshake failure is reported.
    if (qEnvironmentVariableIsSet("F4_QT_HOST_STARTUP_SMOKE_ONLY")) {
        return 2;
    }

    bool colsOk = false;
    bool rowsOk = false;
    const int cols = optionValue(colsOption, legacyColsOption).toInt(&colsOk);
    const int rows = optionValue(rowsOption, legacyRowsOption).toInt(&rowsOk);
    if (!colsOk || !rowsOk || cols <= 0 || rows <= 0) {
        qCritical("Invalid initial terminal size");
        return 2;
    }

    bool fontSizeOk = false;
    const int guiFontSize = parser.value(fontSizeOption).toInt(&fontSizeOk);
    if (!fontSizeOk || guiFontSize < 6 || guiFontSize > 72) {
        qCritical("Invalid GUI font size");
        return 2;
    }
    QString guiFontFamily = parser.value(fontFamilyOption).trimmed();
    const QFileInfo fontFile(guiFontFamily);
    if (!guiFontFamily.isEmpty() && fontFile.isFile()) {
        const int fontId = QFontDatabase::addApplicationFont(fontFile.absoluteFilePath());
        const QStringList families = QFontDatabase::applicationFontFamilies(fontId);
        if (families.isEmpty()) {
            qWarning().noquote() << "Unable to load GUI font file:" << fontFile.absoluteFilePath();
        } else {
            guiFontFamily = families.constFirst();
        }
    }

    QQuickStyle::setStyle(QStringLiteral("Basic"));
#if defined(__USE_QWK) && (defined(Q_OS_WIN) || defined(Q_OS_MACOS))
    QQuickWindow::setDefaultAlphaBuffer(true);
#else
    QQuickWindow::setDefaultAlphaBuffer(false);
#endif

    QQmlApplicationEngine engine;
    engine.addImportPath(QStringLiteral(":/"));

    F4IconSet iconSet;
    iconSet.setName(parser.value(iconSetOption));
    engine.addImageProvider(iconSet.providerId(), new F4IconProvider);

#if defined(__USE_QWK)
    qDebug() << "Using QWK";
    QWK::registerTypes(&engine);
#else
    qDebug() << "Using dummy QWK";
    DummyQWK::registerTypes(&engine);
#endif

    QtShellController controller(connectAddress, nonce, cols, rows, &engine);
    F4GalleryBridge galleryBridge(&engine, &engine, &iconSet);
    QObject::connect(&controller, &QtShellController::fatalError, &app, [&app](const QString &message) {
        qCritical().noquote() << message;
        // fatalError may be emitted synchronously by waitForConnected(), before
        // app.exec(). Queue the exit so it is never discarded by Qt.
        QMetaObject::invokeMethod(&app, []() {
            QCoreApplication::exit(2);
        }, Qt::QueuedConnection);
    });
    QObject::connect(&controller, &QtShellController::sceneChanged,
                     &galleryBridge, [&controller, &galleryBridge, &iconSet]() {
        const QVariantMap scene = controller.scene();
        const QString sceneIconSet = scene.value(
            QStringLiteral("qmlIconSet")).toString();
        if (!sceneIconSet.isEmpty()) {
            iconSet.setName(sceneIconSet);
        }
        galleryBridge.synchronizeScene(scene);
    });
    QObject::connect(&galleryBridge, &F4GalleryBridge::uiActionRequested,
                     &controller, &QtShellController::sendUiAction);

    // The f4 core waits for hello before it starts SetupUI. Complete the
    // loopback connection before the synchronous QML load so core setup and
    // QML object construction run concurrently. If an unexpectedly slow
    // connection exceeds this small budget, its existing asynchronous socket
    // signals remain armed and preserve the old startup behaviour.
    controller.waitForInitialHandshake(250);
    if (!controller.startupError().isEmpty()) {
        qCritical().noquote() << controller.startupError();
        return 2;
    }

    engine.rootContext()->setContextProperty(QStringLiteral("qtShell"), &controller);
    engine.rootContext()->setContextProperty(QStringLiteral("qtGallery"), &galleryBridge);
    engine.rootContext()->setContextProperty(QStringLiteral("qtIcons"), &iconSet);
    engine.rootContext()->setContextProperty(QStringLiteral("f4GuiFontFamily"), guiFontFamily);
    engine.rootContext()->setContextProperty(QStringLiteral("f4GuiFontPixelSize"), guiFontSize);
#if defined(__USE_QWK)
    const QString platformName = QGuiApplication::platformName();
    const bool useQwkAtRuntime = platformName != QStringLiteral("offscreen")
        && platformName != QStringLiteral("minimal");
    engine.rootContext()->setContextProperty(QStringLiteral("f4UsesQwk"), useQwkAtRuntime);
#else
    engine.rootContext()->setContextProperty(QStringLiteral("f4UsesQwk"), false);
#endif

    const QUrl url(QStringLiteral("qrc:/F4QtHost/qml/main.qml"));
    QObject::connect(&engine, &QQmlApplicationEngine::objectCreated, &app, [url](QObject *object, const QUrl &objectUrl) {
        if (!object && objectUrl == url) {
            QCoreApplication::exit(3);
        }
    }, Qt::QueuedConnection);

    engine.load(url);

    // QQmlApplicationEngine may process platform/socket events while building
    // the object graph. Do not enter app.exec() if that exposed a startup
    // connection error during the synchronous load.
    if (!controller.startupError().isEmpty()) {
        qCritical().noquote() << controller.startupError();
        return 2;
    }

    QQuickWindow *rootWindow = nullptr;
    std::unique_ptr<WindowGeometryPersistence> windowGeometry;
    QTimer startupShowFallback;
    if (!engine.rootObjects().isEmpty()) {
        rootWindow = qobject_cast<QQuickWindow *>(
            engine.rootObjects().constFirst());
        if (rootWindow) {
            windowGeometry =
                std::make_unique<WindowGeometryPersistence>(
                    rootWindow, parser.value(windowGeometryFileOption));
            // main.qml starts hidden. Restore its screen, normal geometry and
            // intended window state without presenting the empty shell.
            windowGeometry->restoreDeferred();
            QObject::connect(rootWindow, &QQuickWindow::frameSwapped,
                             &galleryBridge,
                             &F4GalleryBridge::notifyFrameSwapped);

            auto shown = std::make_shared<bool>(false);
            const QPointer<QQuickWindow> guardedWindow(rootWindow);
            const QPointer<WindowGeometryPersistence> guardedGeometry(
                windowGeometry.get());
            const QPointer<QTimer> guardedFallback(&startupShowFallback);
            const auto revealWindow =
                [shown, guardedWindow, guardedGeometry,
                 guardedFallback](const QString &reason) {
                if (*shown || !guardedWindow || !guardedGeometry)
                    return;
                *shown = true;
                if (guardedFallback)
                    guardedFallback->stop();
                guardedWindow->requestUpdate();
                guardedGeometry->showRestored();
                applyMacInitialShowWorkaround(guardedWindow);
                if (F4NavigationBenchmarkTrace::enabled()) {
                    F4NavigationBenchmarkTrace::event(
                        QStringLiteral("qt.startup.window.shown"), {}, {
                            {QStringLiteral("reason"), reason},
                            {QStringLiteral("visibility"),
                             int(guardedWindow->visibility())},
                        });
                }
            };
            const auto revealAfterSemanticScene =
                [rootWindow, revealWindow, &controller]() {
                if (!QtShellController::initialSceneReadyForDisplay(
                        controller.presentationScene())) {
                    return;
                }
                // sceneChanged is synchronous. Queue one turn so QML's scene
                // bindings, retained-surface capture and Loader activation
                // finish before the native window becomes visible.
                QTimer::singleShot(0, rootWindow,
                                   [revealWindow]() {
                    revealWindow(QStringLiteral("semantic-scene"));
                });
            };
            QObject::connect(&controller, &QtShellController::sceneChanged,
                             rootWindow, revealAfterSemanticScene);

            startupShowFallback.setSingleShot(true);
            // A failed loopback connection has its own 2 s fatal deadline.
            // Keep the visual fallback later so an unreachable core never
            // flashes an empty shell before the process reports the error.
            startupShowFallback.setInterval(3000);
            QObject::connect(&startupShowFallback, &QTimer::timeout,
                             rootWindow, [revealWindow]() {
                // A protocol/version error must not leave a healthy process
                // permanently invisible. The normal path cancels this by
                // making revealWindow idempotent.
                revealWindow(QStringLiteral("timeout"));
            });
            startupShowFallback.start();

            // A very fast core may have published its first scene during QML
            // construction. Handle that race using the same queued reveal.
            revealAfterSemanticScene();
        }
    }

    const int exitCode = app.exec();

    if (windowGeometry) {
        windowGeometry->save();
        windowGeometry.reset();
    }

    // Tear down QML while the context objects are still alive. Otherwise the
    // engine reevaluates bindings against null qtShell/qtGallery objects while
    // unwinding the stack, which produces misleading shutdown warnings.
    const auto rootObjects = engine.rootObjects();
    for (QObject *rootObject : rootObjects) {
        delete rootObject;
    }

    return exitCode;
}
