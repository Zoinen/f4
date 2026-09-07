#include "F4ApplicationIcon.h"

#include <QtGlobal>

#if !defined(Q_OS_MACOS)
#include <QGuiApplication>
#include <QIcon>
#endif

#if defined(Q_OS_WIN)
#include <QEvent>
#include <QPointer>
#include <QTimer>
#include <QWindow>
#include <qt_windows.h>

namespace
{
void applyNativeWindowIcons(QWindow *window)
{
    if (!window->isTopLevel() || !window->handle())
        return;
    const HWND hwnd = reinterpret_cast<HWND>(window->winId());
    const UINT nativeDpi = GetDpiForWindow(hwnd);
    const UINT dpi = nativeDpi ? nativeDpi : 96;
    const auto load = [dpi](int dip) {
        const int size = MulDiv(dip, dpi, 96);
        return reinterpret_cast<HICON>(LoadImageW(
            GetModuleHandleW(nullptr), MAKEINTRESOURCEW(101), IMAGE_ICON,
            size, size, LR_SHARED));
    };
    // Match cmd/f4/window_icon_windows.go: the taskbar needs 24 DIP, not
    // Qt's SM_CXICON (32 DIP). At 175% use the authored 42 px frame directly
    // instead of giving Explorer 56 px artwork to downsample. LoadImage also
    // avoids Qt's QPixmap -> HICON conversion. LR_SHARED owns these handles.
    const HICON titlebar = load(16);
    const HICON taskbar = load(24);
    if (titlebar && taskbar) {
        SendMessageW(hwnd, WM_SETICON, ICON_SMALL, reinterpret_cast<LPARAM>(titlebar));
        SendMessageW(hwnd, WM_SETICON, ICON_BIG, reinterpret_cast<LPARAM>(taskbar));
    }
}

class NativeWindowIcons final : public QObject
{
public:
    using QObject::QObject;
    bool eventFilter(QObject *object, QEvent *event) override
    {
        switch (event->type()) {
        case QEvent::Show:
        case QEvent::WinIdChange:
        case QEvent::ScreenChangeInternal:
        case QEvent::DevicePixelRatioChange:
        case QEvent::WindowIconChange:
            if (auto *window = qobject_cast<QWindow *>(object)) {
                // Qt may replace its HICON while processing these events.
                // Apply ours after it finishes, including on monitor changes
                // and native-window recreation. Destruction cancels the call.
                QTimer::singleShot(0, window, [window] { applyNativeWindowIcons(window); });
            }
            break;
        default:
            break;
        }
        return false;
    }
};
}
#endif

namespace F4ApplicationIcon
{
bool isBundleManaged() noexcept
{
#if defined(Q_OS_MACOS)
    return true;
#else
    return false;
#endif
}

void installRuntimeFallback()
{
#if !defined(Q_OS_MACOS)
    const QIcon applicationIcon(
#if defined(Q_OS_WIN)
        QStringLiteral(":/F4QtHost/icons/app/f4.ico"));
#else
        QStringLiteral(":/F4QtHost/icons/app/f4.svg"));
#endif
    if (!applicationIcon.isNull())
        QGuiApplication::setWindowIcon(applicationIcon);
#if defined(Q_OS_WIN)
    if (QGuiApplication::platformName() == QStringLiteral("windows")) {
        static QPointer<NativeWindowIcons> filter;
        if (!filter) {
            filter = new NativeWindowIcons(qGuiApp);
            qGuiApp->installEventFilter(filter);
        }
        for (QWindow *window : QGuiApplication::topLevelWindows())
            applyNativeWindowIcons(window);
    }
#endif
#endif
}
}
