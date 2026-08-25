#include <QtPlugin>

Q_IMPORT_PLUGIN(QOffscreenIntegrationPlugin)
#if defined(Q_OS_WIN)
Q_IMPORT_PLUGIN(QWindowsIntegrationPlugin)
#endif
