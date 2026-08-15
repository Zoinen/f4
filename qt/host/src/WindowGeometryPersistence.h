#pragma once

#include <QObject>
#include <QRect>
#include <QString>
#include <QTimer>

#include <memory>

class QSettings;
class QWindow;

enum class PersistedWindowState {
    Windowed,
    Maximized,
    FullScreen,
};

struct PersistedWindowGeometry {
    bool valid = false;
    QRect normalGeometry;
    QString screenName;
    QRect screenAvailableGeometry;
    PersistedWindowState state = PersistedWindowState::Windowed;
};

struct WindowScreenGeometry {
    QString name;
    QRect availableGeometry;
};

class WindowGeometryPersistence final : public QObject
{
    Q_OBJECT

public:
    explicit WindowGeometryPersistence(QWindow *window,
                                       const QString &settingsFile = {},
                                       QObject *parent = nullptr);
    ~WindowGeometryPersistence() override;

    bool restore();
    void save();

    static PersistedWindowGeometry read(QSettings &settings);
    static void write(QSettings &settings,
                      const PersistedWindowGeometry &geometry);
    static QRect resolvedNormalGeometry(
        const PersistedWindowGeometry &stored,
        const QList<WindowScreenGeometry> &screens,
        const QString &primaryScreenName,
        const QSize &minimumSize = QSize(320, 240));

protected:
    bool eventFilter(QObject *watched, QEvent *event) override;

private:
    void rememberWindowedGeometry();
    PersistedWindowState currentPersistentState() const;

    QWindow *m_window = nullptr;
    std::unique_ptr<QSettings> m_settings;
    QRect m_normalGeometry;
    PersistedWindowState m_lastNonMinimizedState =
        PersistedWindowState::Windowed;
    bool m_restoring = false;
    QTimer m_saveTimer;
};
