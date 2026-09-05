#include "DummyQWK.h"
#include "TestExtUiStateController.h"

#include <QAccessible>
#include <QColor>
#include <QCoreApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QUrl>
#include <QUrlQuery>
#include <QVariantList>
#include <QVariantMap>
#include <QWheelEvent>
#include <QtQml>
#include <QtTest>

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
    void setPointerInputEnabled(bool value) { m_pointerInputEnabled = value; }
    bool inputMethodForwardingEnabled() const
    {
        return m_inputMethodForwardingEnabled;
    }
    void setInputMethodForwardingEnabled(bool value)
    {
        m_inputMethodForwardingEnabled = value;
    }
    bool terminalInputEnabled() const { return m_terminalInputEnabled; }
    void setTerminalInputEnabled(bool value) { m_terminalInputEnabled = value; }
    bool renderingEnabled() const { return m_renderingEnabled; }
    void setRenderingEnabled(bool value) { m_renderingEnabled = value; }

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
    bool m_renderingEnabled = true;
};

class TestShell final : public TestExtUiStateController
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)

public:
    int initialCols() const { return 100; }
    int initialRows() const { return 34; }

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
    Q_INVOKABLE QUrl rasterizedLucideSource(const QString &name,
                                            int logicalSize,
                                            qreal devicePixelRatio,
                                            const QColor &tint) const
    {
        QUrl source(QStringLiteral("qrc:/F4QtHost/icons/lucide/%1.svg")
                        .arg(name));
        QUrlQuery query;
        query.addQueryItem(QStringLiteral("size"),
                           QString::number(logicalSize));
        query.addQueryItem(QStringLiteral("dpr"),
                           QString::number(devicePixelRatio, 'g', 12));
        query.addQueryItem(QStringLiteral("color"),
                           tint.name(QColor::HexArgb));
        source.setQuery(query);
        return source;
    }
    Q_INVOKABLE QUrl fileIconSource(const QString &, const QString &, bool,
                                    int, qreal, qlonglong) const
    {
        return {};
    }
};

QVariantMap workspaceTabs(bool queueActive, bool queueClosable = true)
{
    return {
        {QStringLiteral("visible"), true},
        {QStringLiteral("tabs"), QVariantList{
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("commander-tab")},
                 {QStringLiteral("index"), 0},
				 {QStringLiteral("number"), 1},
                 {QStringLiteral("text"), QStringLiteral("Commander")},
                 {QStringLiteral("surfaceKind"), QStringLiteral("panels")},
                 {QStringLiteral("iconName"), QStringLiteral("panels-top-left")},
                 {QStringLiteral("active"), !queueActive},
                 {QStringLiteral("closable"), false},
                 {QStringLiteral("action"), QStringLiteral("workspace.activate")},
             },
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("queue-tab")},
                 {QStringLiteral("index"), 1},
				 {QStringLiteral("number"), 2},
                 {QStringLiteral("text"), QStringLiteral("Queue")},
                 {QStringLiteral("surfaceKind"), QStringLiteral("operationsQueue")},
                 {QStringLiteral("iconName"), QStringLiteral("list-checks")},
                 {QStringLiteral("active"), queueActive},
                 {QStringLiteral("closable"), queueClosable},
                 {QStringLiteral("action"), QStringLiteral("workspace.activate")},
                 {QStringLiteral("closeAction"), QStringLiteral("workspace.close")},
             },
         }},
        {QStringLiteral("newTab"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("workspace-new")},
             {QStringLiteral("visible"), true},
             {QStringLiteral("action"), QStringLiteral("workspace.new")},
         }},
        {QStringLiteral("counter"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("workspace-screen-counter")},
             {QStringLiteral("text"), QStringLiteral("2 screens")},
             {QStringLiteral("visible"), true},
             {QStringLiteral("action"), QStringLiteral("workspace.list")},
         }},
    };
}

QVariantMap panelScene()
{
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("workspaceTabs"), workspaceTabs(false)},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("panels")},
             {QStringLiteral("terminalActive"), false},
             {QStringLiteral("showLeftPanel"), true},
             {QStringLiteral("showRightPanel"), true},
             {QStringLiteral("panels"), QVariantList{}},
         }},
    };
}

QVariantMap filePanel(int side, bool loading,
                      const QString &sortMode = QStringLiteral("name"),
                      bool sortReverse = false)
{
    return {
        {QStringLiteral("id"), QStringLiteral("panel-%1").arg(side)},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), side},
        {QStringLiteral("active"), side == 0},
        {QStringLiteral("path"), QStringLiteral("/Users/zoin/Documents")},
        {QStringLiteral("title"), QStringLiteral("/Users/zoin/Documents")},
        {QStringLiteral("viewMode"), QStringLiteral("detailed")},
        {QStringLiteral("viewModeName"), QStringLiteral("detailed")},
        {QStringLiteral("presentation"), QStringLiteral("list")},
        {QStringLiteral("galleryLayoutMode"), QStringLiteral("masonry")},
        {QStringLiteral("sortModeName"), sortMode},
        {QStringLiteral("sortReverse"), sortReverse},
        {QStringLiteral("sourceKind"), QStringLiteral("local")},
        {QStringLiteral("loading"), loading},
        {QStringLiteral("entries"), QVariantList{}},
        {QStringLiteral("columns"), QVariantList{}},
    };
}

QVariantMap loadingPanelScene(
    bool loading, const QString &sortMode = QStringLiteral("name"),
    bool sortReverse = false)
{
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("workspaceTabs"), workspaceTabs(false)},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("panels")},
             {QStringLiteral("terminalActive"), false},
             {QStringLiteral("showLeftPanel"), true},
             {QStringLiteral("showRightPanel"), true},
             {QStringLiteral("panels"), QVariantList{
                  filePanel(0, loading, sortMode, sortReverse),
                  filePanel(1, false)}},
         }},
    };
}

QVariantMap terminalScene()
{
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("workspaceTabs"), workspaceTabs(false)},
        {QStringLiteral("keyBar"), QVariantMap{
             {QStringLiteral("visible"), true},
             {QStringLiteral("items"), QVariantList{
                  QVariantMap{
                      {QStringLiteral("key"), QStringLiteral("F1")},
                      {QStringLiteral("text"), QStringLiteral("Help")},
                  },
              }},
         }},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("panels")},
             {QStringLiteral("terminalActive"), true},
             {QStringLiteral("terminalBusy"), true},
             {QStringLiteral("showKeyBar"), true},
             {QStringLiteral("showLeftPanel"), true},
             {QStringLiteral("showRightPanel"), true},
             {QStringLiteral("panels"), QVariantList{}},
             {QStringLiteral("commandLine"), QVariantMap{
                  {QStringLiteral("id"), QStringLiteral("command-line")},
                  {QStringLiteral("visible"), true},
                  {QStringLiteral("prompt"), QStringLiteral("f4 % ")},
                  {QStringLiteral("text"), QString()},
              }},
             {QStringLiteral("terminal"), QVariantMap{
                  {QStringLiteral("id"), QStringLiteral("terminal")},
                  {QStringLiteral("rows"), QVariantList{}},
              }},
         }},
    };
}

QVariantMap task(int id, const QString &state, int progress,
                 bool cancellable = true, bool hasDetails = true)
{
    const bool terminal = state == QStringLiteral("Done")
            || state == QStringLiteral("Error")
            || state == QStringLiteral("Cancelled");
    return {
        {QStringLiteral("id"), QStringLiteral("queue-task-%1").arg(id)},
        {QStringLiteral("taskId"), id},
        {QStringLiteral("index"), id - 1},
        {QStringLiteral("type"), id % 2 ? QStringLiteral("Copy")
                                         : QStringLiteral("Move")},
        {QStringLiteral("description"),
         QStringLiteral("Operation number %1").arg(id)},
        {QStringLiteral("state"), state},
        {QStringLiteral("stateClass"), state.toLower()},
        {QStringLiteral("currentFile"),
         QStringLiteral("/tmp/file-%1.dat").arg(id)},
        {QStringLiteral("displayText"),
         QStringLiteral("Processing file-%1.dat").arg(id)},
        {QStringLiteral("progress"), progress},
        {QStringLiteral("totalText"), QStringLiteral("%1 / 100 MiB").arg(progress)},
        {QStringLiteral("speed"), QStringLiteral("42 MiB/s")},
        {QStringLiteral("error"),
         state == QStringLiteral("Error")
             ? QStringLiteral("The source disappeared") : QString()},
        {QStringLiteral("cancellable"), cancellable && !terminal},
        {QStringLiteral("hasDetails"), hasDetails},
        {QStringLiteral("terminal"), terminal},
        {QStringLiteral("active"), !terminal},
    };
}

QVariantMap queueModel(const QVariantList &items, int selectedTaskId,
                       bool hasActive = true, const QString &error = {})
{
    int selected = 0;
    int terminalCount = 0;
    int errorCount = 0;
    for (int i = 0; i < items.size(); ++i) {
        const QVariantMap item = items.at(i).toMap();
        if (item.value(QStringLiteral("taskId")).toInt() == selectedTaskId)
            selected = i;
        if (item.value(QStringLiteral("terminal")).toBool())
            ++terminalCount;
        if (item.value(QStringLiteral("state")).toString()
            == QStringLiteral("Error"))
            ++errorCount;
    }
    return {
        {QStringLiteral("id"), QStringLiteral("operations-queue")},
        {QStringLiteral("kind"), QStringLiteral("operationsQueue")},
        {QStringLiteral("title"), QStringLiteral("Operations Queue")},
        {QStringLiteral("selected"), selected},
        {QStringLiteral("selectedTaskId"), selectedTaskId},
        {QStringLiteral("tabId"), QStringLiteral("queue-tab")},
        {QStringLiteral("runningCount"), hasActive ? 1 : 0},
        {QStringLiteral("queuedCount"), qMax(0, items.size() - terminalCount - 1)},
        {QStringLiteral("completedCount"), terminalCount},
        {QStringLiteral("errorCount"), errorCount},
        {QStringLiteral("hasActive"), hasActive},
        {QStringLiteral("canClear"), terminalCount > 0},
        {QStringLiteral("canClose"), !hasActive},
        {QStringLiteral("cancelText"), QStringLiteral("Cancel selected")},
        {QStringLiteral("clearText"), QStringLiteral("Clear completed")},
        {QStringLiteral("emptyText"), QStringLiteral("No operations")},
        {QStringLiteral("detailHint"),
         QStringLiteral("Enter or double-click to open details")},
        {QStringLiteral("error"), error},
        {QStringLiteral("items"), items},
    };
}

QVariantMap queueScene(const QVariantMap &queue)
{
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("workspaceTabs"),
         workspaceTabs(true, queue.value(QStringLiteral("canClose")).toBool())},
        {QStringLiteral("operationsQueue"), queue},
    };
}

QVariantMap documentScene()
{
    QVariantList rows;
    for (int i = 0; i < 40; ++i) {
        rows.append(QVariantMap{
            {QStringLiteral("offset"), i * 10},
            {QStringLiteral("endOffset"), (i + 1) * 10},
            {QStringLiteral("text"), QStringLiteral("row %1").arg(i)},
        });
    }
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("workspaceTabs"), workspaceTabs(false)},
        {QStringLiteral("surface"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("retained-document")},
             {QStringLiteral("kind"), QStringLiteral("viewer")},
             {QStringLiteral("rows"), rows},
         }},
    };
}

QVariantMap dialogScene()
{
    QVariantMap scene = panelScene();
    scene.insert(QStringLiteral("dialogs"), QVariantList{
        QVariantMap{
            {QStringLiteral("id"), QStringLiteral("appearance-dialog")},
            {QStringLiteral("kind"), QStringLiteral("dialog")},
            {QStringLiteral("title"), QStringLiteral("Appearance")},
            {QStringLiteral("x"), 18},
            {QStringLiteral("y"), 3},
            {QStringLiteral("w"), 64},
            {QStringLiteral("h"), 27},
            {QStringLiteral("showClose"), true},
            {QStringLiteral("children"), QVariantList{
                 QVariantMap{
                     {QStringLiteral("id"), QStringLiteral("appearance-label")},
                     {QStringLiteral("kind"), QStringLiteral("text")},
                     {QStringLiteral("text"), QStringLiteral("Appearance content")},
                     {QStringLiteral("x"), 2},
                     {QStringLiteral("y"), 2},
                     {QStringLiteral("w"), 40},
                     {QStringLiteral("h"), 1},
                 },
                 QVariantMap{
                     {QStringLiteral("id"), QStringLiteral("appearance-checkbox")},
                     {QStringLiteral("kind"), QStringLiteral("checkbox")},
                     {QStringLiteral("text"), QStringLiteral("Show hidden files")},
                     {QStringLiteral("x"), 2},
                     {QStringLiteral("y"), 4},
                     {QStringLiteral("w"), 40},
                     {QStringLiteral("h"), 1},
                 },
                 QVariantMap{
                     {QStringLiteral("id"), QStringLiteral("appearance-navigation")},
                     {QStringLiteral("kind"), QStringLiteral("radioGroup")},
                     {QStringLiteral("items"), QVariantList{
                          QStringLiteral("First"), QStringLiteral("Second")}},
                     {QStringLiteral("selected"), 0},
                     {QStringLiteral("x"), 2},
                     {QStringLiteral("y"), 7},
                     {QStringLiteral("w"), 40},
                     {QStringLiteral("h"), 3},
                 },
             }},
        },
    });
    return scene;
}

QPoint itemCenter(QQuickItem *item)
{
    const QPointF scenePoint = item->mapToScene(
        QPointF(item->width() / 2.0, item->height() / 2.0));
    return scenePoint.toPoint();
}

QQuickItem *visualItemWithText(QQuickItem *root, const QString &text)
{
    if (!root)
        return nullptr;
    if (root->property("text").isValid()
        && root->property("text").toString() == text) {
        return root;
    }
    for (QQuickItem *child : root->childItems()) {
        if (QQuickItem *match = visualItemWithText(child, text))
            return match;
    }
    return nullptr;
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

QQuickItem *queueDelegate(QQuickItem *surface, int taskId)
{
    QVariant result;
    const bool invoked = QMetaObject::invokeMethod(
        surface, "delegateForTaskId",
        Q_RETURN_ARG(QVariant, result),
        Q_ARG(QVariant, QVariant(taskId)));
    return invoked ? qobject_cast<QQuickItem *>(result.value<QObject *>())
                   : nullptr;
}

QQuickItem *visualItem(QQuickItem *root, const QString &objectName)
{
    if (!root) {
        return nullptr;
    }
    if (root->objectName() == objectName) {
        return root;
    }
    for (QQuickItem *child : root->childItems()) {
        if (QQuickItem *match = visualItem(child, objectName)) {
            return match;
        }
    }
    return nullptr;
}

struct QueueFixture
{
    TestShell shell;
    TestGallery gallery;
    TestIcons icons;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;

    explicit QueueFixture(const QVariantMap &scene, bool usesQwk = false)
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
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4GuiFontPixelSize"), 13);
        engine.rootContext()->setContextProperty(QStringLiteral("f4UsesQwk"),
                                                  usesQwk);
        DummyQWK::registerTypes(&engine);
        engine.load(QUrl(QStringLiteral("qrc:/F4QtHost/qml/main.qml")));
        if (engine.rootObjects().isEmpty())
            return;
        window = qobject_cast<QQuickWindow *>(engine.rootObjects().constFirst());
        if (!window)
            return;
        window->resize(1024, 720);
        window->show();
        window->requestActivate();
        QCoreApplication::processEvents();
    }

    QQuickItem *item(const QString &name) const
    {
        return window ? window->findChild<QQuickItem *>(name) : nullptr;
    }

    QObject *object(const QString &name) const
    {
        return window ? window->findChild<QObject *>(name) : nullptr;
    }
};
}

class F4OperationsQueueTests final : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();
    void queueUsesNativeAccessibleSurfaceAndGuardsActiveClose();
    void plusButtonIsInteractiveInsideQwkTitleBar();
    void progressUpdatesKeepModelAndDelegateIdentity();
    void keyboardMouseAndClearActionsUseStableTaskIdentity();
    void wheelScrollsNativelyAndEmptyErrorStateIsVisible();
    void queueTabPreservesPanelDocumentAndQueueViewState();
    void terminalModeKeepsPersistentPanelsSurfaceVisible();
    void panelLoadingPulseIsDelayedLocalAndDoesNotMoveRendererButton();
    void rendererPopupClosesOnOutsidePress();
    void semanticDialogsMoveResizeAndUseZoinWindowButtons();
};

void F4OperationsQueueTests::initTestCase()
{
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    qmlRegisterType<TestGrid>("F4QtHost", 1, 0, "VtuiGridItem");
}

void F4OperationsQueueTests::queueUsesNativeAccessibleSurfaceAndGuardsActiveClose()
{
    const QVariantList items{task(1, QStringLiteral("Running"), 12)};
    QueueFixture fixture(queueScene(queueModel(items, 1, true)));
    QVERIFY(fixture.window);

    QQuickItem *surface = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (surface = fixture.item(QStringLiteral("operationsQueueSurface"))), 3000);
    QVERIFY(surface->isVisible());
    QVERIFY(fixture.item(QStringLiteral("operationsQueueList")));
    QVERIFY(fixture.item(QStringLiteral("operationsQueueCancelButton")));
    QVERIFY(fixture.item(QStringLiteral("operationsQueueClearButton")));
    QVERIFY(fixture.item(QStringLiteral("operationsQueueScrollBar")));
    QVERIFY(!fixture.item(QStringLiteral("workspace-screen-counter")));
    QCOMPARE(fixture.item(QStringLiteral("operationsQueueCancelButton"))
                 ->property("f4Themed").toBool(), true);
    QCOMPARE(fixture.item(QStringLiteral("operationsQueueClearButton"))
                 ->property("f4Themed").toBool(), true);
    QQuickItem *runningSummary = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (runningSummary = fixture.item(
             QStringLiteral("operationsQueueSummary-running"))), 1000);
    QCOMPARE(runningSummary->property("lucideName").toString(),
             QStringLiteral("circle-play"));
    const QVariantMap queueTabPresentation{
        {QStringLiteral("surfaceKind"), QStringLiteral("operationsQueue")},
        {QStringLiteral("iconName"), QStringLiteral("list-checks")},
    };
    QVariant queueTabIconName;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabIconName",
        Q_RETURN_ARG(QVariant, queueTabIconName),
        Q_ARG(QVariant, queueTabPresentation)));
    QCOMPARE(queueTabIconName.toString(), QStringLiteral("list-checks"));
    const QVariantMap numberedTabPresentation{
        {QStringLiteral("number"), 7},
        {QStringLiteral("text"), QStringLiteral("report.txt")},
    };
    QVariant numberedTabLabel;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabLabel",
        Q_RETURN_ARG(QVariant, numberedTabLabel),
        Q_ARG(QVariant, numberedTabPresentation)));
    QCOMPARE(numberedTabLabel.toString(), QStringLiteral("report.txt 7"));
    const QVariantMap numberOnlyTabPresentation{
        {QStringLiteral("number"), 7},
        {QStringLiteral("text"), QString()},
    };
    QVariant numberOnlyTabLabel;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabLabel",
        Q_RETURN_ARG(QVariant, numberOnlyTabLabel),
        Q_ARG(QVariant, numberOnlyTabPresentation)));
    QCOMPARE(numberOnlyTabLabel.toString(), QStringLiteral("7"));
	QVariant tabNumberColor;
	QVERIFY(QMetaObject::invokeMethod(
		fixture.window, "workspaceTabNumberColor",
		Q_RETURN_ARG(QVariant, tabNumberColor)));
	QCOMPARE(tabNumberColor.value<QColor>(),
			 fixture.window->property("mutedText").value<QColor>());

    const QVariantMap tooltipTabPresentation{
        {QStringLiteral("number"), 7},
        {QStringLiteral("text"), QStringLiteral("report.txt")},
        {QStringLiteral("tooltipPrimary"), QStringLiteral("/left/full/path")},
        {QStringLiteral("tooltipSecondary"), QStringLiteral("/right/full/path")},
        {QStringLiteral("shortcutAvailable"), true},
    };
    QVariant macShortcut;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabShortcut",
        Q_RETURN_ARG(QVariant, macShortcut),
        Q_ARG(QVariant, tooltipTabPresentation),
        Q_ARG(QVariant, QStringLiteral("osx"))));
    QCOMPARE(macShortcut.toString(), QStringLiteral("⌥7"));
    QVariant linuxShortcut;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabShortcut",
        Q_RETURN_ARG(QVariant, linuxShortcut),
        Q_ARG(QVariant, tooltipTabPresentation),
        Q_ARG(QVariant, QStringLiteral("linux"))));
    QCOMPARE(linuxShortcut.toString(), QStringLiteral("Alt+7"));
    QVariant macTooltip;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabToolTip",
        Q_RETURN_ARG(QVariant, macTooltip),
        Q_ARG(QVariant, tooltipTabPresentation),
        Q_ARG(QVariant, QStringLiteral("osx"))));
    QCOMPARE(macTooltip.toString(),
             QStringLiteral("/left/full/path\n/right/full/path\t⌥7"));

    QVariant unavailableShortcut;
    QVariantMap unavailableTab = tooltipTabPresentation;
    unavailableTab.insert(QStringLiteral("shortcutAvailable"), false);
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabShortcut",
        Q_RETURN_ARG(QVariant, unavailableShortcut),
        Q_ARG(QVariant, unavailableTab),
        Q_ARG(QVariant, QStringLiteral("linux"))));
    QVERIFY(unavailableShortcut.toString().isEmpty());

    QQuickItem *workspaceBar = fixture.item(QStringLiteral("workspaceBar"));
    QVERIFY(workspaceBar);
    QQuickItem *queueTitle = visualItemWithText(workspaceBar,
                                                QStringLiteral("Queue"));
    QQuickItem *queueNumber = visualItemWithText(workspaceBar,
                                                 QStringLiteral("2"));
    QVERIFY(queueTitle);
    QVERIFY(queueNumber);
    QCOMPARE(queueNumber->property("text").toString(), QStringLiteral("2"));
    QVERIFY(queueTitle->mapToScene(QPointF{}).x()
            < queueNumber->mapToScene(QPointF{}).x());
    QVariant naturalTabWidth;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "preferredWorkspaceTabWidth",
        Q_RETURN_ARG(QVariant, naturalTabWidth),
        Q_ARG(QVariant, 180), Q_ARG(QVariant, false)));
    QCOMPARE(naturalTabWidth.toInt(), 226);
    QVariant cappedTabWidth;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "preferredWorkspaceTabWidth",
        Q_RETURN_ARG(QVariant, cappedTabWidth),
        Q_ARG(QVariant, 400), Q_ARG(QVariant, true)));
    QCOMPARE(cappedTabWidth.toInt(), 280);

    QVariant tabWeight;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabFontWeight",
        Q_RETURN_ARG(QVariant, tabWeight)));
    QCOMPARE(tabWeight.toInt(), int(QFont::Normal));
    QVariant activeTabColor;
    QVariant inactiveTabColor;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabTextColor",
        Q_RETURN_ARG(QVariant, activeTabColor), Q_ARG(QVariant, true)));
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabTextColor",
        Q_RETURN_ARG(QVariant, inactiveTabColor), Q_ARG(QVariant, false)));
    const QColor activeColor = activeTabColor.value<QColor>();
    const QColor inactiveColor = inactiveTabColor.value<QColor>();
    QCOMPARE(activeColor, fixture.window->property("textColor").value<QColor>());
    QCOMPARE(inactiveColor,
             fixture.window->property("mutedText").value<QColor>());
    QVERIFY(activeColor.lightnessF() > inactiveColor.lightnessF());

    QQuickItem *grid = fixture.window->findChild<QQuickItem *>();
    Q_UNUSED(grid);
    QCOMPARE(fixture.window->property("fallbackExplanation").toString(), QString());

    QAccessibleInterface *surfaceInterface =
        QAccessible::queryAccessibleInterface(surface);
    QVERIFY(surfaceInterface);
    QCOMPARE(surfaceInterface->role(), QAccessible::Table);
    QCOMPARE(surfaceInterface->text(QAccessible::Name),
             QStringLiteral("Operations Queue"));

    QVariant canClose;
    const QVariant closeTab = QVariantMap{
        {QStringLiteral("id"), QStringLiteral("queue-tab")},
        {QStringLiteral("closable"), true},
    };
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabCanClose",
        Q_RETURN_ARG(QVariant, canClose),
        Q_ARG(QVariant, closeTab)));
    QVERIFY(!canClose.toBool());
}

void F4OperationsQueueTests::plusButtonIsInteractiveInsideQwkTitleBar()
{
    QueueFixture fixture(queueScene(queueModel({}, -1, false)), true);
    QVERIFY(fixture.window);
    QQuickItem *plusButton = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (plusButton = fixture.item(QStringLiteral("workspace-new"))), 3000);
    QVERIFY(plusButton->isVisible());
    QVERIFY(plusButton->property("qwkHitTestRegistered").toBool());
    QVERIFY(fixture.window->property("workspaceBarHitTestRegistered").toBool());

    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(plusButton));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1000);
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("target")),
             QVariant(QStringLiteral("workspace-new")));
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("action")),
             QVariant(QStringLiteral("workspace.new")));
}

void F4OperationsQueueTests::progressUpdatesKeepModelAndDelegateIdentity()
{
    QVariantList items{
        task(1, QStringLiteral("Running"), 5),
        task(2, QStringLiteral("Queued"), 0),
        task(3, QStringLiteral("Done"), 100, false),
    };
    QueueFixture fixture(queueScene(queueModel(items, 1, true)));
    QVERIFY(fixture.window);

    QQuickItem *surface = nullptr;
    QQuickItem *row = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (surface = fixture.item(QStringLiteral("operationsQueueSurface"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT((row = queueDelegate(surface, 1)), 3000);
    QObject *model = surface->findChild<QObject *>(
        QStringLiteral("operationsQueueRowsModel"));
    QVERIFY(model);
    QCOMPARE(model->property("count").toInt(), 3);
    QCOMPARE(row->property("progress").toInt(), 5);

    items[0] = task(1, QStringLiteral("Running"), 67);
    items.append(task(4, QStringLiteral("Queued"), 0));
    fixture.shell.setScene(queueScene(queueModel(items, 1, true)));

    QTRY_COMPARE_WITH_TIMEOUT(row->property("progress").toInt(), 67, 3000);
    QCOMPARE(fixture.item(QStringLiteral("operationsQueueSurface")), surface);
    QCOMPARE(surface->findChild<QObject *>(
                 QStringLiteral("operationsQueueRowsModel")), model);
    QCOMPARE(queueDelegate(surface, 1), row);
    QCOMPARE(model->property("count").toInt(), 4);
    QCOMPARE(surface->property("localSelectedTaskId").toInt(), 1);
}

void F4OperationsQueueTests::keyboardMouseAndClearActionsUseStableTaskIdentity()
{
    const QVariantList items{
        task(1, QStringLiteral("Running"), 20),
        task(2, QStringLiteral("Running"), 40),
        task(3, QStringLiteral("Done"), 100, false),
    };
    QueueFixture fixture(queueScene(queueModel(items, 1, true)));
    QVERIFY(fixture.window);
    QQuickItem *surface = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (surface = fixture.item(QStringLiteral("operationsQueueSurface"))), 3000);

    fixture.shell.clearActions();
    QTest::keyClick(fixture.window, Qt::Key_Down);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.select"));
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("taskId")).toInt(),
             2);
    QCOMPARE(surface->property("localSelectedTaskId").toInt(), 2);

    QTest::keyClick(fixture.window, Qt::Key_Return);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 2, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.activate"));
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("taskId")).toInt(),
             2);

    QQuickItem *cancel = fixture.item(QStringLiteral("operationsQueueCancelButton"));
    QVERIFY(cancel && cancel->isEnabled());
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(cancel));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 3, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.cancel"));
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("taskId")).toInt(),
             2);

    QQuickItem *clear = fixture.item(QStringLiteral("operationsQueueClearButton"));
    QVERIFY(clear && clear->isEnabled());
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(clear));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 4, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.clearCompleted"));
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("target")),
             QStringLiteral("operations-queue"));

    QQuickItem *row3 = queueDelegate(surface, 3);
    QVERIFY(row3);
    QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                       itemCenter(row3));
    QTRY_VERIFY_WITH_TIMEOUT(fixture.shell.actions.size() >= 6, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.activate"));
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("taskId")).toInt(),
             3);

    // A native button which owns focus must keep Return; the application-wide
    // queue activation shortcut must not also open task details.
    fixture.shell.clearActions();
    clear->forceActiveFocus(Qt::OtherFocusReason);
    QVERIFY(clear->hasActiveFocus());
    QTest::keyClick(fixture.window, Qt::Key_Return);
    QTest::qWait(50);
    QCOMPARE(fixture.shell.actions.size(), 0);
    QTest::keyClick(fixture.window, Qt::Key_Space);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.clearCompleted"));

    QAccessibleInterface *rowInterface =
        QAccessible::queryAccessibleInterface(row3);
    QVERIFY(rowInterface);
    QAccessibleActionInterface *rowActions = rowInterface->actionInterface();
    QVERIFY(rowActions);
    QVERIFY(rowActions->actionNames().contains(
        QAccessibleActionInterface::pressAction()));
    fixture.shell.clearActions();
    rowActions->doAction(QAccessibleActionInterface::pressAction());
    QTRY_VERIFY_WITH_TIMEOUT(fixture.shell.actions.size() >= 2, 1000);
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("action")),
             QStringLiteral("queue.activate"));
    QCOMPARE(fixture.shell.actions.constLast().value(QStringLiteral("taskId")).toInt(),
             3);
}

void F4OperationsQueueTests::wheelScrollsNativelyAndEmptyErrorStateIsVisible()
{
    QVariantList items;
    for (int id = 1; id <= 40; ++id)
        items.append(task(id, QStringLiteral("Queued"), 0));
    QVariantMap initialQueue = queueModel(items, 6, true);
    initialQueue.insert(QStringLiteral("top"), 5);
    QueueFixture fixture(queueScene(initialQueue));
    QVERIFY(fixture.window);
    QQuickItem *list = nullptr;
    QQuickItem *surface = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (surface = fixture.item(QStringLiteral("operationsQueueSurface"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (list = fixture.item(QStringLiteral("operationsQueueList"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(list->property("contentHeight").toReal()
                             > list->height(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(list->property("contentY").toReal()
             - 5.0 * surface->property("rowHeight").toReal()) < 0.5,
        1000);

    const qreal before = list->property("contentY").toReal();
    sendPixelWheel(fixture.window, itemCenter(list), -87);
    QTRY_VERIFY_WITH_TIMEOUT(list->property("contentY").toReal() > before, 1500);
    const qreal scrolledY = list->property("contentY").toReal();
    items[10] = task(11, QStringLiteral("Running"), 58);
    fixture.shell.setScene(queueScene(queueModel(items, 6, true)));
    QTest::qWait(40);
    QCOMPARE(list->property("contentY").toReal(), scrolledY);

    const qreal maximumY = list->property("contentHeight").toReal()
            - list->height();
    list->setProperty("contentY", maximumY);
    QQuickItem *scrollBar = fixture.item(
        QStringLiteral("operationsQueueScrollBar"));
    QVERIFY(scrollBar);
    QCOMPARE(scrollBar->property("hostWindow").value<QObject *>(), fixture.window);
    for (const QColor color : {QColor("#56b2d7"), QColor("#de7851")}) {
        QVERIFY(fixture.window->setProperty("galleryScrollBarHandleColor", color));
        QCOMPARE(scrollBar->property("handleColor").value<QColor>(), color);
        QVERIFY(fixture.window->setProperty("galleryScrollBarPressedColor", color));
        QCOMPARE(scrollBar->property("handlePressedColor").value<QColor>(), color);
        QVERIFY(!fixture.window->grabWindow().isNull());
    }
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(scrollBar->property("position").toReal()
             - (1.0 - scrollBar->property("size").toReal())) < 0.001,
        1000);

    fixture.shell.setScene(queueScene(queueModel(
        {}, 0, false, QStringLiteral("Unable to load the operation queue"))));
    QQuickItem *empty = fixture.item(QStringLiteral("operationsQueueEmptyState"));
    QVERIFY(empty);
    QTRY_VERIFY_WITH_TIMEOUT(empty->isVisible(), 1000);
    QAccessibleInterface *emptyInterface =
        QAccessible::queryAccessibleInterface(empty);
    QVERIFY(emptyInterface);
    QCOMPARE(emptyInterface->text(QAccessible::Name),
             QStringLiteral("Unable to load the operation queue"));
}

void F4OperationsQueueTests::queueTabPreservesPanelDocumentAndQueueViewState()
{
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    QQuickItem *panelPair = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (panelPair = fixture.item(QStringLiteral("persistentPanelPair"))), 3000);
    QQuickItem *prewarmedDocument = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (prewarmedDocument = fixture.item(
             QStringLiteral("documentSurface"))), 3000);
    QVERIFY(!prewarmedDocument->isVisible());
    QVERIFY(!prewarmedDocument->property("interactionActive").toBool());

    QVariantList items;
    for (int id = 1; id <= 30; ++id)
        items.append(task(id, QStringLiteral("Queued"), 0));
    const QVariantMap queue = queueModel(items, 1, true);
    fixture.shell.setScene(queueScene(queue));

    QQuickItem *queueSurface = nullptr;
    QQuickItem *queueList = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (queueSurface = fixture.item(QStringLiteral("operationsQueueSurface"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (queueList = fixture.item(QStringLiteral("operationsQueueList"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(queueList->property("contentHeight").toReal()
                             > queueList->height(), 3000);
    queueList->setProperty("contentY", 180.0);
    QVERIFY(QMetaObject::invokeMethod(queueList, "flick",
                                      Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, -900.0)));
    QTRY_VERIFY_WITH_TIMEOUT(queueList->property("flicking").toBool(), 1000);

    fixture.shell.setScene(panelScene());
    QTRY_VERIFY_WITH_TIMEOUT(panelPair->isVisible(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        !queueSurface->property("interactionActive").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(!queueList->property("flicking").toBool(), 1000);
    const qreal frozenQueueY = queueList->property("contentY").toReal();
    QTest::qWait(60);
    QCOMPARE(queueList->property("contentY").toReal(), frozenQueueY);
    QCOMPARE(fixture.item(QStringLiteral("persistentPanelPair")), panelPair);
    QCOMPARE(fixture.item(QStringLiteral("operationsQueueSurface")), queueSurface);
    QVERIFY(!queueSurface->isVisible());
    // The retained queue snapshot still says hasActive=true, but it is stale
    // while Commander is current.  Fresh workspaceTabs.closable must win so a
    // task which completed in the background does not leave a phantom guard.
    QVariant inactiveCanClose;
    const QVariant inactiveQueueTab = QVariantMap{
        {QStringLiteral("id"), QStringLiteral("queue-tab")},
        {QStringLiteral("closable"), true},
    };
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "workspaceTabCanClose",
        Q_RETURN_ARG(QVariant, inactiveCanClose),
        Q_ARG(QVariant, inactiveQueueTab)));
    QVERIFY(inactiveCanClose.toBool());

    fixture.shell.setScene(queueScene(queue));
    QTRY_VERIFY_WITH_TIMEOUT(queueSurface->isVisible(), 1000);
    QCOMPARE(fixture.item(QStringLiteral("operationsQueueList")), queueList);
    QCOMPARE(queueList->property("contentY").toReal(), frozenQueueY);

    fixture.shell.setScene(documentScene());
    QQuickItem *document = prewarmedDocument;
    QQuickItem *documentList = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(document->isVisible(), 3000);
    QCOMPARE(fixture.item(QStringLiteral("documentSurface")), document);
    QTRY_VERIFY_WITH_TIMEOUT(
        (documentList = fixture.item(QStringLiteral("documentList"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(document->property("interactionActive").toBool(),
                             1000);
    QTRY_VERIFY_WITH_TIMEOUT(documentList->property("contentHeight").toReal()
                             > documentList->height(), 3000);
    documentList->setProperty("contentY", 0.0);
    document->setProperty("windowRequestPending", true);
    QVERIFY(QMetaObject::invokeMethod(documentList, "flick",
                                      Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, -700.0)));
    QTRY_VERIFY_WITH_TIMEOUT(documentList->property("flicking").toBool(), 1000);

    fixture.shell.setScene(queueScene(queue));
    QTRY_VERIFY_WITH_TIMEOUT(queueSurface->isVisible(), 1000);
    QCOMPARE(fixture.item(QStringLiteral("documentSurface")), document);
    QVERIFY(!document->isVisible());
    QVERIFY(!document->property("interactionActive").toBool());
    QTRY_VERIFY_WITH_TIMEOUT(!documentList->property("flicking").toBool(),
                             1000);
    QVERIFY(!document->property("windowRequestPending").toBool());
    const qreal frozenDocumentY = documentList->property("contentY").toReal();
    QTest::qWait(60);
    QCOMPARE(documentList->property("contentY").toReal(), frozenDocumentY);

    fixture.shell.setScene(documentScene());
    QTRY_VERIFY_WITH_TIMEOUT(document->isVisible(), 1000);
    QCOMPARE(fixture.item(QStringLiteral("documentList")), documentList);
    QCOMPARE(documentList->property("contentY").toReal(), frozenDocumentY);
    QCOMPARE(fixture.item(QStringLiteral("persistentPanelPair")), panelPair);
}

void F4OperationsQueueTests::terminalModeKeepsPersistentPanelsSurfaceVisible()
{
    QueueFixture fixture(terminalScene());
    QVERIFY(fixture.window);
    QQuickItem *panelsLayer = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (panelsLayer = fixture.item(QStringLiteral("persistentPanelsLayer"))),
        3000);
    QVERIFY(panelsLayer->isVisible());
    QQuickItem *documentLayer = fixture.item(
        QStringLiteral("persistentDocumentLayer"));
    QVERIFY(documentLayer);
    QVERIFY(!documentLayer->isVisible());
    QQuickItem *panelPair = fixture.item(QStringLiteral("persistentPanelPair"));
    QVERIFY(panelPair);
    QVERIFY(!panelPair->isVisible());

    QQuickItem *terminal = fixture.item(QStringLiteral("terminalBackdrop"));
    QQuickItem *commandLine = fixture.item(QStringLiteral("commandLineView"));
    QQuickItem *keyBar = fixture.item(QStringLiteral("keyBar"));
    QVERIFY(terminal);
    QVERIFY(commandLine);
    QVERIFY(keyBar);
    QVERIFY(terminal->isVisible());
    QVERIFY(commandLine->isVisible());
    QVERIFY(keyBar->isVisible());
    QVERIFY(commandLine->height() > 0.0);
    QVERIFY(keyBar->height() > 0.0);
    QVERIFY(terminal->y() + terminal->height() <= commandLine->y() + 0.5);
    QVERIFY(commandLine->y() + commandLine->height() <= keyBar->y() + 0.5);
}

void F4OperationsQueueTests::panelLoadingPulseIsDelayedLocalAndDoesNotMoveRendererButton()
{
    QueueFixture fixture(loadingPanelScene(false));
    QVERIFY(fixture.window);

    QQuickItem *path = nullptr;
    QQuickItem *pulse = nullptr;
    QQuickItem *sortButton = nullptr;
    QQuickItem *sortButtonContent = nullptr;
    QQuickItem *sortLabel = nullptr;
    QQuickItem *sortDirection = nullptr;
    QQuickItem *sortChevron = nullptr;
    QObject *sortMenu = nullptr;
    QQuickItem *renderer = nullptr;
    QQuickItem *rendererContent = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (path = fixture.item(QStringLiteral("panelPathTitle-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (pulse = fixture.item(QStringLiteral("panelLoadingIndicator-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortButton = fixture.item(QStringLiteral("panelSortButton-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortButtonContent = fixture.item(
             QStringLiteral("panelSortButtonContent-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortLabel = fixture.item(QStringLiteral("panelSortLabel-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortDirection = fixture.item(
             QStringLiteral("panelSortDirectionIcon-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortChevron = fixture.item(QStringLiteral("panelSortChevron-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortMenu = fixture.object(QStringLiteral("panelSortMenu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (renderer = fixture.item(QStringLiteral("panelRendererButton-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (rendererContent = fixture.item(
             QStringLiteral("panelRendererButtonContent-0"))), 3000);

    QCOMPARE(path->property("text").toString(),
             QStringLiteral("/Users/zoin/Documents"));
    QVERIFY(path->property("backgroundOnHoverOnly").toBool());
    QCOMPARE(path->property("leadingInset").toReal(), 0.0);
    QCOMPARE(path->property("breadcrumbFontPixelSize").toReal(), 13.0);
    QCOMPARE(path->property("pathTextColor").value<QColor>(),
             QColor(QStringLiteral("#e8edf2")));
    QCOMPARE(path->property("pathHoveredColor").value<QColor>(),
             QColor(QStringLiteral("#222c38")));
    QCOMPARE(path->property("pathItemHoveredColor").value<QColor>(),
             QColor(QStringLiteral("#2a3745")));
    QCOMPARE(path->property("pathItemPressedColor").value<QColor>(),
             QColor(QStringLiteral("#10161e")));
    const QUrl localDriveSource = path->property(
        "localDriveIconSource").toUrl();
    const QUrl networkDriveSource = path->property(
        "networkDriveIconSource").toUrl();
    QCOMPARE(localDriveSource.path(),
             QStringLiteral("/F4QtHost/icons/lucide/hard-drive.svg"));
    QCOMPARE(networkDriveSource.path(),
             QStringLiteral("/F4QtHost/icons/lucide/network.svg"));
    QCOMPARE(QUrlQuery(localDriveSource).queryItemValue(
                 QStringLiteral("size")), QStringLiteral("18"));
    QCOMPARE(QUrlQuery(networkDriveSource).queryItemValue(
                 QStringLiteral("size")), QStringLiteral("18"));
    QCOMPARE(QColor(QUrlQuery(localDriveSource).queryItemValue(
                 QStringLiteral("color"))),
             QColor(QStringLiteral("#e8edf2")));
    QCOMPARE(QColor(QUrlQuery(networkDriveSource).queryItemValue(
                 QStringLiteral("color"))),
             QColor(QStringLiteral("#e8edf2")));
    QVERIFY(!pulse->isVisible());
    QCOMPARE(sortLabel->property("text").toString(), QStringLiteral("Name"));
    QCOMPARE(sortDirection->property("lucideName").toString(),
             QStringLiteral("arrow-up"));
    QVERIFY(sortDirection->x() < sortLabel->x());
    QVERIFY(sortLabel->x() < sortChevron->x());
    const qreal sortHorizontalPadding = sortButton->width()
            - sortButtonContent->width();
    const qreal rendererHorizontalPadding = renderer->width()
            - rendererContent->width();
    QCOMPARE(sortHorizontalPadding, 16.0);
    QCOMPARE(rendererHorizontalPadding, sortHorizontalPadding);
    QVERIFY(sortButton->x() < renderer->x());

    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(sortButton));
    QTRY_VERIFY_WITH_TIMEOUT(sortMenu->property("opened").toBool(), 1000);
    QVERIFY(fixture.shell.actions.isEmpty());
    QQuickItem *sortNameLabel = nullptr;
    QQuickItem *sortNameCheck = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortNameLabel = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("panelSortChoiceLabel-name-0"))), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortNameCheck = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("panelSortChoiceCheck-name-0"))), 1000);
    QVERIFY(sortNameCheck->isVisible());
    QVERIFY(sortNameCheck->x() < sortNameLabel->x());
    QCOMPARE(QColor(QUrlQuery(sortNameCheck->property("source").toUrl())
                 .queryItemValue(QStringLiteral("color"))),
             QColor(QStringLiteral("#4e9bd4")));
    const QPoint firstSortChoice(
        qRound(sortMenu->property("x").toReal()
               + sortMenu->property("width").toReal() / 2.0),
        qRound(sortMenu->property("y").toReal() + 6.0 + 31.0 / 2.0));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      firstSortChoice);
    QTRY_VERIFY_WITH_TIMEOUT(!sortMenu->property("opened").toBool(), 1000);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1000);
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("action"))
                 .toString(),
             QStringLiteral("panel.sort"));
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("side"))
                 .toInt(),
             0);
    QCOMPARE(fixture.shell.actions.constFirst().value(QStringLiteral("mode"))
                 .toString(),
             QStringLiteral("name"));
    const qreal rendererX = renderer->x();

    // A normal local read which completes inside the grace period must never
    // produce a visible loading frame.
    fixture.shell.setScene(loadingPanelScene(true));
    QTest::qWait(45);
    QVERIFY(!pulse->isVisible());
    fixture.shell.setScene(loadingPanelScene(false));
    QTest::qWait(150);
    QVERIFY(!pulse->isVisible());
    QCOMPARE(renderer->x(), rendererX);

    // A genuinely slow load reveals the compact Braille pulse beside the
    // clean path, advances locally in QML, and hides synchronously on ACK.
    fixture.shell.setScene(loadingPanelScene(true));
    QTRY_VERIFY_WITH_TIMEOUT(pulse->isVisible(), 500);
    const QString firstFrame = pulse->property("text").toString();
    QVERIFY(!firstFrame.isEmpty());
    QCOMPARE(path->property("text").toString(),
             QStringLiteral("/Users/zoin/Documents"));
    QCOMPARE(renderer->x(), rendererX);
    QTRY_VERIFY_WITH_TIMEOUT(
        pulse->property("text").toString() != firstFrame, 500);

    fixture.shell.setScene(loadingPanelScene(false));
    QTRY_VERIFY_WITH_TIMEOUT(!pulse->isVisible(), 200);
    QCOMPARE(renderer->x(), rendererX);

    fixture.shell.setScene(
        loadingPanelScene(false, QStringLiteral("size"), false));
    QTRY_COMPARE_WITH_TIMEOUT(sortLabel->property("text").toString(),
                              QStringLiteral("Size"), 1000);
    QTRY_COMPARE_WITH_TIMEOUT(sortDirection->property("lucideName").toString(),
                              QStringLiteral("arrow-down"), 1000);

    fixture.shell.setScene(
        loadingPanelScene(false, QStringLiteral("size"), true));
    QTRY_COMPARE_WITH_TIMEOUT(sortDirection->property("lucideName").toString(),
                              QStringLiteral("arrow-up"), 1000);
}

void F4OperationsQueueTests::rendererPopupClosesOnOutsidePress()
{
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    QQuickItem *button = nullptr;
    QObject *popup = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (button = fixture.item(QStringLiteral("panelRendererButton-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (popup = fixture.object(QStringLiteral("panelRendererMenu-0"))),
        3000);

    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(button));
    QTRY_VERIFY_WITH_TIMEOUT(popup->property("opened").toBool(), 1000);

    // Pick an unambiguous point in the panel body, outside both the popup and
    // its renderer button. The same press must only dismiss the popup.
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      QPoint(10, fixture.window->height() - 10));
    QTRY_VERIFY_WITH_TIMEOUT(!popup->property("opened").toBool(), 1000);
}

void F4OperationsQueueTests::semanticDialogsMoveResizeAndUseZoinWindowButtons()
{
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    // Overlay repeaters are driven by scene changes after QML construction in
    // production, so enter the dialog scene through the same transition.
    fixture.shell.setScene(dialogScene());
    QCOMPARE(fixture.shell.overlayState()->dialogs().size(), 1);

    // Drive the real OverlayHost model as production does. Updating the
    // frame for the same overlay key must retain the Loader/GenericDialog
    // instance, otherwise a just-finished drag loses its local geometry.
    QQuickItem *overlayHost = fixture.item(QStringLiteral("semanticOverlayHost"));
    QVERIFY(overlayHost);
    QObject *frameModel = overlayHost->findChild<QObject *>(
        QStringLiteral("semanticOverlayFrameModel"));
    QObject *frameRepeater = overlayHost->findChild<QObject *>(
        QStringLiteral("semanticOverlayRepeater"));
    QVERIFY(frameModel);
    QVERIFY(frameRepeater);
    QTRY_COMPARE(frameModel->property("count").toInt(), 1);
    QQuickItem *overlayLoader = nullptr;
    QVERIFY(QMetaObject::invokeMethod(
        frameRepeater, "itemAt", Q_RETURN_ARG(QQuickItem *, overlayLoader),
        Q_ARG(int, 0)));
    QVERIFY(overlayLoader);
    QQuickItem *liveHeader = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (liveHeader = overlayLoader->findChild<QQuickItem *>(
             QStringLiteral("dialogMoveHandle"))),
        3000);
    QQuickItem *liveDialog = liveHeader->parentItem();
    QVERIFY(liveDialog);
    QVERIFY(QMetaObject::invokeMethod(
        liveDialog, "setUserGeometry",
        Q_ARG(QVariant, QVariant(180.0)),
        Q_ARG(QVariant, QVariant(120.0)),
        Q_ARG(QVariant, QVariant(560.0)),
        Q_ARG(QVariant, QVariant(420.0))));
    const QQuickItem *liveDialogBefore = liveDialog;
    QVariantMap updatedScene = dialogScene();
    QVariantList updatedDialogs = updatedScene.value(
        QStringLiteral("dialogs")).toList();
    QVariantMap updatedDialog = updatedDialogs.constFirst().toMap();
    updatedDialog.insert(QStringLiteral("x"), 22);
    updatedDialog.insert(QStringLiteral("y"), 5);
    updatedDialogs[0] = updatedDialog;
    updatedScene.insert(QStringLiteral("dialogs"), updatedDialogs);
    fixture.shell.setScene(updatedScene);
    QTRY_VERIFY_WITH_TIMEOUT(
        (liveHeader = overlayLoader->findChild<QQuickItem *>(
             QStringLiteral("dialogMoveHandle"))),
        1000);
    QQuickItem *liveDialogAfter = liveHeader->parentItem();
    QVERIFY(liveDialogBefore == liveDialogAfter);
    QCOMPARE(liveDialogAfter->x(), 180.0);
    QCOMPARE(liveDialogAfter->y(), 120.0);
    QCOMPARE(liveDialogAfter->width(), 560.0);
    QCOMPARE(liveDialogAfter->height(), 420.0);

    QVariant overlayFrames;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "overlayFrames", Q_RETURN_ARG(QVariant, overlayFrames)));
    QCOMPARE(overlayFrames.toList().size(), 1);
    QVariant createdOverlay;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "createDialogOverlay",
        Q_RETURN_ARG(QVariant, createdOverlay),
        Q_ARG(QVariant, overlayFrames.toList().constFirst())));
    QObject *createdOverlayObject = createdOverlay.value<QObject *>();
    QVERIFY(createdOverlayObject);
    auto *createdOverlayItem = qobject_cast<QQuickItem *>(createdOverlayObject);
    QVERIFY(createdOverlayItem);
    createdOverlayItem->setZ(1000);

    QQuickItem *dialog = nullptr;
    QQuickItem *header = nullptr;
    QQuickItem *resizeCorner = nullptr;
    QQuickItem *maximizeButton = nullptr;
    QQuickItem *closeButton = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (header = createdOverlayObject->findChild<QQuickItem *>(
             QStringLiteral("dialogMoveHandle"))), 3000);
    dialog = header->parentItem();
    QVERIFY(dialog);
    QCOMPARE(dialog->objectName(),
             QStringLiteral("semanticDialog-appearance-dialog"));
    QTRY_VERIFY_WITH_TIMEOUT(
        (resizeCorner = createdOverlayObject->findChild<QQuickItem *>(QStringLiteral(
             "dialogResizeBottomRight"))), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (maximizeButton = createdOverlayObject->findChild<QQuickItem *>(QStringLiteral(
             "dialogMaximizeButton"))), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (closeButton = createdOverlayObject->findChild<QQuickItem *>(
             QStringLiteral("dialogCloseButton"))),
        1000);

    QTRY_VERIFY_WITH_TIMEOUT(
        visualItemWithText(createdOverlayItem, QStringLiteral("Appearance content")),
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        visualItemWithText(createdOverlayItem,
                           QStringLiteral("Show hidden files")),
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        visualItemWithText(createdOverlayItem, QStringLiteral("First")),
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        visualItemWithText(createdOverlayItem, QStringLiteral("Second")),
        1000);
    QQuickItem *dialogBody = createdOverlayObject->findChild<QQuickItem *>(
        QStringLiteral("dialogBody"));
    QQuickItem *dialogBodyScrollBar = createdOverlayObject->findChild<QQuickItem *>(
        QStringLiteral("dialogBodyScrollBar"));
    QVERIFY(dialogBody);
    QVERIFY(dialogBodyScrollBar);
    QTRY_VERIFY_WITH_TIMEOUT(!dialogBodyScrollBar->isVisible(), 1000);

    QVERIFY(header->isVisible());
    QVERIFY(resizeCorner->isVisible());
    QCOMPARE(maximizeButton->property("source").toUrl().toString(),
             QStringLiteral(
                 "qrc:/ZoinGallery/resources/WindowMaximize.svg"));
    QCOMPARE(closeButton->property("source").toUrl().toString(),
             QStringLiteral("qrc:/ZoinGallery/resources/WindowClose.svg"));

    // Geometry is kept locally for a fluid drag, then committed to the core
    // in character cells so the authoritative vtui layout follows it.
    QVERIFY(QMetaObject::invokeMethod(
        dialog, "setUserGeometry",
        Q_ARG(QVariant, QVariant(180.0)),
        Q_ARG(QVariant, QVariant(120.0)),
        Q_ARG(QVariant, QVariant(560.0)),
        Q_ARG(QVariant, QVariant(420.0))));
    QCOMPARE(dialog->x(), 180.0);
    QCOMPARE(dialog->y(), 120.0);
    QCOMPARE(dialog->width(), 560.0);
    QCOMPARE(dialog->height(), 420.0);

    // Updating the semantic payload for the same dialog must not replace the
    // local geometry while the authoritative action is being acknowledged.
    const QRectF start(dialog->x(), dialog->y(), dialog->width(),
                       dialog->height());
    QVERIFY(QMetaObject::invokeMethod(
        dialog, "resizeFrom",
        Q_ARG(QVariant, QVariant(10)),
        Q_ARG(QVariant, QVariant(48.0)),
        Q_ARG(QVariant, QVariant(36.0)),
        Q_ARG(QVariant, QVariant::fromValue(start))));
    QCOMPARE(dialog->width(), 608.0);
    QCOMPARE(dialog->height(), 456.0);

    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(dialog, "commitGeometry"));
    QCOMPARE(fixture.shell.actions.size(), 1);
    const QVariantMap geometryAction = fixture.shell.actions.constLast();
    QCOMPARE(geometryAction.value(QStringLiteral("target")).toString(),
             QStringLiteral("appearance-dialog"));
    QCOMPARE(geometryAction.value(QStringLiteral("action")).toString(),
             QStringLiteral("dialog.geometry"));
    QCOMPARE(geometryAction.value(QStringLiteral("x")).toInt(), 23);
    QCOMPARE(geometryAction.value(QStringLiteral("y")).toInt(), 6);
    QCOMPARE(geometryAction.value(QStringLiteral("w")).toInt(), 76);
    QCOMPARE(geometryAction.value(QStringLiteral("h")).toInt(), 23);

    QVERIFY(QMetaObject::invokeMethod(dialog, "toggleMaximized"));
    QTRY_VERIFY_WITH_TIMEOUT(dialog->property("maximized").toBool(), 1000);
    QCOMPARE(dialog->x(), 12.0);
    QCOMPARE(dialog->width(), qreal(fixture.window->width() - 24));
    QCOMPARE(maximizeButton->property("source").toUrl().toString(),
             QStringLiteral("qrc:/ZoinGallery/resources/WindowRestore.svg"));
    QVERIFY(!resizeCorner->isVisible());

    QVERIFY(QMetaObject::invokeMethod(dialog, "toggleMaximized"));
    QTRY_VERIFY_WITH_TIMEOUT(!dialog->property("maximized").toBool(), 1000);
    QCOMPARE(dialog->x(), start.x());
    QCOMPARE(dialog->y(), start.y());
    QCOMPARE(dialog->width(), 608.0);
    QCOMPARE(dialog->height(), 456.0);
    QVERIFY(resizeCorner->isVisible());

    // The header keeps resolving semantic theme properties after creation.
    const QColor firstHeader(31, 73, 109);
    const QColor secondHeader(102, 43, 87);
    fixture.window->setProperty("dialogHeaderBg", firstHeader);
    QTRY_COMPARE(header->property("color").value<QColor>(), firstHeader);
    QVERIFY(!fixture.window->grabWindow().isNull());
    fixture.window->setProperty("dialogHeaderBg", secondHeader);
    QTRY_COMPARE(header->property("color").value<QColor>(), secondHeader);
    QVERIFY(!fixture.window->grabWindow().isNull());
}

QTEST_MAIN(F4OperationsQueueTests)

#include "F4OperationsQueueTests.moc"
