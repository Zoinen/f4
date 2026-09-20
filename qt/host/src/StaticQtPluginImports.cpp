#include <QtPlugin>

Q_IMPORT_PLUGIN(QOffscreenIntegrationPlugin)

#if defined(Q_OS_WIN)
Q_IMPORT_PLUGIN(QWindowsIntegrationPlugin)
#elif defined(Q_OS_LINUX)
Q_IMPORT_PLUGIN(QXcbIntegrationPlugin)
Q_IMPORT_PLUGIN(QXcbEglIntegrationPlugin)
Q_IMPORT_PLUGIN(QXcbGlxIntegrationPlugin)
Q_IMPORT_PLUGIN(QWaylandIntegrationPlugin)
Q_IMPORT_PLUGIN(QWaylandEglClientBufferPlugin)
Q_IMPORT_PLUGIN(QWaylandXdgShellIntegrationPlugin)
#endif

Q_IMPORT_PLUGIN(QGifPlugin)
Q_IMPORT_PLUGIN(QICOPlugin)
#if defined(F4_QT_HAS_JPEG_PLUGIN)
Q_IMPORT_PLUGIN(QJpegPlugin)
#endif
Q_IMPORT_PLUGIN(QSvgPlugin)
#if defined(F4_QT_USE_WINDOWS_MEDIA_PLUGIN)
Q_IMPORT_PLUGIN(QWindowsMediaPlugin)
#else
Q_IMPORT_PLUGIN(QFFmpegMediaPlugin)
#endif
