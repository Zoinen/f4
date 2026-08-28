#include "DummyQWK.h"
#include "F4TextRenderingPolicy.h"

#include <QCoreApplication>
#include <QColor>
#include <QFont>
#include <QImage>
#include <QPainter>
#include <QPointF>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QScopeGuard>
#include <QSvgRenderer>
#include <QUrl>
#include <QUrlQuery>
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
    Q_PROPERTY(QVariantList commandMenus READ commandMenus NOTIFY commandMenusChanged)

public:
    int initialCols() const { return 110; }
    int initialRows() const { return 34; }
    QVariantMap scene() const { return m_scene; }
    QVariantMap commandLine() const { return m_commandLine; }
    QVariantList commandMenus() const { return m_commandMenus; }

    void setScene(const QVariantMap &scene)
    {
        m_scene = scene;
        m_commandLine = scene.value(QStringLiteral("shell")).toMap()
                            .value(QStringLiteral("commandLine")).toMap();
        const QVariantList menus = scene.value(QStringLiteral("menus")).toList();
        if (menus != m_commandMenus) {
            m_commandMenus = menus;
            emit commandMenusChanged();
        }
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
    void deliverCommandMenuStates(const QVariantList &states)
    {
        emit commandMenuStatesChanged(states);
    }
    void setCommandMenus(const QVariantList &menus)
    {
        if (menus == m_commandMenus)
            return;
        m_commandMenus = menus;
        emit commandMenusChanged();
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
    void commandMenusChanged();
    void commandMenuStatesChanged(const QVariantList &states);
    void panelActivationChanged(int activePanel, qulonglong revision);
    void compactPresentationChanged(const QVariantMap &patch);
    void messageReceived(const QVariantMap &message);
    void uiActionSent(const QVariantMap &action);

private:
    QVariantMap m_scene;
    QVariantMap m_commandLine;
    QVariantList m_commandMenus;
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

    Q_INVOKABLE QUrl iconSource(const QString &name, int, qreal) const
    {
        return QUrl(QStringLiteral("qrc:/F4QtHost/icons/lucide/%1.svg")
                        .arg(name));
    }
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
        {QStringLiteral("showFileInfo"), true},
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

QQuickItem *visualItemWithObjectNamePrefix(QQuickItem *root,
                                           const QString &prefix)
{
    if (!root)
        return nullptr;
    if (root->objectName().startsWith(prefix))
        return root;
    for (QQuickItem *child : root->childItems()) {
        if (QQuickItem *match = visualItemWithObjectNamePrefix(child, prefix))
            return match;
    }
    return nullptr;
}

QQuickItem *visualItemWithSource(QQuickItem *root, const QUrl &source)
{
    if (!root)
        return nullptr;
    if (root->property("source").isValid()
        && root->property("source").toUrl() == source) {
        return root;
    }
    for (QQuickItem *child : root->childItems()) {
        if (QQuickItem *match = visualItemWithSource(child, source))
            return match;
    }
    return nullptr;
}

QImage renderSvgReference(const QUrl &source, const QSize &physicalSize,
                          const QColor &tint, const QColor &background)
{
    QSvgRenderer renderer(QStringLiteral(":") + source.path());
    if (!renderer.isValid() || !physicalSize.isValid())
        return {};

    QImage icon(physicalSize, QImage::Format_ARGB32_Premultiplied);
    icon.fill(Qt::transparent);
    QPainter iconPainter(&icon);
    renderer.render(&iconPainter,
                    QRectF(QPointF{}, QSizeF(physicalSize)));
    iconPainter.end();

    if (tint.isValid() && tint.alpha() > 0) {
        QPainter tintPainter(&icon);
        tintPainter.setCompositionMode(QPainter::CompositionMode_SourceIn);
        tintPainter.fillRect(icon.rect(), tint);
    }

    QImage result(physicalSize, QImage::Format_ARGB32_Premultiplied);
    result.fill(background);
    QPainter resultPainter(&result);
    resultPainter.drawImage(QPoint{}, icon);
    return result;
}

QString exactImageDifference(const QImage &actualImage,
                             const QImage &expectedImage)
{
    if (actualImage.size() != expectedImage.size()) {
        return QStringLiteral("size %1x%2 != %3x%4")
            .arg(actualImage.width()).arg(actualImage.height())
            .arg(expectedImage.width()).arg(expectedImage.height());
    }
    const QImage actual = actualImage.convertToFormat(
        QImage::Format_ARGB32_Premultiplied);
    const QImage expected = expectedImage.convertToFormat(
        QImage::Format_ARGB32_Premultiplied);
    qsizetype differences = 0;
    QPoint firstDifference(-1, -1);
    for (int y = 0; y < actual.height(); ++y) {
        const auto *actualLine = reinterpret_cast<const QRgb *>(
            actual.constScanLine(y));
        const auto *expectedLine = reinterpret_cast<const QRgb *>(
            expected.constScanLine(y));
        for (int x = 0; x < actual.width(); ++x) {
            if (actualLine[x] == expectedLine[x])
                continue;
            if (firstDifference.x() < 0)
                firstDifference = QPoint(x, y);
            ++differences;
        }
    }
    if (differences == 0)
        return {};
    return QStringLiteral("%1 differing pixels; first at (%2,%3)")
        .arg(differences).arg(firstDifference.x()).arg(firstDifference.y());
}

bool imageContainsColor(const QImage &image, const QColor &color)
{
    const QImage actual = image.convertToFormat(
        QImage::Format_ARGB32_Premultiplied);
    const QRgb expected = color.rgba();
    for (int y = 0; y < actual.height(); ++y) {
        const auto *line = reinterpret_cast<const QRgb *>(
            actual.constScanLine(y));
        for (int x = 0; x < actual.width(); ++x) {
            if (line[x] == expected)
                return true;
        }
    }
    return false;
}

struct QuickViewFixture
{
    TestShell shell;
    TestGallery gallery;
    TestIcons icons;
    F4TextRenderingPolicy textRenderingPolicy;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;

    explicit QuickViewFixture(const QVariantMap &scene,
                              bool galleryAvailable = false,
                              bool usesQwk = false)
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
            QStringLiteral("qtTextRendering"), &textRenderingPolicy);
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
    void semanticHorizontalSplitStaysOnNativeSurface();
    void functionBarShowsExplicitFunctionKeysAndForwardsMouseModifiers();
    void readyUnifiedRendererLoaderIsVisible();
    void panelFileInfoSettingTogglesFooterWithoutRebuildingPanel();
    void fastFindOverlayIsIndependentFromPanelFooter();
    void galleryPanelColorsAreGroupedAndRemainLive();
    void themeDialogFontRenderingControlIsLiveAndThemeAware();
    void themeColorListHoverAndPressFlashHaveExplicitLifetimes();
    void themeDialogControlsStayOnPhysicalPixelGridAt175Percent();
    void rendererChoicesUseProductOrderAndShortcuts();
    void coverUncoverPreservesFilePanelAndRendererObjects();
    void compactActivationPreservesPanelObjectsAndRebindsOnlyFocus();
    void compactCatalogUpdatesOnlyChangedPanelPresentation();
    void compactChromeUpdatesWorkspaceTabsWithoutRebuildingPanels();
    void workspaceSeparatorBreaksUnderActiveTab();
    void workspaceTabTextParentsStayOnPhysicalPixelGrid();
    void chromeIconsUseMatchingPhysicalTargetSizes();
    void panelDriveButtonUsesPathIconAndRequestsDriveMenu();
    void driveMenuIconsUseSemanticModelAndLiveTheme();
    void compactMenuStructureTransfersFocusWithoutSceneRebind();
    void menuKeyboardSelectionSurvivesStationaryPointerPatch();
    void pathBreadcrumbTextStaysFixedWhenNavigatingDeeper();
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

void F4QuickViewSurfaceTests::panelFileInfoSettingTogglesFooterWithoutRebuildingPanel()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);
    QQuickItem *const leftPanel = fixture.item(QStringLiteral("filePanel-0"));
    QQuickItem *const footer = fixture.item(QStringLiteral("panelStatus-0"));
    QQuickItem *const loader = fixture.item(QStringLiteral("galleryPanelContent-0"));
    QVERIFY(leftPanel);
    QVERIFY(footer);
    QVERIFY(loader);
    QTRY_VERIFY_WITH_TIMEOUT(loader->property("item").value<QObject *>(), 3000);
    QObject *const galleryHost = loader->property("item").value<QObject *>();
    QTRY_VERIFY_WITH_TIMEOUT(footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(footer->height() > 0.0, 3000);
    const qreal footerHeight = footer->height();
    const qreal contentHeightWithFooter = loader->height();

    QVariantMap projectedPanel = panel(0, true);
    projectedPanel.remove(QStringLiteral("entries"));
    projectedPanel.remove(QStringLiteral("highlightStyles"));
    projectedPanel.insert(QStringLiteral("showFileInfo"), false);
    QSignalSpy sceneChanged(&fixture.shell, &TestShell::sceneChanged);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });

    QTRY_VERIFY_WITH_TIMEOUT(!footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(footer->height()) < 0.01, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(loader->height() >= contentHeightWithFooter + footerHeight - 1.0, 3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("galleryPanelContent-0")), loader);
    QCOMPARE(loader->property("item").value<QObject *>(), galleryHost);

    projectedPanel.insert(QStringLiteral("showFileInfo"), true);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(footer->height() - footerHeight) < 0.01, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(loader->height() - contentHeightWithFooter) < 1.0, 3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(loader->property("item").value<QObject *>(), galleryHost);
}

void F4QuickViewSurfaceTests::fastFindOverlayIsIndependentFromPanelFooter()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);
    QQuickItem *const leftPanel = fixture.item(QStringLiteral("filePanel-0"));
    QQuickItem *const footer = fixture.item(QStringLiteral("panelStatus-0"));
    QQuickItem *const footerSelection = fixture.item(
        QStringLiteral("panelStatusSelection-0"));
    QQuickItem *const overlay = fixture.item(
        QStringLiteral("panelFastFindOverlay-0"));
    QQuickItem *const overlayText = fixture.item(
        QStringLiteral("panelFastFindText-0"));
    QQuickItem *const overlayCursor = fixture.item(
        QStringLiteral("panelFastFindCursor-0"));
    QQuickItem *const loader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QVERIFY(leftPanel);
    QVERIFY(footer);
    QVERIFY(footerSelection);
    QVERIFY(overlay);
    QVERIFY(!fixture.item(QStringLiteral("panelFastFindHeader-0")));
    QVERIFY(overlayText);
    QVERIFY(overlayCursor);
    QVERIFY(loader);
    QTRY_VERIFY_WITH_TIMEOUT(loader->property("item").value<QObject *>(),
                             3000);
    QObject *const galleryHost = loader->property("item").value<QObject *>();
    QVERIFY(!overlay->isVisible());

    QVariantMap projectedPanel = panel(0, true);
    projectedPanel.remove(QStringLiteral("entries"));
    projectedPanel.remove(QStringLiteral("highlightStyles"));
    projectedPanel.insert(QStringLiteral("showFileInfo"), false);
    projectedPanel.insert(QStringLiteral("fastFind"), false);
    projectedPanel.insert(QStringLiteral("fastFindText"), QString{});
    projectedPanel.insert(QStringLiteral("selectedCount"), 7);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });

    QTRY_VERIFY_WITH_TIMEOUT(!footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!overlay->isVisible(), 3000);
    const qreal contentHeightWithoutFooter = loader->height();

    projectedPanel.insert(QStringLiteral("fastFind"), true);
    projectedPanel.insert(QStringLiteral("fastFindText"),
                          QStringLiteral("needle"));
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });

    QTRY_VERIFY_WITH_TIMEOUT(overlay->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(overlayCursor->isVisible(), 1000);
    QCOMPARE(overlayText->property("text").toString(),
             QStringLiteral("needle"));
    QVERIFY(overlayCursor->x() > overlayText->x());
    QVERIFY(overlayCursor->x() < overlayText->x() + overlayText->width());
    const qreal dpr = fixture.window->property("dpr").toReal();
    const qreal expectedCursorWidth = std::round(2.0 * dpr) / dpr;
    QCOMPARE(overlayCursor->width(), expectedCursorWidth);
    QVERIFY(overlayCursor->height() > 0.0);
    QCOMPARE(footerSelection->property("text").toString(),
             QStringLiteral("7 selected"));
    QVERIFY(!footer->isVisible());
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(overlay->x() + overlay->width() / 2.0
             - leftPanel->width() / 2.0) < 0.01,
        1000);
    QVERIFY(overlay->property("radius").toReal() > 0.0);
    QVERIFY(qAbs(loader->height() - contentHeightWithoutFooter) < 0.01);
    QVERIFY(overlay->z() > loader->z());
    QVERIFY(overlay->y() >= loader->y());
    QVERIFY(overlay->y() + overlay->height()
            <= loader->y() + loader->height() + 0.01);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(loader->property("item").value<QObject *>(), galleryHost);

    const QColor dialogBackground(QStringLiteral("#26384a"));
    const QColor queryTextColor(QStringLiteral("#f4d35e"));
    fixture.window->setProperty("dialogBg", dialogBackground);
    fixture.window->setProperty("textColor", queryTextColor);
    QTRY_COMPARE_WITH_TIMEOUT(overlay->property("color").value<QColor>(),
                               dialogBackground, 1000);
    QTRY_COMPARE_WITH_TIMEOUT(overlayText->property("color").value<QColor>(),
                              queryTextColor, 1000);

    projectedPanel.insert(QStringLiteral("showFileInfo"), true);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(overlay->isVisible(), 3000);
    QVERIFY(overlay->y() + overlay->height() <= footer->y() + 0.01);
    QVERIFY(loader->height() < contentHeightWithoutFooter);

    projectedPanel.insert(QStringLiteral("showFileInfo"), false);
    projectedPanel.insert(QStringLiteral("fastFind"), false);
    projectedPanel.insert(QStringLiteral("fastFindText"), QString{});
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(!footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!overlay->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!overlayCursor->isVisible(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(loader->height() - contentHeightWithoutFooter) < 0.01, 3000);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(loader->property("item").value<QObject *>(), galleryHost);
}

void F4QuickViewSurfaceTests::semanticHorizontalSplitStaysOnNativeSurface()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("width"), 100);
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("panelLayout"), QVariantMap{
        {QStringLiteral("columns"), 100},
        {QStringLiteral("splitColumn"), 44},
        {QStringLiteral("leftBottomInsetRows"), 0},
        {QStringLiteral("rightBottomInsetRows"), 0},
    });
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *grid = fixture.item<TestGrid>(QStringLiteral("vtuiGrid"));
    auto *left = fixture.item(QStringLiteral("filePanel-0"));
    auto *right = fixture.item(QStringLiteral("filePanel-1"));
    QVERIFY(grid);
    QVERIFY(left);
    QVERIFY(right);

    QTRY_VERIFY_WITH_TIMEOUT(!grid->renderingEnabled(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.window->property("panelSplitRatio").toReal() - 0.44)
            < 0.0001,
        3000);
    const qreal expectedSplit = fixture.window->width() * 0.44;
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(left->width() - expectedSplit) < 1.0, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(right->x() - expectedSplit) < 1.0, 3000);

    QVariant fallback;
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "needsFallbackGrid",
                                      Q_RETURN_ARG(QVariant, fallback)));
    QVERIFY(!fallback.toBool());
}

void F4QuickViewSurfaceTests::galleryPanelColorsAreGroupedAndRemainLive()
{
    QuickViewFixture fixture(shellScene(), true);
    QVERIFY(fixture.window);

    auto *loader = fixture.item(QStringLiteral("galleryPanelContent-0"));
    auto *path = fixture.item(QStringLiteral("panelPathTitle-0"));
    QVERIFY(loader);
    QVERIFY(path);
    QTRY_COMPARE_WITH_TIMEOUT(loader->property("status").toInt(), 1, 3000);
    QObject *const gallery = loader->property("item").value<QObject *>();
    QVERIFY(gallery);

    const QVariantList definitions =
        fixture.window->property("themeColorDefinitions").toList();
    QStringList panelColorIds;
    for (const QVariant &definitionValue : definitions) {
        const QVariantMap definition = definitionValue.toMap();
        if (definition.value(QStringLiteral("group")).toString()
            == QStringLiteral("Panel Colors")) {
            panelColorIds.append(
                definition.value(QStringLiteral("id")).toString());
        }
    }
    for (const QString &required : {
             QStringLiteral("galleryPanelBackgroundColor"),
             QStringLiteral("galleryTextColor"),
             QStringLiteral("galleryCardCursorBorderColor"),
             QStringLiteral("galleryItemHoverColor"),
             QStringLiteral("galleryPreviewBackdropColor"),
             QStringLiteral("galleryScrollBarHandleColor"),
             QStringLiteral("galleryScrollBarTrackHoverColor"),
             QStringLiteral("galleryPathTextColor"),
         }) {
        QVERIFY2(panelColorIds.contains(required), qPrintable(required));
    }

    const QColor textColor(QStringLiteral("#123456"));
    const QColor cursorBorder(QStringLiteral("#234567"));
    const QColor cardHover(QStringLiteral("#345678"));
    const QColor scrollHandle(QStringLiteral("#456789"));
    const QColor scrollTrack(QStringLiteral("#556677"));
    const QColor pathText(QStringLiteral("#6789ab"));
    const QColor pathBackground(QStringLiteral("#223344"));

    QVERIFY(fixture.window->setProperty("galleryTextColor", textColor));
    QVERIFY(fixture.window->setProperty("galleryCardCursorBorderColor",
                                        cursorBorder));
    QVERIFY(fixture.window->setProperty("galleryItemHoverColor", cardHover));
    QVERIFY(fixture.window->setProperty("galleryScrollBarHandleColor",
                                        scrollHandle));
    QVERIFY(fixture.window->setProperty("galleryScrollBarTrackHoverColor",
                                        scrollTrack));
    QVERIFY(fixture.window->setProperty("galleryPathTextColor", pathText));
    QVERIFY(fixture.window->setProperty("galleryPathBackgroundColor",
                                        pathBackground));

    const auto liveTheme = [gallery] {
        return gallery->property("theme").toMap();
    };
    QTRY_COMPARE_WITH_TIMEOUT(
        liveTheme().value(QStringLiteral("text")).value<QColor>(), textColor,
        3000);
    QCOMPARE(liveTheme().value(QStringLiteral("cardCursorBorder"))
                 .value<QColor>(),
             cursorBorder);
    QCOMPARE(liveTheme().value(QStringLiteral("itemHover")).value<QColor>(),
             cardHover);
    QCOMPARE(liveTheme().value(QStringLiteral("scrollBarHandle"))
                 .value<QColor>(),
             scrollHandle);
    QCOMPARE(liveTheme().value(QStringLiteral("scrollBarTrackHovered"))
                 .value<QColor>(),
             scrollTrack);
    QTRY_COMPARE_WITH_TIMEOUT(path->property("pathTextColor").value<QColor>(),
                              pathText, 3000);
    QCOMPARE(path->property("pathBackgroundColor").value<QColor>(),
             pathBackground);
    QCOMPARE(loader->property("item").value<QObject *>(), gallery);
}

void F4QuickViewSurfaceTests::themeDialogFontRenderingControlIsLiveAndThemeAware()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);

    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    QVERIFY(dialog);
    auto *combo = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeFontRenderTypeCombo"));
    QVERIFY(combo);
    auto *wheelCombo = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeMouseWheelCombo"));
    QVERIFY(wheelCombo);
    QObject *const comboBackground = combo->findChild<QObject *>(
        QStringLiteral("themeFontRenderTypeComboBackground"));
    QVERIFY(comboBackground);

    QCOMPARE(fixture.textRenderingPolicy.options().size(), 3);
    QCOMPARE(fixture.window->property("fontRenderType").toInt(),
             int(QQuickWindow::NativeTextRendering));
    QCOMPARE(combo->property("currentText").toString(),
             QStringLiteral("NativeRendering"));
    QCOMPARE(fixture.window->property("mouseWheelMode").toString(),
             QStringLiteral("gui"));
    QCOMPARE(wheelCombo->property("currentText").toString(),
             QStringLiteral("GUI scrolling"));
    QCOMPARE(fixture.window->property("mouseWheelModeOptions")
                 .toList().size(), 2);

    const QColor dialogBackground(QStringLiteral("#102030"));
    const QColor controlBackground(QStringLiteral("#304050"));
    const QColor controlBorder(QStringLiteral("#405060"));
    const QColor accent(QStringLiteral("#607080"));
    const QColor selected(QStringLiteral("#708090"));
    for (const auto &entry : {
             qMakePair("dialogBg", dialogBackground),
             qMakePair("controlBg", controlBackground),
             qMakePair("controlBorder", controlBorder),
             qMakePair("dialogAccent", accent),
             qMakePair("selectedBg", selected),
         }) {
        QVERIFY(fixture.window->setProperty(entry.first, entry.second));
    }
    dialog->resize(720, 560);
    dialog->show();
    dialog->requestActivate();
    QCoreApplication::processEvents();

    QImage normalFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(normalFrame = dialog->grabWindow()).isNull(), 3000);
    QVERIFY(imageContainsColor(normalFrame, dialogBackground));
    QCOMPARE(comboBackground->property("color").value<QColor>(),
             controlBackground);
    QCOMPARE(comboBackground->property("testBorderColor").value<QColor>(),
             controlBorder);

    QVERIFY(QMetaObject::invokeMethod(combo, "forceActiveFocus"));
    QCoreApplication::processEvents();
    QCOMPARE(comboBackground->property("testBorderColor").value<QColor>(),
             accent);

    QObject *const popup = combo->property("popup").value<QObject *>();
    QVERIFY(popup);
    QVERIFY(QMetaObject::invokeMethod(popup, "open"));
    QTRY_VERIFY_WITH_TIMEOUT(popup->property("visible").toBool(), 1000);
    QImage selectedFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(selectedFrame = dialog->grabWindow()).isNull(), 3000);
    QVERIFY(imageContainsColor(selectedFrame, selected));
    QVERIFY(QMetaObject::invokeMethod(popup, "close"));
    QTRY_VERIFY_WITH_TIMEOUT(!popup->property("visible").toBool(), 1000);

    const QColor changedDialogBackground(QStringLiteral("#a0b0c0"));
    const QColor changedControlBackground(QStringLiteral("#b0c0d0"));
    QVERIFY(fixture.window->setProperty("dialogBg", changedDialogBackground));
    QVERIFY(fixture.window->setProperty("controlBg", changedControlBackground));
    QTRY_COMPARE_WITH_TIMEOUT(
        comboBackground->property("color").value<QColor>(),
        changedControlBackground, 3000);
    QImage changedFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(changedFrame = dialog->grabWindow()).isNull()
            && imageContainsColor(changedFrame, changedDialogBackground),
        3000);

    fixture.textRenderingPolicy.setRenderType(
        int(QQuickWindow::QtTextRendering));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property("fontRenderType").toInt(),
        int(QQuickWindow::QtTextRendering), 1000);
    QTRY_COMPARE_WITH_TIMEOUT(combo->property("currentText").toString(),
                              QStringLiteral("QtRendering"), 1000);
    QVERIFY(fixture.window->setProperty("mouseWheelMode",
                                        QStringLiteral("console")));
    QTRY_COMPARE_WITH_TIMEOUT(
        wheelCombo->property("currentText").toString(),
        QStringLiteral("F4 console"), 1000);
    QCOMPARE(fixture.window->property("mouseWheelMode").toString(),
             QStringLiteral("console"));
    QVERIFY(fixture.window->setProperty("mouseWheelMode",
                                        QStringLiteral("gui")));
#if QT_VERSION >= QT_VERSION_CHECK(6, 8, 0)
    fixture.textRenderingPolicy.setRenderType(
        int(QQuickWindow::CurveTextRendering));
    QTRY_COMPARE_WITH_TIMEOUT(
        combo->property("currentText").toString(),
        QStringLiteral("CurveRendering"), 1000);
#endif

    QMetaObject::invokeMethod(popup, "close");
    dialog->hide();
}

void F4QuickViewSurfaceTests::themeColorListHoverAndPressFlashHaveExplicitLifetimes()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);

    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    QVERIFY(dialog);

    const QVariantList definitions = fixture.window->property(
        "themeColorDefinitions").toList();
    int colorIndex = -1;
    QString colorId;
    for (int index = 0; index < definitions.size(); ++index) {
        const QVariantMap definition = definitions.at(index).toMap();
        const QString candidate = definition.value(QStringLiteral("id"))
                                      .toString();
        if (!candidate.isEmpty() && fixture.window->property(
                candidate.toUtf8().constData()).isValid()) {
            colorIndex = index;
            colorId = candidate;
            break;
        }
    }
    QVERIFY(colorIndex >= 0);
    QVERIFY(!colorId.isEmpty());

    const QColor original(QStringLiteral("#123456"));
    const QColor activeHighlight(QStringLiteral("#00ff00"));
    QCOMPARE(dialog->property("activeFlashColor").value<QColor>(),
             activeHighlight);
    QVERIFY(fixture.window->setProperty(
        colorId.toUtf8().constData(), original));
    dialog->setProperty("selectedIndex", colorIndex == 0 ? 1 : 0);

    const auto invokeColorMethod = [&](const char *method) {
        return QMetaObject::invokeMethod(
            dialog, method, Qt::DirectConnection,
            Q_ARG(QVariant, QVariant(colorId)));
    };

    // Hover preview reaches green and remains there until the pointer leaves.
    QVERIFY(invokeColorMethod("flash"));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property(colorId.toUtf8().constData()).value<QColor>(),
        activeHighlight, 1000);
    QTest::qWait(220);
    QCOMPARE(fixture.window->property(colorId.toUtf8().constData())
                 .value<QColor>(), activeHighlight);
    QVERIFY(invokeColorMethod("endHoverFlash"));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property(colorId.toUtf8().constData()).value<QColor>(),
        original, 1000);

    // Once active, hover is quiet, but an actual press still marks the row
    // green while held and returns to the source color on release.
    dialog->setProperty("selectedIndex", colorIndex);
    QCoreApplication::processEvents();
    QCOMPARE(fixture.window->property(colorId.toUtf8().constData())
                 .value<QColor>(), original);
    QVERIFY(invokeColorMethod("flash"));
    QTest::qWait(220);
    QCOMPARE(fixture.window->property(colorId.toUtf8().constData())
                 .value<QColor>(), original);

    QVERIFY(invokeColorMethod("startPressFlash"));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property(colorId.toUtf8().constData()).value<QColor>(),
        activeHighlight, 1000);
    QTest::qWait(220);
    QCOMPARE(fixture.window->property(colorId.toUtf8().constData())
                 .value<QColor>(), activeHighlight);
    QVERIFY(invokeColorMethod("endPressFlash"));
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property(colorId.toUtf8().constData()).value<QColor>(),
        original, 1000);
    QTest::qWait(220);
    QCOMPARE(fixture.window->property(colorId.toUtf8().constData())
                 .value<QColor>(), original);
}

void F4QuickViewSurfaceTests::themeDialogControlsStayOnPhysicalPixelGridAt175Percent()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);

    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    QVERIFY(dialog);
    const qreal dpr = dialog->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required");

    dialog->resize(720, 560);
    dialog->show();
    dialog->requestActivate();
    QTRY_VERIFY_WITH_TIMEOUT(dialog->isVisible(), 1000);
    QCoreApplication::processEvents();

    QQuickItem *const dialogRoot = dialog->contentItem();
    QVERIFY(dialogRoot);

    QQuickItem *const themeItemsList = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeItemsList"));
    QVERIFY(themeItemsList);
    QTRY_VERIFY_WITH_TIMEOUT(themeItemsList->isVisible(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(themeItemsList->height() >= 35.0, 1000);
    QVERIFY(themeItemsList->property("count").toInt() > 0);
    QTRY_VERIFY_WITH_TIMEOUT(
        themeItemsList->property("contentHeight").toReal()
            > themeItemsList->height(),
        1000);

    const auto verifyWholePhysicalCoordinate = [dpr](
            qreal logicalCoordinate, const QString &description) {
        const qreal physicalCoordinate = logicalCoordinate * dpr;
        const QByteArray details = QStringLiteral(
            "%1 is %2 physical pixels at DPR %3")
                                       .arg(description)
                                       .arg(physicalCoordinate, 0, 'f', 6)
                                       .arg(dpr, 0, 'f', 2)
                                       .toUtf8();
        QVERIFY2(qAbs(physicalCoordinate - qRound(physicalCoordinate))
                     < 0.001,
                 details.constData());
    };
    const auto verifyItem = [&](QQuickItem *item, const QString &name) {
        QVERIFY2(item, qPrintable(name));
        if (!item)
            return;
        const QPointF origin = item->mapToItem(dialogRoot, QPointF{});
        verifyWholePhysicalCoordinate(origin.x(), name + QStringLiteral(" x"));
        verifyWholePhysicalCoordinate(origin.y(), name + QStringLiteral(" y"));
        verifyWholePhysicalCoordinate(item->width(),
                                      name + QStringLiteral(" width"));
        verifyWholePhysicalCoordinate(item->height(),
                                      name + QStringLiteral(" height"));
    };

    const QStringList controlNames{
        QStringLiteral("themeFontRenderTypePanel"),
        QStringLiteral("themeFontRenderTypeCombo"),
        QStringLiteral("themeFontRenderTypeComboIndicator"),
        QStringLiteral("themeFontRenderTypeComboBackground"),
        QStringLiteral("themeMouseWheelPanel"),
        QStringLiteral("themeMouseWheelCombo"),
        QStringLiteral("themeMouseWheelComboIndicator"),
        QStringLiteral("themeMouseWheelComboBackground"),
        QStringLiteral("themeHeaderDivider"),
        QStringLiteral("themeDialogHeader"),
        QStringLiteral("themeColorFilter"),
        QStringLiteral("themeItemsList"),
        QStringLiteral("themeListScrollBar"),
        QStringLiteral("themeColorDivider"),
        QStringLiteral("themeActiveColorRow"),
        QStringLiteral("themeColorGroupBadge"),
        QStringLiteral("themeColorPreviewSwatch"),
        QStringLiteral("themeColorWheel"),
        QStringLiteral("themeHueSlider"),
        QStringLiteral("themeHueInputBox"),
        QStringLiteral("themeSaturationSlider"),
        QStringLiteral("themeSaturationInputBox"),
        QStringLiteral("themeLightnessSlider"),
        QStringLiteral("themeLightnessInputBox"),
        QStringLiteral("themeAlphaSlider"),
        QStringLiteral("themeAlphaInputBox"),
        QStringLiteral("themeRgbHexRow"),
        QStringLiteral("themeRedInputBox"),
        QStringLiteral("themeGreenInputBox"),
        QStringLiteral("themeBlueInputBox"),
        QStringLiteral("themeHexInputBox"),
        QStringLiteral("themeFooterDivider"),
        QStringLiteral("themeColorFooter"),
        QStringLiteral("themeResetElementButton"),
        QStringLiteral("themeResetAllButton"),
        QStringLiteral("themeSaveButton"),
        QStringLiteral("themeCloseButton"),
    };
    for (const QString &name : controlNames)
        verifyItem(dialog->findChild<QQuickItem *>(name), name);

    const QStringList optionVisualNames{
        QStringLiteral("themeFontRenderTypeLabels"),
        QStringLiteral("themeFontRenderTypeTitle"),
        QStringLiteral("themeFontRenderTypeDescription"),
        QStringLiteral("themeMouseWheelLabels"),
        QStringLiteral("themeMouseWheelTitle"),
        QStringLiteral("themeMouseWheelDescription"),
    };
    for (const QString &name : optionVisualNames) {
        QQuickItem *const item = dialog->findChild<QQuickItem *>(name);
        QVERIFY2(item, qPrintable(name));
        if (!item)
            continue;
        const QPointF origin = item->mapToItem(dialogRoot, QPointF{});
        verifyWholePhysicalCoordinate(origin.x(), name + QStringLiteral(" x"));
        verifyWholePhysicalCoordinate(origin.y(), name + QStringLiteral(" y"));
    }

    const QStringList optionTextLeafNames{
        QStringLiteral("themeFontRenderTypeTitle"),
        QStringLiteral("themeFontRenderTypeDescription"),
        QStringLiteral("themeMouseWheelTitle"),
        QStringLiteral("themeMouseWheelDescription"),
    };
    for (const QString &name : optionTextLeafNames) {
        QQuickItem *const item = dialog->findChild<QQuickItem *>(name);
        QVERIFY2(item, qPrintable(name));
        if (!item)
            continue;

        const QPointF origin = item->mapToItem(dialogRoot, QPointF{});
        const QPointF xAxis = item->mapToItem(dialogRoot, QPointF(1.0, 0.0))
            - origin;
        const QPointF yAxis = item->mapToItem(dialogRoot, QPointF(0.0, 1.0))
            - origin;
        const QByteArray transformDetails = QStringLiteral(
            "%1 has a non-unit scene transform: x=(%2,%3), y=(%4,%5)")
                                                  .arg(name)
                                                  .arg(xAxis.x(), 0, 'f', 6)
                                                  .arg(xAxis.y(), 0, 'f', 6)
                                                  .arg(yAxis.x(), 0, 'f', 6)
                                                  .arg(yAxis.y(), 0, 'f', 6)
                                                  .toUtf8();
        QVERIFY2(qAbs(xAxis.x() - 1.0) < 0.001
                     && qAbs(xAxis.y()) < 0.001
                     && qAbs(yAxis.x()) < 0.001
                     && qAbs(yAxis.y() - 1.0) < 0.001,
                 transformDetails.constData());
        QCOMPARE(item->property("renderType").toInt(),
                 fixture.window->property("fontRenderType").toInt());
        QVERIFY(!item->property("text").toString().isEmpty());
    }

    QImage renderedDialog;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(renderedDialog = dialog->grabWindow()).isNull(), 3000);
    const qreal renderedScaleX = qreal(renderedDialog.width())
        / dialogRoot->width();
    const qreal renderedScaleY = qreal(renderedDialog.height())
        / dialogRoot->height();
    const QColor captionColor = fixture.window->property("mutedText")
                                    .value<QColor>();
    const QColor captionBackground = fixture.window->property("dialogHeaderBg")
                                         .value<QColor>();
    const auto colorDistanceSquared = [](const QColor &left,
                                         const QColor &right) {
        const int red = left.red() - right.red();
        const int green = left.green() - right.green();
        const int blue = left.blue() - right.blue();
        return red * red + green * green + blue * blue;
    };
    for (const QString &name : {
             QStringLiteral("themeFontRenderTypeDescription"),
             QStringLiteral("themeMouseWheelDescription"),
         }) {
        QQuickItem *const item = dialog->findChild<QQuickItem *>(name);
        QVERIFY2(item, qPrintable(name));
        if (!item)
            continue;
        const QRectF sceneRect = item->mapRectToItem(dialogRoot,
                                                     item->boundingRect());
        const int left = qFloor(sceneRect.left() * renderedScaleX);
        const int top = qFloor(sceneRect.top() * renderedScaleY);
        const int right = qCeil(sceneRect.right() * renderedScaleX);
        const int bottom = qCeil(sceneRect.bottom() * renderedScaleY);
        const QRect pixelRect = QRect(QPoint(left, top),
                                      QPoint(right - 1, bottom - 1))
                                    .intersected(renderedDialog.rect());
        QVERIFY2(pixelRect.isValid(), qPrintable(name));

        int captionLikePixels = 0;
        for (int y = pixelRect.top(); y <= pixelRect.bottom(); ++y) {
            for (int x = pixelRect.left(); x <= pixelRect.right(); ++x) {
                const QColor pixel = renderedDialog.pixelColor(x, y);
                if (colorDistanceSquared(pixel, captionColor)
                    < colorDistanceSquared(pixel, captionBackground)) {
                    ++captionLikePixels;
                }
            }
        }
        const QByteArray renderDetails = QStringLiteral(
            "%1 produced only %2 caption-like pixels in the rendered frame")
                                              .arg(name)
                                              .arg(captionLikePixels)
                                              .toUtf8();
        QVERIFY2(captionLikePixels >= 12, renderDetails.constData());
    }

    const auto verifyLayoutRow = [&](QQuickItem *item, const QString &name) {
        QVERIFY2(item, qPrintable(name));
        if (!item)
            return;
        const QPointF origin = item->mapToItem(dialogRoot, QPointF{});
        verifyWholePhysicalCoordinate(origin.x(), name + QStringLiteral(" x"));
        verifyWholePhysicalCoordinate(origin.y(), name + QStringLiteral(" y"));
        verifyWholePhysicalCoordinate(item->height(),
                                      name + QStringLiteral(" height"));
    };
    const QStringList layoutRowNames{
        QStringLiteral("themeHueRow"),
        QStringLiteral("themeSaturationRow"),
        QStringLiteral("themeLightnessRow"),
        QStringLiteral("themeAlphaRow"),
    };
    for (const QString &name : layoutRowNames)
        verifyLayoutRow(dialog->findChild<QQuickItem *>(name), name);

    QQuickItem *const indicator = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeFontRenderTypeComboIndicator"));
    QVERIFY(indicator);
    const QUrl iconSource = indicator->property("rasterizedIconSource")
                                .toUrl();
    QVERIFY(iconSource.isValid());
    QCOMPARE(iconSource.fileName(), QStringLiteral("chevron-down.svg"));
    QCOMPARE(QUrlQuery(iconSource).queryItemValue(QStringLiteral("size")),
             QStringLiteral("14"));
    QCOMPARE(QUrlQuery(iconSource).queryItemValue(QStringLiteral("dpr")),
             QStringLiteral("1.75"));

    QQuickItem *const wheelIndicator = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeMouseWheelComboIndicator"));
    QVERIFY(wheelIndicator);
    const QUrl wheelIconSource = wheelIndicator->property(
        "rasterizedIconSource").toUrl();
    QVERIFY(wheelIconSource.isValid());
    QCOMPARE(wheelIconSource.fileName(), QStringLiteral("chevron-down.svg"));
    QCOMPARE(QUrlQuery(wheelIconSource).queryItemValue(
                 QStringLiteral("size")), QStringLiteral("14"));
    QCOMPARE(QUrlQuery(wheelIconSource).queryItemValue(
                 QStringLiteral("dpr")), QStringLiteral("1.75"));

    QObject *const popup = fixture.window->findChild<QObject *>(
        QStringLiteral("themeFontRenderTypeCombo"))
                               ->property("popup")
                               .value<QObject *>();
    QVERIFY(popup);
    QVERIFY(QMetaObject::invokeMethod(popup, "open"));
    QTRY_VERIFY_WITH_TIMEOUT(popup->property("visible").toBool(), 1000);

    QQuickItem *const popupBackground = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeFontRenderTypePopupBackground"));
    QQuickItem *const popupList = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeFontRenderTypePopupList"));
    QVERIFY(popupBackground);
    QVERIFY(popupList);
    verifyItem(popupBackground, QStringLiteral("themeFontRenderTypePopupBackground"));
    verifyItem(popupList, QStringLiteral("themeFontRenderTypePopupList"));
    verifyWholePhysicalCoordinate(
        popupList->property("contentHeight").toReal(),
        QStringLiteral("themeFontRenderTypePopupList content height"));

    QVERIFY(QMetaObject::invokeMethod(popup, "close"));
    QTRY_VERIFY_WITH_TIMEOUT(!popup->property("visible").toBool(), 1000);
    dialog->hide();
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
    QTRY_VERIFY_WITH_TIMEOUT(leftHost->property("activeFocus").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!rightHost->property("activeFocus").toBool(),
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
    QQuickItem *const columnHeader = fixture.item(
        QStringLiteral("panelColumnHeader-0"));
    QQuickItem *const leftLoader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QQuickItem *const rightLoader = fixture.item(
        QStringLiteral("galleryPanelContent-1"));
    QVERIFY(leftPanel);
    QVERIFY(rightPanel);
    QVERIFY(pathTitle);
    QVERIFY(columnHeader);
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
    QVERIFY(!columnHeader->isVisible());

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
    projectedPanel.insert(QStringLiteral("galleryColumns"), QVariantList{
        QVariantMap{
            {QStringLiteral("id"), QStringLiteral("name")},
            {QStringLiteral("role"), QStringLiteral("name")},
            {QStringLiteral("title"), QStringLiteral("Name")},
            {QStringLiteral("width"), 50},
        },
        QVariantMap{
            {QStringLiteral("id"), QStringLiteral("size")},
            {QStringLiteral("role"), QStringLiteral("size")},
            {QStringLiteral("title"), QStringLiteral("Size")},
            {QStringLiteral("width"), 14},
        },
    });
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

    // Renderer state and its external header must commit in the same event-loop
    // turn. A deferred renderer update leaves a visible mixed old/new frame.
    QCOMPARE(leftHost->property("appliedPresentationMode").toString(),
             QStringLiteral("details"));
    QVERIFY(columnHeader->isVisible());
    QVERIFY(columnHeader->height() > 0.0);

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
    initialScene.insert(QStringLiteral("menuBar"), QVariantMap{
        {QStringLiteral("selected"), 0},
        {QStringLiteral("active"), false},
    });
    initialScene.insert(QStringLiteral("keyBar"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("modifier"), QStringLiteral("normal")},
    });
    initialScene.insert(QStringLiteral("toast"), QVariantMap{
        {QStringLiteral("visible"), false},
    });
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
    const QVariantMap compactMenuBar = {
        {QStringLiteral("selected"), 2},
        {QStringLiteral("active"), true},
    };
    const QVariantMap compactKeyBar = {
        {QStringLiteral("visible"), true},
        {QStringLiteral("modifier"), QStringLiteral("ctrl-shift")},
    };
    const QVariantMap compactToast = {
        {QStringLiteral("visible"), true},
        {QStringLiteral("text"), QStringLiteral("Compact toast")},
    };
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("panel_chrome")},
        {QStringLiteral("activePanel"), 0},
        {QStringLiteral("workspaceTabs"), compactTabs},
        {QStringLiteral("menuBar"), compactMenuBar},
        {QStringLiteral("keyBar"), compactKeyBar},
        {QStringLiteral("toast"), compactToast},
    });

    QTRY_VERIFY_WITH_TIMEOUT(
        fixture.window->property("workspaceTabs").toMap() == compactTabs,
        3000);
    QCOMPARE(sceneChanged.size(), 0);
    QCOMPARE(fixture.window->property("workspaceTabsOverride").toMap(),
             compactTabs);
    QCOMPARE(fixture.window->property("menuBarModel").toMap(),
             compactMenuBar);
    QCOMPARE(fixture.window->property("keyBarModel").toMap(),
             compactKeyBar);
    QCOMPARE(fixture.window->property("toastModel").toMap(), compactToast);
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
    QVERIFY(fixture.window->property("menuBarOverride").isNull());
    QVERIFY(fixture.window->property("keyBarOverride").isNull());
    QVERIFY(fixture.window->property("toastOverride").isNull());
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);
}

void F4QuickViewSurfaceTests::workspaceSeparatorBreaksUnderActiveTab()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("workspaceTabs"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("activeIndex"), 3},
        {QStringLiteral("tabs"), QVariantList{
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("workspace-tab-1")},
                 {QStringLiteral("text"), QStringLiteral("First")},
                 {QStringLiteral("active"), false},
                 {QStringLiteral("closable"), true},
             },
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("workspace-tab-2")},
                 {QStringLiteral("text"), QStringLiteral("Second")},
                 {QStringLiteral("active"), false},
                 {QStringLiteral("closable"), true},
             },
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("workspace-tab-3")},
                 {QStringLiteral("text"), QStringLiteral("Third")},
                 {QStringLiteral("active"), false},
                 {QStringLiteral("closable"), true},
             },
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("workspace-tab-4")},
                 {QStringLiteral("text"), QStringLiteral("Fourth")},
                 {QStringLiteral("active"), true},
                 {QStringLiteral("closable"), true},
             },
         }},
        {QStringLiteral("newTab"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("workspace-new")},
             {QStringLiteral("visible"), true},
             {QStringLiteral("action"), QStringLiteral("workspace.new")},
         }},
        {QStringLiteral("counter"), QVariantMap{}},
    });

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *const workspaceBar = fixture.item(
        QStringLiteral("workspaceBar"));
    QQuickItem *const leftSeparator = fixture.item(
        QStringLiteral("workspaceSeparatorLeft"));
    QQuickItem *const rightSeparator = fixture.item(
        QStringLiteral("workspaceSeparatorRight"));
    QQuickItem *inactiveDivider = nullptr;
    QQuickItem *rightInactiveDivider = nullptr;
    QVERIFY(workspaceBar);
    QVERIFY(leftSeparator);
    QVERIFY(rightSeparator);

    QTRY_VERIFY_WITH_TIMEOUT(workspaceBar->isVisible(), 3000);
    const auto tabForTitle = [&](const QString &title) {
        QQuickItem *label = visualItemWithText(
            fixture.window->contentItem(), title);
        return label && label->parentItem() && label->parentItem()->parentItem()
            ? label->parentItem()->parentItem() : nullptr;
    };
    const auto tabChildWithObjectName = [](QQuickItem *tab,
                                           const QString &objectName) {
        if (!tab)
            return static_cast<QQuickItem *>(nullptr);
        for (QQuickItem *child : tab->childItems()) {
            if (child->objectName() == objectName)
                return child;
        }
        return static_cast<QQuickItem *>(nullptr);
    };
    QQuickItem *activeTab = nullptr;
    QQuickItem *inactiveTab = nullptr;
    QQuickItem *secondInactiveTab = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (activeTab = tabForTitle(QStringLiteral("Fourth"))) != nullptr, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (inactiveTab = tabForTitle(QStringLiteral("First"))) != nullptr,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (secondInactiveTab = tabForTitle(QStringLiteral("Second"))) != nullptr,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (inactiveDivider = tabChildWithObjectName(
             inactiveTab, QStringLiteral("workspace-tab-1-divider"))) != nullptr,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (rightInactiveDivider = tabChildWithObjectName(
             secondInactiveTab, QStringLiteral("workspace-tab-2-divider")))
            != nullptr,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(activeTab->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(inactiveTab->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(secondInactiveTab->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(leftSeparator->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(rightSeparator->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(inactiveDivider->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(rightInactiveDivider->isVisible(), 3000);

    const QPointF barOrigin = workspaceBar->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPointF leftOrigin = leftSeparator->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPointF rightOrigin = rightSeparator->mapToItem(
        fixture.window->contentItem(), QPointF{});
    QVERIFY(qAbs(leftOrigin.x() + leftSeparator->width() - barOrigin.x())
            < 0.51);
    QVERIFY(qAbs(rightOrigin.x() - barOrigin.x() - workspaceBar->width())
            < 0.51);

    QImage rendered;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(rendered = fixture.window->grabWindow()).isNull(), 3000);
    const qreal scale = rendered.devicePixelRatio();
    const QPointF activeOrigin = activeTab->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const auto renderedColor = [&](qreal x, qreal y) {
        return rendered.pixelColor(
            qBound(0, qFloor(x * scale), rendered.width() - 1),
            qBound(0, qFloor(y * scale), rendered.height() - 1));
    };
    const QColor separator = fixture.window->property(
        "separatorColor").value<QColor>();
    const QPointF inactiveOrigin = inactiveTab->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPointF secondInactiveOrigin = secondInactiveTab->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPointF dividerOrigin = inactiveDivider->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const qreal inactiveTabGap = secondInactiveOrigin.x()
        - inactiveOrigin.x() - inactiveTab->width();
    const qreal dividerCenter = dividerOrigin.x()
        + inactiveDivider->width() / 2;
    const qreal gapCenter = inactiveOrigin.x() + inactiveTab->width()
        + inactiveTabGap / 2;
    QVERIFY2(qAbs(inactiveTabGap - 4) < 0.51,
             "The inactive divider must not change Row spacing");
    QVERIFY2(qAbs(dividerCenter - gapCenter) < 0.51,
             "The inactive divider must be centered in the existing gap");
    const qreal separatorY = activeOrigin.y() + activeTab->height() - 0.5;
    const QColor outsideTab = renderedColor(barOrigin.x() - 2, separatorY);
    const QColor underInactive = renderedColor(
        inactiveOrigin.x() + inactiveTab->width() / 2, separatorY);
    const QColor underActive = renderedColor(
        activeOrigin.x() + activeTab->width() / 2, separatorY);
    const qreal betweenTabsX = inactiveOrigin.x() + inactiveTab->width()
        + inactiveTabGap / 2;
    const QColor betweenTabs = renderedColor(betweenTabsX, separatorY);
    const QColor activeTopCenter = renderedColor(
        activeOrigin.x() + activeTab->width() / 2,
        activeOrigin.y() + 0.5);
    const QColor activeTopLeftCorner = renderedColor(
        activeOrigin.x() + 0.5, activeOrigin.y() + 0.5);
    const QColor activeTopRightCorner = renderedColor(
        activeOrigin.x() + activeTab->width() - 0.5,
        activeOrigin.y() + 0.5);
    QCOMPARE(outsideTab, separator);
    QCOMPARE(underInactive, separator);
    QCOMPARE(betweenTabs, separator);
    QCOMPARE(activeTopCenter, separator);
    QVERIFY2(activeTopLeftCorner != separator,
             "The active tab's upper-left border must be rounded");
    QVERIFY2(activeTopRightCorner != separator,
             "The active tab's upper-right border must be rounded");
    QVERIFY2(underActive != separator,
             "The active tab must not draw a lower separator");

    const QPointF secondCenter = secondInactiveOrigin
        + QPointF(secondInactiveTab->width() / 2,
                  secondInactiveTab->height() / 2);
    QTest::mouseMove(fixture.window, secondCenter.toPoint());
    QTRY_VERIFY_WITH_TIMEOUT(
        secondInactiveTab->property("hoverActive").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!inactiveDivider->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!rightInactiveDivider->isVisible(), 3000);
}

void F4QuickViewSurfaceTests::workspaceTabTextParentsStayOnPhysicalPixelGrid()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("workspaceTabs"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("activeIndex"), 0},
        {QStringLiteral("tabs"), QVariantList{
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("workspace-tab-1")},
                 {QStringLiteral("text"), QStringLiteral("system32 — system32")},
                 {QStringLiteral("number"), 1},
                 {QStringLiteral("surfaceKind"), QStringLiteral("panels")},
                 {QStringLiteral("active"), true},
                 {QStringLiteral("closable"), true},
             },
         }},
        {QStringLiteral("newTab"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("workspace-new")},
             {QStringLiteral("visible"), true},
             {QStringLiteral("action"), QStringLiteral("workspace.new")},
         }},
        {QStringLiteral("counter"), QVariantMap{}},
    });

    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required");
    QQuickItem *const rootItem = fixture.window->contentItem();
    QQuickItem *label = nullptr;
    QQuickItem *title = nullptr;
    QQuickItem *number = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (title = visualItemWithText(
             rootItem, QStringLiteral("system32 — system32"))), 3000);
    label = title->parentItem();
    QVERIFY(label);
    QCOMPARE(label->objectName(),
             QStringLiteral("workspace-tab-label-workspace-tab-1"));
    QTRY_VERIFY_WITH_TIMEOUT(
        (number = visualItemWithText(label, QStringLiteral("1"))), 3000);
    QCOMPARE(title->parentItem(), label);
    QCOMPARE(number->parentItem(), label);

    const auto verifyWholePhysicalCoordinate = [dpr](
            qreal logicalCoordinate, const QString &description) {
        const qreal physicalCoordinate = logicalCoordinate * dpr;
        const QByteArray details = QStringLiteral(
            "%1 is %2 physical pixels at DPR %3")
                                       .arg(description)
                                       .arg(physicalCoordinate, 0, 'f', 6)
                                       .arg(dpr, 0, 'f', 2)
                                       .toUtf8();
        QVERIFY2(qAbs(physicalCoordinate - qRound(physicalCoordinate))
                     < 0.001,
                 details.constData());
    };
    const QPointF labelOrigin = label->mapToItem(rootItem, QPointF{});
    const QPointF titleOrigin = title->mapToItem(rootItem, QPointF{});
    const QPointF numberOrigin = number->mapToItem(rootItem, QPointF{});

    verifyWholePhysicalCoordinate(labelOrigin.x(),
                                  QStringLiteral("workspace label scene x"));
    verifyWholePhysicalCoordinate(labelOrigin.y(),
                                  QStringLiteral("workspace label scene y"));
    verifyWholePhysicalCoordinate(label->height(),
                                  QStringLiteral("workspace label height"));
    verifyWholePhysicalCoordinate(title->y(),
                                  QStringLiteral("workspace title local y"));
    verifyWholePhysicalCoordinate(number->y(),
                                  QStringLiteral("workspace number local y"));
    verifyWholePhysicalCoordinate(titleOrigin.y(),
                                  QStringLiteral("workspace title scene y"));
    verifyWholePhysicalCoordinate(numberOrigin.y(),
                                  QStringLiteral("workspace number scene y"));
}

void F4QuickViewSurfaceTests::chromeIconsUseMatchingPhysicalTargetSizes()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("workspaceTabs"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("activeIndex"), 0},
        {QStringLiteral("tabs"), QVariantList{
             QVariantMap{
                 {QStringLiteral("id"), QStringLiteral("workspace-tab-1")},
                 {QStringLiteral("text"), QStringLiteral("First")},
                 {QStringLiteral("surfaceKind"), QStringLiteral("panels")},
                 {QStringLiteral("active"), true},
                 {QStringLiteral("closable"), true},
             },
         }},
        {QStringLiteral("newTab"), QVariantMap{
             {QStringLiteral("id"), QStringLiteral("workspace-new")},
             {QStringLiteral("visible"), true},
             {QStringLiteral("action"), QStringLiteral("workspace.new")},
         }},
        {QStringLiteral("counter"), QVariantMap{}},
    });

    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (dpr <= 1.0 || qFuzzyCompare(dpr, qRound(dpr)))
        QSKIP("fractional-DPR invocation required");

    QQuickItem *const rootItem = fixture.window->contentItem();
    QQuickItem *const pathControl = fixture.item(
        QStringLiteral("panelPathTitle-0"));
    QQuickItem *const rightPathControl = fixture.item(
        QStringLiteral("panelPathTitle-1"));
    QQuickItem *const driveButton = fixture.item(
        QStringLiteral("panelDriveButton-0"));
    QQuickItem *const rightDriveButton = fixture.item(
        QStringLiteral("panelDriveButton-1"));
    QVERIFY(pathControl);
    QVERIFY(rightPathControl);
    QVERIFY(driveButton);
    QVERIFY(rightDriveButton);

    QQuickItem *const driveIcon = visualItemWithObjectNamePrefix(
        pathControl, QStringLiteral("pathDriveIcon"));
    QQuickItem *const rightDriveIcon = visualItemWithObjectNamePrefix(
        rightPathControl, QStringLiteral("pathDriveIcon"));
    QQuickItem *const driveButtonIcon = visualItemWithObjectNamePrefix(
        driveButton, QStringLiteral("panelDriveButtonIcon-0"));
    QQuickItem *const rightDriveButtonIcon = visualItemWithObjectNamePrefix(
        rightDriveButton, QStringLiteral("panelDriveButtonIcon-1"));
    QVERIFY(driveIcon);
    QVERIFY(rightDriveIcon);
    QVERIFY(driveButtonIcon);
    QVERIFY(rightDriveButtonIcon);
    QVERIFY(!driveIcon->isVisible());
    QVERIFY(!rightDriveIcon->isVisible());
    QTRY_VERIFY_WITH_TIMEOUT(
        pathControl->property("currentDriveIconSource").toUrl().isValid(),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        rightPathControl->property("currentDriveIconSource").toUrl().isValid(),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        driveButtonIcon->property("source").toUrl().isValid(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        rightDriveButtonIcon->property("source").toUrl().isValid(), 3000);
    QCOMPARE(driveButtonIcon->property("source").toUrl(),
             pathControl->property("currentDriveIconSource").toUrl());
    QCOMPARE(rightDriveButtonIcon->property("source").toUrl(),
             rightPathControl->property("currentDriveIconSource").toUrl());
    const auto verifyPhysicalRect = [dpr, rootItem](
                                        QQuickItem *icon,
                                        const QString &description) {
        QVERIFY2(icon, qPrintable(description));
        const QPointF origin = icon->mapToItem(rootItem, QPointF{});
        const QPointF parentOrigin = icon->parentItem()
            ? icon->parentItem()->mapToItem(rootItem, QPointF{})
            : QPointF{};
        const QList<QPair<QString, qreal>> edges{
            {QStringLiteral("left"), origin.x()},
            {QStringLiteral("top"), origin.y()},
            {QStringLiteral("right"), origin.x() + icon->width()},
            {QStringLiteral("bottom"), origin.y() + icon->height()},
        };
        for (const auto &[edgeName, edge] : edges) {
            const qreal physicalEdge = edge * dpr;
            const QByteArray details = QStringLiteral(
                "%1 %2 edge is %3 physical pixels; item=(%4,%5 %6x%7), "
                "parent=(%8,%9 %10x%11) in logical pixels")
                                           .arg(description, edgeName)
                                           .arg(physicalEdge, 0, 'f', 6)
                                           .arg(origin.x(), 0, 'f', 6)
                                           .arg(origin.y(), 0, 'f', 6)
                                           .arg(icon->width(), 0, 'f', 6)
                                           .arg(icon->height(), 0, 'f', 6)
                                           .arg(parentOrigin.x(), 0, 'f', 6)
                                           .arg(parentOrigin.y(), 0, 'f', 6)
                                           .arg(icon->parentItem()
                                                    ? icon->parentItem()->width()
                                                    : 0.0,
                                                0, 'f', 6)
                                           .arg(icon->parentItem()
                                                    ? icon->parentItem()->height()
                                                    : 0.0,
                                                0, 'f', 6)
                                           .toUtf8();
            QVERIFY2(qAbs(physicalEdge - qRound(physicalEdge)) < 0.001,
                     details.constData());
        }
    };
    const auto verifyIcon = [dpr, &verifyPhysicalRect](
                                QQuickItem *icon,
                                const QString &description) {
        QVERIFY2(icon, qPrintable(description));
        verifyPhysicalRect(icon, description);
        const QUrl source = icon->property("source").toUrl();
        QVERIFY2(source.isValid(), qPrintable(description));
        const QUrlQuery query(source);
        bool logicalOk = false;
        bool sourceDprOk = false;
        const int logicalSize = query.queryItemValue(
            QStringLiteral("size")).toInt(&logicalOk);
        const qreal sourceDpr = query.queryItemValue(
            QStringLiteral("dpr")).toDouble(&sourceDprOk);
        QVERIFY2(logicalOk && logicalSize > 0,
                 qPrintable(source.toString()));
        QVERIFY2(sourceDprOk && sourceDpr > 0,
                 qPrintable(source.toString()));
        QVERIFY(qAbs(sourceDpr - dpr) < 0.001);
        const QColor requestedTint(query.queryItemValue(
            QStringLiteral("color")));
        QVERIFY2(requestedTint.isValid(), qPrintable(source.toString()));

        const qreal physicalWidth = icon->width() * dpr;
        const qreal physicalHeight = icon->height() * dpr;
        QVERIFY2(qAbs(physicalWidth - qRound(physicalWidth)) < 0.001,
                 qPrintable(description));
        QVERIFY2(qAbs(physicalHeight - qRound(physicalHeight)) < 0.001,
                 qPrintable(description));
        QCOMPARE(qRound(physicalWidth), qRound(logicalSize * sourceDpr));
        QCOMPARE(qRound(physicalHeight), qRound(logicalSize * sourceDpr));
    };

    const auto verifyDirectSvgIcon = [dpr, &verifyPhysicalRect](
                                         QQuickItem *icon,
                                         const QString &description) {
        QVERIFY2(icon, qPrintable(description));
        verifyPhysicalRect(icon, description);
        const QUrl source = icon->property("source").toUrl();
        QVERIFY2(source.isValid() && source.path().endsWith(
                     QStringLiteral(".svg")),
                 qPrintable(description));
        QTRY_COMPARE_WITH_TIMEOUT(icon->property("status").toInt(), 1, 3000);

        const qreal physicalWidth = icon->width() * dpr;
        const qreal physicalHeight = icon->height() * dpr;
        const QSize sourceSize = icon->property("sourceSize").toSize();
        QVERIFY2(sourceSize.isValid(), qPrintable(description));
        QCOMPARE(qRound(sourceSize.width() * dpr), qRound(physicalWidth));
        QCOMPARE(qRound(sourceSize.height() * dpr), qRound(physicalHeight));
        QCOMPARE(qRound(icon->implicitWidth() * dpr), qRound(physicalWidth));
        QCOMPARE(qRound(icon->implicitHeight() * dpr), qRound(physicalHeight));
    };

    QQuickItem *workspaceIcon = nullptr;
    QQuickItem *workspaceClose = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (workspaceIcon = visualItemWithObjectNamePrefix(
             rootItem, QStringLiteral("workspace-tab-icon-"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (workspaceClose = visualItemWithObjectNamePrefix(
             rootItem, QStringLiteral("workspace-close-"))), 3000);

    QQuickItem *const appButton = fixture.item(
        QStringLiteral("appIconButton"));
    QQuickItem *const minimizeButton = fixture.item(
        QStringLiteral("minimizeButton"));
    QQuickItem *const maximizeButton = fixture.item(
        QStringLiteral("maximizeButton"));
    QQuickItem *const closeButton = fixture.item(
        QStringLiteral("closeButton"));
    QQuickItem *const titleBar = fixture.item(
        QStringLiteral("titleBar"));
    QVERIFY(appButton);
    QVERIFY(minimizeButton);
    QVERIFY(maximizeButton);
    QVERIFY(closeButton);
    QVERIFY(titleBar);
    QQuickItem *appIcon = nullptr;
    QQuickItem *minimizeIcon = nullptr;
    QQuickItem *maximizeIcon = nullptr;
    QQuickItem *closeIcon = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (appIcon = visualItemWithSource(
             appButton,
             QUrl(QStringLiteral("qrc:/F4QtHost/icons/app/f4.svg")))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (minimizeIcon = visualItemWithObjectNamePrefix(
             minimizeButton, QStringLiteral("titleBarButtonIcon"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (maximizeIcon = visualItemWithObjectNamePrefix(
             maximizeButton, QStringLiteral("titleBarButtonIcon"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (closeIcon = visualItemWithObjectNamePrefix(
             closeButton, QStringLiteral("titleBarButtonIcon"))), 3000);
    verifyDirectSvgIcon(appIcon, QStringLiteral("application icon"));
    verifyDirectSvgIcon(minimizeIcon, QStringLiteral("minimize icon"));
    verifyDirectSvgIcon(maximizeIcon, QStringLiteral("maximize icon"));
    verifyDirectSvgIcon(closeIcon, QStringLiteral("close icon"));

    const QPointF titleBarOrigin = titleBar->mapToItem(rootItem, QPointF{});
    const qreal titleBarCenterY = titleBarOrigin.y() + titleBar->height() / 2;
    const auto verifyWindowControlCenter = [
            dpr, rootItem, titleBarCenterY](QQuickItem *icon,
                                            const QString &description) {
        QVERIFY2(icon, qPrintable(description));
        const QPointF origin = icon->mapToItem(rootItem, QPointF{});
        const qreal iconCenterY = origin.y() + icon->height() / 2;
        const qreal delta = qAbs(iconCenterY - titleBarCenterY) * dpr;
        QVERIFY2(delta <= 0.51,
                 qPrintable(QStringLiteral("%1 center is %2 physical px "
                                           "from title bar center")
                                .arg(description)
                                .arg(delta, 0, 'f', 6)));
    };
    verifyWindowControlCenter(minimizeIcon,
                              QStringLiteral("minimize icon"));
    verifyWindowControlCenter(maximizeIcon,
                              QStringLiteral("maximize icon"));
    verifyWindowControlCenter(closeIcon, QStringLiteral("close icon"));

    QTest::mouseMove(fixture.window, QPoint(450, 300));
    minimizeButton->setOpacity(1.0);
    maximizeButton->setOpacity(1.0);
    closeButton->setOpacity(1.0);
    fixture.window->requestUpdate();
    QImage chromeFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(chromeFrame = fixture.window->grabWindow()).isNull(), 3000);
    QCOMPARE(chromeFrame.size(),
             QSize(qRound(fixture.window->width() * dpr),
                   qRound(fixture.window->height() * dpr)));
    const QColor chromeBackground = fixture.window->property(
        "windowBackgroundColor").value<QColor>();
    const auto verifyDirectSvgPixels = [dpr, rootItem, &chromeFrame,
                                        chromeBackground](
                                           QQuickItem *icon,
                                           const QString &description) {
        const QPointF origin = icon->mapToItem(rootItem, QPointF{});
        const QSize physicalSize(qRound(icon->width() * dpr),
                                 qRound(icon->height() * dpr));
        const QRect physicalRect(
            QPoint(qRound(origin.x() * dpr), qRound(origin.y() * dpr)),
            physicalSize);
        QVERIFY2(chromeFrame.rect().contains(physicalRect),
                 qPrintable(description));
        const QVariant tintValue = icon->property("color");
        const QColor tint = tintValue.isValid()
            ? tintValue.value<QColor>() : QColor{};
        const QImage expected = renderSvgReference(
            icon->property("source").toUrl(), physicalSize, tint,
            chromeBackground);
        QVERIFY2(!expected.isNull(), qPrintable(description));
        const QString difference = exactImageDifference(
            chromeFrame.copy(physicalRect), expected);
        QVERIFY2(difference.isEmpty(),
                 qPrintable(description + QStringLiteral(": ")
                            + difference));
    };
    verifyDirectSvgPixels(appIcon, QStringLiteral("application icon"));
    verifyDirectSvgPixels(minimizeIcon, QStringLiteral("minimize icon"));
    verifyDirectSvgPixels(maximizeIcon, QStringLiteral("maximize icon"));
    verifyDirectSvgPixels(closeIcon, QStringLiteral("close icon"));

    QQuickItem *const pathSeparator = visualItemWithObjectNamePrefix(
        pathControl, QStringLiteral("pathBreadcrumbRoot-separator"));
    QQuickItem *const rightPathSeparator = visualItemWithObjectNamePrefix(
        rightPathControl, QStringLiteral("pathBreadcrumbRoot-separator"));
    const QList<QPair<QString, QQuickItem *>> alwaysPresentIcons{
        {QStringLiteral("workspace tab icon"), workspaceIcon},
        {QStringLiteral("workspace close icon"), workspaceClose},
        {QStringLiteral("path drive button icon"), driveButtonIcon},
        {QStringLiteral("path separator icon"), pathSeparator},
        {QStringLiteral("right path drive button icon"), rightDriveButtonIcon},
        {QStringLiteral("right path separator icon"), rightPathSeparator},
        {QStringLiteral("sort chevron"),
         fixture.item(QStringLiteral("panelSortChevron-0"))},
        {QStringLiteral("right sort chevron"),
         fixture.item(QStringLiteral("panelSortChevron-1"))},
        {QStringLiteral("renderer mode icon"),
         fixture.item(QStringLiteral("panelRendererButtonIcon-0"))},
        {QStringLiteral("right renderer mode icon"),
         fixture.item(QStringLiteral("panelRendererButtonIcon-1"))},
        {QStringLiteral("renderer chevron"),
         fixture.item(QStringLiteral("panelRendererButtonChevron-0"))},
        {QStringLiteral("right renderer chevron"),
         fixture.item(QStringLiteral("panelRendererButtonChevron-1"))},
    };
    for (const auto &[description, icon] : alwaysPresentIcons)
        verifyIcon(icon, description);

    QCOMPARE(QUrlQuery(pathSeparator->property("source").toUrl())
                 .queryItemValue(QStringLiteral("size")),
             QStringLiteral("12"));
    QCOMPARE(qRound(pathSeparator->width() * dpr), qRound(12.0 * dpr));
    QCOMPARE(qRound(rightPathSeparator->width() * dpr),
             qRound(12.0 * dpr));

    QQuickItem *const sortButton = fixture.item(
        QStringLiteral("panelSortButton-0"));
    QQuickItem *const sortContent = fixture.item(
        QStringLiteral("panelSortButtonContent-0"));
    QQuickItem *const sortLabel = fixture.item(
        QStringLiteral("panelSortLabel-0"));
    QVERIFY(sortButton);
    QVERIFY(sortContent);
    QVERIFY(sortLabel);
    const QPointF sortButtonOrigin = sortButton->mapToItem(rootItem, QPointF{});
    const QPointF sortContentOrigin = sortContent->mapToItem(rootItem, QPointF{});
    const QPointF sortLabelOrigin = sortLabel->mapToItem(rootItem, QPointF{});
    const auto verifyPhysicalY = [dpr](qreal logicalY,
                                       const QString &description) {
        const qreal physicalY = logicalY * dpr;
        QVERIFY2(qAbs(physicalY - qRound(physicalY)) < 0.001,
                 qPrintable(QStringLiteral("%1: %2 physical px")
                                .arg(description).arg(physicalY, 0, 'f', 6)));
    };
    verifyPhysicalY(sortButtonOrigin.y(), QStringLiteral("sort button y"));
    verifyPhysicalY(sortContentOrigin.y(), QStringLiteral("sort content y"));
    verifyPhysicalY(sortLabelOrigin.y(), QStringLiteral("sort label y"));
    const qreal contentCenterDelta = qAbs(
        sortContentOrigin.y() + sortContent->height() / 2.0
        - sortButtonOrigin.y() - sortButton->height() / 2.0) * dpr;
    const qreal labelCenterDelta = qAbs(
        sortLabelOrigin.y() + sortLabel->height() / 2.0
        - sortContentOrigin.y() - sortContent->height() / 2.0) * dpr;
    QVERIFY2(contentCenterDelta <= 0.51,
             qPrintable(QStringLiteral("sort content center delta: %1 px")
                            .arg(contentCenterDelta, 0, 'f', 6)));
    QVERIFY2(labelCenterDelta <= 0.51,
             qPrintable(QStringLiteral("sort label center delta: %1 px")
                            .arg(labelCenterDelta, 0, 'f', 6)));

    QObject *const sortMenu = fixture.window->findChild<QObject *>(
        QStringLiteral("panelSortMenu-0"));
    QVERIFY(sortMenu);
    QVERIFY(QMetaObject::invokeMethod(sortMenu, "open"));
    QQuickItem *sortCheck = nullptr;
    QQuickItem *sortChoiceIcon = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortCheck = visualItemWithObjectNamePrefix(
             rootItem, QStringLiteral("panelSortChoiceCheck-"))), 3000);
    verifyIcon(sortCheck, QStringLiteral("sort dropdown check"));
    QTRY_VERIFY_WITH_TIMEOUT(
        (sortChoiceIcon = visualItemWithObjectNamePrefix(
             rootItem, QStringLiteral("panelSortChoiceIcon-"))), 3000);
    verifyIcon(sortChoiceIcon, QStringLiteral("sort dropdown icon"));
    QVERIFY(QMetaObject::invokeMethod(sortMenu, "close"));

    QObject *const rendererMenu = fixture.window->findChild<QObject *>(
        QStringLiteral("panelRendererMenu-0"));
    QVERIFY(rendererMenu);
    QVERIFY(QMetaObject::invokeMethod(rendererMenu, "open"));
    QQuickItem *rendererCheck = nullptr;
    QQuickItem *rendererChoiceIcon = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (rendererCheck = visualItemWithObjectNamePrefix(
             rootItem, QStringLiteral("panelRendererChoiceCheck-"))), 3000);
    verifyIcon(rendererCheck, QStringLiteral("renderer dropdown check"));
    QTRY_VERIFY_WITH_TIMEOUT(
        (rendererChoiceIcon = visualItemWithObjectNamePrefix(
             rootItem, QStringLiteral("panelRendererChoiceIcon-"))), 3000);
    verifyIcon(rendererChoiceIcon, QStringLiteral("renderer dropdown icon"));
    QVERIFY(QMetaObject::invokeMethod(rendererMenu, "close"));
}

void F4QuickViewSurfaceTests::panelDriveButtonUsesPathIconAndRequestsDriveMenu()
{
    QuickViewFixture fixture(shellScene({}, 0), true, true);
    QVERIFY(fixture.window);

    auto *const pathControl = fixture.item(
        QStringLiteral("panelPathTitle-0"));
    auto *const driveButton = fixture.item(
        QStringLiteral("panelDriveButton-0"));
    QVERIFY(pathControl);
    QVERIFY(driveButton);

    auto *const embeddedIcon = visualItemWithObjectNamePrefix(
        pathControl, QStringLiteral("pathDriveIcon"));
    auto *const buttonIcon = visualItemWithObjectNamePrefix(
        driveButton, QStringLiteral("panelDriveButtonIcon-0"));
    QVERIFY(embeddedIcon);
    QVERIFY(buttonIcon);
    QVERIFY(!embeddedIcon->isVisible());

    QTRY_VERIFY_WITH_TIMEOUT(
        pathControl->property("currentDriveIconSource").toUrl().isValid(),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        buttonIcon->property("source").toUrl().isValid(), 3000);
    QCOMPARE(buttonIcon->property("source").toUrl(),
             pathControl->property("currentDriveIconSource").toUrl());

    const qreal dpr = fixture.window->devicePixelRatio();
    const QPointF buttonOrigin = buttonIcon->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const qreal physicalWidth = buttonIcon->width() * dpr;
    const qreal physicalHeight = buttonIcon->height() * dpr;
    QVERIFY(qAbs(buttonOrigin.x() * dpr
                 - qRound(buttonOrigin.x() * dpr)) < 0.001);
    QVERIFY(qAbs(buttonOrigin.y() * dpr
                 - qRound(buttonOrigin.y() * dpr)) < 0.001);
    QVERIFY(qAbs(physicalWidth - qRound(physicalWidth)) < 0.001);
    QVERIFY(qAbs(physicalHeight - qRound(physicalHeight)) < 0.001);

    fixture.shell.clearActions();
    QTest::mouseClick(
        fixture.window, Qt::LeftButton, Qt::NoModifier,
        driveButton->mapToScene(QPointF(driveButton->width() / 2,
                                         driveButton->height() / 2)).toPoint());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    const QVariantMap action = fixture.shell.actions.constFirst();
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.driveMenu"));
    QCOMPARE(action.value(QStringLiteral("side")).toInt(), 0);
}

void F4QuickViewSurfaceTests::driveMenuIconsUseSemanticModelAndLiveTheme()
{
    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"), QVariantList{QVariantMap{
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("x"), 5},
        {QStringLiteral("y"), 4},
        {QStringLiteral("w"), 30},
        {QStringLiteral("h"), 6},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("viewHeight"), 3},
        {QStringLiteral("items"), QVariantList{
             QVariantMap{
                 {QStringLiteral("index"), 0},
                 {QStringLiteral("text"), QStringLiteral("Other panel")},
                 {QStringLiteral("icon"), QStringLiteral("panels-top-left")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("disabled"), false},
                 {QStringLiteral("checked"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 1},
                 {QStringLiteral("text"), QStringLiteral("C: Local")},
                 {QStringLiteral("icon"), QStringLiteral("hard-drive")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("disabled"), true},
                 {QStringLiteral("checked"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 2},
                 {QStringLiteral("text"), QStringLiteral("Plain row")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("disabled"), false},
                 {QStringLiteral("checked"), false},
             },
         }},
    }});

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *popup = nullptr;
    QQuickItem *selectedRow = nullptr;
    QQuickItem *normalIcon = nullptr;
    QQuickItem *disabledIcon = nullptr;
    QQuickItem *iconRowText = nullptr;
    QQuickItem *plainRowText = nullptr;
    QQuickItem *const visualRoot = fixture.window->contentItem();
    QTRY_VERIFY_WITH_TIMEOUT(
        (popup = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuPopup-drive-menu"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (selectedRow = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItem-drive-menu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (normalIcon = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemIcon-drive-menu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (disabledIcon = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemIcon-drive-menu-1"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (iconRowText = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemText-drive-menu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (plainRowText = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemText-drive-menu-2"))), 3000);

    QTRY_VERIFY_WITH_TIMEOUT(normalIcon->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(disabledIcon->isVisible(), 3000);
    QCOMPARE(normalIcon->property("semanticIconName").toString(),
             QStringLiteral("panels-top-left"));
    QCOMPARE(disabledIcon->property("semanticIconName").toString(),
             QStringLiteral("hard-drive"));
    QVERIFY(normalIcon->property("semanticIconSource").toUrl().isValid());
    QCOMPARE(iconRowText->x(), plainRowText->x());

    const QColor themedText(QStringLiteral("#ff31c48d"));
    const QColor themedMuted(QStringLiteral("#ff8a5cf5"));
    const QColor themedSelected(QStringLiteral("#ff9c3d26"));
    fixture.window->setProperty("textColor", themedText);
    fixture.window->setProperty("mutedText", themedMuted);
    fixture.window->setProperty("selectedBg", themedSelected);
    QTRY_COMPARE_WITH_TIMEOUT(
        normalIcon->property("semanticIconColor").value<QColor>(),
        themedText, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        disabledIcon->property("semanticIconColor").value<QColor>(),
        themedMuted, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(selectedRow->property("color").value<QColor>(),
                              themedSelected, 3000);

    const qreal dpr = fixture.window->devicePixelRatio();
    const QPointF iconOrigin = normalIcon->mapToItem(
        fixture.window->contentItem(), QPointF{});
    QVERIFY(qAbs(iconOrigin.x() * dpr - qRound(iconOrigin.x() * dpr)) < 0.001);
    QVERIFY(qAbs(iconOrigin.y() * dpr - qRound(iconOrigin.y() * dpr)) < 0.001);
    QVERIFY(qAbs(normalIcon->width() * dpr
                 - qRound(normalIcon->width() * dpr)) < 0.001);
    QVERIFY(qAbs(normalIcon->height() * dpr
                 - qRound(normalIcon->height() * dpr)) < 0.001);

    fixture.window->requestUpdate();
    QImage rendered;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(rendered = fixture.window->grabWindow()).isNull(), 3000);
    QVERIFY(imageContainsColor(rendered, themedSelected));
}

void F4QuickViewSurfaceTests::compactMenuStructureTransfersFocusWithoutSceneRebind()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);

    auto *const grid = fixture.item<TestGrid>(QStringLiteral("vtuiGrid"));
    auto *const leftLoader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QVERIFY(grid);
    QVERIFY(leftLoader);
    QTRY_VERIFY_WITH_TIMEOUT(leftLoader->property("item").value<QObject *>(),
                             3000);
    QObject *const leftHost = leftLoader->property("item").value<QObject *>();
    QTRY_VERIFY_WITH_TIMEOUT(leftHost->property("activeFocus").toBool(), 3000);

    const QVariantMap menu = {
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("x"), 5},
        {QStringLiteral("y"), 4},
        {QStringLiteral("w"), 30},
        {QStringLiteral("h"), 6},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("viewHeight"), 3},
        {QStringLiteral("items"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 0},
             {QStringLiteral("text"), QStringLiteral("C: Local")},
             {QStringLiteral("icon"), QStringLiteral("hard-drive")},
             {QStringLiteral("separator"), false},
             {QStringLiteral("disabled"), false},
         }}},
    };
    QSignalSpy sceneChanged(&fixture.shell, &TestShell::sceneChanged);

    // This mirrors a validated compact scene_patch that changes only menus.
    // Keyboard ownership must move before the signal delivery returns, so the
    // first Down cannot reach the still-loaded Gallery underneath the popup.
    fixture.shell.setCommandMenus(QVariantList{menu});
    QVERIFY(grid->hasActiveFocus());
    QVERIFY(!leftHost->property("activeFocus").toBool());
    QCOMPARE(sceneChanged.size(), 0);

    QQuickItem *popup = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (popup = visualItemWithObjectNamePrefix(
             fixture.window->contentItem(),
             QStringLiteral("semanticMenuPopup-drive-menu"))), 3000);
    QCOMPARE(sceneChanged.size(), 0);

    // Up/Down uses the state-only signal and must neither rebind the scene nor
    // churn focus after the popup has taken keyboard ownership.
    fixture.shell.deliverCommandMenuStates(QVariantList{QVariantMap{
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("top"), 0},
    }});
    QVERIFY(grid->hasActiveFocus());
    QCOMPARE(sceneChanged.size(), 0);

    fixture.shell.setCommandMenus({});
    QVERIFY(leftHost->property("activeFocus").toBool());
    QVERIFY(!grid->hasActiveFocus());
    QCOMPARE(sceneChanged.size(), 0);
}

void F4QuickViewSurfaceTests::menuKeyboardSelectionSurvivesStationaryPointerPatch()
{
    QVariantMap scene = shellScene({}, 0);
    const auto menuWithSelection = [](int selected) {
        QVariantList items;
        for (int index = 0; index < 30; ++index) {
            items.push_back(QVariantMap{
                {QStringLiteral("index"), index},
                {QStringLiteral("text"),
                 QStringLiteral("Drive row %1").arg(index)},
                {QStringLiteral("separator"), false},
                {QStringLiteral("disabled"), false},
            });
        }
        return QVariantMap{
            {QStringLiteral("id"), QStringLiteral("drive-menu")},
            {QStringLiteral("kind"), QStringLiteral("menu")},
            {QStringLiteral("role"), QStringLiteral("vmenu")},
            {QStringLiteral("x"), 5},
            {QStringLiteral("y"), 4},
            {QStringLiteral("w"), 30},
            {QStringLiteral("h"), 6},
            {QStringLiteral("selected"), selected},
            {QStringLiteral("viewHeight"), 3},
            {QStringLiteral("items"), items},
        };
    };
    scene.insert(QStringLiteral("menus"),
                 QVariantList{menuWithSelection(0)});

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *rowZero = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (rowZero = visualItemWithObjectNamePrefix(
             fixture.window->contentItem(),
             QStringLiteral("semanticMenuItem-drive-menu-0"))), 3000);

    const QPoint pointer = rowZero->mapToScene(
        QPointF(rowZero->width() / 2, rowZero->height() / 2)).toPoint();
    // The first local event establishes a stable window-coordinate baseline.
    // A real second move owns the menu selection.
    QTest::mouseMove(fixture.window, pointer - QPoint(2, 0));
    QTest::mouseMove(fixture.window, pointer);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("menu.select"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("index")).toInt(), 0);

    fixture.shell.clearActions();
    fixture.shell.deliverCommandMenuStates(QVariantList{QVariantMap{
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("selected"), 20},
        {QStringLiteral("top"), 17},
    }});

    QQuickItem *rowTwenty = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (rowTwenty = visualItemWithObjectNamePrefix(
             fixture.window->contentItem(),
             QStringLiteral("semanticMenuItem-drive-menu-20"))), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        rowTwenty->property("color").value<QColor>(),
        fixture.window->property("selectedBg").value<QColor>(), 3000);
    QCOMPARE(visualItemWithObjectNamePrefix(
                 fixture.window->contentItem(),
                 QStringLiteral("semanticMenuItem-drive-menu-0")),
             rowZero);
    QTest::qWait(100);
    QCOMPARE(fixture.shell.actions.size(), 0);
}

void F4QuickViewSurfaceTests::pathBreadcrumbTextStaysFixedWhenNavigatingDeeper()
{
    const QFont previousFont = QGuiApplication::font();
    const auto restoreFont = qScopeGuard([previousFont]() {
        QGuiApplication::setFont(previousFont);
    });
    QFont appFont(QStringLiteral("Consolas"));
    appFont.setPixelSize(18);
    QGuiApplication::setFont(appFont);

    const auto sceneWithPath = [](const QString &path) {
        QVariantMap scene = shellScene({}, 0);
        QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
        QVariantList panels = shell.value(QStringLiteral("panels")).toList();
        QVariantMap leftPanel = panels.at(0).toMap();
        leftPanel.insert(QStringLiteral("path"), path);
        leftPanel.insert(QStringLiteral("title"), path);
        panels[0] = leftPanel;
        shell.insert(QStringLiteral("panels"), panels);
        scene.insert(QStringLiteral("shell"), shell);
        return scene;
    };

    QuickViewFixture fixture(sceneWithPath(
                                 QStringLiteral("C:\\WINDOWS\\system32")),
                             true, true);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001 && qAbs(dpr - 2.0) >= 0.001)
        QSKIP("175% or 200% scale invocation required");
    fixture.window->resize(1800, 640);
    QCoreApplication::processEvents();
    QTest::qWait(50);

    QQuickItem *const rootItem = fixture.window->contentItem();
    QQuickItem *const pathControl = fixture.item(
        QStringLiteral("panelPathTitle-0"));
    QVERIFY(rootItem);
    QVERIFY(pathControl);

    QQuickItem *system32Before = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (system32Before = visualItemWithObjectNamePrefix(
             pathControl,
             QStringLiteral("pathBreadcrumb-1-text"))) != nullptr,
        3000);
    QTest::qWait(50);
    const QPointF beforeOrigin = system32Before->mapToItem(
        rootItem, QPointF{});
    const auto verifyPhysicalOrigin = [dpr](const QPointF &origin,
                                             const QString &state) {
        const qreal physicalX = origin.x() * dpr;
        const qreal physicalY = origin.y() * dpr;
        const QString details = QStringLiteral(
            "%1 system32 text origin is (%2, %3) physical px")
                                    .arg(state)
                                    .arg(physicalX, 0, 'f', 6)
                                    .arg(physicalY, 0, 'f', 6);
        QVERIFY2(qAbs(physicalX - qRound(physicalX)) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physicalY - qRound(physicalY)) < 0.001,
                 qPrintable(details));
    };
    verifyPhysicalOrigin(beforeOrigin, QStringLiteral("before navigation"));
    QImage frameBefore;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(frameBefore = fixture.window->grabWindow()).isNull(), 3000);
    const auto physicalRect = [rootItem, dpr](QQuickItem *item) {
        const QPointF topLeft = item->mapToItem(rootItem, QPointF{});
        const int left = qFloor(topLeft.x() * dpr);
        const int top = qFloor(topLeft.y() * dpr);
        const int right = qCeil((topLeft.x() + item->width()) * dpr);
        const int bottom = qCeil((topLeft.y() + item->height()) * dpr);
        return QRect(left, top, right - left, bottom - top);
    };
    const QRect beforeRect = physicalRect(system32Before);
    const QImage beforeText = frameBefore.copy(beforeRect);

    fixture.shell.setScene(sceneWithPath(
        QStringLiteral("C:\\WINDOWS\\system32\\az")));
    QTRY_COMPARE_WITH_TIMEOUT(pathControl->property("text").toString(),
                              QStringLiteral("C:\\WINDOWS\\system32\\az"),
                              3000);
    QTest::qWait(100);
    QCoreApplication::processEvents();

    QQuickItem *system32After = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (system32After = visualItemWithObjectNamePrefix(
             pathControl,
             QStringLiteral("pathBreadcrumb-1-text"))) != nullptr,
        3000);
    const QPointF afterOrigin = system32After->mapToItem(
        rootItem, QPointF{});
    verifyPhysicalOrigin(afterOrigin, QStringLiteral("after navigation"));
    const qreal shiftPhysical = (afterOrigin.x() - beforeOrigin.x()) * dpr;
    QVERIFY2(qAbs(shiftPhysical) < 0.001,
             qPrintable(QStringLiteral(
                 "system32 breadcrumb moved by %1 physical px")
                            .arg(shiftPhysical, 0, 'f', 6)));

    QImage frameAfter;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(frameAfter = fixture.window->grabWindow()).isNull(), 3000);
    const QRect afterRect = physicalRect(system32After);
    QCOMPARE(afterRect, beforeRect);
    const QString difference = exactImageDifference(
        frameAfter.copy(afterRect), beforeText);
    QVERIFY2(difference.isEmpty(), qPrintable(difference));

    pathControl->setProperty("editMode", true);
    QCoreApplication::processEvents();
    QTest::qWait(20);
    QQuickItem *const pathField = fixture.item(QStringLiteral("pathField"));
    QQuickItem *const dynamicPart = visualItemWithObjectNamePrefix(
        pathControl, QStringLiteral("pathDynamicPart"));
    QVERIFY(pathField);
    QVERIFY(dynamicPart);
    QCOMPARE(pathField->property("font").value<QFont>(),
             system32After->property("font").value<QFont>());
    QVERIFY(pathField->property("visible").toBool());
    QVERIFY(!dynamicPart->property("visible").toBool());
    QVERIFY(!system32After->isVisible());
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
    auto *commandLineView = fixture.item(QStringLiteral("commandLineView"));
    auto *promptItem = fixture.item(QStringLiteral("commandLinePrompt"));
    auto *inputItem = fixture.item(QStringLiteral("commandLineInput"));
    auto *cursor = fixture.item(QStringLiteral("commandLineCursor"));
    QVERIFY(presentation);
    QVERIFY(commandLineView);
    QVERIFY(promptItem);
    QVERIFY(inputItem);
    QVERIFY(cursor);
    QCOMPARE(fixture.window->property("commandLineBg").value<QColor>().alpha(),
             0);
    QCOMPARE(commandLineView->property("color").value<QColor>(),
             QColor(Qt::transparent));
    const QColor themedCommandLineBackground(QStringLiteral("#264653"));
    QVERIFY(fixture.window->setProperty("commandLineBg",
                                        themedCommandLineBackground));
    QTRY_COMPARE_WITH_TIMEOUT(
        commandLineView->property("color").value<QColor>(),
        themedCommandLineBackground, 1000);
    QVERIFY(fixture.window->setProperty("commandLineBg",
                                        QColor(Qt::transparent)));
    QTRY_COMPARE_WITH_TIMEOUT(
        commandLineView->property("color").value<QColor>(),
        QColor(Qt::transparent), 1000);
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
