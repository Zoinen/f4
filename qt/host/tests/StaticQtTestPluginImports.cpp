#include <QtPlugin>

Q_IMPORT_PLUGIN(QOffscreenIntegrationPlugin)

#if defined(F4_QT_USE_WINDOWS_MEDIA_PLUGIN)
Q_IMPORT_PLUGIN(QWindowsMediaPlugin)
#elif defined(F4_QT_USE_FFMPEG_PLUGIN)
Q_IMPORT_PLUGIN(QFFmpegMediaPlugin)
#endif
#if defined(Q_OS_WIN)
Q_IMPORT_PLUGIN(QWindowsIntegrationPlugin)
#endif
