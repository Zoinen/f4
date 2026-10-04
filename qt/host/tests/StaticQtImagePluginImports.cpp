#include <QtPlugin>

Q_IMPORT_PLUGIN(QGifPlugin)
Q_IMPORT_PLUGIN(QICOPlugin)
#if defined(F4_QT_HAS_JPEG_PLUGIN)
Q_IMPORT_PLUGIN(QJpegPlugin)
#endif
Q_IMPORT_PLUGIN(QSvgPlugin)
