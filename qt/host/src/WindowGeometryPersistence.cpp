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
    const QSize &minimumSize)
{
    if (!stored.valid || screens.isEmpty())
        return {};

    const WindowScreenGeometry *target = nullptr;
    for (const auto &screen : screens) {
        if (screen.name == stored.screenName) {
            target = &screen;
            break;
        }
    }
    if (!target) {
        for (const auto &screen : screens) {
            if (screen.availableGeometry.contains(
                    stored.normalGeometry.center())) {
                target = &screen;
                break;
            }
        }
    }
    if (!target) {
        for (const auto &screen : screens) {
            if (screen.name == primaryScreenName) {
                target = &screen;
                break;
            }
        }
    }
    if (!target)
        target = &screens.constFirst();

    QRect geometry = stored.normalGeometry;
    const QRect available = target->availableGeometry;
    if (!available.isValid())
        return geometry;

    // Preserve the offset relative to the saved monitor. This keeps the
    // window in the same physical place when a monitor's global origin moves
    // because another display was attached or removed.
    if (stored.screenAvailableGeometry.isValid()) {
        geometry.moveTopLeft(
            available.topLeft()
            + (stored.normalGeometry.topLeft()
               - stored.screenAvailableGeometry.topLeft()));
    }

    const int minimumWidth = std::min(minimumSize.width(), available.width());
    const int minimumHeight = std::min(minimumSize.height(), available.height());
    geometry.setWidth(std::clamp(geometry.width(), minimumWidth,
                                 available.width()));
    geometry.setHeight(std::clamp(geometry.height(), minimumHeight,
                                  available.height()));
    geometry.moveLeft(std::clamp(geometry.left(), available.left(),
                                 available.right() - geometry.width() + 1));
    geometry.moveTop(std::clamp(geometry.top(), available.top(),
                                available.bottom() - geometry.height() + 1));
    return geometry;
}

bool WindowGeometryPersistence::restore()
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
        QSize(std::max(1, m_window->minimumWidth()),
              std::max(1, m_window->minimumHeight())));
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
    if (stored.state == PersistedWindowState::Maximized)
        m_window->showMaximized();
    else if (stored.state == PersistedWindowState::FullScreen)
        m_window->showFullScreen();
    else
        m_window->showNormal();
    m_restoring = false;
    return true;
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
    write(*m_settings, stored);
}

bool WindowGeometryPersistence::eventFilter(QObject *watched, QEvent *event)
{
    if (watched == m_window && !m_restoring) {
        if (event->type() == QEvent::Close) {
            // The Go owner may terminate the sidecar immediately after QML's
            // closing handler sends quit. Persist before that signal crosses
            // the process boundary instead of relying only on app.exec()
            // returning in time.
            m_saveTimer.stop();
            save();
        } else if (event->type() == QEvent::Move
                   || event->type() == QEvent::Resize) {
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
