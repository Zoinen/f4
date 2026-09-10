#include "DummyQWK.h"
#include "TestExtUiStateController.h"

#include <QCoreApplication>
#include <QElapsedTimer>
#include <QFile>
#include <QFontDatabase>
#include <QGuiApplication>
#include <QJsonDocument>
#include <QJsonObject>
#include <QPointF>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QStyleHints>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>
#include <QWheelEvent>
#include <QtQml>
#include <QtTest>

#include <cmath>
#include <atomic>
#include <limits>

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
    Q_PROPERTY(bool renderingEnabled READ renderingEnabled
               WRITE setRenderingEnabled)

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
    bool renderingEnabled() const { return m_renderingEnabled; }
    void setRenderingEnabled(bool enabled) { m_renderingEnabled = enabled; }

    Q_INVOKABLE void sendQtKey(int, const QString &, bool, int) {}
    Q_INVOKABLE void sendClipboardPaste() {}
    Q_INVOKABLE void sendQtText(const QString &) {}

signals:
    void keyboardActivity();
    void edgeNavigationAboutToForward(int key);

private:
    QObject *m_controller = nullptr;
    QString m_fontFamily;
    int m_fontPixelSize = 13;
    bool m_pointerInputEnabled = false;
    bool m_inputMethodForwardingEnabled = false;
    bool m_terminalInputEnabled = true;
    bool m_renderingEnabled = true;
};

class TestShell final : public TestExtUiStateController
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)

public:
    int initialCols() const { return 90; }
    int initialRows() const { return 30; }

    void setScene(const QVariantMap &scene)
    {
        applyScene(scene);
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
    void uiActionSent(const QVariantMap &action);
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
    Q_PROPERTY(bool benchmarkTraceEnabled READ benchmarkTraceEnabled CONSTANT)

public:
    bool available() const { return false; }
    QObject *nullObject() const { return nullptr; }
    bool viewerVisible() const { return false; }
    int viewerSide() const { return 0; }
    QUrl emptyUrl() const { return {}; }
    bool benchmarkTraceEnabled() const
    { return qEnvironmentVariableIntValue("F4_NAV_BENCHMARK_TRACE") != 0; }
    Q_INVOKABLE void recordDocumentWindowCommit(const QVariantMap &window)
    {
        qInfo() << "document transaction kind/prepare/rowsAndPlacement/finish ms"
                << window.value("kind") << window.value("prepareMs")
                << window.value("rowsAndPlacementMs") << window.value("finishMs");
    }

    Q_INVOKABLE QObject *sessionForSide(int) const { return nullptr; }
    Q_INVOKABLE void setScrollingMouseCursor(bool scrollingMode,
                                              int direction = 0,
                                              qreal devicePixelRatio = 0)
    {
        scrollingCursorRequests.append(QVariantMap{
            {QStringLiteral("scrollingMode"), scrollingMode},
            {QStringLiteral("direction"), direction},
            {QStringLiteral("devicePixelRatio"), devicePixelRatio},
        });
    }
    void clearScrollingCursorRequests() { scrollingCursorRequests.clear(); }
    Q_INVOKABLE void closeViewer() {}

    QVector<QVariantMap> scrollingCursorRequests;
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
        return QUrl(QStringLiteral(
            "qrc:/F4QtHost/icons/lucide/file-code.svg"));
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
            {QStringLiteral("offset"), visualRow * 500 + 37},
            {QStringLiteral("endOffset"), visualRow * 500 + 137},
            {QStringLiteral("contentKey"),
             QStringLiteral("editor-row-%1-v1").arg(visualRow)},
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
        {QStringLiteral("defaultBackground"), QStringLiteral("#242424")},
        {QStringLiteral("iconColor"), QStringLiteral("#8AE234")},
        {QStringLiteral("topBarLeft"), QStringLiteral(" window.txt")},
        {QStringLiteral("topBarRight"), QStringLiteral(" UTF-8 │ Text │ 42%     ")},
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
        {QStringLiteral("iconColor"), QStringLiteral("#8AE234")},
        {QStringLiteral("topBarLeft"), QStringLiteral(" window.txt")},
        {QStringLiteral("topBarRight"), QStringLiteral(" UTF-8 │ 41,2     ")},
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

QVariantMap terminalFrame(int firstRow, int count, int viewportStart,
                          int generation, int contentExtent = 50'000,
                          bool followTail = false)
{
    const QVariantList rows = editorRows(firstRow, count);
    return {
        {QStringLiteral("id"), QStringLiteral("terminal-window-test")},
        {QStringLiteral("kind"), QStringLiteral("terminal")},
        {QStringLiteral("defaultBackground"), QStringLiteral("#000000")},
        {QStringLiteral("columns"), 90},
        {QStringLiteral("followTail"), followTail},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("scrollAction"), QStringLiteral("terminal.scroll")},
        {QStringLiteral("selectionEnabled"), true},
        {QStringLiteral("windowRows"), rows},
        {QStringLiteral("windowStart"), firstRow},
        {QStringLiteral("windowEnd"), firstRow + count},
        {QStringLiteral("viewportStart"), viewportStart},
        {QStringLiteral("viewportSpan"), 20},
        {QStringLiteral("viewportRow"),
         qMax(0, viewportStart - firstRow)},
        {QStringLiteral("viewportRows"), 20},
        {QStringLiteral("contentExtent"), contentExtent},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("cursorAbsoluteRow"), viewportStart + 2},
        {QStringLiteral("cursorX"), 3},
        {QStringLiteral("cursorShape"), QStringLiteral("block")},
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
    QVariant state;
    if (QMetaObject::invokeMethod(surface, "topState", Qt::DirectConnection,
                                  Q_RETURN_ARG(QVariant, state))) {
        const QVariantMap mapped = state.toMap();
        if (mapped.contains(QStringLiteral("extent")))
            return mapped.value(QStringLiteral("extent")).toReal();
    }
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
            QStringLiteral("f4GuiFontFamily"),
#if defined(Q_OS_WIN)
            QStringLiteral("Consolas"));
#else
            QStringLiteral("Monaco"));
#endif
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
    void documentLeavesStayOnPhysicalPixelGridAt175Percent();
    void documentMarkupIsAlwaysLiteral();
    void denseDocumentOpenPresentationTiming();
    void realDocumentWindowPresentationTiming();
    void realEditorSelectionKeepsCurrentStyledRowsAfterViewer();
    void realEditorFirstViewportStartsOnExactRowBoundary();
    void nativeDocumentSlotsKeepNestedValuesAndBatchNotifications();
    void editorCursorFollowsActiveTheme();
    void shortEditorSelectionDoesNotScroll();
    void nativeDocumentModelLifetimeMatchesPhysicalPool();
    void standaloneMetadataDoesNotCarryRowPayload();
    void sameDocumentWindowsDoNotResetShellInteraction();
    void openingDocumentKeepsHiddenPanelPresentationStable();
    void sameCountStyledRunsRetainVisualObjects();
    void streamSelectionStateKeepsBaseRowsAndSuffixPixelsStable();
    void nativeStyledRunMutationIgnoresStaleContentKey();
    void recenteredWindowReplacesEachSlotOnlyOnce();
    void scrollbarLatestIntentAndLayoutEpochCommitAtomically();
    void homeEndRetiresQueuedScrollIntentBeforeForwarding();
    void homeEndKeepsPendingScrollWhenOverlayOwnsInput();
    void acknowledgedViewportMayClampToZero();
    void standaloneViewportIsNegotiatedBeforeOpening();
    void styledDocumentRunsAreVisible_data();
    void styledDocumentRunsAreVisible();
    void compactDocumentUpdatesKeepViewportActive();
    void closingDocumentDoesNotReprojectItsRows();
    void unrelatedStreamsDoNotReprojectDocumentRows();
    void openingDocumentHasNoStaleOrUnpositionedFrame_data();
    void openingDocumentHasNoStaleOrUnpositionedFrame();
    void documentSurfaceDoesNotPaintItsOwnBackdrop();
    void documentHeaderShowsFullPathsForViewerAndEditor();
    void nativeViewportExcludesHeaderAndKeepsBottomCursorVisible();
    void standaloneDocumentsEndAtSharedKeyBarSeparator();
    void finalViewportAlignsLastRowBelowFractionalBottom();
    void middleButtonAutoScrollsStandaloneDocuments_data();
    void middleButtonAutoScrollsStandaloneDocuments();
    void middleButtonAutoScrollsEmbeddedTerminal();
    void editorPointerEventsAreForwardedAsSemanticMouseActions();
    void editorEdgeSelectionUsesCommittedSourceFragments();
    void fractionalPixelWheelCoalescesUntilAckAndPreservesAnchor();
    void activeFlickRebasesAtomicallyAcrossWindowAck();
    void activeUpwardEditorFlickKeepsStableSlotsAcrossAck();
    void frameOnlyEditorUpdateDoesNotResetLiveFlick();
    void overlappingEditorUpdateTouchesOnlyChangedSlots();
    void scrollBarReflectsGlobalExtentAndKnownState();
    void editorScrollBarEndpointMapsLastViewportToRowNinety();
    void editorCursorTracksAbsoluteWindowRowAndVisibility();
    void documentCursorBlinkSettles_data();
    void documentCursorBlinkSettles();
    void terminalScrollbackUsesBoundedWindowAndNativeViewport();
    void terminalFractionalRestDoesNotRequestCurrentRow();
    void terminalCompleteWindowDoesNotPrefetchBeforeContentStart();
    void viewerFractionalRestDoesNotRequestCurrentOffset();
    void terminalFollowTailTracksVisibleEndAndUserScroll();
    void terminalDragSelectionSendsAbsoluteClipboardRange();
    void terminalDragSelectionAutoScrollsBeyondViewportByDistance();
    void terminalSelectionUsesNearestInsertionBoundary();
    void terminalDoubleAndTripleClickSelectWordAndParagraph();
    void legacyRowsRemainScrollableWithoutWindowProtocol();
};

void F4DocumentSurfaceTests::documentLeavesStayOnPhysicalPixelGridAt175Percent()
{
    if (qAbs(qGuiApp->devicePixelRatio() - 1.75) > 0.01)
        QSKIP("Run this regression with QT_SCALE_FACTOR=1.75");
    QVariantMap frame = editorFrame(0, 60, 0, 1);
    QVariantList rows = frame.value("windowRows").toList();
    QVariantMap first = rows[0].toMap();
    first.insert("runs", QVariantList{
        QVariantMap{{"text", "abc"}, {"foreground", "#39dc85"}},
        QVariantMap{{"text", " secondary"}, {"foreground", "#ffffff"}},
    });
    first.insert("visualWidth", 13);
    rows[0] = first;
    frame.insert("windowRows", rows);
    frame.insert("selection", true);
    frame.insert("selectionAnchorRow", 0);
    frame.insert("selectionAnchorColumn", 0);
    frame.insert("cursorAbsoluteRow", 0);
    frame.insert("cursorAbsoluteColumn", 3);
    frame.insert("selectionForeground", "#ffffff");
    frame.insert("selectionBackground", "#3b6290");
    frame.insert("selectionBold", false);
    frame.insert("selectionUnderline", false);
    frame.insert("selectionStrikeout", false);
    frame.insert("secondaryCarets", QVariantList{
        QVariantMap{{"cursorAbsoluteRow", 0}, {"cursorAbsoluteColumn", 12},
                    {"selection", true}, {"selectionAnchorRow", 0}, {"selectionAnchorColumn", 5}},
    });
    auto scene = documentScene(frame);
    // Include the production menu lane so the titlebar reserves its height;
    // otherwise its empty mock background covers the document header capture.
    scene.insert("menuBar", QVariantMap{{"items", QVariantList{}}});
    DocumentFixture fixture(scene, 599);
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    QTest::qWait(80);
    int textLeaves = 0;
    QList<QQuickItem *> pending{fixture.surface};
    while (!pending.isEmpty()) {
        auto *item = pending.takeLast();
        pending.append(item->childItems());
        if (!item->isVisible())
            continue;
        if (item->objectName() == "editorSecondaryCursor-0") {
            const auto origin = item->mapToItem(fixture.window->contentItem(), QPointF()) * fixture.window->devicePixelRatio();
            const qreal physicalWidth = item->width() * fixture.window->devicePixelRatio();
            const qreal physicalHeight = item->height() * fixture.window->devicePixelRatio();
            qInfo() << "secondary caret" << origin << physicalWidth << physicalHeight;
            QVERIFY(qAbs(origin.x() - qRound64(origin.x())) < .001 && qAbs(origin.y() - qRound64(origin.y())) < .001);
            QVERIFY(qAbs(physicalWidth - qRound64(physicalWidth)) < .001 && qAbs(physicalHeight - qRound64(physicalHeight)) < .001);
        }
        const QByteArray type = item->metaObject()->className();
        if (!type.startsWith("QQuickText") && !type.contains("Image")
            && item->objectName() != "documentHeaderLucideIcon")
            continue;
        const QPointF origin = item->mapToItem(fixture.window->contentItem(), QPointF());
        const qreal dpr = fixture.window->devicePixelRatio();
        const QPointF physical = origin * dpr;
        qInfo() << "document leaf" << item->objectName() << type << physical;
        QVERIFY2(!item->objectName().isEmpty(), "Every document text/image leaf needs an objectName");
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < 0.001
                     && qAbs(physical.y() - qRound64(physical.y())) < 0.001,
                 qPrintable(QString("%1 physical origin (%2,%3)")
                                .arg(item->objectName()).arg(physical.x()).arg(physical.y())));
        const QPointF unitX = item->mapToItem(fixture.window->contentItem(), QPointF(1, 0)) - origin;
        const QPointF unitY = item->mapToItem(fixture.window->contentItem(), QPointF(0, 1)) - origin;
        QVERIFY(qAbs(unitX.x() - 1) < 0.001 && qAbs(unitX.y()) < 0.001);
        QVERIFY(qAbs(unitY.y() - 1) < 0.001 && qAbs(unitY.x()) < 0.001);
        if (item->objectName() == "documentRunText"
            && !item->property("text").toString().isEmpty()) {
            const qreal advance = item->implicitWidth() / item->property("text").toString().size();
            QVERIFY(qAbs(advance - fixture.surface->property("terminalCellWidth").toReal()) < .05);
        }
        ++textLeaves;
    }
    QVERIFY(textLeaves > 4);
    // A fractional ancestor move must update every actual text/image leaf,
    // not merely the document container's origin.
    auto *pixelAncestor = fixture.surface->parentItem();
    pixelAncestor->setPosition(pixelAncestor->position() + QPointF(.17, .29));
    QTest::qWait(30);
    QList<QQuickItem *> movedItems{fixture.surface};
    while (!movedItems.isEmpty()) {
        auto *item = movedItems.takeLast();
        movedItems.append(item->childItems());
        const QByteArray type = item->metaObject()->className();
        if (!item->isVisible() || (!type.startsWith("QQuickText") && !type.contains("Image")
            && item->objectName() != "documentHeaderLucideIcon"))
            continue;
        const QPointF physical = item->mapToItem(fixture.window->contentItem(), QPointF())
                * fixture.window->devicePixelRatio();
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < .001
                     && qAbs(physical.y() - qRound64(physical.y())) < .001,
                 qPrintable(QString("moved %1 origin (%2,%3)")
                                .arg(item->objectName()).arg(physical.x()).arg(physical.y())));
    }
    const QImage capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    const QString capturePath = qEnvironmentVariable("F4_DOCUMENT_PIXEL_CAPTURE");
    if (!capturePath.isEmpty())
        QVERIFY(capture.save(capturePath));
}

void F4DocumentSurfaceTests::documentMarkupIsAlwaysLiteral()
{
    QVariantMap frame = viewerFrame(0, 80, 0, 1);
    frame.insert("topBarLeft", "<b>file.txt</b>");
    QVariantList rows = frame.value("windowRows").toList();
    auto plain = rows[0].toMap();
    plain.insert("text", "<b>literal document markup</b>");
    rows[0] = plain;
    auto styled = rows[1].toMap();
    styled.insert("runs", QVariantList{QVariantMap{{"text", "<i>literal styled markup</i>"}}});
    rows[1] = styled;
    frame.insert("windowRows", rows);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    int checked = 0;
    QList<QQuickItem *> items{fixture.surface};
    while (!items.isEmpty()) {
        auto *item = items.takeLast();
        items.append(item->childItems());
        if (item->objectName() == "documentPlainText" && !item->isVisible())
            QVERIFY2(item->property("text").toString().isEmpty(),
                     "Hidden plain fallback must not duplicate styled text layout");
        if (!item->isVisible() || !item->property("text").toString().contains("<"))
            continue;
        QCOMPARE(item->property("textFormat").toInt(), 0); // QQuickText::PlainText
        ++checked;
    }
    QVERIFY(checked >= 3);
}

void F4DocumentSurfaceTests::nativeDocumentModelLifetimeMatchesPhysicalPool()
{
    DocumentFixture fixture(documentScene(viewerFrame(0, 80, 0, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    const auto models = fixture.surface->findChildren<DocumentRowsModel *>();
    QCOMPARE(models.size(), 1);
    for (int repeat = 0; repeat < 3; ++repeat) {
        QVERIFY(fixture.surface->setProperty("embedded", true));
        QCoreApplication::processEvents();
        QVERIFY(fixture.surface->setProperty("embedded", false));
        QCoreApplication::processEvents();
        QCOMPARE(fixture.surface->findChildren<DocumentRowsModel *>(), models);
    }
}

void F4DocumentSurfaceTests::standaloneMetadataDoesNotCarryRowPayload()
{
    auto frame = viewerFrame(0, 80, 0, 1);
    frame.insert("documentKey", "metadata-only-viewer");
    frame.insert("rows", frame.value("windowRows"));
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    const QVariantMap metadata = fixture.surface->property("frame").toMap();
    QVERIFY(!metadata.contains("rows"));
    QVERIFY(!metadata.contains("windowRows"));
    QCOMPARE(fixture.surface->property("displayedRows").toList(),
             frame.value("windowRows").toList());
    // The authoritative native transaction remains complete for reducer
    // validation. Only presentation metadata crosses scalar QML bindings.
    QCOMPARE(fixture.shell.surfaceRegistry()->document().value("windowRows"),
             frame.value("windowRows"));
    auto *registry = fixture.shell.surfaceRegistry();
    const qulonglong publication = metadata.value("nativeWindowRevision").toULongLong();
    QCOMPARE(registry->documentWindowRows("metadata-only-viewer", publication),
             frame.value("windowRows"));
    QVERIFY(!registry->documentWindowRows("other-document", publication).isValid());
    auto next = frame;
    next.insert("viewportStart", 10);
    registry->applyDocument(next, registry->documentRevision() + 1);
    QVERIFY(!registry->documentWindowRows("metadata-only-viewer", publication).isValid());
    const auto nextMetadata = registry->documentMetadata();
    const auto nextPublication = nextMetadata.value("nativeWindowRevision").toULongLong();
    QVERIFY(nextPublication > publication);
    QCOMPARE(registry->documentWindowRows("metadata-only-viewer", nextPublication),
             frame.value("windowRows"));
}

void F4DocumentSurfaceTests::sameDocumentWindowsDoNotResetShellInteraction()
{
    auto frame = editorFrame(0, 80, 0, 1);
    frame.insert("documentKey", "interaction-document");
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    auto *store = fixture.window->findChild<QObject *>("shellSceneStore");
    QVERIFY(store);
    QSignalSpy reset(store, SIGNAL(sceneReset()));
    QSignalSpy structural(store, SIGNAL(structuralSurfaceUpdated()));
    QVERIFY(reset.isValid());
    QVERIFY(structural.isValid());
    frame.insert("windowGeneration", 2);
    frame.insert("viewportStart", 1);
    fixture.shell.setScene(documentScene(frame));
    QCOMPARE(reset.size(), 0);
    QCOMPARE(structural.size(), 0);
    frame.insert("documentKey", "next-interaction-document");
    fixture.shell.setScene(documentScene(frame));
    QCOMPARE(reset.size(), 0);
    QCOMPARE(structural.size(), 1);
    fixture.shell.setScene(documentScene({}));
    QCOMPARE(reset.size(), 0);
    QCOMPARE(structural.size(), 2);
    // The retained frame deliberately survives close. Reopening that exact
    // document is nevertheless an active-surface transition: focus/gallery
    // coordination must run once without restoring broad sceneReset churn.
    fixture.shell.setScene(documentScene(frame));
    QCOMPARE(reset.size(), 0);
    QCOMPARE(structural.size(), 3);
}

void F4DocumentSurfaceTests::openingDocumentKeepsHiddenPanelPresentationStable()
{
    const auto scene = documentScene(viewerFrame(0, 80, 0, 1));
    const auto shell = scene.value("shell").toMap();
    DocumentFixture fixture(scene);
    QVERIFY(fixture.ready());
    auto *loader = fixture.window->findChild<QQuickItem *>("persistentPanelsLayer");
    QVERIFY(loader);
    auto *panels = qobject_cast<QQuickItem *>(loader->property("item").value<QObject *>());
    QVERIFY(panels);
    QSignalSpy frames(panels, SIGNAL(frameChanged()));
    QVERIFY(frames.isValid());
    auto *registry = fixture.shell.surfaceRegistry();
    registry->applyShell({}, registry->shellRevision() + 1);
    QVERIFY(!registry->hasShell());
    QCOMPARE(panels->property("frame").toMap(), shell);
    // Clearing the active shell changes visibility, not the retained hidden
    // panel presentation. Equal native->retained map rebinding fans out over
    // every panel/control even though no displayed panel data changed.
    QCOMPARE(frames.size(), 0);
    registry->applyShell(shell, registry->shellRevision() + 1);
    QVERIFY(registry->hasShell());
    QCOMPARE(panels->property("frame").toMap(), shell);
    auto changed = shell;
    changed.insert("title", "updated while visible");
    registry->applyShell(changed, registry->shellRevision() + 1);
    QCOMPARE(panels->property("frame").toMap(), changed);
    QVERIFY(frames.size() > 0);
}

void F4DocumentSurfaceTests::sameCountStyledRunsRetainVisualObjects()
{
    auto frame = editorFrame(0, 80, 0, 1);
    frame.insert("documentKey", "stable-run-objects");
    auto rows = frame.value("windowRows").toList();
    auto row = rows.first().toMap();
    row.remove("text");
    QVariantList runs{QVariantMap{{"text", "literal <b>"}, {"foreground", "#15bcad"}},
                      QVariantMap{{"text", " secondary"}, {"foreground", "#eeeeee"}}};
    row.insert("runs", runs);
    rows[0] = row;
    frame.insert("windowRows", rows);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    auto findRun = [&fixture](const QString &text) -> QQuickItem * {
        QList<QQuickItem *> pending{fixture.surface};
        while (!pending.isEmpty()) {
            auto *item = pending.takeLast();
            pending.append(item->childItems());
            if (item->isVisible() && item->objectName() == "documentRunText"
                && item->property("text").toString() == text)
                return item;
        }
        return nullptr;
    };
    QPointer<QQuickItem> original = findRun("literal <b>");
    QVERIFY(original);
    const auto textProperty = original->metaObject()->property(
        original->metaObject()->indexOfProperty("text"));
    QSignalSpy textChanges(original, textProperty.notifySignal());
    QVERIFY(textChanges.isValid());
    auto firstRun = runs[0].toMap();
    firstRun.insert("foreground", "#fa318a");
    runs[0] = firstRun;
    row.insert("runs", runs);
    row.insert("contentKey", "styled-v2");
    rows[0] = row;
    frame.insert("windowRows", rows);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow", Qt::DirectConnection));
    QVERIFY2(original, "same-count style update destroyed the existing Text leaf");
    QCOMPARE(findRun("literal <b>"), original.data());
    QCOMPARE(original->property("color").value<QColor>(), QColor("#fa318a"));
    QCOMPARE(textChanges.size(), 0);
    firstRun.insert("text", "changed literal");
    runs[0] = firstRun;
    row.insert("runs", runs);
    row.insert("contentKey", "styled-v3");
    rows[0] = row;
    frame.insert("windowRows", rows);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow", Qt::DirectConnection));
    QCOMPARE(findRun("changed literal"), original.data());
    QCOMPARE(textChanges.size(), 1);
    runs.append(QVariantMap{{"text", " new third run"}, {"foreground", "#18bade"}});
    row.insert("runs", runs);
    row.insert("contentKey", "styled-v4");
    rows[0] = row;
    frame.insert("windowRows", rows);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow", Qt::DirectConnection));
    QVERIFY(findRun(" new third run"));
    runs = QVariantList{firstRun};
    row.insert("runs", runs);
    row.insert("contentKey", "styled-v5");
    rows[0] = row;
    frame.insert("windowRows", rows);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow", Qt::DirectConnection));
    QVERIFY(findRun("changed literal"));
    QVERIFY(!findRun(" secondary"));
    QVERIFY(!findRun(" new third run"));
}

void F4DocumentSurfaceTests::streamSelectionStateKeepsBaseRowsAndSuffixPixelsStable()
{
    auto frame = editorFrame(0, 60, 0, 1);
    frame.insert("documentKey", "stream-selection-overlay");
    frame.insert("layoutRevision", 4);
    frame.insert("scrollLeft", 2);
    frame.insert("viewportColumns", 32);
    frame.insert("cursorVisible", false); // Keep intentional caret blink out of captures.
    frame.insert("cursorAbsoluteRow", 0);
    frame.insert("cursorAbsoluteColumn", 5);
    frame.insert("cursorVisualColumn", 3);
    frame.insert("selection", true);
    frame.insert("selectionAnchorRow", 0);
    frame.insert("selectionAnchorColumn", 3);
    frame.insert("selectionForeground", "#f8f8f2");
    frame.insert("selectionBackground", "#3b6290");
    frame.insert("selectionBold", false);
    frame.insert("selectionUnderline", false);
    frame.insert("selectionStrikeout", false);
    auto rows = frame.value("windowRows").toList();
    auto first = rows.first().toMap();
    first.remove("text");
    first.insert("runs", QVariantList{
        QVariantMap{{"text", QString::fromUtf8("界45    ")},
                    {"foreground", "#39dc85"}},
        QVariantMap{{"text", "9 suffix-stable-abcdefgh"},
                    {"foreground", "#eeeeee"}},
    });
    first.insert("visualWidth", 30);
    first.insert("contentKey", "immutable-base-row");
    rows[0] = first;
    auto second = rows[1].toMap();
    second.insert("visualWidth", 12);
    rows[1] = second;
    frame.insert("windowRows", rows);

    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    auto findText = [&fixture](const QString &objectName,
                               const QString &text) -> QQuickItem * {
        QList<QQuickItem *> pending{fixture.surface};
        while (!pending.isEmpty()) {
            auto *item = pending.takeLast();
            pending.append(item->childItems());
            if (item->objectName() == objectName
                && item->property("text").toString() == text)
                return item;
        }
        return nullptr;
    };
    auto findVisibleClip = [&fixture](const QString &rowText) -> QQuickItem * {
        QList<QQuickItem *> pending{fixture.surface};
        while (!pending.isEmpty()) {
            auto *candidate = pending.takeLast();
            pending.append(candidate->childItems());
            if (candidate->objectName() == "documentEditorSelectedText"
                && candidate->isVisible()
                && candidate->property("text").toString() == rowText) {
                return candidate->parentItem();
            }
        }
        return nullptr;
    };
    const QString firstRun = QString::fromUtf8("界45    ");
    const QString secondRun = QStringLiteral("9 suffix-stable-abcdefgh");
    const QString completeRow = firstRun + secondRun;
    QTRY_VERIFY(findText("documentRunText", firstRun));
    QTRY_VERIFY(findText("documentRunText", secondRun));
    QTRY_VERIFY(findText("documentEditorSelectedText", completeRow));
    QPointer<QQuickItem> baseFirst = findText("documentRunText", firstRun);
    QPointer<QQuickItem> baseSecond = findText("documentRunText", secondRun);
    QPointer<QQuickItem> selectedText = findText(
        "documentEditorSelectedText", completeRow);
    QQuickItem *clip = selectedText->parentItem();
    QVERIFY(clip && clip->objectName() == "documentEditorSelectionClip");
    QCOMPARE(clip->property("color").value<QColor>(), QColor("#3b6290"));
    QCOMPARE(selectedText->property("color").value<QColor>(), QColor("#f8f8f2"));

    const auto textProperty = baseSecond->metaObject()->property(
        baseSecond->metaObject()->indexOfProperty("text"));
    QSignalSpy suffixTextChanges(baseSecond, textProperty.notifySignal());
    QVERIFY(suffixTextChanges.isValid());
    const auto selectedProperty = selectedText->metaObject()->property(
        selectedText->metaObject()->indexOfProperty("text"));
    QSignalSpy selectedTextChanges(selectedText, selectedProperty.notifySignal());
    QVERIFY(selectedTextChanges.isValid());
    auto *rowItem = baseFirst->parentItem();
    while (rowItem && rowItem->objectName() != "documentRowDelegate")
        rowItem = rowItem->parentItem();
    QVERIFY(rowItem);
    auto captureSuffix = [&]() {
        fixture.window->update();
        QTest::qWait(20);
        const QImage image = fixture.window->grabWindow();
        const qreal dpr = fixture.window->devicePixelRatio();
        const qreal cell = fixture.surface->property("terminalCellWidth").toReal();
        const qreal inset = fixture.surface->property("textHorizontalInset").toReal();
        const QPointF start = rowItem->mapToItem(
            fixture.window->contentItem(), QPointF(inset + 14 * cell, 2));
        const QRect area(qRound(start.x() * dpr), qRound(start.y() * dpr),
                         qMax(1, qRound(10 * cell * dpr)),
                         qMax(1, qRound((rowItem->height() - 4) * dpr)));
        return image.copy(area.intersected(image.rect()));
    };
    const QImage stableSuffix = captureSuffix();
    QVERIFY(!stableSuffix.isNull());
    fixture.surface->setProperty("poolSlotWriteCount", 0);
    QSignalSpy documentChanges(fixture.shell.surfaceRegistry(),
                               &SurfaceRegistry::documentChanged);

    int revision = 2;
    for (const int focusColumn : {6, 4, 8, 1, 5}) {
        frame.insert("cursorAbsoluteColumn", focusColumn);
        frame.insert("cursorVisualColumn", focusColumn - 2);
        fixture.shell.surfaceRegistry()->applyDocumentState(frame, revision++);
        QVariantMap state;
        for (const char *name : {
                 "id", "documentKey", "layoutRevision", "windowGeneration",
                 "cursorLine", "cursorPos", "cursorVisualRow",
                 "cursorVisualColumn", "cursorVisible", "cursorShape",
                 "cursorAbsoluteRow", "cursorAbsoluteColumn", "selection",
                 "selectionAnchorRow", "selectionAnchorColumn",
                 "selectionForeground", "selectionBackground", "selectionBold",
                 "selectionUnderline", "selectionStrikeout", "topBarRight"}) {
            const QString key = QString::fromLatin1(name);
            state.insert(key, frame.value(key));
        }
        emit fixture.shell.compactPresentationChanged({{"surfaceState", state}});
        QCoreApplication::processEvents();
        QTRY_VERIFY(findVisibleClip(completeRow));
        QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), 0);
        QCOMPARE(documentChanges.size(), 0);
        QCOMPARE(findText("documentRunText", firstRun), baseFirst.data());
        QCOMPARE(findText("documentRunText", secondRun), baseSecond.data());
        QCOMPARE(findText("documentEditorSelectedText", completeRow),
                 selectedText.data());
        QCOMPARE(suffixTextChanges.size(), 0);
        QCOMPARE(selectedTextChanges.size(), 0);
        QCOMPARE(captureSuffix(), stableSuffix);
    }

    // Multiline selection uses each row's shared projected visual width; it
    // still changes only overlay geometry and never the row model.
    frame.insert("cursorAbsoluteRow", 1);
    frame.insert("cursorAbsoluteColumn", 4);
    frame.insert("cursorVisualRow", 1);
    frame.insert("cursorVisualColumn", 2);
    fixture.shell.surfaceRegistry()->applyDocumentState(frame, revision++);
    QVariantMap state;
    for (const char *name : {
             "id", "documentKey", "layoutRevision", "windowGeneration",
             "cursorLine", "cursorPos", "cursorVisualRow", "cursorVisualColumn",
             "cursorVisible", "cursorShape", "cursorAbsoluteRow",
             "cursorAbsoluteColumn", "selection", "selectionAnchorRow",
             "selectionAnchorColumn", "selectionForeground",
             "selectionBackground", "selectionBold", "selectionUnderline",
             "selectionStrikeout", "topBarRight"}) {
        const QString key = QString::fromLatin1(name);
        state.insert(key, frame.value(key));
    }
    emit fixture.shell.compactPresentationChanged({{"surfaceState", state}});
    QCoreApplication::processEvents();
    QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), 0);
    QCOMPARE(documentChanges.size(), 0);
    QCOMPARE(findText("documentRunText", firstRun), baseFirst.data());
    QCOMPARE(findText("documentRunText", secondRun), baseSecond.data());
    QCOMPARE(suffixTextChanges.size(), 0);

    // QML keeps the compact overlay fenced even if a legacy embedding emits a
    // stale override directly. Production rejects these before emission too.
    const QList<QPair<QString, QVariant>> staleFences{
        {QStringLiteral("documentKey"), QStringLiteral("other-document")},
        {QStringLiteral("layoutRevision"), 3},
        {QStringLiteral("windowGeneration"), 0},
    };
    for (const auto &[field, value] : staleFences) {
        QVariantMap stale = state;
        stale.insert(field, value);
        stale.insert(QStringLiteral("cursorAbsoluteColumn"), 19);
        emit fixture.shell.compactPresentationChanged({{"surfaceState", stale}});
        QCoreApplication::processEvents();
        QCOMPARE(fixture.surface->property("cursorFrame").toMap()
                     .value(QStringLiteral("cursorAbsoluteColumn")).toInt(), 5);
        emit fixture.shell.compactPresentationChanged({{"surfaceState", state}});
        QCoreApplication::processEvents();
        QCOMPARE(fixture.surface->property("cursorFrame").toMap()
                     .value(QStringLiteral("cursorAbsoluteColumn")).toInt(), 4);
    }

    // An inactive selection is an explicit complete compact state, not an
    // omitted value which could leave the previous overlay alive.
    state.insert(QStringLiteral("selection"), false);
    state.insert(QStringLiteral("selectionAnchorRow"), 0);
    state.insert(QStringLiteral("selectionAnchorColumn"), 0);
    emit fixture.shell.compactPresentationChanged({{"surfaceState", state}});
    QCoreApplication::processEvents();
    QTRY_VERIFY(!clip->isVisible());
    QVERIFY(!fixture.surface->property("cursorFrame").toMap()
                 .value(QStringLiteral("selection")).toBool());
    QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), 0);
    QCOMPARE(documentChanges.size(), 0);
    QCOMPARE(findText("documentRunText", firstRun), baseFirst.data());
    QCOMPARE(findText("documentRunText", secondRun), baseSecond.data());
    QCOMPARE(suffixTextChanges.size(), 0);
    QCOMPARE(captureSuffix(), stableSuffix);
}

void F4DocumentSurfaceTests::nativeStyledRunMutationIgnoresStaleContentKey()
{
    auto frame = editorFrame(0, 80, 0, 1);
    frame.insert("documentKey", "native-styled-row-mutation");
    auto rows = frame.value("windowRows").toList();
    auto row = rows.first().toMap();
    row.remove("text");
    const QString staleContentKey = row.value("contentKey").toString();
    QVariantList runs{QVariantMap{{"text", "selected bytes"},
                                  {"foreground", "#eeeeee"}}};
    row.insert("runs", runs);
    rows[0] = row;
    frame.insert("windowRows", rows);

    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    auto findRun = [&fixture]() -> QQuickItem * {
        QList<QQuickItem *> items{fixture.surface};
        while (!items.isEmpty()) {
            auto *item = items.takeLast();
            items.append(item->childItems());
            if (item->isVisible() && item->objectName() == "documentRunText"
                && item->property("text").toString() == "selected bytes")
                return item;
        }
        return nullptr;
    };
    QPointer<QQuickItem> runLabel = findRun();
    QVERIFY(runLabel);
    auto *runBackground = runLabel->parentItem();
    QVERIFY(runBackground);
    QCOMPARE(runBackground->property("color").value<QColor>(),
             QColor(Qt::transparent));

    // contentKey is an optimization hint supplied by another process. A
    // stale hint must never suppress authoritative nested run/style data.
    auto selectedRun = runs.first().toMap();
    selectedRun.insert("background", "#3b6290");
    runs[0] = selectedRun;
    row.insert("runs", runs);
    QCOMPARE(row.value("contentKey").toString(), staleContentKey);
    rows[0] = row;
    frame.insert("windowRows", rows);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow",
                                      Qt::DirectConnection));

    QVERIFY2(runLabel, "style mutation destroyed the stable run Text leaf");
    QCOMPARE(findRun(), runLabel.data());
    QCOMPARE(runBackground->property("color").value<QColor>(),
             QColor("#3b6290"));
}

void F4DocumentSurfaceTests::editorCursorFollowsActiveTheme()
{
    DocumentFixture fixture(documentScene(editorFrame(0, 60, 0, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    auto *cursor = findEditorCursor(fixture.surface);
    QVERIFY(cursor);
    auto *palette = fixture.window->property("themePaletteObject").value<QObject *>();
    QVERIFY(palette);
    QVERIFY(palette->setProperty("textColor", QColor("#14d5a9")));
    QTRY_COMPARE(cursor->property("color").value<QColor>(), QColor("#14d5a9"));
    QVERIFY(palette->setProperty("textColor", QColor("#f160a8")));
    QTRY_COMPARE(cursor->property("color").value<QColor>(), QColor("#f160a8"));
}

void F4DocumentSurfaceTests::nativeDocumentSlotsKeepNestedValuesAndBatchNotifications()
{
    DocumentRowsModel model;
    QSignalSpy inserted(&model, &QAbstractItemModel::rowsInserted);
    QSignalSpy changed(&model, &QAbstractItemModel::dataChanged);
    model.ensureCapacity(120);
    QCOMPARE(model.rowCount(), 120);
    QCOMPARE(inserted.size(), 1);
    model.ensureCapacity(120);
    QCOMPARE(inserted.size(), 1);
    const QVariantMap row{{"offset", 123}, {"runs", QVariantList{
        QVariantMap{{"text", "literal <b> & script"}, {"foreground", "#13579b"}}}}};
    const QVariantMap slot{{"loaded", true}, {"rowData", row}};
    model.set(10, slot);
    model.set(12, slot);
    QCOMPARE(changed.size(), 0);
    QCOMPARE(model.data(model.index(10), DocumentRowsModel::RowDataRole).toMap(), row);
    model.commit();
    QCOMPARE(changed.size(), 2);
    QCOMPARE(changed.first()[0].value<QModelIndex>().row(), 10);
    QCOMPARE(changed.first()[1].value<QModelIndex>().row(), 10);
    QCOMPARE(changed.last()[0].value<QModelIndex>().row(), 12);
    QCOMPARE(changed.last()[1].value<QModelIndex>().row(), 12);
    model.set(10, slot);
    model.commit();
    QCOMPARE(changed.size(), 2);
    QCOMPARE(model.get(10), slot);
}

void F4DocumentSurfaceTests::realDocumentWindowPresentationTiming()
{
    const QString fixturePath = qEnvironmentVariable("F4_DOCUMENT_TIMING_FIXTURE");
    if (fixturePath.isEmpty())
        QSKIP("Set F4_DOCUMENT_TIMING_FIXTURE to an exported semantic document window");
    QFile input(fixturePath);
    QVERIFY(input.open(QIODevice::ReadOnly));
    const QVariantMap source = QJsonDocument::fromJson(input.readAll()).object().toVariantMap();
    QVERIFY(!source.value("windowRows").toList().isEmpty());
    DocumentFixture fixture(documentScene({}), 1186);
    QVERIFY(fixture.window);
    fixture.window->resize(2208, 1186);
    QTRY_VERIFY((fixture.surface = fixture.window->findChild<QQuickItem *>("documentSurface")));
    QTRY_COMPARE(fixture.surface->property("reportedViewportColumns").toInt(),
        int(fixture.surface->property("prospectiveViewportWidth").toDouble()
            / fixture.surface->property("terminalCellWidth").toDouble()));
    for (int cycle = 0; cycle < 6; ++cycle) {
        auto frame = source;
        const QString key = QString("real-document-%1").arg(cycle);
        frame.insert("documentKey", key);
        frame.insert("geometryRevision", fixture.surface->property("geometryRevision"));
        bool swapped = false;
        QElapsedTimer timer;
        std::atomic<qint64> animatedNs{0}, synchronizedNs{0};
        const auto animatedConnection = QObject::connect(fixture.window, &QQuickWindow::afterAnimating,
            fixture.window, [&] {
                if (!animatedNs.load() && fixture.surface->property("appliedDocumentKey").toString() == key)
                    animatedNs.store(timer.nsecsElapsed());
            });
        const auto syncConnection = QObject::connect(fixture.window, &QQuickWindow::beforeSynchronizing,
            fixture.window, [&] {
                if (animatedNs.load() && !synchronizedNs.load())
                    synchronizedNs.store(timer.nsecsElapsed());
            }, Qt::DirectConnection);
        const auto connection = QObject::connect(fixture.window, &QQuickWindow::frameSwapped,
            fixture.window, [&] {
                if (fixture.surface->property("appliedDocumentKey").toString() == key)
                    swapped = true;
            });
        timer.start();
        fixture.shell.setScene(documentScene(frame));
        const qint64 stateNs = timer.nsecsElapsed();
        while (!swapped && timer.elapsed() < 5000)
            QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
        QVERIFY(swapped);
        qInfo() << "real document cycle/state apply/afterAnimating/beforeSync/ready swap ms"
                << cycle << stateNs / 1e6 << animatedNs.load() / 1e6
                << synchronizedNs.load() / 1e6 << timer.nsecsElapsed() / 1e6;
        int activeRows = 0, textLeaves = 0, inViewportLeaves = 0;
        qsizetype shapedCharacters = 0;
        QList<QQuickItem *> demandItems{fixture.surface};
        while (!demandItems.isEmpty()) {
            auto *item = demandItems.takeLast();
            demandItems.append(item->childItems());
            if (item->objectName() == "documentRowDelegate"
                && item->property("contentActive").toBool())
                ++activeRows;
            if ((item->objectName() == "documentRunText"
                 || item->objectName() == "documentPlainText")
                && !item->property("text").toString().isEmpty()) {
                ++textLeaves;
                shapedCharacters += item->property("text").toString().size();
                const qreal top = item->mapToItem(fixture.list, QPointF()).y();
                if (top + item->height() > 0 && top < fixture.list->height())
                    ++inViewportLeaves;
            }
        }
        qInfo() << "real document activeRows/nonemptyText/inViewportText/shapedChars/sourceRows"
                << activeRows << textLeaves << inViewportLeaves << shapedCharacters
                << source.value("windowRows").toList().size();
        QObject::disconnect(connection);
        QObject::disconnect(animatedConnection);
        QObject::disconnect(syncConnection);
        fixture.shell.surfaceRegistry()->applyDocument({}, cycle * 2 + 2);
        QCoreApplication::processEvents();
        // Match the live F3/Esc/F4/Esc workload: the editor occupies the same
        // physical row slots between viewer opens, so rows must really change.
        auto editor = editorFrame(0, source.value("windowRows").toList().size(), 0, 1);
        const QString editorKey = QString("real-intermediate-editor-%1").arg(cycle);
        editor.insert("documentKey", editorKey);
        fixture.shell.setScene(documentScene(editor));
        QTRY_COMPARE(fixture.surface->property("appliedDocumentKey").toString(), editorKey);
        fixture.shell.surfaceRegistry()->applyDocument({}, cycle * 2 + 3);
        QCoreApplication::processEvents();
    }
}

void F4DocumentSurfaceTests::realEditorSelectionKeepsCurrentStyledRowsAfterViewer()
{
    const QString path = qEnvironmentVariable("F4_EDITOR_SELECTION_FIXTURE");
    if (path.isEmpty())
        QSKIP("Set F4_EDITOR_SELECTION_FIXTURE to the real editor style fixture");
    QFile file(path);
    QVERIFY(file.open(QIODevice::ReadOnly));
    const auto source = QJsonDocument::fromJson(file.readAll()).object().toVariantMap();
    auto plain = source.value("plain").toMap();
    auto selected = source.value("selected").toMap();
    QVERIFY(!plain.isEmpty() && !selected.isEmpty());
    auto viewer = viewerFrame(15000, 138, 17000, 0);
    QFile viewerFile(qEnvironmentVariable("F4_DOCUMENT_TIMING_FIXTURE"));
    if (viewerFile.open(QIODevice::ReadOnly))
        viewer = QJsonDocument::fromJson(viewerFile.readAll()).object().toVariantMap();
    viewer.insert("documentKey", "preceding-viewer");
    DocumentFixture fixture(documentScene({}), 250);
    QVERIFY(fixture.ready());
    fixture.window->resize(579, 250);
    QVERIFY(fixture.window->setProperty("ch", 23.0));
    QTRY_VERIFY(qAbs(fixture.surface->property("rowHeight").toReal()
                     * fixture.window->devicePixelRatio() - 40.0) < 0.001);
    QTRY_COMPARE(
        fixture.surface->property("reportedViewportColumns").toInt(),
        int(fixture.surface->property("prospectiveViewportWidth").toReal()
            / fixture.surface->property("terminalCellWidth").toReal()));
    QVERIFY(fixture.surface->property("reportedViewportColumns").toInt() > 0);
    viewer.remove("geometryRevision");
    fixture.shell.setScene(documentScene(viewer));
    QTRY_COMPARE(fixture.surface->property("appliedDocumentKey").toString(), QString("preceding-viewer"));
    fixture.shell.setScene(documentScene({}));
    QTRY_VERIFY(!fixture.surface->property("interactionActive").toBool());
    plain.insert("documentKey", "selected-editor");
    selected.insert("documentKey", "selected-editor");
    plain.remove("geometryRevision");
    selected.remove("geometryRevision");
    fixture.shell.setScene(documentScene(plain));
    QTRY_COMPARE(fixture.surface->property("appliedDocumentKey").toString(), QString("selected-editor"));
    const auto firstRow = plain.value("windowRows").toList().first().toMap();
    const QString expected = firstRow.value("runs").toList().first().toMap().value("text").toString();
    QVERIFY(expected.startsWith("MZ"));
    auto verifyFirstRow = [&] {
        QQuickItem *firstText = nullptr;
        qreal firstY = 1e9, firstX = 1e9;
        QList<QQuickItem *> pending{fixture.surface};
        while (!pending.isEmpty()) {
            auto *item = pending.takeLast();
            pending.append(item->childItems());
            if (item->objectName() != "documentRunText" || item->property("text").toString().isEmpty())
                continue;
            const auto at = item->mapToItem(fixture.list, QPointF());
            if (at.y() + item->height() <= 0 || at.y() >= fixture.list->height())
                continue;
            if (at.y() < firstY - 1 || (qAbs(at.y() - firstY) < 1 && at.x() < firstX)) {
                firstText = item;
                firstY = at.y(); firstX = at.x();
            }
        }
        QVERIFY(firstText);
        qInfo() << "editor first presented run" << firstText->property("text").toString().left(32)
                << "contentY" << fixture.list->property("contentY") << "top" << topExtent(fixture.surface, fixture.list);
        QVERIFY(firstText->property("text").toString().startsWith("MZ"));
    };
    verifyFirstRow();
    QCOMPARE(plain.value("windowGeneration"), selected.value("windowGeneration"));
    QCOMPARE(plain.value("windowContentKey"), selected.value("windowContentKey"));
    QCOMPARE(plain.value("windowRows"), selected.value("windowRows"));
    fixture.shell.setScene(documentScene(selected));
    QTRY_COMPARE(fixture.surface->property("displayedRows").toList(), selected.value("windowRows").toList());
    verifyFirstRow();
    QSignalSpy settledFrames(fixture.window, &QQuickWindow::frameSwapped);
    for (int update = 0; update < 40; ++update) {
        const auto &next = update % 2 == 0 ? plain : selected;
        fixture.shell.setScene(documentScene(next));
        QTRY_COMPARE(fixture.surface->property("displayedRows").toList(),
                     next.value("windowRows").toList());
        fixture.window->update();
        QTRY_VERIFY(settledFrames.size() > update);
        QCOMPARE(topExtent(fixture.surface, fixture.list), 0.0);
    }
    verifyFirstRow();
    fixture.window->update();
    QTest::qWait(30);
    const QImage capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    int selectedPixels = 0;
    for (int y = 0; y < capture.height(); ++y)
        for (int x = 0; x < capture.width(); ++x)
            if (capture.pixelColor(x, y).rgb() == QColor("#3b6290").rgb())
                ++selectedPixels;
    const QString capturePath = qEnvironmentVariable("F4_EDITOR_SELECTION_CAPTURE");
    if (!capturePath.isEmpty())
        QVERIFY(capture.save(capturePath));
    qInfo() << "editor selected background pixels" << selectedPixels;
    QVERIFY(selectedPixels > 100);
}

void F4DocumentSurfaceTests::realEditorFirstViewportStartsOnExactRowBoundary()
{
#if defined(Q_OS_WIN)
    QVERIFY(QFontDatabase::addApplicationFont(
                QStringLiteral("C:/Windows/Fonts/consola.ttf")) >= 0);
#endif
    const QString path = qEnvironmentVariable("F4_EDITOR_SELECTION_FIXTURE");
    if (path.isEmpty())
        QSKIP("Set F4_EDITOR_SELECTION_FIXTURE to the real editor style fixture");
    QFile file(path);
    QVERIFY(file.open(QIODevice::ReadOnly));
    const auto source = QJsonDocument::fromJson(file.readAll()).object().toVariantMap();
    auto editor = source.value("plain").toMap();
    QVERIFY(!editor.value("windowRows").toList().isEmpty());

    // The clipboard image is 1013x437 physical pixels at 168 DPI: a
    // 579x250 logical window at 175%. GuiFontSize=18 resolves to a 23
    // logical-pixel console cell, while semantic glyphs remain 13 px.
    DocumentFixture fixture(documentScene({}), 250);
    QVERIFY(fixture.ready());
    fixture.window->resize(579, 250);
    QVERIFY(fixture.window->setProperty("ch", 23.0));
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(
        fixture.surface->property("rowHeight").toReal()
            * fixture.window->devicePixelRatio() - 40.0) < 0.001, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("reportedViewportRows").toInt() > 0, 3000);

    auto viewer = viewerFrame(15000, 138, 17000, 1, 125, 100000);
    QFile viewerFile(qEnvironmentVariable("F4_DOCUMENT_TIMING_FIXTURE"));
    if (viewerFile.open(QIODevice::ReadOnly))
        viewer = QJsonDocument::fromJson(viewerFile.readAll()).object().toVariantMap();
    viewer.insert("documentKey", "preceding-production-viewer");
    viewer.remove("geometryRevision");
    fixture.shell.setScene(documentScene(viewer));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.surface->property("appliedDocumentKey").toString(),
        QString("preceding-production-viewer"), 3000);
    fixture.shell.setScene(documentScene({}));
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("interactionActive").toBool(), 3000);

    editor.insert("documentKey", "production-pyw-editor-reused-address");
    editor.remove("geometryRevision");
    fixture.shell.setScene(documentScene(editor));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.surface->property("appliedDocumentKey").toString(),
        QString("production-pyw-editor-reused-address"), 3000);

    // A semantic document key used to be the Go object address. Exercise the
    // real allocator-reuse failure: an old editor can leave an unacknowledged
    // local scroll in the retained ListView, close, and a new editor can then
    // reopen at the same address with the same top-of-file transaction.
    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const int loadedSlotStart = fixture.surface->property("loadedSlotStart").toInt();
    QVERIFY(fixture.list->setProperty(
        "contentY", (loadedSlotStart + 30) * rowHeight));
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("contentY").toReal()
            >= (loadedSlotStart + 29.9) * rowHeight, 3000);
    const qulonglong firstPublication =
        fixture.surface->property("frame").toMap()
            .value("nativeWindowRevision").toULongLong();
    fixture.shell.setScene(documentScene({}));
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("interactionActive").toBool(), 3000);
    QCOMPARE(fixture.surface->property("appliedDocumentKey").toString(),
             QString());
    QVERIFY(!fixture.surface->property("windowInitialized").toBool());
    QVERIFY(!fixture.list->property("visible").toBool());
    fixture.shell.setScene(documentScene(editor));
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("interactionActive").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("frame").toMap()
            .value("nativeWindowRevision").toULongLong() > firstPublication,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    QQuickItem *firstRow = nullptr;
    QQuickItem *firstText = nullptr;
    qreal firstTextY = std::numeric_limits<qreal>::max();
    qreal firstTextX = std::numeric_limits<qreal>::max();
    QList<QQuickItem *> pending{fixture.surface};
    while (!pending.isEmpty()) {
        auto *item = pending.takeLast();
        pending.append(item->childItems());
        if (!item->isVisible() || item->objectName() != "documentRunText"
            || item->property("text").toString().isEmpty())
            continue;
        const QPointF at = item->mapToItem(fixture.list, QPointF());
        if (at.y() + item->height() <= 0 || at.y() >= fixture.list->height())
            continue;
        if (at.y() < firstTextY - 1
            || (qAbs(at.y() - firstTextY) < 1 && at.x() < firstTextX)) {
            firstText = item;
            firstTextY = at.y();
            firstTextX = at.x();
        }
    }
    QVERIFY(firstText);
    firstRow = firstText->parentItem();
    while (firstRow && firstRow->objectName() != "documentRowDelegate")
        firstRow = firstRow->parentItem();
    QVERIFY(firstRow);
    QVERIFY2(firstText->property("text").toString().startsWith("MZ"),
             qPrintable(QString("first visible row is '%1'")
                            .arg(firstText->property("text").toString().left(32))));
    auto *cursor = findEditorCursor(fixture.surface);
    QVERIFY(cursor);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->isVisible(), 3000);

    const qreal rowTop = firstRow->mapToItem(fixture.list, QPointF()).y();
    const qreal textTop = firstText->mapToItem(fixture.list, QPointF()).y();
    const qreal cursorTop = cursor->mapToItem(fixture.list, QPointF()).y();
    qInfo() << "real pyw first row/text/cursor/contentY/originY/slot/height/dpr"
            << rowTop << textTop << cursorTop
            << fixture.list->property("contentY")
            << fixture.list->property("originY")
            << fixture.surface->property("loadedSlotStart")
            << fixture.surface->property("rowHeight")
            << fixture.window->devicePixelRatio();
    QVERIFY2(qAbs(rowTop) < 0.001,
             qPrintable(QString("first editor row starts at %1 logical px")
                            .arg(rowTop, 0, 'f', 6)));
    QVERIFY(textTop >= -0.001);
    QVERIFY(textTop + firstText->height() <= firstRow->height() + 0.001);
    QCOMPARE(cursor->property("windowRow").toInt(), 0);
    QVERIFY(qAbs(cursorTop - (rowTop + 2.0))
            <= 0.5 / fixture.window->devicePixelRatio() + 0.001);
    QVERIFY(cursorTop >= 0.0);
    QVERIFY(cursorTop + cursor->height() <= firstRow->height() + 0.001);
}

void F4DocumentSurfaceTests::recenteredWindowReplacesEachSlotOnlyOnce()
{
    auto frame = viewerFrame(0, 80, 0, 1);
    frame.insert("documentKey", "replace-once-old");
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    fixture.surface->setProperty("poolSlotWriteCount", 0);
    frame = viewerFrame(10000, 80, 10000, 1);
    frame.insert("documentKey", "replace-once-new");
    fixture.shell.setScene(documentScene(frame));
    QTRY_COMPARE(fixture.surface->property("appliedDocumentKey").toString(),
                 QString("replace-once-new"));
    QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), 80);
    QCOMPARE(fixture.surface->property("displayedRows").toList(), frame.value("windowRows").toList());
}

void F4DocumentSurfaceTests::denseDocumentOpenPresentationTiming()
{
    DocumentFixture fixture(documentScene({}), 1186);
    QVERIFY(fixture.window);
    fixture.window->resize(2208, 1186);
    QTRY_VERIFY((fixture.surface = fixture.window->findChild<QQuickItem *>("documentSurface")));
    QTRY_COMPARE(fixture.surface->property("reportedViewportColumns").toInt(),
        int(fixture.surface->property("prospectiveViewportWidth").toDouble()
            / fixture.surface->property("terminalCellWidth").toDouble()));
    qInfo() << "dense geometry window/document/columns/cell"
            << fixture.window->width() << fixture.surface->width()
            << fixture.surface->property("reportedViewportColumns")
            << fixture.surface->property("terminalCellWidth");
    const int columns = fixture.surface->property("reportedViewportColumns").toInt();
    QVariantList rows;
    for (int row = 0; row < 180; ++row) {
        rows.append(QVariantMap{{"offset", row * columns}, {"endOffset", (row + 1) * columns},
            {"runs", QVariantList{QVariantMap{{"text", QString(columns, u'x')},
                                               {"foreground", "#dddddd"}}}}});
    }
    QList<qint64> times;
    for (int cycle = 0; cycle < 6; ++cycle) {
        QVariantMap frame = viewerFrame(0, 180, 0, 1);
        const QString documentKey = QString("dense-document-%1").arg(cycle);
        frame.insert("documentKey", documentKey);
        frame.insert("windowRows", rows);
        frame.insert("contentExtent", 10000000);
        bool swapped = false;
        const auto connection = QObject::connect(fixture.window, &QQuickWindow::frameSwapped,
            fixture.window, [&] {
                if (fixture.surface->property("appliedDocumentKey").toString() == documentKey)
                    swapped = true;
            });
        QElapsedTimer elapsed;
        elapsed.start();
        fixture.shell.setScene(documentScene(frame));
        while (!swapped && elapsed.elapsed() < 5000)
            QCoreApplication::processEvents(QEventLoop::AllEvents, 10);
        QVERIFY(swapped);
        if (cycle == 0) {
            QList<QQuickItem *> leaves{fixture.surface};
            while (!leaves.isEmpty()) {
                auto *leaf = leaves.takeLast();
                leaves.append(leaf->childItems());
                if (leaf->isVisible() && leaf->objectName() == "documentRunText") {
                    qInfo() << "dense actual leaf font/width/advance" << leaf->property("font")
                        << leaf->implicitWidth() << leaf->implicitWidth() / columns;
                    QCOMPARE(leaf->implicitWidth() / columns,
                             fixture.surface->property("terminalCellWidth").toDouble());
                    break;
                }
            }
        }
        times.append(elapsed.nsecsElapsed());
        QObject::disconnect(connection);
        fixture.shell.surfaceRegistry()->applyDocument({}, cycle * 2 + 2);
        QCoreApplication::processEvents();
    }
    std::sort(times.begin(), times.end());
    qInfo() << "dense ready-window to actual swapped frame ms: min/median/max"
            << times.first() / 1e6 << times[times.size() / 2] / 1e6 << times.last() / 1e6;
}

void F4DocumentSurfaceTests::acknowledgedViewportMayClampToZero()
{
    auto frame = editorFrame(0, 40, 0, 1);
    frame.insert("documentKey", "clamped-viewport");
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "sendWindowRequest",
        Q_ARG(QVariant, 200), Q_ARG(QVariant, 0),
        Q_ARG(QVariant, 0), Q_ARG(QVariant, false)));
    QVERIFY(fixture.surface->property("windowRequestPending").toBool());
    frame.insert("windowGeneration", fixture.surface->property("requestedGeneration"));
    // The backend's acknowledged target is authoritative, including zero.
    // It may clamp a formerly valid destination after the extent changes.
    frame.insert("viewportStart", 0);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY(!fixture.surface->property("windowRequestPending").toBool());
    QCOMPARE(fixture.surface->property("stableTopExtent").toDouble(), 0.0);
    QCOMPARE(topExtent(fixture.surface, fixture.list), 0.0);
}

void F4DocumentSurfaceTests::scrollbarLatestIntentAndLayoutEpochCommitAtomically()
{
    QVariantMap frame = viewerFrame(0, 80, 200, 1);
    frame.insert("documentKey", "document-latest");
    frame.insert("layoutRevision", 4);
    frame.insert("geometryRevision", 1);
    frame.insert("defaultBackground", "#102030");
    frame.insert("cursorVisualColumn", 3);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    fixture.shell.clearActions();
    const auto request = [&](int offset) {
        return QMetaObject::invokeMethod(fixture.surface, "sendWindowRequest",
            Q_ARG(QVariant, offset), Q_ARG(QVariant, 0),
            Q_ARG(QVariant, 0), Q_ARG(QVariant, false));
    };
    QVERIFY(request(800));
    QVERIFY(request(200)); // Returning to the current page must cancel 800.
    QVERIFY(request(1200));
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.last().value("layoutRevision").toInt(), 4);
    const int activeGeneration = fixture.shell.actions.first().value("generation").toInt();
    const QVariantList before = fixture.surface->property("displayedRows").toList();
    QVariantMap stale = viewerFrame(600, 80, 800, activeGeneration);
    stale.insert("documentKey", "document-latest");
    stale.insert("layoutRevision", 4);
    stale.insert("defaultBackground", frame.value("defaultBackground"));
    stale.insert("cursorVisualColumn", frame.value("cursorVisualColumn"));
    fixture.shell.setScene(documentScene(stale));
    QTest::qWait(50);
    QCOMPARE(fixture.surface->property("displayedRows").toList(), stale.value("windowRows").toList());
    QCOMPARE(fixture.shell.actions.size(), 2);
    QCOMPARE(fixture.shell.actions.last().value("offset").toInt(), 1200);
    const int latestGeneration = fixture.shell.actions.last().value("generation").toInt();
    QVERIFY(latestGeneration > activeGeneration);
    QVariantMap ready = viewerFrame(1000, 80, 1200, latestGeneration);
    ready.insert("documentKey", "document-latest");
    ready.insert("layoutRevision", 5);
    ready.insert("geometryRevision", 1);
    ready.insert("layoutPending", true);
    ready.insert("contentExtent", 50000);
    ready.insert("defaultBackground", "#304050");
    ready.insert("cursorVisualColumn", 19);
    fixture.shell.setScene(documentScene(ready));
    QTest::qWait(50);
    QCOMPARE(fixture.surface->property("displayedRows").toList(), stale.value("windowRows").toList());
    QCOMPARE(fixture.surface->property("contentExtent"), frame.value("contentExtent"));
    QCOMPARE(fixture.surface->property("presentationFrame").toMap().value("defaultBackground"),
             frame.value("defaultBackground"));
    QCOMPARE(fixture.surface->property("cursorFrame").toMap().value("cursorVisualColumn").toInt(), 3);
    ready.insert("layoutPending", false);
    fixture.shell.setScene(documentScene(ready));
    QTRY_COMPARE(fixture.surface->property("appliedLayoutRevision").toInt(), 5);
    QTRY_VERIFY(!fixture.surface->property("windowRequestPending").toBool());
    QCOMPARE(fixture.surface->property("displayedRows").toList(), ready.value("windowRows").toList());
    QCOMPARE(fixture.surface->property("appliedLayoutRevision").toInt(), 5);
    QCOMPARE(fixture.surface->property("contentExtent").toInt(), 50000);
    QCOMPARE(fixture.surface->property("presentationFrame").toMap().value("defaultBackground"),
             ready.value("defaultBackground"));
    QCOMPARE(fixture.surface->property("cursorFrame").toMap().value("cursorVisualColumn").toInt(), 19);
    QVERIFY(request(1500));
    const int failedGeneration = fixture.surface->property("requestedGeneration").toInt();
    QVERIFY(request(1600));
    QVariantMap failed = ready;
    failed.insert("layoutPending", true);
    failed.insert("windowRequestGeneration", failedGeneration);
    failed.insert("loadError", "range read failed");
    fixture.shell.setScene(documentScene(failed));
    QTest::qWait(50);
    QVERIFY(fixture.surface->property("windowRequestPending").toBool());
    QVERIFY(!fixture.surface->property("topBarRightText").toString().contains("range read failed"));
    failed.insert("windowRequestGeneration", fixture.surface->property("requestedGeneration"));
    fixture.shell.setScene(documentScene(failed));
    QTRY_VERIFY(!fixture.surface->property("windowRequestPending").toBool());
    QCOMPARE(fixture.surface->property("displayedRows").toList(), ready.value("windowRows").toList());
    QVERIFY(fixture.surface->property("topBarRightText").toString().contains("range read failed"));
    QVariantMap opening = ready;
    opening.insert("documentKey", "different-document");
    opening.insert("layoutPending", true);
    fixture.shell.setScene(documentScene(opening));
    QTRY_VERIFY(!fixture.list->isVisible());
    QCOMPARE(fixture.surface->property("displayedRows").toList(), ready.value("windowRows").toList());
}

void F4DocumentSurfaceTests::homeEndRetiresQueuedScrollIntentBeforeForwarding()
{
    auto frame = viewerFrame(0, 80, 200, 10, 10, 5000);
    frame.insert("documentKey", "home-cancels-qml-destination");
    frame.insert("layoutRevision", 4);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    fixture.shell.clearActions();
    const auto request = [&](int offset) {
        return QMetaObject::invokeMethod(fixture.surface, "sendWindowRequest",
            Q_ARG(QVariant, offset), Q_ARG(QVariant, 0),
            Q_ARG(QVariant, 0), Q_ARG(QVariant, false));
    };

    QVERIFY(request(800)); // Active generation 11.
    QVERIFY(request(1200)); // Replaceable destination, not yet sent.
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.first().value("documentKey").toString(),
             QString("home-cancels-qml-destination"));
    QCOMPARE(fixture.shell.actions.first().value("contentKey").toString(),
             QString("home-cancels-qml-destination"));
    QCOMPARE(fixture.surface->property("requestedGeneration").toInt(), 11);
    QVERIFY(fixture.surface->property("windowRequestPending").toBool());
    QVERIFY(!fixture.surface->property("pendingWindowIntent").isNull());

    auto *grid = fixture.window->findChild<TestGrid *>();
    QVERIFY(grid);
    // Production VtuiGridItem emits this synchronously before forwarding the
    // single Home press to Go.
    emit grid->edgeNavigationAboutToForward(Qt::Key_Home);
    QCOMPARE(fixture.surface->property("canceledWindowGeneration").toInt(), 11);
    QVERIFY(!fixture.surface->property("windowRequestPending").toBool());
    QVERIFY(fixture.surface->property("pendingWindowIntent").isNull());

    auto home = viewerFrame(0, 80, 0, 12, 10, 5000);
    home.insert("documentKey", "home-cancels-qml-destination");
    home.insert("layoutRevision", 4);
    fixture.shell.setScene(documentScene(home));
    QTRY_COMPARE(fixture.surface->property("appliedWindowGeneration").toInt(),
                 12);
    QCOMPARE(topExtent(fixture.surface, fixture.list), 0.0);
    // The retired 1200 destination must not be resurrected as generation 13
    // after the Home frame commits.
    QCOMPARE(fixture.shell.actions.size(), 1);

    // Editor Home/End, including selection-bearing Shift variants, uses the
    // same fresh-generation fence as the viewer. Retire both the active row
    // request and its replaceable destination before Go handles the key.
    auto editor = editorFrame(0, 80, 0, 10, 5000);
    editor.insert("documentKey", "editor-edge-selection-generation");
    editor.insert("layoutRevision", 4);
    DocumentFixture editorFixture(documentScene(editor));
    QVERIFY(editorFixture.ready());
    QTRY_VERIFY(editorFixture.surface->property("windowInitialized").toBool());
    editorFixture.shell.clearActions();
    const auto editorRequest = [&](int row) {
        return QMetaObject::invokeMethod(
            editorFixture.surface, "sendWindowRequest",
            Q_ARG(QVariant, row), Q_ARG(QVariant, 0),
            Q_ARG(QVariant, 0), Q_ARG(QVariant, false));
    };
    QVERIFY(editorRequest(200));
    QVERIFY(editorRequest(400));
    auto *editorGrid = editorFixture.window->findChild<TestGrid *>();
    QVERIFY(editorGrid);
    emit editorGrid->edgeNavigationAboutToForward(Qt::Key_End);
    QVERIFY(!editorFixture.surface->property("windowRequestPending").toBool());
    QVERIFY(editorFixture.surface->property("pendingWindowIntent").isNull());
    QCOMPARE(editorFixture.surface->property("canceledWindowGeneration").toInt(),
             11);

    // A scroll reply already in flight must not move the viewport after the
    // caret/selection command has fenced that generation.
    auto lateScroll = editorFrame(200, 80, 200, 11, 5000);
    lateScroll.insert("documentKey", "editor-edge-selection-generation");
    lateScroll.insert("layoutRevision", 4);
    editorFixture.shell.setScene(documentScene(lateScroll));
    QCoreApplication::processEvents();
    QCOMPARE(editorFixture.surface->property("appliedWindowGeneration").toInt(),
             10);
    QCOMPARE(topExtent(editorFixture.surface, editorFixture.list), 0.0);

    // Go allocates fence+1 for the Home/End result. That authoritative frame
    // is accepted, and the discarded queued row 400 is never resurrected.
    auto end = editorFrame(4920, 80, 4920, 12, 5000);
    end.insert("documentKey", "editor-edge-selection-generation");
    end.insert("layoutRevision", 4);
    editorFixture.shell.setScene(documentScene(end));
    QTRY_COMPARE(
        editorFixture.surface->property("appliedWindowGeneration").toInt(), 12);
    QCOMPARE(topExtent(editorFixture.surface, editorFixture.list), 4920.0);
    QCOMPARE(editorFixture.shell.actions.size(), 1);
}

void F4DocumentSurfaceTests::homeEndKeepsPendingScrollWhenOverlayOwnsInput()
{
    const auto verify = [](bool editor, bool operationsOverlay) {
        const QString key = editor ? QStringLiteral("overlay-editor")
                                   : QStringLiteral("overlay-viewer");
        auto frame = editor ? editorFrame(0, 80, 0, 10, 5000)
                            : viewerFrame(0, 80, 0, 10, 10, 5000);
        frame.insert(QStringLiteral("documentKey"), key);
        frame.insert(QStringLiteral("layoutRevision"), 4);
        DocumentFixture fixture(documentScene(frame));
        QVERIFY(fixture.ready());
        QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
        fixture.shell.clearActions();

        const int firstDestination = editor ? 200 : 800;
        QVERIFY(QMetaObject::invokeMethod(
            fixture.surface, "sendWindowRequest", Qt::DirectConnection,
            Q_ARG(QVariant, firstDestination), Q_ARG(QVariant, 0),
            Q_ARG(QVariant, 0), Q_ARG(QVariant, false)));
        QCOMPARE(fixture.surface->property("requestedGeneration").toInt(), 11);
        QVERIFY(fixture.surface->property("windowRequestPending").toBool());

        if (operationsOverlay) {
            fixture.shell.surfaceRegistry()->applyOperationsQueue({
                {QStringLiteral("id"), QStringLiteral("edge-queue")},
                {QStringLiteral("kind"), QStringLiteral("operationsQueue")},
                {QStringLiteral("title"), QStringLiteral("Operations")},
                {QStringLiteral("items"), QVariantList{}},
            }, 100);
        } else {
            fixture.shell.overlayState()->applyDialogsState({
                {QStringLiteral("dialogs"), QVariantList{QVariantMap{
                    {QStringLiteral("id"), QStringLiteral("edge-dialog")},
                    {QStringLiteral("kind"), QStringLiteral("dialog")},
                    {QStringLiteral("title"), QStringLiteral("Blocking")},
                    {QStringLiteral("x"), 4},
                    {QStringLiteral("y"), 2},
                    {QStringLiteral("w"), 30},
                    {QStringLiteral("h"), 10},
                    {QStringLiteral("controls"), QVariantList{}},
                    {QStringLiteral("buttons"), QVariantList{}},
                }}},
            }, 100);
        }
        QCoreApplication::processEvents();

        auto *grid = fixture.window->findChild<TestGrid *>();
        QVERIFY(grid);
        emit grid->edgeNavigationAboutToForward(
            editor ? Qt::Key_End : Qt::Key_Home);
        QCOMPARE(fixture.surface->property("canceledWindowGeneration").toInt(),
                 0);
        QVERIFY(fixture.surface->property("windowRequestPending").toBool());
        QCOMPARE(fixture.surface->property("requestedGeneration").toInt(), 11);

        // The overlay consumes Home/End, so the in-flight document result is
        // still authoritative and must be allowed to acknowledge generation 11.
        auto acknowledged = editor
            ? editorFrame(firstDestination, 80, firstDestination, 11, 5000)
            : viewerFrame(firstDestination, 80, firstDestination, 11, 10, 5000);
        acknowledged.insert(QStringLiteral("documentKey"), key);
        acknowledged.insert(QStringLiteral("layoutRevision"), 4);
        fixture.shell.surfaceRegistry()->applyDocument(acknowledged, 101);
        QTRY_COMPARE(fixture.surface->property("appliedWindowGeneration").toInt(),
                     11);
        QCOMPARE(topExtent(fixture.surface, fixture.list),
                 qreal(firstDestination));
        QVERIFY(!fixture.surface->property("windowRequestPending").toBool());

        if (operationsOverlay) {
            fixture.shell.surfaceRegistry()->applyOperationsQueue({}, 102);
        } else {
            fixture.shell.overlayState()->applyDialogsState({
                {QStringLiteral("dialogs"), QVariantList{}},
            }, 102);
        }
        QCoreApplication::processEvents();

        const int secondDestination = firstDestination + (editor ? 200 : 400);
        QVERIFY(QMetaObject::invokeMethod(
            fixture.surface, "sendWindowRequest", Qt::DirectConnection,
            Q_ARG(QVariant, secondDestination), Q_ARG(QVariant, 0),
            Q_ARG(QVariant, 0), Q_ARG(QVariant, false)));
        QCOMPARE(fixture.surface->property("requestedGeneration").toInt(), 12);
        emit grid->edgeNavigationAboutToForward(
            editor ? Qt::Key_End : Qt::Key_Home);
        QCOMPARE(fixture.surface->property("canceledWindowGeneration").toInt(),
                 12);
        QVERIFY(!fixture.surface->property("windowRequestPending").toBool());
    };

    // Both kinds of surface and both input-owning layers exercise the same
    // pre-forward fence: dialogs cover the viewer, Operations covers the editor.
    verify(false, false);
    verify(true, true);
}

void F4DocumentSurfaceTests::standaloneViewportIsNegotiatedBeforeOpening()
{
    DocumentFixture fixture(documentScene({}));
    QVERIFY(fixture.window);
    QTRY_VERIFY((fixture.surface = fixture.window->findChild<QQuickItem *>("documentSurface")));
    QVariantMap geometry;
    QTRY_VERIFY([&] {
        for (const auto &action : std::as_const(fixture.shell.actions)) {
            if (action.value("action") == "document.viewport"
                && action.value("scope") == "standalone")
                geometry = action;
        }
        return geometry.value("columns").toInt() > 0
            && geometry.value("rows").toInt() > 0;
    }());
    const int revision = geometry.value("geometryRevision").toInt();
    QVERIFY(revision > 0);
    fixture.shell.clearActions();
    fixture.shell.setScene(documentScene(viewerFrame(0, 80, 0, 1)));
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    fixture.shell.surfaceRegistry()->applyDocument({}, 2);
    QTest::qWait(30);
    for (const auto &action : std::as_const(fixture.shell.actions)) {
        if (action.value("action") == "document.viewport") {
            QVERIFY(action.value("rows").toInt() > 0);
            QCOMPARE(action.value("geometryRevision").toInt(), revision);
        }
    }
    fixture.shell.clearActions();
    fixture.window->resize(800, fixture.window->height());
    QTRY_VERIFY([&] {
        for (const auto &action : std::as_const(fixture.shell.actions)) {
            if (action.value("action") == "document.viewport"
                && action.value("columns").toInt() > geometry.value("columns").toInt()
                && action.value("geometryRevision").toInt() > revision)
                return true;
        }
        return false;
    }());
}

void F4DocumentSurfaceTests::styledDocumentRunsAreVisible_data()
{
    QTest::addColumn<QString>("kind");
    QTest::newRow("viewer") << QString("viewer");
    QTest::newRow("editor") << QString("editor");
    QTest::newRow("terminal") << QString("terminal");
}

void F4DocumentSurfaceTests::styledDocumentRunsAreVisible()
{
    QFETCH(QString, kind);
    QVariantMap frame = kind == "terminal" ? terminalFrame(0, 30, 0, 1)
                       : kind == "editor" ? editorFrame(0, 30, 0, 1)
                                           : viewerFrame(0, 30, 0, 1);
    QVariantList rows = frame.value(QStringLiteral("windowRows")).toList();
    QVariantMap first = rows.first().toMap();
    first.insert(QStringLiteral("runs"), QVariantList{
        QVariantMap{{QStringLiteral("text"), QStringLiteral("styled content")},
                    {QStringLiteral("foreground"), QStringLiteral("#39dc85")},
                    {QStringLiteral("bold"), true}},
        QVariantMap{{QStringLiteral("text"), QStringLiteral(" theme content")}},
    });
    rows[0] = first;
    frame.insert(QStringLiteral("windowRows"), rows);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    const auto findRun = [&](const QString &text) -> QQuickItem * {
        QList<QQuickItem *> pending{fixture.list};
        while (!pending.isEmpty()) {
            auto *item = pending.takeLast();
            if (item->objectName() == QStringLiteral("documentRunText")
                && item->isVisible() && item->property("text").toString() == text)
                return item;
            pending.append(item->childItems());
        }
        return nullptr;
    };
    QTRY_VERIFY_WITH_TIMEOUT(findRun(QStringLiteral("styled content")), 1000);
    auto *styled = findRun(QStringLiteral("styled content"));
    QCOMPARE(styled->property("color").value<QColor>(), QColor("#39dc85"));
    QVERIFY(styled->property("font").value<QFont>().bold());
    QVERIFY(styled->width() > 0);
    QVERIFY(styled->mapToItem(fixture.list, QPointF()).y() >= 0);
    QVERIFY(styled->mapToItem(fixture.list, QPointF()).y() < fixture.list->height());
    auto *themed = findRun(QStringLiteral(" theme content"));
    QVERIFY(themed);
    const qreal styledX = styled->mapToItem(fixture.list, QPointF()).x();
    const qreal themedX = themed->mapToItem(fixture.list, QPointF()).x();
    QVERIFY(themedX > styledX);
    QVERIFY(themedX >= styledX + styled->width() - 1.0);
    QVERIFY(fixture.window->setProperty("textColor", QColor("#e39142")));
    QTRY_COMPARE(themed->property("color").value<QColor>(), QColor("#e39142"));

    // Updating row runs dynamically (as during fast scrolling) must place
    // run segments synchronously without overlap or reset to x=0.
    first.insert(QStringLiteral("runs"), QVariantList{
        QVariantMap{{QStringLiteral("text"), QStringLiteral("000001EE90: ")}},
        QVariantMap{{QStringLiteral("text"), QStringLiteral("00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00 |................|")}},
    });
    first.insert(QStringLiteral("contentKey"), QStringLiteral("styled-row-v2"));
    rows[0] = first;
    frame.insert(QStringLiteral("windowRows"), rows);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(findRun(QStringLiteral("000001EE90: ")), 1000);
    auto *addrRun = findRun(QStringLiteral("000001EE90: "));
    auto *bytesRun = findRun(QStringLiteral("00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 00 |................|"));
    QVERIFY(addrRun);
    QVERIFY(bytesRun);
    const qreal addrX = addrRun->mapToItem(fixture.list, QPointF()).x();
    const qreal bytesX = bytesRun->mapToItem(fixture.list, QPointF()).x();
    QVERIFY(bytesX > addrX);
    QVERIFY(bytesX >= addrX + addrRun->width() - 1.0);
    QVERIFY(!fixture.window->grabWindow().isNull());
}

void F4DocumentSurfaceTests::compactDocumentUpdatesKeepViewportActive()
{
    QVariantMap frame = editorFrame(0, 60, 0, 1);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    QTRY_VERIFY(fixture.surface->property("reportedViewportRows").toInt() > 0);
    fixture.shell.clearActions();
    QSignalSpy activeChanges(fixture.surface, SIGNAL(interactionActiveChanged()));
    // Production commits the typed store, then publishes the compact surface
    // projection in the same event. No intermediate "closed" document exists.
    for (int revision = 2; revision <= 4; ++revision) {
        frame.insert(QStringLiteral("windowGeneration"), revision);
        fixture.shell.surfaceRegistry()->applyDocument(frame, revision);
        emit fixture.shell.compactPresentationChanged({
            {QStringLiteral("surfacePresent"), true},
            {QStringLiteral("surface"), frame},
        });
        QCoreApplication::processEvents();
    }
    QCOMPARE(activeChanges.size(), 0);
    for (const auto &action : fixture.shell.actions) {
        QVERIFY2(action.value(QStringLiteral("action")) != "document.viewport",
                 "A document update must not reset or re-register its viewport");
    }
}

void F4DocumentSurfaceTests::closingDocumentDoesNotReprojectItsRows()
{
    DocumentFixture fixture(documentScene(editorFrame(0, 120, 0, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    QSignalSpy frames(fixture.surface, SIGNAL(frameChanged()));
    const int writes = fixture.surface->property("poolSlotWriteCount").toInt();
    fixture.shell.surfaceRegistry()->applyDocument({}, 2);
    QCoreApplication::processEvents();
    QVERIFY(!fixture.surface->isVisible());
    QCOMPARE(frames.size(), 0);
    QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), writes);
}

void F4DocumentSurfaceTests::unrelatedStreamsDoNotReprojectDocumentRows()
{
    DocumentFixture fixture(documentScene(editorFrame(0, 120, 0, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    QSignalSpy frames(fixture.surface, SIGNAL(frameChanged()));
    QSignalSpy active(fixture.surface, SIGNAL(interactionActiveChanged()));
    auto *registry = fixture.shell.surfaceRegistry();
    registry->applyShell({}, 2);
    const QVariantMap shell{{QStringLiteral("kind"), QStringLiteral("panels")},
                            {QStringLiteral("title"), QStringLiteral("Panels")}};
    registry->applyShell(shell, 3);
    emit fixture.shell.compactPresentationChanged({
        {QStringLiteral("shellPresent"), true},
        {QStringLiteral("replaceShell"), true},
        {QStringLiteral("shell"), shell},
    });
    registry->applyOperationsQueue({{QStringLiteral("kind"), "operationsQueue"}}, 1);
    QCoreApplication::processEvents();
    QCOMPARE(frames.size(), 0);
    QCOMPARE(active.size(), 0);
}

void F4DocumentSurfaceTests::openingDocumentHasNoStaleOrUnpositionedFrame_data()
{
    QTest::addColumn<bool>("editor");
    QTest::addColumn<bool>("reopen");
    QTest::newRow("first-viewer") << false << false;
    QTest::newRow("first-editor") << true << false;
    QTest::newRow("reopen-viewer") << false << true;
    QTest::newRow("reopen-editor") << true << true;
}

void F4DocumentSurfaceTests::openingDocumentHasNoStaleOrUnpositionedFrame()
{
    QFETCH(bool, editor);
    QFETCH(bool, reopen);
    QVariantMap oldFrame = editor ? editorFrame(0, 60, 0, 1)
                                  : viewerFrame(0, 60, 0, 1);
    oldFrame.insert(QStringLiteral("documentKey"), QStringLiteral("old-document"));
    DocumentFixture fixture(documentScene(reopen ? oldFrame : QVariantMap{}));
    if (!reopen) {
        QVERIFY(fixture.window);
        fixture.window->setProperty("documentSurfacePrewarmed", true);
        fixture.surface = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("documentSurface"));
        QVERIFY(fixture.surface);
        fixture.list = fixture.surface->findChild<QQuickItem *>(QStringLiteral("documentList"));
        fixture.scrollBar = fixture.surface->findChild<QQuickItem *>(QStringLiteral("documentScrollBar"));
    }
    QVERIFY(fixture.ready());
    if (reopen)
        QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    fixture.shell.surfaceRegistry()->applyDocument({}, 2);
    QTRY_VERIFY(!fixture.surface->isVisible());

    const qreal wantedTop = editor ? 220 : 1200;
    QVariantMap nextFrame = editor ? editorFrame(200, 100, 220, 1)
                                   : viewerFrame(1000, 100, 1200, 1, 10, 5000);
    nextFrame.insert(QStringLiteral("id"), QStringLiteral("new-document"));
    nextFrame.insert(QStringLiteral("documentKey"), QStringLiteral("new-document"));
    struct PresentedState { bool initialized; QString key; qreal top; bool firstLineReady; };
    QList<PresentedState> frames;
    // beforeSynchronizing observes exactly what the next frame will consume;
    // waiting only for the final state misses a one-frame empty/stale surface.
    const auto connection = QObject::connect(fixture.window,
        &QQuickWindow::beforeSynchronizing, fixture.window, [&] {
            if (fixture.surface->isVisible()) {
                bool firstLineReady = false;
                const QString expected = editor ? "editor row 220" : "byte row 1200";
                QList<QQuickItem *> pending{fixture.list};
                while (!pending.isEmpty()) {
                    auto *item = pending.takeLast();
                    pending.append(item->childItems());
                    if (!item->isVisible() || item->objectName() != "documentPlainText"
                        || item->property("text").toString() != expected)
                        continue;
                    const qreal y = item->mapToItem(fixture.list, QPointF()).y();
                    firstLineReady |= y >= 0 && y < fixture.surface->property("rowHeight").toReal();
                }
                frames.append({fixture.surface->property("windowInitialized").toBool(),
                    fixture.surface->property("appliedDocumentKey").toString(),
                    topExtent(fixture.surface, fixture.list), firstLineReady});
            }
        }, Qt::DirectConnection);
    fixture.shell.surfaceRegistry()->applyDocument(nextFrame, 3);
    fixture.window->update();
    QTRY_VERIFY(!frames.isEmpty());
    QTest::qWait(80);
    QObject::disconnect(connection);
    for (const auto &frame : frames) {
        QVERIFY2(frame.initialized, "A visible document frame had no viewport placement");
        QCOMPARE(frame.key, QStringLiteral("new-document"));
        QCOMPARE(frame.top, wantedTop);
        QVERIFY2(frame.firstLineReady, "Ready metadata must never expose empty/stale first-line text");
    }
    QVERIFY(!fixture.window->grabWindow().isNull());
}

void F4DocumentSurfaceTests::documentSurfaceDoesNotPaintItsOwnBackdrop()
{
    DocumentFixture fixture(documentScene(viewerFrame(0, 20, 0, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QCOMPARE(fixture.surface->property("color").value<QColor>().alphaF(), 0.0);
    QQuickItem *row = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        QList<QQuickItem *> pending{fixture.list};
        while (!pending.isEmpty()) {
            QQuickItem *item = pending.takeFirst();
            if (item->objectName() == QStringLiteral("documentRowDelegate")) {
                row = item;
                return true;
            }
            pending.append(item->childItems());
        }
        return false;
    }(), 3000);
    QCOMPARE(row->property("color").value<QColor>().alphaF(), 0.0);

    QVariant returned;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "runBackground", Q_RETURN_ARG(QVariant, returned),
        Q_ARG(QVariant, QStringLiteral("#242424"))));
    QCOMPARE(QColor(returned.toString()).alphaF(), 0.0);
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "runBackground", Q_RETURN_ARG(QVariant, returned),
        Q_ARG(QVariant, QStringLiteral("#884422"))));
    QCOMPARE(QColor(returned.toString()), QColor(QStringLiteral("#884422")));
}

void F4DocumentSurfaceTests::documentHeaderShowsFullPathsForViewerAndEditor()
{
    const auto verify = [&](const QVariantMap &frame,
                            const QString &expectedLeft,
                            const QString &expectedRight) {
        DocumentFixture fixture(documentScene(frame));
        QVERIFY(fixture.ready());
        auto *header = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("documentHeader"));
        auto *left = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("documentHeaderLeft"));
        auto *right = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("documentHeaderRight"));
        auto *icon = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("documentHeaderIcon"));
        auto *lucideIcon = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("documentHeaderLucideIcon"));
        QVERIFY(header);
        QVERIFY(left);
        QVERIFY(right);
        QVERIFY(icon);
        QVERIFY(lucideIcon);
        QTRY_VERIFY_WITH_TIMEOUT(header->isVisible(), 3000);
        QTRY_VERIFY_WITH_TIMEOUT(icon->isVisible(), 3000);
        QTRY_VERIFY_WITH_TIMEOUT(lucideIcon->isVisible(), 3000);
        QCOMPARE(left->property("text").toString(), expectedLeft.trimmed());
        QCOMPARE(right->property("text").toString(), expectedRight.trimmed());
        QVERIFY(fixture.surface->property("documentFileIconAvailable").toBool());
        QCOMPARE(fixture.surface->property("documentFileIconColor").value<QColor>(),
                 QColor(QStringLiteral("#8AE234")));
        QVERIFY(fixture.surface->property("documentFileIconSource")
                    .toUrl()
                    .toString()
                    .contains(QStringLiteral("file-code.svg")));
        QVERIFY(left->x() > icon->x());
        QVERIFY(fixture.surface->property("documentHeaderHeight").toReal() > 0);
        QVERIFY(fixture.surface->property("topInset").toReal()
                > fixture.surface->property("surfaceMenuInset").toReal());

        const qreal headerBottom = header->mapToScene(
            QPointF(0, header->height())).y();
        const qreal listTop = fixture.list->mapToScene(QPointF(0, 0)).y();
        QVERIFY(qAbs(headerBottom - listTop) < 0.001);
    };

    QVariantMap viewer = viewerFrame(0, 20, 0, 1);
    viewer.insert(QStringLiteral("path"),
                  QStringLiteral("C:\\work\\viewer\\window.txt"));
    viewer.insert(QStringLiteral("baseName"), QStringLiteral("window.txt"));
    verify(viewer, QStringLiteral("C:\\work\\viewer\\window.txt"),
           QStringLiteral(" UTF-8 │ Text │ 42%     "));
    QVariantMap editor = editorFrame(0, 40, 0, 1);
    editor.insert(QStringLiteral("path"),
                  QStringLiteral("C:\\work\\editor\\window.txt"));
    editor.insert(QStringLiteral("baseName"), QStringLiteral("window.txt"));
    verify(editor, QStringLiteral("C:\\work\\editor\\window.txt"),
           QStringLiteral(" UTF-8 │ 41,2     "));

    QVariantMap generatedEditor = editorFrame(0, 40, 0, 1);
    generatedEditor.insert(QStringLiteral("path"),
                           QStringLiteral("C:\\Temp\\f4-generated.txt"));
    generatedEditor.insert(QStringLiteral("baseName"),
                           QStringLiteral("f4-generated.txt"));
    generatedEditor.insert(QStringLiteral("topBarLeft"),
                           QStringLiteral(" Search results: needle"));
    verify(generatedEditor, QStringLiteral("Search results: needle"),
           QStringLiteral(" UTF-8 │ 41,2     "));
}

void F4DocumentSurfaceTests::nativeViewportExcludesHeaderAndKeepsBottomCursorVisible()
{
    QVariantMap frame = editorFrame(0, 80, 0, 1);
    frame.insert(QStringLiteral("viewportSpan"), 30);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    QVariantMap viewportAction;
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString()
                == QStringLiteral("document.viewport")) {
                viewportAction = action;
            }
        }
        return !viewportAction.isEmpty()
            && viewportAction.value(QStringLiteral("rows")).toInt() > 0;
    }(), 3000);

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const int completeRows = static_cast<int>(std::floor(
        (fixture.list->height() + 0.001) / rowHeight));
    QCOMPARE(viewportAction.value(QStringLiteral("rows")).toInt(), completeRows);
    QCOMPARE(fixture.surface->property("reportedViewportRows").toInt(),
             completeRows);
    QVERIFY(completeRows > 0);
    QVERIFY(completeRows < 30);

    frame.insert(QStringLiteral("viewportSpan"), completeRows);
    frame.insert(QStringLiteral("cursorAbsoluteRow"), completeRows - 1);
    frame.insert(QStringLiteral("windowGeneration"), 2);
    fixture.shell.setScene(documentScene(frame));

    QQuickItem *cursor = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT((cursor = findEditorCursor(fixture.surface)) != nullptr,
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->isVisible(), 3000);
    const qreal cursorTop = cursor->mapToItem(fixture.list, 0, 0).y();
    QVERIFY(cursorTop >= 0.0);
    QVERIFY(cursorTop + cursor->height() <= fixture.list->height() + 0.001);

    const int reportedRows = fixture.surface->property("reportedViewportRows").toInt();
    QCOMPARE(fixture.surface->property("reportedViewportTarget").toString(),
             QStringLiteral("app"));
    QVERIFY(reportedRows > 0);
    fixture.shell.clearActions();
    fixture.surface->setProperty("interactionActive", false);
    QTest::qWait(100);
    // Standalone geometry is negotiated for the window session, rather than
    // owned by the currently displayed file. Deactivating/closing a document
    // must not clear the app viewport and make the next document inherit a
    // zero-row layout.
    for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
        QVERIFY2(action.value(QStringLiteral("rows")).toInt() != 0,
                 "standalone document deactivation cleared app viewport");
    }
    QCOMPARE(fixture.surface->property("reportedViewportRows").toInt(),
             reportedRows);
}

void F4DocumentSurfaceTests::standaloneDocumentsEndAtSharedKeyBarSeparator()
{
    const auto sceneWithKeyBar = [](const QVariantMap &frame) {
        QVariantMap scene = documentScene(frame);
        scene.insert(QStringLiteral("keyBar"), QVariantMap{
            {QStringLiteral("visible"), true},
            {QStringLiteral("items"), QVariantList{
                 QVariantMap{{QStringLiteral("key"), QStringLiteral("F1")},
                             {QStringLiteral("text"), QStringLiteral("Help")}},
             }},
        });
        return scene;
    };
    const auto verify = [&](const QVariantMap &frame) {
        DocumentFixture fixture(sceneWithKeyBar(frame));
        QVERIFY(fixture.ready());
        auto *keyBar = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("keyBar"));
        auto *separator = fixture.window->findChild<QQuickItem *>(
            QStringLiteral("keyBarTopSeparator"));
        QVERIFY(keyBar);
        QVERIFY(separator);
        QTRY_VERIFY_WITH_TIMEOUT(keyBar->isVisible(), 3000);
        QTRY_VERIFY_WITH_TIMEOUT(fixture.list->height() > 0, 3000);

        const qreal listBottom = fixture.list->mapToScene(
            QPointF(0, fixture.list->height())).y();
        const qreal keyBarTop = keyBar->mapToScene(QPointF(0, 0)).y();
        const qreal separatorTop = separator->mapToScene(QPointF(0, 0)).y();
        QVERIFY(qAbs(listBottom - keyBarTop) < 0.001);
        QVERIFY(qAbs(separatorTop - keyBarTop) < 0.001);
        QVERIFY(separator->height() > 0);
        QVERIFY(qAbs(fixture.surface->property("bottomInset").toReal()
                     - keyBar->height()) < 0.001);
    };

    verify(viewerFrame(0, 20, 0, 1));
    verify(editorFrame(0, 40, 0, 1));
}

void F4DocumentSurfaceTests::finalViewportAlignsLastRowBelowFractionalBottom()
{
    // A 599 px surface leaves a one-pixel remainder below a 30-row,
    // 20 px/row semantic window. The last row must be aligned to the bottom
    // of the ListView rather than left one pixel below its visible area.
    const QVariantMap frame = viewerFrame(700, 30, 700, 1, 10, 1000);
    DocumentFixture fixture(documentScene(frame), 599);
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.surface->property("displayedRows").toList().size(), 30,
        3000);

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const qreal minimumY = fixture.surface->property("loadedSlotStart").toInt()
                           * rowHeight;
    const qreal maximumY = qMax(
        minimumY,
        fixture.surface->property("loadedSlotEnd").toInt() * rowHeight
            - fixture.list->height());
    qInfo() << "end placement dpr/contentY/maximumY/height/rowHeight"
            << fixture.window->devicePixelRatio()
            << fixture.list->property("contentY") << maximumY
            << fixture.list->height() << rowHeight;
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.list->property("contentY").toReal() - maximumY) < 0.01,
        3000);
    QVERIFY(maximumY > minimumY);

    const qreal loadedBottom =
        fixture.surface->property("loadedSlotEnd").toInt() * rowHeight;
    QVERIFY(loadedBottom
            <= fixture.list->property("contentY").toReal()
                   + fixture.list->height() + 0.01);
}

void F4DocumentSurfaceTests::editorPointerEventsAreForwardedAsSemanticMouseActions()
{
    QVariantMap frame = editorFrame(0, 40, 0, 1);
    frame.insert("documentKey", "source-fragment-editor");
    frame.insert("layoutRevision", 9);
    frame.insert("scrollLeft", 17);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "sendWindowRequest",
        Q_ARG(QVariant, 200), Q_ARG(QVariant, 0),
        Q_ARG(QVariant, 0), Q_ARG(QVariant, false)));
    const int pendingGeneration = fixture.surface->property("requestedGeneration").toInt();
    const QVariantList pointerRows = fixture.surface->property("displayedRows").toList();
    fixture.shell.clearActions();

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const QPointF scenePoint = fixture.list->mapToScene(
        QPointF(34, rowHeight * 1.5));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::ShiftModifier,
                      scenePoint.toPoint());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 2, 1500);
    const QVariantMap press = fixture.shell.actions.constFirst();
    QCOMPARE(press.value(QStringLiteral("action")), QStringLiteral("editor.mouse"));
    QCOMPARE(press.value(QStringLiteral("phase")), QStringLiteral("press"));
    QCOMPARE(press.value(QStringLiteral("button")), QStringLiteral("left"));
    QVERIFY(press.value(QStringLiteral("column")).toInt() >= 0);
    QCOMPARE(press.value(QStringLiteral("row")).toInt(), 1);
    QCOMPARE(press.value("documentKey").toString(), QString("source-fragment-editor"));
    QCOMPARE(press.value("layoutRevision").toInt(), 9);
    QCOMPARE(press.value("rowOffset").toInt(), 537);
    QCOMPARE(press.value("scrollLeft").toInt(), 17);
    QVERIFY(!fixture.surface->property("windowRequestPending").toBool());
    QCOMPARE(fixture.surface->property("requestedGeneration").toInt(), pendingGeneration);
    QVERIFY(press.value(QStringLiteral("shift")).toBool());
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("phase")),
             QStringLiteral("release"));
    QVariantMap canceled = editorFrame(180, 40, 200, pendingGeneration);
    canceled.insert("documentKey", "source-fragment-editor");
    canceled.insert("layoutRevision", 9);
    canceled.insert("scrollLeft", 17);
    fixture.shell.setScene(documentScene(canceled));
    QTest::qWait(30);
    QCOMPARE(fixture.surface->property("displayedRows").toList(), pointerRows);

    // A ListView may have a partially clipped first delegate while native
    // scrolling or a window rebase settles.  The pointer row must follow the
    // delegate under it, not a viewport-local row grid anchored at y=0.
    fixture.list->setProperty(
        "contentY", fixture.list->property("contentY").toReal()
                        + rowHeight * 0.6);
    QCoreApplication::processEvents();
    fixture.shell.clearActions();
    const QPointF fractionalPoint = fixture.list->mapToScene(
        QPointF(34, rowHeight * 0.6));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      fractionalPoint.toPoint());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 2, 1500);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("row")).toInt(), 1);

    fixture.shell.clearActions();
    QTest::mousePress(fixture.window, Qt::RightButton, Qt::NoModifier,
                      scenePoint.toPoint());
    QTest::mouseMove(fixture.window, (scenePoint + QPointF(30, 24)).toPoint());
    QTest::mouseRelease(fixture.window, Qt::RightButton, Qt::NoModifier,
                        (scenePoint + QPointF(30, 24)).toPoint());
    QTRY_VERIFY_WITH_TIMEOUT(fixture.shell.actions.size() >= 3, 1500);
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("button")),
             QStringLiteral("right"));
    bool sawMove = false;
    for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
        sawMove = sawMove || action.value(QStringLiteral("moved")).toBool();
    }
    QVERIFY(sawMove);

    fixture.shell.clearActions();
    QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                       scenePoint.toPoint());
    QTRY_VERIFY_WITH_TIMEOUT(!fixture.shell.actions.isEmpty(), 1500);
    bool sawDoubleClick = false;
    for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
        sawDoubleClick = sawDoubleClick
            || action.value(QStringLiteral("doubleClick")).toBool();
    }
    QVERIFY(sawDoubleClick);

    fixture.shell.clearActions();
    const qreal beforeWheelY = fixture.list->property("contentY").toReal();
    sendPixelWheel(fixture.window, scenePoint.toPoint(), -120);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("contentY").toReal() > beforeWheelY, 1000);
    QTest::qWait(260);
    for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
        QVERIFY2(action.value(QStringLiteral("action")).toString()
                     != QStringLiteral("editor.mouse"),
                 "editor wheel input must stay in the QML scroll pipeline");
    }
}

void F4DocumentSurfaceTests::editorEdgeSelectionUsesCommittedSourceFragments()
{
    QVariantMap frame = editorFrame(20, 100, 40, 1);
    frame.insert("documentKey", "edge-source-editor");
    frame.insert("layoutRevision", 2);
    frame.insert("scrollLeft", 17);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    fixture.shell.clearActions();
    const auto sendPointer = [&](qreal x, qreal y, const QString &phase) {
        const QVariantMap point{{"x", x}, {"y", y},
            {"button", int(Qt::LeftButton)},
            {"buttons", phase == "release" ? 0 : int(Qt::LeftButton)},
            {"modifiers", 0}};
        return QMetaObject::invokeMethod(fixture.surface, "sendEditorMouse",
            Q_ARG(QVariant, point), Q_ARG(QVariant, phase),
            Q_ARG(QVariant, phase == "move"), Q_ARG(QVariant, false));
    };
    const auto mouseActions = [&] {
        QList<QVariantMap> actions;
        for (const auto &action : std::as_const(fixture.shell.actions)) {
            if (action.value("action") == "editor.mouse")
                actions.append(action);
        }
        return actions;
    };
    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    QVERIFY(sendPointer(100, rowHeight, "press"));
    QVERIFY(sendPointer(100, fixture.list->height() + 300, "move"));
    QVERIFY(sendPointer(100, fixture.list->height() + 300, "move"));
    QTRY_COMPARE(mouseActions().size(), 2);
    const auto edge = mouseActions().last();
    const int firstEdgeRow = (edge.value("rowOffset").toInt() - 37) / 500;
    QVERIFY(firstEdgeRow > 40 + fixture.list->height() / rowHeight);
    QVERIFY(firstEdgeRow <= 40 + fixture.list->height() / rowHeight + 2);
    QTest::qWait(50);
    QCOMPARE(mouseActions().size(), 2); // No unchanged endpoint queue.

    frame.insert("viewportStart", 43);
    frame.insert("viewportRow", 23);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY(mouseActions().size() > 2); // Same held physical pointer.
    QCOMPARE(mouseActions().last().value("rowOffset").toInt(),
             edge.value("rowOffset").toInt() + 3 * 500);
    QCOMPARE(mouseActions().last().value("scrollLeft").toInt(), 17);
    QVERIFY(sendPointer(100, fixture.list->height() + 300, "release"));
    const int releasedCount = mouseActions().size();
    frame.insert("viewportStart", 46);
    frame.insert("viewportRow", 26);
    fixture.shell.setScene(documentScene(frame));
    QTest::qWait(50);
    QCOMPARE(mouseActions().size(), releasedCount);

    // Left-edge columns remain signed so Go can reveal hidden text.
    fixture.shell.clearActions();
    QVERIFY(sendPointer(-20, rowHeight, "press"));
    QVERIFY(mouseActions().last().value("column").toInt() < 0);
    frame.insert("scrollLeft", 14);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY(mouseActions().size() > 1);
    QCOMPARE(mouseActions().last().value("scrollLeft").toInt(), 14);
    const int beforeReflow = mouseActions().size();
    frame.insert("layoutRevision", 3);
    fixture.shell.setScene(documentScene(frame));
    QTest::qWait(50);
    QCOMPARE(mouseActions().size(), beforeReflow);

    // In-viewport placement is not an edge gesture, even past a short line.
    fixture.shell.clearActions();
    QVERIFY(sendPointer(300, rowHeight, "press"));
    frame.insert("scrollLeft", 17);
    fixture.shell.setScene(documentScene(frame));
    QTest::qWait(50);
    QCOMPARE(mouseActions().size(), 1);
    QVERIFY(sendPointer(300, rowHeight, "release"));
}

void F4DocumentSurfaceTests::middleButtonAutoScrollsStandaloneDocuments_data()
{
    QTest::addColumn<QString>("kind");
    QTest::newRow("viewer") << QStringLiteral("viewer");
    QTest::newRow("editor") << QStringLiteral("editor");
}

void F4DocumentSurfaceTests::middleButtonAutoScrollsStandaloneDocuments()
{
    QFETCH(QString, kind);
    const bool editor = kind == QStringLiteral("editor");
    QVariantMap frame = editor
        ? editorFrame(0, 140, 40, 7, 5000)
        : viewerFrame(0, 140, 400, 7, 10, 5000);
    const QString documentKey = QStringLiteral("middle-scroll-") + kind;
    frame.insert(QStringLiteral("documentKey"), documentKey);
    frame.insert(QStringLiteral("layoutRevision"), 11);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    auto *middleArea = fixture.surface->findChild<QQuickItem *>(
        QStringLiteral("documentMiddleButtonArea"));
    auto *autoScroll = fixture.surface->findChild<QObject *>(
        QStringLiteral("documentMouseAutoScrollController"));
    QVERIFY(middleArea);
    QVERIFY(autoScroll);
    fixture.shell.clearActions();
    fixture.gallery.clearScrollingCursorRequests();

    const QString expectedAction = editor ? QStringLiteral("editor.scroll")
                                          : QStringLiteral("viewer.scrollWindow");
    const QString extentKey = editor ? QStringLiteral("visualRow")
                                     : QStringLiteral("offset");
    const auto matchingActions = [&] {
        QList<QVariantMap> actions;
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString()
                == expectedAction) {
                actions.append(action);
            }
        }
        return actions;
    };

    const QPointF centerInList(fixture.list->width() / 2,
                               fixture.list->height() / 2);
    const QPoint center = fixture.list->mapToScene(centerInList).toPoint();
    const QPoint lower = fixture.list
        ->mapToScene(centerInList + QPointF(0, fixture.list->height() * .30))
        .toPoint();
    const QPoint lowest = fixture.list
        ->mapToScene(centerInList + QPointF(0, fixture.list->height() * .45))
        .toPoint();

    // The middle-only hover layer must remain transparent to ordinary editor
    // selection and wheel delivery.
    if (editor) {
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                          center);
        QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 2, 1000);
        QCOMPARE(fixture.shell.actions.constFirst()
                     .value(QStringLiteral("action")).toString(),
                 QStringLiteral("editor.mouse"));
        QCOMPARE(fixture.shell.actions.constFirst()
                     .value(QStringLiteral("phase")).toString(),
                 QStringLiteral("press"));
        QCOMPARE(fixture.shell.actions.constLast()
                     .value(QStringLiteral("phase")).toString(),
                 QStringLiteral("release"));
        fixture.shell.clearActions();
    }
    const qreal beforeWheel = fixture.list->property("contentY").toReal();
    sendPixelWheel(fixture.window, center, -12);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("contentY").toReal() > beforeWheel, 1000);
    QVERIFY(fixture.surface->property("wheelGestureActive").toBool());
    fixture.shell.clearActions();
    const qreal initialContentY =
        fixture.list->property("contentY").toReal();

    // A press starts the shared frame-driven gesture. Once the pointer has
    // crossed its dead zone, release ends the held gesture for both document
    // kinds and must not leak an editor.mouse middle-button action.
    QTest::mousePress(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(autoScroll->property("scrollingMode").toBool(),
                             1000);
    QVERIFY(!autoScroll->property("animationRunning").toBool());
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.gallery.scrollingCursorRequests.isEmpty(), 1000);
    const QVariantMap cursorArmed =
        fixture.gallery.scrollingCursorRequests.constLast();
    QCOMPARE(cursorArmed.value(QStringLiteral("scrollingMode")).toBool(), true);
    QCOMPARE(cursorArmed.value(QStringLiteral("direction")).toInt(), 0);
    QVERIFY(!fixture.surface->property("wheelGestureActive").toBool());
    QTest::mouseMove(fixture.window, lower, 20);
    QTRY_VERIFY_WITH_TIMEOUT(
        autoScroll->property("animationRunning").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("contentY").toReal() > initialContentY + 1,
        1500);
    QTRY_VERIFY_WITH_TIMEOUT(!matchingActions().isEmpty(), 1500);
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.gallery.scrollingCursorRequests.isEmpty()
            && fixture.gallery.scrollingCursorRequests.constLast()
                   .value(QStringLiteral("direction")).toInt() == 1,
        1000);
    const QVariantMap firstRequest = matchingActions().constFirst();
    QCOMPARE(firstRequest.value(QStringLiteral("target")).toString(),
             editor ? QStringLiteral("editor-window-test")
                    : QStringLiteral("document-under-test"));
    QCOMPARE(firstRequest.value(QStringLiteral("documentKey")).toString(),
             documentKey);
    QCOMPARE(firstRequest.value(QStringLiteral("layoutRevision")).toInt(), 11);
    const int firstExtent = firstRequest.value(extentKey).toInt();
    const int firstGeneration =
        firstRequest.value(QStringLiteral("generation")).toInt();
    QVERIFY(firstGeneration > 7);

    // The native/core round trip is deliberately left unacknowledged. New
    // animation frames must replace one pending destination with the newest
    // pointer intent instead of flooding IPC or freezing at the first sample.
    QTest::mouseMove(fixture.window, lowest, 20);
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        const QVariant pending =
            fixture.surface->property("pendingWindowIntent");
        return !pending.isNull()
            && pending.toMap().value(QStringLiteral("extent")).toInt()
                   > firstExtent;
    }(), 1500);
    QCOMPARE(matchingActions().size(), 1);

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const qreal loadedMaximum = qMax(
        fixture.surface->property("loadedSlotStart").toInt() * rowHeight,
        fixture.surface->property("loadedSlotEnd").toInt() * rowHeight
            - fixture.list->height());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("contentY").toReal() >= loadedMaximum - 1,
        3000);
    const int latestExtentBeforeAck = static_cast<int>(std::floor(
        fixture.surface->property("pendingWindowIntent").toMap()
            .value(QStringLiteral("extent")).toDouble()));

    // Reaching the edge of the bounded row pool is not the end of the
    // gesture. The first core ACK extends/rebases that pool, dispatches the
    // one newest destination, and leaves the same physical pointer driving it.
    const int ackWindowStart = qMax(0, firstExtent - (editor ? 40 : 400));
    QVariantMap ack = editor
        ? editorFrame(ackWindowStart, 140, firstExtent,
                      firstGeneration, 5000)
        : viewerFrame(ackWindowStart, 140, firstExtent,
                      firstGeneration, 10, 5000);
    ack.insert(QStringLiteral("documentKey"), documentKey);
    ack.insert(QStringLiteral("layoutRevision"), 11);
    fixture.shell.setScene(documentScene(ack));
    QTRY_VERIFY_WITH_TIMEOUT(matchingActions().size() >= 2, 3000);
    QVERIFY(autoScroll->property("scrollingMode").toBool());
    QVERIFY(autoScroll->property("animationRunning").toBool());
    const QVariantMap afterAck = matchingActions().constLast();
    QVERIFY(afterAck.value(QStringLiteral("generation")).toInt()
            > firstGeneration);
    QVERIFY2(afterAck.value(extentKey).toInt() >= latestExtentBeforeAck,
             qPrintable(QStringLiteral("dispatched %1, pending-before-ack %2")
                            .arg(afterAck.value(extentKey).toInt())
                            .arg(latestExtentBeforeAck)));

    QTest::mouseRelease(fixture.window, Qt::MiddleButton, Qt::NoModifier,
                        lowest);
    QTRY_VERIFY_WITH_TIMEOUT(
        !autoScroll->property("scrollingMode").toBool(), 1000);
    QVERIFY(!autoScroll->property("animationRunning").toBool());
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.gallery.scrollingCursorRequests.isEmpty()
            && !fixture.gallery.scrollingCursorRequests.constLast()
                    .value(QStringLiteral("scrollingMode")).toBool(),
        1000);
    if (editor) {
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            QVERIFY2(action.value(QStringLiteral("action")).toString()
                         != QStringLiteral("editor.mouse"),
                     "GUI middle-button scrolling must not move the editor caret");
        }
    }

    // A stationary click intentionally leaves browser-style auto-scroll
    // armed. Later buttonless hover motion scrolls, and a second stationary
    // click toggles it off.
    const QPoint upper = fixture.list
        ->mapToScene(centerInList - QPointF(0, fixture.list->height() * .30))
        .toPoint();
    const qreal beforeHoverScroll =
        fixture.list->property("contentY").toReal();
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(autoScroll->property("scrollingMode").toBool(),
                             1000);
    QVERIFY(!autoScroll->property("animationRunning").toBool());
    QTest::mouseMove(fixture.window, upper, 20);
    QTRY_VERIFY_WITH_TIMEOUT(
        autoScroll->property("animationRunning").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.list->property("contentY").toReal() < beforeHoverScroll - 1,
        1500);
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, upper);
    QTRY_VERIFY_WITH_TIMEOUT(
        !autoScroll->property("scrollingMode").toBool(), 1000);

    // A blocking overlay owns subsequent input and must cancel an armed
    // stationary gesture.
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(autoScroll->property("scrollingMode").toBool(),
                             1000);
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), QVariantList{QVariantMap{
             {QStringLiteral("id"), QStringLiteral("middle-scroll-dialog")},
             {QStringLiteral("kind"), QStringLiteral("dialog")},
             {QStringLiteral("title"), QStringLiteral("Blocking")},
             {QStringLiteral("x"), 4},
             {QStringLiteral("y"), 2},
             {QStringLiteral("w"), 30},
             {QStringLiteral("h"), 10},
             {QStringLiteral("controls"), QVariantList{}},
             {QStringLiteral("buttons"), QVariantList{}},
         }}},
    }, 100);
    QTRY_VERIFY_WITH_TIMEOUT(
        !autoScroll->property("scrollingMode").toBool(), 1000);
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), QVariantList{}},
    }, 101);
    QCoreApplication::processEvents();

    // Changing to console input cannot leave GUI motion behind. The existing
    // console middle-button path remains authoritative and does not start the
    // QML frame animation.
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(autoScroll->property("scrollingMode").toBool(),
                             1000);
    fixture.window->setProperty("mouseWheelMode", QStringLiteral("console"));
    QTRY_VERIFY_WITH_TIMEOUT(
        !autoScroll->property("scrollingMode").toBool(), 1000);
    fixture.shell.clearActions();
    const qreal beforeConsoleMiddle =
        fixture.list->property("contentY").toReal();
    QTest::mousePress(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTest::mouseMove(fixture.window, lower, 20);
    QTest::mouseRelease(fixture.window, Qt::MiddleButton, Qt::NoModifier,
                        lower);
    QTest::qWait(80);
    QCOMPARE(fixture.list->property("contentY").toReal(),
             beforeConsoleMiddle);
    QVERIFY(!autoScroll->property("animationRunning").toBool());
    if (editor) {
        QList<QVariantMap> mouseActions;
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString()
                == QStringLiteral("editor.mouse")) {
                mouseActions.append(action);
            }
        }
        QCOMPARE(mouseActions.size(), 3);
        QCOMPARE(mouseActions.at(0).value(QStringLiteral("phase")).toString(),
                 QStringLiteral("press"));
        QCOMPARE(mouseActions.at(0).value(QStringLiteral("button")).toString(),
                 QStringLiteral("middle"));
        QCOMPARE(mouseActions.at(1).value(QStringLiteral("phase")).toString(),
                 QStringLiteral("move"));
        QCOMPARE(mouseActions.at(1).value(QStringLiteral("button")).toString(),
                 QStringLiteral("middle"));
        QCOMPARE(mouseActions.at(2).value(QStringLiteral("phase")).toString(),
                 QStringLiteral("release"));
        QCOMPARE(mouseActions.at(2).value(QStringLiteral("button")).toString(),
                 QStringLiteral("none"));
    }
    fixture.window->setProperty("mouseWheelMode", QStringLiteral("gui"));

    // Coordinates from the old wrap/geometry epoch cannot survive reflow.
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(autoScroll->property("scrollingMode").toBool(),
                             1000);
    frame.insert(QStringLiteral("layoutRevision"), 12);
    frame.insert(QStringLiteral("layoutPending"), true);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        !autoScroll->property("scrollingMode").toBool(), 1000);
    QVERIFY(!autoScroll->property("animationRunning").toBool());

    // Once the new layout is committed, deactivation cancels the gesture and
    // any grabbed middle press without leaving a frame animation alive.
    const int restoredGeneration =
        afterAck.value(QStringLiteral("generation")).toInt() + 1;
    QVariantMap restored = editor
        ? editorFrame(0, 140, 40, restoredGeneration, 5000)
        : viewerFrame(0, 140, 400, restoredGeneration, 10, 5000);
    restored.insert(QStringLiteral("documentKey"), documentKey);
    restored.insert(QStringLiteral("layoutRevision"), 12);
    fixture.shell.setScene(documentScene(restored));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.surface->property("appliedLayoutRevision").toInt(), 12, 3000);
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(autoScroll->property("scrollingMode").toBool(),
                             1000);
    fixture.surface->setProperty("interactionActive", false);
    QTRY_VERIFY_WITH_TIMEOUT(
        !autoScroll->property("scrollingMode").toBool(), 1000);
    QVERIFY(!autoScroll->property("animationRunning").toBool());
}

void F4DocumentSurfaceTests::initTestCase()
{
    QQuickWindow::setTextRenderType(QQuickWindow::NativeTextRendering);
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    QGuiApplication::styleHints()->setCursorFlashTime(0);
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
    qint64 previousTimeMs = 0;
    for (const FrameSample &sample : std::as_const(samples)) {
        QVERIFY(sample.flicking);
        QVERIFY2(sample.extent + 0.05 >= previousExtent,
                 qPrintable(QStringLiteral("non-monotonic frame: %1 -> %2")
                                .arg(previousExtent).arg(sample.extent)));
        const qreal elapsedSeconds =
            qBound(0.001,
                   qreal(sample.timeMs - previousTimeMs) / 1000.0,
                   0.1);
        const qreal continuousAdvanceLimit =
            previousSpeed * elapsedSeconds + 25.0;
        QVERIFY2(sample.extent - previousExtent < continuousAdvanceLimit,
                 qPrintable(QStringLiteral("discontinuous frame: %1 -> %2 "
                                           "in %3 ms (limit %4)")
                                .arg(previousExtent).arg(sample.extent)
                                .arg(sample.timeMs - previousTimeMs)
                                .arg(continuousAdvanceLimit)));
        QVERIFY(sample.contentY + 0.05 >= previousContentY);
        QVERIFY(sample.velocity * velocityAfter > 0);
        QVERIFY(qAbs(sample.velocity) <= previousSpeed + 10.0);
        previousExtent = sample.extent;
        previousContentY = sample.contentY;
        previousSpeed = qAbs(sample.velocity);
        previousTimeMs = sample.timeMs;
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
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.list->property("contentY").toReal() - contentYBefore)
            > 0.001,
        1000);
    QVERIFY(fixture.list->property("moving").toBool());
    QVERIFY(fixture.list->property("flicking").toBool());
    QVERIFY(qAbs(fixture.list->property("verticalVelocity").toReal()) > 1.0);
    QVERIFY(fixture.shell.actions.isEmpty());
}

void F4DocumentSurfaceTests::overlappingEditorUpdateTouchesOnlyChangedSlots()
{
    QVariantMap frame = editorFrame(40, 90, 70, 1);
    DocumentFixture fixture(documentScene(frame), 400);
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list) - 70.0) < 0.01,
        3000);

    QObject *rowModel = fixture.surface->findChild<QObject *>(
        QStringLiteral("documentRowsModel"));
    QVERIFY(rowModel);
    const int poolCount = rowModel->property("count").toInt();

    QQuickItem *untouchedText = nullptr;
    QList<QQuickItem *> pendingItems{fixture.surface};
    while (!pendingItems.isEmpty()) {
        auto *item = pendingItems.takeLast();
        pendingItems.append(item->childItems());
        if (item->isVisible() && item->objectName() == "documentPlainText"
            && item->property("text").toString() == "editor row 75") {
            untouchedText = item;
            break;
        }
    }
    QVERIFY(untouchedText);
    const auto textProperty = untouchedText->metaObject()->property(
        untouchedText->metaObject()->indexOfProperty("text"));
    QSignalSpy untouchedTextChanges(untouchedText, textProperty.notifySignal());
    QVERIFY(untouchedTextChanges.isValid());

    // A Shift/drag selection repaint keeps the same absolute window and
    // changes only the content key of the endpoint row.
    QVariantList rows = frame.value(QStringLiteral("windowRows")).toList();
    QVariantMap changed = rows.at(44).toMap();
    changed.insert(QStringLiteral("contentKey"),
                   QStringLiteral("editor-row-84-selected"));
    changed.insert(QStringLiteral("text"),
                   QStringLiteral("editor row 84 selected"));
    rows[44] = changed;
    frame.insert(QStringLiteral("windowRows"), rows);
    fixture.surface->setProperty("poolSlotWriteCount", 0);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow",
                                      Qt::DirectConnection));
    QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), 1);
    QCOMPARE(untouchedTextChanges.size(), 0);
    QCOMPARE(rowModel->property("count").toInt(), poolCount);
    QVERIFY(qAbs(topExtent(fixture.surface, fixture.list) - 70.0) < 0.01);

    // A one-row edge scroll keeps all 89 overlapping slots in place. One
    // entering row is filled and the single leaving slot is cleared.
    frame = editorFrame(41, 90, 71, 2);
    rows = frame.value(QStringLiteral("windowRows")).toList();
    changed = rows.at(43).toMap();
    changed.insert(QStringLiteral("contentKey"),
                   QStringLiteral("editor-row-84-selected"));
    changed.insert(QStringLiteral("text"),
                   QStringLiteral("editor row 84 selected"));
    rows[43] = changed;
    frame.insert(QStringLiteral("windowRows"), rows);
    fixture.surface->setProperty("poolSlotWriteCount", 0);
    fixture.shell.setScene(documentScene(frame));
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "applyFrameWindow",
                                      Qt::DirectConnection));
    QCOMPARE(fixture.surface->property("poolSlotWriteCount").toInt(), 2);
    QCOMPARE(rowModel->property("count").toInt(), poolCount);
    QVERIFY(qAbs(topExtent(fixture.surface, fixture.list) - 71.0) < 0.01);
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
    QVERIFY(visibleRows > 0.0);
    QVERIFY(visibleRows < 30.0);
    QVERIFY(expectedVisibleSpan > 0.0);
    QVERIFY(expectedVisibleSpan < 750.0);
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
    const qreal visibleRows = fixture.list->height()
                              / fixture.surface->property("rowHeight").toReal();
    qInfo() << "editor end dpr/contentY/topExtent/expected/height"
            << fixture.window->devicePixelRatio() << fixture.list->property("contentY")
            << topExtent(fixture.surface, fixture.list) << (100.0 - visibleRows)
            << fixture.list->height();
    QVERIFY(visibleRows > 0.0);
    QVERIFY(visibleRows < 10.0);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.scrollBar->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(topExtent(fixture.surface, fixture.list)
             - (100.0 - visibleRows)) < 0.000001,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.scrollBar->property("position").toReal()
             - (1.0 - visibleRows / 100.0))
            < 0.000001,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.scrollBar->property("size").toReal()
             - visibleRows / 100.0)
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
    QVERIFY(cursor->width() > 2.0);
    QVERIFY(cursor->height() > cursor->width());
    const qreal cursorInViewport = cursor->mapToItem(fixture.list, 0, 0).y();
    QVERIFY(cursorInViewport >= 0);
    QVERIFY(cursorInViewport < fixture.list->height());

    frame.insert(QStringLiteral("cursorVisible"), false);
    frame.insert(QStringLiteral("windowGeneration"), 2);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(!cursor->isVisible(), 3000);

    frame.insert(QStringLiteral("cursorVisible"), true);
    frame.insert(QStringLiteral("cursorShape"), QStringLiteral("underline"));
    frame.insert(QStringLiteral("windowGeneration"), 3);
    fixture.shell.setScene(documentScene(frame));
    QTRY_COMPARE_WITH_TIMEOUT(cursor->width(), 2.0, 3000);
    QVERIFY(cursor->height() > cursor->width());
    QCOMPARE(cursor->property("color").value<QColor>(),
             fixture.window->property("textColor").value<QColor>());
    cursor->setProperty("blinkOn", false);
    auto *grid = fixture.window->findChild<TestGrid *>();
    QVERIFY(grid);
    emit grid->keyboardActivity();
    QTRY_VERIFY_WITH_TIMEOUT(cursor->property("blinkOn").toBool(), 1000);

    frame.insert(QStringLiteral("cursorAbsoluteRow"), 150);
    frame.insert(QStringLiteral("windowGeneration"), 4);
    fixture.shell.setScene(documentScene(frame));
    QTRY_COMPARE_WITH_TIMEOUT(cursor->property("windowRow").toInt(), -1, 3000);
    QVERIFY(!cursor->isVisible());
}

void F4DocumentSurfaceTests::documentCursorBlinkSettles_data()
{
    QTest::addColumn<QVariantMap>("frame");
    QTest::newRow("editor") << editorFrame(0, 60, 0, 1);
    QTest::newRow("terminal") << terminalFrame(0, 60, 0, 1);
}

void F4DocumentSurfaceTests::documentCursorBlinkSettles()
{
    QFETCH(QVariantMap, frame);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(fixture.window->isActive(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    QQuickItem *cursor = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT((cursor = findEditorCursor(fixture.surface)),
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->isVisible(), 3000);
    QVERIFY(cursor->setProperty("blinkInterval", 20));
    QVERIFY(QMetaObject::invokeMethod(cursor, "restartBlink"));
    QTRY_VERIFY_WITH_TIMEOUT(
        cursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        !cursor->property("blinkTimerRunning").toBool(), 500);
    QVERIFY(cursor->property("blinkOn").toBool());

    // The render thread can still have a finite tail of already-queued frames
    // after the GUI-thread timer stops. Drain that tail until there has been a
    // real quiet period, then observe longer than either caret interval so a
    // surviving blink loop cannot hide between two short waits.
    QSignalSpy settledFrames(fixture.window, &QQuickWindow::frameSwapped);
    QVERIFY(settledFrames.isValid());
    QElapsedTimer settleDeadline;
    QElapsedTimer quietPeriod;
    settleDeadline.start();
    quietPeriod.start();
    int observedFrames = 0;
    while (quietPeriod.elapsed() < 300 && settleDeadline.elapsed() < 3000) {
        QTest::qWait(10);
        if (settledFrames.size() != observedFrames) {
            observedFrames = settledFrames.size();
            quietPeriod.restart();
        }
    }
    QVERIFY2(quietPeriod.elapsed() >= 300,
             "document surface never reached frame quiescence");
    settledFrames.clear();
    QTest::qWait(700);
    QCOMPARE(settledFrames.size(), 0);

    auto *grid = fixture.window->findChild<TestGrid *>();
    QVERIFY(grid);
    emit grid->keyboardActivity();
    QTRY_VERIFY_WITH_TIMEOUT(
        cursor->property("blinkTimerRunning").toBool(), 500);

    // A retained document may stay instantiated under another surface. It
    // must settle immediately instead of finishing a hidden blink cycle.
    QVERIFY(fixture.surface->setProperty("interactionActive", false));
    QTRY_VERIFY_WITH_TIMEOUT(
        !cursor->property("blinkTimerRunning").toBool(), 500);
    QVERIFY(cursor->property("blinkOn").toBool());
}

void F4DocumentSurfaceTests::terminalScrollbackUsesBoundedWindowAndNativeViewport()
{
    DocumentFixture fixture(documentScene(
        terminalFrame(970, 90, 1000, 7)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.surface->property("displayedRows").toList().size(), 90, 3000);
    QVERIFY(fixture.surface->property("terminalSurface").toBool());
    QVERIFY(fixture.surface->property("hasWindowProtocol").toBool());
    QTRY_VERIFY_WITH_TIMEOUT(fixture.scrollBar->isVisible(), 3000);

    QVariantMap viewportAction;
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString()
                == QStringLiteral("terminal.viewport")) {
                viewportAction = action;
            }
        }
        return viewportAction.value(QStringLiteral("rows")).toInt() > 0;
    }(), 3000);
    QCOMPARE(viewportAction.value(QStringLiteral("target")).toString(),
             QStringLiteral("terminal-window-test"));

    fixture.shell.clearActions();
    const QPoint wheelPoint = fixture.list->mapToScene(
        QPointF(180, fixture.list->height() / 2)).toPoint();
    sendPixelWheel(fixture.window, wheelPoint, 120);
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString()
                == QStringLiteral("terminal.scroll")) {
                return action.value(QStringLiteral("visualRow")).toInt()
                       < 1000
                    && !action.value(QStringLiteral("followTail")).toBool()
                    && action.value(QStringLiteral("generation")).toInt() == 8;
            }
        }
        return false;
    }(), 3000);
}

void F4DocumentSurfaceTests::terminalFractionalRestDoesNotRequestCurrentRow()
{
    QVariantMap frame = terminalFrame(0, 40, 2, 7, 40, true);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    // A fractional contentY is a local presentation coordinate. The Go
    // terminal protocol accepts only an integer visualRow, so row 2.25 and
    // row 2 are the same remote destination and must not form an ACK loop.
    fixture.surface->setProperty("windowRequestPending", false);
    fixture.surface->setProperty("terminalFollowTailIntent", true);
    fixture.surface->setProperty("terminalFollowTailInitialized", true);
    fixture.shell.clearActions();

    QVariant accepted;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "sendWindowRequest", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, accepted),
        Q_ARG(QVariant, QVariant(2.25)),
        Q_ARG(QVariant, QVariant(0.25)),
        Q_ARG(QVariant, QVariant(0.0)),
        Q_ARG(QVariant, QVariant(true))));
    QVERIFY(!accepted.toBool());
    QTest::qWait(80);
    QVERIFY(fixture.shell.actions.isEmpty());

    // Crossing the visual-row boundary remains a real request and retains
    // the local sub-row placement for the eventual ACK.
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "sendWindowRequest", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, accepted),
        Q_ARG(QVariant, QVariant(3.25)),
        Q_ARG(QVariant, QVariant(0.25)),
        Q_ARG(QVariant, QVariant(0.0)),
        Q_ARG(QVariant, QVariant(true))));
    QVERIFY(accepted.toBool());
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("terminal.scroll"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("visualRow")).toInt(), 3);

    // While that request is in flight, another fractional presentation of
    // the same wire destination is not a second intent either.
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "sendWindowRequest", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, accepted),
        Q_ARG(QVariant, QVariant(3.75)),
        Q_ARG(QVariant, QVariant(0.75)),
        Q_ARG(QVariant, QVariant(0.0)),
        Q_ARG(QVariant, QVariant(true))));
    QVERIFY(!accepted.toBool());
    QCOMPARE(fixture.surface->property("pendingWindowIntent").toMap(),
             QVariantMap{});
    QCOMPARE(fixture.shell.actions.size(), 1);
}

void F4DocumentSurfaceTests::terminalCompleteWindowDoesNotPrefetchBeforeContentStart()
{
    DocumentFixture fixture(documentScene(
        terminalFrame(0, 40, 3, 7, 40, true)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const int loadedStart = fixture.surface
                                ->property("loadedSlotStart").toInt();
    fixture.surface->setProperty("rebasingWindow", true);
    fixture.list->setProperty("contentY",
                              (loadedStart + 2.25) * rowHeight);
    fixture.surface->setProperty("rebasingWindow", false);
    fixture.surface->setProperty("windowRequestPending", false);
    fixture.shell.clearActions();

    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "maybeRequestWindow",
                                      Qt::DirectConnection));
    QTest::qWait(80);
    for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
        QVERIFY2(action.value(QStringLiteral("action")).toString()
                     != QStringLiteral("terminal.scroll"),
                 "complete terminal window redundantly requested its own tail");
    }
}

void F4DocumentSurfaceTests::viewerFractionalRestDoesNotRequestCurrentOffset()
{
    DocumentFixture fixture(documentScene(
        viewerFrame(0, 40, 20, 7, 10, 1000)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    fixture.surface->setProperty("windowRequestPending", false);
    fixture.shell.clearActions();

    QVariant accepted;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "sendWindowRequest", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, accepted),
        Q_ARG(QVariant, QVariant(20.75)),
        Q_ARG(QVariant, QVariant(0.75)),
        Q_ARG(QVariant, QVariant(0.0)),
        Q_ARG(QVariant, QVariant(true))));
    QVERIFY(!accepted.toBool());
    QVERIFY(fixture.shell.actions.isEmpty());

    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "sendWindowRequest", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, accepted),
        Q_ARG(QVariant, QVariant(21.25)),
        Q_ARG(QVariant, QVariant(0.25)),
        Q_ARG(QVariant, QVariant(0.0)),
        Q_ARG(QVariant, QVariant(true))));
    QVERIFY(accepted.toBool());
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("viewer.scrollWindow"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("offset")).toInt(), 21);
}

void F4DocumentSurfaceTests::terminalFollowTailTracksVisibleEndAndUserScroll()
{
    QVariantMap frame = terminalFrame(950, 70, 1000, 7, 1020, true);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    QVERIFY(fixture.surface->property("terminalFollowTailIntent").toBool());

    const auto requestWindow = [&](qreal extent, bool preserveAnchor) {
        QVariant accepted;
        const qreal fraction = extent - std::floor(extent);
        const bool invoked = QMetaObject::invokeMethod(
            fixture.surface, "sendWindowRequest", Qt::DirectConnection,
            Q_RETURN_ARG(QVariant, accepted),
            Q_ARG(QVariant, QVariant(extent)),
            Q_ARG(QVariant, QVariant(fraction)),
            Q_ARG(QVariant, QVariant(0.0)),
            Q_ARG(QVariant, QVariant(preserveAnchor)));
        return invoked && accepted.toBool();
    };
    const auto lastAction = [&](const QString &name) {
        QVariantMap found;
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString() == name)
                found = action;
        }
        return found;
    };

    // A background overscan request can observe old local geometry while a
    // newer output frame is already authoritative. It must preserve follow
    // intent; only an actual wheel/drag/scrollbar gesture may suspend it.
    fixture.surface->setProperty("rebasingWindow", true);
    fixture.list->setProperty(
        "contentY",
        (fixture.surface->property("loadedSlotStart").toInt() + 2)
            * fixture.surface->property("rowHeight").toReal());
    fixture.surface->setProperty("rebasingWindow", false);
    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(fixture.surface, "maybeRequestWindow",
                                      Qt::DirectConnection));
    QTRY_VERIFY_WITH_TIMEOUT(
        !lastAction(QStringLiteral("terminal.scroll")).isEmpty(), 1000);
    QVariantMap action = lastAction(QStringLiteral("terminal.scroll"));
    QVERIFY(action.value(QStringLiteral("followTail")).toBool());
    QCOMPARE(action.value(QStringLiteral("generation")).toInt(), 8);
    frame = terminalFrame(950, 70, 1000, 8, 1020, true);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("windowRequestPending").toBool(), 3000);

    // The native viewport exposes more rows than the stale integer viewport
    // in this fixture. Its top is therefore before Go's maxTop even though the
    // final row is visible. The request must carry intent, not infer it from
    // equality with that stale coordinate.
    const qreal visibleTailTop = topExtent(fixture.surface, fixture.list);
    QVERIFY(visibleTailTop < 1000.0);
    fixture.shell.clearActions();
    QVERIFY(requestWindow(visibleTailTop, false));
    QTRY_VERIFY_WITH_TIMEOUT(
        !lastAction(QStringLiteral("terminal.scroll")).isEmpty(), 1000);
    action = lastAction(QStringLiteral("terminal.scroll"));
    QVERIFY(action.value(QStringLiteral("followTail")).toBool());
    QVERIFY(action.value(QStringLiteral("visualRow")).toInt() < 1000);
    QCOMPARE(action.value(QStringLiteral("generation")).toInt(), 9);

    frame = terminalFrame(950, 70, 1000, 9, 1020, true);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("windowRequestPending").toBool(), 3000);
    const qreal beforeLiveOutput = topExtent(fixture.surface, fixture.list);
    frame = terminalFrame(955, 70, 1005, 9, 1025, true);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        topExtent(fixture.surface, fixture.list) > beforeLiveOutput + 4.0,
        3000);

    // The first upward wheel sample suspends follow-tail immediately, before
    // the 180 ms window-request coalescer runs.
    fixture.shell.clearActions();
    const QPoint wheelPoint = fixture.list->mapToScene(
        QPointF(180, fixture.list->height() / 2)).toPoint();
    sendPixelWheel(fixture.window, wheelPoint, 120);
    QTRY_VERIFY_WITH_TIMEOUT(
        !lastAction(QStringLiteral("terminal.followTail")).isEmpty(), 1000);
    QVERIFY(!lastAction(QStringLiteral("terminal.followTail"))
                 .value(QStringLiteral("followTail")).toBool());
    QVERIFY(!fixture.surface->property("terminalFollowTailIntent").toBool());
    QTRY_VERIFY_WITH_TIMEOUT(
        !lastAction(QStringLiteral("terminal.scroll")).isEmpty(), 1500);
    action = lastAction(QStringLiteral("terminal.scroll"));
    QVERIFY(!action.value(QStringLiteral("followTail")).toBool());

    const int pinnedViewport = action.value(QStringLiteral("visualRow")).toInt();
    const int pinnedGeneration = action.value(QStringLiteral("generation")).toInt();
    frame = terminalFrame(qMax(0, pinnedViewport - 30), 70,
                          pinnedViewport, pinnedGeneration, 1025, false);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        !fixture.surface->property("windowRequestPending").toBool(), 3000);
    const qreal pinnedTop = topExtent(fixture.surface, fixture.list);
    frame = terminalFrame(qMax(0, pinnedViewport - 30), 70,
                          pinnedViewport, pinnedGeneration, 1030, false);
    fixture.shell.setScene(documentScene(frame));
    QTest::qWait(100);
    QVERIFY(qAbs(topExtent(fixture.surface, fixture.list) - pinnedTop) < 0.01);

    // A request that reaches the final row explicitly resumes follow-tail;
    // the next output frame then advances the native viewport again.
    fixture.shell.clearActions();
    QVERIFY(requestWindow(1030.0, false));
    action = lastAction(QStringLiteral("terminal.scroll"));
    QVERIFY(!action.isEmpty());
    QVERIFY(action.value(QStringLiteral("followTail")).toBool());
    QVERIFY(fixture.surface->property("terminalFollowTailIntent").toBool());
    const int resumeGeneration = action.value(QStringLiteral("generation")).toInt();
    // Unlike the intentionally stale geometry above, this final core ACK
    // honors terminal.viewport. Otherwise the native 30-row tail requests
    // another ACK for row 1000 while this fake core remains at row 1010.
    const int nativeRows = fixture.surface->property("reportedViewportRows").toInt();
    const int resumeViewport = 1030 - nativeRows;
    frame = terminalFrame(980, 50, resumeViewport, resumeGeneration, 1030, true);
    frame.insert("viewportSpan", nativeRows);
    frame.insert("viewportRows", nativeRows);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY2_WITH_TIMEOUT(
        !fixture.surface->property("windowRequestPending").toBool(),
        qPrintable(QString("tail ack %1, pending %2, requested %3, top %4, extent %5")
            .arg(resumeGeneration)
            .arg(fixture.surface->property("requestedGeneration").toInt())
            .arg(fixture.surface->property("requestedExtent").toReal())
            .arg(topExtent(fixture.surface, fixture.list))
            .arg(fixture.surface->property("contentExtent").toReal())), 3000);
    const qreal resumedTop = topExtent(fixture.surface, fixture.list);
    frame = terminalFrame(985, 50, resumeViewport + 5, resumeGeneration, 1035, true);
    frame.insert("viewportSpan", nativeRows);
    frame.insert("viewportRows", nativeRows);
    fixture.shell.setScene(documentScene(frame));
    QTRY_VERIFY_WITH_TIMEOUT(
        topExtent(fixture.surface, fixture.list) > resumedTop + 4.0, 3000);
}

void F4DocumentSurfaceTests::terminalDragSelectionSendsAbsoluteClipboardRange()
{
    DocumentFixture fixture(documentScene(
        terminalFrame(970, 90, 1000, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    fixture.shell.clearActions();

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const QPoint start = fixture.list->mapToScene(
        QPointF(24, rowHeight * 1.5)).toPoint();
    const QPoint end = fixture.list->mapToScene(
        QPointF(64, rowHeight * 3.5)).toPoint();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, start);
    QTest::mouseMove(fixture.window, end, 20);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, end);

    QVariantMap copyAction;
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("action")).toString()
                == QStringLiteral("terminal.copySelection")) {
                copyAction = action;
                return true;
            }
        }
        return false;
    }(), 3000);
    QCOMPARE(copyAction.value(QStringLiteral("target")).toString(),
             QStringLiteral("terminal-window-test"));
    QCOMPARE(copyAction.value(QStringLiteral("startRow")).toInt(), 1001);
    QCOMPARE(copyAction.value(QStringLiteral("endRow")).toInt(), 1003);
    QVERIFY(copyAction.value(QStringLiteral("endColumn")).toInt()
            > copyAction.value(QStringLiteral("startColumn")).toInt());
    QVERIFY(copyAction.value(QStringLiteral("endExclusive")).toBool());
    QVERIFY(fixture.surface->property("terminalSelectionVisible").toBool());
    QVERIFY(!fixture.surface->property("terminalSelectionDragging").toBool());
}

void F4DocumentSurfaceTests::terminalSelectionUsesNearestInsertionBoundary()
{
    DocumentFixture fixture(documentScene(
        terminalFrame(970, 90, 1000, 1)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    const qreal cellWidth = fixture.surface
                                ->property("terminalCellWidth")
                                .toReal();
    const qreal inset = fixture.surface
                            ->property("textHorizontalInset")
                            .toReal();
    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    QVariant leftHalf;
    QVariant rightHalf;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "terminalSelectionPointAt", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, leftHalf),
        Q_ARG(QVariant, QVariant(inset + 4.25 * cellWidth)),
        Q_ARG(QVariant, QVariant(1.5 * rowHeight))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "terminalSelectionPointAt", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, rightHalf),
        Q_ARG(QVariant, QVariant(inset + 4.75 * cellWidth)),
        Q_ARG(QVariant, QVariant(1.5 * rowHeight))));
    QCOMPARE(leftHalf.toMap().value(QStringLiteral("cellColumn")).toInt(), 4);
    QCOMPARE(rightHalf.toMap().value(QStringLiteral("cellColumn")).toInt(), 4);
    QCOMPARE(leftHalf.toMap().value(QStringLiteral("column")).toInt(), 4);
    QCOMPARE(rightHalf.toMap().value(QStringLiteral("column")).toInt(), 5);

    QVariant range;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "beginTerminalSelectionAt", Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(1001)), Q_ARG(QVariant, QVariant(4))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "extendTerminalSelectionTo", Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(1001)), Q_ARG(QVariant, QVariant(1))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "terminalSelectionRangeForRow", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, range), Q_ARG(QVariant, QVariant(1001)),
        Q_ARG(QVariant, QVariant(fixture.list->width()))));
    QCOMPARE(range.toMap().value(QStringLiteral("start")).toInt(), 1);
    QCOMPARE(range.toMap().value(QStringLiteral("end")).toInt(), 4);

    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "beginTerminalSelectionAt", Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(1001)), Q_ARG(QVariant, QVariant(5))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "extendTerminalSelectionTo", Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(1001)), Q_ARG(QVariant, QVariant(8))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "terminalSelectionRangeForRow", Qt::DirectConnection,
        Q_RETURN_ARG(QVariant, range), Q_ARG(QVariant, QVariant(1001)),
        Q_ARG(QVariant, QVariant(fixture.list->width()))));
    QCOMPARE(range.toMap().value(QStringLiteral("start")).toInt(), 5);
    QCOMPARE(range.toMap().value(QStringLiteral("end")).toInt(), 8);

    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(fixture.surface,
                                      "commitTerminalSelection",
                                      Qt::DirectConnection));
    QTRY_VERIFY_WITH_TIMEOUT(!fixture.shell.actions.isEmpty(), 1000);
    const QVariantMap action = fixture.shell.actions.constLast();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("terminal.copySelection"));
    QVERIFY(action.value(QStringLiteral("endExclusive")).toBool());
}

void F4DocumentSurfaceTests::terminalDoubleAndTripleClickSelectWordAndParagraph()
{
    QVariantMap frame = terminalFrame(970, 90, 1000, 1);
    QVariantList rows = frame.value(QStringLiteral("windowRows")).toList();
    for (int absoluteRow = 1001; absoluteRow < 1004; ++absoluteRow) {
        const int index = absoluteRow - 970;
        QVariantMap row = rows.at(index).toMap();
        row.insert(QStringLiteral("logicalRowStart"), 1001);
        row.insert(QStringLiteral("logicalRowEnd"), 1004);
        if (absoluteRow == 1002)
            row.insert(QStringLiteral("text"), QStringLiteral("alpha beta gamma"));
        rows[index] = row;
    }
    frame.insert(QStringLiteral("windowRows"), rows);

    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);

    QVariant clickCount;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "handleTerminalSelectionPressAt",
        Qt::DirectConnection, Q_RETURN_ARG(QVariant, clickCount),
        Q_ARG(QVariant, QVariant(1002)), Q_ARG(QVariant, QVariant(7)),
        Q_ARG(QVariant, QVariant(7)), Q_ARG(QVariant, QVariant(1000.0))));
    QCOMPARE(clickCount.toInt(), 1);
    QVERIFY(QMetaObject::invokeMethod(fixture.surface,
                                      "commitTerminalSelection",
                                      Qt::DirectConnection));

    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "handleTerminalSelectionPressAt",
        Qt::DirectConnection, Q_RETURN_ARG(QVariant, clickCount),
        Q_ARG(QVariant, QVariant(1002)), Q_ARG(QVariant, QVariant(7)),
        Q_ARG(QVariant, QVariant(7)), Q_ARG(QVariant, QVariant(1200.0))));
    QCOMPARE(clickCount.toInt(), 2);
    QVERIFY(fixture.surface->property("terminalSelectionVisible").toBool());
    QVERIFY(!fixture.surface->property("terminalSelectionDragging").toBool());
    QCOMPARE(fixture.surface->property("terminalSelectionAnchorRow").toInt(),
             1002);
    QCOMPARE(fixture.surface->property("terminalSelectionFocusRow").toInt(),
             1002);
    QCOMPARE(fixture.surface
                 ->property("terminalSelectionAnchorColumn").toInt(), 6);
    QCOMPARE(fixture.surface
                 ->property("terminalSelectionFocusColumn").toInt(), 10);

    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "handleTerminalSelectionPressAt",
        Qt::DirectConnection, Q_RETURN_ARG(QVariant, clickCount),
        Q_ARG(QVariant, QVariant(1002)), Q_ARG(QVariant, QVariant(7)),
        Q_ARG(QVariant, QVariant(7)), Q_ARG(QVariant, QVariant(1350.0))));
    QCOMPARE(clickCount.toInt(), 3);
    QCOMPARE(fixture.surface->property("terminalSelectionAnchorRow").toInt(),
             1001);
    QCOMPARE(fixture.surface->property("terminalSelectionFocusRow").toInt(),
             1003);
    QCOMPARE(fixture.surface
                 ->property("terminalSelectionAnchorColumn").toInt(), 0);
    QCOMPARE(fixture.surface
                 ->property("terminalSelectionFocusColumn").toInt(), 90);
}

void F4DocumentSurfaceTests::terminalDragSelectionAutoScrollsBeyondViewportByDistance()
{
    DocumentFixture fixture(documentScene(
        terminalFrame(930, 130, 1000, 1, 1030, true)));
    QVERIFY(fixture.ready());
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.surface->property("windowInitialized").toBool(), 3000);
    fixture.shell.clearActions();

    const qreal rowHeight = fixture.surface->property("rowHeight").toReal();
    const qreal initialTop = topExtent(fixture.surface, fixture.list);
    const QPoint start = fixture.list->mapToScene(
        QPointF(48, fixture.list->height() / 2)).toPoint();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, start);
    QVERIFY(fixture.surface->property("terminalSelectionDragging").toBool());
    const int anchor = fixture.surface
                           ->property("terminalSelectionAnchorRow")
                           .toInt();

    QVariant nearVelocity;
    QVariant farVelocity;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "terminalSelectionAutoScrollVelocity",
        Qt::DirectConnection, Q_RETURN_ARG(QVariant, nearVelocity),
        Q_ARG(QVariant, QVariant(-rowHeight / 2))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "terminalSelectionAutoScrollVelocity",
        Qt::DirectConnection, Q_RETURN_ARG(QVariant, farVelocity),
        Q_ARG(QVariant, QVariant(-rowHeight * 4))));
    QVERIFY(nearVelocity.toReal() < 0);
    QVERIFY(farVelocity.toReal() < nearVelocity.toReal());
    QVERIFY(qAbs(farVelocity.toReal()) > qAbs(nearVelocity.toReal()) * 3);

    // Model a grabbed pointer above the viewport. MouseArea continues to own
    // the real pointer grab outside its bounds; invoking the same QML handler
    // directly keeps this offscreen test independent of window-system clipping.
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "updateTerminalSelectionPointer",
        Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(48.0)),
        Q_ARG(QVariant, QVariant(-rowHeight * 4))));
    QVERIFY(fixture.surface
                ->property("terminalSelectionAutoScrollDistance")
                .toReal() < 0);
    QObject *timer = fixture.surface->findChild<QObject *>(
        QStringLiteral("terminalSelectionAutoScrollTimer"));
    QVERIFY(timer);
    QTRY_VERIFY_WITH_TIMEOUT(timer->property("running").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        topExtent(fixture.surface, fixture.list) < initialTop - 2.0, 1500);
    QVERIFY(fixture.surface->property("terminalSelectionFocusRow").toInt()
            < anchor);

    QVariantMap followAction;
    for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
        if (action.value(QStringLiteral("action")).toString()
            == QStringLiteral("terminal.followTail")) {
            followAction = action;
        }
    }
    QVERIFY(!followAction.isEmpty());
    QVERIFY(!followAction.value(QStringLiteral("followTail")).toBool());

    // A dialog can cover the retained terminal without changing the terminal
    // frame itself. It must synchronously retire the pointer-owned repeating
    // timer instead of continuing to request invisible frames underneath.
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), QVariantList{QVariantMap{
             {QStringLiteral("id"), QStringLiteral("terminal-selection-dialog")},
             {QStringLiteral("kind"), QStringLiteral("dialog")},
             {QStringLiteral("title"), QStringLiteral("Blocking")},
             {QStringLiteral("x"), 4},
             {QStringLiteral("y"), 2},
             {QStringLiteral("w"), 30},
             {QStringLiteral("h"), 10},
             {QStringLiteral("controls"), QVariantList{}},
             {QStringLiteral("buttons"), QVariantList{}},
         }}},
    }, 200);
    QTRY_VERIFY_WITH_TIMEOUT(!timer->property("running").toBool(), 1000);
    QVERIFY(!fixture.surface
                 ->property("terminalSelectionDragging").toBool());
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), QVariantList{}},
    }, 201);
    QCoreApplication::processEvents();

    QVERIFY(QMetaObject::invokeMethod(fixture.surface,
                                      "commitTerminalSelection",
                                      Qt::DirectConnection));
    QVERIFY(!timer->property("running").toBool());
    QCOMPARE(fixture.surface
                 ->property("terminalSelectionAutoScrollDistance")
                 .toReal(),
             0.0);

    const qreal upwardTop = topExtent(fixture.surface, fixture.list);
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "beginTerminalSelectionAt", Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(std::floor(upwardTop))),
        Q_ARG(QVariant, QVariant(4))));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.surface, "updateTerminalSelectionPointer",
        Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(48.0)),
        Q_ARG(QVariant, QVariant(fixture.list->height() + rowHeight * 3))));
    QTRY_VERIFY_WITH_TIMEOUT(
        topExtent(fixture.surface, fixture.list) > upwardTop + 2.0, 1500);
    QVERIFY(QMetaObject::invokeMethod(fixture.surface,
                                      "commitTerminalSelection",
                                      Qt::DirectConnection));
    QVERIFY(!timer->property("running").toBool());
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, start);
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
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("version"), 4},
        {QStringLiteral("presentation"), QStringLiteral("qml")},
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("newTab"), QVariantMap{}},
             {QStringLiteral("counter"), QVariantMap{}},
         }},
        // A bounded descriptor may still carry its initial rows without the
        // optional document-window paging extension. v4 no longer discovers
        // document surfaces through the former top-level frames stack.
        {QStringLiteral("surface"), legacyFrame},
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

void F4DocumentSurfaceTests::middleButtonAutoScrollsEmbeddedTerminal()
{
    auto frame = terminalFrame(0, 140, 40, 7, 140, true);
    DocumentFixture fixture(documentScene(frame));
    QVERIFY(fixture.ready());
    fixture.surface->setProperty("embedded", true);
    QTRY_VERIFY(fixture.surface->property("windowInitialized").toBool());
    auto *scroll = fixture.surface->findChild<QObject *>("documentMouseAutoScrollController");
    QVERIFY(scroll);
    const QPoint center = fixture.list->mapToItem(fixture.window->contentItem(),
        QPointF(fixture.list->width()/2,fixture.list->height()/2)).toPoint();
    const QPoint upper = center - QPoint(0, 90);
    fixture.shell.clearActions();
    fixture.gallery.clearScrollingCursorRequests();
    const qreal before = fixture.list->property("contentY").toReal();
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY_WITH_TIMEOUT(scroll->property("scrollingMode").toBool(), 1000);
    QVERIFY(!fixture.gallery.scrollingCursorRequests.isEmpty());
    QVERIFY(fixture.gallery.scrollingCursorRequests.last().value("scrollingMode").toBool());
    QTest::mouseMove(fixture.window, upper, 20);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.list->property("contentY").toReal() < before - 1, 1500);
    bool requested = false;
    QTRY_VERIFY_WITH_TIMEOUT(([&] {
        for (const auto &action : std::as_const(fixture.shell.actions)) {
            if (action.value("action").toString() == "terminal.scroll" && !action.value("followTail").toBool())
                requested = true;
        }
        return requested;
    })(), 1500);
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, upper);
    QTRY_VERIFY(!scroll->property("scrollingMode").toBool());
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, center);
    QTRY_VERIFY(scroll->property("scrollingMode").toBool());
    fixture.surface->setProperty("interactionActive", false);
    QTRY_VERIFY(!scroll->property("scrollingMode").toBool());
    QVERIFY(!fixture.gallery.scrollingCursorRequests.last().value("scrollingMode").toBool());
}

void F4DocumentSurfaceTests::shortEditorSelectionDoesNotScroll()
{
    auto frame = editorFrame(0, 12, 0, 1, 12);
    frame.insert("viewportRows", 45);
    frame.insert("viewportSpan", 12);
    auto rows = frame.value("windowRows").toList();
    for (int i = 0; i < rows.size(); ++i) {
        auto row = rows[i].toMap();
        row.insert("visualWidth", 20);
        rows[i] = row;
    }
    frame.insert("windowRows", rows);
    DocumentFixture fixture(documentScene(QVariantMap{}), 600);
    QVERIFY(fixture.ready());
    QVERIFY(fixture.window->setProperty("ch", 23.0));
    QTest::qWait(100);
    fixture.window->resize(1200, 1186);
    QTest::qWait(100);
    frame.insert("layoutRevision", 2);
    frame.insert("geometryRevision", 100);
    fixture.shell.setScene(documentScene(frame));
    QTest::qWait(100);
    const auto firstRowY = [&fixture]() {
        QList<QQuickItem *> pending{fixture.list};
        while (!pending.isEmpty()) {
            auto *item = pending.takeLast();
            pending.append(item->childItems());
            if (item->objectName() == "documentRowDelegate"
                    && item->property("loaded").toBool()
                    && item->property("rowData").toMap().value("visualRow").toInt() == 0)
                return item->mapToItem(fixture.list, QPointF()).y();
        }
        return -99999.0;
    };
    const qreal initialRowY = firstRowY();
    // At 175%, the old contentY/rowHeight calculation reported row 1.54375
    // here and moved the actual first row from 0 to -35.2857 logical px.
    // A real delegate coordinate catches this even if the reported top clamps.

    QVERIFY(qAbs(initialRowY) < 0.001);
    for (int step = 0; step < 30; ++step) {
        const int row = step < 15 ? qMin(step, 11) : qMax(0, 26 - step);
        frame.insert("selection", true);
        frame.insert("selectionAnchorRow", 0);
        frame.insert("selectionAnchorColumn", 0);
        frame.insert("cursorAbsoluteRow", row);
        frame.insert("cursorAbsoluteColumn", 5);
        // Occurrence highlighting changes base row styles while selecting.
        auto styledRows = frame.value("windowRows").toList();
        for (int i = 0; i < styledRows.size(); ++i) {
            auto styledRow = styledRows[i].toMap();
            styledRow.remove("text");
            styledRow.insert("contentKey", QString("selection-%1-%2").arg(step).arg(i));
            styledRow.insert("runs", QVariantList{QVariantMap{
                {"text", "repeated selection text"},
                {"foreground", "#ffffff"},
                {"background", step % 2 ? "#663366" : "#222222"}}});
            styledRows[i] = styledRow;
        }
        frame.insert("windowRows", styledRows);
        frame.insert("windowContentKey", QString("selection-%1").arg(step));
        // Exercise both full publication and the keyboard's compact update.
        if (step % 2 == 0)
            fixture.shell.setScene(documentScene(frame));
        else {
            fixture.shell.surfaceRegistry()->applyDocumentState(frame, step + 2);
            emit fixture.shell.compactPresentationChanged({{"surfaceState", frame}});
        }
        QTest::qWait(30);
        QVERIFY2(qAbs(firstRowY() - initialRowY) < 0.001,
                 qPrintable(QString("First row jumped from %1 to %2").arg(initialRowY).arg(firstRowY())));
        QCOMPARE(topExtent(fixture.surface, fixture.list), 0.0);
        int visibleClips = 0;
        QList<QQuickItem *> pending{fixture.surface};
        while (!pending.isEmpty()) {
            auto *leaf = pending.takeLast();
            pending.append(leaf->childItems());
            if (leaf->isVisible() && (leaf->objectName() == "documentRunText"
                    || leaf->objectName() == "documentPlainText"
                    || leaf->objectName() == "documentEditorSelectedText")) {
                const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
                const qreal dpr = fixture.window->devicePixelRatio();
                QVERIFY(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001);
                QVERIFY(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001);
                const auto dx = leaf->mapToItem(fixture.window->contentItem(), QPointF(1, 0)) - origin;
                const auto dy = leaf->mapToItem(fixture.window->contentItem(), QPointF(0, 1)) - origin;
                QVERIFY(QLineF(dx, QPointF(1, 0)).length() < 0.001);
                QVERIFY(QLineF(dy, QPointF(0, 1)).length() < 0.001);
            }
            if (leaf->objectName() == "documentEditorSelectionClip"
                    && leaf->isVisible() && leaf->width() > 0)
                ++visibleClips;
        }
        QVERIFY(visibleClips > 0);
    }
    QVERIFY(fixture.window->grabWindow().save(".diagnostics/short-editor-selection-175.png"));
}
