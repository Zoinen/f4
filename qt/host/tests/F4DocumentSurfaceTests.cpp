#include "DummyQWK.h"

#include <QCoreApplication>
#include <QElapsedTimer>
#include <QPointF>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>
#include <QWheelEvent>
#include <QtQml>
#include <QtTest>

#include <cmath>

namespace
{
class TestGrid : public QQuickItem
{
    Q_OBJECT
    Q_PROPERTY(QObject *controller READ controller WRITE setController)
    Q_PROPERTY(QString fontFamily READ fontFamily WRITE setFontFamily)
    Q_PROPERTY(int fontPixelSize READ fontPixelSize WRITE setFontPixelSize)
    Q_PROPERTY(qreal cellWidth READ cellWidth CONSTANT)
    Q_PROPERTY(qreal cellHeight READ cellHeight CONSTANT)
    Q_PROPERTY(bool pointerInputEnabled READ pointerInputEnabled
               WRITE setPointerInputEnabled)
    Q_PROPERTY(bool inputMethodForwardingEnabled READ inputMethodForwardingEnabled
               WRITE setInputMethodForwardingEnabled)
    Q_PROPERTY(bool terminalInputEnabled READ terminalInputEnabled
               WRITE setTerminalInputEnabled)

public:
    using QQuickItem::QQuickItem;

    QObject *controller() const { return m_controller; }
    void setController(QObject *controller) { m_controller = controller; }
    QString fontFamily() const { return m_fontFamily; }
    void setFontFamily(const QString &family) { m_fontFamily = family; }
    int fontPixelSize() const { return m_fontPixelSize; }
    void setFontPixelSize(int size) { m_fontPixelSize = size; }
    qreal cellWidth() const { return 8.0; }
    qreal cellHeight() const { return 20.0; }
    bool pointerInputEnabled() const { return m_pointerInputEnabled; }
    void setPointerInputEnabled(bool enabled) { m_pointerInputEnabled = enabled; }
    bool inputMethodForwardingEnabled() const
    {
        return m_inputMethodForwardingEnabled;
    }
    void setInputMethodForwardingEnabled(bool enabled)
    {
        m_inputMethodForwardingEnabled = enabled;
    }
    bool terminalInputEnabled() const { return m_terminalInputEnabled; }
    void setTerminalInputEnabled(bool enabled) { m_terminalInputEnabled = enabled; }

    Q_INVOKABLE void sendQtKey(int, const QString &, bool, int) {}
    Q_INVOKABLE void sendClipboardPaste() {}
    Q_INVOKABLE void sendQtText(const QString &) {}

private:
    QObject *m_controller = nullptr;
    QString m_fontFamily;
    int m_fontPixelSize = 13;
    bool m_pointerInputEnabled = false;
    bool m_inputMethodForwardingEnabled = false;
    bool m_terminalInputEnabled = true;
};

class TestShell final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)
    Q_PROPERTY(QVariantMap scene READ scene NOTIFY sceneChanged)
    Q_PROPERTY(QVariantMap presentationScene READ scene NOTIFY sceneChanged)

public:
    int initialCols() const { return 90; }
    int initialRows() const { return 30; }
    QVariantMap scene() const { return m_scene; }

    void setScene(const QVariantMap &scene)
    {
        m_scene = scene;
        emit sceneChanged();
    }

    void clearActions() { actions.clear(); }

    Q_INVOKABLE void sendUiAction(const QVariantMap &action)
    {
        actions.append(action);
        emit uiActionSent(action);
    }
    Q_INVOKABLE void sendQuit() {}
    Q_INVOKABLE void sendKey(int, int, bool, int) {}

    QVector<QVariantMap> actions;

signals:
    void sceneChanged();
    void uiActionSent(const QVariantMap &action);

private:
    QVariantMap m_scene;
};

class TestGallery final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool available READ available CONSTANT)
    Q_PROPERTY(QObject *viewerSession READ nullObject CONSTANT)
    Q_PROPERTY(bool viewerVisible READ viewerVisible CONSTANT)
    Q_PROPERTY(int viewerSide READ viewerSide CONSTANT)
    Q_PROPERTY(QUrl panelComponentUrl READ emptyUrl CONSTANT)
    Q_PROPERTY(QUrl viewerComponentUrl READ emptyUrl CONSTANT)

public:
    bool available() const { return false; }
    QObject *nullObject() const { return nullptr; }
    bool viewerVisible() const { return false; }
    int viewerSide() const { return 0; }
    QUrl emptyUrl() const { return {}; }

    Q_INVOKABLE QObject *sessionForSide(int) const { return nullptr; }
    Q_INVOKABLE void closeViewer() {}
};

class TestIcons final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(qulonglong revision READ revision CONSTANT)
    Q_PROPERTY(bool system READ system CONSTANT)
    Q_PROPERTY(bool fileIconsAreFullColor READ fileIconsAreFullColor CONSTANT)

public:
    qulonglong revision() const { return 1; }
    bool system() const { return false; }
    bool fileIconsAreFullColor() const { return false; }

    Q_INVOKABLE QUrl iconSource(const QString &, int, qreal) const { return {}; }
    Q_INVOKABLE QUrl fileIconSource(const QString &, const QString &, bool,
                                    int, qreal, qlonglong) const
    {
        return {};
    }
};

QVariantList byteRows(int firstOffset, int count, int stride = 10)
{
    QVariantList rows;
    rows.reserve(count);
    for (int index = 0; index < count; ++index) {
        const int start = firstOffset + index * stride;
        rows.append(QVariantMap{
            {QStringLiteral("offset"), start},
            {QStringLiteral("endOffset"), start + stride},
            {QStringLiteral("text"),
             QStringLiteral("byte row %1").arg(start)},
        });
    }
    return rows;
}

QVariantList editorRows(int firstRow, int count)
{
    QVariantList rows;
    rows.reserve(count);
    for (int index = 0; index < count; ++index) {
        const int visualRow = firstRow + index;
        rows.append(QVariantMap{
            {QStringLiteral("visualRow"), visualRow},
            {QStringLiteral("text"),
             QStringLiteral("editor row %1").arg(visualRow)},
        });
    }
    return rows;
}

QVariantMap documentScene(const QVariantMap &frame)
{
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("newTab"), QVariantMap{}},
             {QStringLiteral("counter"), QVariantMap{}},
         }},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("terminalActive"), false},
         }},
        {QStringLiteral("surface"), frame},
    };
}

QVariantMap viewerFrame(int firstOffset, int count, int viewportStart,
                        int generation, int stride = 10,
                        int contentExtent = 2000)
{
    const QVariantList rows = byteRows(firstOffset, count, stride);
    return {
        {QStringLiteral("id"), QStringLiteral("document-under-test")},
        {QStringLiteral("kind"), QStringLiteral("viewer")},
        {QStringLiteral("scrollUnit"), QStringLiteral("bytes")},
        {QStringLiteral("rows"), rows.mid(0, qMin(30, rows.size()))},
        {QStringLiteral("windowRows"), rows},
        {QStringLiteral("windowStart"), firstOffset},
        {QStringLiteral("windowEnd"), firstOffset + count * stride},
        {QStringLiteral("viewportStart"), viewportStart},
        {QStringLiteral("viewportSpan"), 30 * stride},
        {QStringLiteral("viewportRow"),
         qMax(0, (viewportStart - firstOffset) / stride)},
        {QStringLiteral("contentExtent"), contentExtent},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("windowGeneration"), generation},
    };
}

QVariantMap editorFrame(int firstRow, int count, int viewportStart,
                        int generation, int contentExtent = 500)
{
    const QVariantList rows = editorRows(firstRow, count);
    return {
        {QStringLiteral("id"), QStringLiteral("editor-window-test")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("rows"), rows.mid(0, qMin(20, rows.size()))},
        {QStringLiteral("windowRows"), rows},
        {QStringLiteral("windowStart"), firstRow},
        {QStringLiteral("windowEnd"), firstRow + count},
        {QStringLiteral("viewportStart"), viewportStart},
        {QStringLiteral("viewportSpan"), 20},
        {QStringLiteral("viewportRow"),
         qMax(0, viewportStart - firstRow)},
        {QStringLiteral("contentExtent"), contentExtent},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("cursorAbsoluteRow"), viewportStart + 2},
        {QStringLiteral("cursorVisualColumn"), 0},
        {QStringLiteral("cursorVisible"), true},
        {QStringLiteral("windowGeneration"), generation},
    };
}

void sendPixelWheel(QQuickWindow *window, const QPoint &position, int deltaY)
{
    QWheelEvent event(position,
                      window->mapToGlobal(position),
                      QPoint(0, deltaY),
                      {},
                      Qt::NoButton,
                      Qt::NoModifier,
                      Qt::NoScrollPhase,
                      false);
    QCoreApplication::sendEvent(window, &event);
}

qreal topExtent(QQuickItem *surface, QQuickItem *list)
{
    const QVariantList rows = surface->property("displayedRows").toList();
    if (rows.isEmpty())
        return 0;

    const qreal rowHeight = surface->property("rowHeight").toReal();
    const qreal raw = qMax<qreal>(0, list->property("contentY").toReal())
            / rowHeight
        - surface->property("loadedSlotStart").toInt();
    const int index = qBound(0, static_cast<int>(std::floor(raw)),
                             rows.size() - 1);
    const qreal fraction = raw - std::floor(raw);
    const QVariantMap row = rows.at(index).toMap();
    const QString scrollUnit = surface->property("frame").toMap()
                                   .value(QStringLiteral("scrollUnit"))
                                   .toString();
    if (scrollUnit == QStringLiteral("rows"))
        return row.value(QStringLiteral("visualRow")).toReal() + fraction;

    const qreal start = row.value(QStringLiteral("offset")).toReal();
    qreal end = row.value(QStringLiteral("endOffset")).toReal();
    if (end <= start && index + 1 < rows.size()) {
        end = rows.at(index + 1).toMap()
                  .value(QStringLiteral("offset")).toReal();
    }
    return start + (end - start) * fraction;
}

QQuickItem *findEditorCursor(QObject *root)
{
    const auto items = root->findChildren<QQuickItem *>();
    for (QQuickItem *item : items) {
        if (item->property("windowRow").isValid())
            return item;
    }
    return nullptr;
}

struct DocumentFixture {
    TestShell shell;
    TestGallery gallery;
    TestIcons icons;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;
    QQuickItem *surface = nullptr;
    QQuickItem *list = nullptr;
    QQuickItem *scrollBar = nullptr;

    explicit DocumentFixture(const QVariantMap &scene, int windowHeight = 600)
    {
        shell.setScene(scene);
        engine.addImportPath(QStringLiteral(":"));
        engine.rootContext()->setContextProperty(QStringLiteral("qtShell"),
                                                  &shell);
        engine.rootContext()->setContextProperty(QStringLiteral("qtGallery"),
                                                  &gallery);
        engine.rootContext()->setContextProperty(QStringLiteral("qtIcons"),
                                                  &icons);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4GuiFontFamily"), QStringLiteral("Monaco"));
        engine.rootContext()->setContextProperty(QStringLiteral("f4GuiFontPixelSize"),
                                                  13);
        engine.rootContext()->setContextProperty(QStringLiteral("f4UsesQwk"),
                                                  false);
        DummyQWK::registerTypes(&engine);
        engine.load(QUrl(QStringLiteral("qrc:/F4QtHost/qml/main.qml")));
        if (engine.rootObjects().isEmpty())
            return;

        window = qobject_cast<QQuickWindow *>(engine.rootObjects().constFirst());
        if (!window)
            return;
        window->resize(720, windowHeight);
        window->show();
        window->requestActivate();
        QCoreApplication::processEvents();
        surface = window->findChild<QQuickItem *>(QStringLiteral("documentSurface"));
        if (!surface)
            return;
        list = surface->findChild<QQuickItem *>(QStringLiteral("documentList"));
        scrollBar = surface->findChild<QQuickItem *>(
            QStringLiteral("documentScrollBar"));
    }

    bool ready() const
    {
        return window && surface && list && scrollBar;
    }
};
}

class F4DocumentSurfaceTests final : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();
    void fractionalPixelWheelCoalescesUntilAckAndPreservesAnchor();
    void activeFlickRebasesAtomicallyAcrossWindowAck();
    void activeUpwardEditorFlickKeepsStableSlotsAcrossAck();
    void frameOnlyEditorUpdateDoesNotResetLiveFlick();
    void scrollBarReflectsGlobalExtentAndKnownState();
    void editorScrollBarEndpointMapsLastViewportToRowNinety();
    void editorCursorTracksAbsoluteWindowRowAndVisibility();
    void legacyRowsRemainScrollableWithoutWindowProtocol();
};

void F4DocumentSurfaceTests::initTestCase()
{
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    qmlRegisterType<TestGrid>("F4QtHost", 1, 0, "VtuiGridItem");
}

void F4DocumentSurfaceTests::fractionalPixelWheelCoalescesUntilAckAndPreservesAnchor()
{
    QVariantMap frame = viewerFrame(0, 80, 200, 4);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.surface->property("displayedRows").toList().size(),
                              80, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 200.0) < 0.01, 3000);

    fixture.shell.clearActions();
    const qreal initialY = fixture.list->property("contentY").toReal();
    sendPixelWheel(fixture.window, QPoint(300, 300), -13);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.list->property("contentY").toReal()
             - (initialY + 13.0)) < 0.25, 1000);
    QVERIFY(std::fmod(fixture.list->property("contentY").toReal(),
                      fixture.surface->property("rowHeight").toReal()) != 0.0);

    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("action")),
             QStringLiteral("viewer.scrollWindow"));
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("offset")).toInt(),
             206);
    QVERIFY(fixture.surface->property("windowRequestPending").toBool());

    // More pixel input remains local while the first semantic request is in
    // flight. It must neither be dropped nor create a second request.
    sendPixelWheel(fixture.window, QPoint(300, 300), -7);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 210.0) < 0.01, 1000);
    QVERIFY(fixture.surface->property("wheelGestureActive").toBool());
    QTest::qWait(60);
    QCOMPARE(fixture.shell.actions.size(), 1);
    const qreal physicalYBeforeAck =
        fixture.list->property("contentY").toReal();
    QObject *rowModel = fixture.surface->findChild<QObject *>(
        QStringLiteral("documentRowsModel"));
    QVERIFY(rowModel);
    const int poolCountBeforeAck = rowModel->property("count").toInt();

    // The acknowledgement replaces the bounded row window. Because both
    // windows contain extent 210, the visible fractional anchor stays fixed
    // even though local contentY is rebased from one model origin to another.
    frame = viewerFrame(80, 80, 200, 5);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("windowRequestPending").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 210.0) < 0.01, 3000);
    QCOMPARE(fixture.list->property("contentY").toReal(),
             physicalYBeforeAck);
    QCOMPARE(rowModel->property("count").toInt(), poolCountBeforeAck);
    QCOMPARE(fixture.surface->property("displayedRows").toList()
                 .constFirst().toMap().value(QStringLiteral("offset")).toInt(),
             0);
    // Once the pixel gesture has been idle for 180 ms, compaction may safely
    // recenter local coordinates. The same global fractional anchor remains.
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("wheelGestureActive").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("displayedRows").toList().constFirst().toMap()
                .value(QStringLiteral("offset")).toInt() == 80,
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 210.0) < 0.01, 1000);
    QTest::qWait(80);
    // The 7 px accumulated while generation 5 was pending is committed only
    // after that ACK, as a single follow-up request rather than being lost.
    QCOMPARE(fixture.shell.actions.size(), 2);
    QCOMPARE(fixture.shell.actions.constLast()
                 .value(QStringLiteral("offset")).toInt(), 210);
    QCOMPARE(fixture.shell.actions.constLast()
                 .value(QStringLiteral("generation")).toInt(), 6);
}

void F4DocumentSurfaceTests::activeFlickRebasesAtomicallyAcrossWindowAck()
{
    QVariantMap frame = viewerFrame(0, 120, 400, 4, 10, 4000);
    DocumentFixture fixture(documentScene(frame), 400);
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 400.0) < 0.01,
        3000);

    QSignalSpy flickStarted(fixture.list, SIGNAL(flickStarted()));
    QSignalSpy flickEnded(fixture.list, SIGNAL(flickEnded()));
    QSignalSpy movementEnded(fixture.list, SIGNAL(movementEnded()));
    QVERIFY(flickStarted.isValid());
    QVERIFY(flickEnded.isValid());
    QVERIFY(movementEnded.isValid());

    QVERIFY(QMetaObject::invokeMethod(fixture.list, "flick",
                                      Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, -900.0)));
    QTRY_VERIFY_WITH_TIMEOUT(fixture.list->property("flicking").toBool(),
                             1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.list->property("verticalVelocity").toReal()) > 100.0,
        1000);

    flickStarted.clear();
    flickEnded.clear();
    movementEnded.clear();
    const qreal extentBefore = topExtent(fixture.surface, fixture.list);
    const qreal contentYBefore = fixture.list->property("contentY").toReal();
    const qreal velocityBefore =
        fixture.list->property("verticalVelocity").toReal();
    QObject *rowModel = fixture.surface->findChild<QObject *>(
        QStringLiteral("documentRowsModel"));
    QVERIFY(rowModel);
    const int poolCountBefore = rowModel->property("count").toInt();
    QVERIFY(poolCountBefore >= 120);

    // Model a request that is already in flight.  The replacement window
    // overlaps the live top anchor but has a different local origin.
    fixture.surface->setProperty("windowRequestPending", true);
    fixture.surface->setProperty("requestedExtent", extentBefore);
    fixture.surface->setProperty("requestedFraction",
                                 extentBefore / 10.0
                                     - std::floor(extentBefore / 10.0));
    fixture.surface->setProperty("requestedGeneration", 5);
    fixture.surface->setProperty("resumeVelocity", velocityBefore);
    fixture.surface->setProperty("requestPreservesLiveAnchor", true);

    struct FrameSample {
        qint64 timeMs = 0;
        qreal extent = 0;
        qreal contentY = 0;
        qreal velocity = 0;
        bool flicking = false;
    };
    QVector<FrameSample> samples;
    QElapsedTimer clock;
    clock.start();
    frame = viewerFrame(200, 120, qFloor(extentBefore), 5, 10, 4000);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow",
                                      Qt::DirectConnection));

    // The newly loaded rows occupy already allocated slots. The logical top
    // row, physical contentY, fractional pixel anchor and native kinetic
    // timeline are all unchanged in the same event-loop turn.
    const qreal extentAfter = topExtent(fixture.surface, fixture.list);
    const qreal contentYAfter = fixture.list->property("contentY").toReal();
    const qreal velocityAfter =
        fixture.list->property("verticalVelocity").toReal();
    QVERIFY(qAbs(extentAfter - extentBefore) < 0.01);
    QCOMPARE(contentYAfter, contentYBefore);
    QVERIFY(fixture.list->property("moving").toBool());
    QVERIFY(fixture.list->property("flicking").toBool());
    QVERIFY(qAbs(velocityAfter - velocityBefore) < 0.01);
    QCOMPARE(flickStarted.size(), 0);
    QCOMPARE(flickEnded.size(), 0);
    QCOMPARE(movementEnded.size(), 0);
    QVERIFY(!fixture.surface->property("windowRequestPending").toBool());
    QCOMPARE(rowModel->property("count").toInt(), poolCountBefore);
    QVERIFY(fixture.surface->property("loadedSlotStart").toInt() >= 0);
    QVERIFY(fixture.surface->property("loadedSlotEnd").toInt()
            <= poolCountBefore);
    const int visibleRows = qCeil(fixture.list->height()
                                  / fixture.surface->property("rowHeight")
                                        .toReal());
    const int liveRowDelegates =
        fixture.surface->property("liveRowDelegateCount").toInt();
    QVERIFY(liveRowDelegates > 0);
    QVERIFY2(liveRowDelegates <= visibleRows + 10,
             qPrintable(QStringLiteral("pool materialized %1 delegates for %2 visible rows")
                            .arg(liveRowDelegates).arg(visibleRows)));

    for (int frameIndex = 0; frameIndex < 5; ++frameIndex) {
        fixture.window->update();
        QTest::qWait(16);
        samples.append(FrameSample{
            clock.elapsed(),
            topExtent(fixture.surface, fixture.list),
            fixture.list->property("contentY").toReal(),
            fixture.list->property("verticalVelocity").toReal(),
            fixture.list->property("flicking").toBool(),
        });
    }
    QCOMPARE(samples.size(), 5);

    // Sample actual presented frames.  No frame may expose the new window at
    // contentY == 0 (a roughly 200-byte backward jump here), and velocity may
    // only evolve through the original Flickable deceleration.
    qreal previousExtent = extentAfter;
    qreal previousContentY = contentYAfter;
    qreal previousSpeed = qAbs(velocityAfter);
    for (const FrameSample &sample : std::as_const(samples)) {
        QVERIFY(sample.flicking);
        QVERIFY2(sample.extent + 0.05 >= previousExtent,
                 qPrintable(QStringLiteral("non-monotonic frame: %1 -> %2")
                                .arg(previousExtent).arg(sample.extent)));
        QVERIFY2(sample.extent - previousExtent < 30.0,
                 qPrintable(QStringLiteral("discontinuous frame: %1 -> %2")
                                .arg(previousExtent).arg(sample.extent)));
        QVERIFY(sample.contentY + 0.05 >= previousContentY);
        QVERIFY(sample.velocity * velocityAfter > 0);
        QVERIFY(qAbs(sample.velocity) <= previousSpeed + 10.0);
        previousExtent = sample.extent;
        previousContentY = sample.contentY;
        previousSpeed = qAbs(sample.velocity);
    }
    QCOMPARE(flickStarted.size(), 0);
    QCOMPARE(flickEnded.size(), 0);
    QCOMPARE(movementEnded.size(), 0);
}

void F4DocumentSurfaceTests::activeUpwardEditorFlickKeepsStableSlotsAcrossAck()
{
    QVariantMap frame = editorFrame(40, 90, 70, 8);
    DocumentFixture fixture(documentScene(frame), 400);
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 70.0) < 0.01,
        3000);

    QVERIFY(QMetaObject::invokeMethod(fixture.list, "flick",
                                      Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, 900.0)));
    QTRY_VERIFY_WITH_TIMEOUT(fixture.list->property("flicking").toBool(),
                             1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("verticalVelocity").toReal() < -100.0,
        1000);

    QSignalSpy flickEnded(fixture.list, SIGNAL(flickEnded()));
    QSignalSpy movementEnded(fixture.list, SIGNAL(movementEnded()));
    const qreal extentBefore = topExtent(fixture.surface, fixture.list);
    const qreal contentYBefore = fixture.list->property("contentY").toReal();
    const qreal velocityBefore =
        fixture.list->property("verticalVelocity").toReal();
    fixture.surface->setProperty("windowRequestPending", true);
    fixture.surface->setProperty("requestedExtent", extentBefore);
    fixture.surface->setProperty("requestedFraction",
                                 extentBefore - std::floor(extentBefore));
    fixture.surface->setProperty("requestedGeneration", 9);
    fixture.surface->setProperty("resumeVelocity", velocityBefore);
    fixture.surface->setProperty("requestPreservesLiveAnchor", true);

    frame = editorFrame(10, 90, qFloor(extentBefore), 9);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow",
                                      Qt::DirectConnection));

    QVERIFY(qAbs(topExtent(fixture.surface, fixture.list) - extentBefore)
            < 0.01);
    QCOMPARE(fixture.list->property("contentY").toReal(), contentYBefore);
    QCOMPARE(fixture.list->property("verticalVelocity").toReal(),
             velocityBefore);
    QVERIFY(fixture.list->property("moving").toBool());
    QVERIFY(fixture.list->property("flicking").toBool());
    QCOMPARE(flickEnded.size(), 0);
    QCOMPARE(movementEnded.size(), 0);

    QCOMPARE(flickEnded.size(), 0);
    QCOMPARE(movementEnded.size(), 0);
}

void F4DocumentSurfaceTests::frameOnlyEditorUpdateDoesNotResetLiveFlick()
{
    QVariantMap frame{
        {QStringLiteral("id"), QStringLiteral("live-editor-test")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("rows"), editorRows(30, 30)},
        {QStringLiteral("windowRows"), editorRows(0, 100)},
        {QStringLiteral("windowStart"), 0},
        {QStringLiteral("windowEnd"), 100},
        {QStringLiteral("viewportStart"), 30},
        {QStringLiteral("viewportSpan"), 30},
        {QStringLiteral("viewportRow"), 30},
        {QStringLiteral("contentExtent"), 200},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("cursorAbsoluteRow"), 32},
        {QStringLiteral("cursorVisualColumn"), 2},
        {QStringLiteral("cursorVisible"), true},
        {QStringLiteral("windowGeneration"), 1},
    };
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    QSignalSpy contentYChanges(fixture.list, SIGNAL(contentYChanged()));
    QVERIFY(contentYChanges.isValid());
    const qreal manualY =
        fixture.surface->property("loadedSlotStart").toInt()
            * fixture.surface->property("rowHeight").toReal()
        + 633.25;
    fixture.list->setProperty("contentY", manualY);
    QCOMPARE(fixture.list->property("contentY").toReal(), manualY);
    QVERIFY(contentYChanges.count() > 0);

    QVERIFY(QMetaObject::invokeMethod(fixture.list, "flick",
                                      Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, -600.0)));
    QTRY_VERIFY_WITH_TIMEOUT(fixture.list->property("flicking").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.list->property("verticalVelocity").toReal()) > 1.0,
        1000);

    fixture.shell.clearActions();
    const qreal contentYBefore = fixture.list->property("contentY").toReal();
    const qreal velocityBefore =
        fixture.list->property("verticalVelocity").toReal();
    const int changesBefore = contentYChanges.count();
    const QString signatureBefore =
        fixture.surface->property("appliedWindowSignature").toString();

    // Cursor-only editor scenes are frequent while a native flick is live.
    // Apply the queued frame synchronously so an animation clock tick cannot
    // disguise a model-replacement jump as ordinary inertial movement.
    frame.insert(QStringLiteral("cursorAbsoluteRow"), 33);
    frame.insert(QStringLiteral("cursorVisualColumn"), 7);
    fixture.shell.setScene(documentScene(frame));
    QCOMPARE(fixture.surface->property("frame").toMap()
                 .value(QStringLiteral("cursorAbsoluteRow")).toInt(),
             33);
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow",
                                      Qt::DirectConnection));

    QCOMPARE(fixture.surface->property("appliedWindowSignature").toString(),
             signatureBefore);
    QCOMPARE(fixture.list->property("contentY").toReal(), contentYBefore);
    QCOMPARE(contentYChanges.count(), changesBefore);
    QVERIFY(fixture.list->property("moving").toBool());
    QVERIFY(fixture.list->property("flicking").toBool());
    QCOMPARE(fixture.list->property("verticalVelocity").toReal(),
             velocityBefore);
    QVERIFY(fixture.shell.actions.isEmpty());

    // Once the event loop advances, inertia must continue from that exact
    // state rather than having been silently cancelled by the frame update.
    QTest::qWait(30);
    QVERIFY(fixture.list->property("moving").toBool());
    QVERIFY(fixture.list->property("flicking").toBool());
    QVERIFY(qAbs(fixture.list->property("verticalVelocity").toReal()) > 1.0);
    QVERIFY(fixture.list->property("contentY").toReal() != contentYBefore);
    QVERIFY(fixture.shell.actions.isEmpty());
}

void F4DocumentSurfaceTests::scrollBarReflectsGlobalExtentAndKnownState()
{
    QVariantMap frame = viewerFrame(1000, 80, 1250, 1, 25, 10000);
    frame.insert(QStringLiteral("viewportSpan"), 750);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.scrollBar->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.scrollBar->property("position").toReal() - 0.125) < 0.002,
        3000);

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const qreal visibleRows = fixture.list->height() / rowHeight;
    const qreal expectedVisibleSpan = visibleRows * 25.0;
    QCOMPARE(visibleRows, 30.0);
    QCOMPARE(expectedVisibleSpan, 750.0);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.scrollBar->property("size").toReal()
             - expectedVisibleSpan / 10000.0) < 0.000001,
        3000);

    // An editor whose background index has not established a total yet must
    // not expose a falsely authoritative thumb.
    frame.insert(QStringLiteral("contentExtentKnown"), false);
    frame.insert(QStringLiteral("windowGeneration"), 2);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(!fixture.scrollBar->isVisible(), 3000);
}

void F4DocumentSurfaceTests::editorScrollBarEndpointMapsLastViewportToRowNinety()
{
    const QVariantMap frame{
        {QStringLiteral("id"), QStringLiteral("editor-endpoint-test")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("rows"), editorRows(90, 10)},
        {QStringLiteral("windowRows"), editorRows(0, 100)},
        {QStringLiteral("windowStart"), 0},
        {QStringLiteral("windowEnd"), 100},
        {QStringLiteral("viewportStart"), 90},
        {QStringLiteral("viewportSpan"), 10},
        {QStringLiteral("viewportRow"), 90},
        {QStringLiteral("contentExtent"), 100},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("cursorAbsoluteRow"), 90},
        {QStringLiteral("cursorVisualColumn"), 0},
        {QStringLiteral("cursorVisible"), true},
        {QStringLiteral("windowGeneration"), 1},
    };
    DocumentFixture fixture(documentScene(frame), 200);
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QCOMPARE(fixture.list->height()
                 / fixture.surface->property("rowHeight").toReal(),
             10.0);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.scrollBar->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 90.0) < 0.000001,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.scrollBar->property("position").toReal() - 0.9)
            < 0.000001,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.scrollBar->property("size").toReal() - 0.1)
            < 0.000001,
        3000);
    QCOMPARE(fixture.scrollBar->property("position").toReal()
                 + fixture.scrollBar->property("size").toReal(),
             1.0);
}

void F4DocumentSurfaceTests::editorCursorTracksAbsoluteWindowRowAndVisibility()
{
    QVariantMap frame{
        {QStringLiteral("id"), QStringLiteral("editor-under-test")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("rows"), editorRows(50, 30)},
        {QStringLiteral("windowRows"), editorRows(40, 80)},
        {QStringLiteral("windowStart"), 40},
        {QStringLiteral("windowEnd"), 120},
        {QStringLiteral("viewportStart"), 50},
        {QStringLiteral("viewportSpan"), 30},
        {QStringLiteral("viewportRow"), 10},
        {QStringLiteral("contentExtent"), 500},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("cursorAbsoluteRow"), 52},
        {QStringLiteral("cursorVisualColumn"), 4},
        {QStringLiteral("cursorShape"), QStringLiteral("block")},
        {QStringLiteral("cursorVisible"), true},
        {QStringLiteral("windowGeneration"), 1},
    };
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    QQuickItem *cursor = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT((cursor = findEditorCursor(fixture.surface)) != nullptr,
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->isVisible(), 3000);
    QCOMPARE(cursor->property("windowRow").toInt(), 12);
    const qreal cursorInViewport = cursor->mapToItem(fixture.list, 0, 0).y();
    QVERIFY(cursorInViewport >= 0);
    QVERIFY(cursorInViewport < fixture.list->height());

    frame.insert(QStringLiteral("cursorVisible"), false);
    frame.insert(QStringLiteral("windowGeneration"), 2);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(!cursor->isVisible(), 3000);

    frame.insert(QStringLiteral("cursorVisible"), true);
    frame.insert(QStringLiteral("cursorAbsoluteRow"), 150);
    frame.insert(QStringLiteral("windowGeneration"), 3);
    fixture.shell.setScene(documentScene(frame));
    QTRY_COMPARE_WITH_TIMEOUT(cursor->property("windowRow").toInt(), -1, 3000);
    QVERIFY(!cursor->isVisible());
}

void F4DocumentSurfaceTests::legacyRowsRemainScrollableWithoutWindowProtocol()
{
    QVariantList rows;
    for (int index = 0; index < 50; ++index) {
        rows.append(QVariantMap{
            {QStringLiteral("text"), QStringLiteral("legacy %1").arg(index)},
        });
    }
    const QVariantMap legacyFrame{
        {QStringLiteral("id"), QStringLiteral("legacy-viewer")},
        {QStringLiteral("kind"), QStringLiteral("viewer")},
        {QStringLiteral("rows"), rows},
    };
    const QVariantMap legacyScene{
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("newTab"), QVariantMap{}},
             {QStringLiteral("counter"), QVariantMap{}},
         }},
        {QStringLiteral("frames"), QVariantList{legacyFrame}},
    };

    DocumentFixture fixture(legacyScene);
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QVERIFY(!fixture.surface->property("hasWindowProtocol").toBool());
    QCOMPARE(fixture.surface->property("displayedRows").toList().size(), 50);
    QVERIFY(!fixture.scrollBar->isVisible());

    fixture.shell.clearActions();
    const qreal initialY = fixture.list->property("contentY").toReal();
    sendPixelWheel(fixture.window, QPoint(300, 300), -13);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.list->property("contentY").toReal()
             - (initialY + 13.0)) < 0.25, 1000);
    QTest::qWait(260);
    QVERIFY(fixture.shell.actions.isEmpty());
}

QTEST_MAIN(F4DocumentSurfaceTests)

#include "F4DocumentSurfaceTests.moc"
