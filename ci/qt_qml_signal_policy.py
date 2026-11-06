"""Qt 6.11.1 conservative signal mask; keep the SDK NotifyList prefix intact.

Unobserved render-stage signals can collide with QML property notifications in
Qt's 64-bit mask and post cross-thread proxy events each frame. Allocation and
deletion of NotifyList are owned by qqmlengine.cpp, so its private extension
does not require changing installed headers or the QQmlData layout.
"""

_EXTENSION = '''

namespace {
struct F4SignalNotifyList final : QQmlData::NotifyList {
    QAtomicInteger<quint64> masks[8];
    F4SignalNotifyList() { for (auto &mask : masks) mask.storeRelaxed(0); }
};
}
'''

_OLD_QUERY = '''    return ddata->signalHasEndpoint(index);
}'''
_NEW_QUERY = '''    auto *list = ddata->notifyList.loadRelaxed();
    if (!list)
        return false;
    const auto *wide = static_cast<const F4SignalNotifyList *>(list);
    return wide->masks[(unsigned(index) / 64) % 8].loadRelaxed()
            & (1ULL << quint64(index % 64));
}'''
_OLD_ADD = '''    list->connectionMask.storeRelaxed(
            list->connectionMask.loadRelaxed() | (1ULL << quint64(index % 64)));'''

REPLACEMENTS = (
    ('using namespace Qt::StringLiterals;', 'using namespace Qt::StringLiterals;' + _EXTENSION),
    (_OLD_QUERY, _NEW_QUERY),
    ('list = new NotifyList;', 'list = new F4SignalNotifyList;'),
    (_OLD_ADD, _OLD_ADD + '''
    auto &mask = static_cast<F4SignalNotifyList *>(list)->masks[(unsigned(index) / 64) % 8];
    mask.storeRelaxed(mask.loadRelaxed() | (1ULL << quint64(index % 64)));'''),
    ('            delete list;', '            delete static_cast<F4SignalNotifyList *>(list);'),
    ('            list->connectionMask.storeRelaxed(0);', '''            list->connectionMask.storeRelaxed(0);
            for (auto &mask : static_cast<F4SignalNotifyList *>(list)->masks)
                mask.storeRelaxed(0);'''),
)
