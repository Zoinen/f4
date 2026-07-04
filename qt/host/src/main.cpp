#include "QtShellController.h"

#include <QCommandLineOption>
#include <QCommandLineParser>
#include <QCoreApplication>
#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QUrl>

int main(int argc, char *argv[])
{
    QGuiApplication app(argc, argv);
    QGuiApplication::setApplicationName(QStringLiteral("f4 Qt Host"));
    QGuiApplication::setOrganizationName(QStringLiteral("f4"));

    QCommandLineParser parser;
    parser.setApplicationDescription(QStringLiteral("Qt/QML sidecar renderer for f4"));
    parser.addHelpOption();
    const QCommandLineOption connectOption(QStringLiteral("f4-qt-connect"), QStringLiteral("Host:port to connect back to."), QStringLiteral("address"));
    const QCommandLineOption nonceOption(QStringLiteral("f4-qt-nonce"), QStringLiteral("One-time handshake nonce."), QStringLiteral("nonce"));
    const QCommandLineOption colsOption(QStringLiteral("f4-qt-cols"), QStringLiteral("Initial grid columns."), QStringLiteral("cols"), QStringLiteral("100"));
    const QCommandLineOption rowsOption(QStringLiteral("f4-qt-rows"), QStringLiteral("Initial grid rows."), QStringLiteral("rows"), QStringLiteral("30"));
    parser.addOptions({connectOption, nonceOption, colsOption, rowsOption});
    parser.process(app);

    const QString connectAddress = parser.value(connectOption);
    const QString nonce = parser.value(nonceOption);
    if (connectAddress.isEmpty() || nonce.isEmpty()) {
        qCritical("f4-qt-host requires --f4-qt-connect and --f4-qt-nonce");
        return 2;
    }

    bool colsOk = false;
    bool rowsOk = false;
    const int cols = parser.value(colsOption).toInt(&colsOk);
    const int rows = parser.value(rowsOption).toInt(&rowsOk);
    if (!colsOk || !rowsOk || cols <= 0 || rows <= 0) {
        qCritical("Invalid initial terminal size");
        return 2;
    }

    QQuickStyle::setStyle(QStringLiteral("Basic"));

    QQmlApplicationEngine engine;
    engine.addImportPath(QStringLiteral(":/"));

    QtShellController controller(connectAddress, nonce, cols, rows, &engine);
    QObject::connect(&controller, &QtShellController::fatalError, &app, [](const QString &message) {
        qCritical().noquote() << message;
        QCoreApplication::exit(2);
    });

    engine.rootContext()->setContextProperty(QStringLiteral("qtShell"), &controller);

    const QUrl url(QStringLiteral("qrc:/F4QtHost/qml/main.qml"));
    QObject::connect(&engine, &QQmlApplicationEngine::objectCreated, &app, [url](QObject *object, const QUrl &objectUrl) {
        if (!object && objectUrl == url) {
            QCoreApplication::exit(3);
        }
    }, Qt::QueuedConnection);

    engine.load(url);
    return app.exec();
}
