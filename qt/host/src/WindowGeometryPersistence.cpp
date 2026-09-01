#include "WindowGeometryPersistence.h"

#include <QEvent>
#include <QGuiApplication>
#include <QScreen>
#include <QSettings>
#include <QTimer>
#include <QWindow>

#include <algorithm>

namespace {
constexpr auto settingsGroup = "MainWindowGeometry";
constexpr int settingsVersion = 1;
constexpr int minimumRestoredWidth = 320;
constexpr int minimumRestoredHeight = 240;
constexpr int restoreStabilizationDelayMs = 50;

QString stateName(PersistedWindowState state)
{
    switch (state) {
    case PersistedWindowState::Maximized:
        return QStringLiteral("maximized");
    case PersistedWindowState::FullScreen:
        return QStringLiteral("fullscreen");
    case PersistedWindowState::Windowed:
    default:
        return QStringLiteral("windowed");
    }
}

PersistedWindowState stateFromName(const QString &name)
{
    if (name == QStringLiteral("maximized"))
        return PersistedWindowState::Maximized;
    if (name == QStringLiteral("fullscreen"))
        return PersistedWindowState::FullScreen;
    return PersistedWindowState::Windowed;
}

QList<WindowScreenGeometry> currentScreens()
{
    QList<WindowScreenGeometry> result;
    const auto screens = QGuiApplication::screens();
    result.reserve(screens.size());
    for (const QScreen *screen : screens) {
        if (screen) {
            result.append({screen->name(), screen->availableGeometry()});
        }
    }
    return result;
}

QScreen *screenNamed(const QString &name)
{
    for (QScreen *screen : QGuiApplication::screens()) {
        if (screen && screen->name() == name)
            return screen;
    }
    return nullptr;
}

const WindowScreenGeometry *matchingStoredScreen(
    const QList<WindowScreenGeometry> &screens,
    const PersistedWindowGeometry &stored)
{
    for (const auto &screen : screens) {
        if (screen.availableGeometry.isValid()
            && screen.name == stored.screenName) {
            return &screen;
        }
    }
    return nullptr;
}

const WindowScreenGeometry *largestIntersectingScreen(
    const QList<WindowScreenGeometry> &screens, const QRect &geometry)
{
    const WindowScreenGeometry *target = nullptr;
    qint64 largestArea = 0;
    for (const auto &screen : screens) {
        if (!screen.availableGeometry.isValid())
            continue;
        const QRect intersection = screen.availableGeometry.intersected(
            geometry);
        const qint64 area = intersection.isValid()
            ? qint64(intersection.width()) * intersection.height() : 0;
        if (area > largestArea) {
            largestArea = area;
            target = &screen;
        }
    }
    return target;
}

const WindowScreenGeometry *fallbackScreen(
    const QList<WindowScreenGeometry> &screens,
    const QString &primaryScreenName)
{
    for (const auto &screen : screens) {
        if (screen.availableGeometry.isValid()
            && screen.name == primaryScreenName) {
            return &screen;
        }
    }
    for (const auto &screen : screens) {
        if (screen.availableGeometry.isValid())
            return &screen;
    }
    return nullptr;
}

QRect fitRestoredGeometry(
    QRect geometry, const QRect &savedAvailable, const QRect &available,
    const QSize &minimumSize, const QMargins &frameMargins)
{
    if (savedAvailable.isValid()) {
        geometry.moveTopLeft(
            available.topLeft()
            + (geometry.topLeft() - savedAvailable.topLeft()));
    }
    const bool intersectedBeforeFitting = available.intersects(geometry);
    int leftFrame = std::max(0, frameMargins.left());
    int topFrame = std::max(0, frameMargins.top());
    int rightFrame = std::max(0, frameMargins.right());
    int bottomFrame = std::max(0, frameMargins.bottom());
    if (leftFrame + rightFrame >= available.width())
        leftFrame = rightFrame = 0;
    if (topFrame + bottomFrame >= available.height())
        topFrame = bottomFrame = 0;
    const int maximumWidth = std::max(
        1, available.width() - leftFrame - rightFrame);
    const int maximumHeight = std::max(
        1, available.height() - topFrame - bottomFrame);
    geometry.setWidth(std::clamp(
        geometry.width(),
        std::clamp(std::max(1, minimumSize.width()), 1, maximumWidth),
        maximumWidth));
    geometry.setHeight(std::clamp(
        geometry.height(),
        std::clamp(std::max(1, minimumSize.height()), 1, maximumHeight),
        maximumHeight));
    const int minimumLeft = available.left() + leftFrame;
    const int maximumLeft = available.right() - rightFrame
        - geometry.width() + 1;
    const int minimumTop = available.top() + topFrame;
    const int maximumTop = available.bottom() - bottomFrame
        - geometry.height() + 1;
    if (!intersectedBeforeFitting) {
        geometry.moveCenter(QRect(
            QPoint(minimumLeft, minimumTop),
            QPoint(available.right() - rightFrame,
                   available.bottom() - bottomFrame)).center());
    }
    geometry.moveLeft(std::clamp(
        geometry.left(), minimumLeft, maximumLeft));
    geometry.moveTop(std::clamp(
        geometry.top(), minimumTop, maximumTop));
    return geometry;
}
}

WindowGeometryPersistence::WindowGeometryPersistence(
    QWindow *window, const QString &settingsFile, QObject *parent)
    : QObject(parent)
    , m_window(window)
{
    m_saveTimer.setSingleShot(true);
    m_saveTimer.setInterval(300);
    connect(&m_saveTimer, &QTimer::timeout,
            this, &WindowGeometryPersistence::save);

    if (settingsFile.isEmpty()) {
        m_settings = std::make_unique<QSettings>(
            QSettings::IniFormat, QSettings::UserScope,
            QStringLiteral("f4"), QStringLiteral("f4-qt-host"));
    } else {
        m_settings = std::make_unique<QSettings>(settingsFile,
                                                 QSettings::IniFormat);
    }

    if (m_window) {
        m_window->installEventFilter(this);
        rememberWindowedGeometry();
        connect(m_window, &QWindow::visibilityChanged, this,
                [this](QWindow::Visibility visibility) {
            if (m_restoring)
                return;
            if (visibility == QWindow::Maximized)
                m_lastNonMinimizedState = PersistedWindowState::Maximized;
            else if (visibility == QWindow::FullScreen)
                m_lastNonMinimizedState = PersistedWindowState::FullScreen;
            else if (visibility == QWindow::Windowed) {
                m_lastNonMinimizedState = PersistedWindowState::Windowed;
                // The platform restores the normal frame during this signal.
                // Capture it after that native transition has settled.
                QTimer::singleShot(0, this, [this] {
                    rememberWindowedGeometry();
                });
            }
            m_saveTimer.start();
        });
    }
}

WindowGeometryPersistence::~WindowGeometryPersistence()
{
    if (m_window)
        m_window->removeEventFilter(this);
}

PersistedWindowGeometry WindowGeometryPersistence::read(QSettings &settings)
{
    PersistedWindowGeometry result;
    settings.beginGroup(QString::fromLatin1(settingsGroup));
    const int version = settings.value(QStringLiteral("version"), 0).toInt();
    result.normalGeometry = settings.value(
        QStringLiteral("normalGeometry")).toRect();
    result.screenName = settings.value(QStringLiteral("screenName")).toString();
    result.screenAvailableGeometry = settings.value(
        QStringLiteral("screenAvailableGeometry")).toRect();
    result.state = stateFromName(
        settings.value(QStringLiteral("state"),
                       QStringLiteral("windowed")).toString());
    settings.endGroup();
    result.valid = version == settingsVersion
        && result.normalGeometry.isValid()
        && result.normalGeometry.width() > 0
        && result.normalGeometry.height() > 0;
    return result;
}

void WindowGeometryPersistence::write(
    QSettings &settings, const PersistedWindowGeometry &geometry)
{
    settings.beginGroup(QString::fromLatin1(settingsGroup));
    settings.setValue(QStringLiteral("version"), settingsVersion);
    settings.setValue(QStringLiteral("normalGeometry"),
                      geometry.normalGeometry);
    settings.setValue(QStringLiteral("screenName"), geometry.screenName);
    settings.setValue(QStringLiteral("screenAvailableGeometry"),
                      geometry.screenAvailableGeometry);
    settings.setValue(QStringLiteral("state"), stateName(geometry.state));
    settings.endGroup();
    settings.sync();
}

QRect WindowGeometryPersistence::resolvedNormalGeometry(
    const PersistedWindowGeometry &stored,
    const QList<WindowScreenGeometry> &screens,
    const QString &primaryScreenName,
    const QSize &minimumSize,
    const QMargins &frameMargins)
{
    if (!stored.valid || screens.isEmpty())
        return {};
    const WindowScreenGeometry *target = matchingStoredScreen(
        screens, stored);
    if (!target)
        target = largestIntersectingScreen(screens, stored.normalGeometry);
    if (!target)
        target = fallbackScreen(screens, primaryScreenName);
    if (!target)
        return {};
    return fitRestoredGeometry(
        stored.normalGeometry, stored.screenAvailableGeometry,
        target->availableGeometry, minimumSize, frameMargins);
}

bool WindowGeometryPersistence::restore()
{
    return restoreImpl(false);
}

bool WindowGeometryPersistence::restoreDeferred()
{
    return restoreImpl(true);
}

bool WindowGeometryPersistence::restoreImpl(bool deferShow)
{
    if (!m_window || !m_settings)
        return false;
    const PersistedWindowGeometry stored = read(*m_settings);
    if (!stored.valid)
        return false;

    const QString primaryName = QGuiApplication::primaryScreen()
        ? QGuiApplication::primaryScreen()->name() : QString();
    const QRect geometry = resolvedNormalGeometry(
        stored, currentScreens(), primaryName,
        minimumRestoreSize(), m_window->frameMargins());
    if (!geometry.isValid())
        return false;

    m_restoring = true;
    QScreen *targetScreen = screenNamed(stored.screenName);
    if (!targetScreen)
        targetScreen = QGuiApplication::screenAt(geometry.center());
    if (!targetScreen)
        targetScreen = QGuiApplication::primaryScreen();
    if (targetScreen)
        m_window->setScreen(targetScreen);
    m_window->setGeometry(geometry);
    m_normalGeometry = geometry;
    m_lastNonMinimizedState = stored.state;
    if (!deferShow) {
        applyRestoredWindowState();
        scheduleRestoreStabilization();
    } else {
        m_restoring = false;
    }
    return true;
}

void WindowGeometryPersistence::showRestored()
{
    if (!m_window || m_window->isVisible())
        return;
    m_restoring = true;
    applyRestoredWindowState();
    scheduleRestoreStabilization();
}

void WindowGeometryPersistence::applyRestoredWindowState()
{
    if (!m_window)
        return;

    // Creating/showing the native window establishes its real non-client
    // frame and per-monitor DPI. Reapply the client geometry only after that
    // happened; otherwise Windows/QWK converts a pre-show frame as if it were
    // client geometry and the saved height grows on every launch.
    m_window->showNormal();
    applyNormalGeometryWithCurrentFrame();
    if (m_lastNonMinimizedState == PersistedWindowState::Maximized)
        m_window->showMaximized();
    else if (m_lastNonMinimizedState == PersistedWindowState::FullScreen)
        m_window->showFullScreen();
}

void WindowGeometryPersistence::applyNormalGeometryWithCurrentFrame()
{
    if (!m_window || !m_normalGeometry.isValid())
        return;

    QScreen *screen = m_window->screen();
    if (!screen)
        screen = QGuiApplication::screenAt(m_normalGeometry.center());
    if (!screen)
        screen = QGuiApplication::primaryScreen();
    if (!screen)
        return;

    PersistedWindowGeometry current;
    current.valid = true;
    current.normalGeometry = m_normalGeometry;
    current.screenName = screen->name();
    current.screenAvailableGeometry = screen->availableGeometry();
    const QRect fitted = resolvedNormalGeometry(
        current, currentScreens(), screen->name(), minimumRestoreSize(),
        m_window->frameMargins());
    if (!fitted.isValid())
        return;

    m_window->setGeometry(fitted);
    m_normalGeometry = fitted;
}

void WindowGeometryPersistence::scheduleRestoreStabilization()
{
    QTimer::singleShot(0, this, [this] {
        if (!m_window) {
            m_restoring = false;
            return;
        }
        if (m_lastNonMinimizedState == PersistedWindowState::Windowed)
            applyNormalGeometryWithCurrentFrame();

        QTimer::singleShot(restoreStabilizationDelayMs, this, [this] {
            if (m_window
                && m_lastNonMinimizedState
                    == PersistedWindowState::Windowed) {
                applyNormalGeometryWithCurrentFrame();
            }
            m_restoring = false;
        });
    });
}

void WindowGeometryPersistence::save()
{
    if (!m_window || !m_settings)
        return;
    rememberWindowedGeometry();
    if (!m_normalGeometry.isValid())
        return;

    QScreen *screen = m_window->screen();
    if (!screen)
        screen = QGuiApplication::screenAt(m_normalGeometry.center());
    if (!screen)
        screen = QGuiApplication::primaryScreen();

    PersistedWindowGeometry stored;
    stored.valid = true;
    stored.normalGeometry = m_normalGeometry;
    stored.state = currentPersistentState();
    if (screen) {
        stored.screenName = screen->name();
        stored.screenAvailableGeometry = screen->availableGeometry();
    }
    const QString primaryName = QGuiApplication::primaryScreen()
        ? QGuiApplication::primaryScreen()->name() : QString();
    const QRect sanitized = resolvedNormalGeometry(
        stored, currentScreens(), primaryName, minimumRestoreSize(),
        m_window->frameMargins());
    if (!sanitized.isValid())
        return;
    stored.normalGeometry = sanitized;
    m_normalGeometry = sanitized;
    write(*m_settings, stored);
}

bool WindowGeometryPersistence::eventFilter(QObject *watched, QEvent *event)
{
    if (watched == m_window) {
        if (event->type() == QEvent::Close) {
            // The Go owner may terminate the sidecar immediately after QML's
            // closing handler sends quit. Persist before that signal crosses
            // the process boundary instead of relying only on app.exec()
            // returning in time.
            m_saveTimer.stop();
            save();
        } else if (!m_restoring
                   && (event->type() == QEvent::Move
                       || event->type() == QEvent::Resize)) {
            rememberWindowedGeometry();
            m_saveTimer.start();
        }
    }
    return QObject::eventFilter(watched, event);
}

void WindowGeometryPersistence::rememberWindowedGeometry()
{
    if (!m_window || m_restoring
        || m_window->visibility() != QWindow::Windowed
        || m_window->windowStates() != Qt::WindowNoState)
        return;
    const QRect geometry = m_window->geometry();
    if (geometry.isValid() && geometry.width() > 0 && geometry.height() > 0)
        m_normalGeometry = geometry;
}

QSize WindowGeometryPersistence::minimumRestoreSize() const
{
    return QSize(
        std::max(minimumRestoredWidth,
                 m_window ? m_window->minimumWidth() : 0),
        std::max(minimumRestoredHeight,
                 m_window ? m_window->minimumHeight() : 0));
}

PersistedWindowState WindowGeometryPersistence::currentPersistentState() const
{
    if (!m_window)
        return PersistedWindowState::Windowed;
    if (m_window->visibility() == QWindow::Maximized)
        return PersistedWindowState::Maximized;
    if (m_window->visibility() == QWindow::FullScreen)
        return PersistedWindowState::FullScreen;
    // Minimized is intentionally transient: reopening an application into an
    // invisible Dock/taskbar state is surprising. Restore the state from
    // which it was minimized instead.
    if (m_window->visibility() == QWindow::Minimized)
        return m_lastNonMinimizedState;
    // Closing first hides the QML window and only then lets app.exec()
    // return. Keep the last visible state so a maximized/fullscreen window is
    // not accidentally serialized as windowed during normal shutdown.
    if (m_window->visibility() == QWindow::Hidden)
        return m_lastNonMinimizedState;
    return PersistedWindowState::Windowed;
}
