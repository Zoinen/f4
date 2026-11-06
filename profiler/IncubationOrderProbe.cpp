#include <QGuiApplication>
#include <QQmlComponent>
#include <QQmlEngine>
#include <QQmlIncubator>
#include <QThread>
#include <cstdio>
#include <functional>
#include <memory>

class Root final : public QQmlIncubator {
public:
    Root(int id, QList<int> &order)
        : QQmlIncubator(Asynchronous), id(id), order(order) {}
    std::function<void()> onInitialized;

protected:
    void setInitialState(QObject *) override {
        if (onInitialized)
            onInitialized();
    }

    void statusChanged(Status status) override {
        if (status == Ready)
            order.append(id);
    }

private:
    int id;
    QList<int> &order;
};

int main(int argc, char **argv) {
    QGuiApplication app(argc, argv);
    QQmlEngine engine;
    QQmlIncubationController controller;
    engine.setIncubationController(&controller);
    QQmlComponent component(&engine);
    component.setData(R"(
        import QtQuick
        Item {
            property alias nestedReady: nested.item
            Loader {
                id: nested
                sourceComponent: Component { Item {} }
            }
        }
    )", QUrl(QStringLiteral("qrc:/IncubationOrderProbe.qml")));
    if (!component.isReady()) {
        qWarning().noquote() << component.errorString();
        return 2;
    }
    QList<int> order;
    Root older(1, order), newer(2, order);
    const bool submitDuringInitialization = app.arguments().contains(
        QStringLiteral("--initialized"));
    if (submitDuringInitialization) {
        older.onInitialized = [&] {
            component.create(newer);
            QThread::msleep(5);
        };
    }
    component.create(older);
    if (app.arguments().contains(QStringLiteral("--aged")))
        QThread::msleep(35);
    if (!submitDuringInitialization)
        component.create(newer);
    for (int slice = 0; slice < 100 && controller.incubatingObjectCount(); ++slice)
        controller.incubateFor(1);
    std::unique_ptr<QObject> first(older.object()), second(newer.object());
    if (!first || !second || order.size() != 2
        || !first->property("nestedReady").value<QObject *>()
        || !second->property("nestedReady").value<QObject *>()) {
        return 3;
    }
    std::printf("ready order %d %d, both nested loaders ready\n", order[0], order[1]);
    return order == QList<int>{1, 2} ? 0 : 1;
}
