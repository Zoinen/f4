// Profiling services are linked only into opt-in test builds, never the host.
#include <QtPlugin>
Q_IMPORT_PLUGIN(QTcpServerConnectionFactory)
Q_IMPORT_PLUGIN(QQmlDebugServerFactory)
Q_IMPORT_PLUGIN(QQmlProfilerServiceFactory)
Q_IMPORT_PLUGIN(QQuickProfilerAdapterFactory)
