#include <QtPlugin>

Q_IMPORT_PLUGIN(QOffscreenIntegrationPlugin)

#if defined(Q_OS_WIN)
Q_IMPORT_PLUGIN(QWindowsIntegrationPlugin)
#elif defined(Q_OS_LINUX)
Q_IMPORT_PLUGIN(QXcbIntegrationPlugin)
#endif

Q_IMPORT_PLUGIN(QGifPlugin)
Q_IMPORT_PLUGIN(QIcoPlugin)
Q_IMPORT_PLUGIN(QJpegPlugin)
Q_IMPORT_PLUGIN(QSvgPlugin)
