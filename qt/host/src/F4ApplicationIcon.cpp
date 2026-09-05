#include "F4ApplicationIcon.h"

#include <QtGlobal>

#if !defined(Q_OS_MACOS)
#include <QGuiApplication>
#include <QIcon>
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
        QStringLiteral(":/F4QtHost/icons/app/f4.svg"));
    if (!applicationIcon.isNull())
        QGuiApplication::setWindowIcon(applicationIcon);
#endif
}
}
