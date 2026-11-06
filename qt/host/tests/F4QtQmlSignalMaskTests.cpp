#include "QmlSignalSource.h"
#include <QCoreApplication>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQmlEngine>
#include <QtQmlMeta/private/qqmlconnections_p.h>
#include <QtQml/private/qqmldata_p.h>
#include <QtCore/private/qmetaobject_p.h>
#include <QtCore/private/qobject_p.h>
#include <QScopedValueRollback>
#include <QtTest>
#include <memory>
#include <thread>
#include <cctype>

class CountingApplication final : public QCoreApplication {
public:
    using QCoreApplication::QCoreApplication;
    int proxyCalls = 0;
    const QObject *observedSource = nullptr;
    bool notify(QObject *receiver, QEvent *event) override {
        if (event->type() == QEvent::MetaCall
            && receiver->metaObject() == &QObject::staticMetaObject
            && observedSource
            && static_cast<QAbstractMetaCallEvent *>(event)->sender() == observedSource)
            ++proxyCalls;
        return QCoreApplication::notify(receiver, event);
    }
};
static void drain() {
    QCoreApplication::sendPostedEvents(nullptr, QEvent::MetaCall);
    QCoreApplication::processEvents();
    QCoreApplication::sendPostedEvents(nullptr, QEvent::DeferredDelete);
}
static QByteArray scene() {
    return R"(import SignalTest 1.0
Object {
    id: root
    property int received: 0
    property int lastValue: -1
    property int boundValue: signalSource.value
    property Connections relay: Connections {
        target: signalSource
        function onObserved(value) { root.received += 1; root.lastValue = value }
    }
})";
}
class F4QtQmlSignalMaskTests final : public QObject {
    Q_OBJECT
private slots:
    void unobservedSignalAvoidsQmlQueueAndPreservesCppReceiver() {
        QQmlEngine engine;
        QmlSignalSource source;
        engine.rootContext()->setContextProperty("signalSource", &source);
        QQmlComponent component(&engine);component.setData(scene(), QUrl());
        std::unique_ptr<QObject> root(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        drain();
        auto *data = QQmlData::get(&source, false);
        QVERIFY(data);
        QVERIFY(data->signalHasEndpoint(QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::observed))));
        QVERIFY(!data->signalHasEndpoint(QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::unobserved))));
        int cppCalls = 0;
        connect(&source, &QmlSignalSource::unobserved, root.get(), [&] { ++cppCalls; }, Qt::QueuedConnection);
        auto *app = static_cast<CountingApplication *>(QCoreApplication::instance());
        QScopedValueRollback<const QObject *> activeSource(app->observedSource, &source);
        const int before = app->proxyCalls;
        std::thread worker([&] { for (int i = 0; i < 100; ++i) emit source.unobserved(); });
        worker.join();drain();
        QCOMPARE(cppCalls, 100);
        QCOMPARE(root->property("received").toInt(), 0);
        QCOMPARE(app->proxyCalls - before, 0);
    }
    void connectedSignalsStillCopyArgumentsAndUpdateBindings() {
        QQmlEngine engine;
        QmlSignalSource source;
        engine.rootContext()->setContextProperty("signalSource", &source);
        QQmlComponent component(&engine);component.setData(scene(), QUrl());
        std::unique_ptr<QObject> root(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        std::thread worker([&] { emit source.observed(42); source.setValue(23); });
        worker.join();drain();
        QCOMPARE(root->property("received").toInt(), 1);
        QCOMPARE(root->property("lastValue").toInt(), 42);
        QCOMPARE(root->property("boundValue").toInt(), 23);
    }
    void mask64CollisionAvoidsUnobservedProxy() {
        QQmlEngine engine;
        QmlSignalSource source;
        engine.rootContext()->setContextProperty("signalSource", &source);
        QQmlComponent component(&engine);component.setData(scene(), QUrl());
        std::unique_ptr<QObject> root(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        drain();
        const int observed = QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::observed));
        const int collision = QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::collision));
        QCOMPARE(collision - observed, 64);
        QVERIFY(QQmlData::get(&source, false)->signalHasEndpoint(collision));
        auto *app = static_cast<CountingApplication *>(QCoreApplication::instance());
        QScopedValueRollback<const QObject *> activeSource(app->observedSource, &source);
        const int before = app->proxyCalls;
        std::thread worker([&] { emit source.collision(); emit source.observed(17); });
        worker.join();drain();
        QCOMPARE(app->proxyCalls - before, 1);
        QCOMPARE(root->property("received").toInt(), 1);
        QCOMPARE(root->property("lastValue").toInt(), 17);
    }
    void mask512CollisionRemainsConservative() {
        QQmlEngine engine;
        QmlSignalSource source;
        engine.rootContext()->setContextProperty("signalSource", &source);
        QQmlComponent component(&engine);component.setData(scene(), QUrl());
        std::unique_ptr<QObject> root(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        drain();
        const int observed = QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::observed));
        const int far = QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::farCollision));
        QCOMPARE(far - observed, 512);
        auto *app = static_cast<CountingApplication *>(QCoreApplication::instance());
        QScopedValueRollback<const QObject *> activeSource(app->observedSource, &source);
        const int before = app->proxyCalls;
        std::thread worker([&] { emit source.farCollision(); emit source.observed(19); });
        worker.join();drain();
        QCOMPARE(app->proxyCalls - before, 2);
        QCOMPARE(root->property("received").toInt(), 1);
        QCOMPARE(root->property("lastValue").toInt(), 19);
    }
    void allSignalIndicesRemainDeliverable() {
        QQmlEngine engine;
        QmlSignalSource source;
        engine.rootContext()->setContextProperty("signalSource", &source);
        QByteArray qml = "import SignalTest 1.0\nObject { id: root; property int received: 0; property Connections relay: Connections { target: signalSource;\n";
        QList<QMetaMethod> methods;
        const auto *meta = source.metaObject();
        for (int i = meta->methodOffset(); i < meta->methodCount(); ++i) {
            const auto method = meta->method(i);
            if (method.methodType() != QMetaMethod::Signal)
                continue;
            auto name = method.name();name[0] = char(std::toupper(static_cast<unsigned char>(name[0])));
            qml += "function on" + name + "(" + (method.parameterCount() ? "value" : "") + ") { root.received += 1 }\n";
            methods.append(method);
        }
        qml += "} }";
        QQmlComponent component(&engine);component.setData(qml, QUrl());
        std::unique_ptr<QObject> root(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        QVERIFY(methods.size() > 512);
        std::atomic_bool invoked{true};
        std::thread worker([&] {
            for (const auto &method : methods) {
                const bool ok = method.parameterCount()
                    ? method.invoke(&source, Qt::DirectConnection, Q_ARG(int, 41))
                    : method.invoke(&source, Qt::DirectConnection);
                if (!ok) invoked.store(false);
            }
        });
        worker.join();drain();
        QVERIFY(invoked.load());
        QCOMPARE(root->property("received").toInt(), methods.size());
    }
    void repeatedEngineAndNotifierDestruction() {
        for (int i = 0; i < 50; ++i) {
            QQmlEngine engine;
            QmlSignalSource source;
            engine.rootContext()->setContextProperty("signalSource", &source);
            QQmlComponent component(&engine);component.setData(scene(), QUrl());
            std::unique_ptr<QObject> root(component.create());
            QVERIFY2(root, qPrintable(component.errorString()));
            std::thread worker([&] { emit source.observed(i); source.setValue(i + 1); });
            worker.join();drain();
            QCOMPARE(root->property("received").toInt(), 1);
            QCOMPARE(root->property("lastValue").toInt(), i);
            QCOMPARE(root->property("boundValue").toInt(), i + 1);
        }
    }
    void retainedNotifyListCanReconnect() {
        QQmlEngine engine;
        QmlSignalSource source;
        engine.rootContext()->setContextProperty("signalSource", &source);
        QQmlComponent component(&engine);component.setData(scene(), QUrl());
        std::unique_ptr<QObject> root(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        root.reset();drain();
        auto *data = QQmlData::get(&source, false);
        QVERIFY(data);
        const auto *originalList = data->notifyList.loadRelaxed();
        QVERIFY(originalList);
        data->disconnectNotifiers(QQmlData::DeleteNotifyList::No);
        QCOMPARE(data->notifyList.loadRelaxed(), originalList);
        const int observed = QMetaObjectPrivate::signalIndex(QMetaMethod::fromSignal(&QmlSignalSource::observed));
        QVERIFY(!QQmlData::isSignalConnected(data, &source, observed));
        root.reset(component.create());
        QVERIFY2(root, qPrintable(component.errorString()));
        QCOMPARE(data->notifyList.loadRelaxed(), originalList);
        std::thread worker([&] { emit source.observed(37); source.setValue(29); });
        worker.join();drain();
        QCOMPARE(root->property("received").toInt(), 1);
        QCOMPARE(root->property("lastValue").toInt(), 37);
        QCOMPARE(root->property("boundValue").toInt(), 29);
    }
};
int main(int argc, char **argv) {
    CountingApplication application(argc, argv);
    qmlRegisterType<QObject>("SignalTest", 1, 0, "Object");
    qmlRegisterType<QQmlConnections>("SignalTest", 1, 0, "Connections");
    F4QtQmlSignalMaskTests test;
    return QTest::qExec(&test, argc, argv);
}
#include "F4QtQmlSignalMaskTests.moc"
