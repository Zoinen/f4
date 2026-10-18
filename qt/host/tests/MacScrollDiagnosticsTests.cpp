#include "MacScrollDiagnostics.h"
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QQuickWindow>
#include <QTemporaryDir>
#include <QTest>
#include <QWheelEvent>

class MacScrollDiagnosticsTests : public QObject {
    Q_OBJECT
private slots:
    void disabledWithoutPath() {
        QQuickWindow window;
        QVERIFY(!MacScrollDiagnostics::start(&window, {}));
    }
    void recordsWheelWithoutConsumingIt() {
        QTemporaryDir dir;
        QQuickWindow window;
        const auto path = dir.filePath("scroll.jsonl");
        auto *trace = MacScrollDiagnostics::start(&window, path);
        QVERIFY(trace);
        QWheelEvent wheel(QPointF(10, 10), QPointF(10, 10), QPoint(0, -17),
                          QPoint(0, -120), Qt::NoButton, Qt::NoModifier,
                          Qt::ScrollUpdate, false);
        QCoreApplication::sendEvent(&window, &wheel);
        trace->flush();
        delete trace;
        QFile file(path);
        QVERIFY(file.open(QIODevice::ReadOnly));
        bool found = false;
        while (!file.atEnd()) {
            const auto row = QJsonDocument::fromJson(file.readLine()).object();
            if (row["kind"] == "qtWheel" && row["windowReceiver"].toBool()) {
                QCOMPARE(row["py"].toInt(), -17);
                QCOMPARE(row["phase"].toInt(), int(Qt::ScrollUpdate));
                found = true;
            }
        }
        QVERIFY(found);
        // The window still receives the event after the diagnostic filter.
        QVERIFY(!wheel.isAccepted());
    }
};
QTEST_MAIN(MacScrollDiagnosticsTests)
#include "MacScrollDiagnosticsTests.moc"
