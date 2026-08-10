#include "F4GalleryBridge.h"
#include "F4IconProvider.h"
#include "QtShellController.h"

#include <QCommandLineOption>
#include <QCommandLineParser>
#include <QCoreApplication>
#include <QFileInfo>
#include <QFontDatabase>
#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickWindow>
#include <QQuickStyle>
#include <QStringList>
#include <QTimer>
#include <QUrl>

#if defined(__USE_QWK)
#include <QWKQuick/qwkquickglobal.h>
#else
#include "DummyQWK.h"
#endif

int main(int argc, char *argv[])
{
    QGuiApplication app(argc, argv);
    QGuiApplication::setApplicationName(QStringLiteral("f4 Qt Host"));
    QGuiApplication::setOrganizationName(QStringLiteral("f4"));

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
    QObject::connect(&controller, &QtShellController::fatalError, &app, [](const QString &message) {
        qCritical().noquote() << message;
        QCoreApplication::exit(2);
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

#if defined(__USE_QWK) && defined(Q_OS_MACOS)
    // Match ZoinGallery's initial-show workaround for transparent QWK windows.
    // The native NSWindow/title-bar controls can otherwise keep their stale
    // startup layout until the first resize.  Re-applying the geometry over
    // queued event-loop turns makes macOS refresh the title bar immediately.
    if (!engine.rootObjects().isEmpty()) {
        if (auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().constFirst())) {
            const QRect initialGeometry = window->geometry();
            window->setGeometry(QRect(0, 0, 0, 0));
            QTimer::singleShot(0, window, [window, initialGeometry]() {
                window->setGeometry(initialGeometry.adjusted(1, 0, 0, 0));
                window->setGeometry(initialGeometry);
                window->requestUpdate();
            });
        }
    }
#endif

    const int exitCode = app.exec();

    // Tear down QML while the context objects are still alive. Otherwise the
    // engine reevaluates bindings against null qtShell/qtGallery objects while
    // unwinding the stack, which produces misleading shutdown warnings.
    const auto rootObjects = engine.rootObjects();
    for (QObject *rootObject : rootObjects) {
        delete rootObject;
    }

    return exitCode;
}
