"""Precise Qt animation time and render-thread job registration before commit."""

PROTOCOL = '''struct F4AnimationClockInterface {
    quint32 version;
    quint32 size;
    bool (*sample)(double *milliseconds);
};
'''

REPLACEMENTS = (
    ('qsgcontext.cpp', (
        ('class QSGDefaultAnimationDriver : public QSGAnimationDriver',
         PROTOCOL + '''namespace {
thread_local double f4AnimationMilliseconds = -1;
bool f4SampleAnimationClock(double *milliseconds)
{
    if (!milliseconds || f4AnimationMilliseconds < 0)
        return false;
    *milliseconds = f4AnimationMilliseconds;
    return true;
}
}
void *f4AnimationClockInterface()
{
    static const F4AnimationClockInterface api = {
        1, sizeof(F4AnimationClockInterface), &f4SampleAnimationClock
    };
    return const_cast<F4AnimationClockInterface *>(&api);
}
class QSGDefaultAnimationDriver : public QSGAnimationDriver'''),
        ('''        advanceAnimation();
    }

    bool isVSyncDependent() const override
    {
        return true;''',
         '''        // Preserve Qt's VSync/timer-mode decisions and expose no stale clock
        // outside the synchronous animation callbacks or on another thread.
        const double previous = f4AnimationMilliseconds;
        f4AnimationMilliseconds = m_mode == VSyncMode ? m_time : -1;
        advanceAnimation();
        f4AnimationMilliseconds = previous;
    }

    bool isVSyncDependent() const override
    {
        return true;'''),
    )),
    ('coreapi/qsgrendererinterface.cpp', (
        ('void *QSGRendererInterface::getResource(QQuickWindow *window, const char *resource) const',
         'void *f4AnimationClockInterface();\n\nvoid *QSGRendererInterface::getResource(QQuickWindow *window, const char *resource) const'),
        ('    if (resource && qstrcmp(resource, "f4.vectorUploadGate.v2") == 0)',
         '    if (resource && qstrcmp(resource, "f4.animationClock.v1") == 0)\n        return f4AnimationClockInterface();\n    if (resource && qstrcmp(resource, "f4.vectorUploadGate.v2") == 0)'),
    )),
    ('../util/qquickanimatorcontroller.cpp', (
        ('#include "qquickanimatorcontroller_p.h"',
         '#include "qquickanimatorcontroller_p.h"\n#include <QtCore/private/qabstractanimation_p.h>\n#include <QtCore/qcoreevent.h>\n#include <QtCore/qthread.h>'),
        ('    // Start pending jobs\n    for (const QSharedPointer<QAbstractAnimationJob> &job : std::as_const(m_rootsPendingStart)) {',
         '    // Start pending jobs\n    const bool startedAnimations = !m_rootsPendingStart.isEmpty();\n    for (const QSharedPointer<QAbstractAnimationJob> &job : std::as_const(m_rootsPendingStart)) {'),
        ('    m_rootsPendingStart.clear();',
         '''    m_rootsPendingStart.clear();
    // A render job must finish timer registration before its first commit.
    // Keep the basic/GUI render loop's event ordering unchanged. Only deliver
    // this render thread's unified timer calls, not general application events.
    if (startedAnimations && QThread::currentThread() != m_window->thread()) {
        if (auto *timer = QQmlAnimationTimer::instance(false);
            timer && timer->hasStartAnimationPending()) {
            timer->startAnimations();
            if (auto *unified = QUnifiedTimer::instance(false))
                QCoreApplication::sendPostedEvents(unified, QEvent::MetaCall);
        }
    }
'''),
    )),
)
