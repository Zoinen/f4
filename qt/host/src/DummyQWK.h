#ifndef F4_DUMMYQWK_H
#define F4_DUMMYQWK_H

#include <QQmlEngine>

static constexpr const char kQwkModuleUri[] = "QWindowKit";

class DummyQWK : public QObject
{
    Q_OBJECT

public:
    explicit DummyQWK(QObject *parent = nullptr) : QObject(parent) {}

    enum SystemButton {
        Unknown,
        WindowIcon,
        Help,
        Minimize,
        Maximize,
        Close,
    };
    Q_ENUM(SystemButton)

    Q_INVOKABLE void setup(QObject *) {}
    Q_INVOKABLE void setWindowAttribute(QString, bool) {}
    Q_INVOKABLE void setSystemButton(SystemButton, QObject *) {}
    Q_INVOKABLE void setSystemButtonArea(QObject *) {}
    Q_INVOKABLE void setTitleBar(QObject *) {}
    Q_INVOKABLE void setHitTestVisible(const QObject *, bool = true) {}
    Q_INVOKABLE void showSystemMenu(const QPoint &) {}

    static void registerTypes(QQmlEngine *engine)
    {
        Q_UNUSED(engine);

        static bool once = false;
        if (once) {
            return;
        }
        once = true;

        qmlRegisterType<DummyQWK>(kQwkModuleUri, 1, 0, "WindowAgent");
        qmlRegisterModule(kQwkModuleUri, 1, 0);
    }
};

#endif // F4_DUMMYQWK_H
