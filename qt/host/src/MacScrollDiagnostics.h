#pragma once

#include <QObject>
#include <memory>

class QQuickWindow;

// App-local, opt-in recorder. Never consumes input or requests frames.
class MacScrollDiagnostics final : public QObject
{
    Q_OBJECT
public:
    static MacScrollDiagnostics *start(QQuickWindow *window, const QString &path);
    ~MacScrollDiagnostics() override;
    void flush();

protected:
    bool eventFilter(QObject *receiver, QEvent *event) override;

private slots:
    void recordPosition();

private:
    MacScrollDiagnostics(QQuickWindow *window, const QString &path);
    void beginGesture();
    struct State;
    std::unique_ptr<State> state;
};
