#pragma once
#include <QtCore/qglobal.h>

QT_BEGIN_NAMESPACE
class QSGNode;

// Optional renderer capability. Every function is render-thread only.
struct F4VectorUploadGateInterface {
    quint32 version;
    quint32 size;
    void (*configure)(QSGNode *root, quint64 generation, bool cpuComplete,
                      bool present, int priority, bool previouslyPrepared);
    void (*remove)(QSGNode *root);
    bool (*ready)(QSGNode *root, quint64 generation);
    bool (*needsFrame)(QSGNode *root, quint64 generation);
    bool (*failed)(QSGNode *root, quint64 generation);
};
QT_END_NAMESPACE
