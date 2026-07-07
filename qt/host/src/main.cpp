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
    const QCommandLineOption connectOption(QStringLiteral("f4-ext-connect"), QStringLiteral("Host:port to connect back to."), QStringLiteral("address"));
    const QCommandLineOption nonceOption(QStringLiteral("f4-ext-nonce"), QStringLiteral("One-time handshake nonce."), QStringLiteral("nonce"));
    const QCommandLineOption colsOption(QStringLiteral("f4-ext-cols"), QStringLiteral("Initial grid columns."), QStringLiteral("cols"), QStringLiteral("100"));
    const QCommandLineOption rowsOption(QStringLiteral("f4-ext-rows"), QStringLiteral("Initial grid rows."), QStringLiteral("rows"), QStringLiteral("30"));
    const QCommandLineOption legacyConnectOption(QStringLiteral("f4-qt-connect"), QStringLiteral("Legacy host:port option."), QStringLiteral("address"));
    const QCommandLineOption legacyNonceOption(QStringLiteral("f4-qt-nonce"), QStringLiteral("Legacy nonce option."), QStringLiteral("nonce"));
    const QCommandLineOption legacyColsOption(QStringLiteral("f4-qt-cols"), QStringLiteral("Legacy initial grid columns."), QStringLiteral("cols"));
    const QCommandLineOption legacyRowsOption(QStringLiteral("f4-qt-rows"), QStringLiteral("Legacy initial grid rows."), QStringLiteral("rows"));
    parser.addOptions({connectOption,
                       nonceOption,
                       colsOption,
                       rowsOption,
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
