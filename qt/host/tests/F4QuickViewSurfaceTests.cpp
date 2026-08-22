#include "DummyQWK.h"

#include <QCoreApplication>
#include <QColor>
#include <QFont>
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

private:
    QObject *m_controller = nullptr;
    QString m_fontFamily;
    int m_fontPixelSize = 13;
    bool m_pointerInputEnabled = false;
    bool m_inputMethodForwardingEnabled = false;
    bool m_terminalInputEnabled = true;
    bool m_renderingEnabled = true;
};

class TestShell final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)
    Q_PROPERTY(QVariantMap scene READ scene NOTIFY sceneChanged)
    Q_PROPERTY(QVariantMap presentationScene READ scene NOTIFY sceneChanged)
    Q_PROPERTY(QVariantMap commandLine READ commandLine NOTIFY commandLineChanged)

public:
    int initialCols() const { return 110; }
    int initialRows() const { return 34; }
    QVariantMap scene() const { return m_scene; }
    QVariantMap commandLine() const { return m_commandLine; }

    void setScene(const QVariantMap &scene)
    {
        m_scene = scene;
        m_commandLine = scene.value(QStringLiteral("shell")).toMap()
                            .value(QStringLiteral("commandLine")).toMap();
        emit commandLineChanged();
        emit sceneChanged();
    }
    void setCommandLine(const QVariantMap &commandLine)
    {
        m_commandLine = commandLine;
        emit commandLineChanged();
    }
    void clearActions() { actions.clear(); }
    void clearKeyEvents() { keyEvents.clear(); }
    void activatePanel(int side, qulonglong revision)
    {
        emit panelActivationChanged(side, revision);
    }
    void deliverMessage(const QVariantMap &message)
    {
        emit messageReceived(message);
    }
    void deliverCompactPresentation(const QVariantMap &patch)
    {
        emit compactPresentationChanged(patch);
    }

    Q_INVOKABLE void sendUiAction(const QVariantMap &action)
    {
        actions.append(action);
        emit uiActionSent(action);
    }
    Q_INVOKABLE void sendQuit() {}
    Q_INVOKABLE void sendKey(int vk, int ch, bool down, int mods)
    {
        keyEvents.append({
            {QStringLiteral("vk"), vk},
            {QStringLiteral("char"), ch},
            {QStringLiteral("down"), down},
            {QStringLiteral("mods"), mods},
        });
    }

    QVector<QVariantMap> actions;
    QVector<QVariantMap> keyEvents;

signals:
    void sceneChanged();
    void commandLineChanged();
    void panelActivationChanged(int activePanel, qulonglong revision);
    void compactPresentationChanged(const QVariantMap &patch);
    void messageReceived(const QVariantMap &message);
    void uiActionSent(const QVariantMap &action);

private:
    QVariantMap m_scene;
    QVariantMap m_commandLine;
};

class TestGallery final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool available READ available CONSTANT)
    Q_PROPERTY(QObject *viewerSession READ nullObject CONSTANT)
    Q_PROPERTY(bool viewerVisible READ viewerVisible CONSTANT)
    Q_PROPERTY(int viewerSide READ viewerSide CONSTANT)
    Q_PROPERTY(QUrl panelComponentUrl READ panelComponentUrl CONSTANT)
    Q_PROPERTY(QUrl viewerComponentUrl READ emptyUrl CONSTANT)

public:
    explicit TestGallery(bool available = false) : m_available(available) {}

    bool available() const { return m_available; }
    QObject *nullObject() const { return nullptr; }
    bool viewerVisible() const { return false; }
    int viewerSide() const { return 0; }
    QUrl emptyUrl() const { return {}; }
    QUrl panelComponentUrl() const
    {
        return m_available
            ? QUrl(QStringLiteral("qrc:/F4QtHost/tests/TestGalleryPanel.qml"))
            : QUrl{};
    }

    Q_INVOKABLE QObject *sessionForSide(int) const { return nullptr; }
    Q_INVOKABLE void closeViewer() {}

signals:
    void viewerChanged();

private:
    bool m_available = false;
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

QVariantList fileEntries(int count)
{
    QVariantList result;
    for (int index = 0; index < count; ++index) {
        result.append(QVariantMap{
            {QStringLiteral("index"), index},
            {QStringLiteral("entryId"), QStringLiteral("entry-%1").arg(index)},
            {QStringLiteral("name"), QStringLiteral("file-%1.txt").arg(index)},
            {QStringLiteral("displayBaseName"),
             QStringLiteral("file-%1").arg(index)},
            {QStringLiteral("displayExtension"), QStringLiteral("txt")},
            {QStringLiteral("sizeText"), QStringLiteral("1 KB")},
            {QStringLiteral("mode"), QStringLiteral("-rw-r--r--")},
        });
    }
    return result;
}

QVariantMap panel(int side, bool active)
{
    return {
        {QStringLiteral("id"), QStringLiteral("panel-%1").arg(side)},
        {QStringLiteral("side"), side},
        {QStringLiteral("active"), active},
        {QStringLiteral("path"), QStringLiteral("/tmp/side-%1").arg(side)},
        {QStringLiteral("title"), QStringLiteral("side-%1").arg(side)},
        {QStringLiteral("viewModeName"), QStringLiteral("detailed")},
        {QStringLiteral("presentation"), QStringLiteral("list")},
        {QStringLiteral("sourceKind"), QStringLiteral("local")},
        {QStringLiteral("previewCapable"), true},
        {QStringLiteral("cursor"), 0},
        {QStringLiteral("top"), 0},
        {QStringLiteral("catalogRevision"), 1},
        {QStringLiteral("selectionRevision"), 1},
        {QStringLiteral("highlightRevision"), 1},
        {QStringLiteral("entries"), fileEntries(120)},
        {QStringLiteral("columns"), QVariantList{}},
    };
}

QVariantList visualRows(int firstRow, int count)
{
    QVariantList rows;
    for (int index = 0; index < count; ++index) {
        const int row = firstRow + index;
        rows.append(QVariantMap{
            {QStringLiteral("visualRow"), row},
            {QStringLiteral("text"), QStringLiteral("preview row %1").arg(row)},
        });
    }
    return rows;
}

QVariantMap quickView(int side, bool active, const QString &contentKey,
                      int firstRow, int viewportStart, int generation,
                      int contentExtent = 400)
{
    const QVariantList rows = visualRows(firstRow, 90);
    const QVariantMap surface{
        {QStringLiteral("id"), QStringLiteral("quick-view-%1").arg(side)},
        {QStringLiteral("kind"), QStringLiteral("quick_view")},
        {QStringLiteral("documentKey"), contentKey},
        {QStringLiteral("scrollAction"), QStringLiteral("quickView.scroll")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("rows"), rows.mid(qMax(0, viewportStart - firstRow), 24)},
        {QStringLiteral("windowRows"), rows},
        {QStringLiteral("windowStart"), firstRow},
        {QStringLiteral("windowEnd"), firstRow + rows.size()},
        {QStringLiteral("viewportStart"), viewportStart},
        {QStringLiteral("viewportSpan"), 24},
        {QStringLiteral("viewportRow"), qMax(0, viewportStart - firstRow)},
        {QStringLiteral("contentExtent"), contentExtent},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("windowGeneration"), generation},
    };
    return {
        {QStringLiteral("id"), QStringLiteral("quick-view-%1").arg(side)},
        {QStringLiteral("kind"), QStringLiteral("quickViewPanel")},
        {QStringLiteral("side"), side},
        {QStringLiteral("sourceSide"), 1 - side},
        {QStringLiteral("active"), active},
        {QStringLiteral("title"), QStringLiteral("Quick View")},
        {QStringLiteral("bottomHint"), QStringLiteral("F2 Wrap")},
        {QStringLiteral("contentKey"), contentKey},
        {QStringLiteral("name"), QStringLiteral("selected.txt")},
        {QStringLiteral("sizeText"), QStringLiteral("Size: 16 KB")},
        {QStringLiteral("previewKind"), QStringLiteral("text")},
        {QStringLiteral("headerRows"), QVariantList{
             QVariantMap{{QStringLiteral("text"), QStringLiteral("selected.txt")}},
             QVariantMap{{QStringLiteral("text"), QStringLiteral("Size: 16 KB")}},
         }},
        {QStringLiteral("surface"), surface},
    };
}

QVariantMap shellScene(const QVariantList &quickViews = {}, int activeSide = 1)
{
    return {
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("presentation"), QStringLiteral("qml")},
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("newTab"), QVariantMap{}},
             {QStringLiteral("counter"), QVariantMap{}},
         }},
        {QStringLiteral("keyBar"), QVariantMap{
             {QStringLiteral("items"), QVariantList{}},
         }},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("shell")},
             {QStringLiteral("terminalActive"), false},
             {QStringLiteral("showPanels"), true},
             {QStringLiteral("showLeftPanel"), true},
             {QStringLiteral("showRightPanel"), true},
             {QStringLiteral("activePanel"), activeSide},
             {QStringLiteral("panels"), QVariantList{
                  panel(0, activeSide == 0), panel(1, activeSide == 1),
              }},
             {QStringLiteral("quickViews"), quickViews},
             {QStringLiteral("commandLine"), QVariantMap{
                  {QStringLiteral("visible"), false},
              }},
         }},
    };
}

void sendPixelWheel(QQuickWindow *window, const QPoint &position, int deltaY)
{
    QWheelEvent event(position, window->mapToGlobal(position),
                      QPoint(0, deltaY), {}, Qt::NoButton, Qt::NoModifier,
                      Qt::NoScrollPhase, false);
    QCoreApplication::sendEvent(window, &event);
}

qreal topVisualRow(QQuickItem *surface, QQuickItem *list)
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
    return rows.at(index).toMap().value(QStringLiteral("visualRow")).toReal()
        + raw - std::floor(raw);
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

struct QuickViewFixture
{
    TestShell shell;
    TestGallery gallery;
    TestIcons icons;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;

    explicit QuickViewFixture(const QVariantMap &scene,
                              bool galleryAvailable = false)
        : gallery(galleryAvailable)
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
                                                  false);
        DummyQWK::registerTypes(&engine);
        engine.load(QUrl(QStringLiteral("qrc:/F4QtHost/qml/main.qml")));
        if (engine.rootObjects().isEmpty())
            return;
        window = qobject_cast<QQuickWindow *>(engine.rootObjects().constFirst());
        if (!window)
            return;
        window->resize(900, 640);
        window->show();
        window->requestActivate();
        QCoreApplication::processEvents();
    }

    template<typename T = QQuickItem>
    T *item(const QString &objectName) const
    {
        return window ? window->findChild<T *>(objectName) : nullptr;
    }
};
}

class F4QuickViewSurfaceTests final : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();
    void semanticSceneGatesOnlyGridRendering();
    void functionBarShowsExplicitFunctionKeysAndForwardsMouseModifiers();
    void readyUnifiedRendererLoaderIsVisible();
    void rendererChoicesUseProductOrderAndShortcuts();
    void coverUncoverPreservesFilePanelAndRendererObjects();
    void compactActivationPreservesPanelObjectsAndRebindsOnlyFocus();
    void compactCatalogUpdatesOnlyChangedPanelPresentation();
    void compactChromeUpdatesWorkspaceTabsWithoutRebuildingPanels();
    void embeddedWheelCoalescesAndUsesQuickViewContract();
    void contentKeyChangeDropsOldGestureAndAnchor();
    void clickActivatesCoveredSideAndFocusStaysOutOfHiddenPanel();
    void previewKindsSelectExactlyOneNativeBody();
    void commandLineUsesOriginalSemanticRendererAndCursor();
    void commandLineCursorTracksFirstTextPatch();
    void autocompleteReturnTargetsShellCommandHandler();
    void widePanelDoesNotRevealTerminalBackdrop();
};

void F4QuickViewSurfaceTests::initTestCase()
{
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    qmlRegisterType<TestGrid>("F4QtHost", 1, 0, "VtuiGridItem");
}

void F4QuickViewSurfaceTests::semanticSceneGatesOnlyGridRendering()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);
    auto *grid = fixture.item<TestGrid>(QStringLiteral("vtuiGrid"));
    QVERIFY(grid);

    // The compatibility grid remains present and callable as the global
    // keyboard/IME sink underneath native semantic surfaces; only its costly
    // texture rendering is disabled.
    QVERIFY(grid->isVisible());
    QTRY_VERIFY_WITH_TIMEOUT(!grid->renderingEnabled(), 3000);

    QVariantMap fallbackScene = shellScene();
    fallbackScene.insert(QStringLiteral("presentation"),
                         QStringLiteral("text"));
    fixture.shell.setScene(fallbackScene);
    QTRY_VERIFY_WITH_TIMEOUT(grid->renderingEnabled(), 3000);
    QVERIFY(grid->isVisible());
}

void F4QuickViewSurfaceTests::functionBarShowsExplicitFunctionKeysAndForwardsMouseModifiers()
{
    QVariantList items;
    for (int index = 0; index < 12; ++index) {
        items.append(QVariantMap{
            {QStringLiteral("index"), index},
            {QStringLiteral("key"), QStringLiteral("F%1").arg(index + 1)},
            {QStringLiteral("text"), QStringLiteral("Action %1").arg(index + 1)},
        });
    }
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("keyBar"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("modifier"), QStringLiteral("normal")},
        {QStringLiteral("items"), items},
    });

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *keyBar = fixture.item(QStringLiteral("keyBar"));
    QVERIFY(keyBar);
    QQuickItem *f1 = visualItemWithText(keyBar, QStringLiteral("F1"));
    QQuickItem *f12 = visualItemWithText(keyBar, QStringLiteral("F12"));
    QQuickItem *f12Label = visualItemWithText(keyBar,
                                              QStringLiteral("Action 12"));
    QVERIFY(f1);
    QVERIFY(f12);
    QVERIFY(f12Label);
    QCOMPARE(f1->property("text").toString(), QStringLiteral("F1"));
    QCOMPARE(f12->property("text").toString(), QStringLiteral("F12"));
    QVERIFY(f12Label->mapToScene(QPointF{}).x()
            >= f12->mapToScene(QPointF(f12->width(), 0)).x() + 6.0);
    QQuickItem *f12Button = f12->parentItem();
    QVERIFY(f12Button);
    QCOMPARE(f12Button->property("functionKey").toString(),
             QStringLiteral("F12"));
    QCOMPARE(f12Button->property("functionIndex").toInt(), 11);

    fixture.shell.clearKeyEvents();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::ShiftModifier,
                      f12Button->mapToScene(
                          QPointF(f12Button->width() / 2.0,
                                  f12Button->height() / 2.0)).toPoint());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.keyEvents.size(), 2, 1000);
    QCOMPARE(fixture.shell.keyEvents[0].value(QStringLiteral("vk")).toInt(),
             0x7b);
    QCOMPARE(fixture.shell.keyEvents[0].value(QStringLiteral("down")).toBool(),
             true);
    QCOMPARE(fixture.shell.keyEvents[0].value(QStringLiteral("mods")).toInt(),
             0x0010);
    QCOMPARE(fixture.shell.keyEvents[1].value(QStringLiteral("vk")).toInt(),
             0x7b);
    QCOMPARE(fixture.shell.keyEvents[1].value(QStringLiteral("down")).toBool(),
             false);
    QCOMPARE(fixture.shell.keyEvents[1].value(QStringLiteral("mods")).toInt(),
             0x0010);
}

void F4QuickViewSurfaceTests::readyUnifiedRendererLoaderIsVisible()
{
    // Exercise the production FilePanelView, not a copied Loader expression.
    // Its footer is also named `status`, so an unqualified Loader.status in
    // the visibility binding resolves to that sibling item and hides a fully
    // loaded renderer even though the semantic panel still has 120 entries.
    QuickViewFixture fixture(shellScene(), true);
    QVERIFY(fixture.window);
    auto *panel = fixture.item(QStringLiteral("filePanel-0"));
    auto *loader = fixture.item(QStringLiteral("galleryPanelContent-0"));
    auto *failure = fixture.item(QStringLiteral("panelRendererFailure-0"));
    QVERIFY(panel);
    QVERIFY(loader);
    QVERIFY(failure);
    QTRY_COMPARE_WITH_TIMEOUT(loader->property("status").toInt(), 1, 3000);
    QVERIFY(loader->property("item").value<QObject *>());
    QTRY_VERIFY_WITH_TIMEOUT(panel->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(loader->isVisible(), 3000);
    QVERIFY(!failure->isVisible());
}

void F4QuickViewSurfaceTests::rendererChoicesUseProductOrderAndShortcuts()
{
    QuickViewFixture fixture(shellScene(), true);
    QVERIFY(fixture.window);
    auto *panel = fixture.item(QStringLiteral("filePanel-0"));
    QVERIFY(panel);

    const QVariantList choices = panel->property("rendererChoices").toList();
    QCOMPARE(choices.size(), 8);
    const QList<QPair<QString, QString>> expected{
        {QStringLiteral("Columns · 2"), QStringLiteral("Ctrl+1")},
        {QStringLiteral("Columns · 3"), QStringLiteral("Ctrl+2")},
        {QStringLiteral("Details"), QStringLiteral("Ctrl+3")},
        {QStringLiteral("Icons"), QStringLiteral("Ctrl+5")},
        {QStringLiteral("Grid"), QStringLiteral("Ctrl+6")},
        {QStringLiteral("Masonry"), QStringLiteral("Ctrl+7")},
    };
    for (qsizetype i = 0; i < expected.size(); ++i) {
        const QVariantMap choice = choices.at(i).toMap();
        QCOMPARE(choice.value(QStringLiteral("label")).toString(),
                 expected.at(i).first);
        QCOMPARE(choice.value(QStringLiteral("shortcut")).toString(),
                 expected.at(i).second);
    }
    QVERIFY(choices.at(6).toMap().value(QStringLiteral("heading")).toBool());
    const QVariantMap wide = choices.at(7).toMap();
    QCOMPARE(wide.value(QStringLiteral("label")).toString(),
             QStringLiteral("Wide panel"));
    QCOMPARE(wide.value(QStringLiteral("shortcut")).toString(),
             QStringLiteral("Ctrl+4"));
}

void F4QuickViewSurfaceTests::coverUncoverPreservesFilePanelAndRendererObjects()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);
    auto *panel = fixture.item(QStringLiteral("filePanel-0"));
    auto *loader = fixture.item(QStringLiteral("galleryPanelContent-0"));
    auto *failure = fixture.item(QStringLiteral("panelRendererFailure-0"));
    QVERIFY(panel);
    QVERIFY(loader);
    QVERIFY(failure);
    QVERIFY(failure->isVisible());

    auto *const persistentPair = fixture.item(QStringLiteral("persistentPanelPair"));
    QVERIFY(persistentPair);
    // Optional native surfaces are created lazily.  The normal two-panel
    // startup must not pay for two hidden Quick View object trees.
    QVERIFY(!fixture.item(QStringLiteral("quickViewPanel-0")));

    fixture.shell.setScene(shellScene(
        QVariantList{quickView(0, false, QStringLiteral("file-A"), 0, 20, 1)}));
    QQuickItem *persistentQuick = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (persistentQuick = fixture.item(QStringLiteral("quickViewPanel-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(persistentQuick->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!panel->isVisible(), 3000);
    QCOMPARE(fixture.item(QStringLiteral("galleryPanelContent-0")), loader);
    QCOMPARE(fixture.item(QStringLiteral("panelRendererFailure-0")), failure);
    QCOMPARE(fixture.item(QStringLiteral("persistentPanelPair")), persistentPair);

    QVariant fallback;
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "needsFallbackGrid",
                                      Q_RETURN_ARG(QVariant, fallback)));
    QVERIFY(!fallback.toBool());

    fixture.shell.setScene(shellScene());
    QTRY_VERIFY_WITH_TIMEOUT(!persistentQuick->isVisible(), 3000);
    QCOMPARE(fixture.item(QStringLiteral("quickViewPanel-0")), persistentQuick);
    QTRY_VERIFY_WITH_TIMEOUT(panel->isVisible(), 3000);
    QCOMPARE(fixture.item(QStringLiteral("galleryPanelContent-0")), loader);
    QCOMPARE(fixture.item(QStringLiteral("panelRendererFailure-0")), failure);
    QVERIFY(failure->isVisible());
}

void F4QuickViewSurfaceTests::compactActivationPreservesPanelObjectsAndRebindsOnlyFocus()
{
    QuickViewFixture fixture(shellScene({}, 1), true);
    QVERIFY(fixture.window);

    QQuickItem *const leftPanel = fixture.item(QStringLiteral("filePanel-0"));
    QQuickItem *const rightPanel = fixture.item(QStringLiteral("filePanel-1"));
    QQuickItem *const leftLoader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QQuickItem *const rightLoader = fixture.item(
        QStringLiteral("galleryPanelContent-1"));
    QVERIFY(leftPanel);
    QVERIFY(rightPanel);
    QVERIFY(leftLoader);
    QVERIFY(rightLoader);
    QTRY_VERIFY_WITH_TIMEOUT(leftLoader->property("item").value<QObject *>(),
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(rightLoader->property("item").value<QObject *>(),
                             3000);
    QObject *const leftHost = leftLoader->property("item").value<QObject *>();
    QObject *const rightHost = rightLoader->property("item").value<QObject *>();
    QVERIFY(!leftHost->property("panelActive").toBool());
    QVERIFY(rightHost->property("panelActive").toBool());

    QSignalSpy sceneChanged(&fixture.shell, &TestShell::sceneChanged);
    fixture.shell.activatePanel(0, 1);

    QTRY_VERIFY_WITH_TIMEOUT(leftHost->property("panelActive").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!rightHost->property("panelActive").toBool(),
                             3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);
}

void F4QuickViewSurfaceTests::compactCatalogUpdatesOnlyChangedPanelPresentation()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);

    QQuickItem *const leftPanel = fixture.item(QStringLiteral("filePanel-0"));
    QQuickItem *const rightPanel = fixture.item(QStringLiteral("filePanel-1"));
    QQuickItem *const pathTitle = fixture.item(
        QStringLiteral("panelPathTitle-0"));
    QQuickItem *const leftLoader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QQuickItem *const rightLoader = fixture.item(
        QStringLiteral("galleryPanelContent-1"));
    QVERIFY(leftPanel);
    QVERIFY(rightPanel);
    QVERIFY(pathTitle);
    QVERIFY(leftLoader);
    QVERIFY(rightLoader);
    QTRY_VERIFY_WITH_TIMEOUT(leftLoader->property("item").value<QObject *>(),
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(rightLoader->property("item").value<QObject *>(),
                             3000);
    QObject *const leftHost = leftLoader->property("item").value<QObject *>();
    QObject *const rightHost = rightLoader->property("item").value<QObject *>();
    const QVariantMap initialRightPanel =
        rightPanel->property("panel").toMap();
    QSignalSpy sceneChanged(&fixture.shell, &TestShell::sceneChanged);
    QSignalSpy leftPanelChanged(leftPanel, SIGNAL(panelChanged()));
    QSignalSpy rightPanelChanged(rightPanel, SIGNAL(panelChanged()));

    QVariantMap projectedPanel = panel(0, true);
    projectedPanel.remove(QStringLiteral("entries"));
    projectedPanel.remove(QStringLiteral("highlightStyles"));
    projectedPanel.insert(QStringLiteral("path"), QStringLiteral("D:/next"));
    projectedPanel.insert(QStringLiteral("title"), QStringLiteral("D:/next"));
    projectedPanel.insert(QStringLiteral("loading"), true);
    projectedPanel.insert(QStringLiteral("catalogRevision"), qulonglong(9));
    projectedPanel.insert(QStringLiteral("cursor"), 3);
    projectedPanel.insert(QStringLiteral("cursorEntryId"),
                          QStringLiteral("entry-next"));
    projectedPanel.insert(QStringLiteral("galleryLayoutMode"),
                          QStringLiteral("details"));
    projectedPanel.insert(QStringLiteral("galleryColumnCount"), 3);
    projectedPanel.insert(QStringLiteral("galleryDensity"), 28);
    projectedPanel.insert(QStringLiteral("sortModeName"),
                          QStringLiteral("size"));
    projectedPanel.insert(QStringLiteral("sortReverse"), true);
    projectedPanel.insert(QStringLiteral("fastFind"), true);
    projectedPanel.insert(QStringLiteral("fastFindText"),
                          QStringLiteral("next"));
    projectedPanel.insert(QStringLiteral("selectedCount"), 2);
    projectedPanel.insert(QStringLiteral("totalCount"), 9);
    const QVariantMap compactTabs = {
        {QStringLiteral("visible"), true},
        {QStringLiteral("tabs"), QVariantList{}},
        {QStringLiteral("activeText"), QStringLiteral("D:/next")},
    };

    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("panel_catalog")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
        {QStringLiteral("workspaceTabs"), compactTabs},
    });

    QTRY_COMPARE_WITH_TIMEOUT(
        leftPanel->property("panel").toMap().value(
            QStringLiteral("path")).toString(),
        QStringLiteral("D:/next"), 3000);
    QCOMPARE(pathTitle->property("text").toString(),
             QStringLiteral("D:/next"));
    QCOMPARE(leftPanel->property("backendLoading").toBool(), true);
    QCOMPARE(leftHost->property("panel").toMap(), projectedPanel);
    QCOMPARE(leftHost->property("fastFindActive").toBool(), true);
    QCOMPARE(fixture.window->property("workspaceTabs").toMap(), compactTabs);
    QCOMPARE(fixture.window->property(
                 "leftPanelPresentationOverride").toMap(), projectedPanel);
    QVERIFY(!fixture.window->property(
                 "leftPanelPresentationOverride").toMap().contains(
                     QStringLiteral("entries")));
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(leftPanelChanged.size(), 1);
    QCOMPARE(rightPanelChanged.size(), 0);
    QCOMPARE(rightPanel->property("panel").toMap(), initialRightPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);

    // The full protocol signal remains available to native C++ consumers,
    // but QML must never inspect its catalog payload or use it as chrome.
    QVariantMap ignoredPanel = projectedPanel;
    ignoredPanel.insert(QStringLiteral("path"),
                        QStringLiteral("D:/raw-message-ignored"));
    fixture.shell.deliverMessage({
        {QStringLiteral("type"), QStringLiteral("panel_catalog")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), ignoredPanel},
        {QStringLiteral("workspaceTabs"), QVariantMap{
             {QStringLiteral("activeText"),
              QStringLiteral("raw-message-ignored")},
         }},
    });
    QCoreApplication::processEvents();
    QCOMPARE(pathTitle->property("text").toString(),
             QStringLiteral("D:/next"));
    QCOMPARE(fixture.window->property("workspaceTabs").toMap(), compactTabs);
    QCOMPARE(leftPanelChanged.size(), 1);
    QCOMPARE(rightPanelChanged.size(), 0);

    // A complete authoritative scene clears both current-only projections.
    QVariantMap authoritativeScene = shellScene({}, 0);
    QVariantMap authoritativeShell = authoritativeScene.value(
        QStringLiteral("shell")).toMap();
    QVariantList authoritativePanels = authoritativeShell.value(
        QStringLiteral("panels")).toList();
    QVariantMap authoritativeLeft = authoritativePanels.at(0).toMap();
    authoritativeLeft.insert(QStringLiteral("path"),
                             QStringLiteral("D:/authoritative"));
    authoritativeLeft.insert(QStringLiteral("title"),
                             QStringLiteral("D:/authoritative"));
    authoritativePanels[0] = authoritativeLeft;
    authoritativeShell.insert(QStringLiteral("panels"),
                              authoritativePanels);
    authoritativeScene.insert(QStringLiteral("shell"), authoritativeShell);
    fixture.shell.setScene(authoritativeScene);
    QTRY_COMPARE_WITH_TIMEOUT(pathTitle->property("text").toString(),
                              QStringLiteral("D:/authoritative"), 3000);
    QVERIFY(fixture.window->property(
                "leftPanelPresentationOverride").isNull());
    QVERIFY(fixture.window->property(
                "rightPanelPresentationOverride").isNull());
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);
}

void F4QuickViewSurfaceTests::compactChromeUpdatesWorkspaceTabsWithoutRebuildingPanels()
{
    const auto workspaceTabs = [](const QString &id, const QString &text) {
        return QVariantMap{
            {QStringLiteral("visible"), true},
            {QStringLiteral("tabs"), QVariantList{
                 QVariantMap{
                     {QStringLiteral("id"), id},
                     {QStringLiteral("text"), text},
                     {QStringLiteral("active"), true},
                     {QStringLiteral("closable"), false},
                 },
             }},
            {QStringLiteral("newTab"), QVariantMap{}},
            {QStringLiteral("counter"), QVariantMap{}},
        };
    };

    QVariantMap initialScene = shellScene({}, 0);
    initialScene.insert(QStringLiteral("workspaceTabs"),
                        workspaceTabs(QStringLiteral("workspace-old"),
                                      QStringLiteral("Old")));
    QuickViewFixture fixture(initialScene, true);
    QVERIFY(fixture.window);

    QQuickItem *const leftPanel = fixture.item(QStringLiteral("filePanel-0"));
    QQuickItem *const rightPanel = fixture.item(QStringLiteral("filePanel-1"));
    QQuickItem *const leftLoader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QQuickItem *const rightLoader = fixture.item(
        QStringLiteral("galleryPanelContent-1"));
    QVERIFY(leftPanel);
    QVERIFY(rightPanel);
    QVERIFY(leftLoader);
    QVERIFY(rightLoader);
    QTRY_VERIFY_WITH_TIMEOUT(leftLoader->property("item").value<QObject *>(),
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(rightLoader->property("item").value<QObject *>(),
                             3000);
    QObject *const leftHost = leftLoader->property("item").value<QObject *>();
    QObject *const rightHost = rightLoader->property("item").value<QObject *>();
    const QVariantMap initialTabs = workspaceTabs(
        QStringLiteral("workspace-old"), QStringLiteral("Old"));
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.window->property("workspaceTabs").toMap() == initialTabs,
        3000);

    QSignalSpy sceneChanged(&fixture.shell, &TestShell::sceneChanged);
    const QVariantMap compactTabs = workspaceTabs(
        QStringLiteral("workspace-compact"), QStringLiteral("Compact"));
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("panel_chrome")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("workspaceTabs"), compactTabs},
    });

    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.window->property("workspaceTabs").toMap() == compactTabs,
        3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(fixture.window->property("workspaceTabsOverride").toMap(),
             compactTabs);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);

    // Raw protocol messages remain observable to native consumers, but QML
    // accepts chrome only from the controller's validated compact signal.
    fixture.shell.deliverMessage({
        {QStringLiteral("type"), QStringLiteral("panel_chrome")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("workspaceTabs"),
         workspaceTabs(QStringLiteral("workspace-rejected"),
                       QStringLiteral("Rejected"))},
        {QStringLiteral("side"), 0},
    });
    QCoreApplication::processEvents();
    QCOMPARE(fixture.window->property("workspaceTabs").toMap(), compactTabs);
    QCOMPARE(fixture.window->property("workspaceTabsOverride").toMap(),
             compactTabs);

    // A complete authoritative scene clears the scalar override and resumes
    // the normal presentation binding without replacing either panel host.
    QVariantMap nextScene = shellScene({}, 0);
    nextScene.insert(QStringLiteral("workspaceTabs"),
                     workspaceTabs(QStringLiteral("workspace-scene"),
                                   QStringLiteral("Scene")));
    fixture.shell.setScene(nextScene);
    const QVariantMap sceneTabs = workspaceTabs(
        QStringLiteral("workspace-scene"), QStringLiteral("Scene"));
    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.window->property("workspaceTabs").toMap() == sceneTabs,
        3000);
    QVERIFY(fixture.window->property("workspaceTabsOverride").isNull());
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);
}

void F4QuickViewSurfaceTests::commandLineUsesOriginalSemanticRendererAndCursor()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    const QString text(180, QLatin1Char('x'));
    const QString promptIdentity = QStringLiteral("zoin@host");
    const QString promptLocation = QStringLiteral(":/path$ ");
    const QString prompt = promptIdentity + promptLocation;
    const QColor identityColor(QStringLiteral("#8ae234"));
    const QColor locationColor(QStringLiteral("#d3d7cf"));
    const QVariantList promptRuns{
        QVariantMap{
            {QStringLiteral("text"), promptIdentity},
            {QStringLiteral("foreground"), identityColor.name()},
            {QStringLiteral("background"), QStringLiteral("#555753")},
        },
        QVariantMap{
            {QStringLiteral("text"), promptLocation},
            {QStringLiteral("foreground"), locationColor.name()},
            {QStringLiteral("background"), QStringLiteral("#555753")},
        },
    };
    const QVariantList renderedRuns{QVariantMap{
        {QStringLiteral("text"), prompt + text},
        {QStringLiteral("foreground"), QStringLiteral("#e8edf2")},
        {QStringLiteral("background"), QStringLiteral("#000000")},
    }};
    shell.insert(QStringLiteral("commandLine"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("prompt"), prompt},
        {QStringLiteral("promptRuns"), promptRuns},
        {QStringLiteral("text"), text},
        {QStringLiteral("cursorPosition"), text.size()},
        {QStringLiteral("runs"), renderedRuns},
        {QStringLiteral("cursorPrefixRuns"), renderedRuns},
        {QStringLiteral("cursorShape"), QStringLiteral("underline")},
        {QStringLiteral("cursorVisible"), true},
    });
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *presentation = fixture.item(QStringLiteral("commandLinePresentation"));
    auto *promptItem = fixture.item(QStringLiteral("commandLinePrompt"));
    auto *inputItem = fixture.item(QStringLiteral("commandLineInput"));
    auto *cursor = fixture.item(QStringLiteral("commandLineCursor"));
    QVERIFY(presentation);
    QVERIFY(promptItem);
    QVERIFY(inputItem);
    QVERIFY(cursor);
    QCOMPARE(fixture.window->property("commandLineBg").value<QColor>().alpha(),
             0);
    QVERIFY2(presentation->width() > fixture.window->width() * 0.90,
             "the restored semantic command renderer must use the full row");
    QCOMPARE(presentation->x(), 16.0);
    QCOMPARE(cursor->height(), 2.0);
    QVERIFY(cursor->width() > 2.0);
    QVERIFY(cursor->isVisible());
    QCOMPARE(cursor->property("color").value<QColor>(), QColor(Qt::white));
    cursor->setProperty("blinkOn", false);
    auto *grid = fixture.window->findChild<TestGrid *>();
    QVERIFY(grid);
    emit grid->keyboardActivity();
    QTRY_VERIFY_WITH_TIMEOUT(cursor->property("blinkOn").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->x() > presentation->x(), 1000);

    QVariantMap updatedScene = scene;
    QVariantMap updatedShell = updatedScene.value(QStringLiteral("shell")).toMap();
    QVariantMap updatedCommandLine = updatedShell
        .value(QStringLiteral("commandLine")).toMap();
    updatedCommandLine.insert(QStringLiteral("text"), text + QStringLiteral("x"));
    updatedCommandLine.insert(QStringLiteral("cursorPosition"), text.size() + 1);
    updatedShell.insert(QStringLiteral("commandLine"), updatedCommandLine);
    updatedScene.insert(QStringLiteral("shell"), updatedShell);
    fixture.shell.setScene(updatedScene);
    QTRY_COMPARE_WITH_TIMEOUT(cursor->property("textPosition").toInt(),
                              text.size() + 1, 1000);

    updatedCommandLine.insert(QStringLiteral("text"), QStringLiteral("a"));
    updatedCommandLine.insert(QStringLiteral("cursorPosition"), 1);
    updatedShell.insert(QStringLiteral("commandLine"), updatedCommandLine);
    updatedScene.insert(QStringLiteral("shell"), updatedShell);
    fixture.shell.setScene(updatedScene);
    QTRY_COMPARE_WITH_TIMEOUT(cursor->property("textPosition").toInt(), 1, 1000);
    const qreal cursorBeforeTyping = cursor->x();

    updatedCommandLine.insert(QStringLiteral("text"), QStringLiteral("ab"));
    updatedCommandLine.insert(QStringLiteral("cursorPosition"), 2);
    updatedShell.insert(QStringLiteral("commandLine"), updatedCommandLine);
    updatedScene.insert(QStringLiteral("shell"), updatedShell);
    fixture.shell.setScene(updatedScene);
    QTRY_COMPARE_WITH_TIMEOUT(cursor->property("textPosition").toInt(), 2, 1000);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->x() > cursorBeforeTyping, 1000);

    QVERIFY2(promptItem->width() <= presentation->width() * 0.5 + 0.5,
             "the prompt may consume at most half of the command row");
    QVERIFY(promptItem->property("ignoreRunBackground").toBool());
    bool foundIdentityRun = false;
    bool foundLocationRun = false;
    QList<QQuickItem *> promptChildren = promptItem->childItems();
    for (qsizetype index = 0; index < promptChildren.size(); ++index) {
        QQuickItem *child = promptChildren.at(index);
        promptChildren.append(child->childItems());
        const QString childText = child->property("text").toString();
        const QColor childColor = child->property("color").value<QColor>();
        if (childText == promptIdentity && childColor == identityColor) {
            foundIdentityRun = true;
            QCOMPARE(child->parentItem()->property("color").value<QColor>().alpha(),
                     0);
        }
        if (childText == promptLocation && childColor == locationColor) {
            foundLocationRun = true;
            QCOMPARE(child->parentItem()->property("color").value<QColor>().alpha(),
                     0);
        }
    }
    QVERIFY2(foundIdentityRun,
             "the user/host prompt run must preserve its semantic foreground");
    QVERIFY2(foundLocationRun,
             "the path/suffix prompt run must preserve its semantic foreground");
    QCOMPARE(inputItem->property("text").toString(), QStringLiteral("ab"));
    QVERIFY2(inputItem->width() >= presentation->width() * 0.5 - 0.5,
             "the command input must retain at least half of the row");

    auto *backdrop = fixture.item(QStringLiteral("terminalBackdrop"));
    auto *commandLine = fixture.item(QStringLiteral("commandLineView"));
    QVERIFY(backdrop);
    QVERIFY(commandLine);
    const QColor terminalColor = backdrop->property("color").value<QColor>();
    const QColor commandLineColor =
        commandLine->property("color").value<QColor>();
    QCOMPARE(terminalColor.alphaF(), 0.0);
    QCOMPARE(commandLineColor.alphaF(), 0.0);
}

void F4QuickViewSurfaceTests::commandLineCursorTracksFirstTextPatch()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap commandLine{
        {QStringLiteral("visible"), true},
        {QStringLiteral("prompt"), QStringLiteral("> ")},
        {QStringLiteral("text"), QString()},
        {QStringLiteral("cursorPosition"), 0},
        {QStringLiteral("cursorShape"), QStringLiteral("underline")},
        {QStringLiteral("cursorVisible"), true},
    };
    shell.insert(QStringLiteral("commandLine"), commandLine);
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *input = fixture.item(QStringLiteral("commandLineInput"));
    auto *cursor = fixture.item(QStringLiteral("commandLineCursor"));
    QVERIFY(input);
    QVERIFY(cursor);
    QTRY_COMPARE_WITH_TIMEOUT(input->property("cursorPosition").toInt(), 0,
                              1000);
    const qreal initialCursorX = cursor->x();

    // Production delivers typing as a dedicated command_line patch: the
    // scene does not change, while text and cursorPosition change together.
    commandLine.insert(QStringLiteral("text"), QStringLiteral("a"));
    commandLine.insert(QStringLiteral("cursorPosition"), 1);
    fixture.shell.setCommandLine(commandLine);
    QTRY_COMPARE_WITH_TIMEOUT(input->property("text").toString(),
                              QStringLiteral("a"), 1000);
    QTRY_COMPARE_WITH_TIMEOUT(input->property("cursorPosition").toInt(), 1,
                              1000);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->x() > initialCursorX, 1000);

    commandLine.insert(QStringLiteral("text"), QStringLiteral("ab"));
    commandLine.insert(QStringLiteral("cursorPosition"), 2);
    fixture.shell.setCommandLine(commandLine);
    QTRY_COMPARE_WITH_TIMEOUT(input->property("cursorPosition").toInt(), 2,
                              1000);

    // Cursor-only patches model Left/Right, and subsequent typing must remain
    // synchronized rather than depending on that first navigation gesture.
    commandLine.insert(QStringLiteral("cursorPosition"), 1);
    fixture.shell.setCommandLine(commandLine);
    QTRY_COMPARE_WITH_TIMEOUT(input->property("cursorPosition").toInt(), 1,
                              1000);
    commandLine.insert(QStringLiteral("text"), QStringLiteral("acb"));
    commandLine.insert(QStringLiteral("cursorPosition"), 2);
    fixture.shell.setCommandLine(commandLine);
    QTRY_COMPARE_WITH_TIMEOUT(input->property("cursorPosition").toInt(), 2,
                              1000);
}

void F4QuickViewSurfaceTests::widePanelDoesNotRevealTerminalBackdrop()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("wide"), true);
    shell.insert(QStringLiteral("widePanel"), 1);
    shell.insert(QStringLiteral("terminal"), QVariantMap{
        {QStringLiteral("rows"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 0},
             {QStringLiteral("text"), QStringLiteral("must stay covered")},
         }}},
    });
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *backdrop = fixture.item(QStringLiteral("terminalBackdrop"));
    auto *widePanel = fixture.item(QStringLiteral("filePanel-1"));
    auto *passivePanel = fixture.item(QStringLiteral("filePanel-0"));
    QVERIFY(backdrop);
    QVERIFY(widePanel);
    QVERIFY(passivePanel);
    QTRY_VERIFY_WITH_TIMEOUT(widePanel->isVisible(), 3000);
    QVERIFY(!passivePanel->isVisible());
    QVERIFY(!backdrop->isVisible());
    QCOMPARE(widePanel->x(), 0.0);
    QCOMPARE(widePanel->width(), fixture.window->width());
}

void F4QuickViewSurfaceTests::autocompleteReturnTargetsShellCommandHandler()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("menus"), QVariantList{QVariantMap{
        {QStringLiteral("id"), QStringLiteral("autocomplete-menu")},
        {QStringLiteral("role"), QStringLiteral("autocomplete")},
        {QStringLiteral("query"), QStringLiteral("ls")},
        {QStringLiteral("items"), QVariantList{QVariantMap{
             {QStringLiteral("text"), QStringLiteral("ls")},
         }}},
    }});

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    fixture.shell.clearActions();
    QTest::keyClick(fixture.window, Qt::Key_Return);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    const QVariantMap action = fixture.shell.actions.constFirst();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("command.submit"));
    QCOMPARE(action.value(QStringLiteral("target")).toString(),
             QStringLiteral("shell"));
}

void F4QuickViewSurfaceTests::embeddedWheelCoalescesAndUsesQuickViewContract()
{
    QVariantMap qv = quickView(0, false, QStringLiteral("file-A"), 0, 20, 4);
    QuickViewFixture fixture(shellScene(QVariantList{qv}));
    QVERIFY(fixture.window);
    auto *surface = fixture.item(QStringLiteral("quickViewDocumentSurface-0"));
    QVERIFY(surface);
    auto *list = surface->findChild<QQuickItem *>(QStringLiteral("documentList"));
    auto *scrollBar = surface->findChild<QQuickItem *>(
        QStringLiteral("documentScrollBar"));
    QVERIFY(list);
    QVERIFY(scrollBar);
    QTRY_VERIFY_WITH_TIMEOUT(surface->property("windowInitialized").toBool(),
                             3000);
    QCOMPARE(surface->property("topInset").toReal(), 0.0);
    QCOMPARE(surface->property("bottomInset").toReal(), 0.0);
    QCOMPARE(list->y(), 0.0);
    QCOMPARE(list->height(), surface->height());
    QTRY_VERIFY_WITH_TIMEOUT(scrollBar->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(topVisualRow(surface, list) - 20.0) < 0.01,
                             3000);

    fixture.shell.clearActions();
    const QPointF center = surface->mapToScene(
        QPointF(surface->width() / 2, surface->height() / 2));
    sendPixelWheel(fixture.window, center.toPoint(), -13);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(topVisualRow(surface, list) - 20.65) < 0.02,
                             1000);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    const QVariantMap request = fixture.shell.actions.constFirst();
    QCOMPARE(request.value(QStringLiteral("action")).toString(),
             QStringLiteral("quickView.scroll"));
    QCOMPARE(request.value(QStringLiteral("target")).toString(),
             QStringLiteral("quick-view-0"));
    QCOMPARE(request.value(QStringLiteral("contentKey")).toString(),
             QStringLiteral("file-A"));
    QCOMPARE(request.value(QStringLiteral("visualRow")).toInt(), 20);
    QCOMPARE(request.value(QStringLiteral("generation")).toInt(), 5);
    QVERIFY(surface->property("windowRequestPending").toBool());

    sendPixelWheel(fixture.window, center.toPoint(), -7);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(topVisualRow(surface, list) - 21.0) < 0.02,
                             1000);
    QTest::qWait(260);
    QCOMPARE(fixture.shell.actions.size(), 1);

    QVariantMap surfaceMap = qv.value(QStringLiteral("surface")).toMap();
    surfaceMap.insert(QStringLiteral("windowGeneration"), 5);
    qv.insert(QStringLiteral("surface"), surfaceMap);
    fixture.shell.setScene(shellScene(QVariantList{qv}));
    QTRY_VERIFY_WITH_TIMEOUT(
        !surface->property("windowRequestPending").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(topVisualRow(surface, list) - 21.0) < 0.02,
                             3000);
    QVERIFY(scrollBar->property("size").toReal() > 0);
    QVERIFY(scrollBar->property("position").toReal() > 0);
}

void F4QuickViewSurfaceTests::contentKeyChangeDropsOldGestureAndAnchor()
{
    const QVariantMap qvA = quickView(0, false, QStringLiteral("file-A"),
                                      0, 20, 1, 500);
    QuickViewFixture fixture(shellScene(QVariantList{qvA}));
    QVERIFY(fixture.window);
    auto *surface = fixture.item(QStringLiteral("quickViewDocumentSurface-0"));
    QVERIFY(surface);
    auto *list = surface->findChild<QQuickItem *>(QStringLiteral("documentList"));
    QVERIFY(list);
    QTRY_VERIFY_WITH_TIMEOUT(surface->property("windowInitialized").toBool(),
                             3000);

    fixture.shell.clearActions();
    const QPointF center = surface->mapToScene(
        QPointF(surface->width() / 2, surface->height() / 2));
    sendPixelWheel(fixture.window, center.toPoint(), -13);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    QVERIFY(surface->property("windowRequestPending").toBool());
    QVERIFY(QMetaObject::invokeMethod(list, "flick", Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, -500.0)));

    const QVariantMap qvB = quickView(0, false, QStringLiteral("file-B"),
                                      80, 100, 2, 600);
    fixture.shell.setScene(shellScene(QVariantList{qvB}));
    QTRY_COMPARE_WITH_TIMEOUT(surface->property("appliedDocumentKey").toString(),
                              QStringLiteral("file-B"), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        !surface->property("windowRequestPending").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!list->property("flicking").toBool(), 3000);
    QCOMPARE(surface->property("queuedScrollBarPosition").toReal(), -1.0);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(topVisualRow(surface, list) - 100.0) < 0.01,
                             3000);
}

void F4QuickViewSurfaceTests::clickActivatesCoveredSideAndFocusStaysOutOfHiddenPanel()
{
    QVariantMap passive = quickView(0, false, QStringLiteral("file-A"),
                                    0, 20, 1);
    QuickViewFixture fixture(shellScene(QVariantList{passive}, 1));
    QVERIFY(fixture.window);
    auto *quickPanel = fixture.item(QStringLiteral("quickViewPanel-0"));
    auto *grid = fixture.window->findChild<TestGrid *>();
    QVERIFY(quickPanel);
    QVERIFY(grid);

    fixture.shell.clearActions();
    const QPointF clickPoint = quickPanel->mapToScene(
        QPointF(quickPanel->width() / 2, 10));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      clickPoint.toPoint());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.activate"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("side")).toInt(), 0);
    QTRY_VERIFY_WITH_TIMEOUT(grid->hasActiveFocus(), 1500);

    passive.insert(QStringLiteral("active"), true);
    fixture.shell.setScene(shellScene(QVariantList{passive}, 0));
    QTRY_VERIFY_WITH_TIMEOUT(grid->hasActiveFocus(), 1500);
    auto *coveredPanel = fixture.item(QStringLiteral("filePanel-0"));
    QVERIFY(coveredPanel);
    QVERIFY(!coveredPanel->isVisible());
}

void F4QuickViewSurfaceTests::previewKindsSelectExactlyOneNativeBody()
{
    QVariantMap qv = quickView(0, false, QStringLiteral("directory-A"),
                               0, 0, 1);
    qv.insert(QStringLiteral("previewKind"), QStringLiteral("directory"));
    qv.insert(QStringLiteral("headerRows"), QVariantList{});
    QuickViewFixture fixture(shellScene(QVariantList{qv}));
    QVERIFY(fixture.window);
    auto *document = fixture.item(QStringLiteral("quickViewDocumentSurface-0"));
    auto *directory = fixture.item(QStringLiteral("quickViewDirectoryList-0"));
    auto *image = fixture.item(QStringLiteral("quickViewImage-0"));
    auto *loading = fixture.item(QStringLiteral("quickViewLoading-0"));
    auto *error = fixture.item(QStringLiteral("quickViewError-0"));
    auto *header = fixture.item(QStringLiteral("quickViewHeader-0"));
    QVERIFY(document);
    QVERIFY(directory);
    QVERIFY(image);
    QVERIFY(loading);
    QVERIFY(error);
    QVERIFY(header);
    QTRY_VERIFY_WITH_TIMEOUT(directory->isVisible(), 3000);
    QVERIFY(!document->isVisible());
    QVERIFY(!loading->isVisible());
    QVERIFY(!error->isVisible());

    qv.insert(QStringLiteral("previewKind"), QStringLiteral("image"));
    qv.insert(QStringLiteral("contentKey"), QStringLiteral("image-B"));
    qv.insert(QStringLiteral("imageSource"), QString{});
    qv.insert(QStringLiteral("imageWidth"), 1);
    qv.insert(QStringLiteral("imageHeight"), 1);
    fixture.shell.setScene(shellScene(QVariantList{qv}));
    QTRY_VERIFY_WITH_TIMEOUT(image->isVisible(), 3000);
    QVERIFY(!directory->isVisible());
    QVERIFY(!document->isVisible());

    qv.insert(QStringLiteral("previewKind"), QStringLiteral("loading"));
    qv.insert(QStringLiteral("contentKey"), QStringLiteral("loading-C"));
    qv.insert(QStringLiteral("loading"), true);
    qv.insert(QStringLiteral("label"), QStringLiteral("Loading preview"));
    fixture.shell.setScene(shellScene(QVariantList{qv}));
    QTRY_VERIFY_WITH_TIMEOUT(loading->isVisible(), 3000);
    QVERIFY(!image->isVisible());
    QVERIFY(!error->isVisible());

    qv.insert(QStringLiteral("previewKind"), QStringLiteral("error"));
    qv.insert(QStringLiteral("contentKey"), QStringLiteral("error-D"));
    qv.insert(QStringLiteral("loading"), false);
    qv.insert(QStringLiteral("error"), QStringLiteral("Preview failed"));
    fixture.shell.setScene(shellScene(QVariantList{qv}));
    QTRY_VERIFY_WITH_TIMEOUT(error->isVisible(), 3000);
    QVERIFY(!loading->isVisible());
    QVERIFY(!document->isVisible());

    qv.insert(QStringLiteral("previewKind"), QStringLiteral("empty"));
    qv.insert(QStringLiteral("contentKey"), QStringLiteral("empty-E"));
    qv.insert(QStringLiteral("error"), QString{});
    qv.insert(QStringLiteral("headerRows"), QVariantList{
        QVariantMap{{QStringLiteral("text"), QStringLiteral("No selection")}},
    });
    fixture.shell.setScene(shellScene(QVariantList{qv}));
    QTRY_VERIFY_WITH_TIMEOUT(header->height() > 0, 3000);
    QVERIFY(!document->isVisible());
    QVERIFY(!directory->isVisible());
    QVERIFY(!image->isVisible());
    QVERIFY(!loading->isVisible());
    QVERIFY(!error->isVisible());
}

QTEST_MAIN(F4QuickViewSurfaceTests)
#include "F4QuickViewSurfaceTests.moc"
