#include <QSGRendererInterface>
#include "PointerRowAnchor.h"
#include "DummyQWK.h"
#include "F4TextRenderingPolicy.h"
#include "TestExtUiStateController.h"
#include <ZoinGallery/GalleryPreferences.h>
#include <ZoinGallery/GalleryRuntime.h>
#include <ZoinGallery/GallerySession.h>

#include <QCoreApplication>
#include <QColor>
#include <QElapsedTimer>
#include <QFile>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QTcpServer>
#include <QTcpSocket>
#include <QTemporaryDir>
#include <QFont>
#include <QFontMetricsF>
#include <QGuiApplication>
#include <QImage>
#include <QMetaProperty>
#include <QPainter>
#include <QPointF>
#include <QPointer>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickTextDocument>
#include <QTextDocument>
#include <QTextBlock>
#include <QTextFragment>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QScopeGuard>
#include <QSettings>
#include <QStyleHints>
#include <QStringList>
#include <QSvgRenderer>
#include <QUrl>
#include <QUrlQuery>
#include <QVariantList>
#include <QVariantMap>
#include <QWheelEvent>
#include <QtQml>
#include <QtTest>

#include <cmath>
#include <algorithm>

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
    Q_INVOKABLE QPoint pointerScreenPosition() const { return QCursor::pos(); }
    Q_INVOKABLE bool pointerEventIsCurrent(QQuickItem *item, qreal x, qreal y) {
        return F4PointerRowAnchor::eventIsCurrent(item, x, y);
    }
    Q_INVOKABLE bool preservePointerRowOffset(QQuickItem *row, qreal previousSceneY) {
        return F4PointerRowAnchor::preserve(window(), row, previousSceneY);
    }

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

class TestShell final : public TestExtUiStateController
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)

public:
    int initialCols() const { return 110; }
    int initialRows() const { return 34; }

    void setScene(const QVariantMap &scene)
    {
        applyScene(scene);
    }
    void setCommandLine(const QVariantMap &commandLine)
    {
        applyCommandLine(commandLine);
    }
    void clearActions()
    {
        actions.clear();
        standaloneViewportActions.clear();
    }
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
        if (menus == overlayState()->commandMenus())
            return;
        applyCommandMenus(menus);
    }

    Q_INVOKABLE void sendUiAction(const QVariantMap &action)
    {
        // Standalone document geometry is negotiated as soon as the persistent
        // document surface has been measured, even while these tests are
        // showing panels or Quick View.  Keep that session protocol traffic
        // observable without mixing it into assertions about the one user
        // interaction each Quick View test sends.
        if (action.value(QStringLiteral("target")).toString()
                    == QStringLiteral("app")
            && action.value(QStringLiteral("action")).toString()
                    == QStringLiteral("document.viewport")
            && action.value(QStringLiteral("scope")).toString()
                    == QStringLiteral("standalone")) {
            standaloneViewportActions.append(action);
            emit uiActionSent(action);
            return;
        }
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
    QVector<QVariantMap> standaloneViewportActions;
    QVector<QVariantMap> keyEvents;

signals:
    void uiActionSent(const QVariantMap &action);

};

class TestGallery final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool available READ available CONSTANT)
    Q_PROPERTY(QObject *settings MEMBER preferences CONSTANT)
    Q_PROPERTY(QObject *viewerSession READ viewerSession NOTIFY viewerChanged)
    Q_PROPERTY(bool viewerVisible READ viewerVisible NOTIFY viewerChanged)
    Q_PROPERTY(bool viewerMounted READ viewerMounted NOTIFY viewerChanged)
    Q_PROPERTY(int viewerState MEMBER presentationState NOTIFY viewerChanged)
    Q_PROPERTY(int quickViewSide MEMBER destinationSide NOTIFY viewerChanged)
    Q_PROPERTY(QVariantMap quickView MEMBER quickView NOTIFY viewerChanged)
    Q_PROPERTY(int viewerSide READ viewerSide NOTIFY viewerChanged)
    Q_PROPERTY(QUrl panelComponentUrl READ panelComponentUrl CONSTANT)
    Q_PROPERTY(QUrl viewerComponentUrl READ viewerComponentUrl NOTIFY viewerChanged)

public:
    explicit TestGallery(bool available = false) : m_available(available) {}
    QObject *preferences = nullptr;

    bool available() const { return m_available; }
    QObject *viewerSession() const { return m_viewerSession; }
    bool viewerVisible() const { return m_viewerUrl.isValid() && presentationState != 1; }
    bool viewerMounted() const { return m_viewerUrl.isValid(); }
    int presentationState = 3;
    int destinationSide = -1;
    QVariantMap quickView;
    Q_INVOKABLE void expandQuickView() { presentationState = 2; emit viewerChanged(); }
    Q_INVOKABLE void collapseQuickView() { presentationState = 4; emit viewerChanged(); }
    Q_INVOKABLE void settleViewer() { presentationState = presentationState == 4 ? 1 : 3; emit viewerChanged(); }
    Q_INVOKABLE void requestActivate(int side) { quickView["active"] = side == destinationSide; emit viewerChanged(); }
    QUrl viewerComponentUrl() const { return m_viewerUrl; }
    void showViewer(const QUrl &url, QObject *session = nullptr)
    {
        m_viewerSession = session;
        m_viewerUrl = url;
        emit viewerChanged();
    }
    int viewerSide() const { return destinationSide < 0 ? 0 : 1 - destinationSide; }
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
    QUrl m_viewerUrl;
    QPointer<QObject> m_viewerSession;
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
        QUrl source(QStringLiteral("qrc:/F4QtHost/icons/%1/%2.svg")
                        .arg(name == "android-logo" || name == "apple-logo" ? "streamline" : "lucide", name));
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

class TestThemePersistence final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QString themeFilePath READ themeFilePath CONSTANT)

public:
    QString themeFilePath() const
    {
        return QStringLiteral("test-gui_theme.ini");
    }

    Q_INVOKABLE QVariantMap loadTheme() const { return m_theme; }
    Q_INVOKABLE bool saveTheme(const QVariantMap &theme)
    {
        m_theme = theme;
        return true;
    }
    Q_INVOKABLE bool resetTheme()
    {
        ++m_resetCalls;
        m_theme.clear();
        return true;
    }

    void setTheme(const QVariantMap &theme) { m_theme = theme; }
    QVariantMap theme() const { return m_theme; }
    int resetCalls() const { return m_resetCalls; }

private:
    QVariantMap m_theme;
    int m_resetCalls = 0;
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

QVariantMap titledUserMenu(const QString &title, bool populated = false)
{
    return {
        {QStringLiteral("id"), QStringLiteral("user-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("title"), title},
        {QStringLiteral("x"), 17},
        {QStringLiteral("y"), 5},
        {QStringLiteral("w"), 50},
        {QStringLiteral("h"), populated ? 4 : 2},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("top"), 0},
        {QStringLiteral("bottomHint"), QStringLiteral(" Del Ins Ctrl+F4 Ctrl+Up/Down ")},
        {QStringLiteral("items"), populated ? QVariantList{
             QVariantMap{{QStringLiteral("index"), 0},
                         {QStringLiteral("text"), QStringLiteral("Build")}},
             QVariantMap{{QStringLiteral("index"), 1},
                         {QStringLiteral("text"), QStringLiteral("Test")},
                         {QStringLiteral("shortcut"), QStringLiteral("F3")}},
         } : QVariantList{}},
    };
}

void sendPixelWheel(QQuickWindow *window, const QPoint &position, int deltaY)
{
    QWheelEvent event(position, window->mapToGlobal(position),
                      QPoint(0, deltaY), {}, Qt::NoButton, Qt::NoModifier,
                      Qt::NoScrollPhase, false);
    QCoreApplication::sendEvent(window, &event);
}

void sendAngleWheel(QQuickWindow *window, const QPoint &position, int deltaY)
{
    QWheelEvent event(position, window->mapToGlobal(position),
                      {}, QPoint(0, deltaY), Qt::NoButton, Qt::NoModifier,
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

QVariantMap keyBarModel(int count, bool alternate = false)
{
    QVariantList items;
    for (int index = 0; index < count; ++index) {
        items.append(QVariantMap{
            {QStringLiteral("key"), QStringLiteral("F%1").arg(index + 1)},
            {QStringLiteral("text"), QStringLiteral("%1 %2")
                .arg(alternate ? QStringLiteral("Edit") : QStringLiteral("Open"))
                .arg(index + 1)},
            {QStringLiteral("icon"), alternate ? QStringLiteral("pencil")
                                               : QStringLiteral("circle-play")},
            {QStringLiteral("alternatives"), QVariantList{}},
        });
    }
    return {{QStringLiteral("visible"), true},
            {QStringLiteral("modifier"), QStringLiteral("normal")},
            {QStringLiteral("items"), items}};
}

QVariantMap qmlObjectProperties(const QVariant &value)
{
    if (QObject *object = value.value<QObject *>())
    {
        QVariantMap result;
        const QMetaObject *metaObject = object->metaObject();
        for (int index = QObject::staticMetaObject.propertyCount();
             index < metaObject->propertyCount(); ++index)
        {
            const QMetaProperty property = metaObject->property(index);
            if (property.isReadable())
                result.insert(QString::fromLatin1(property.name()),
                              property.read(object));
        }
        return result;
    }
    return value.toMap();
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

QQuickItem *visualItemWithObjectName(QQuickItem *root, const QString &name)
{
    if (!root)
        return nullptr;
    if (root->objectName() == name)
        return root;
    for (QQuickItem *child : root->childItems()) {
        if (QQuickItem *match = visualItemWithObjectName(child, name))
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
    TestThemePersistence themePersistence;
    F4TextRenderingPolicy textRenderingPolicy;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;

    explicit QuickViewFixture(const QVariantMap &scene,
                              bool galleryAvailable = false,
                              bool usesQwk = false,
                              QString worktreeBranch = {},
                              QString guiFontFamily = QStringLiteral("Monaco"),
                              QString systemUiFontFamily =
                                  QStringLiteral("Sans Serif"),
                              QString systemMonospaceFontFamily =
                                  QStringLiteral("monospace"))
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
        engine.rootContext()->setContextProperty(QStringLiteral("qtTheme"),
                                                  &themePersistence);
        engine.rootContext()->setContextProperty(
            QStringLiteral("qtTextRendering"), &textRenderingPolicy);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4GuiFontFamily"), guiFontFamily);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4SystemUiFontFamily"),
            systemUiFontFamily);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4SystemMonospaceFontFamily"),
            systemMonospaceFontFamily);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4GuiFontPixelSize"), 13);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4WorktreeBranchName"), worktreeBranch);
        if (!worktreeBranch.isEmpty()) {
            engine.rootContext()->setContextProperty(
                QStringLiteral("f4WorktreeBranch"), worktreeBranch);
        }
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

static void moveNativePointer(QQuickWindow *window, const QPoint &point, int delay = -1)
{
    QCursor::setPos(window->mapToGlobal(point));
    QTest::mouseMove(window, point, delay);
}

class F4QuickViewSurfaceTests final : public QObject
{
    Q_OBJECT

private slots:
    void nativeSettingsPagePreservesConfigurator();
    void initTestCase();
    void qmlImportsWithoutInstalledQt();
    void compiledHostLoadsItsQmlModule();
    void expandedViewerKeepsWorkspaceChromeAccessible();
    void cachedGalleryViewerCentersFirstNativeZoomAt175Percent();
    void quickViewRetainsViewerAcrossPresentationChanges();
    void semanticSceneGatesOnlyGridRendering();
    void semanticHorizontalSplitStaysOnNativeSurface();
    void functionBarShowsExplicitFunctionKeysAndForwardsMouseModifiers();
    void functionBarSameCountUpdatesPreserveDelegates();
    void functionBarLeavesStaySharpAndThemeLiveAt175Percent();
    void readyUnifiedRendererLoaderIsVisible();
    void panelFileInfoSettingKeepsStatusOverlayAndContentGeometry();
    void panelStatusLeavesStayOnPhysicalPixelGrid();
    void panelStatusFitsLongestRowWithoutWrapping();
    void liveSelectionStatusUsesCompactPatches();
    void sortGroupLeavesStayOnPhysicalPixelGrid();
    void fastFindOverlayIsIndependentFromPanelFooter();
    void nativeFontRolesUsePlatformDefaultsAt175Percent();
    void fastFindOverlayAvoidsStatusAndStaysPixelAligned();
    void galleryPanelColorsAreGroupedAndRemainLive();
    void themeConfiguratorExposesOnlyLiveColorProperties();
    void themeColorEditorUsesOklchCoordinates();
    void themeConfiguratorRestoresSavedTheme();
    void themeSelectionBordersAreLiveAndPersisted();
    void themeBooleanOptionsFollowLivePalette();
    void themeDialogFontRenderingControlIsLiveAndThemeAware();
    void themeColorListHoverAndPressFlashHaveExplicitLifetimes();
    void themeDialogControlsStayOnPhysicalPixelGridAt175Percent();
    void userMenuRecordDialogLeavesStaySharpAt175Percent();
    void far3ImportDialogLeavesStaySharpAt175Percent_data();
    void far3ImportDialogLeavesStaySharpAt175Percent();
    void largeHistoryLatencyProfile();
    void historyHeldUpKeepsRowsOnPhysicalPixels();
    void rendererChoicesUseProductOrderAndShortcuts();
    void rendererZoomControlsFollowLayoutCapability();
    void coverUncoverPreservesFilePanelAndRendererObjects();
    void compactActivationPreservesPanelObjectsAndRebindsOnlyFocus();
    void pointerActivationPreviewHandsOffBothPanelCursors();
    void compactCatalogUpdatesOnlyChangedPanelPresentation();
    void compactChromeUpdatesWorkspaceTabsWithoutRebuildingPanels();
    void panelPathBarsToggleWithoutLosingContent();
    void panelExpandButtonsShareHoverAndRestore();
    void panelSplitterCoalescesGoUpdates();
    void workspaceDragHitOnlyAcceptsPanelTabs();
    void workspaceSeparatorBreaksUnderActiveTab();
    void workspaceTabWheelActivatesAdjacentTabs();
    void workspaceTabMiddleClickClosesClickedTab();
    void workspaceCloseButtonHasTightHitAreaAndHoverFeedback();
    void workspaceTabTextParentsStayOnPhysicalPixelGrid();
    void worktreeBranchAppearsCenteredInTitleBar();
    void worktreeBranchIsCenteredInTitleBar();
    void chromeIconsUseMatchingPhysicalTargetSizes();
    void panelDriveButtonUsesPathIconAndRequestsDriveMenu();
    void driveMenuIconsUseSemanticModelAndLiveTheme();
    void standaloneMenuTitleKeepsEmptyMenuAndActionsUsable();
    void attachedMenusDoNotShowStandaloneTitles_data();
    void attachedMenusDoNotShowStandaloneTitles();
    void standaloneMenuTitleLeavesStaySharpAt175Percent();
    void menuScrollBarUsesNativeExtentAndCommitsMouseDrag();
    void historyLastRowFitsNativeViewport_data();
    void historyFirstVisibleFrameHasFinalPosition();
    void historyLastRowFitsNativeViewport();
    void menuBarPopupStartsUnderClickedItem();
    void f9MenuOverlaysPathRowWithoutMovingContent();
    void appIconMenuUsesSemanticCategoriesAndCommands();
    void nestedMenuAnchorsHeadersTagsAndChevronsOnPhysicalPixels();
    void nestedMenuHoverUsesDelayedSubmenuAction();
    void compactMenuStructureTransfersFocusWithoutSceneRebind();
    void menuKeyboardSelectionSurvivesStationaryPointerPatch();
    void pathBreadcrumbTextStaysFixedWhenNavigatingDeeper();
    void uriBreadcrumbKeepsSchemeTogetherAndNavigates();
    void commandMenusKeepPanelCursorWhileBlockingInput();
    void quickSearchPaletteDefaultAndResetArePink();
    void deviceBreadcrumbLabelsPreserveCanonicalNavigationAt175Percent_data();
    void deviceBreadcrumbLabelsPreserveCanonicalNavigationAt175Percent();
    void pluginPathIconFollowsPanelOnPhysicalGrid();
    void embeddedWheelCoalescesAndUsesQuickViewContract();
    void contentKeyChangeDropsOldGestureAndAnchor();
    void clickActivatesCoveredSideAndFocusStaysOutOfHiddenPanel();
    void previewKindsSelectExactlyOneNativeBody();
    void commandLineUsesOriginalSemanticRendererAndCursor();
    void commandLineCursorTracksFirstTextPatch();
    void commandLineAutoHideRevealsUpward();
    void commandLinePanelToggleIsImmediate();
    void commandLineMultilineWrapAndPixelGrid();
    void commandLineFontBaselineCaretAndCompactHeight();
    void commandLineEmptyBaseline();
    void commandLineFontBaselineCaretAndCompactHeight_data();
    void commandLineHeightChangesOncePerEdit();
    void commandLineGraphicalCaretIsOptionalAndPersisted();
    void commandLineClickTransfersFocus();
    void commandLineDropOutlineMatchesPanels();
    void panelCursorBlinkSettlesAndBlockingMenuStopsIt();
    void autocompleteReturnTargetsShellCommandHandler();
    void autocompleteSelectionPreviewsAndRestoresQuery();
    void autocompleteOutsideClickDismissesWithoutChangingText();
    void autocompleteHoverFollowsPopupResize();
    void widePanelDoesNotRevealTerminalBackdrop();
    void shortenedPanelsRevealTerminalRows();
    void semanticTableDialog();
    void terminalScrollBarStaysInsideTheExposedPanelSide();
};

void F4QuickViewSurfaceTests::qmlImportsWithoutInstalledQt()
{
#ifndef QT_STATIC
    QSKIP("Shared Qt builds intentionally load installed QML modules");
#endif
    QQmlEngine engine;
    engine.setImportPathList({QStringLiteral("qrc:/qt-project.org/imports"),
                              QStringLiteral("qrc:/qt/qml")});
    QQmlComponent component(&engine);
    component.setData(R"(
        import QtQuick
        import QtQuick.Controls
        import QtQuick.Layouts
        import QtQuick.Shapes
        import QtQuick.Effects
        Item {
            Connections { target: null }
            Timer { interval: 100 }
            ListModel { ListElement { title: "item" } }
            RowLayout { Button { text: "OK" } ComboBox { model: ["one", "two"] } }
            Shape { }
            MultiEffect { }
        }
    )", QUrl(QStringLiteral("qrc:/portable-import-test.qml")));
    QScopedPointer<QObject> object(component.create());
    QVERIFY2(object, qPrintable(component.errorString()));
}

void F4QuickViewSurfaceTests::compiledHostLoadsItsQmlModule()
{
    const QString hostPath = qEnvironmentVariable("F4_COMPILED_QT_HOST");
    if (hostPath.isEmpty()) QSKIP("Set F4_COMPILED_QT_HOST to validate the production executable");
    QTemporaryDir temporary;
    QVERIFY(temporary.isValid());
    const QString tracePath = temporary.filePath(QStringLiteral("startup.jsonl"));
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost));
    QProcess host;
    auto environment = QProcessEnvironment::systemEnvironment();
    environment.insert(QStringLiteral("QT_QPA_PLATFORM"), QStringLiteral("offscreen"));
    environment.insert(QStringLiteral("QT_QUICK_BACKEND"), QStringLiteral("software"));
    environment.insert(QStringLiteral("F4_NAV_BENCHMARK_TRACE"), QStringLiteral("1"));
    environment.insert(QStringLiteral("F4_NAV_BENCHMARK_QT_OUTPUT"), tracePath);
    environment.remove(QStringLiteral("F4_QT_HOST_STARTUP_SMOKE_ONLY"));
    host.setProcessEnvironment(environment);
    host.setProcessChannelMode(QProcess::MergedChannels);
    const auto stopHost = qScopeGuard([&host] {
        host.kill();
        host.waitForFinished(1000);
    });
    host.start(hostPath, {QStringLiteral("--f4-ext-connect=127.0.0.1:%1").arg(server.serverPort()),
                          QStringLiteral("--f4-ext-nonce=qml-module-test")});
    QVERIFY(host.waitForStarted());
    QTRY_VERIFY_WITH_TIMEOUT(server.hasPendingConnections(), 5000);
    QScopedPointer<QTcpSocket> peer(server.nextPendingConnection());
    QVERIFY(peer);
    QJsonObject loaded;
    const auto readLoadedEvent = [&] {
        QFile trace(tracePath);
        if (!trace.open(QIODevice::ReadOnly)) return false;
        for (const auto &line : trace.readAll().split('\n')) {
            const auto start = line.indexOf('{');
            if (start < 0) continue;
            const auto event = QJsonDocument::fromJson(line.mid(start)).object();
            if (event.value(QStringLiteral("event")).toString() == QStringLiteral("qt.startup.qml.loaded")) {
                loaded = event;
                return true;
            }
        }
        return false;
    };
    QTRY_VERIFY_WITH_TIMEOUT(readLoadedEvent(), 5000);
    QCOMPARE(loaded.value(QStringLiteral("rootObjectCount")).toInt(), 1);
    QCOMPARE(host.state(), QProcess::Running);
}

void F4QuickViewSurfaceTests::nativeSettingsPagePreservesConfigurator()
{
    QTemporaryDir settingsDirectory;
    QVERIFY(settingsDirectory.isValid());
    const auto previousFormat = QSettings::defaultFormat();
    const auto previousOrganization = QCoreApplication::organizationName();
    QSettings::setDefaultFormat(QSettings::IniFormat);
    QSettings::setPath(QSettings::IniFormat, QSettings::UserScope, settingsDirectory.path());
    QCoreApplication::setOrganizationName("F4NativeSettingsTest");
    const auto restoreSettings = qScopeGuard([&] {
        QSettings::setDefaultFormat(previousFormat);
        QCoreApplication::setOrganizationName(previousOrganization);
    });
    auto scene = shellScene();
    scene.insert("dialogs", QVariantList{QVariantMap{{"id","test-settings"}, {"kind","dialog"},
        {"layout","settings"}, {"title","Settings"}, {"x",2}, {"y",2}, {"w",100}, {"h",42}, {"modal",true}, {"showClose",true},
        {"children",QVariantList{QVariantMap{{"id","categories"},{"kind","table"},
            {"layoutRole","navigation"},{"showHeader",false},{"cursor",0},
            {"columns",QVariantList{QVariantMap{{"title","Categories"},{"width",24}}}},
            {"rows",QVariantList{QVariantMap{{"cells",QStringList{"Appearance"}}},
                                QVariantMap{{"cells",QStringList{"Panels"}}}}}},
            QVariantMap{{"id","core-page"},{"kind","text"},
            {"text","Core settings remain unchanged"},{"layoutRole","content"},{"w",50},{"h",1}},
            QVariantMap{{"id","category-title"},{"kind","text"},{"layoutRole","content-title"},{"text","Appearance"}},
            QVariantMap{{"id","settings-apply"},{"kind","button"},{"layoutRole","apply"},{"text","Apply"}},
            QVariantMap{{"id","settings-ok"},{"kind","button"},{"layoutRole","accept"},{"text","OK"}},
            QVariantMap{{"id","settings-cancel"},{"kind","button"},{"layoutRole","cancel"},{"text","Cancel"}}}}}});
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);
    QVERIFY(QMetaObject::invokeMethod(fixture.window,"showApplicationSettings"));
    fixture.shell.setScene(scene);
    fixture.window->resize(1400, 1050);
    QTest::qWait(150);
    QQuickItem *body = nullptr;
    const auto findBody = [&]() {
        return visualItemWithObjectName(fixture.window->contentItem(), "settingsDialogBody");
    };
    QTRY_VERIFY((body = findBody()));
    QTRY_COMPARE(body->property("selectedNativePage").toString(),"gui");
    auto *categories = visualItemWithObjectName(body,"dialogWidget-categoriesTableRows");
    QVERIFY(categories);
    QCOMPARE(categories->property("count").toInt(), 5);
    QCOMPARE(categories->property("currentIndex").toInt(), 2);
    auto *guiLabel = visualItemWithObjectName(body,"dialogWidget-categoriesTableCell-2-0");
    QVERIFY(guiLabel);
    QCOMPARE(guiLabel->property("text").toString(), "GUI");
    QVERIFY(QFile::exists(":/F4QtHost/icons/lucide/app-window.svg"));
    QVERIFY(!visualItemWithObjectName(body,"nativeSettingsPage-gui"));
    auto *pointer = visualItemWithObjectName(body,"dialogWidget-categoriesTablePointer");
    QVERIFY(pointer);
    fixture.shell.clearActions();
    const QPoint guiPoint = guiLabel->mapToScene(QPointF(10, guiLabel->height()/2)).toPoint();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, guiPoint);
    QTRY_COMPARE(body->property("selectedNativePage").toString(),"gui");
    auto *content = visualItemWithObjectName(body,"themeConfiguratorContent");
    QTRY_VERIFY((content = visualItemWithObjectName(body,"themeConfiguratorContent")));
    auto *sharedTitle = visualItemWithObjectName(body,"dialogWidget-category-titleText");
    auto *sharedApply = visualItemWithObjectName(body,"dialogWidget-settings-applyButton");
    auto *sharedOK = visualItemWithObjectName(body,"dialogWidget-settings-okButton");
    auto *sharedCancel = visualItemWithObjectName(body,"dialogWidget-settings-cancelButton");
    QVERIFY(sharedTitle && sharedApply && sharedOK && sharedCancel);
    QVERIFY(sharedTitle->isVisible());
    QCOMPARE(sharedTitle->property("text").toString(), "GUI");
    QVERIFY(sharedApply->isVisible() && sharedOK->isVisible() && sharedCancel->isVisible());
    QVERIFY(content->findChild<QQuickItem *>("themeItemsList"));
    QVERIFY(!content->findChild<QQuickItem *>("themeSaveButton")->isVisible());
    QVERIFY(content->findChild<QQuickItem *>("themeRestoreSavedButton"));
    QVERIFY(content->findChild<QQuickItem *>("themeColorEditor"));
    auto *caretOption = visualItemWithObjectName(content,"themeCommandLineCaretCheckBox");
    QVERIFY(caretOption);
    QVERIFY(caretOption->property("checked").toBool());
    QVERIFY(fixture.shell.actions.isEmpty()); // no GUI values or controls sent to Go
    QVERIFY(QMetaObject::invokeMethod(fixture.window,"showApplicationSettings"));
    QCOMPARE(fixture.shell.actions.last().value("action").toString(),"settings.open");
    QCOMPARE(visualItemWithObjectName(body,"themeConfiguratorContent"),content);
    QTest::qWait(150);
    const qreal dpr=fixture.window->devicePixelRatio();
    int leaves=0;
    const auto inspect=[&](auto &&self,QQuickItem *item)->void {
        if(item->isVisible() && (item->property("renderType").isValid() || item->inherits("QQuickImage"))) {
            ++leaves;
            const auto origin=item->mapToItem(fixture.window->contentItem(),QPointF{});
            const QString detail=QString("%1 %2 physical=(%3,%4)").arg(item->objectName(),item->metaObject()->className())
                .arg(origin.x()*dpr,0,'f',6).arg(origin.y()*dpr,0,'f',6);
            QVERIFY2(!item->objectName().isEmpty(),qPrintable(detail));
            QVERIFY2(qAbs(origin.x()*dpr-qRound(origin.x()*dpr))<0.001,qPrintable(detail));
            QVERIFY2(qAbs(origin.y()*dpr-qRound(origin.y()*dpr))<0.001,qPrintable(detail));
            QCOMPARE(item->mapToItem(fixture.window->contentItem(),QPointF(1,0))-origin,QPointF(1,0));
            QCOMPARE(item->mapToItem(fixture.window->contentItem(),QPointF(0,1))-origin,QPointF(0,1));
        }
        for(auto *child:item->childItems())self(self,child);
    };
    inspect(inspect,content);
    inspect(inspect,categories);
    for (auto *shared : {sharedTitle, sharedApply, sharedOK, sharedCancel}) inspect(inspect, shared);
    QVERIFY(leaves>20);
    ZoinGallery::RuntimeOptions galleryOptions;
    galleryOptions.maxDecodeThreads = 4;
    auto *galleryRuntime = ZoinGallery::GalleryRuntime::install(&fixture.engine, galleryOptions);
    fixture.gallery.preferences = galleryRuntime->preferences();
    body->setProperty("selectedNativePage", "gallery");
    QQuickItem *galleryPage = nullptr;
    QTRY_VERIFY((galleryPage = visualItemWithObjectName(body, "gallerySettingsPage")));
    QTest::qWait(200);
    auto *galleryPreferences = qobject_cast<ZoinGallery::GalleryPreferences *>(fixture.gallery.preferences);
    QVERIFY(galleryPreferences);
    QTRY_VERIFY(!galleryPreferences->busy());
    auto *clearCache = visualItemWithObjectName(galleryPage, "galleryCacheClear");
    auto *clearLabel = visualItemWithObjectName(galleryPage, "galleryCacheClearText");
    QVERIFY(clearCache);
    QVERIFY(clearLabel);
    const QString clearText = clearLabel->property("text").toString();
    const QPointF clearOrigin = clearCache->mapToScene({});
    galleryPreferences->refresh();
    QCOMPARE(clearLabel->property("text").toString(), clearText);
    QVERIFY(clearCache->isEnabled());
    QCOMPARE(clearCache->mapToScene({}), clearOrigin);
    auto *imageMode = visualItemWithObjectName(galleryPage, "galleryImageMode");
    auto *folderMode = visualItemWithObjectName(galleryPage, "galleryFolderMode");
    auto *conversion = visualItemWithObjectName(galleryPage, "galleryColorConversion");
    auto *animation = visualItemWithObjectName(galleryPage, "galleryAnimateResizing");
    QVERIFY(imageMode);
    QVERIFY(folderMode);
    QVERIFY(conversion);
    QVERIFY(animation);
    QCOMPARE(imageMode->property("count").toInt(), 3);
    QCOMPARE(folderMode->property("count").toInt(), 3);
    QVERIFY(conversion->property("checkState").isValid());
    QVERIFY(animation->property("checkState").isValid());
    QVERIFY(visualItemWithObjectName(galleryPage, "galleryCacheLimitInputTextInput"));
    QVERIFY(visualItemWithObjectName(galleryPage, "galleryCacheLocationInputTextInput"));
    for (auto *combo : {imageMode, folderMode}) {
        auto *popup = combo->property("popup").value<QObject *>();
        QVERIFY(popup);
        QVERIFY(QMetaObject::invokeMethod(popup, "open"));
        QTRY_VERIFY(popup->property("visible").toBool());
        const QStringList expectedChoices{"Off", "On", "Cache only"};
        for (int i = 0; i < expectedChoices.size(); ++i) {
            QQuickItem *choice = nullptr;
            const auto name = combo->objectName() + "PopupItemText-" + QString::number(i);
            QTRY_VERIFY((choice = visualItemWithObjectName(fixture.window->contentItem(), name)));
            QCOMPARE(choice->property("text").toString(), expectedChoices[i]);
            inspect(inspect, choice);
        }
        auto *popupContent = popup->property("contentItem").value<QQuickItem *>();
        QVERIFY(popupContent);
        inspect(inspect, popupContent);
        QVERIFY(QMetaObject::invokeMethod(popup, "close"));
    }
    const auto draftValues = [&]() {
        return galleryPage->property("draft").value<QJSValue>().toVariant().toMap();
    };
    imageMode->forceActiveFocus();
    QTest::keyClick(fixture.window, Qt::Key_End);
    QCOMPARE(imageMode->property("currentIndex").toInt(), 2);
    QCOMPARE(draftValues().value("imageMode").toInt(), 2);
    const bool wasChecked = conversion->property("checked").toBool();
    conversion->forceActiveFocus();
    QTest::keyClick(fixture.window, Qt::Key_Space);
    QCOMPARE(conversion->property("checked").toBool(), !wasChecked);
    QCOMPARE(draftValues().value("convertColors").toBool(), !wasChecked);
    QTest::keyClick(fixture.window, Qt::Key_Space);
    auto *limitInput = visualItemWithObjectName(galleryPage, "galleryCacheLimitInputTextInput");
    limitInput->forceActiveFocus();
    QTest::keyClick(fixture.window, Qt::Key_A, Qt::ControlModifier);
    for (auto key : {Qt::Key_2, Qt::Key_0, Qt::Key_4, Qt::Key_8})
        QTest::keyClick(fixture.window, key);
    QCOMPARE(draftValues().value("diskLimitMiB").toInt(), 2048);
    // The next usage update must preserve edits that have not been applied.
    QSignalSpy usageRefreshed(galleryPreferences, &ZoinGallery::GalleryPreferences::changed);
    galleryPreferences->refresh();
    QTRY_VERIFY(!usageRefreshed.isEmpty());
    QCOMPARE(draftValues().value("diskLimitMiB").toInt(), 2048);
    QCOMPARE(clearLabel->property("text").toString(), clearText);
    QVERIFY(clearCache->isEnabled());
    QTest::qWait(150);
    leaves = 0;
    inspect(inspect, galleryPage);
    QVERIFY(leaves > 40);
    QVERIFY(visualItemWithObjectName(galleryPage, "galleryCacheLimitInput"));
    QVERIFY(visualItemWithObjectName(galleryPage, "galleryDecoderFormats-0"));
    const auto galleryCapture = qEnvironmentVariable("F4_GALLERY_SETTINGS_CAPTURE");
    if (!galleryCapture.isEmpty()) QVERIFY(fixture.window->grabWindow().save(galleryCapture));
    auto *galleryViewport = visualItemWithObjectName(body, "nativeSettingsViewport");
    auto *quickHeading = visualItemWithObjectName(galleryPage, "galleryQuickViewTitle");
    auto *builtin = visualItemWithObjectName(galleryPage, "galleryBuiltinQuickView");
    auto *hover = visualItemWithObjectName(galleryPage, "galleryHoverQuickView");
    QVERIFY(quickHeading && builtin && hover);
    QVERIFY(!builtin->property("checked").toBool());
    QVERIFY(hover->property("checked").toBool());
    const auto retainedGalleryDraft = draftValues();
    QQmlComponent quickPreferencesComponent(&fixture.engine);
    quickPreferencesComponent.setData(R"(
        import QtQml
        QtObject {
            property var values: ({useBuiltinF4Viewer: false, previewOnHover: true})
            property string error: ""
            function apply(next) { values = Object.assign({}, next); return true }
        }
    )", QUrl());
    QScopedPointer<QObject> quickPreferences(quickPreferencesComponent.create());
    QVERIFY(quickPreferences);
    galleryPage->setProperty("quickViewPreferences", QVariant::fromValue(quickPreferences.data()));
    QVERIFY(QMetaObject::invokeMethod(galleryPage, "resetDraft"));
    QVERIFY(!galleryPage->property("dirty").toBool());
    const QVariantMap editedQuickView{{"useBuiltinF4Viewer", true}, {"previewOnHover", false}};
    galleryPage->setProperty("quickViewDraft", editedQuickView);
    QVERIFY(galleryPage->property("dirty").toBool());
    QVERIFY(QMetaObject::invokeMethod(galleryPage, "resetDraft"));
    QVERIFY(!galleryPage->property("dirty").toBool());
    QVERIFY(!builtin->property("checked").toBool());
    galleryPage->setProperty("quickViewDraft", editedQuickView);
    QVERIFY(QMetaObject::invokeMethod(galleryPage, "applyDraft"));
    QVERIFY(!galleryPage->property("dirty").toBool());
    QVERIFY(builtin->property("checked").toBool());
    QVERIFY(!hover->property("checked").toBool());
    for (auto it = retainedGalleryDraft.cbegin(); it != retainedGalleryDraft.cend(); ++it)
        QVERIFY(QMetaObject::invokeMethod(galleryPage, "change",
            Q_ARG(QVariant, it.key()), Q_ARG(QVariant, it.value())));
    galleryViewport->setProperty("contentY", qMax(0.0, quickHeading->y() - 120));
    QTest::qWait(150);
    inspect(inspect, galleryPage);
    if (!galleryCapture.isEmpty()) QVERIFY(fixture.window->grabWindow().save(galleryCapture + "-quickview.png"));
    auto *contentBar = visualItemWithObjectName(body, "nativeSettingsVerticalScrollBar");
    QVERIFY(contentBar && contentBar->isVisible());
    const qreal viewportRight = galleryViewport->mapToScene(QPointF(galleryViewport->width(), 0)).x();
    const qreal barLeft = contentBar->mapToScene({}).x();
    qInfo() << "Settings scrollbar physical edges:" << viewportRight * dpr << barLeft * dpr;
    QVERIFY2(barLeft >= viewportRight + 3.5, "Settings scrollbar overlaps content instead of occupying the right gutter");
    auto *outerDialog = visualItemWithObjectName(fixture.window->contentItem(), "semanticDialog-test-settings");
    QVERIFY(outerDialog);
    const qreal dialogRight = outerDialog->mapToScene(QPointF(outerDialog->width(), 0)).x();
    const qreal barRight = contentBar->mapToScene(QPointF(contentBar->width(), 0)).x();
    QVERIFY(qAbs((dialogRight - barRight) * dpr - qRound(4 * dpr)) < 0.001);
    for (auto *part : {contentBar, visualItemWithObjectName(contentBar, "nativeSettingsVerticalScrollBarHandle")}) {
        QVERIFY(part);
        const QPointF origin = part->mapToScene({}) * dpr;
        for (const qreal coordinate : {origin.x(), origin.y(), part->width() * dpr, part->height() * dpr})
            QVERIFY2(qAbs(coordinate - qRound(coordinate)) < 0.001,
                     qPrintable(QString("%1 physical=%2").arg(part->objectName()).arg(coordinate, 0, 'f', 6)));
    }
    galleryViewport->setProperty("contentY", galleryViewport->property("contentHeight").toReal() - galleryViewport->height());
    QTest::qWait(150);
    inspect(inspect, galleryPage);
    if (!galleryCapture.isEmpty()) QVERIFY(fixture.window->grabWindow().save(galleryCapture + "-decoders.png"));
    body->setProperty("selectedNativePage", "gui");
    QTest::qWait(100);
    content = visualItemWithObjectName(body,"themeConfiguratorContent");
    QVERIFY(content);
    const auto capture=qgetenv("F4_NATIVE_SETTINGS_CAPTURE");
    if(!capture.isEmpty()) QVERIFY(fixture.window->grabWindow().save(QString::fromLocal8Bit(capture)));
    auto *list = content->findChild<QQuickItem *>("themeItemsList");
    QVERIFY(QMetaObject::invokeMethod(list,"positionViewAtEnd"));
    auto *viewport = visualItemWithObjectName(body,"nativeSettingsViewport");
    QVERIFY(viewport);
    viewport->setProperty("contentY", viewport->property("contentHeight").toReal()-viewport->height());
    QTest::qWait(150);
    inspect(inspect,content);
    if(!capture.isEmpty()) QVERIFY(fixture.window->grabWindow().save(QString::fromLocal8Bit(capture)+"-scrolled.png"));
    // Native and core categories share pointer and keyboard navigation. Only
    // genuine core indices may be dispatched over the semantic boundary.
    pointer->forceActiveFocus();
    fixture.shell.clearActions();
    QTest::keyClick(fixture.window, Qt::Key_Up);
    QTRY_VERIFY(!content->isVisible());
    auto *settingsDialog = visualItemWithObjectName(fixture.window->contentItem(), "semanticDialog-test-settings");
    QVERIFY(settingsDialog);
    const QSizeF settingsSize(settingsDialog->width(), settingsDialog->height());
    body->setProperty("selectedNativePage", "gui");
    QTest::qWait(100);
    QCOMPARE(QSizeF(settingsDialog->width(), settingsDialog->height()), settingsSize);
    body->setProperty("selectedNativePage", "");
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.last().value("target").toString(), "categories");
    QCOMPARE(fixture.shell.actions.last().value("index").toInt(), 1);
    fixture.shell.clearActions();
    pointer->forceActiveFocus();
    QTest::keyClick(fixture.window, Qt::Key_End);
    QTRY_COMPARE(body->property("selectedNativePage").toString(), "terminal-colors");
    QQuickItem *terminalPage = nullptr;
    QTRY_VERIFY((terminalPage = visualItemWithObjectName(body,"terminalColorsPage")));
    QTest::qWait(150);
    QCOMPARE(QSizeF(settingsDialog->width(), settingsDialog->height()), settingsSize);
    inspect(inspect,terminalPage);
    auto *presetCombo = visualItemWithObjectName(terminalPage,"terminalColorsPreset");
    auto *wheel = visualItemWithObjectName(terminalPage,"terminalColorWheel");
    QVERIFY(presetCombo);
    QVERIFY(wheel);
    QCOMPARE(presetCombo->property("count").toInt(), 3);
    terminalPage->setProperty("selectedIndex", 12);
    QVERIFY(QMetaObject::invokeMethod(wheel,"selectPoint",Q_ARG(QVariant,wheel->width()*0.7),Q_ARG(QVariant,wheel->height()*0.4)));
    QCOMPARE(terminalPage->property("draftPreset").toString(), "custom");
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"resetDraft"));
    for (const QSize size : {QSize(700,850), QSize(1400,1050)}) {
        fixture.window->resize(size);
        QTest::qWait(150);
        inspect(inspect,terminalPage);
        auto *loader = visualItemWithObjectName(body,"nativeSettingsContent");
        QCOMPARE(terminalPage->width(), loader->width());
        QVERIFY(terminalPage->width() <= viewport->width());
        QVERIFY(!visualItemWithObjectName(body,"nativeSettingsHorizontalScrollBar")->isVisible());
        QVERIFY(viewport->property("contentHeight").toReal() >= viewport->height());
        QVERIFY(qAbs(viewport->property("contentWidth").toReal() - viewport->width()) < 0.001);
        for (int i = 0; i < 8; ++i) {
            auto *normal = visualItemWithObjectName(terminalPage, "terminalColorRow" + QString::number(i));
            auto *bright = visualItemWithObjectName(terminalPage, "terminalColorRow" + QString::number(i + 8));
            QVERIFY(normal && bright);
            QCOMPARE(normal->mapToScene({}).y(), bright->mapToScene({}).y());
            QVERIFY(normal->mapToScene({}).x() < bright->mapToScene({}).x());
            for (auto *frame : {normal, bright,
                    visualItemWithObjectName(terminalPage, "terminalColorSwatch" + QString::number(i)),
                    visualItemWithObjectName(terminalPage, "terminalColorSwatch" + QString::number(i + 8))}) {
                QVERIFY(frame);
                const QPointF origin = frame->mapToScene({}) * dpr;
                for (const auto value : {origin.x(), origin.y(), frame->width() * dpr, frame->height() * dpr})
                    QVERIFY2(qAbs(value - qRound(value)) < 0.001,
                             qPrintable(QString("%1 physical coordinate=%2").arg(frame->objectName()).arg(value, 0, 'f', 6)));
            }
            normal = visualItemWithObjectName(terminalPage, "terminalColorPreview" + QString::number(i));
            bright = visualItemWithObjectName(terminalPage, "terminalColorPreview" + QString::number(i + 8));
            QVERIFY(normal && bright);
            QCOMPARE(normal->mapToScene({}).x(), bright->mapToScene({}).x());
            QVERIFY(normal->mapToScene({}).y() < bright->mapToScene({}).y());
        }
        for (auto *shared : {sharedTitle, sharedApply, sharedOK, sharedCancel}) inspect(inspect, shared);
        viewport->setProperty("contentY", viewport->property("contentHeight").toReal() - viewport->height());
        QTest::qWait(50);
        inspect(inspect, terminalPage);
        viewport->setProperty("contentY", 0);
        QVERIFY(wheel->mapToItem(terminalPage,QPointF(wheel->width(),0)).x() <= terminalPage->width());
        if (size.width() == 700) QVERIFY(fixture.window->grabWindow().save(".diagnostics/terminal-colors-narrow-175.png"));
    }
    auto *overrides = visualItemWithObjectName(terminalPage, "terminalColorsEnabled");
    QVERIFY(overrides && overrides->property("checkState").isValid());
    const bool overridesBefore = overrides->property("checked").toBool();
    overrides->forceActiveFocus();
    QTest::keyClick(fixture.window, Qt::Key_Space);
    QCOMPARE(overrides->property("checked").toBool(), !overridesBefore);
    QVERIFY(QMetaObject::invokeMethod(terminalPage, "resetDraft"));
    QVERIFY(visualItemWithObjectName(terminalPage,"terminalColorName15"));
    QVERIFY(visualItemWithObjectName(terminalPage,"terminalColorPreview15"));
    auto *palette = fixture.window->property("terminalPalette").value<QObject *>();
    QVERIFY(palette);
    QCOMPARE(palette->property("preset").toString(), "modern");
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"choosePreset",Q_ARG(QVariant,"classic")));
    QCOMPARE(palette->property("preset").toString(), "modern");
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"resetDraft"));
    QCOMPARE(terminalPage->property("draftPreset").toString(), "modern");
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"choosePreset",Q_ARG(QVariant,"classic")));
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"applyDraft"));
    QCOMPARE(palette->property("preset").toString(), "classic");
    QCOMPARE(fixture.themePersistence.theme().value("terminalPreset").toString(), "classic");
    QVERIFY(QMetaObject::invokeMethod(palette,"load"));
    QCOMPARE(palette->property("preset").toString(), "classic");
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"setColor",Q_ARG(QVariant,12),Q_ARG(QVariant,"invalid")));
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"applyDraft"));
    QCOMPARE(palette->property("preset").toString(), "classic");
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"setColor",Q_ARG(QVariant,12),Q_ARG(QVariant,"#123456")));
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"applyDraft"));
    QCOMPARE(palette->property("preset").toString(), "custom");
    QVERIFY(fixture.themePersistence.theme().value("terminalColors").toString().contains("#123456"));
    QVERIFY(QMetaObject::invokeMethod(terminalPage,"choosePreset",Q_ARG(QVariant,"modern")));
    QVERIFY(fixture.window->grabWindow().save(".diagnostics/terminal-colors-settings-175.png"));
    QVERIFY(fixture.shell.actions.isEmpty());
    auto *coreLabel = visualItemWithObjectName(body,"dialogWidget-categoriesTableCell-0-0");
    QVERIFY(coreLabel);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     coreLabel->mapToScene(QPointF(10, coreLabel->height()/2)).toPoint());
    QTRY_COMPARE(body->property("selectedNativePage").toString(), "");
    QCOMPARE(fixture.shell.actions.last().value("index").toInt(), 0);
    // Pending native drafts survive category changes and use the core footer.
    QCOMPARE(draftValues().value("diskLimitMiB").toInt(), 2048);
    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(sharedApply, "clicked"));
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.last().value("target").toString(), "settings-apply");
    QCOMPARE(galleryPreferences->values().value("diskLimitMiB").toInt(), 2048);
    QCOMPARE(palette->property("preset").toString(), "modern");
    QTRY_VERIFY(!galleryPreferences->busy());
    const auto baselineColor = fixture.window->property("textColor");
    fixture.window->setProperty("textColor", QColor("#aabbcc"));
    QVERIFY(QMetaObject::invokeMethod(terminalPage, "setColor", Q_ARG(QVariant, 12), Q_ARG(QVariant, "invalid")));
    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(sharedOK, "clicked"));
    QVERIFY(fixture.shell.actions.isEmpty());
    QCOMPARE(body->property("selectedNativePage").toString(), "terminal-colors");
    QVERIFY(QMetaObject::invokeMethod(sharedCancel, "clicked"));
    QCOMPARE(fixture.shell.actions.last().value("target").toString(), "settings-cancel");
    QCOMPARE(fixture.window->property("textColor"), baselineColor);
    QCOMPARE(terminalPage->property("draftPreset").toString(), "modern");
    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(sharedOK, "clicked"));
    QCOMPARE(fixture.shell.actions.last().value("target").toString(), "settings-ok");
    fixture.window->setProperty("textColor", QColor("#abcdef"));
    fixture.shell.setScene(shellScene());
    QTRY_COMPARE(fixture.window->property("textColor"), baselineColor);
}

void F4QuickViewSurfaceTests::initTestCase()
{
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    QGuiApplication::styleHints()->setCursorFlashTime(0);
    qmlRegisterType<TestGrid>("F4QtHost", 1, 0, "VtuiGridItem");
}

void F4QuickViewSurfaceTests::worktreeBranchIsCenteredInTitleBar()
{
    QuickViewFixture fixture(shellScene(), false, true,
                             QStringLiteral("feature/worktree-test"));
    QVERIFY(fixture.window);

    QQuickItem *const titleBar = fixture.item(QStringLiteral("titleBar"));
    QQuickItem *const branchLabel = fixture.item(
        QStringLiteral("worktreeBranchLabel"));
    QVERIFY(titleBar);
    QVERIFY(branchLabel);
    QTRY_VERIFY_WITH_TIMEOUT(branchLabel->isVisible(), 1000);
    QCOMPARE(branchLabel->property("text").toString(),
             QStringLiteral("feature/worktree-test"));

    const QPointF titleBarOrigin = titleBar->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPointF branchOrigin = branchLabel->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const qreal titleBarCenterX = titleBarOrigin.x() + titleBar->width() / 2;
    const qreal branchCenterX = branchOrigin.x() + branchLabel->width() / 2;
    QVERIFY2(qAbs(titleBarCenterX - branchCenterX) <= 0.51,
             qPrintable(QStringLiteral("title=%1 branch=%2 labelX=%3 labelWidth=%4")
                            .arg(titleBarCenterX)
                            .arg(branchCenterX)
                            .arg(branchOrigin.x())
                            .arg(branchLabel->width())));
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
            {QStringLiteral("icon"), index == 11
                 ? QStringLiteral("panels-top-left")
                 : QStringLiteral("circle-play")},
            {QStringLiteral("alternatives"), index == 11
                 ? QVariantList{
                       QVariantMap{
                           {QStringLiteral("modifier"), QStringLiteral("shift")},
                           {QStringLiteral("text"), QStringLiteral("Shift action")},
                           {QStringLiteral("icon"), QStringLiteral("pencil")},
                       },
                       QVariantMap{
                           {QStringLiteral("modifier"), QStringLiteral("ctrl")},
                           {QStringLiteral("text"), QStringLiteral("Ctrl action")},
                           {QStringLiteral("icon"), QStringLiteral("copy")},
                       },
                   }
                 : QVariantList{}},
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
    const QColor themedFBarBackground(QStringLiteral("#654321"));
    const QColor themedMutedText(QStringLiteral("#708090"));
    const QColor themedAccent(QStringLiteral("#607080"));
    QVERIFY(fixture.window->setProperty("fBarBg", themedFBarBackground));
    QVERIFY(fixture.window->setProperty("mutedText", themedMutedText));
    QVERIFY(fixture.window->setProperty("dialogAccent", themedAccent));
    QTRY_COMPARE_WITH_TIMEOUT(
        keyBar->property("color").value<QColor>(),
        themedFBarBackground, 1000);
    QQuickItem *f1 = visualItemWithText(keyBar, QStringLiteral("F1"));
    QQuickItem *f12 = visualItemWithText(keyBar, QStringLiteral("F12"));
    QQuickItem *f12Label = visualItemWithText(keyBar,
                                              QStringLiteral("Action 12"));
    QVERIFY(f1);
    QVERIFY(f12);
    QVERIFY(f12Label);
    QQuickItem *f12Button = f12->parentItem();
    QVERIFY(f12Button);
    QQuickItem *f12Icon = nullptr;
    for (QQuickItem *child : f12Button->childItems()) {
        const QUrl source = child->property("source").toUrl();
        if (source.path()
            == QStringLiteral("/F4QtHost/icons/lucide/panels-top-left.svg")) {
            f12Icon = child;
            break;
        }
    }
    QVERIFY(f12Icon);
    QVERIFY(f12Icon->isVisible());
    QCOMPARE(f1->property("text").toString(), QStringLiteral("F1"));
    QCOMPARE(f12->property("text").toString(), QStringLiteral("F12"));
    QCOMPARE(f12->property("color").value<QColor>(), themedMutedText);
    QVariant mnemonic;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "mnemonicText", Q_RETURN_ARG(QVariant, mnemonic),
        Q_ARG(QVariant, QVariant(QStringLiteral("&File"))),
        Q_ARG(QVariant, QVariant(QString()))));
    QCOMPARE(mnemonic.toString(),
             QStringLiteral("<font color=\"%1\">F</font>ile")
                 .arg(themedAccent.name(QColor::HexRgb)));
    QCOMPARE(f12Icon->property("source").toUrl().path(),
             QStringLiteral("/F4QtHost/icons/lucide/panels-top-left.svg"));
    QVERIFY(f12Label->mapToScene(QPointF{}).x()
            >= f12Icon->mapToScene(QPointF(f12Icon->width(), 0)).x() + 6.0);
    QVERIFY(f12->mapToScene(QPointF{}).x()
            >= f12Label->mapToScene(QPointF(f12Label->width(), 0)).x() + 6.0);
    QCOMPARE(f12Button->property("functionKey").toString(),
              QStringLiteral("F12"));
    QCOMPARE(f12Button->property("functionIndex").toInt(), 11);
    QCOMPARE(f12Button->property("iconName").toString(),
             QStringLiteral("panels-top-left"));
    QVERIFY(f12->mapToScene(QPointF(f12->width(), 0)).x()
            <= f12Button->mapToScene(QPointF(f12Button->width(), 0)).x());

    fixture.shell.clearKeyEvents();
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier,
                      f12Button->mapToScene(
                          QPointF(f12Button->width() / 2.0,
                                  f12Button->height() / 2.0)).toPoint());
    auto *alternativeMenu = fixture.window->findChild<QObject *>(
        QStringLiteral("keyBarAlternativeMenu"));
    QVERIFY(alternativeMenu);
    QTRY_VERIFY_WITH_TIMEOUT(alternativeMenu->property("opened").toBool(),
                             1000);
    auto *alternativeList = fixture.item(QStringLiteral(
        "keyBarAlternativeList"));
    QVERIFY(alternativeList);
    QTRY_COMPARE_WITH_TIMEOUT(alternativeList->property("count").toInt(), 2,
                              1000);
    QQuickItem *shiftRow = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (shiftRow = visualItemWithObjectNamePrefix(
             alternativeList, QStringLiteral("keyBarAlternative-shift")))
            != nullptr,
        1000);
    auto *shiftLabel = visualItemWithObjectNamePrefix(
        alternativeList, QStringLiteral("keyBarAlternativeLabel-shift"));
    auto *shiftShortcut = visualItemWithObjectNamePrefix(
        alternativeList, QStringLiteral("keyBarAlternativeShortcut-shift"));
    auto *shiftIcon = visualItemWithObjectNamePrefix(
        alternativeList, QStringLiteral("keyBarAlternativeIcon-shift"));
    QVERIFY(shiftRow);
    QVERIFY(shiftLabel);
    QVERIFY(shiftShortcut);
    QVERIFY(shiftIcon);
    const qreal barDpr = fixture.window->devicePixelRatio();
    QCOMPARE(shiftRow->property("radius").toReal(), qRound(5.0 * barDpr) / barDpr);
    QCOMPARE(shiftLabel->property("text").toString(),
             QStringLiteral("Shift action"));
    QCOMPARE(shiftShortcut->property("text").toString(),
             QStringLiteral("Shift+F12"));
    QCOMPARE(shiftIcon->property("source").toUrl().path(),
             QStringLiteral("/F4QtHost/icons/lucide/pencil.svg"));

    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      QPoint(50, 50));
    QTRY_VERIFY_WITH_TIMEOUT(!alternativeMenu->property("opened").toBool(),
                             1000);

    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier,
                      f12Button->mapToScene(
                          QPointF(f12Button->width() / 2.0,
                                  f12Button->height() / 2.0)).toPoint());
    QTRY_VERIFY_WITH_TIMEOUT(alternativeMenu->property("opened").toBool(),
                             1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (shiftRow = visualItemWithObjectNamePrefix(
             alternativeList, QStringLiteral("keyBarAlternative-shift")))
            != nullptr,
        1000);

    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      shiftRow->mapToScene(
                          QPointF(shiftRow->width() / 2.0,
                                  shiftRow->height() / 2.0)).toPoint());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.keyEvents.size(), 2, 1000);
    QVERIFY(!alternativeMenu->property("opened").toBool());
    QCOMPARE(fixture.shell.keyEvents[0].value(QStringLiteral("vk")).toInt(),
             0x7b);
    QCOMPARE(fixture.shell.keyEvents[0].value(QStringLiteral("mods")).toInt(),
             0x0010);
    QCOMPARE(fixture.shell.keyEvents[1].value(QStringLiteral("down")).toBool(),
             false);

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

void F4QuickViewSurfaceTests::rendererZoomControlsFollowLayoutCapability()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);

    auto *menu = fixture.window->findChild<QObject *>(
        QStringLiteral("panelRendererMenu-0"));
    auto *zoomRow = fixture.item(QStringLiteral("panelRendererZoomRow-0"));
    auto *zoomSlider = fixture.item(
        QStringLiteral("panelRendererZoomSlider-0"));
    QVERIFY(menu);
    QVERIFY(zoomRow);
    QVERIFY(zoomSlider);
    QVERIFY(QMetaObject::invokeMethod(menu, "open"));
    QTRY_VERIFY_WITH_TIMEOUT(zoomRow->isVisible(), 3000);
    QVERIFY(zoomSlider->isVisible());

    QVariantMap compactPanel = panel(0, true);
    compactPanel.remove(QStringLiteral("entries"));
    compactPanel.insert(QStringLiteral("galleryLayoutMode"),
                        QStringLiteral("details"));
    compactPanel.insert(QStringLiteral("galleryDensity"), 61);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), compactPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(zoomRow->isVisible(), 3000);
    QVERIFY(zoomSlider->isVisible());

    compactPanel.insert(QStringLiteral("galleryLayoutMode"),
                        QStringLiteral("columns"));
    compactPanel.insert(QStringLiteral("galleryDensity"), 34);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), compactPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(zoomRow->isVisible(), 3000);
    QVERIFY(zoomSlider->isVisible());

    compactPanel.insert(QStringLiteral("galleryLayoutMode"),
                        QStringLiteral("icons"));
    compactPanel.insert(QStringLiteral("galleryDensity"), 64);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), compactPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(zoomRow->isVisible(), 3000);
    QVERIFY(zoomSlider->isVisible());
    QVERIFY(QMetaObject::invokeMethod(menu, "close"));
}

void F4QuickViewSurfaceTests::panelFileInfoSettingKeepsStatusOverlayAndContentGeometry()
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

    QTRY_VERIFY_WITH_TIMEOUT(footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(footer->height() - footerHeight) < 0.01, 3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(loader->height() - contentHeightWithFooter) < 0.01, 3000);
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
    fixture.window->resize(1900, 900);
    QQuickItem *const leftPanel = fixture.item(QStringLiteral("filePanel-0"));
    QQuickItem *const footer = fixture.item(QStringLiteral("panelStatus-0"));
    QQuickItem *const footerSelection = fixture.item(
        QStringLiteral("panelStatusFiles-0"));
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
    projectedPanel.insert(QStringLiteral("selectedFiles"), 7);
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });

    QTRY_VERIFY_WITH_TIMEOUT(footer->isVisible(), 3000);
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
             QStringLiteral("7"));
    QVERIFY(footer->isVisible());
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(overlay->x() + overlay->width() / 2.0
             - leftPanel->width() / 2.0) * dpr <= 1,
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
    QVERIFY(qAbs(overlay->y() + overlay->height() - footer->y() - footer->height()) < 0.01);
    QVERIFY(qAbs(loader->height() - contentHeightWithoutFooter) < 0.01);

    projectedPanel.insert(QStringLiteral("showFileInfo"), false);
    projectedPanel.insert(QStringLiteral("fastFind"), false);
    projectedPanel.insert(QStringLiteral("fastFindText"), QString{});
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panel"), projectedPanel},
    });
    QTRY_VERIFY_WITH_TIMEOUT(footer->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!overlay->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!overlayCursor->isVisible(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(loader->height() - contentHeightWithoutFooter) < 0.01, 3000);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(loader->property("item").value<QObject *>(), galleryHost);
}

void F4QuickViewSurfaceTests::nativeFontRolesUsePlatformDefaultsAt175Percent()
{
    const QString uiFontFamily = QStringLiteral("F4 Platform UI Test");
    const QString fixedFontFamily = QStringLiteral("F4 Platform Fixed Test");

    QVariantMap scene = shellScene({}, 0);
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantList panels = shell.value(QStringLiteral("panels")).toList();
    QVariantMap leftPanel = panels.at(0).toMap();
    leftPanel.insert(QStringLiteral("fastFind"), true);
    leftPanel.insert(QStringLiteral("fastFindText"),
                     QStringLiteral("Native UI"));
    panels[0] = leftPanel;
    shell.insert(QStringLiteral("panels"), panels);
    shell.insert(QStringLiteral("commandLine"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("prompt"), QStringLiteral("zoin$ ")},
        {QStringLiteral("text"), QStringLiteral("echo fixed-width")},
        {QStringLiteral("cursorPosition"), 16},
    });
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene, true, true, {}, {}, uiFontFamily,
                             fixedFontFamily);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    QVERIFY2(qAbs(dpr - 1.75) < 0.001,
             "Run this regression with QT_SCALE_FACTOR=1.75");
    fixture.window->resize(1200, 700);
    QCoreApplication::processEvents();

    QCOMPARE(fixture.window->property("uiFontFamily").toString(),
             uiFontFamily);
    QCOMPARE(fixture.window->property("guiMonospaceFontFamily").toString(),
             fixedFontFamily);

    QQuickItem *const grid = fixture.item(QStringLiteral("vtuiGrid"));
    QQuickItem *const uiLeaf = fixture.item(
        QStringLiteral("panelFastFindText-0"));
    QQuickItem *const fixedLeaf = fixture.item(
        QStringLiteral("commandLineInput"));
    QVERIFY(grid);
    QVERIFY(uiLeaf);
    QVERIFY(fixedLeaf);
    QTRY_VERIFY_WITH_TIMEOUT(uiLeaf->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(fixedLeaf->isVisible(), 3000);
    QCOMPARE(grid->property("fontFamily").toString(), fixedFontFamily);
    QCOMPARE(uiLeaf->property("font").value<QFont>().family(), uiFontFamily);
    QCOMPARE(fixedLeaf->property("font").value<QFont>().family(),
             fixedFontFamily);

    QQuickItem *const content = fixture.window->contentItem();
    const auto verifyLeaf = [content, dpr](QQuickItem *leaf) {
        const QPointF origin = leaf->mapToItem(content, QPointF{});
        const QPointF physical = origin * dpr;
        const QString details = QStringLiteral(
            "%1 origin is (%2, %3) physical px")
                                    .arg(leaf->objectName())
                                    .arg(physical.x(), 0, 'f', 6)
                                    .arg(physical.y(), 0, 'f', 6);
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physical.y() - qRound64(physical.y())) < 0.001,
                 qPrintable(details));
        QCOMPARE(leaf->mapToItem(content, QPointF(1, 0)) - origin,
                 QPointF(1, 0));
        QCOMPARE(leaf->mapToItem(content, QPointF(0, 1)) - origin,
                 QPointF(0, 1));
    };
    verifyLeaf(uiLeaf);
    verifyLeaf(fixedLeaf);

    QImage frame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(frame = fixture.window->grabWindow()).isNull(), 3000);
    QVERIFY(frame.save(QStringLiteral(
        "/tmp/f4-native-font-policy-175.png")));
    const qreal scaleX = qreal(frame.width()) / fixture.window->width();
    const qreal scaleY = qreal(frame.height()) / fixture.window->height();
    const auto verifyRenderedText = [content, &frame, scaleX, scaleY](
                                        QQuickItem *leaf) {
        const QPointF origin = leaf->mapToItem(content, QPointF{});
        const QRect cropRect(
            qFloor(origin.x() * scaleX), qFloor(origin.y() * scaleY),
            qCeil(leaf->width() * scaleX),
            qCeil(leaf->height() * scaleY));
        const QImage crop = frame.copy(cropRect.intersected(frame.rect()));
        QVERIFY2(!crop.isNull(), qPrintable(leaf->objectName()));
        const QColor foreground = leaf->property("color").value<QColor>();
        int foregroundPixels = 0;
        for (int y = 0; y < crop.height(); ++y) {
            for (int x = 0; x < crop.width(); ++x) {
                const QColor pixel = crop.pixelColor(x, y);
                const int distance = qAbs(pixel.red() - foreground.red())
                                   + qAbs(pixel.green() - foreground.green())
                                   + qAbs(pixel.blue() - foreground.blue());
                if (pixel.alpha() > 0 && distance <= 36)
                    ++foregroundPixels;
            }
        }
        QVERIFY2(foregroundPixels >= 4,
                 qPrintable(QStringLiteral(
                     "%1 rendered only %2 foreground-like pixels")
                                .arg(leaf->objectName())
                                .arg(foregroundPixels)));
    };
    verifyRenderedText(uiLeaf);
    verifyRenderedText(fixedLeaf);
}

void F4QuickViewSurfaceTests::fastFindOverlayAvoidsStatusAndStaysPixelAligned()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);
    const auto dpr = fixture.window->devicePixelRatio();
    if (qEnvironmentVariable("QT_SCALE_FACTOR") == "1.75") QCOMPARE(dpr, 1.75);
    int centered = 0;
    int shifted = 0;
    for (const int windowWidth : {1901, 1103, 741, 1901}) {
        fixture.window->resize(windowWidth, 900);
        for (int scenario = 0; scenario < 3; ++scenario) {
            for (int side = 0; side < 2; ++side) {
                auto state = panel(side, side == 0);
                state.remove("entries");
                state.remove("highlightStyles");
                state["fastFind"] = true;
                state["fastFindText"] = scenario == 2
                    ? "a-long-search-query-that-needs-to-be-elided-in-a-narrow-panel" : "needle";
                state["selectedCount"] = scenario == 1 ? 1358023 : 0;
                state["selectedFiles"] = 1234567;
                state["selectedDirectories"] = 123456;
                state["selectedSize"] = 999.9 * (1LL << 30);
                state["totalFiles"] = 3279;
                state["totalDirectories"] = 112;
                state["totalSize"] = 59.4 * (1LL << 30);
                state["freeSpaceKnown"] = true;
                state["freeSpace"] = 135.0 * (1LL << 30);
                state["diskTotalSpace"] = 12.7 * (1LL << 40);
                fixture.shell.deliverCompactPresentation({
                    {"type", "scene_patch"}, {"side", side}, {"panel", state}});
            }
            QTest::qWait(80);
            for (int side = 0; side < 2; ++side) {
                const QString suffix = "-" + QString::number(side);
                auto *panelItem = fixture.item("filePanel" + suffix);
                auto *overlay = fixture.item("panelFastFindOverlay" + suffix);
                auto *status = fixture.item("panelStatus" + suffix);
                QVERIFY(panelItem && overlay && status);
                QVERIFY(overlay->isVisible() && status->isVisible());
                for (const auto *prefix : {"panelFastFindText", "panelFastFindIcon",
                                           "panelFastFindCursor", "panelFastFindOverlay"}) {
                    auto *item = fixture.item(prefix + suffix);
                    QVERIFY(item);
                    const auto origin = item->mapToItem(fixture.window->contentItem(), QPointF());
                    const auto physical = origin*dpr;
                    qInfo() << item->objectName() << "physical origin" << physical;
                    QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .01
                             && qAbs(physical.y()-qRound64(physical.y())) < .01,
                        qPrintable(QString("%1 physical %2,%3").arg(item->objectName())
                            .arg(physical.x()).arg(physical.y())));
                    QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
                    QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
                    QVERIFY(qAbs(item->width()*dpr-qRound64(item->width()*dpr)) < .01);
                    QVERIFY(qAbs(item->height()*dpr-qRound64(item->height()*dpr)) < .01);
                    const auto local = item->mapToItem(overlay, QPointF());
                    QVERIFY(local.x() >= -.01 && local.y() >= -.01);
                    QVERIFY(local.x()+item->width() <= overlay->width()+.01);
                    QVERIFY(local.y()+item->height() <= overlay->height()+.01);
                }
                const auto searchOrigin = overlay->mapToItem(panelItem, QPointF());
                const auto statusOrigin = status->mapToItem(panelItem, QPointF());
                const qreal gap = fixture.window->property("panelContentSpacing").toReal();
                QVERIFY(qAbs(searchOrigin.y()+overlay->height() - panelItem->height()+gap)*dpr <= 1);
                QVERIFY(searchOrigin.x()*dpr >= gap*dpr-1);
                QVERIFY((statusOrigin.x()-searchOrigin.x()-overlay->width())*dpr >= gap*dpr-1);
                const qreal centeredX = (panelItem->width()-overlay->width())/2;
                if (centeredX+overlay->width()+gap <= statusOrigin.x()) {
                    QVERIFY(qAbs(searchOrigin.x()-centeredX)*dpr <= 1);
                    ++centered;
                } else {
                    QVERIFY(qAbs(searchOrigin.x()+overlay->width()+gap-statusOrigin.x())*dpr <= 1);
                    ++shifted;
                }
            }
            const auto capture = fixture.window->grabWindow();
            QVERIFY(!capture.isNull());
            if (qEnvironmentVariableIsSet("F4_FAST_FIND_CAPTURE"))
                QVERIFY(capture.save(qEnvironmentVariable("F4_FAST_FIND_CAPTURE")
                    + QString("-%1-%2.png").arg(windowWidth).arg(scenario)));
        }
    }
    QVERIFY(centered > 0 && shifted > 0);
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

    auto *splitter = fixture.item(QStringLiteral("mainPanelSplitter"));
    QVERIFY(splitter);
    const QPointF splitterOrigin = splitter->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPoint splitterCenter = (splitterOrigin
                                   + QPointF(splitter->width() / 2,
                                             splitter->height() / 2))
                                      .toPoint();
    QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                       splitterCenter);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(fixture.window->property("panelSplitRatio").toReal() - 0.5)
            < 0.0001,
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(left->width() - fixture.window->width() * 0.5)
                                 < 1.0,
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(right->x() - fixture.window->width() * 0.5)
                                 < 1.0,
                             3000);
}

void F4QuickViewSurfaceTests::galleryPanelColorsAreGroupedAndRemainLive()
{
    QuickViewFixture fixture(shellScene(), true);
    QVERIFY(fixture.window);

    auto *loader = fixture.item(QStringLiteral("galleryPanelContent-0"));
    auto *path = fixture.item(QStringLiteral("panelPathTitle-0"));
    auto *panelHeader = fixture.item(QStringLiteral("panelHeader-0"));
    auto *panelHeaderPanelBackground = fixture.item(
        QStringLiteral("panelHeaderPanelBackground-0"));
    QVERIFY(loader);
    QVERIFY(path);
    QVERIFY(panelHeader);
    QVERIFY(panelHeaderPanelBackground);
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
    const QColor titleBarBackground(QStringLiteral("#112233"));
    const QColor panelHeaderBackground(QStringLiteral("#80445566"));
    const QColor fileText(QStringLiteral("#aabbcc"));
    const QColor folderText(QStringLiteral("#ddeeff"));

    QVERIFY(fixture.window->setProperty("titleBarBg", titleBarBackground));
    QVERIFY(fixture.window->setProperty("panelPathBg",
                                        panelHeaderBackground));
    QVERIFY(fixture.window->setProperty("galleryTextColor", textColor));
    QVERIFY(fixture.window->setProperty("galleryFileTextColor", fileText));
    QVERIFY(fixture.window->setProperty("galleryFolderTextColor", folderText));
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
        return qmlObjectProperties(gallery->property("theme"));
    };
    QTRY_COMPARE_WITH_TIMEOUT(
        liveTheme().value(QStringLiteral("text")).value<QColor>(), textColor,
        3000);
    QCOMPARE(liveTheme().value(QStringLiteral("cardCursorBorder"))
                 .value<QColor>(),
             cursorBorder);
    QCOMPARE(liveTheme().value(QStringLiteral("itemHover")).value<QColor>(),
             cardHover);
    QCOMPARE(liveTheme().value(QStringLiteral("fileText")).value<QColor>(),
             fileText);
    QCOMPARE(liveTheme().value(QStringLiteral("folderText")).value<QColor>(),
             folderText);
    QCOMPARE(liveTheme().value(QStringLiteral("neutralFileTextColors")).toBool(),
             true);
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
    QCOMPARE(panelHeader->property("color").value<QColor>(),
             titleBarBackground);
    QCOMPARE(panelHeaderPanelBackground->property("color").value<QColor>(),
             panelHeaderBackground);
    QCOMPARE(loader->property("item").value<QObject *>(), gallery);
}

void F4QuickViewSurfaceTests::themeConfiguratorExposesOnlyLiveColorProperties()
{
    QuickViewFixture fixture(shellScene(), true);
    QVERIFY(fixture.window);

    const QVariantList definitions =
        fixture.window->property("themeColorDefinitions").toList();
    QStringList colorIds;
    for (const QVariant &definitionValue : definitions) {
        colorIds.append(definitionValue.toMap()
                            .value(QStringLiteral("id"))
                            .toString());
    }

    for (const QString &obsolete : {
             QStringLiteral("terminalBg"),
             QStringLiteral("markedBg"),
             QStringLiteral("markedText"),
             QStringLiteral("folderIconColor"),
             QStringLiteral("panelBg"),
             QStringLiteral("panelBgAlt"),
             QStringLiteral("panelHeaderBg"),
             QStringLiteral("panelBorder"),
             QStringLiteral("chromeBg"),
         }) {
        QVERIFY2(!colorIds.contains(obsolete), qPrintable(obsolete));
        QVERIFY(!fixture.window->property(obsolete.toUtf8().constData())
                     .isValid());
    }

    for (const QString &required : {
             QStringLiteral("commandLineBg"),
              QStringLiteral("titleBarBg"),
              QStringLiteral("fBarBg"),
              QStringLiteral("separatorActiveColor"),
              QStringLiteral("galleryFileTextColor"),
              QStringLiteral("galleryFolderTextColor"),
              QStringLiteral("galleryFolderIconColor"),
         }) {
        QVERIFY2(colorIds.contains(required), qPrintable(required));
    }

    const QVariantMap expectedDefaults{
        {QStringLiteral("windowBackgroundColor"), QStringLiteral("#191d23")},
        {QStringLiteral("titleBarBg"), QStringLiteral("#19202b")},
        {QStringLiteral("fBarBg"), QStringLiteral("#19202b")},
        {QStringLiteral("panelPathBg"), QStringLiteral("#26576478")},
        {QStringLiteral("commandLineBg"), QStringLiteral("#141921")},
        {QStringLiteral("separatorColor"), QStringLiteral("#2d3642")},
        {QStringLiteral("galleryPanelBackgroundColor"),
         QStringLiteral("#00000000")},
        {QStringLiteral("galleryViewerBackgroundColor"),
         QStringLiteral("#00000000")},
        {QStringLiteral("galleryItemBackgroundColor"),
         QStringLiteral("#00000000")},
        {QStringLiteral("galleryDirectoryBackgroundColor"),
         QStringLiteral("#00000000")},
        {QStringLiteral("galleryItemHoverColor"),
         QStringLiteral("#1a75afe5")},
        {QStringLiteral("galleryScrollBarHandleColor"),
         QStringLiteral("#434b57")},
        {QStringLiteral("galleryScrollBarBackgroundHoverColor"),
         QStringLiteral("#5f6875")},
        {QStringLiteral("galleryScrollBarHoverColor"),
         QStringLiteral("#7f8896")},
        {QStringLiteral("galleryScrollBarPressedColor"),
         QStringLiteral("#47515d")},
        {QStringLiteral("galleryPathBackgroundColor"),
         QStringLiteral("#00000000")},
        {QStringLiteral("galleryPathHoverColor"),
         QStringLiteral("#2a3745")},
    };
    QVariantMap configuredDefaults;
    for (const QVariant &definitionValue : definitions) {
        const QVariantMap definition = definitionValue.toMap();
        const QString id = definition.value(QStringLiteral("id")).toString();
        configuredDefaults.insert(
            id, definition.value(QStringLiteral("defaultColor")));
    }
    for (auto it = expectedDefaults.constBegin();
         it != expectedDefaults.constEnd(); ++it) {
        QVERIFY2(configuredDefaults.contains(it.key()),
                 qPrintable(it.key()));
        const QColor expected(it.value().toString());
        QCOMPARE(QColor(configuredDefaults.value(it.key()).toString()),
                 expected);
        QCOMPARE(fixture.window
                     ->property(it.key().toUtf8().constData())
                     .value<QColor>(), expected);
    }

    QStringList panelsGroupIds;
    QVariantMap activeAccent;
    for (const QVariant &definitionValue : definitions) {
        const QVariantMap definition = definitionValue.toMap();
        if (definition.value(QStringLiteral("group")).toString()
            == QStringLiteral("Panels"))
            panelsGroupIds.append(definition.value(QStringLiteral("id"))
                                    .toString());
        if (definition.value(QStringLiteral("id")).toString()
            == QStringLiteral("activeBorder")) {
            activeAccent = definition;
        }
    }
    QCOMPARE(panelsGroupIds, QStringList{QStringLiteral("panelPathBg")});
    QCOMPARE(activeAccent.value(QStringLiteral("group")).toString(),
             QStringLiteral("Icons & Accents"));
    QCOMPARE(activeAccent.value(QStringLiteral("name")).toString(),
             QStringLiteral("Attention / Active Label"));

    auto *loader = fixture.item(QStringLiteral("galleryPanelContent-0"));
    QVERIFY(loader);
    QTRY_COMPARE_WITH_TIMEOUT(loader->property("status").toInt(), 1, 3000);
    QObject *const gallery = loader->property("item").value<QObject *>();
    QVERIFY(gallery);
    const QVariantMap theme = qmlObjectProperties(gallery->property("theme"));
    for (const QString &unusedGalleryKey : {
             QStringLiteral("background"),
             QStringLiteral("backgroundAlternate"),
             QStringLiteral("border"),
             QStringLiteral("activeBorder"),
             QStringLiteral("marked"),
             QStringLiteral("chrome"),
             QStringLiteral("chromeText"),
         }) {
        QVERIFY2(!theme.contains(unusedGalleryKey),
                 qPrintable(unusedGalleryKey));
    }
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

void F4QuickViewSurfaceTests::themeColorEditorUsesOklchCoordinates()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);

    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    QVERIFY(dialog);
    auto *hueSlider = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeHueSlider"));
    auto *chromaSlider = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeChromaSlider"));
    auto *chromaInput = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeChromaInputBox"));
    QVERIFY(hueSlider);
    QVERIFY(chromaSlider);
    QVERIFY(chromaInput);

    const QColor red(QStringLiteral("#ff0000"));
    QVERIFY(QMetaObject::invokeMethod(
        dialog, "setFromColor", Qt::DirectConnection,
        Q_ARG(QVariant, QVariant(red))));

    // sRGB red is approximately OKLCH(0.628, 0.258, 29.2deg).
    const qreal hueDegrees = dialog->property("selectedHue").toReal() * 360;
    const qreal chroma = dialog->property("selectedChroma").toReal();
    const qreal lightness = dialog->property("selectedLightness").toReal();
    QVERIFY(qAbs(hueDegrees - 29.2) < 0.2);
    QVERIFY(qAbs(chroma - 0.258) < 0.002);
    QVERIFY(qAbs(lightness - 0.628) < 0.002);
    QCOMPARE(chromaSlider->property("from").toReal(), 0.0);
    QCOMPARE(chromaSlider->property("to").toReal(), 0.4);
    const qreal dialogDpr = dialog->devicePixelRatio();
    QCOMPARE(chromaInput->property("implicitWidth").toReal(),
             qRound(38.0 * dialogDpr) / dialogDpr);

    dialog->show();
    QTRY_VERIFY_WITH_TIMEOUT(dialog->isVisible(), 1000);
    constexpr qreal requestedChroma = 0.35;
    constexpr qreal initialHue = 0.20;
    constexpr qreal highLightness = 0.95;
    QVERIFY(dialog->setProperty("selectedChroma", requestedChroma));
    QVERIFY(dialog->setProperty("selectedHue", initialHue));
    QVERIFY(dialog->setProperty("selectedLightness", highLightness));
    QVERIFY(QMetaObject::invokeMethod(dialog, "applyCurrentColor",
                                      Qt::DirectConnection));
    QCOMPARE(dialog->property("selectedChroma").toReal(), requestedChroma);
    QCOMPARE(dialog->property("selectedHue").toReal(), initialHue);

    // L and H may require a different sRGB gamut mapping, but they must not
    // feed that mapped chroma back into the independent editor coordinate.
    constexpr qreal lowerLightness = 0.55;
    QVERIFY(dialog->setProperty("selectedLightness", lowerLightness));
    QVERIFY(QMetaObject::invokeMethod(dialog, "applyCurrentColor",
                                      Qt::DirectConnection));
    QCOMPARE(dialog->property("selectedChroma").toReal(), requestedChroma);
    QCOMPARE(dialog->property("selectedHue").toReal(), initialHue);

    constexpr qreal changedHue = 0.75;
    QVERIFY(dialog->setProperty("selectedHue", changedHue));
    QVERIFY(QMetaObject::invokeMethod(dialog, "applyCurrentColor",
                                      Qt::DirectConnection));
    QCOMPARE(dialog->property("selectedChroma").toReal(), requestedChroma);
    QCOMPARE(dialog->property("selectedLightness").toReal(),
             lowerLightness);
    dialog->hide();
}

void F4QuickViewSurfaceTests::themeConfiguratorRestoresSavedTheme()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);

    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    auto *footer = dialog ? dialog->findChild<QQuickItem *>(
        QStringLiteral("themeColorFooter")) : nullptr;
    auto *resetElementButton = dialog ? dialog->findChild<QQuickItem *>(
        QStringLiteral("themeResetElementButton")) : nullptr;
    auto *resetAllButton = dialog ? dialog->findChild<QQuickItem *>(
        QStringLiteral("themeResetAllButton")) : nullptr;
    auto *restoreSavedButton = dialog ? dialog->findChild<QQuickItem *>(
        QStringLiteral("themeRestoreSavedButton")) : nullptr;
    auto *saveButton = dialog ? dialog->findChild<QQuickItem *>(
        QStringLiteral("themeSaveButton")) : nullptr;
    auto *closeButton = dialog ? dialog->findChild<QQuickItem *>(
        QStringLiteral("themeCloseButton")) : nullptr;
    QVERIFY(dialog);
    QVERIFY(footer);
    QVERIFY(resetElementButton);
    QVERIFY(resetAllButton);
    QVERIFY(restoreSavedButton);
    QVERIFY(saveButton);
    QVERIFY(closeButton);

    dialog->resize(580, 560);
    dialog->show();
    QTRY_VERIFY_WITH_TIMEOUT(dialog->isVisible(), 1000);
    QCoreApplication::processEvents();
    const auto verifyFooterLayout = [&]() {
        qreal previousRight = 0;
        QStringList footerButtonGeometry;
        for (QQuickItem *button : {resetElementButton, resetAllButton,
                                   restoreSavedButton, saveButton,
                                   closeButton}) {
            const QPointF origin = button->mapToItem(footer, QPointF{});
            footerButtonGeometry.append(QStringLiteral("%1:%2+%3")
                                            .arg(button->objectName())
                                            .arg(origin.x())
                                            .arg(button->width()));
            QVERIFY(origin.x() >= previousRight);
            previousRight = origin.x() + button->width();
        }
        const QByteArray footerLayoutDetails = QStringLiteral(
            "footer buttons end at %1 but footer width is %2 (%3)")
                                                   .arg(previousRight)
                                                   .arg(footer->width())
                                                   .arg(
                                                       footerButtonGeometry.join(
                                                           QStringLiteral(", ")))
                                                   .toUtf8();
        QVERIFY2(previousRight <= footer->width(),
                 footerLayoutDetails.constData());
    };
    verifyFooterLayout();
    dialog->resize(720, 560);
    QCoreApplication::processEvents();
    verifyFooterLayout();

    const QColor savedWindowBackground(QStringLiteral("#112233"));
    const QColor savedLegacyChrome(QStringLiteral("#445566"));
    const QVariantMap savedTheme{
        {QStringLiteral("windowBackgroundColor"),
         savedWindowBackground.name(QColor::HexRgb)},
        {QStringLiteral("chromeBg"),
         savedLegacyChrome.name(QColor::HexRgb)},
        // This was the shipped value before brick hover became visible.
        {QStringLiteral("galleryItemHoverColor"),
         QStringLiteral("#00000000")},
        {QStringLiteral("fontRenderType"), QStringLiteral("QtRendering")},
        {QStringLiteral("mouseWheelMode"), QStringLiteral("console")},
        {QStringLiteral("neutralFileTextColors"), false},
    };
    fixture.themePersistence.setTheme(savedTheme);

    QVERIFY(fixture.window->setProperty("windowBackgroundColor",
                                        QColor(QStringLiteral("#abcdef"))));
    QVERIFY(fixture.window->setProperty("titleBarBg",
                                        QColor(QStringLiteral("#102030"))));
    QVERIFY(fixture.window->setProperty("fBarBg",
                                        QColor(QStringLiteral("#203040"))));
    fixture.textRenderingPolicy.setRenderType(
        int(QQuickWindow::NativeTextRendering));
    QVERIFY(fixture.window->setProperty("mouseWheelMode",
                                        QStringLiteral("gui")));
    QVERIFY(fixture.window->setProperty("galleryNeutralFileTextColors", true));

    // Reset All is an editor operation. It must not erase the saved snapshot,
    // otherwise Restore Saved could not undo the previewed defaults.
    QVERIFY(QMetaObject::invokeMethod(resetAllButton, "clicked",
                                      Qt::DirectConnection));
    QCOMPARE(fixture.themePersistence.resetCalls(), 0);
    QCOMPARE(fixture.themePersistence.theme(), savedTheme);
    QCoreApplication::processEvents();
    verifyFooterLayout();
    QVERIFY(fixture.window->setProperty("galleryItemHoverColor",
                                        QColor(QStringLiteral("#abcdef"))));

    QVERIFY(QMetaObject::invokeMethod(restoreSavedButton, "clicked",
                                      Qt::DirectConnection));
    QCOMPARE(fixture.window->property("windowBackgroundColor")
                 .value<QColor>(), savedWindowBackground);
    QCOMPARE(fixture.window->property("titleBarBg").value<QColor>(),
             savedLegacyChrome);
    QCOMPARE(fixture.window->property("fBarBg").value<QColor>(),
             savedLegacyChrome);
    QCOMPARE(fixture.window->property("galleryItemHoverColor")
                 .value<QColor>(),
             QColor(QStringLiteral("#1a75afe5")));
    QCOMPARE(fixture.textRenderingPolicy.renderTypeName(),
             QStringLiteral("QtRendering"));
    QCOMPARE(fixture.window->property("mouseWheelMode").toString(),
             QStringLiteral("console"));
    QCOMPARE(fixture.window->property("galleryNeutralFileTextColors").toBool(),
             false);
    QCOMPARE(dialog->property("statusToast").toString(),
             QStringLiteral("Restored saved theme"));
    QCoreApplication::processEvents();
    verifyFooterLayout();
    dialog->hide();
}

void F4QuickViewSurfaceTests::themeSelectionBordersAreLiveAndPersisted()
{
    QuickViewFixture fixture(shellScene(), true);
    QVERIFY(fixture.window);
    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    QVERIFY(dialog);
    auto *checkBox = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeSelectionBorderCheckBox"));
    QVERIFY(checkBox);
    const auto liveBorderSetting = [&] {
        return qmlObjectProperties(fixture.window->property("galleryThemePalette"))
            .value(QStringLiteral("showSelectionBorders")).toBool();
    };
    QVERIFY(fixture.window->property("galleryShowSelectionBorders").toBool());
    QVERIFY(checkBox->property("checked").toBool());
    QVERIFY(liveBorderSetting());
    dialog->show();
    QTRY_VERIFY(dialog->isVisible());
    const int initialActions = fixture.shell.actions.size();
    QTest::mouseClick(dialog, Qt::LeftButton, Qt::NoModifier,
        checkBox->mapToScene(QPointF(checkBox->width() / 2, checkBox->height() / 2)).toPoint());
    QVERIFY(!fixture.window->property("galleryShowSelectionBorders").toBool());
    QVERIFY(!checkBox->property("checked").toBool());
    QVERIFY(!liveBorderSetting());
    QCOMPARE(fixture.shell.actions.size(), initialActions);

    const auto clickButton = [dialog](const QString &name) {
        auto *button = dialog->findChild<QQuickItem *>(name);
        return button && QMetaObject::invokeMethod(button, "clicked", Qt::DirectConnection);
    };
    QVERIFY(clickButton(QStringLiteral("themeSaveButton")));
    QVERIFY(fixture.themePersistence.theme().contains(QStringLiteral("showSelectionBorders")));
    QCOMPARE(fixture.themePersistence.theme().value(QStringLiteral("showSelectionBorders")).toBool(), false);
    QVERIFY(clickButton(QStringLiteral("themeResetAllButton")));
    QVERIFY(checkBox->property("checked").toBool());
    QVERIFY(liveBorderSetting());
    QVERIFY(clickButton(QStringLiteral("themeRestoreSavedButton")));
    QVERIFY(!checkBox->property("checked").toBool());
    QVERIFY(!liveBorderSetting());

    // QSettings returns strings, and old themes have no selection-border key.
    for (const QString &saved : {QStringLiteral("true"), QStringLiteral("false")}) {
        fixture.themePersistence.setTheme({{QStringLiteral("showSelectionBorders"), saved}});
        QVERIFY(clickButton(QStringLiteral("themeRestoreSavedButton")));
        QCOMPARE(liveBorderSetting(), saved == QStringLiteral("true"));
        QCOMPARE(checkBox->property("checked").toBool(), liveBorderSetting());
    }
    fixture.themePersistence.setTheme({{QStringLiteral("windowBackgroundColor"), QStringLiteral("#123456")}});
    QVERIFY(clickButton(QStringLiteral("themeRestoreSavedButton")));
    QVERIFY(liveBorderSetting());
    QVERIFY(checkBox->property("checked").toBool());
    dialog->hide();
}

void F4QuickViewSurfaceTests::functionBarSameCountUpdatesPreserveDelegates()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("keyBar"), keyBarModel(12));
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    const auto itemNamed = [&fixture](const QString &name) {
        return visualItemWithObjectName(fixture.window->contentItem(), name);
    };
    QVector<QPointer<QQuickItem>> delegates;
    for (int index = 1; index <= 12; ++index) {
        auto *item = itemNamed(QStringLiteral("key-bar-action-%1").arg(index));
        QVERIFY(item);
        delegates.append(item);
    }

    QVector<qint64> timings;
    for (int iteration = 0; iteration < 100; ++iteration) {
        QElapsedTimer timer;
        timer.start();
        fixture.shell.chromeState()->applyState(
            {{QStringLiteral("keyBar"), keyBarModel(12, iteration % 2 == 0)}},
            iteration + 2);
        timings.append(timer.nsecsElapsed());
        QCoreApplication::processEvents();
    }
    std::sort(timings.begin(), timings.end());
    qInfo("KEYBAR_UPDATE n=100 synchronous_ms p50=%.6f p95=%.6f max=%.6f",
          timings[49] / 1e6, timings[94] / 1e6, timings[99] / 1e6);
    for (int index = 1; index <= delegates.size(); ++index) {
        QVERIFY2(delegates[index - 1],
                 "same-count keybar update destroyed an existing F-key delegate");
        QCOMPARE(itemNamed(QStringLiteral("key-bar-action-%1").arg(index)),
                 delegates[index - 1].data());
        QCOMPARE(itemNamed(QStringLiteral("key-bar-label-%1").arg(index))
                     ->property("text").toString(),
                 QStringLiteral("Open %1").arg(index));
    }

    // The actual item count remains authoritative, including empty or
    // nonstandard keybars. There is no fixed-size replacement model.
    for (int count : {5, 14, 0, 7}) {
        fixture.shell.chromeState()->applyState(
            {{QStringLiteral("keyBar"), keyBarModel(count, true)}}, 200 + count);
        QCoreApplication::processEvents();
        for (int index = 1; index <= count; ++index) {
            auto *item = itemNamed(QStringLiteral("key-bar-action-%1").arg(index));
            QVERIFY(item);
            QCOMPARE(item->property("functionIndex").toInt(), index - 1);
            QCOMPARE(itemNamed(QStringLiteral("key-bar-label-%1").arg(index))
                         ->property("text").toString(),
                     QStringLiteral("Edit %1").arg(index));
        }
        QVERIFY(!itemNamed(QStringLiteral("key-bar-action-%1").arg(count + 1)));
    }
}

void F4QuickViewSurfaceTests::functionBarLeavesStaySharpAndThemeLiveAt175Percent()
{
    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("menuBar"), QVariantMap{{QStringLiteral("items"), QVariantList{}}});
    scene.insert(QStringLiteral("keyBar"), keyBarModel(12));
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) > 0.001)
        QSKIP("175% DPR invocation required");
    auto *keyBar = fixture.item(QStringLiteral("keyBar"));
    QVERIFY(keyBar);
    auto *root = fixture.window->contentItem();

    for (int theme = 0; theme < 2; ++theme) {
        const QColor background(theme ? QStringLiteral("#213546") : QStringLiteral("#654321"));
        const QColor foreground(theme ? QStringLiteral("#f0d0a0") : QStringLiteral("#b8e2fa"));
        const QColor secondary(theme ? QStringLiteral("#9ab8d6") : QStringLiteral("#d8b0e0"));
        QVERIFY(fixture.window->setProperty("fBarBg", background));
        QVERIFY(fixture.window->setProperty("chromeText", foreground));
        QVERIFY(fixture.window->setProperty("mutedText", secondary));
        fixture.shell.chromeState()->applyState(
            {{QStringLiteral("keyBar"), keyBarModel(12, theme != 0)}}, 500 + theme);
        fixture.window->resize(theme ? 937 : 900, 640);
        QTest::mouseMove(fixture.window, QPoint(450, 300));
        QCoreApplication::processEvents();
        fixture.window->requestUpdate();
        QImage frame;
        QTRY_VERIFY_WITH_TIMEOUT(!(frame = fixture.window->grabWindow()).isNull(), 3000);
        const QString capture = qEnvironmentVariable("F4_KEYBAR_TEST_CAPTURE");
        if (!capture.isEmpty())
            QVERIFY(frame.save(capture + QStringLiteral("-%1.png").arg(theme)));
        QCOMPARE(keyBar->property("color").value<QColor>(), background);

        for (int index = 1; index <= 12; ++index) {
            for (const auto &prefix : {QStringLiteral("key-bar-action-"), QStringLiteral("key-bar-separator-")}) {
                auto *edge = visualItemWithObjectName(root, prefix + QString::number(index));
                QVERIFY(edge);
                const auto physical = edge->mapToItem(root, QPointF{}) * dpr;
                for (const auto value : {physical.x(), physical.y(), edge->width()*dpr, edge->height()*dpr})
                    QVERIFY2(qAbs(value-qRound(value)) < .001,
                             qPrintable(QString("%1 physical=%2").arg(edge->objectName()).arg(value, 0, 'f', 6)));
            }
            for (const QString &prefix : {QStringLiteral("key-bar-label-"),
                                          QStringLiteral("key-bar-shortcut-"),
                                          QStringLiteral("key-bar-icon-")}) {
                auto *leaf = visualItemWithObjectName(root, prefix + QString::number(index));
                QVERIFY(leaf);
                QVERIFY(leaf->isVisible());
                const QPointF origin = leaf->mapToItem(root, QPointF{});
                const QPointF xAxis = leaf->mapToItem(root, QPointF(1, 0)) - origin;
                const QPointF yAxis = leaf->mapToItem(root, QPointF(0, 1)) - origin;
                const QByteArray details = QStringLiteral("%1 origin=(%2,%3) physical at DPR %4")
                    .arg(leaf->objectName()).arg(origin.x() * dpr, 0, 'f', 6)
                    .arg(origin.y() * dpr, 0, 'f', 6).arg(dpr).toUtf8();
                QVERIFY2(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001, details.constData());
                QVERIFY2(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001, details.constData());
                QVERIFY2(qAbs(xAxis.x() - 1) < 0.001 && qAbs(xAxis.y()) < 0.001 &&
                         qAbs(yAxis.x()) < 0.001 && qAbs(yAxis.y() - 1) < 0.001,
                         "keybar visual leaf inherited a non-unit scene transform");
                if (prefix == QStringLiteral("key-bar-label-")) {
                    QCOMPARE(leaf->property("color").value<QColor>(), foreground);
                    QCOMPARE(leaf->property("renderType").toInt(), int(QQuickWindow::NativeTextRendering));
                } else if (prefix == QStringLiteral("key-bar-shortcut-")) {
                    QCOMPARE(leaf->property("color").value<QColor>(), secondary);
                } else {
                    QCOMPARE(QUrlQuery(leaf->property("source").toUrl())
                                 .queryItemValue(QStringLiteral("color")), foreground.name(QColor::HexArgb));
                }
            }
        }
        const QPointF origin = keyBar->mapToItem(root, QPointF{});
        const QRect crop(qRound(origin.x() * dpr), qRound(origin.y() * dpr),
                         qRound(keyBar->width() * dpr), qRound(keyBar->height() * dpr));
        QVERIFY(frame.rect().contains(crop));
        const QImage keybarFrame = frame.copy(crop);
        QVERIFY(imageContainsColor(keybarFrame, background));
        QVERIFY2(imageContainsColor(keybarFrame, foreground), "keybar action text did not render in the live theme");
        QVERIFY2(imageContainsColor(keybarFrame, secondary), "small function-key captions did not render in the live theme");
    }
}

void F4QuickViewSurfaceTests::themeBooleanOptionsFollowLivePalette()
{
    QuickViewFixture fixture(shellScene());
    QVERIFY(fixture.window);
    auto *dialog = fixture.window->findChild<QQuickWindow *>(
        QStringLiteral("themeColorConfigurator"));
    QVERIFY(dialog);
    dialog->show();
    QTRY_VERIFY(dialog->isVisible());
    for (const QString &prefix : {QStringLiteral("themeNeutralFileText"),
                                  QStringLiteral("themeSelectionBorder")}) {
        auto *row = dialog->findChild<QQuickItem *>(prefix + QStringLiteral("Panel"));
        auto *title = dialog->findChild<QQuickItem *>(prefix + QStringLiteral("Title"));
        auto *description = dialog->findChild<QQuickItem *>(prefix + QStringLiteral("Description"));
        auto *checkBox = dialog->findChild<QQuickItem *>(prefix + QStringLiteral("CheckBox"));
        auto *indicator = dialog->findChild<QQuickItem *>(prefix + QStringLiteral("Indicator"));
        auto *mark = dialog->findChild<QQuickItem *>(prefix + QStringLiteral("CheckMark"));
        QVERIFY(row && title && description && checkBox && indicator && mark);
        for (const bool alternate : {false, true}) {
            const QColor background(alternate ? "#352847" : "#263e45");
            const QColor text(alternate ? "#ffdfbb" : "#e1fbd3");
            const QColor muted(alternate ? "#adcbfb" : "#d2a3eb");
            const QColor accent(alternate ? "#7ccd54" : "#ea8255");
            const QColor control(alternate ? "#493259" : "#153429");
            for (const auto &setting : {qMakePair("dialogHeaderBg", background),
                    qMakePair("textColor", text), qMakePair("mutedText", muted),
                    qMakePair("dialogAccent", accent), qMakePair("controlBg", control)}) {
                QVERIFY(fixture.window->setProperty(setting.first, setting.second));
            }
            QCOMPARE(row->property("color").value<QColor>(), background);
            QCOMPARE(title->property("color").value<QColor>(), text);
            QCOMPARE(description->property("color").value<QColor>(), muted);
            for (const bool checked : {true, false}) {
                const char *setting = prefix == QStringLiteral("themeSelectionBorder")
                    ? "galleryShowSelectionBorders" : "galleryNeutralFileTextColors";
                QVERIFY(fixture.window->setProperty(setting, checked));
                QTest::mouseMove(dialog, QPoint(4, 4));
                QTRY_COMPARE(indicator->property("color").value<QColor>(), checked ? accent : control);
                QCOMPARE(mark->isVisible(), checked);
                if (!checked) {
                    checkBox->forceActiveFocus(Qt::TabFocusReason);
                    QTRY_VERIFY(checkBox->hasActiveFocus());
                }
                QImage frame;
                QTRY_VERIFY(!(frame = dialog->grabWindow()).isNull());
                const QRectF rect = row->mapRectToScene(row->boundingRect());
                const qreal dpr = dialog->devicePixelRatio();
                const QImage rowFrame = frame.copy(QRect(qRound(rect.x() * dpr), qRound(rect.y() * dpr),
                    qRound(rect.width() * dpr), qRound(rect.height() * dpr)));
                QVERIFY(imageContainsColor(rowFrame, background));
                QVERIFY(imageContainsColor(rowFrame, checked ? accent : control));
                QVERIFY(imageContainsColor(rowFrame, text));
                if (!checked)
                    QVERIFY(imageContainsColor(rowFrame, accent));
            }
        }
    }
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

void F4QuickViewSurfaceTests::historyHeldUpKeepsRowsOnPhysicalPixels()
{
    QVariantList rows;
    for (int index = 0; index < 240; ++index)
        rows.append(QVariantMap{{"index", index}, {"details", QVariantMap{
            {"kind", "history"}, {"columns", "command"}, {"primary", "command text"},
            {"path", "C:/work"}, {"date", "2026-09-14 00:00:00"}}}});
    QVariantMap menu{{"id", "history-scroll"}, {"kind", "menu"}, {"role", "vmenu"}, {"title", "History [query]"},
        {"w", 90}, {"h", 32}, {"x", 3}, {"y", 2}, {"selected", 239}, {"top", 210}, {"items", rows}};
    auto scene = shellScene(); scene["menus"] = QVariantList{menu};
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    fixture.window->resize(1001, 643);
    QTest::qWait(60);
    auto *root = fixture.window->contentItem();
    const qreal dpr = fixture.window->devicePixelRatio();
    QCOMPARE(dpr, 1.75);
    auto *title = visualItemWithObjectName(root, "semanticMenuTitle-history-scroll");
    QVERIFY(title && title->isVisible());
    QCOMPARE(title->property("text").toString(), QString("History [query]"));
    const auto titleOrigin = title->mapToItem(root, QPointF());
    QVERIFY(qAbs(titleOrigin.x()*dpr-qRound64(titleOrigin.x()*dpr)) < .001);
    QVERIFY(qAbs(titleOrigin.y()*dpr-qRound64(titleOrigin.y()*dpr)) < .001);
    QCOMPARE(title->mapToItem(root,QPointF(1,0))-titleOrigin,QPointF(1,0));
    QCOMPARE(title->mapToItem(root,QPointF(0,1))-titleOrigin,QPointF(0,1));
    qreal scrollingTop = -1;
    QList<int> positions;
    for (int index = 238; index >= 158; --index) positions.append(index);
    for (int index = 159; index <= 238; ++index) positions.append(index);
    for (int step = 0; step < positions.size(); ++step) {
        const int index = positions[step];
        if (step == 81) scrollingTop = -1;
        menu["selected"] = index; menu["top"] = qMax(0, index - 10);
        fixture.shell.applyCommandMenus({menu}, true);
        fixture.shell.deliverCommandMenuStates(fixture.shell.overlayState()->commandMenuStates());
        QTest::qWait(4);
        const auto capture = fixture.window->grabWindow();
        QVERIFY(!capture.isNull());
        QQuickItem *row = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((row = visualItemWithObjectName(root, "semanticMenuItem-history-scroll-" + QString::number(index))), 1000);
        if ((step < 81 && index <= 218) || (step >= 81 && index >= 181)) {
            const qreal top = row->mapToItem(root, QPointF()).y() * dpr;
            if (scrollingTop >= 0)
                QVERIFY2(qAbs(top - scrollingTop) < .001,
                    qPrintable(QString("top row jumped from %1 to %2 physical px at row %3").arg(scrollingTop).arg(top).arg(index)));
            scrollingTop = top;
        }
        for (const auto &column : {QString("primary"), QString("path"), QString("date")}) {
            auto *leaf = visualItemWithObjectName(row, "semanticHistory-history-scroll-" + QString::number(index) + "-" + column);
            QVERIFY(leaf && leaf->isVisible());
            const auto origin = leaf->mapToItem(root, QPointF());
            for (const qreal coordinate : {origin.x(), origin.y(), row->mapToItem(root, QPointF()).y()}) {
                const qreal physical = coordinate * dpr;
                QVERIFY2(qAbs(physical - qRound64(physical)) < .001,
                    qPrintable(QString("row %1 %2: %3 physical px").arg(index).arg(column).arg(physical, 0, 'f', 6)));
            }
            QCOMPARE(leaf->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0));
            QCOMPARE(leaf->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1));
        }
        if (index == 158 && qEnvironmentVariableIsSet("F4_HISTORY_SCROLL_CAPTURE"))
            QVERIFY(capture.save(qEnvironmentVariable("F4_HISTORY_SCROLL_CAPTURE")));
    }
}

void F4QuickViewSurfaceTests::largeHistoryLatencyProfile()
{
    if (!qEnvironmentVariableIsSet("F4_HISTORY_PROFILE")) QSKIP("opt-in latency profile");
    for (const auto &kind : {QString("commands"), QString("files"), QString("folders")}) {
        const int count = kind == "commands" ? 2432 : kind == "files" ? 999 : 1294;
        QVariantList items;
        for (int i=0; i<count; ++i) {
            const auto path = QString("C:/Projects/folder-%1/long filename with spaces %1.txt").arg(i);
            QVariantMap details{{"kind","history"},{"columns",kind=="commands"?"command":"dated"},
                {"primary",kind=="commands"?"app --input "+path+" --check":path},
                {"path","C:/Projects"},{"date","2026-09-13 12:34:56"}};
            items.append(QVariantMap{{"index",i},{"details",details}});
        }
        QVariantMap menu{{"id","profile-history"},{"kind","menu"},{"role","vmenu"},
            {"title",kind+" history"},{"x",4},{"y",3},{"w",120},{"h",42},
            {"selected",count-1},{"top",count-39},{"viewHeight",38},{"items",items}};
        QuickViewFixture fixture(shellScene());
        QVERIFY(fixture.window);
        fixture.window->resize(1400,900);
        QTest::qWait(40);
        qInfo() << "HISTORY window" << fixture.window->isVisible() << fixture.window->isExposed()
                << fixture.window->size();
        QList<double> openTimes, pageTimes, applyTimes;
        QVariantList retainedMenus;
        const auto present = [&](const QVariantList &menus, QList<double> &samples, bool stateOnly = false) {
            QVariantList wireMenus = menus;
            if (stateOnly) {
                auto wireMenu = wireMenus.first().toMap();
                wireMenu.remove("items");
                wireMenus[0] = wireMenu;
            }
            const auto packet=QJsonDocument::fromVariant(wireMenus).toJson(QJsonDocument::Compact);
            QSignalSpy swapped(fixture.window,&QQuickWindow::frameSwapped);
            QElapsedTimer timer; timer.start();
            auto decoded = QJsonDocument::fromJson(packet).toVariant().toList();
            if (stateOnly) {
                // The protocol reducer validates the revision and shares this
                // accepted QVariantList. Its rejection paths have controller tests.
                auto header = decoded.first().toMap();
                header["items"] = retainedMenus.first().toMap().value("items");
                decoded[0] = header;
            }
            fixture.shell.applyCommandMenus(decoded,true);
            fixture.shell.deliverCommandMenuStates(fixture.shell.overlayState()->commandMenuStates());
            retainedMenus = decoded;
            const double apply=timer.nsecsElapsed()/1e6;
            fixture.window->requestUpdate();
            while(swapped.isEmpty() && timer.elapsed()<5000) {
                QCoreApplication::processEvents(QEventLoop::AllEvents,1);
                if(swapped.isEmpty()) QTest::qWait(1);
            }
            QVERIFY(!swapped.isEmpty());
            samples.append(timer.nsecsElapsed()/1e6);
            applyTimes.append(apply);
        };
        for(int i=0;i<5;++i) {
            present(QVariantList{menu},openTimes);
            if (QTest::currentTestFailed()) return;
            fixture.shell.applyCommandMenus({},true);
            QCoreApplication::processEvents();
        }
        fixture.shell.applyCommandMenus(QVariantList{menu},true);
        QTest::qWait(30);
        for(int i=1;i<=40;++i) {
            const int selected=count-1-(i*30)%(count-1);
            menu["selected"]=selected; menu["top"]=qMax(0,selected-10);
            present(QVariantList{menu},pageTimes,true);
            if (QTest::currentTestFailed()) return;
        }
        const auto report=[&](const QString &phase,QList<double> values) {
            std::sort(values.begin(),values.end());
            qInfo().noquote()<<QString("HISTORY %1 %2 p50=%3ms p95=%4ms")
                .arg(kind,phase).arg(values[values.size()/2],0,'f',3)
                .arg(values[qMin(values.size()-1,values.size()*95/100)],0,'f',3);
        };
        report("open",openTimes);report("page",pageTimes);report("apply",applyTimes);
    }
}

void F4QuickViewSurfaceTests::userMenuRecordDialogLeavesStaySharpAt175Percent()
{
    const QString path = qEnvironmentVariable("F4_USERMENU_EDITOR_SCENE");
    if (path.isEmpty()) QSKIP("Generate the Go editor scene with F4_USERMENU_EDITOR_SCENE first");
    QFile file(path);
    QVERIFY(file.open(QIODevice::ReadOnly));
    const auto node = QJsonDocument::fromJson(file.readAll()).toVariant().toMap();
    QVERIFY(!node.isEmpty());
    auto scene = shellScene();
    scene["dialogs"] = QVariantList{node};
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    fixture.window->resize(1300, 1100);
    QTest::qWait(150);
    auto *root = fixture.window->contentItem();
    auto *dialog = visualItemWithObjectName(root, "semanticDialog-id:settings-record-dialog");
    QVERIFY(dialog && dialog->isVisible());
    const qreal dpr = fixture.window->devicePixelRatio();
    QCOMPARE(dpr, 1.75);
    int leaves = 0;
    const auto inspect = [&](auto &&self, QQuickItem *item) -> void {
        if (item->isVisible() && (item->property("renderType").isValid() || item->inherits("QQuickImage"))) {
            ++leaves;
            const auto origin = item->mapToItem(root, QPointF{});
            const QString detail = QString("%1 physical=(%2,%3)").arg(item->objectName())
                .arg(origin.x()*dpr,0,'f',6).arg(origin.y()*dpr,0,'f',6);
            QVERIFY2(!item->objectName().isEmpty(), qPrintable(detail));
            QVERIFY2(qAbs(origin.x()*dpr-qRound(origin.x()*dpr))<.001, qPrintable(detail));
            QVERIFY2(qAbs(origin.y()*dpr-qRound(origin.y()*dpr))<.001, qPrintable(detail));
            QCOMPARE(item->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0));
            QCOMPARE(item->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1));
        }
        for (auto *child : item->childItems()) self(self,child);
    };
    inspect(inspect,dialog);
    QVERIFY(leaves >= 10);
    QVERIFY(fixture.window->grabWindow().save(path + ".png"));
}

void F4QuickViewSurfaceTests::far3ImportDialogLeavesStaySharpAt175Percent_data()
{
    QTest::addColumn<bool>("userMenu");
    QTest::newRow("history") << false;
    QTest::newRow("user-menu") << true;
}

void F4QuickViewSurfaceTests::far3ImportDialogLeavesStaySharpAt175Percent()
{
    QFETCH(bool, userMenu);
    auto scene = shellScene();
    scene.insert("dialogs", QVariantList{QVariantMap{{"id", "far3-import"}, {"kind", "dialog"},
        {"title", userMenu ? "Import Far3 user menu" : "Import Far3 history"}, {"x", 20}, {"y", 10}, {"w", 60}, {"h", 11},
        {"modal", true}, {"showClose", true}, {"children", QVariantList{
            QVariantMap{{"id","far3-label"},{"kind","text"},{"text",userMenu ? "FarMenu.ini or Far folder:" : "Far3 folder or history.db:"},{"x",22},{"y",12},{"w",25},{"h",1}},
            QVariantMap{{"id","far3-path"},{"kind","edit"},{"text",R"(C:\Programs\Far3)"},{"focused",true},{"x",22},{"y",14},{"w",56},{"h",1}},
            QVariantMap{{"id","far3-note"},{"kind","text"},{"text",userMenu ? "Import into the global menu draft." : "Merge histories; skip duplicates."},{"x",22},{"y",16},{"w",32},{"h",1}},
            QVariantMap{{"id","far3-import-button"},{"kind","button"},{"text","Import"},{"x",40},{"y",18},{"w",10},{"h",1}},
            QVariantMap{{"id","far3-cancel"},{"kind","button"},{"text","Cancel"},{"x",52},{"y",18},{"w",10},{"h",1}}
        }}}});
    if (userMenu) {
        auto dialogs = scene.value("dialogs").toList();
        auto dialog = dialogs[0].toMap();
        auto children = dialog.value("children").toList();
        children.append(QVariantMap{{"id","far3-detail"},{"kind","text"},
            {"text","Apply saves the imported entries."},{"x",22},{"y",17},{"w",34},{"h",1}});
        dialog["children"] = children;
        dialogs[0] = dialog;
        scene["dialogs"] = dialogs;
    }
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    const auto dpr = fixture.window->devicePixelRatio();
    QCOMPARE(dpr, 1.75);
    QStringList names{"semanticDialogTitle", "titleBarButtonIcon", "dialogWidget-far3-labelText",
        "dialogWidget-far3-noteText", "dialogWidget-far3-pathEditTextInput",
        "dialogWidget-far3-import-buttonButtonText", "dialogWidget-far3-cancelButtonText"};
    if (userMenu) names.append("dialogWidget-far3-detailText");
    for (const QSize size : {QSize(1101, 803), QSize(801, 601)}) {
        fixture.window->resize(size);
        QTest::qWait(60);
        auto *dialog = visualItemWithObjectName(root, "semanticDialog-far3-import");
        QVERIFY(dialog && dialog->isVisible());
        const auto bounds = dialog->mapRectToItem(root, dialog->boundingRect());
        for (const auto &name : names) {
            auto *scope = name == "titleBarButtonIcon"
                ? visualItemWithObjectName(dialog, "dialogCloseButton") : dialog;
            QVERIFY(scope);
            auto *leaf = visualItemWithObjectName(scope, name);
            QVERIFY2(leaf && leaf->isVisible(), qPrintable(name));
            const auto origin = leaf->mapToItem(root, QPointF());
            for (qreal coordinate : {origin.x(), origin.y()}) {
                const qreal physical = coordinate*dpr;
                QVERIFY2(qAbs(physical-qRound(physical)) < .001,
                    qPrintable(QStringLiteral("%1: %2 physical px").arg(name).arg(physical,0,'f',6)));
            }
            QCOMPARE(leaf->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0));
            QCOMPARE(leaf->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1));
            QVERIFY(bounds.contains(origin));
        }
    }
    const QImage capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    const QString path = qEnvironmentVariable("F4_FAR3_DIALOG_CAPTURE");
    if (!path.isEmpty()) QVERIFY(capture.save(path + (userMenu ? "-usermenu.png" : "-history.png")));
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

    dialog->resize(720, 720);
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
        QStringLiteral("themeNeutralFileTextPanel"),
        QStringLiteral("themeNeutralFileTextLabels"),
        QStringLiteral("themeNeutralFileTextTitle"),
        QStringLiteral("themeNeutralFileTextDescription"),
        QStringLiteral("themeNeutralFileTextCheckBox"),
        QStringLiteral("themeNeutralFileTextCheckBoxText"),
        QStringLiteral("themeNeutralFileTextIndicator"),
        QStringLiteral("themeNeutralFileTextCheckMark"),
        QStringLiteral("themeSelectionBorderPanel"),
        QStringLiteral("themeSelectionBorderLabels"),
        QStringLiteral("themeSelectionBorderTitle"),
        QStringLiteral("themeSelectionBorderDescription"),
        QStringLiteral("themeSelectionBorderCheckBox"),
        QStringLiteral("themeSelectionBorderCheckBoxText"),
        QStringLiteral("themeSelectionBorderIndicator"),
        QStringLiteral("themeSelectionBorderCheckMark"),
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
        QStringLiteral("themeChromaSlider"),
        QStringLiteral("themeChromaInputBox"),
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
        QStringLiteral("themeRestoreSavedButton"),
        QStringLiteral("themeSaveButton"),
        QStringLiteral("themeCloseButton"),
    };
    for (const QString &name : controlNames)
        verifyItem(dialog->findChild<QQuickItem *>(name), name);

    for (const QString &name : {
             QStringLiteral("themeNeutralFileTextCheckMark"),
             QStringLiteral("themeSelectionBorderCheckMark"),
         }) {
        QQuickItem *const mark = dialog->findChild<QQuickItem *>(name);
        QVERIFY(mark);
        const QUrl source = mark->property("source").toUrl();
        QVERIFY(source.isValid());
        QCOMPARE(source.fileName(), QStringLiteral("check.svg"));
        QCOMPARE(QUrlQuery(source).queryItemValue(QStringLiteral("size")),
                 QStringLiteral("12"));
        QCOMPARE(QUrlQuery(source).queryItemValue(QStringLiteral("dpr")),
                 QStringLiteral("1.75"));
        QCOMPARE(mark->property("opticalVerticalOffset").toReal() * dpr,
                 1.0);
        QCOMPARE(mark->property("sourceSize").toSize(), QSize(12, 12));
    }

    const QStringList optionVisualNames{
        QStringLiteral("themeFontRenderTypeLabels"),
        QStringLiteral("themeFontRenderTypeTitle"),
        QStringLiteral("themeFontRenderTypeDescription"),
        QStringLiteral("themeMouseWheelLabels"),
        QStringLiteral("themeMouseWheelTitle"),
        QStringLiteral("themeMouseWheelDescription"),
        QStringLiteral("themeNeutralFileTextLabels"),
        QStringLiteral("themeNeutralFileTextTitle"),
        QStringLiteral("themeNeutralFileTextDescription"),
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
        QStringLiteral("themeNeutralFileTextTitle"),
        QStringLiteral("themeNeutralFileTextDescription"),
        QStringLiteral("themeNeutralFileTextCheckBoxText"),
        QStringLiteral("themeSelectionBorderTitle"),
        QStringLiteral("themeSelectionBorderDescription"),
        QStringLiteral("themeSelectionBorderCheckBoxText"),
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
             QStringLiteral("themeNeutralFileTextDescription"),
             QStringLiteral("themeSelectionBorderDescription"),
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
        QStringLiteral("themeChromaRow"),
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
        QStringLiteral("themeFontRenderTypeComboPopupBackground"));
    QQuickItem *const popupList = dialog->findChild<QQuickItem *>(
        QStringLiteral("themeFontRenderTypeComboPopupList"));
    QVERIFY(popupBackground);
    QVERIFY(popupList);
    verifyItem(popupBackground, QStringLiteral("themeFontRenderTypeComboPopupBackground"));
    verifyItem(popupList, QStringLiteral("themeFontRenderTypeComboPopupList"));
    verifyWholePhysicalCoordinate(
        popupList->property("contentHeight").toReal(),
        QStringLiteral("themeFontRenderTypeComboPopupList content height"));

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

void F4QuickViewSurfaceTests::pointerActivationPreviewHandsOffBothPanelCursors()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);

    QQuickItem *const leftLoader = fixture.item(
        QStringLiteral("galleryPanelContent-0"));
    QQuickItem *const rightLoader = fixture.item(
        QStringLiteral("galleryPanelContent-1"));
    QVERIFY(leftLoader);
    QVERIFY(rightLoader);
    QTRY_VERIFY_WITH_TIMEOUT(leftLoader->property("item").value<QObject *>(),
                             3000);
    QTRY_VERIFY_WITH_TIMEOUT(rightLoader->property("item").value<QObject *>(),
                             3000);
    QObject *const leftHost = leftLoader->property("item").value<QObject *>();
    QObject *const rightHost = rightLoader->property("item").value<QObject *>();
    QVERIFY(leftHost->property("panelActive").toBool());
    QVERIFY(leftHost->property("showCursor").toBool());
    QVERIFY(!rightHost->property("panelActive").toBool());
    QVERIFY(!rightHost->property("showCursor").toBool());
    QCOMPARE(fixture.window->property(
                 "pointerPanelActivationOverride").toInt(), -1);

    // This signal is emitted inside the inactive panel's mouse-down stack.
    // Both cursor bindings must flip before it returns; waiting for another
    // event-loop turn would allow one frame containing two cursors.
    QVERIFY(QMetaObject::invokeMethod(rightHost,
                                      "beginPointerActivationPreview"));
    QCOMPARE(fixture.window->property(
                 "pointerPanelActivationOverride").toInt(), 1);
    QVERIFY(!leftHost->property("panelActive").toBool());
    QVERIFY(!leftHost->property("showCursor").toBool());
    QVERIFY(rightHost->property("panelActive").toBool());
    QVERIFY(rightHost->property("showCursor").toBool());
    QCOMPARE(fixture.shell.actions.size(), 0);

    // Authoritative compact activation replaces the preview atomically: the
    // override disappears without momentarily restoring the former cursor.
    fixture.shell.activatePanel(1, 1);
    QCOMPARE(fixture.window->property(
                 "pointerPanelActivationOverride").toInt(), -1);
    QVERIFY(!leftHost->property("showCursor").toBool());
    QVERIFY(rightHost->property("showCursor").toBool());
    QCOMPARE(fixture.shell.actions.size(), 0);
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

    const QVariantMap layoutState = {
        {QStringLiteral("id"), projectedPanel.value(QStringLiteral("id"))},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("catalogRevision"), qulonglong(9)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), qulonglong(3)},
        {QStringLiteral("galleryLayoutMode"), QStringLiteral("icons")},
        {QStringLiteral("galleryColumnCount"), 3},
        {QStringLiteral("galleryDensity"), 64},
        {QStringLiteral("galleryLayoutRevision"), qulonglong(2)},
        {QStringLiteral("galleryColumns"),
         projectedPanel.value(QStringLiteral("galleryColumns"))},
        {QStringLiteral("separateFileExtensions"), true},
    };
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelLayoutState"), layoutState},
    });

    // A renderer-only update must become visible in the same event-loop turn
    // without replacing the panel map that feeds path, footer, search and
    // selection bindings.
    QCOMPARE(leftHost->property("appliedPresentationMode").toString(),
             QStringLiteral("icons"));
    QVERIFY(!columnHeader->isVisible());
    QCOMPARE(leftHost->property("layoutState").toMap(), layoutState);
    QCOMPARE(leftHost->property("panel").toMap(), projectedPanel);
    QCOMPARE(leftPanel->property("panel").toMap(), projectedPanel);
    QCOMPARE(fixture.window->property(
                 "leftPanelPresentationOverride").toMap(), projectedPanel);
    QCOMPARE(fixture.window->property(
                 "leftPanelLayoutStateOverride").toMap(), layoutState);
    QCOMPARE(leftPanelChanged.size(), 1);
    QCOMPARE(rightPanelChanged.size(), 0);
    QCOMPARE(sceneChanged.size(), 0);

    QVariantMap densityDelta = {
        {QStringLiteral("id"), projectedPanel.value(QStringLiteral("id"))},
        {QStringLiteral("kind"), QStringLiteral("filePanel")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("catalogRevision"), qulonglong(9)},
        {QStringLiteral("metadataDeferred"), true},
        {QStringLiteral("metadataRevision"), qulonglong(3)},
        {QStringLiteral("galleryDensity"), 72},
        {QStringLiteral("galleryLayoutRevision"), qulonglong(3)},
    };
    fixture.shell.deliverCompactPresentation({
        {QStringLiteral("type"), QStringLiteral("scene_patch")},
        {QStringLiteral("side"), 0},
        {QStringLiteral("panelLayoutState"), densityDelta},
    });
    QVariantMap mergedLayoutState = layoutState;
    mergedLayoutState.insert(QStringLiteral("galleryDensity"), 72);
    mergedLayoutState.insert(QStringLiteral("galleryLayoutRevision"),
                             qulonglong(3));
    QCOMPARE(leftHost->property("appliedPresentationMode").toString(),
             QStringLiteral("icons"));
    QCOMPARE(leftHost->property("layoutState").toMap(), mergedLayoutState);
    QCOMPARE(fixture.window->property(
                 "leftPanelLayoutStateOverride").toMap(), mergedLayoutState);
    QCOMPARE(leftPanelChanged.size(), 1);
    QCOMPARE(rightPanelChanged.size(), 0);
    QCOMPARE(sceneChanged.size(), 0);

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
    QVERIFY(fixture.window->property(
                "leftPanelLayoutStateOverride").isNull());
    QVERIFY(fixture.window->property(
                "rightPanelLayoutStateOverride").isNull());
    QCOMPARE(fixture.item(QStringLiteral("filePanel-0")), leftPanel);
    QCOMPARE(fixture.item(QStringLiteral("filePanel-1")), rightPanel);
    QCOMPARE(leftLoader->property("item").value<QObject *>(), leftHost);
    QCOMPARE(rightLoader->property("item").value<QObject *>(), rightHost);
}

void F4QuickViewSurfaceTests::panelPathBarsToggleWithoutLosingContent()
{
    QVariantMap scene = shellScene();
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *headers[2];
    QQuickItem *contents[2];
    qreal initialY[2], initialHeight[2];
    for (int side=0; side<2; ++side) {
        headers[side] = fixture.item(QString("panelHeader-%1").arg(side));
        contents[side] = fixture.item(QString("galleryPanelContent-%1").arg(side));
        QVERIFY(headers[side]);
        QVERIFY(contents[side]);
        initialY[side] = contents[side]->y();
        initialHeight[side] = contents[side]->height();
    }
    for (bool hidden : {true, false, true, false}) {
        auto shell = scene.value("shell").toMap();
        shell["hidePanelPathBar"] = hidden;
        scene["shell"] = shell;
        fixture.shell.setScene(scene);
        QTest::qWait(40);
        for (int side=0; side<2; ++side) {
            QCOMPARE(headers[side]->isVisible(), !hidden);
            auto *expand = visualItemWithObjectNamePrefix(fixture.window->contentItem(), QString("panelExpandButton-%1").arg(side));
            QVERIFY(expand);
            QCOMPARE(expand->isVisible(), !hidden);
            QVERIFY(fixture.item(QString("galleryPanelContent-%1").arg(side)) == contents[side]);
            if (hidden) {
                QCOMPARE(headers[side]->height(), 0.0);
                QCOMPARE(contents[side]->y(), 0.0);
                QCOMPARE(contents[side]->height(), initialHeight[side]+initialY[side]);
            } else {
                QCOMPARE(contents[side]->y(), initialY[side]);
                QCOMPARE(contents[side]->height(), initialHeight[side]);
            }
        }
    }
}

void F4QuickViewSurfaceTests::panelSplitterCoalescesGoUpdates()
{
    QVariantMap scene = shellScene();
    auto shell = scene.value("shell").toMap();
    shell["id"] = "split-test";
    scene["shell"] = shell;
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *splitter = fixture.item("mainPanelSplitter");
    QVERIFY(splitter);
    fixture.shell.clearActions();
    for (int i=0; i<100; ++i)
        QVERIFY(QMetaObject::invokeMethod(splitter, "ratioRequested", Q_ARG(double, 0.4+i*0.001)));
    QCOMPARE(fixture.shell.actions.size(), 0);
    QTest::qWait(40);
    QCOMPARE(fixture.shell.actions.size(), 1);
    auto action = fixture.shell.actions.last();
    QCOMPARE(action.value("action").toString(), QString("panel.setSplit"));
    QCOMPARE(action.value("target").toString(), QString("split-test"));
    QCOMPARE(action.value("ratioMillionths").toInt(), 499000);
    fixture.shell.clearActions();
    const QPoint center = splitter->mapToScene(QPointF(splitter->width()/2+1, splitter->height()/2)).toPoint();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, center);
    QTest::mouseMove(fixture.window, center+QPoint(70,0));
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, center+QPoint(70,0));
    QVERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.last().value("ratioMillionths").toInt(),
        qRound(fixture.window->property("panelSplitRatio").toReal()*1000000));
    fixture.shell.clearActions();
    const QPoint moved = splitter->mapToScene(QPointF(splitter->width()/2+1, splitter->height()/2)).toPoint();
    QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier, moved);
    QTest::qWait(40);
    QVERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.last().value("ratioMillionths").toInt(), 500000);
    QVERIFY(fixture.shell.actions.size() <= 2);
}

void F4QuickViewSurfaceTests::panelExpandButtonsShareHoverAndRestore()
{
    QVariantMap scene = shellScene();
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *left = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "panelExpandButton-0");
    auto *right = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "panelExpandButton-1");
    auto *splitter = fixture.item("mainPanelSplitter");
    QVERIFY(left);
    QVERIFY(right);
    QVERIFY(splitter);
    auto center = [](QQuickItem *item) {
        return item->mapToScene(QPointF(item->width()/2, item->height()/2)).toPoint();
    };
    QTest::mouseMove(fixture.window, QPoint(40, 300));
    QTRY_COMPARE(left->property("revealed").toBool(), false);
    QCOMPARE(right->property("revealed").toBool(), false);
    for (auto *target : {left, right, splitter}) {
        QTest::mouseMove(fixture.window, center(target));
        QTRY_VERIFY2(left->property("revealed").toBool(), qPrintable(target->objectName()));
        QVERIFY(right->property("revealed").toBool());
    }
    const qreal dpr = fixture.window->devicePixelRatio();
    for (int side = 0; side < 2; ++side) {
        auto *button = side == 0 ? left : right;
        auto *view = fixture.item(QString("panelRendererButton-%1").arg(side));
        auto *icon = visualItemWithObjectNamePrefix(fixture.window->contentItem(), QString("panelExpandIcon-%1").arg(side));
        QVERIFY(view);
        QVERIFY(icon);
        const QPointF origin = button->mapToScene(QPointF());
        QCOMPARE(button->height(), view->height());
        QCOMPARE(button->property("iconName").toString(), side == 0
            ? QString("arrow-right-from-line") : QString("arrow-left-from-line"));
        auto *background = visualItemWithObjectNamePrefix(fixture.window->contentItem(), QString("panelExpandBackground-%1").arg(side));
        QVERIFY(background);
        QCOMPARE(background->property("radius").toReal(), 5.0);
        if (side == 0) {
            const QPointF edge = view->mapToScene(QPointF(view->width(), 0));
            QVERIFY(qAbs(edge.x() - origin.x()) < 0.01);
            QVERIFY(qAbs(origin.x() + button->width() - fixture.item("filePanel-1")->x()) < 0.01);
        } else {
            auto *drive = fixture.item("panelDriveButton-1");
            QVERIFY(drive);
            QCOMPARE(origin.x(), fixture.item("filePanel-1")->x());
            QVERIFY(qAbs(origin.x() + button->width() - drive->mapToScene(QPointF()).x()) < 0.01);
        }
        for (auto *item : {button, icon}) {
            const QPointF sceneOrigin = item->mapToScene(QPointF());
            const QPointF physical = sceneOrigin * dpr;
            qInfo() << item->objectName() << physical;
            QVERIFY(qAbs(physical.x()-qRound(physical.x())) < 0.01);
            QVERIFY(qAbs(physical.y()-qRound(physical.y())) < 0.01);
            QVERIFY(qAbs(item->width()*dpr-qRound(item->width()*dpr)) < 0.01);
            QVERIFY(qAbs(item->height()*dpr-qRound(item->height()*dpr)) < 0.01);
            QCOMPARE(item->mapToScene(QPointF(1,0))-sceneOrigin, QPointF(1,0));
            QCOMPARE(item->mapToScene(QPointF(0,1))-sceneOrigin, QPointF(0,1));
        }
        fixture.shell.clearActions();
        // Include the divider's hit lane, not just the center of the button.
        const QPoint hit = button->mapToScene(QPointF(side == 0 ? button->width()-1 : 1,
            button->height()/2)).toPoint();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, hit);
        QTRY_COMPARE(fixture.shell.actions.size(), 1);
        const auto action = fixture.shell.actions.first();
        QCOMPARE(action.value("action").toString(), QString("panel.setWide"));
        QCOMPARE(action.value("side").toInt(), side);
        QCOMPARE(action.value("enabled").toBool(), true);
        QVERIFY(!splitter->property("dragging").toBool());
    }
    QTest::mouseMove(fixture.window, center(right));
    QTest::qWait(750);
    auto *tip = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "panelExpandToolTipText-1");
    QVERIFY(tip);
    QVERIFY(tip->isVisible());
    const QPointF tipOrigin = tip->mapToScene(QPointF());
    qInfo() << tip->objectName() << tipOrigin * dpr;
    QVERIFY(qAbs(tipOrigin.x()*dpr-qRound(tipOrigin.x()*dpr)) < 0.01);
    QVERIFY(qAbs(tipOrigin.y()*dpr-qRound(tipOrigin.y()*dpr)) < 0.01);
    QCOMPARE(tip->mapToScene(QPointF(1,0))-tipOrigin, QPointF(1,0));
    QCOMPARE(tip->mapToScene(QPointF(0,1))-tipOrigin, QPointF(0,1));
    const auto capture = qEnvironmentVariable("F4_PANEL_EXPAND_CAPTURE");
    if (!capture.isEmpty())
        QVERIFY(fixture.window->grabWindow().save(capture));
    QVariantMap shell = scene.value("shell").toMap();
    shell["wide"] = true;
    shell["widePanel"] = 1;
    scene["shell"] = shell;
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!left->isVisible());
    QVERIFY(right->isVisible());
    QCOMPARE(right->property("iconName").toString(), QString("arrow-right-from-line"));
    QTRY_VERIFY(!splitter->isVisible());
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, center(right));
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.first().value("enabled").toBool(), false);
    shell["widePanel"] = 0;
    scene["shell"] = shell;
    fixture.shell.setScene(scene);
    QTRY_VERIFY(left->isVisible());
    QCOMPARE(left->property("iconName").toString(), QString("arrow-left-from-line"));
}

void F4QuickViewSurfaceTests::workspaceDragHitOnlyAcceptsPanelTabs()
{
    QVariantList tabs;
    const QStringList kinds = {"panels", "panels", "operationsQueue", "editor"};
    for (int i = 0; i < kinds.size(); ++i)
        tabs.append(QVariantMap{{"id", QString("workspace-tab-%1").arg(i)},
                                {"text", QString::number(i)}, {"surfaceKind", kinds[i]},
                                {"active", i == 0}, {"closable", false}});
    auto scene = shellScene({}, 0);
    scene.insert("workspaceTabs", QVariantMap{{"visible", true}, {"tabs", tabs}});
    QuickViewFixture fixture(scene, true);
    QVERIFY(fixture.window);
    fixture.window->resize(1800, 900);
    auto *bar = fixture.item("workspaceBar");
    QVERIFY(bar);
    QTRY_VERIFY(bar->width() > 0);
    for (int i = 0; i < kinds.size(); ++i) {
        auto *tab = visualItemWithObjectNamePrefix(fixture.window->contentItem(), QString("workspace-tab-%1").arg(i));
        if (kinds[i] == "operationsQueue") {
            // Native queues live in the title-bar dropdown, never a drag target tab.
            QVERIFY(!tab);
            continue;
        }
        QVERIFY(tab);
        const auto point = tab->mapToItem(bar, QPointF(tab->width()/2, tab->height()/2));
        QVariant result;
        QVERIFY(QMetaObject::invokeMethod(bar, "dragWorkspaceHit", Q_RETURN_ARG(QVariant, result),
                                         Q_ARG(QVariant, point.x()), Q_ARG(QVariant, point.y())));
        const auto hit = result.toMap();
        if (i < 2) {
            QCOMPARE(hit.value("target").toString(), QString("workspace-tab-%1").arg(i));
            QCOMPARE(hit.value("active").toBool(), i == 0);
        } else {
            QVERIFY(hit.isEmpty());
        }
    }
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
    // grabWindow() may return physical pixels with DPR metadata == 1 on
    // the software/offscreen backend. Derive the actual capture scale.
    const qreal scale = qreal(rendered.width()) / fixture.window->width();
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

void F4QuickViewSurfaceTests::workspaceCloseButtonHasTightHitAreaAndHoverFeedback()
{
    QVariantMap scene = shellScene();
    scene.insert("workspaceTabs", QVariantMap{{"visible", true}, {"tabs", QVariantList{
        QVariantMap{{"id", "workspace-tab-close-test"}, {"text", "Workspace"},
            {"index", 0}, {"active", true}, {"closable", true},
            {"action", "workspace.activate"}, {"closeAction", "workspace.close"}}
    }}});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *close = visualItemWithObjectNamePrefix(fixture.window->contentItem(),
                                                "workspace-close-workspace-tab-close-test");
    QVERIFY(close);
    QTRY_VERIFY(close->isVisible());
    QTest::qWait(100);
    const QPoint outside = close->mapToScene(QPointF(-3, close->height() / 2)).toPoint();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, outside);
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.first().value("action").toString(),
             QString("workspace.activate"));
    fixture.shell.clearActions();
    QTest::mouseMove(fixture.window, outside);
    QTest::qWait(50);
    const qreal dpr = fixture.window->devicePixelRatio();
    const QPointF origin = close->mapToScene(QPointF());
    const QRect crop(qRound(origin.x() * dpr), qRound(origin.y() * dpr),
                     qRound(close->width() * dpr), qRound(close->height() * dpr));
    const QImage normal = fixture.window->grabWindow().copy(crop);
    const QPoint center = close->mapToScene(QPointF(close->width() / 2,
                                                   close->height() / 2)).toPoint();
    QTest::mouseMove(fixture.window, center);
    QTest::qWait(50);
    const QImage hovered = fixture.window->grabWindow();
    QVERIFY2(hovered.copy(crop) != normal, "close button has no visible hover feedback");
    if (qAbs(dpr - 1.75) < .001) {
        for (qreal coordinate : {origin.x(), origin.y(), close->width(), close->height()})
            QVERIFY(qAbs(coordinate * dpr - qRound64(coordinate * dpr)) < .001);
        QCOMPARE(close->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(close->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
    }
    const QString capture = qEnvironmentVariable("F4_CLOSE_HOVER_CAPTURE");
    if (!capture.isEmpty())
        QVERIFY(hovered.save(capture));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, center);
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.first().value("action").toString(),
             QString("workspace.close"));
    auto tabModel = scene.value("workspaceTabs").toMap();
    auto tabs = tabModel.value("tabs").toList();
    auto tab = tabs.first().toMap();
    tab["active"] = false;
    tabs[0] = tab;
    tabModel["tabs"] = tabs;
    scene["workspaceTabs"] = tabModel;
    fixture.shell.setScene(scene);
    QTRY_VERIFY((close = visualItemWithObjectNamePrefix(fixture.window->contentItem(),
        "workspace-close-workspace-tab-close-test")) && !close->isVisible());
}

void F4QuickViewSurfaceTests::workspaceTabMiddleClickClosesClickedTab()
{
    QVariantList tabs;
    for (int index = 0; index < 3; ++index) {
        tabs.append(QVariantMap{{"id", QString("workspace-tab-%1").arg(index)},
            {"text", "Workspace"}, {"index", index}, {"active", index == 0},
            {"closable", index != 2}, {"action", "workspace.activate"},
            {"closeAction", "workspace.close"}});
    }
    QVariantMap scene = shellScene();
    scene.insert("workspaceTabs", QVariantMap{{"visible", true}, {"tabs", tabs}});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    for (int index = 0; index < 3; ++index) {
        auto *tab = visualItemWithObjectNamePrefix(fixture.window->contentItem(),
            QString("workspace-tab-%1").arg(index));
        QVERIFY(tab);
        QTRY_VERIFY(tab->isVisible());
        fixture.shell.clearActions();
        const QPoint point = tab->mapToScene(QPointF(tab->width() / 2,
                                                      tab->height() / 2)).toPoint();
        QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier, point);
        if (index == 2) {
            QTest::qWait(50);
            QCOMPARE(fixture.shell.actions.size(), 0);
            continue;
        }
        QTRY_COMPARE(fixture.shell.actions.size(), 1);
        QCOMPARE(fixture.shell.actions.first().value("action").toString(),
                 QString("workspace.close"));
        QCOMPARE(fixture.shell.actions.first().value("target").toString(),
                 QString("workspace-tab-%1").arg(index));
        QCOMPARE(fixture.shell.actions.first().value("index").toInt(), index);
        fixture.shell.clearActions();
        QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, point);
        QTRY_COMPARE(fixture.shell.actions.size(), 1);
        QCOMPARE(fixture.shell.actions.first().value("action").toString(),
                 QString("workspace.activate"));
        QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, point);
        QTest::qWait(30);
        QCOMPARE(fixture.shell.actions.size(), 1);
    }
}

void F4QuickViewSurfaceTests::workspaceTabWheelActivatesAdjacentTabs()
{
    const auto workspaceTabs = [](int activeIndex) {
        QVariantList tabs;
        for (int index = 0; index < 3; ++index) {
            tabs.append(QVariantMap{
                {QStringLiteral("id"),
                 QStringLiteral("workspace-tab-%1").arg(index + 1)},
                {QStringLiteral("text"),
                 QStringLiteral("Tab %1").arg(index + 1)},
                {QStringLiteral("index"), index},
                {QStringLiteral("active"), index == activeIndex},
                {QStringLiteral("action"),
                 QStringLiteral("workspace.activate")},
                {QStringLiteral("closable"), true},
            });
        }
        return QVariantMap{
            {QStringLiteral("visible"), true},
            {QStringLiteral("activeIndex"), activeIndex},
            {QStringLiteral("tabs"), tabs},
            {QStringLiteral("newTab"), QVariantMap{}},
            {QStringLiteral("counter"), QVariantMap{}},
        };
    };

    QVariantMap scene = shellScene();
    scene.insert(QStringLiteral("workspaceTabs"), workspaceTabs(1));
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *const workspaceBar = fixture.item(
        QStringLiteral("workspaceBar"));
    QVERIFY(workspaceBar);
    QTRY_VERIFY_WITH_TIMEOUT(workspaceBar->isVisible(), 3000);

    const QPointF origin = workspaceBar->mapToItem(
        fixture.window->contentItem(), QPointF{});
    const QPoint position = (origin + QPointF(workspaceBar->width() / 2,
                                               workspaceBar->height() / 2))
                                .toPoint();

    QTest::mouseMove(fixture.window, position);
    sendAngleWheel(fixture.window, position, 120);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 3000);
    QCOMPARE(fixture.shell.actions.at(0).value(QStringLiteral("action")),
             QVariant(QStringLiteral("workspace.activate")));
    QCOMPARE(fixture.shell.actions.at(0).value(QStringLiteral("target")),
             QVariant(QStringLiteral("workspace-tab-1")));
    QCOMPARE(fixture.shell.actions.at(0).value(QStringLiteral("index")),
             QVariant(0));

    // Simulate the authoritative response before checking the boundaries and
    // the opposite direction. Wheel navigation must not wrap around.
    fixture.shell.clearActions();
    QVariantMap nextScene = scene;
    nextScene.insert(QStringLiteral("workspaceTabs"), workspaceTabs(0));
    fixture.shell.setScene(nextScene);
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property("workspaceTabs").toMap().value(
            QStringLiteral("activeIndex")), QVariant(0), 3000);

    QTest::mouseMove(fixture.window, QPoint(0, 0));
    QTest::mouseMove(fixture.window, position);
    sendAngleWheel(fixture.window, position, 120);
    QTest::qWait(50);
    QCOMPARE(fixture.shell.actions.size(), 0);

    sendAngleWheel(fixture.window, position, -120);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 3000);
    QCOMPARE(fixture.shell.actions.at(0).value(QStringLiteral("target")),
             QVariant(QStringLiteral("workspace-tab-2")));
    QCOMPARE(fixture.shell.actions.at(0).value(QStringLiteral("index")),
             QVariant(1));

    fixture.shell.clearActions();
    nextScene.insert(QStringLiteral("workspaceTabs"), workspaceTabs(2));
    fixture.shell.setScene(nextScene);
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.window->property("workspaceTabs").toMap().value(
            QStringLiteral("activeIndex")), QVariant(2), 3000);
    QTest::mouseMove(fixture.window, QPoint(0, 0));
    QTest::mouseMove(fixture.window, position);
    sendAngleWheel(fixture.window, position, -120);
    QTest::qWait(50);
    QCOMPARE(fixture.shell.actions.size(), 0);
}

void F4QuickViewSurfaceTests::quickViewRetainsViewerAcrossPresentationChanges()
{
    QTemporaryDir directory;
    const QString imagePath = directory.filePath("docked.png");
    QImage image(QSize(2400, 1600), QImage::Format_ARGB32_Premultiplied);
    image.fill(Qt::cyan);
    QVERIFY(image.save(imagePath));
    QVariantMap dockView{{"id", "quick"}, {"side", 1}, {"sourceSide", 0},
        {"previewKind", "image"}, {"imageRenderer", "gallery"}, {"entryId", "image"},
        {"title", "Quick View"}, {"bottomHint", "legacy footer"}};
    QuickViewFixture fixture(shellScene({dockView}, 0), false, true);
    QVERIFY(fixture.window);
    QCOMPARE(fixture.window->devicePixelRatio(), qreal(1.75));
    auto *runtime = ZoinGallery::GalleryRuntime::install(&fixture.engine);
    auto *session = runtime->createExternalSession("docked-presentation");
    const auto shutdown = qScopeGuard([&] { runtime->shutdown(); });
    QVERIFY(session->applyExternalCatalog({QVariantMap{
        {"entryId", "image"}, {"index", 0}, {"name", "docked.png"},
        {"localPath", imagePath}, {"isDir", false}, {"isImage", true},
        {"size", QFileInfo(imagePath).size()}, {"mtimeNs", qint64(0)}
    }}, 1));
    QVERIFY(session->applyExternalState("image", 0, {}, 1));
    session->setViewerOpen(true);
    fixture.gallery.presentationState = 1;
    fixture.gallery.destinationSide = 1;
    fixture.gallery.quickView = {{"entryId", "image"}, {"active", false}};
    fixture.gallery.showViewer(QUrl("qrc:/F4QtHost/qml/GalleryViewerHost.qml"), session);
    QQuickItem *viewer = nullptr;
    QTRY_VERIFY((viewer = fixture.item("embeddedGalleryViewer")));
    auto *viewport = viewer->property("flickableArea").value<QQuickItem *>();
    QVERIFY(viewport);
    QTRY_VERIFY(viewport->property("imageTextureReady").toBool());
    QTRY_COMPARE(viewer->property("transitionProgress").toReal(), 1.0);
    auto *layer = fixture.item("galleryViewerLayer");
    QVERIFY(layer);
    QTRY_COMPARE(layer->property("fullProgress").toReal(), 0.0);
    auto *legacyTitle = fixture.item("quickViewTitle-1");
    QVERIFY(legacyTitle);
    QVERIFY(!legacyTitle->isVisible());
    QVERIFY(!fixture.item("quickViewFooterText-1")->isVisible());
    QCOMPARE(fixture.window->property("normalSurfaceOpacity").toReal(), 1.0);
    QVERIFY(!viewer->hasActiveFocus());
    const auto validateEndpoint = [&] {
        QCOMPARE(fixture.item("embeddedGalleryViewer"), viewer);
        QVERIFY(session->viewerOpen());
        QVERIFY(layer->isVisible());
        QVERIFY(viewer->isVisible());
        QVERIFY(viewer->width() > 100 && viewer->height() > 100);
        QCOMPARE(viewer->property("transitionProgress").toReal(), 1.0);
        QCOMPARE(viewer->property("viewerContentVisible").toBool(), true);
        const auto origin = layer->mapToItem(fixture.window->contentItem(), QPointF());
        for (qreal value : {origin.x(), origin.y(), layer->width(), layer->height()})
            QVERIFY(qAbs(value * 1.75 - qRound(value * 1.75)) < 0.001);
        QCOMPARE(layer->mapToItem(fixture.window->contentItem(), QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(layer->mapToItem(fixture.window->contentItem(), QPointF(0, 1)) - origin, QPointF(0, 1));
        // Include every visible text/image leaf, not just the clip wrapper.
        const auto inspect = [&](auto &&self, QQuickItem *item) -> void {
            if (item->isVisible() && (item->property("renderType").isValid() || item->inherits("QQuickImage"))) {
                const auto p = item->mapToItem(fixture.window->contentItem(), QPointF());
                const auto details = QString("%1 physical=(%2,%3)").arg(item->objectName()).arg(p.x()*1.75).arg(p.y()*1.75);
                QVERIFY2(!item->objectName().isEmpty(), qPrintable(details));
                QVERIFY2(qAbs(p.x()*1.75-qRound(p.x()*1.75))<0.001, qPrintable(details));
                QVERIFY2(qAbs(p.y()*1.75-qRound(p.y()*1.75))<0.001, qPrintable(details));
                QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(1,0))-p, QPointF(1,0));
                QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(0,1))-p, QPointF(0,1));
            }
            for (auto *child : item->childItems()) self(self, child);
        };
        inspect(inspect, viewer);
    };
    validateEndpoint();
    for (int side : {0, 1}) {
        dockView["side"] = side;
        dockView["sourceSide"] = 1 - side;
        fixture.shell.setScene(shellScene({dockView}, 1 - side));
        fixture.gallery.destinationSide = side;
        if (side == 1) viewport->setProperty("rotationMode", 1);
        QTRY_VERIFY(!viewport->property("isRotating").toBool());
        emit fixture.gallery.viewerChanged();
        QCoreApplication::processEvents();
        auto *splitter = fixture.item("mainPanelSplitter");
        QVERIFY(splitter);
        QVERIFY(splitter->isEnabled());
        // The trailing half of the gutter overlaps the right Quick View.
        // Exercise the real mouse grab, including movement across the viewer.
        const QPoint grab = splitter->mapToScene(QPointF(
            splitter->width() * 0.75, splitter->height() / 2)).toPoint();
        const qreal beforeRatio = fixture.window->property("panelSplitRatio").toReal();
        QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, grab);
        QVERIFY2(splitter->property("dragging").toBool(), "docked viewer intercepted the panel splitter press");
        QTest::mouseMove(fixture.window, grab + QPoint(70, 0));
        QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, grab + QPoint(70, 0));
        QTRY_VERIFY(fixture.window->property("panelSplitRatio").toReal() > beforeRatio + 0.03);
        QVERIFY(!splitter->property("dragging").toBool());
        for (const auto &name : {"panelSplitterTrack", "panelSplitterLine"}) {
            auto *leaf = visualItemWithObjectName(splitter, name);
            QVERIFY(leaf);
            const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
            for (const qreal physical : {origin.x()*1.75, origin.y()*1.75,
                                         leaf->width()*1.75, leaf->height()*1.75})
                QVERIFY2(qAbs(physical-qRound(physical)) < 0.001,
                         qPrintable(QString("%1 physical coordinate %2").arg(name).arg(physical)));
            QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
        }
        QVERIFY(fixture.window->grabWindow().save(QString(".diagnostics/quick-view-splitter-%1-175.png").arg(side)));
        QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
            splitter->mapToScene(QPointF(splitter->width() * 0.75, splitter->height()/2)).toPoint());
        QTRY_VERIFY(qAbs(fixture.window->property("panelSplitRatio").toReal()-0.5) < 0.001);
        validateEndpoint();
        QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
            viewport->mapToScene(QPointF(viewport->width()/2, viewport->height()/2)).toPoint());
        QTRY_COMPARE(fixture.gallery.presentationState, 3);
        QTRY_COMPARE(layer->property("fullProgress").toReal(), 1.0);
        validateEndpoint();
        // A custom absolute zoom survives both directions of the geometry change.
        QVERIFY(QMetaObject::invokeMethod(viewport, "zoomTo100", Q_ARG(QVariant, true)));
        QTRY_VERIFY(!viewport->property("viewportAnimationRunning").toBool());
        const qreal zoom = viewport->property("zoomScale").toReal();
        auto *imageItem = viewport->property("image").value<QQuickItem *>();
        QVERIFY(imageItem);
        const QPointF centerBefore((viewport->width()/2 - imageItem->x())/zoom,
                                   (viewport->height()/2 - imageItem->y())/zoom);
        QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
            viewport->mapToScene(QPointF(viewport->width()/2, viewport->height()/2)).toPoint());
        QTRY_COMPARE(fixture.gallery.presentationState, 1);
        QTRY_COMPARE(layer->property("fullProgress").toReal(), 0.0);
        QTRY_COMPARE(viewport->property("zoomScale").toReal(), zoom);
        const QPointF centerAfter((viewport->width()/2 - imageItem->x())/zoom,
                                  (viewport->height()/2 - imageItem->y())/zoom);
        const auto centerError = (centerAfter - centerBefore) * zoom * 1.75;
        qInfo() << "presentation physical center drift" << centerError;
        QVERIFY(qAbs(centerError.x()) <= 1.01 && qAbs(centerError.y()) <= 1.01);
        validateEndpoint();
        const QString capture = qEnvironmentVariable("F4_QUICKVIEW_TEST_CAPTURE");
        QImage rendered;
        if (fixture.window->rendererInterface()->graphicsApi() != QSGRendererInterface::Software)
            QTRY_VERIFY(imageContainsColor(rendered = fixture.window->grabWindow(), Qt::cyan));
        else
            rendered = fixture.window->grabWindow();
        if (!capture.isEmpty()) QVERIFY(rendered.save(capture + QString("-%1.png").arg(side)));
    }
    fixture.gallery.expandQuickView();
    QTest::qWait(40);
    fixture.gallery.collapseQuickView();
    QTRY_COMPARE(fixture.gallery.presentationState, 1);
    validateEndpoint();
    fixture.window->resize(937, 677);
    QTest::qWait(80);
    validateEndpoint();
    QVERIFY(session->applyExternalCatalog({QVariantMap{
        {"entryId", "missing"}, {"index", 0}, {"name", "missing.png"},
        {"localPath", directory.filePath("missing.png")}, {"isDir", false}, {"isImage", true}
    }}, 2));
    QVERIFY(session->applyExternalState("missing", 0, {}, 2));
    fixture.gallery.quickView["entryId"] = "missing";
    emit fixture.gallery.viewerChanged();
    auto *failure = fixture.item("galleryViewerLoadFailure");
    QVERIFY(failure);
    QTRY_VERIFY(failure->isVisible());
    QTest::qWait(300);
    validateEndpoint();
    const QString failureCapture = qEnvironmentVariable("F4_QUICKVIEW_TEST_CAPTURE");
    const auto failureFrame = fixture.window->grabWindow();
    if (fixture.window->rendererInterface()->graphicsApi() != QSGRendererInterface::Software)
        QVERIFY2(!imageContainsColor(failureFrame, Qt::cyan), "Failed preview retained the previous image");
    if (!failureCapture.isEmpty()) QVERIFY(failureFrame.save(failureCapture + "-error.png"));
}

void F4QuickViewSurfaceTests::cachedGalleryViewerCentersFirstNativeZoomAt175Percent()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QSize sourceSize(1396, 768);
    const QString imagePath = directory.filePath(QStringLiteral("cached-viewer.png"));
    QImage image(sourceSize, QImage::Format_ARGB32_Premultiplied);
    image.fill(Qt::cyan);
    QVERIFY(image.save(imagePath));

    QuickViewFixture fixture(shellScene(), false, true);
    QVERIFY(fixture.window);
    fixture.window->resize(2194, 1186);
    QCoreApplication::processEvents();
    const qreal dpr = fixture.window->devicePixelRatio();
    QCOMPARE(dpr, qreal(1.75));
    auto *layer = fixture.item(QStringLiteral("galleryViewerLayer"));
    QVERIFY(layer);
    QTRY_VERIFY(layer->width() > 2000 && layer->height() > 1000);

    auto *runtime = ZoinGallery::GalleryRuntime::install(&fixture.engine);
    QVERIFY(runtime);
    auto *session = runtime->createExternalSession(
        QStringLiteral("cached-host-viewer-dpr"));
    QVERIFY(session);
    const auto shutdown = qScopeGuard([&] { runtime->shutdown(); });
    QVERIFY(session->applyExternalCatalog({QVariantMap{
        {QStringLiteral("entryId"), QStringLiteral("cached-image")},
        {QStringLiteral("index"), 0},
        {QStringLiteral("name"), QStringLiteral("cached-viewer.png")},
        {QStringLiteral("localPath"), imagePath},
        {QStringLiteral("isDir"), false},
        {QStringLiteral("isImage"), true},
        {QStringLiteral("selected"), false},
        {QStringLiteral("size"), QFileInfo(imagePath).size()},
        {QStringLiteral("mtimeNs"), qint64(0)},
    }}, 1));
    QVERIFY(session->applyExternalState(QStringLiteral("cached-image"), 0, {}, 1));
    session->setViewerOpen(true);
    // Warm the exact fit request before the real shell Loader attaches the
    // session. Its synchronous tier publication must see the final host DPR.
    session->requestViewer(qCeil(layer->width() * dpr),
                           qCeil(layer->height() * dpr));
    QTRY_COMPARE_WITH_TIMEOUT(session->viewerSourceLevel(), 1, 5000);
    QTRY_COMPARE_WITH_TIMEOUT(session->imageOriginalSizeAt(0), sourceSize, 5000);
    const QUrl cachedFit = session->viewerSource();
    QVERIFY(!cachedFit.isEmpty());

    fixture.gallery.showViewer(
        QUrl(QStringLiteral("qrc:/F4QtHost/qml/GalleryViewerHost.qml")), session);
    QQuickItem *viewer = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT((viewer = fixture.item(
        QStringLiteral("embeddedGalleryViewer"))), 5000);
    QTRY_COMPARE_WITH_TIMEOUT(viewer->property("transitionProgress").toReal(), 1.0,
                             5000);
    QTRY_VERIFY_WITH_TIMEOUT(!viewer->property("transitioning").toBool(), 5000);
    auto *viewport = viewer->property("flickableArea").value<QQuickItem *>();
    QVERIFY(viewport);
    QTRY_VERIFY(viewport->property("imageTextureReady").toBool());
    QCOMPARE(viewport->property("devicePixelRatio").toReal(), dpr);
    QCOMPARE(session->viewerSource(), cachedFit);
    QCOMPARE(session->viewerSourceLevel(), 1);
    const QSizeF initialLogicalSize = viewport->property("originalSize").toSizeF();

    // Exercise the first actual '*' press; a second press would conceal the
    // stale logical dimensions by recalculating the already-loaded native tier.
    viewer->forceActiveFocus();
    QTest::keyClick(fixture.window, Qt::Key_Asterisk);
    QTRY_COMPARE_WITH_TIMEOUT(session->viewerSourceLevel(), 2, 5000);
    QTRY_VERIFY_WITH_TIMEOUT(!viewport->property("viewportAnimationRunning").toBool(),
                            5000);
    QTRY_COMPARE_WITH_TIMEOUT(viewport->property("zoomScale").toReal(), 1.0, 5000);
    auto *native = fixture.item(QStringLiteral("galleryViewerNativeImage"));
    auto *shader = fixture.item(QStringLiteral("galleryViewerImageShader"));
    QVERIFY(native);
    QVERIFY(shader);
    QTRY_COMPARE_WITH_TIMEOUT(native->property("status").toInt(), 1, 5000);
    QCoreApplication::processEvents();

    const QRectF imageRect = shader->mapRectToItem(viewport, shader->boundingRect());
    const QPointF center = imageRect.center();
    const QPointF expectedCenter(viewport->width() / 2, viewport->height() / 2);
    const QPointF centerError = (center - expectedCenter) * dpr;
    const QSizeF finalLogicalSize = viewport->property("originalSize").toSizeF();
    const QString details = QStringLiteral(
        "[FIX:viewer-first-native-center] dpr=%1 cachedLogical=%2x%3 "
        "nativeLogical=%4x%5 imageCenter=(%6,%7) viewportCenter=(%8,%9) "
        "physicalCenterError=(%10,%11)")
        .arg(dpr).arg(initialLogicalSize.width()).arg(initialLogicalSize.height())
        .arg(finalLogicalSize.width()).arg(finalLogicalSize.height())
        .arg(center.x()).arg(center.y()).arg(expectedCenter.x()).arg(expectedCenter.y())
        .arg(centerError.x()).arg(centerError.y());
    qInfo().noquote() << details;
    QVERIFY2(qAbs(centerError.x()) <= 0.501 && qAbs(centerError.y()) <= 0.501,
             qPrintable(details));
    QVERIFY2(qAbs(finalLogicalSize.width() * dpr - sourceSize.width()) < 0.001,
             qPrintable(details));
    QVERIFY2(qAbs(finalLogicalSize.height() * dpr - sourceSize.height()) < 0.001,
             qPrintable(details));
    QCOMPARE(initialLogicalSize, finalLogicalSize);
    QVERIFY2(qAbs(imageRect.width() * dpr - sourceSize.width()) < 0.001,
             qPrintable(details));
    QVERIFY2(qAbs(imageRect.height() * dpr - sourceSize.height()) < 0.001,
             qPrintable(details));
    const QPointF origin = shader->mapToItem(fixture.window->contentItem(), QPointF());
    for (qreal coordinate : {origin.x() * dpr, origin.y() * dpr})
        QVERIFY2(qAbs(coordinate - qRound(coordinate)) < 0.001, qPrintable(details));
    QCOMPARE(shader->mapToItem(fixture.window->contentItem(), QPointF(1, 0)) - origin,
             QPointF(1, 0));
    QCOMPARE(shader->mapToItem(fixture.window->contentItem(), QPointF(0, 1)) - origin,
             QPointF(0, 1));
}

void F4QuickViewSurfaceTests::expandedViewerKeepsWorkspaceChromeAccessible()
{
    QVariantMap scene = shellScene();
    scene.insert("workspaceTabs", QVariantMap{
        {"visible", true}, {"activeIndex", 0},
        {"tabs", QVariantList{
            QVariantMap{{"id", "workspace-tab-1"}, {"text", "Photos"},
                        {"index", 0}, {"action", "workspace.activate"},
                        {"number", 1}, {"surfaceKind", "panels"},
                        {"active", true}, {"closable", true}},
            QVariantMap{{"id", "workspace-tab-2"}, {"text", "Other"},
                        {"index", 1}, {"action", "workspace.activate"},
                        {"number", 2}, {"surfaceKind", "panels"},
                        {"active", false}, {"closable", true}}}},
        {"newTab", QVariantMap{}}, {"counter", QVariantMap{}}
    });
    QTemporaryDir directory;
    QFile viewerFile(directory.filePath("Viewer.qml"));
    QVERIFY(viewerFile.open(QIODevice::WriteOnly));
    viewerFile.write(R"QML(import QtQuick
Rectangle {
    color: "#e01080"
    property var session
    property var sourcePanel
    property var bridge
    property var keySink
    property var theme
    property bool surfaceActive
    property real devicePixelRatio
    property real surfaceProgress: 1
    property string tabTitle: "roof.jpg — 25%"
    MouseArea { anchors.fill: parent }
})QML");
    viewerFile.close();
    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    auto *originalTab = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "workspace-tab-1");
    QVERIFY(originalTab);
    const qreal folderWidth = originalTab->width();
    fixture.gallery.showViewer(QUrl::fromLocalFile(viewerFile.fileName()));
    QTRY_COMPARE(fixture.window->property("galleryViewerProgress").toReal(), 1.0);
    auto *title = visualItemWithObjectNamePrefix(fixture.window->contentItem(),
                                                "workspace-tab-title-workspace-tab-1");
    QVERIFY(title);
    QTRY_COMPARE(title->property("text").toString(), QString("roof.jpg — 25%"));
    // Switching from panels to the viewer must update both title and width
    // immediately, with no partially faded title or delayed width change.
    QCOMPARE(title->opacity(), 1.0);
    const qreal viewerWidth = originalTab->width();
    QVERIFY(viewerWidth > folderWidth);
    QTest::qWait(220);
    QCOMPARE(originalTab->width(), viewerWidth);
    QCOMPARE(title->opacity(), 1.0);
    const auto *closeLeaf = visualItemWithObjectNamePrefix(fixture.window->contentItem(),
                                                          "workspace-close-workspace-tab-1");
    QVERIFY(closeLeaf);
    const qreal closeInset = originalTab->width() - closeLeaf->x() - closeLeaf->width();
    QVERIFY(qAbs(closeInset - 8) <= 1 / fixture.window->devicePixelRatio());
    auto *outerLoader = fixture.item("galleryViewerLayer");
    auto *innerLoader = outerLoader->property("item").value<QObject *>();
    QVERIFY(innerLoader);
    auto *viewer = innerLoader->property("item").value<QObject *>();
    QVERIFY(viewer);
    viewer->setProperty("tabTitle", "next.jpg — 100%");
    QTRY_COMPARE(title->property("text").toString(), QString("next.jpg — 100%"));
    QCOMPARE(title->opacity(), 1.0);
    QTRY_VERIFY(visualItemWithText(fixture.window->contentItem(), "Other"));
    QTest::qWait(50);
    const auto origin = title->mapToItem(fixture.window->contentItem(), QPointF{});
    const qreal titleDpr = fixture.window->devicePixelRatio();
    for (qreal coordinate : {origin.x(), origin.y()})
        QVERIFY(qAbs(coordinate * titleDpr - qRound64(coordinate * titleDpr)) < .001);
    QCOMPARE(title->mapToItem(fixture.window->contentItem(), QPointF(1,0)) - origin, QPointF(1,0));
    QCOMPARE(title->mapToItem(fixture.window->contentItem(), QPointF(0,1)) - origin, QPointF(0,1));
    fixture.gallery.showViewer(QUrl{});
    QTRY_COMPARE(title->property("text").toString(), QString("Photos"));
    QCOMPARE(title->opacity(), 1.0);
    QCOMPARE(originalTab->width(), folderWidth);
    QTest::qWait(220);
    QCOMPARE(originalTab->width(), folderWidth);
    fixture.gallery.showViewer(QUrl::fromLocalFile(viewerFile.fileName()));
    QTRY_COMPARE(fixture.window->property("galleryViewerProgress").toReal(), 1.0);
    auto *layer = fixture.item("galleryViewerLayer");
    auto *tabs = fixture.item("workspaceBar");
    auto *second = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "workspace-tab-2");
    QVERIFY(layer && tabs && second);
    QVERIFY(QMetaObject::invokeMethod(tabs, "beginWorkspaceDrag"));
    QCOMPARE(tabs->property("dragSourceWorkspace").toString(), QString("workspace-tab-1"));
    auto *first = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "workspace-tab-1");
    QVERIFY(first);
    QVERIFY(!first->property("dragSourceHighlighted").toBool());
    // Simulate the active destination changing while preserving the source ID.
    auto tabModel = scene.value("workspaceTabs").toMap();
    auto models = tabModel.value("tabs").toList();
    auto a = models[0].toMap(); a["active"] = false; models[0] = a;
    auto b = models[1].toMap(); b["active"] = true; models[1] = b;
    tabModel["tabs"] = models; tabModel["activeIndex"] = 1;
    auto destinationScene = scene;
    destinationScene["workspaceTabs"] = tabModel;
    fixture.shell.setScene(destinationScene);
    QTRY_VERIFY((first = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "workspace-tab-1"))
                && first->property("dragSourceHighlighted").toBool());
    second = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "workspace-tab-2");
    QVERIFY(second);
    QCOMPARE(first->property("color").value<QColor>(), QColor("#245c38"));
    QVERIFY(!second->property("dragSourceHighlighted").toBool());
    auto *root = fixture.window->contentItem();
    const qreal dpr = fixture.window->devicePixelRatio();
    const QRectF viewerRect = layer->mapRectToItem(root, layer->boundingRect());
    QVERIFY(viewerRect.top() > 0);
    QVERIFY(layer->clip());
    const auto verifyLeaf = [root, dpr](QQuickItem *leaf) {
        QVERIFY(leaf);
        const QPointF origin = leaf->mapToItem(root, QPointF{});
        for (qreal coordinate : {origin.x(), origin.y()}) {
            const qreal physical = coordinate * dpr;
            QVERIFY2(qAbs(physical - qRound(physical)) < 0.001,
                     qPrintable(QStringLiteral("%1: %2 physical px")
                         .arg(leaf->objectName()).arg(physical, 0, 'f', 6)));
        }
        QCOMPARE(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1));
        QVERIFY(leaf->opacity() > 0.0);
        for (auto *item = leaf->parentItem(); item; item = item->parentItem())
            QCOMPARE(item->opacity(), 1.0);
    };
    for (const QString &id : {QStringLiteral("workspace-tab-1"),
                              QStringLiteral("workspace-tab-2")}) {
        verifyLeaf(visualItemWithObjectNamePrefix(root, "workspace-tab-title-" + id));
        verifyLeaf(visualItemWithObjectNamePrefix(root, "workspace-tab-number-" + id));
        verifyLeaf(visualItemWithObjectNamePrefix(root, "workspace-tab-icon-" + id));
    }
    const QImage rendered = fixture.window->grabWindow();
    QVERIFY(!rendered.isNull());
    QVERIFY(rendered.save(directory.filePath("viewer-chrome.png")));
    const QString capturePath = qEnvironmentVariable("F4_VIEWER_CHROME_CAPTURE");
    if (!capturePath.isEmpty())
        QVERIFY(rendered.save(capturePath));
    tabs->setProperty("dragSourceWorkspace", QString());
    QVERIFY(!first->property("dragSourceHighlighted").toBool());
    fixture.shell.setScene(scene);
    QTRY_VERIFY((second = visualItemWithObjectNamePrefix(root, "workspace-tab-2"))
                && !second->property("current").toBool());
    QTest::qWait(100); // settle the restored tab row before mouse hit testing
    const auto center = second->mapToItem(root, second->boundingRect().center());
    QVERIFY(center.y() < viewerRect.top());
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, center.toPoint());
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(),
             QStringLiteral("workspace.activate"));
    QCOMPARE(fixture.shell.actions.last().value("index").toInt(), 1);
    // Expanded windowed viewing retains tabs; actual fullscreen must occupy
    // the entire content area, and leaving fullscreen must restore the chrome.
    fixture.window->showFullScreen();
    QTRY_COMPARE(fixture.window->visibility(), QWindow::FullScreen);
    QTRY_VERIFY(!tabs->isVisible());
    QTRY_COMPARE(layer->mapToItem(root, QPointF()), QPointF());
    QTRY_COMPARE(layer->size(), QSizeF(qRound(root->width() * dpr) / dpr,
                                         qRound(root->height() * dpr) / dpr));
    qInfo() << "[FIX:gallery-fullscreen] viewer rect"
            << layer->mapRectToItem(root, layer->boundingRect())
            << "DPR" << dpr << "tabs visible" << tabs->isVisible();
    QTest::qWait(100);
    const auto fullscreenCapture = fixture.window->grabWindow();
    QVERIFY(!fullscreenCapture.isNull());
    QCOMPARE(fullscreenCapture.pixelColor(fullscreenCapture.width()/2, 0), QColor("#e01080"));
    QVERIFY(fullscreenCapture.save(QStringLiteral("/tmp/f4-gallery-fullscreen-175.png")));
    fixture.window->showNormal();
    QTRY_VERIFY(tabs->isVisible());
    QTRY_VERIFY(layer->mapToItem(root, QPointF()).y() > 0);
    QTest::qWait(100); // settle the restored row's deferred pixel correction
    for (const QString &id : {QStringLiteral("workspace-tab-1"),
                              QStringLiteral("workspace-tab-2")}) {
        verifyLeaf(visualItemWithObjectNamePrefix(root, "workspace-tab-title-" + id));
        verifyLeaf(visualItemWithObjectNamePrefix(root, "workspace-tab-number-" + id));
        verifyLeaf(visualItemWithObjectNamePrefix(root, "workspace-tab-icon-" + id));
    }
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

    auto *tabs = fixture.item("workspaceBar");
    auto *appIcon = fixture.item("appIconButton");
    auto *queue = fixture.item("operationsQueueButton");
    QVERIFY(tabs && appIcon && queue);
    int tabMouseAreas = 0;
    for (auto *child : tabs->findChildren<QObject *>()) {
        const QVariant cursor = child->property("cursorShape");
        if (!cursor.isValid())
            continue;
        QCOMPARE(cursor.toInt(), int(Qt::ArrowCursor));
        ++tabMouseAreas;
    }
    QVERIFY(tabMouseAreas >= 3); // tab body, close icon, and new-tab button
    const qreal leftInset = qMax(fixture.window->property("macTitleBarLeftPadding").toReal(),
                               appIcon->isVisible() ? appIcon->x() + appIcon->width() : 0.0);
    if (fixture.window->property("useMacNativeTitleBar").toBool()) {
        auto *area = fixture.window->property("macSystemButtonAreaItem").value<QQuickItem *>();
        QVERIFY(area);
        const int center = QRectF(area->mapToItem(rootItem, QPointF()), area->size()).toRect().center().x();
        const qreal tabLeft = tabs->mapToItem(rootItem, QPointF()).x();
        qInfo() << "[FIX:traffic-light-margins] tabs" << tabLeft << "symmetric edge" << 2 * center;
        QCOMPARE(tabLeft, qreal(2 * center));
    } else {
        QVERIFY(qAbs(tabs->x() - leftInset - fixture.window->property("contentSpacing").toReal()) < 1.0 / dpr);
    }
    QVERIFY(tabs->x() + tabs->width() <= queue->x());
    for (const auto &name : {"workspace-tab-title-workspace-tab-1", "workspace-tab-number-workspace-tab-1",
                            "workspace-tab-icon-workspace-tab-1", "workspace-close-workspace-tab-1", "workspaceNewIcon"}) {
        auto *leaf = visualItemWithObjectNamePrefix(rootItem, QString::fromLatin1(name));
        QVERIFY2(leaf && leaf->isVisible(), name);
        const auto origin = leaf->mapToItem(rootItem, QPointF());
        for (qreal coordinate : {origin.x(), origin.y()})
            QVERIFY2(qAbs(coordinate * dpr - qRound64(coordinate * dpr)) < .001, name);
        QCOMPARE(leaf->mapToItem(rootItem, QPointF(1,0)) - origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(rootItem, QPointF(0,1)) - origin, QPointF(0,1));
    }
    QVERIFY(fixture.window->grabWindow().save("/tmp/f4-left-tabs-175.png"));

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

void F4QuickViewSurfaceTests::worktreeBranchAppearsCenteredInTitleBar()
{
    QuickViewFixture fixture(shellScene(), false, true,
                             QStringLiteral("zoin-feature"));
    QVERIFY(fixture.window);
    QQuickItem *const rootItem = fixture.window->contentItem();
    QQuickItem *const titleBar = fixture.item(QStringLiteral("titleBar"));
    QQuickItem *const branchLabel = fixture.item(
        QStringLiteral("worktreeBranchLabel"));
    QVERIFY(titleBar);
    QVERIFY(branchLabel);
    QTRY_VERIFY_WITH_TIMEOUT(branchLabel->isVisible(), 3000);
    QCOMPARE(branchLabel->property("text").toString(),
             QStringLiteral("zoin-feature"));

    const qreal dpr = fixture.window->devicePixelRatio();
    const QPointF titleBarOrigin = titleBar->mapToItem(
        rootItem, QPointF{});
    const QPointF branchOrigin = branchLabel->mapToItem(
        rootItem, QPointF{});
    const qreal titleBarCenter = titleBarOrigin.x() + titleBar->width() / 2;
    const qreal branchCenter = branchOrigin.x() + branchLabel->width() / 2;
    QVERIFY2(qAbs(branchCenter - titleBarCenter) <= 0.5 / dpr + 0.001,
             "worktree branch label is not centered in the title bar");

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
    verifyWholePhysicalCoordinate(branchOrigin.x(),
                                  QStringLiteral("branch label scene x"));
    verifyWholePhysicalCoordinate(branchOrigin.y(),
                                  QStringLiteral("branch label scene y"));
    QVERIFY2(qAbs(branchLabel->scale() - 1.0) < 0.001,
             "worktree branch label must not be scaled");
    QVERIFY2(qAbs(branchLabel->rotation()) < 0.001,
             "worktree branch label must not be rotated");
    QCOMPARE(branchLabel->mapToItem(rootItem, QPointF(1, 0)) - branchOrigin, QPointF(1, 0));
    QCOMPARE(branchLabel->mapToItem(rootItem, QPointF(0, 1)) - branchOrigin, QPointF(0, 1));
    fixture.window->setProperty("worktreeBranchName", QStringLiteral("zoin"));
    QTRY_VERIFY(!branchLabel->isVisible());
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    capture.save("D:/Code/f4-zoin/.diagnostics/hidden-zoin-title-175.png");
    fixture.window->setProperty("worktreeBranchName", QStringLiteral("Zoin"));
    QTRY_VERIFY(branchLabel->isVisible());
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
    QQuickItem *const titleBarBackground = fixture.item(
        QStringLiteral("titleBarBackground"));
    QVERIFY(appButton);
    QVERIFY(minimizeButton);
    QVERIFY(maximizeButton);
    QVERIFY(closeButton);
    QVERIFY(titleBar);
    QVERIFY(titleBarBackground);
    const QColor themedTitleBarBackground(QStringLiteral("#123456"));
    QVERIFY(fixture.window->setProperty("titleBarBg",
                                        themedTitleBarBackground));
    QTRY_COMPARE_WITH_TIMEOUT(
        titleBarBackground->property("color").value<QColor>(),
        themedTitleBarBackground, 1000);
    QQuickItem *appIcon = nullptr;
    QQuickItem *minimizeIcon = nullptr;
    QQuickItem *maximizeIcon = nullptr;
    QQuickItem *closeIcon = nullptr;
    const bool customTitleBarIconsVisible = appButton->isVisible();
    QCOMPARE(minimizeButton->isVisible(), customTitleBarIconsVisible);
    QCOMPARE(maximizeButton->isVisible(), customTitleBarIconsVisible);
    QCOMPARE(closeButton->isVisible(), customTitleBarIconsVisible);
    if (customTitleBarIconsVisible) {
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
        const QColor darkChromeIconColor(QStringLiteral("#ffffff"));
        QCOMPARE(minimizeIcon->property("color").value<QColor>(),
                 darkChromeIconColor);
        QCOMPARE(maximizeIcon->property("color").value<QColor>(),
                 darkChromeIconColor);
        QCOMPARE(closeIcon->property("color").value<QColor>(),
                 darkChromeIconColor);
    }

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
    if (customTitleBarIconsVisible) {
        verifyWindowControlCenter(minimizeIcon,
                                  QStringLiteral("minimize icon"));
        verifyWindowControlCenter(maximizeIcon,
                                  QStringLiteral("maximize icon"));
        verifyWindowControlCenter(closeIcon, QStringLiteral("close icon"));
    }

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
    const QColor titleBarBackgroundColor = fixture.window->property(
        "titleBarBg").value<QColor>();
    const auto verifyDirectSvgPixels = [dpr, rootItem, &chromeFrame,
                                        titleBarBackgroundColor](
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
            titleBarBackgroundColor);
        QVERIFY2(!expected.isNull(), qPrintable(description));
        const QString difference = exactImageDifference(
            chromeFrame.copy(physicalRect), expected);
        QVERIFY2(difference.isEmpty(),
                 qPrintable(description + QStringLiteral(": ")
                            + difference));
    };
    if (customTitleBarIconsVisible) {
        verifyDirectSvgPixels(appIcon, QStringLiteral("application icon"));
        verifyDirectSvgPixels(minimizeIcon, QStringLiteral("minimize icon"));
        verifyDirectSvgPixels(maximizeIcon, QStringLiteral("maximize icon"));
        verifyDirectSvgPixels(closeIcon, QStringLiteral("close icon"));
    }

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
    QTest::qWait(100);
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
    QTest::qWait(100);
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

    // Side 0 is active in this fixture. Exercise the button on side 1 so a
    // parent panel-focus handler cannot consume the first click before the
    // semantic drive-menu action reaches Go.
    auto *const pathControl = fixture.item(
        QStringLiteral("panelPathTitle-1"));
    auto *const driveButton = fixture.item(
        QStringLiteral("panelDriveButton-1"));
    QVERIFY(pathControl);
    QVERIFY(driveButton);

    auto *const embeddedIcon = visualItemWithObjectNamePrefix(
        pathControl, QStringLiteral("pathDriveIcon"));
    auto *const buttonIcon = visualItemWithObjectNamePrefix(
        driveButton, QStringLiteral("panelDriveButtonIcon-1"));
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
    QCOMPARE(action.value(QStringLiteral("side")).toInt(), 1);
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
        {QStringLiteral("viewHeight"), 4},
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
             QVariantMap{{"index", 3}, {"text", "D: Data"}, {"details", QVariantMap{
                 {"isDrive", "true"}, {"name", "D:"}, {"label", "Data"}, {"filesystem", "NTFS"},
                 {"free", "152.4 MiB"}, {"total", "931.4 GiB"}, {"usedFraction", 0.99}}}},
         }},
    }});

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QFont menuFont("Segoe UI");
    menuFont.setPixelSize(14);
    fixture.window->setProperty("font", menuFont);
    QTest::qWait(100);
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
    const QUrl normalGlyphSource = normalIcon->property("source").toUrl();
    QVERIFY2(normalGlyphSource.isValid(),
             "menu icon must expose its actual pixel-aligned image source");
    QVERIFY2(!normalIcon->property("smooth").toBool(),
             "menu icon must not smooth-filter a DPR-sized texture");
    QVERIFY2(!normalIcon->property("mipmap").toBool(),
             "menu icon must not mipmap a DPR-sized texture");
    QCOMPARE(normalIcon->property("sourceSize").toSize(), QSize(15, 15));
    QCOMPARE(iconRowText->x(), plainRowText->x());
    auto *filesystem = visualItemWithObjectName(visualRoot, "semanticMenuDetail-drive-menu-3-filesystem");
    QVERIFY(filesystem);
    qInfo() << "Filesystem column" << filesystem->width() << filesystem->implicitWidth();
    QVERIFY2(filesystem->width() >= filesystem->implicitWidth(), "NTFS must fit without elision");
    const auto ink = QFontMetricsF(iconRowText->property("font").value<QFont>()).tightBoundingRect("Other panel");
    const qreal inkCenter = iconRowText->mapToScene(QPointF(0,
        iconRowText->property("baselineOffset").toReal() + ink.center().y())).y();
    const qreal rowCenter = selectedRow->mapToScene(QPointF(0, selectedRow->height() / 2)).y();
    qInfo() << "Menu text physical center" << inkCenter * fixture.window->devicePixelRatio()
            << "row center" << rowCenter * fixture.window->devicePixelRatio();
    QVERIFY2(qAbs(inkCenter - rowCenter) * fixture.window->devicePixelRatio() <= 1.0,
             "Menu label ink must be vertically centered with its icon");
    const qreal menuDpr = fixture.window->devicePixelRatio();
    for (const auto &name : QStringList{
             "semanticMenuItemText-drive-menu-0", "semanticMenuItemText-drive-menu-1",
             "semanticMenuItemText-drive-menu-2", "semanticMenuItemIcon-drive-menu-0",
             "semanticMenuItemIcon-drive-menu-1", "semanticMenuItemIcon-drive-menu-3",
             "semanticMenuDetail-drive-menu-3-name", "semanticMenuDetail-drive-menu-3-capacity",
             "semanticMenuDetail-drive-menu-3-filesystem"}) {
        auto *leaf = visualItemWithObjectName(visualRoot, name);
        QVERIFY(leaf);
        const QPointF origin = leaf->mapToItem(visualRoot, QPointF{});
        const auto detail = QString("%1 physical=(%2,%3)").arg(name)
            .arg(origin.x() * menuDpr, 0, 'f', 6).arg(origin.y() * menuDpr, 0, 'f', 6);
        QVERIFY2(qAbs(origin.x() * menuDpr - qRound(origin.x() * menuDpr)) < 0.001, qPrintable(detail));
        QVERIFY2(qAbs(origin.y() * menuDpr - qRound(origin.y() * menuDpr)) < 0.001, qPrintable(detail));
        QCOMPARE(leaf->mapToItem(visualRoot, QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(leaf->mapToItem(visualRoot, QPointF(0, 1)) - origin, QPointF(0, 1));
    }
    auto *capacity = visualItemWithObjectName(visualRoot, "semanticMenuDetail-drive-menu-3-capacity");
    QVERIFY(capacity);
    QVERIFY(capacity->width() >= capacity->implicitWidth());
    const qreal columnGap = filesystem->mapToScene({}).x()
        - capacity->mapToScene(QPointF(capacity->width(), 0)).x();
    QVERIFY(columnGap >= 11 && columnGap <= 13);
    QTest::qWait(50);
    const auto menuCapture = qEnvironmentVariable("F4_DRIVE_MENU_CAPTURE");
    if (!menuCapture.isEmpty()) QVERIFY(fixture.window->grabWindow().save(menuCapture));


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
    QTRY_COMPARE_WITH_TIMEOUT(
        QUrlQuery(normalIcon->property("source").toUrl())
            .queryItemValue(QStringLiteral("color")),
        themedText.name(QColor::HexArgb), 3000);

    const qreal dpr = fixture.window->devicePixelRatio();
    QCOMPARE(QUrlQuery(normalGlyphSource)
                 .queryItemValue(QStringLiteral("size")),
             QStringLiteral("15"));
    QCOMPARE(QUrlQuery(normalGlyphSource)
                 .queryItemValue(QStringLiteral("dpr")),
             QString::number(dpr, 'g', 12));
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

void F4QuickViewSurfaceTests::standaloneMenuTitleKeepsEmptyMenuAndActionsUsable()
{
    QVariantMap menu = titledUserMenu(QStringLiteral(" Main menu "));
    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"), QVariantList{menu});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *popup = nullptr;
    QQuickItem *title = nullptr;
    QQuickItem *hint = nullptr;
    QQuickItem *list = nullptr;
    QTRY_VERIFY((popup = visualItemWithObjectName(root, QStringLiteral("semanticMenuPopup-user-menu"))));
    QTRY_VERIFY((title = visualItemWithObjectName(root, QStringLiteral("semanticMenuTitle-user-menu"))));
    QTRY_VERIFY((hint = visualItemWithObjectName(root, QStringLiteral("semanticMenuBottomHint-user-menu"))));
    QTRY_VERIFY((list = visualItemWithObjectName(root, QStringLiteral("semanticMenuList-user-menu"))));
    QTRY_VERIFY(title->isVisible());
    QTRY_VERIFY(hint->isVisible());
    QCOMPARE(title->property("text").toString(), QStringLiteral("Main menu"));
    QCOMPARE(title->property("textFormat").toInt(), int(Qt::PlainText));
    QCOMPARE(list->property("count").toInt(), 0);
    const auto titleBottom = [&] {
        return title->mapToItem(popup, QPointF(0, title->height())).y();
    };
    QTRY_VERIFY(titleBottom() <= hint->mapToItem(popup, QPointF{}).y() + 0.01);
    QVERIFY(title->width() > 0 && title->height() > 0);
    QVERIFY(hint->mapToItem(popup, QPointF(0, hint->height())).y()
            <= popup->height() + 0.01);

    // Same-ID menu patches also carry the breadcrumb when F2 enters a level.
    menu = titledUserMenu(QStringLiteral(" Local menu -> Tools <docs> "), true);
    fixture.shell.setCommandMenus({menu});
    QTRY_COMPARE(title->property("text").toString(), QStringLiteral("Local menu -> Tools <docs>"));
    QTRY_COMPARE(list->property("count").toInt(), 2);
    QQuickItem *first = nullptr;
    QQuickItem *last = nullptr;
    QTRY_VERIFY((first = visualItemWithObjectName(root, QStringLiteral("semanticMenuItem-user-menu-0"))));
    QTRY_VERIFY((last = visualItemWithObjectName(root, QStringLiteral("semanticMenuItem-user-menu-1"))));
    QTRY_VERIFY(first->mapToItem(popup, QPointF{}).y() >= titleBottom() - 0.01);
    QVERIFY(last->mapToItem(popup, QPointF(0, last->height())).y()
            <= hint->mapToItem(popup, QPointF{}).y() + 0.01);
    const qreal titledHeight = popup->height();
    const qreal titledListY = list->y();

    fixture.shell.clearActions();
    const QPoint firstCenter = first->mapToScene(
        QPointF(first->width() / 2, first->height() / 2)).toPoint();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, firstCenter);
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(),
                           [](const QVariantMap &action) {
        return action.value(QStringLiteral("action")) == QStringLiteral("menu.activate")
            && action.value(QStringLiteral("target")) == QStringLiteral("user-menu")
            && action.value(QStringLiteral("index")).toInt() == 0;
    }));

    menu.insert(QStringLiteral("title"), QStringLiteral("   "));
    fixture.shell.setCommandMenus({menu});
    QTRY_VERIFY(!title->isVisible());
    QTRY_VERIFY(popup->height() < titledHeight);
    QCOMPARE(list->property("count").toInt(), 2);
    QTRY_VERIFY(list->y() < titledListY);
}

void F4QuickViewSurfaceTests::attachedMenusDoNotShowStandaloneTitles_data()
{
    QTest::addColumn<QVariantMap>("traits");
    QTest::newRow("menu-bar") << QVariantMap{
        {QStringLiteral("menuBarSubmenu"), true},
    };
    QTest::newRow("nested") << QVariantMap{
        {QStringLiteral("parentId"), QStringLiteral("parent-menu")},
        {QStringLiteral("anchorIndex"), 0},
    };
    QTest::newRow("dropdown") << QVariantMap{
        {QStringLiteral("presentation"), QStringLiteral("dropdown")},
        {QStringLiteral("ownerId"), QStringLiteral("menu-owner")},
    };
}

void F4QuickViewSurfaceTests::attachedMenusDoNotShowStandaloneTitles()
{
    QFETCH(QVariantMap, traits);
    QVariantMap menu = titledUserMenu(QString(), true);
    menu.remove(QStringLiteral("bottomHint"));
    for (auto it = traits.cbegin(); it != traits.cend(); ++it)
        menu.insert(it.key(), it.value());
    QVariantList menus;
    if (traits.contains(QStringLiteral("parentId"))) {
        QVariantMap parent = titledUserMenu(QString(), true);
        parent.insert(QStringLiteral("id"), QStringLiteral("parent-menu"));
        menus.append(parent);
    }
    menus.append(menu);
    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"), menus);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *popup = nullptr;
    QQuickItem *title = nullptr;
    QQuickItem *list = nullptr;
    QTRY_VERIFY((popup = visualItemWithObjectName(root, QStringLiteral("semanticMenuPopup-user-menu"))));
    QTRY_VERIFY((title = visualItemWithObjectName(root, QStringLiteral("semanticMenuTitle-user-menu"))));
    QTRY_VERIFY((list = visualItemWithObjectName(root, QStringLiteral("semanticMenuList-user-menu"))));
    if (traits.contains(QStringLiteral("presentation")))
        QTRY_VERIFY(popup->parentItem()->property("dropdownOpenSettled").toBool());
    const QSizeF popupSize = popup->size();
    const QRectF listGeometry(list->position(), list->size());

    menu.insert(QStringLiteral("title"), QStringLiteral("Must not add a heading"));
    menus.last() = menu;
    fixture.shell.setCommandMenus(menus);
    QTRY_COMPARE(title->property("text").toString(), QStringLiteral("Must not add a heading"));
    QVERIFY(!title->isVisible());
    QTRY_COMPARE(popup->size(), popupSize);
    QTRY_COMPARE(QRectF(list->position(), list->size()), listGeometry);
}

void F4QuickViewSurfaceTests::standaloneMenuTitleLeavesStaySharpAt175Percent()
{
    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"), QVariantList{titledUserMenu(QStringLiteral("Main menu"))});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) > 0.001)
        QSKIP("175% DPR invocation required for menu title and footer leaves");
    auto *root = fixture.window->contentItem();

    // Cover the reported empty F2 menu and the rows displaced by its title,
    // including secondary shortcut/footer text, in two live palettes.
    for (int state = 0; state < 2; ++state) {
        const QColor background(state ? QStringLiteral("#213546") : QStringLiteral("#654321"));
        const QColor foreground(state ? QStringLiteral("#f0d0a0") : QStringLiteral("#b8e2fa"));
        const QColor secondary(state ? QStringLiteral("#9ab8d6") : QStringLiteral("#d8b0e0"));
        QVERIFY(fixture.window->setProperty("dialogHeaderBg", background));
        QVERIFY(fixture.window->setProperty("textColor", foreground));
        QVERIFY(fixture.window->setProperty("mutedText", secondary));
        fixture.shell.setCommandMenus({titledUserMenu(
            state ? QStringLiteral("Local menu -> Tools") : QStringLiteral("Main menu"), state != 0)});
        fixture.window->resize(state ? 937 : 900, 640);
        QCoreApplication::processEvents();
        QStringList leafNames{
            QStringLiteral("semanticMenuTitle-user-menu"),
            QStringLiteral("semanticMenuBottomHint-user-menu"),
        };
        if (state) {
            leafNames.append({QStringLiteral("semanticMenuItemText-user-menu-0"),
                              QStringLiteral("semanticMenuItemText-user-menu-1"),
                              QStringLiteral("semanticMenuItemShortcut-user-menu-1")});
        }
        QList<QQuickItem *> leaves;
        for (const QString &name : leafNames) {
            QQuickItem *leaf = nullptr;
            QTRY_VERIFY((leaf = visualItemWithObjectName(root, name)));
            QTRY_VERIFY(leaf->isVisible());
            QTRY_VERIFY(leaf->width() > 0 && leaf->height() > 0);
            QTRY_VERIFY(qAbs(leaf->mapToItem(root, QPointF{}).x() * dpr
                             - qRound(leaf->mapToItem(root, QPointF{}).x() * dpr)) < 0.001);
            QTRY_VERIFY(qAbs(leaf->mapToItem(root, QPointF{}).y() * dpr
                             - qRound(leaf->mapToItem(root, QPointF{}).y() * dpr)) < 0.001);
            const QPointF origin = leaf->mapToItem(root, QPointF{});
            const QPointF ux = leaf->mapToItem(root, QPointF(1, 0)) - origin;
            const QPointF uy = leaf->mapToItem(root, QPointF(0, 1)) - origin;
            QVERIFY((ux - QPointF(1, 0)).manhattanLength() < 0.001);
            QVERIFY((uy - QPointF(0, 1)).manhattanLength() < 0.001);
            if (name.contains(QStringLiteral("Title"))
                || name.contains(QStringLiteral("BottomHint"))) {
                QVERIFY(qAbs(leaf->width() * dpr - qRound(leaf->width() * dpr)) < 0.001);
                QVERIFY(qAbs(leaf->height() * dpr - qRound(leaf->height() * dpr)) < 0.001);
            }
            QCOMPARE(leaf->property("renderType").toInt(), int(QQuickWindow::NativeTextRendering));
            qInfo().noquote() << QStringLiteral("[FIX:menu-title] %1 physical=(%2,%3) DPR=%4")
                .arg(name).arg(origin.x() * dpr).arg(origin.y() * dpr).arg(dpr);
            leaves.append(leaf);
        }
        fixture.window->requestUpdate();
        QImage frame;
        QTRY_VERIFY_WITH_TIMEOUT(!(frame = fixture.window->grabWindow()).isNull(), 3000);
        for (QQuickItem *leaf : leaves) {
            const QPointF origin = leaf->mapToItem(root, QPointF{});
            const QRect crop(qRound(origin.x() * dpr), qRound(origin.y() * dpr),
                             qRound(leaf->width() * dpr), qRound(leaf->height() * dpr));
            QVERIFY(frame.rect().contains(crop));
            const QImage renderedLeaf = frame.copy(crop);
            const QColor expected = leaf->objectName().contains(QStringLiteral("BottomHint"))
                || leaf->objectName().contains(QStringLiteral("Shortcut")) ? secondary : foreground;
            QVERIFY2(imageContainsColor(renderedLeaf, expected), qPrintable(leaf->objectName()));
            if (leaf->objectName().contains(QStringLiteral("Title")))
                QVERIFY(imageContainsColor(renderedLeaf, background));
        }
        const QString capture = qEnvironmentVariable("F4_MENU_TITLE_TEST_CAPTURE");
        if (!capture.isEmpty())
            QVERIFY(frame.save(capture + QStringLiteral("-%1.png").arg(state)));
    }
}

void F4QuickViewSurfaceTests::historyFirstVisibleFrameHasFinalPosition()
{
    QuickViewFixture fixture(shellScene({}, 0));
    QVERIFY(fixture.window);
    QTest::qWait(50);
    auto *root = fixture.window->contentItem();
    QVariantList items;
    for (int i = 0; i < 100; ++i)
        items.append(QVariantMap{{"index", i}, {"text", QString("entry %1").arg(i)},
            {"details", QVariantMap{{"kind", "history"}, {"primary", QString("entry %1").arg(i)}}}});
    QVariantMap menu{{"id", "history-first"}, {"kind", "menu"}, {"role", "vmenu"},
        {"title", "History"}, {"x", 4}, {"y", 1}, {"w", 70}, {"h", 28},
        {"selected", 99}, {"top", 70}, {"viewHeight", 30}, {"items", items}};
    int frames = 0;
    QStringList errors;
    qreal firstOffset = 0;
    const auto inspectVisiblePosition = [&] {
        auto *list = visualItemWithObjectName(root, "semanticMenuList-history-first");
        if (!list || !list->isVisible() || list->opacity() == 0)
            return;
        const auto offset = list->property("contentY").toReal();
        if (frames++ == 0)
            firstOffset = offset;
        if (qAbs(offset - firstOffset) > .01)
            errors.append(QString("visible offset changed: %1 -> %2").arg(firstOffset).arg(offset));
        auto *last = visualItemWithObjectName(root, "semanticMenuItem-history-first-99");
        if (!last) {
            errors.append("selected row missing from visible frame");
            return;
        }
        const auto rect = last->mapRectToItem(list, QRectF(0, 0, last->width(), last->height()));
        if (rect.top() < -.01 || rect.bottom() > list->height() + .01)
            errors.append(QString("selected row outside first viewport: %1 / %2").arg(rect.bottom()).arg(list->height()));
    };
    const auto connection = QObject::connect(fixture.window, &QQuickWindow::afterAnimating, fixture.window, inspectVisiblePosition);
    fixture.shell.setCommandMenus({menu});
    inspectVisiblePosition();
    QTest::qWait(50);
    // The full semantic scene can follow the direct menu-open publication.
    fixture.shell.setScene([&] {
        auto scene = shellScene({}, 0);
        scene.insert("menus", QVariantList{menu});
        return scene;
    }());
    QTest::qWait(200);
    QObject::disconnect(connection);
    QVERIFY(frames > 0);
    qInfo() << "[FIX:history-first-frame]" << frames << "frames" << errors;
    QVERIFY2(errors.isEmpty(), qPrintable(errors.join('\n')));
}

void F4QuickViewSurfaceTests::historyLastRowFitsNativeViewport_data()
{
    QTest::addColumn<QString>("columns");
    QTest::addColumn<bool>("highlighted");
    QTest::newRow("command") << QString("command") << true;
    QTest::newRow("viewer-editor") << QString("dated") << true;
    QTest::newRow("command-plain") << QString("command") << false;
    QTest::newRow("folders-plain") << QString("dated") << false;
}

void F4QuickViewSurfaceTests::historyLastRowFitsNativeViewport()
{
    QFETCH(QString, columns);
    QFETCH(bool, highlighted);
    auto scene = shellScene({}, 0);
    QVariantList items;
    for (int i = 0; i < 100; ++i)
        items.append(QVariantMap{{"index", i},
            {"details", QVariantMap{{"kind", "history"}, {"columns", columns},
                {"primary", QString("echo <item> %1").arg(i)}, {"path", "/work/example"},
                {"date", "2026-09-13 12:34:56"}, {"primaryMatches", highlighted ? "1111" : ""}}}});
    QVariantMap menu{{"id", "history-fit"}, {"kind", "menu"}, {"role", "vmenu"},
        {"presentation", columns == "command" ? "fullWidth" : "window"},
        {"title", "Commands History"}, {"x", 4}, {"y", 1}, {"w", 70}, {"h", 28},
        {"bottomHint", "Enter Esc Ins Ctrl+T F3 Ctrl+F10 Ctrl+Left/Right F2 Ctrl+F2 Ctrl+Shift+Enter Ctrl+PgDn Shift+Del Del Ctrl+C/Ins"},
        {"selected", 99}, {"top", 70}, {"viewHeight", 30}, {"items", items}};
    scene.insert("menus", QVariantList{menu});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
#ifdef Q_OS_WIN
    // Monaco is the fixture's macOS default; use an installed Windows face.
    fixture.engine.rootContext()->setContextProperty("f4GuiFontFamily", "Consolas");
#endif
    fixture.window->resize(1000, 640);
    auto *root = fixture.window->contentItem();
    QQuickItem *list = nullptr;
    QTRY_VERIFY((list = visualItemWithObjectName(root, "semanticMenuList-history-fit")));
    QTest::qWait(100);
    if (columns == "command") {
        auto *popup = visualItemWithObjectName(root, "semanticMenuPopup-history-fit");
        QVERIFY(popup);
        QCOMPARE(popup->mapToItem(root, QPointF()).x(), 0.0);
        QCOMPARE(popup->width(), qreal(fixture.window->width()));
    }
    auto *hint = visualItemWithObjectName(root, "semanticMenuBottomHint-history-fit");
    auto *scrollBar = visualItemWithObjectName(root, "semanticMenuScrollBar-history-fit");
    QVERIFY(scrollBar && scrollBar->isVisible());
    QCOMPARE(scrollBar->property("thickness").toReal(), 16.0);
    auto *handle = visualItemWithObjectName(root, "semanticMenuScrollBar-history-fitHandle");
    QVERIFY(handle && handle->isVisible());
    const auto handlePhysical = handle->mapToItem(root, QPointF()) * fixture.window->devicePixelRatio();
    QVERIFY(qAbs(handlePhysical.x() - qRound64(handlePhysical.x())) < .02);
    QVERIFY(qAbs(handlePhysical.y() - qRound64(handlePhysical.y())) < .02);
    QVERIFY(hint && hint->isVisible());
    QCOMPARE(hint->property("text").toString(), menu.value("bottomHint").toString());
    const auto hintOrigin = hint->mapToItem(root, QPointF());
    const auto hintPhysical = hintOrigin * fixture.window->devicePixelRatio();
    QVERIFY(qAbs(hintPhysical.x() - qRound64(hintPhysical.x())) < .02);
    QVERIFY(qAbs(hintPhysical.y() - qRound64(hintPhysical.y())) < .02);
    QVERIFY(QLineF(hint->mapToItem(root, QPointF(1,0))-hintOrigin, QPointF(1,0)).length() < .0001);
    QVERIFY(QLineF(hint->mapToItem(root, QPointF(0,1))-hintOrigin, QPointF(0,1)).length() < .0001);
    QVERIFY(hintOrigin.y() >= list->mapToItem(root, QPointF(0,list->height())).y());
    QQuickItem *last = nullptr;
    QTRY_VERIFY((last = visualItemWithObjectName(root, "semanticMenuItem-history-fit-99")));
    const auto rowRect = last->mapRectToItem(list, QRectF(0, 0, last->width(), last->height()));
    qInfo() << "[FIX:history-viewport] last row" << rowRect << "viewport" << list->height();
    QVERIFY2(rowRect.top() >= -.01 && rowRect.bottom() <= list->height()+.01,
        qPrintable(QString("last bottom=%1 viewport=%2").arg(rowRect.bottom()).arg(list->height())));
    auto *text = visualItemWithObjectName(root, "semanticHistory-history-fit-99-primary");
    QVERIFY(text);
    const QPointF origin = text->mapToItem(root, QPointF());
    const QPointF physical = origin * fixture.window->devicePixelRatio();
    QVERIFY(qAbs(physical.x()-qRound64(physical.x())) < .02);
    QVERIFY(qAbs(physical.y()-qRound64(physical.y())) < .02);
    QVERIFY(QLineF(text->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0)).length() < .0001);
    QVERIFY(QLineF(text->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1)).length() < .0001);
    const bool styled = highlighted || columns == "command";
    QCOMPARE(text->property("text").toString().contains("<font color="), styled);
    QVERIFY(text->property("text").toString().contains(styled ? "&lt;item&gt;" : "<item>"));
    QCOMPARE(text->property("text").toString().contains("#75d977"), columns == "command" && !highlighted);
    qreal previousRight = text->mapToItem(root, QPointF(text->width(),0)).x();
    for (const auto &column : {QString("path"), QString("date")}) {
        auto *leaf = visualItemWithObjectName(root, "semanticHistory-history-fit-99-" + column);
        if (column == "path" && columns == "dated") {
            QVERIFY(!leaf);
            continue;
        }
        QVERIFY(leaf && leaf->isVisible());
        if (column == "date") {
            const QFontMetricsF metrics(leaf->property("font").value<QFont>());
            QVERIFY(qAbs(metrics.horizontalAdvance("i") - metrics.horizontalAdvance("W")) < .01);
        }
        const auto p = leaf->mapToItem(root, QPointF());
        QVERIFY(p.x() > previousRight);
        previousRight = p.x() + leaf->width();
        QVERIFY(previousRight <= scrollBar->mapToItem(root, QPointF()).x());
        const auto physical = p * fixture.window->devicePixelRatio();
        QVERIFY(qAbs(physical.x()-qRound64(physical.x())) < .02);
        QVERIFY(qAbs(physical.y()-qRound64(physical.y())) < .02);
        QVERIFY(QLineF(leaf->mapToItem(root,QPointF(1,0))-p,QPointF(1,0)).length() < .0001);
        QVERIFY(QLineF(leaf->mapToItem(root,QPointF(0,1))-p,QPointF(0,1)).length() < .0001);
        QVERIFY(leaf->property("color") != text->property("color"));
    }
    if (qEnvironmentVariableIsSet("F4_HISTORY_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_HISTORY_CAPTURE") + "-" + columns + ".png"));
    // A scroll acknowledgement keeps the cursor unchanged, intentionally off
    // screen. A subsequent keyboard selection must become visible again.
    const qreal bottomOffset = list->property("contentY").toReal();
    fixture.shell.deliverCommandMenuStates({QVariantMap{{"id", "history-fit"}, {"selected",99}, {"top",70}}});
    QTest::qWait(30);
    QCOMPARE(list->property("contentY").toReal(), bottomOffset);
    for (int index = 98; index >= 94; --index) {
        fixture.shell.deliverCommandMenuStates({QVariantMap{{"id", "history-fit"}, {"selected",index}, {"top",70}}});
        QTest::qWait(30);
        qInfo() << "[FIX:history-keyboard-scroll] selected" << index
                << "offset" << list->property("contentY").toReal() << "expected" << bottomOffset;
        QCOMPARE(list->property("contentY").toReal(), bottomOffset);
    }
    fixture.shell.deliverCommandMenuStates({QVariantMap{{"id", "history-fit"}, {"selected",99}, {"top",70}}});
    QTest::qWait(30);
    QCOMPARE(list->property("contentY").toReal(), bottomOffset);
    fixture.shell.deliverCommandMenuStates({QVariantMap{{"id", "history-fit"}, {"selected",99}, {"top",20}}});
    QTRY_VERIFY(list->property("contentY").toReal() < bottomOffset - 100);
    fixture.shell.deliverCommandMenuStates({QVariantMap{{"id", "history-fit"}, {"selected",98}, {"top",69}}});
    QTest::qWait(50);
    auto *selected = visualItemWithObjectName(root, "semanticMenuItem-history-fit-98");
    QVERIFY(selected);
    const auto selectedRect = selected->mapRectToItem(list, QRectF(0,0,selected->width(),selected->height()));
    QVERIFY2(selectedRect.top() >= -.01 && selectedRect.bottom() <= list->height()+.01,
        qPrintable(QString("selection bottom=%1 viewport=%2 contentY=%3").arg(selectedRect.bottom()).arg(list->height()).arg(list->property("contentY").toReal())));
    const qreal hoverOffset = list->property("contentY").toReal();
    auto *overlay = list->parentItem()->parentItem();
    auto *hoveredRow = visualItemWithObjectName(root, "semanticMenuItem-history-fit-90");
    QVERIFY(hoveredRow);
    const QPoint pointer = hoveredRow->mapToItem(root, QPointF(80,hoveredRow->height()/2)).toPoint();
    moveNativePointer(fixture.window, pointer, 10);
    moveNativePointer(fixture.window, pointer + QPoint(5,0), 10);
    QTRY_COMPARE(overlay->property("pointerSelectedIndex").toInt(), 90);
    QTest::qWait(20);
    QCOMPARE(list->property("contentY").toReal(), hoverOffset);
    fixture.shell.deliverCommandMenuStates({QVariantMap{{"id","history-fit"},{"selected",90},{"top",69}}});
    QTest::qWait(30);
    QCOMPARE(list->property("contentY").toReal(), hoverOffset);

    auto *dialog = visualItemWithObjectName(root, "semanticDialog-history-fit");
    QVERIFY(dialog);
    auto *maximize = visualItemWithObjectName(dialog, "dialogMaximizeButton");
    auto *close = visualItemWithObjectName(dialog, "dialogCloseButton");
    QVERIFY(maximize && maximize->isVisible() && close && close->isVisible());
    const auto checkChrome = [&]() {
        for (auto *leaf : {visualItemWithObjectName(dialog, "semanticDialogTitle"),
                          visualItemWithObjectName(maximize, "titleBarButtonIcon"),
                          visualItemWithObjectName(close, "titleBarButtonIcon")}) {
            QVERIFY(leaf && leaf->isVisible());
            const auto origin = leaf->mapToItem(root, QPointF());
            for (qreal value : {origin.x(), origin.y()}) {
                const auto physical = value * fixture.window->devicePixelRatio();
                QVERIFY2(qAbs(physical-qRound64(physical)) < .02,
                    qPrintable(QString("%1: %2 physical px").arg(leaf->objectName()).arg(physical,0,'f',6)));
            }
            QCOMPARE(leaf->mapToItem(root,QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToItem(root,QPointF(0,1))-origin, QPointF(0,1));
        }
    };
    checkChrome();
    const auto clickControl = [&](QQuickItem *item) {
        const auto point = item->mapToItem(root, QPointF(item->width()/2,item->height()/2)).toPoint();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, point);
    };
    const QSizeF restored(dialog->width(), dialog->height());
    clickControl(maximize);
    QTRY_VERIFY(dialog->property("maximized").toBool());
    QTest::qWait(30);
    checkChrome();
    clickControl(maximize);
    QTRY_VERIFY(!dialog->property("maximized").toBool());
    QCOMPARE(QSizeF(dialog->width(), dialog->height()), restored);
    auto *resize = visualItemWithObjectName(dialog, "dialogResizeBottomRight");
    QVERIFY(resize && resize->isVisible());
    const auto corner = resize->mapToItem(root, QPointF(resize->width()/2,resize->height()/2)).toPoint();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, corner);
    moveNativePointer(fixture.window, corner-QPoint(70,50), 20);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, corner-QPoint(70,50));
    QTRY_VERIFY(dialog->width() < restored.width() && dialog->height() < restored.height());
    QTest::qWait(30);
    checkChrome();
    fixture.shell.clearActions();
    clickControl(close);
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), "menu.close");
    QCOMPARE(fixture.shell.actions.last().value("target").toString(), "history-fit");
}

void F4QuickViewSurfaceTests::menuScrollBarUsesNativeExtentAndCommitsMouseDrag()
{
    const auto menuWithRows = [](int count, bool withSeparators = false) {
        QVariantList items;
        items.reserve(count);
        for (int index = 0; index < count; ++index) {
            const bool separator = withSeparators && index > 0
                && index % 5 == 0;
            items.append(QVariantMap{
                {QStringLiteral("index"), index},
                {QStringLiteral("text"),
                 QStringLiteral("Language row %1").arg(index)},
                {QStringLiteral("separator"), separator},
                {QStringLiteral("disabled"), false},
            });
        }
        return QVariantMap{
            {QStringLiteral("id"), QStringLiteral("language-menu")},
            {QStringLiteral("kind"), QStringLiteral("menu")},
            {QStringLiteral("role"), QStringLiteral("vmenu")},
            {QStringLiteral("x"), 20},
            {QStringLiteral("y"), 4},
            {QStringLiteral("w"), 30},
            {QStringLiteral("h"), 4},
            {QStringLiteral("selected"), 0},
            // Deliberately contradict the native popup. The old indicator used
            // this console-derived value and appeared even though every row fit.
            {QStringLiteral("viewHeight"), 1},
            {QStringLiteral("top"), 0},
            {QStringLiteral("items"), items},
        };
    };

    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"),
                 QVariantList{menuWithRows(18, true)});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);

    QQuickItem *const visualRoot = fixture.window->contentItem();
    QQuickItem *popup = nullptr;
    QQuickItem *list = nullptr;
    QQuickItem *scrollBar = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (popup = visualItemWithObjectName(
             visualRoot, QStringLiteral("semanticMenuPopup-language-menu"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (list = visualItemWithObjectName(
             visualRoot, QStringLiteral("semanticMenuList-language-menu"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (scrollBar = visualItemWithObjectName(
             visualRoot,
             QStringLiteral("semanticMenuScrollBar-language-menu"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(list->property("contentHeight").toReal() > 0,
                             3000);
    const qreal exactContentHeight = popup->parentItem()
        ->property("menuContentHeight").toReal();
    const QString extentDetails = QStringLiteral(
        "exact=%1, estimated=%2, list=%3, popup=%4")
        .arg(exactContentHeight)
        .arg(list->property("contentHeight").toReal())
        .arg(list->height())
        .arg(popup->height());
    QVERIFY2(!scrollBar->property("nativeOverflow").toBool(),
             qPrintable(extentDetails));
    QVERIFY2(!scrollBar->isVisible(),
             "a native popup that fits every item must not show a scrollbar");

    fixture.shell.setCommandMenus(QVariantList{menuWithRows(80)});
    QTRY_VERIFY_WITH_TIMEOUT(
        list->property("contentHeight").toReal() > list->height(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(scrollBar->isVisible(), 3000);
    QCOMPARE(scrollBar->property("thickness").toReal(), 8.0);
    const QPointF scrollBarRight = scrollBar->mapToItem(
        popup, QPointF(scrollBar->width(), 0));
    QVERIFY2(qAbs(scrollBarRight.x() - popup->width()) < 0.01,
             "menu scrollbar must be flush with the popup's right edge");

    const QColor themedHandle(QStringLiteral("#ff6a5acd"));
    QVERIFY(fixture.window->setProperty("galleryScrollBarHandleColor",
                                        themedHandle));
    QTRY_COMPARE_WITH_TIMEOUT(
        scrollBar->property("handleColor").value<QColor>(),
        themedHandle, 3000);

    const qreal dpr = fixture.window->devicePixelRatio();
    const QPointF scrollOrigin = scrollBar->mapToItem(visualRoot, QPointF{});
    for (const qreal coordinate : {scrollOrigin.x(), scrollOrigin.y(),
                                   scrollBar->width(), scrollBar->height()}) {
        QVERIFY2(qAbs(coordinate * dpr - qRound(coordinate * dpr)) < 0.001,
                 "menu scrollbar geometry must remain on physical pixels");
    }

    auto *const handle = qobject_cast<QQuickItem *>(
        scrollBar->property("contentItem").value<QObject *>());
    QVERIFY(handle);
    QTRY_VERIFY_WITH_TIMEOUT(handle->height() > 1, 3000);
    const QPoint start = handle->mapToScene(
        QPointF(handle->width() / 2, handle->height() / 2)).toPoint();
    const QPoint end = scrollBar->mapToScene(
        QPointF(scrollBar->width() / 2,
                scrollBar->height() - handle->height() / 2 - 2)).toPoint();

    fixture.shell.clearActions();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, start);
    QTRY_VERIFY_WITH_TIMEOUT(scrollBar->property("pressed").toBool(), 1500);
    QTest::mouseMove(fixture.window, end, 30);
    QTRY_VERIFY_WITH_TIMEOUT(list->property("contentY").toReal() > 1, 1500);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, end);
    QTRY_VERIFY_WITH_TIMEOUT(!scrollBar->property("pressed").toBool(), 1500);

    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    const QVariantMap action = fixture.shell.actions.constFirst();
    QCOMPARE(action.value(QStringLiteral("target")).toString(),
             QStringLiteral("language-menu"));
    QCOMPARE(action.value(QStringLiteral("action")).toString(),
             QStringLiteral("menu.scroll"));
    QVERIFY(action.contains(QStringLiteral("top")));
    QVERIFY(action.value(QStringLiteral("top")).toInt() > 0);
    QVERIFY(!action.contains(QStringLiteral("delta")));

    // ListView's estimated contentHeight may lag or overshoot after replacing
    // a long uniform model with the real menu shape (short separators mixed
    // with normal rows). Visibility follows exact semantic row geometry.
    fixture.shell.setCommandMenus(
        QVariantList{menuWithRows(18, true)});
    QTRY_VERIFY_WITH_TIMEOUT(
        !scrollBar->property("nativeOverflow").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!scrollBar->isVisible(), 3000);

    // The larger Options-shaped menu also needs no scrollbar in a window
    // whose native body can present every row.
    fixture.window->resize(900, 1000);
    fixture.shell.setCommandMenus(
        QVariantList{menuWithRows(26, true)});
    QTRY_VERIFY_WITH_TIMEOUT(
        !scrollBar->property("nativeOverflow").toBool(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(!scrollBar->isVisible(), 3000);
}

void F4QuickViewSurfaceTests::f9MenuOverlaysPathRowWithoutMovingContent()
{
    auto scene = shellScene({}, 0);
    QVariantMap bar{{"active", false}, {"items", QVariantList{
        QVariantMap{{"index", 0}, {"text", "Left"}},
        QVariantMap{{"index", 1}, {"text", "Files"}},
        QVariantMap{{"index", 2}, {"text", "Commands"}}}}};
    scene.insert("menuBar", bar);
    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    QVERIFY(qAbs(dpr - 1.75) < .001);
    auto *menu = fixture.item("panelMenuBar");
    auto *header = fixture.item("panelHeader-0");
    auto *panel = fixture.item("galleryPanelContent-0");
    auto *title = fixture.item("titleBar");
    QVERIFY(menu && header && panel && title);
    QVERIFY(!menu->isVisible());
    const QPointF before = panel->mapToScene(QPointF());
    const QSizeF sizeBefore = panel->size();
    const QImage pathChrome = fixture.window->grabWindow();
    const int chromeX = qRound(menu->width() * .7 * dpr);
    const int chromeTop = qRound(header->mapToScene(QPointF()).y() * dpr);
    const int chromeBottom = qRound((header->mapToScene(QPointF()).y() + header->height()) * dpr) - 1;
    bar.insert("active", true);
    scene.insert("menuBar", bar);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(menu->isVisible());
    QCOMPARE(menu->mapToScene(QPointF()).y(), header->mapToScene(QPointF()).y());
    QCOMPARE(menu->height(), header->height());
    QCOMPARE(menu->width(), fixture.window->contentItem()->width());
    QVERIFY(menu->parentItem() != title);
    QCOMPARE(panel->mapToScene(QPointF()), before);
    QCOMPARE(panel->size(), sizeBefore);
    QTest::qWait(80);
    const QImage menuChrome = fixture.window->grabWindow();
    qInfo() << "[FIX:f9-path-chrome] background and separator pixels"
            << menuChrome.pixelColor(chromeX, chromeTop + 2)
            << menuChrome.pixelColor(chromeX, chromeBottom);
    QCOMPARE(menuChrome.pixelColor(chromeX, chromeTop + 2), pathChrome.pixelColor(chromeX, chromeTop + 2));
    QCOMPARE(menuChrome.pixelColor(chromeX, chromeBottom), pathChrome.pixelColor(chromeX, chromeBottom));
    auto *separator = fixture.item("panelMenuBarSeparator");
    QVERIFY(separator);
    const auto separatorOrigin = separator->mapToScene(QPointF());
    QCOMPARE(separatorOrigin.y() + separator->height(), header->mapToScene(QPointF()).y() + header->height());
    QVERIFY(qAbs(separatorOrigin.y() * dpr - qRound64(separatorOrigin.y() * dpr)) < .001);
    QVERIFY(qAbs(separator->height() * dpr - qRound64(separator->height() * dpr)) < .001);
    for (int index = 0; index < 3; ++index) {
        auto *text = visualItemWithObjectNamePrefix(menu, QString("semanticMenuBarLabel-%1").arg(index));
        QVERIFY2(text && text->isVisible(), qPrintable(QString("missing/hidden menu label %1").arg(index)));
        const auto origin = text->mapToScene(QPointF());
        const auto physical = origin * dpr;
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < .001
            && qAbs(physical.y() - qRound64(physical.y())) < .001,
            qPrintable(QString("menu leaf %1,%2 physical").arg(physical.x()).arg(physical.y())));
        QCOMPARE(text->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(text->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
    }
    QVERIFY(fixture.window->grabWindow().save("/tmp/f4-f9-path-overlay-175.png"));
    bar.insert("active", false);
    scene.insert("menuBar", bar);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!menu->isVisible());
    QCOMPARE(panel->mapToScene(QPointF()), before);
    QCOMPARE(panel->size(), sizeBefore);
}

void F4QuickViewSurfaceTests::appIconMenuUsesSemanticCategoriesAndCommands()
{
    auto scene = shellScene({}, 0);
    const QVariantList commands{
        QVariantMap{{"index", 4}, {"text", "Inspect"}, {"shortcut", "F3"}, {"icon", "images"}},
        QVariantMap{{"index", 5}, {"text", "Unavailable"}, {"disabled", true}}};
    const QVariantList categories{
        QVariantMap{{"index", 2}, {"text", "Files"}, {"items", commands}}};
    scene.insert("menuBar", QVariantMap{{"active", false}, {"items", categories}});
    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    auto *icon = fixture.item("appIconButton");
    QVERIFY(icon);
    // Exercise the non-macOS entrypoint with the shared QML on the macOS CI host.
    icon->setVisible(true);
    QCoreApplication::processEvents();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
        icon->mapToScene(QPointF(icon->width()/2, icon->height()/2)).toPoint());
    auto *popup = fixture.window->findChild<QObject *>("applicationMenuPopup");
    QVERIFY(popup);
    QTRY_VERIFY(popup->property("visible").toBool());
    QCOMPARE(popup->property("count").toInt(), 1);
    auto *submenu = fixture.window->findChild<QObject *>("applicationSubmenu-2");
    QVERIFY(submenu);
    QCOMPARE(submenu->property("count").toInt(), 2);
    QVERIFY(QMetaObject::invokeMethod(submenu, "open"));
    QTRY_VERIFY(submenu->property("visible").toBool());
    QTest::qWait(100);
    const qreal dpr = fixture.window->devicePixelRatio();
    for (const auto &name : {"applicationMenuEntry-Files-text", "applicationMenuEntry-Files-marker",
                            "applicationMenuEntry-Inspect-text", "applicationMenuEntry-Inspect-shortcut",
                            "applicationMenuEntry-Inspect-icon",
                            "applicationMenuEntry-Unavailable-text", "appIconImage"}) {
        auto *leaf = fixture.item(name);
        QVERIFY(leaf && leaf->isVisible());
        const auto origin = leaf->mapToScene(QPointF());
        const auto physical = origin * dpr;
        QVERIFY(qAbs(physical.x() - qRound64(physical.x())) < .001);
        QVERIFY(qAbs(physical.y() - qRound64(physical.y())) < .001);
        QCOMPARE(leaf->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(leaf->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
    }
    QVERIFY(fixture.window->grabWindow().save("/tmp/f4-app-menu-175.png"));
    auto *entry = fixture.item("applicationMenuEntry-Inspect");
    QVERIFY(entry);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
        entry->mapToScene(QPointF(entry->width()/2, entry->height()/2)).toPoint());
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("menuBar.itemActivate"));
    QCOMPARE(fixture.shell.actions.last().value("menuIndex").toInt(), 2);
    QCOMPARE(fixture.shell.actions.last().value("index").toInt(), 4);
}

void F4QuickViewSurfaceTests::menuBarPopupStartsUnderClickedItem()
{
    const QVariantMap menuBar = QVariantMap{
        {QStringLiteral("id"), QStringLiteral("main-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("menuBar")},
        {QStringLiteral("active"), true},
        {QStringLiteral("selected"), 2},
        {QStringLiteral("items"), QVariantList{
             QVariantMap{
                 {QStringLiteral("index"), 0},
                 {QStringLiteral("text"), QStringLiteral("Left")},
             },
             QVariantMap{
                 {QStringLiteral("index"), 1},
                 {QStringLiteral("text"), QStringLiteral("Files")},
             },
             QVariantMap{
                 {QStringLiteral("index"), 2},
                 {QStringLiteral("text"), QStringLiteral("Options")},
             },
         }},
    };
    const QVariantMap optionsMenu = QVariantMap{
        {QStringLiteral("id"), QStringLiteral("options-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("menuBarSubmenu"), true},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("items"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 0},
             {QStringLiteral("text"), QStringLiteral("Panel settings")},
             {QStringLiteral("separator"), false},
             {QStringLiteral("disabled"), false},
         }}},
    };
    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"), QVariantList{optionsMenu});

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    // Simulate the pointer preview selecting Options while the asynchronous
    // menu-bar state is still being installed. The popup is already alive
    // when the model arrives, which is the production race.
    fixture.window->setProperty("menuBarPreviewIndex", 2);
    scene.insert(QStringLiteral("menuBar"), menuBar);
    fixture.shell.setScene(scene);

    QQuickItem *popup = nullptr;
    QQuickItem *menuItem = nullptr;
    QQuickItem *const visualRoot = fixture.window->contentItem();
    QTRY_VERIFY_WITH_TIMEOUT(
        (popup = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuPopup-options-menu"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (menuItem = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuBarItem-2"))),
        3000);

    const qreal popupX = popup->mapToItem(visualRoot, QPointF{}).x();
    const qreal menuItemX = menuItem->mapToItem(visualRoot, QPointF{}).x();
    QVERIFY2(qAbs(popupX - menuItemX) < 0.5,
             qPrintable(QStringLiteral(
                 "popup x=%1, menu item x=%2")
                            .arg(popupX, 0, 'f', 3)
                            .arg(menuItemX, 0, 'f', 3)));

    // The semantic menu stays open until explicit input. Its one-shot hover
    // synchronization may run during construction, but an untouched open
    // menu must not keep Qt Quick's render loop alive.
    QTest::qWait(200);
    QSignalSpy idleMenuFrames(fixture.window, &QQuickWindow::frameSwapped);
    QVERIFY(idleMenuFrames.isValid());
    QTest::qWait(160);
    QCOMPARE(idleMenuFrames.size(), 0);
}

void F4QuickViewSurfaceTests::nestedMenuAnchorsHeadersTagsAndChevronsOnPhysicalPixels()
{
    const QVariantMap parentMenu = {
        {QStringLiteral("id"), QStringLiteral("drive-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("x"), 67},
        {QStringLiteral("y"), 5},
        {QStringLiteral("w"), 24},
        {QStringLiteral("h"), 4},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("viewHeight"), 1},
        {QStringLiteral("items"), QVariantList{
             QVariantMap{
                 {QStringLiteral("index"), 0},
                 {QStringLiteral("id"), QStringLiteral("macos-locations")},
                 {QStringLiteral("text"), QStringLiteral("macOS Locations")},
                 {QStringLiteral("icon"), QStringLiteral("folder")},
                 {QStringLiteral("hasSubmenu"), true},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("disabled"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 1},
                 {QStringLiteral("id"), QStringLiteral("local-drive")},
                 {QStringLiteral("text"), QStringLiteral("C: Local")},
                 {QStringLiteral("icon"), QStringLiteral("hard-drive")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("disabled"), false},
             },
         }},
    };
    const QVariantMap childMenu = {
        {QStringLiteral("id"), QStringLiteral("locations-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("parentId"), QStringLiteral("drive-menu")},
        {QStringLiteral("anchorIndex"), 0},
        {QStringLiteral("x"), 0},
        {QStringLiteral("y"), 0},
        {QStringLiteral("w"), 25},
        {QStringLiteral("h"), 7},
        {QStringLiteral("selected"), 1},
        {QStringLiteral("viewHeight"), 4},
        {QStringLiteral("items"), QVariantList{
             QVariantMap{
                 {QStringLiteral("index"), 0},
                 {QStringLiteral("id"), QStringLiteral("tags-header")},
                 {QStringLiteral("text"), QStringLiteral("Tags")},
                 {QStringLiteral("header"), true},
                 {QStringLiteral("disabled"), true},
                 {QStringLiteral("separator"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 1},
                 {QStringLiteral("id"), QStringLiteral("red")},
                 {QStringLiteral("text"), QStringLiteral("Red")},
                 {QStringLiteral("shortcut"), QStringLiteral("Ctrl+R")},
                 {QStringLiteral("icon"), QStringLiteral("tag-dot")},
                 {QStringLiteral("iconColor"), QStringLiteral("#ff453a")},
                 {QStringLiteral("disabled"), false},
                 {QStringLiteral("separator"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 2},
                 {QStringLiteral("text"), QStringLiteral("Network")},
                 {QStringLiteral("icon"), QStringLiteral("network")},
                 {QStringLiteral("hasSubmenu"), true},
                 {QStringLiteral("disabled"), false},
                 {QStringLiteral("separator"), false},
             },
         }},
    };
    QVariantMap scene = shellScene({}, 0);
    scene.insert(QStringLiteral("menus"), QVariantList{parentMenu, childMenu});

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *const visualRoot = fixture.window->contentItem();
    QQuickItem *parentPopup = nullptr;
    QQuickItem *childPopup = nullptr;
    QQuickItem *parentRow = nullptr;
    QQuickItem *secondParentRow = nullptr;
    QQuickItem *headerText = nullptr;
    QQuickItem *tagDot = nullptr;
    QQuickItem *parentChevron = nullptr;
    QQuickItem *childChevron = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (parentPopup = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuPopup-drive-menu"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (childPopup = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuPopup-locations-menu"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (parentRow = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItem-drive-menu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (secondParentRow = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItem-drive-menu-1"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (headerText = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemText-locations-menu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (tagDot = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemColor-locations-menu-1"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (parentChevron = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemChevron-drive-menu-0"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (childChevron = visualItemWithObjectNamePrefix(
             visualRoot, QStringLiteral("semanticMenuItemChevron-locations-menu-2"))), 3000);

    QVERIFY(parentChevron->isVisible());
    QVERIFY(childChevron->isVisible());
    QVERIFY(tagDot->isVisible());
    QCOMPARE(tagDot->property("color").value<QColor>(), QColor("#ff453a"));
    QVERIFY(headerText->property("font").value<QFont>().bold());
    QCOMPARE(headerText->x(), qRound(10 * fixture.window->devicePixelRatio()) / fixture.window->devicePixelRatio());
    QVERIFY(headerText->y() > 0);

    const QPointF parentOrigin = parentPopup->mapToItem(visualRoot, QPointF{});
    const QPointF childOrigin = childPopup->mapToItem(visualRoot, QPointF{});
    const QPointF rowOrigin = parentRow->mapToItem(visualRoot, QPointF{});
    QVERIFY2(childOrigin.x() < parentOrigin.x(),
             "child menu should flip to the left at the right screen edge");
    QVERIFY(qAbs(childOrigin.y() - rowOrigin.y()) < 1.0);

    const qreal dpr = fixture.window->devicePixelRatio();
    QList<QQuickItem *> pixelItems = {parentPopup, childPopup, tagDot,
                                     parentChevron, childChevron, headerText};
    for (const QString &name : {
             QStringLiteral("semanticMenuItemText-drive-menu-0"),
             QStringLiteral("semanticMenuItemText-drive-menu-1"),
             QStringLiteral("semanticMenuItemText-locations-menu-1"),
             QStringLiteral("semanticMenuItemText-locations-menu-2"),
             QStringLiteral("semanticMenuItemShortcut-locations-menu-1")}) {
        auto *leaf = visualItemWithObjectNamePrefix(visualRoot, name);
        QVERIFY2(leaf, qPrintable(name));
        pixelItems.append(leaf);
    }
    for (QQuickItem *item : pixelItems) {
        const QPointF origin = item->mapToItem(visualRoot, QPointF{});
        QVERIFY(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001);
        QVERIFY(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001);
        QVERIFY2(qAbs(item->width() * dpr - qRound(item->width() * dpr)) < 0.001,
                 qPrintable(QString("%1 origin=(%2,%3) physical size=(%4,%5)").arg(item->objectName()).arg(origin.x()*dpr).arg(origin.y()*dpr).arg(item->width()*dpr).arg(item->height()*dpr)));
        QVERIFY(qAbs(item->height() * dpr - qRound(item->height() * dpr)) < 0.001);
        const QPointF ux = item->mapToItem(visualRoot, QPointF(1, 0)) - origin;
        const QPointF uy = item->mapToItem(visualRoot, QPointF(0, 1)) - origin;
        QVERIFY((ux - QPointF(1, 0)).manhattanLength() < 0.001);
        QVERIFY((uy - QPointF(0, 1)).manhattanLength() < 0.001);
    }
    QVERIFY(!fixture.window->property("font").value<QFont>().bold());
    if (qEnvironmentVariableIsSet("F4_MENU_PIXEL_GRID_CAPTURE")) {
        const QImage capture = fixture.window->grabWindow();
        QVERIFY(!capture.isNull());
        QVERIFY(capture.save(qEnvironmentVariable("F4_MENU_PIXEL_GRID_CAPTURE")));
    }

    // A nested popup must not install another full-window backdrop above its
    // parent. The second parent row remains clickable while the child is open.
    fixture.shell.clearActions();
    const QPoint secondParentCenter = secondParentRow->mapToScene(
        QPointF(secondParentRow->width() / 2,
                secondParentRow->height() / 2)).toPoint();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      secondParentCenter);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("target")).toString(),
             QStringLiteral("drive-menu"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("menu.activate"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("index")).toInt(), 1);

    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      QPoint(2, 2));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("target")).toString(),
             QStringLiteral("drive-menu"));
    QCOMPARE(fixture.shell.actions.constFirst()
                 .value(QStringLiteral("action")).toString(),
             QStringLiteral("menu.closeChain"));
}

void F4QuickViewSurfaceTests::nestedMenuHoverUsesDelayedSubmenuAction() {
  QVariantMap scene = shellScene({}, 0);
  scene.insert(
      QStringLiteral("menus"),
      QVariantList{QVariantMap{
          {QStringLiteral("id"), QStringLiteral("drive-menu")},
          {QStringLiteral("kind"), QStringLiteral("menu")},
          {QStringLiteral("role"), QStringLiteral("vmenu")},
          {QStringLiteral("x"), 8},
          {QStringLiteral("y"), 5},
          {QStringLiteral("w"), 28},
          {QStringLiteral("h"), 4},
          {QStringLiteral("selected"), 0},
          {QStringLiteral("viewHeight"), 1},
          {QStringLiteral("items"),
           QVariantList{QVariantMap{
               {QStringLiteral("index"), 0},
               {QStringLiteral("id"), QStringLiteral("macos-locations")},
               {QStringLiteral("text"), QStringLiteral("macOS Locations")},
               {QStringLiteral("hasSubmenu"), true},
               {QStringLiteral("separator"), false},
               {QStringLiteral("disabled"), false},
           }}},
      }});

  QuickViewFixture fixture(scene);
  QVERIFY(fixture.window);
  QQuickItem *row = nullptr;
  QTRY_VERIFY_WITH_TIMEOUT(
      (row = visualItemWithObjectNamePrefix(
           fixture.window->contentItem(),
           QStringLiteral("semanticMenuItem-drive-menu-0"))),
      3000);
  fixture.shell.clearActions();
  moveNativePointer(fixture.window, QPoint(2, 2));
  moveNativePointer(
      fixture.window,
      row->mapToScene(QPointF(row->width() / 2, row->height() / 2)).toPoint());
  QTest::qWait(90);
  QCOMPARE(fixture.shell.actions.size(), 1);
  QCOMPARE(fixture.shell.actions.constFirst().value("action").toString(), QString("menu.select"));
  QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 2, 1000);
  const QVariantMap action = fixture.shell.actions.last();
  QCOMPARE(action.value(QStringLiteral("target")).toString(),
           QStringLiteral("drive-menu"));
  QCOMPARE(action.value(QStringLiteral("action")).toString(),
           QStringLiteral("menu.openSubmenu"));
  QCOMPARE(action.value(QStringLiteral("index")).toInt(), 0);
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
    moveNativePointer(fixture.window, pointer - QPoint(2, 0));
    moveNativePointer(fixture.window, pointer);
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
    fixture.window->setProperty("commandLineGraphicalCursor", false);
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
    const QColor defaultCommandLineBackground(QStringLiteral("#141921"));
    QCOMPARE(fixture.window->property("commandLineBg").value<QColor>(),
             defaultCommandLineBackground);
    QCOMPARE(commandLineView->property("color").value<QColor>(),
             defaultCommandLineBackground);
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
    QCOMPARE(cursor->height(), qRound(2.0 * fixture.window->devicePixelRatio())
                              / fixture.window->devicePixelRatio());
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

void F4QuickViewSurfaceTests::commandLineHeightChangesOncePerEdit()
{
    auto scene = shellScene();
    auto shell = scene.value("shell").toMap();
    QVariantMap command{{"visible",true},{"multiline",true},{"wordWrap",true},{"text","one"}};
    shell.insert("commandLine",command); scene.insert("shell",shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *box = fixture.item("commandLineView");
    QVERIFY(box);
    QTest::qWait(50);
    QSignalSpy changes(box,&QQuickItem::heightChanged);
    for (const QString &text : {QString("one\ntwo\nthree"),QString("one"),QString("one\n"),QString("app -a -b -c")}) {
        changes.clear();
        command.insert("text",text);
        command.insert("cursorPosition",text.size());
        fixture.shell.setCommandLine(command);
        QTest::qWait(30);
        qInfo() << "command height updates" << text << changes.count();
        QVERIFY2(changes.count() <= 1, "one edit triggered repeated panel relayouts");
    }
}

void F4QuickViewSurfaceTests::commandLineGraphicalCaretIsOptionalAndPersisted()
{
    auto scene = shellScene();
    auto shell = scene.value("shell").toMap();
    QVariantMap command{{"visible",true},{"multiline",true},{"text","first\nsecond"},
                        {"cursorPosition",8},{"cursorVisible",true},{"cursorShape","underline"}};
    shell.insert("commandLine", command);
    scene.insert("shell", shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *cursor = fixture.item("commandLineCursor");
    auto *input = fixture.item("commandLineInput");
    QVERIFY(cursor && input);
    QTest::qWait(50);
    QVERIFY(fixture.window->property("commandLineGraphicalCursor").toBool());
    QVERIFY(cursor->height() > cursor->width()*2);
    const qreal dpr = fixture.window->devicePixelRatio();
    QCOMPARE(cursor->width(), qRound(2*dpr)/dpr);
    for (bool graphical : {false,true}) {
        fixture.window->setProperty("commandLineGraphicalCursor", graphical);
        QTest::qWait(30);
        QVERIFY(graphical ? cursor->height() > cursor->width() : cursor->width() > cursor->height());
        const auto origin = cursor->mapToItem(fixture.window->contentItem(), QPointF());
        for (const auto point : {origin, origin + QPointF(cursor->width(),cursor->height())}) {
            QVERIFY(qAbs(point.x()*dpr-qRound(point.x()*dpr)) < 0.01);
            QVERIFY(qAbs(point.y()*dpr-qRound(point.y()*dpr)) < 0.01);
        }
        QCOMPARE(cursor->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(cursor->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
    }
    QVERIFY(fixture.window->grabWindow().save(".diagnostics/command-graphical-caret-175.png"));
    fixture.window->setProperty("commandLineGraphicalCursor", false);
    QVERIFY(QMetaObject::invokeMethod(fixture.window,"saveThemeToPersistence"));
    QCOMPARE(fixture.themePersistence.theme().value("commandLineGraphicalCursor").toBool(), false);
    fixture.window->setProperty("commandLineGraphicalCursor", true);
    QVERIFY(QMetaObject::invokeMethod(fixture.window,"loadThemeFromPersistence"));
    QCOMPARE(fixture.window->property("commandLineGraphicalCursor").toBool(), false);
    fixture.themePersistence.setTheme({{"showSelectionBorders",true}});
    QVERIFY(QMetaObject::invokeMethod(fixture.window,"loadThemeFromPersistence"));
    QCOMPARE(fixture.window->property("commandLineGraphicalCursor").toBool(), true);
    command.insert("cursorShape", "block");
    fixture.shell.setCommandLine(command);
    QTRY_VERIFY(cursor->width() > qRound(2*dpr)/dpr);
    QVERIFY(cursor->height() > 2);
}

void F4QuickViewSurfaceTests::commandLineClickTransfersFocus()
{
    auto scene = shellScene();
    auto shell = scene.value("shell").toMap();
    QVariantMap command{{"visible",true},{"ownsNavigation",false}};
    shell.insert("commandLine", command);
    scene.insert("shell", shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *box = fixture.item("commandLineView");
    QVERIFY(box);
    const auto colors = [&]() { return qmlObjectProperties(fixture.window->property("galleryThemePalette")); };
    const auto active = colors().value("cursor").value<QColor>();
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     box->mapToScene(QPointF(box->width()/2,box->height()/2)).toPoint());
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), "commandLine.focus");
    command.insert("ownsNavigation", true);
    fixture.shell.setCommandLine(command);
    QTest::qWait(50);
    for (const QString &key : {QString("cursor"),QString("cursorBackground"),QString("cursorBorder"),QString("cardCursorBorder")}) {
        const QColor inactive = colors().value(key).value<QColor>();
        QVERIFY(inactive.isValid());
        QCOMPARE(inactive.red(), inactive.green());
        QCOMPARE(inactive.green(), inactive.blue());
    }
    QVERIFY(colors().value("cursor").value<QColor>() != active);
    for (int side : {0,1}) {
        auto *panel = visualItemWithObjectName(fixture.window->contentItem(), "filePanel-" + QString::number(side));
        QVERIFY(panel);
        for (const QPointF point : {QPointF(panel->width()/2,panel->height()/2), QPointF(10,10)}) {
            fixture.shell.clearActions();
            QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, panel->mapToScene(point).toPoint());
            bool returnedFocus = false;
            for (const auto &action : fixture.shell.actions)
                returnedFocus |= action.value("action").toString() == "panel.activate" && action.value("side").toInt() == side;
            QVERIFY(returnedFocus);
        }
    }
    command.insert("ownsNavigation", false);
    fixture.shell.setCommandLine(command);
    QTRY_COMPARE(colors().value("cursor").value<QColor>(), active);
}

void F4QuickViewSurfaceTests::commandLineDropOutlineMatchesPanels()
{
    auto scene = shellScene();
    auto shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("commandLine"), QVariantMap{{QStringLiteral("visible"), true}});
    scene.insert(QStringLiteral("shell"), shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *box = fixture.item(QStringLiteral("commandLineView"));
    auto *outline = fixture.item(QStringLiteral("commandLineDropOutline"));
    QVERIFY(box && outline);
    QVERIFY(!outline->isVisible());
    box->setProperty("dropHovered", true);
    QTRY_VERIFY(outline->isVisible());
    QCOMPARE(outline->property("color").value<QColor>(), QColor(Qt::transparent));
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto border = QQmlProperty(outline, QStringLiteral("border.width")).read().toReal();
    QCOMPARE(border*dpr, 2.0);
    QCOMPARE(QQmlProperty(outline, QStringLiteral("border.color")).read().value<QColor>(),
             fixture.window->property("gallerySelectionColor").value<QColor>());
    const auto origin = outline->mapToItem(fixture.window->contentItem(), QPointF());
    for (const QPointF point : {origin, origin+QPointF(outline->width(),outline->height())}) {
        QVERIFY(qAbs(point.x()*dpr-qRound(point.x()*dpr)) < 0.01);
        QVERIFY(qAbs(point.y()*dpr-qRound(point.y()*dpr)) < 0.01);
    }
    const QImage capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    QVERIFY(imageContainsColor(capture, fixture.window->property("gallerySelectionColor").value<QColor>()));
    QVERIFY(capture.save(QStringLiteral(".diagnostics/command-drop-outline-175.png")));
    box->setProperty("dropHovered", false);
    QTRY_VERIFY(!outline->isVisible());
}

void F4QuickViewSurfaceTests::commandLineEmptyBaseline()
{
    for (bool rich : {false, true}) {
        auto scene = shellScene();
        auto shell = scene.value(QStringLiteral("shell")).toMap();
        QVariantMap command{{"visible", true}, {"multiline", rich}, {"wordWrap", rich},
                            {"prompt", "> "}, {"text", "abc"},
                            {"cursorPosition", 0}, {"cursorVisible", true}};
        shell.insert("commandLine", command);
        scene.insert("shell", shell);
        QuickViewFixture fixture(scene);
        auto *input = fixture.item(QStringLiteral("commandLineInput"));
        auto *cursor = fixture.item(QStringLiteral("commandLineCursor"));
        QVERIFY(input && cursor);
        QTest::qWait(100);
        const auto baseline = input->mapToScene(QPointF(0, input->baselineOffset())).y();
        const auto caretY = cursor->mapToScene(QPointF()).y();
        for (const QString &text : {QString(), QStringLiteral("abc"), QString()}) {
            command.insert("text", text);
            shell.insert("commandLine", command);
            scene.insert("shell", shell);
            fixture.shell.setScene(scene);
            QTest::qWait(100);
            qInfo() << "[FIX:command-baseline] rich" << rich << "text" << text
                    << "baseline" << input->mapToScene(QPointF(0, input->baselineOffset())).y()
                    << "expected" << baseline << "height" << input->property("contentHeight");
            QCOMPARE(input->mapToScene(QPointF(0, input->baselineOffset())).y(), baseline);
            QCOMPARE(cursor->mapToScene(QPointF()).y(), caretY);
            for (auto *leaf : {input, cursor}) {
                const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
                const auto physical = origin * fixture.window->devicePixelRatio();
                QVERIFY(qAbs(physical.x()-qRound(physical.x())) < 0.01);
                QVERIFY(qAbs(physical.y()-qRound(physical.y())) < 0.01);
                QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
                QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
            }
            QVERIFY(fixture.window->grabWindow().save(QStringLiteral("/tmp/f4-command-%1-%2.png")
                    .arg(rich).arg(text.isEmpty() ? "empty" : "text")));
        }
    }
}

void F4QuickViewSurfaceTests::commandLineFontBaselineCaretAndCompactHeight_data()
{
    QTest::addColumn<QString>("family");
    QTest::newRow("configured-default") << QStringLiteral("Monaco");
    QTest::newRow("configured-consolas") << QStringLiteral("Consolas");
}

void F4QuickViewSurfaceTests::commandLineFontBaselineCaretAndCompactHeight()
{
    QFETCH(QString, family);
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    const QString text = QStringLiteral("pwd\nladalsdl\nasd");
    QVariantMap command{
        {QStringLiteral("visible"), true},
        {QStringLiteral("multiline"), true},
        {QStringLiteral("wordWrap"), true},
        {QStringLiteral("promptRuns"), QVariantList{QVariantMap{
            {QStringLiteral("text"), QStringLiteral("xs@HC D:\\Code\\f4-zoin>")},
            {QStringLiteral("foreground"), QStringLiteral("#8ae234")}}}},
        {QStringLiteral("text"), text},
        {QStringLiteral("cursorPosition"), text.size()},
        {QStringLiteral("cursorVisible"), true},
    };
    shell.insert(QStringLiteral("commandLine"), command);
    scene.insert(QStringLiteral("shell"), shell);
    QuickViewFixture fixture(scene);
    fixture.engine.rootContext()->setContextProperty(QStringLiteral("f4GuiFontFamily"), family);
    QVERIFY(fixture.window);
    auto *input = fixture.item(QStringLiteral("commandLineInput"));
    auto *cursor = fixture.item(QStringLiteral("commandLineCursor"));
    auto *box = fixture.item(QStringLiteral("commandLineView"));
    auto *presentation = fixture.item(QStringLiteral("commandLinePresentation"));
    QVERIFY(input && cursor && box && presentation);
    QTRY_COMPARE(input->property("cursorPosition").toInt(), text.size());
    QTest::qWait(50);
    auto *quickDocument = qvariant_cast<QQuickTextDocument *>(input->property("textDocument"));
    QVERIFY(quickDocument);
    auto *document = quickDocument->textDocument();
    const auto fragment = document->begin().begin().fragment();
    const QFont actualFont = fragment.charFormat().font().resolve(document->defaultFont());
    const QFont expectedFont = input->property("font").value<QFont>();
    const QRectF caret = input->property("cursorRectangle").toRectF();
    const QPointF caretBottom = input->mapToItem(box, caret.bottomLeft());
    const qreal contentHeight = input->property("contentHeight").toReal();
    qInfo() << "font" << actualFont << "expected" << expectedFont
            << "caret bottom" << caretBottom << "cursor bottom" << cursor->y()+cursor->height()
            << "presentation height" << presentation->height() << "text height" << contentHeight;
    QCOMPARE(actualFont.families(), expectedFont.families());
    const qreal pixel = 1.0 / fixture.window->devicePixelRatio();
    QVERIFY(qAbs(cursor->y()+cursor->height()-caretBottom.y()) <= pixel);
    QVERIFY(presentation->height() <= contentHeight +
            qMax(0.0, fixture.window->property("ch").toReal()
                -contentHeight/input->property("lineCount").toInt()) + pixel + 0.01);
    QList<QQuickItem *> items = box->childItems();
    bool checkedPrompt = false;
    for (qsizetype i=0; i<items.size(); ++i) {
        auto *leaf=items.at(i);
        items.append(leaf->childItems());
        if (leaf->objectName() == QStringLiteral("commandLineInput")
                || leaf->objectName() == QStringLiteral("commandLinePromptRun0")) {
            const QPointF origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
            const QPointF physical = origin / pixel;
            qInfo() << leaf->objectName() << "physical origin" << physical;
            QVERIFY(qAbs(physical.x()-qRound(physical.x())) < 0.01);
            QVERIFY(qAbs(physical.y()-qRound(physical.y())) < 0.01);
            QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
        }
        if (leaf->objectName() != QStringLiteral("commandLinePromptRun0")) continue;
        const qreal promptBaseline = leaf->mapToItem(box, QPointF(0,leaf->baselineOffset())).y();
        const qreal inputBaseline = input->mapToItem(box, QPointF(0,input->baselineOffset())).y();
        qInfo() << "baselines" << promptBaseline << inputBaseline;
        QVERIFY(qAbs(promptBaseline-inputBaseline) <= pixel + 0.01);
        checkedPrompt = true;
    }
    QVERIFY(checkedPrompt);
    QVERIFY(fixture.window->grabWindow().save(QStringLiteral(".diagnostics/command-line-metrics-175.png")));
}

void F4QuickViewSurfaceTests::commandLinePanelToggleIsImmediate()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value("shell").toMap();
    QVariantMap command{{"visible",false},{"autoHide",true},{"prompt","> "},{"text","retained"}};
    shell["commandLine"] = command;
    scene["shell"] = shell;
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *box = fixture.item("commandLineView");
    QVERIFY(box);
    QTest::qWait(180);
    for (int cycle=0; cycle<3; ++cycle) {
        command["visible"] = true;
        command["autoHide"] = false;
        shell["commandLine"] = command;
        shell["showPanels"] = false;
        shell["terminalActive"] = true;
        scene["shell"] = shell;
        fixture.shell.setScene(scene);
        QCoreApplication::processEvents();
        QCOMPARE(fixture.window->property("commandLineReveal").toReal(), 1.0);
        QVERIFY(box->height() > 0);
        command["visible"] = false;
        command["autoHide"] = true;
        shell["commandLine"] = command;
        shell["showPanels"] = true;
        shell["terminalActive"] = false;
        scene["shell"] = shell;
        fixture.shell.setScene(scene);
        QCoreApplication::processEvents();
        QCOMPARE(fixture.window->property("commandLineReveal").toReal(), 0.0);
        QCOMPARE(box->height(), 0.0);
    }
}

void F4QuickViewSurfaceTests::commandLineAutoHideRevealsUpward()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap command{
        {QStringLiteral("visible"), false}, {QStringLiteral("autoHide"), true},
        {QStringLiteral("prompt"), QStringLiteral("> ")},
        {QStringLiteral("text"), QStringLiteral("retained command")},
        {QStringLiteral("cursorPosition"), 16},
        {QStringLiteral("cursorVisible"), true}
    };
    shell.insert(QStringLiteral("commandLine"), command);
    scene.insert(QStringLiteral("shell"), shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *box = fixture.item(QStringLiteral("commandLineView"));
    auto *input = fixture.item(QStringLiteral("commandLineInput"));
    QVERIFY(box);
    QVERIFY(input);
    QTRY_COMPARE(box->height(), 0.0);
    QVERIFY(!box->isVisible());
    const qreal bottom = box->y();
    QSignalSpy heights(box, &QQuickItem::heightChanged);
    command.insert(QStringLiteral("visible"), true);
    command.insert(QStringLiteral("ownsNavigation"), true);
    fixture.shell.setCommandLine(command);
    QTest::qWait(220);
    QVERIFY(box->isVisible());
    QVERIFY(box->height() > 0);
    QVERIFY(heights.count() > 2);
    QVERIFY(box->y() < bottom);
    QVERIFY(qAbs(box->y() + box->height() - bottom) < 0.01);
    QCOMPARE(input->property("text").toString(), QStringLiteral("retained command"));
    const qreal dpr = fixture.window->devicePixelRatio();
    QList<QQuickItem *> leaves = box->childItems();
    int checked = 0;
    for (qsizetype i = 0; i < leaves.size(); ++i) {
        auto *leaf = leaves.at(i);
        leaves.append(leaf->childItems());
        if (!leaf->isVisible() || !(leaf->objectName() == QStringLiteral("commandLineInput")
            || leaf->objectName().startsWith(QStringLiteral("commandLinePromptRun"))
            || leaf->objectName() == QStringLiteral("commandLinePromptFallback")
            || leaf->objectName().startsWith(QStringLiteral("commandLineWrapMarker"))))
            continue;
        ++checked;
        const QPointF origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        const QPointF physical = origin * dpr;
        qInfo() << leaf->objectName() << "physical origin" << physical;
        QVERIFY(qAbs(physical.x() - qRound(physical.x())) < 0.01);
        QVERIFY(qAbs(physical.y() - qRound(physical.y())) < 0.01);
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
    }
    QVERIFY(checked >= 2);
    const auto capture = qEnvironmentVariable("F4_COMMAND_REVEAL_CAPTURE");
    if (!capture.isEmpty())
        QVERIFY(fixture.window->grabWindow().save(capture));
    command.insert(QStringLiteral("visible"), false);
    fixture.shell.setCommandLine(command);
    QTest::qWait(220);
    QCOMPARE(box->height(), 0.0);
    QVERIFY(!box->isVisible());
    QCOMPARE(input->property("text").toString(), QStringLiteral("retained command"));
    // An interrupted transition must settle at the latest focus state.
    command.insert(QStringLiteral("visible"), true);
    fixture.shell.setCommandLine(command);
    QTest::qWait(30);
    command.insert(QStringLiteral("visible"), false);
    fixture.shell.setCommandLine(command);
    QTest::qWait(220);
    QCOMPARE(box->height(), 0.0);
    command.insert(QStringLiteral("autoHide"), false);
    command.insert(QStringLiteral("visible"), true);
    fixture.shell.setCommandLine(command);
    QTRY_VERIFY(box->height() > 0);
}

void F4QuickViewSurfaceTests::commandLineMultilineWrapAndPixelGrid()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantMap command{
        {QStringLiteral("visible"), true},
        {QStringLiteral("multiline"), true},
        {QStringLiteral("wordWrap"), true},
        {QStringLiteral("prompt"), QStringLiteral("> ")},
        {QStringLiteral("text"), QStringLiteral("app -arg\nsecond line\nthird")},
        {QStringLiteral("cursorPosition"), 27},
        {QStringLiteral("cursorVisible"), true},
    };
    shell.insert(QStringLiteral("commandLine"), command);
    scene.insert(QStringLiteral("shell"), shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *input = fixture.item(QStringLiteral("commandLineInput"));
    auto *box = fixture.item(QStringLiteral("commandLineView"));
    QVERIFY(input);
    QVERIFY(box);
    QTRY_COMPARE(input->property("lineCount").toInt(), 4);
    const qreal threeRowsHeight = box->height();
    command.insert(QStringLiteral("text"), QStringLiteral("app ") + QStringLiteral("-argument ").repeated(150));
    command.insert(QStringLiteral("cursorPosition"), 1504);
    fixture.shell.setCommandLine(command);
    QTRY_VERIFY(input->property("lineCount").toInt() > 3);
    QTRY_VERIFY(box->height() > threeRowsHeight);
    QVERIFY(box->height() < fixture.window->height() * 0.6);
    command.insert(QStringLiteral("text"), QStringLiteral("app -arg\nsecond line\nthird"));
    command.insert(QStringLiteral("cursorPosition"), 26);
    fixture.shell.setCommandLine(command);
    QTRY_COMPARE(input->property("lineCount").toInt(), 4);
    QTest::qWait(50);
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto verifyLeaves = [&]() {
    QList<QQuickItem *> leaves = box->childItems();
    for (qsizetype i = 0; i < leaves.size(); ++i) {
        auto *leaf = leaves.at(i);
        leaves.append(leaf->childItems());
        if (!leaf->isVisible() || !(leaf->objectName() == QStringLiteral("commandLineInput")
            || leaf->objectName().startsWith(QStringLiteral("commandLinePromptRun"))
            || leaf->objectName() == QStringLiteral("commandLinePromptFallback")
            || leaf->objectName().startsWith(QStringLiteral("commandLineWrapMarker"))))
            continue;
        const QPointF origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        const QPointF physical = origin * dpr;
        qInfo() << leaf->objectName() << "physical origin" << physical;
        QVERIFY(qAbs(physical.x() - qRound(physical.x())) < 0.01);
        QVERIFY(qAbs(physical.y() - qRound(physical.y())) < 0.01);
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
    }
    };
    verifyLeaves();
    command.insert(QStringLiteral("promptRuns"), QVariantList{
        QVariantMap{{QStringLiteral("text"), QStringLiteral("user@host ")},
                    {QStringLiteral("foreground"), QStringLiteral("#8ae234")}},
        QVariantMap{{QStringLiteral("text"), QStringLiteral("D:/work> ")},
                    {QStringLiteral("foreground"), QStringLiteral("#d3d7cf")}}
    });
    fixture.shell.setCommandLine(command);
    QTest::qWait(50);
    verifyLeaves();
    const QImage capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    capture.save(QStringLiteral(".diagnostics/command-line-multiline-175.png"));
    command.insert(QStringLiteral("multiline"), false);
    fixture.shell.setCommandLine(command);
    QTRY_VERIFY(box->height() < threeRowsHeight);
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

void F4QuickViewSurfaceTests::panelCursorBlinkSettlesAndBlockingMenuStopsIt()
{
    QVariantMap scene = shellScene({}, 0);
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    QVariantList panels = shell.value(QStringLiteral("panels")).toList();
    QVariantMap leftPanel = panels.at(0).toMap();
    leftPanel.insert(QStringLiteral("fastFind"), true);
    leftPanel.insert(QStringLiteral("fastFindText"), QStringLiteral("needle"));
    panels[0] = leftPanel;
    shell.insert(QStringLiteral("panels"), panels);
    shell.insert(QStringLiteral("commandLine"), QVariantMap{
        {QStringLiteral("visible"), true},
        {QStringLiteral("prompt"), QStringLiteral("> ")},
        {QStringLiteral("text"), QStringLiteral("find needle")},
        {QStringLiteral("cursorPosition"), 11},
        {QStringLiteral("cursorShape"), QStringLiteral("underline")},
        {QStringLiteral("cursorVisible"), true},
    });
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.window->isActive(), 3000);
    auto *commandCursor = fixture.item(QStringLiteral("commandLineCursor"));
    auto *fastFindCursor = fixture.item(
        QStringLiteral("panelFastFindCursor-0"));
    QVERIFY(commandCursor);
    QVERIFY(fastFindCursor);
    QTRY_VERIFY_WITH_TIMEOUT(commandCursor->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(fastFindCursor->isVisible(), 3000);
    QVERIFY(commandCursor->setProperty("blinkInterval", 20));
    QVERIFY(fastFindCursor->setProperty("blinkInterval", 20));
    QVERIFY(QMetaObject::invokeMethod(commandCursor, "restartBlink"));
    QVERIFY(QMetaObject::invokeMethod(fastFindCursor, "restartBlink"));
    QTRY_VERIFY_WITH_TIMEOUT(
        commandCursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        fastFindCursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        !commandCursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        !fastFindCursor->property("blinkTimerRunning").toBool(), 500);
    QVERIFY(commandCursor->property("blinkOn").toBool());
    QVERIFY(fastFindCursor->property("blinkOn").toBool());

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
             "panel surface never reached frame quiescence");
    settledFrames.clear();
    QTest::qWait(700);
    QCOMPARE(settledFrames.size(), 0);

    auto *grid = fixture.item<TestGrid>(QStringLiteral("vtuiGrid"));
    QVERIFY(grid);
    emit grid->keyboardActivity();
    QTRY_VERIFY_WITH_TIMEOUT(
        commandCursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        fastFindCursor->property("blinkTimerRunning").toBool(), 500);

    const QVariantMap menu{
        {QStringLiteral("id"), QStringLiteral("blink-blocking-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("x"), 4},
        {QStringLiteral("y"), 4},
        {QStringLiteral("w"), 20},
        {QStringLiteral("h"), 3},
        {QStringLiteral("selected"), 0},
        {QStringLiteral("items"), QVariantList{QVariantMap{
             {QStringLiteral("index"), 0},
             {QStringLiteral("text"), QStringLiteral("Menu item")},
             {QStringLiteral("disabled"), false},
             {QStringLiteral("separator"), false},
         }}},
    };
    fixture.shell.setCommandMenus(QVariantList{menu});
    QTRY_VERIFY_WITH_TIMEOUT(
        !commandCursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        !fastFindCursor->property("blinkTimerRunning").toBool(), 500);
    QVERIFY(commandCursor->property("blinkOn").toBool());
    QVERIFY(fastFindCursor->property("blinkOn").toBool());

    fixture.shell.setCommandMenus({});
    QTRY_VERIFY_WITH_TIMEOUT(
        commandCursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        fastFindCursor->property("blinkTimerRunning").toBool(), 500);
}

void F4QuickViewSurfaceTests::shortenedPanelsRevealTerminalRows()
{
    auto scene = shellScene({}, 0);
    auto shell = scene.value("shell").toMap();
    auto rows = visualRows(0, 90);
    for (int i = 1; i < rows.size(); i += 2) {
        auto row = rows[i].toMap();
        row.insert("runs", QVariantList{QVariantMap{{"text", row.value("text")}, {"foreground", "#80c0ff"}}});
        rows[i] = row;
    }
    shell.insert("terminal", QVariantMap{
        {"id", "terminal-shortened"}, {"kind", "terminal"}, {"scrollUnit", "rows"},
        {"windowRows", rows}, {"windowStart", 0}, {"windowEnd", 90},
        {"viewportStart", 30}, {"viewportSpan", 24}, {"viewportRow", 30},
        {"viewportRows", 24}, {"contentExtent", 90},
        {"contentExtentKnown", true}, {"windowGeneration", 1},
    });
    scene.insert("shell", shell);
    QuickViewFixture fixture(scene, true);
    QVERIFY(fixture.window);
    auto *left = fixture.item("filePanel-0");
    auto *right = fixture.item("filePanel-1");
    auto *backdrop = fixture.item("terminalBackdrop");
    QVERIFY(left && right && backdrop);
    const qreal fullHeight = left->height();
    shell.insert("panelLayout", QVariantMap{
        {"columns", 100}, {"splitColumn", 50},
        {"leftBottomInsetRows", 2}, {"rightBottomInsetRows", 4},
    });
    scene.insert("shell", shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(left->height() < fullHeight);
    QVERIFY(right->height() < left->height());
    QVERIFY(backdrop->isVisible());
    QVERIFY(fixture.item("terminalDocumentSurface"));
    const qreal dpr = fixture.window->devicePixelRatio();
    for (auto *panel : {left, right}) {
        const QPointF bottom = panel->mapToItem(fixture.window->contentItem(), QPointF(0, panel->height()));
        QVERIFY(qAbs(bottom.y()*dpr - qRound64(bottom.y()*dpr)) < .001);
        auto *footer = fixture.item(panel == left ? "panelStatus-0" : "panelStatus-1");
        QVERIFY(footer);
        QList<QQuickItem *> pending{footer};
        int leaves = 0;
        while (!pending.isEmpty()) {
            auto *item = pending.takeLast();
            pending.append(item->childItems());
            if (!QByteArray(item->metaObject()->className()).startsWith("QQuickText")) continue;
            const QPointF origin = item->mapToItem(fixture.window->contentItem(), QPointF());
            qInfo() << item->objectName() << origin*dpr;
            QVERIFY(!item->objectName().isEmpty());
            QVERIFY(qAbs(origin.x()*dpr - qRound64(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr - qRound64(origin.y()*dpr)) < .001);
            QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
            ++leaves;
        }
        QVERIFY(leaves > 0);
    }
    auto *terminal = fixture.item("terminalDocumentSurface");
    QCOMPARE(terminal->height(), fixture.item("terminalExposedLeft")->height());
    QCOMPARE(fixture.item("terminalDocumentSurfaceRight")->height(), fixture.item("terminalExposedRight")->height());
    QCOMPARE(terminal->mapToItem(fixture.window->contentItem(), QPointF()).y(),
             fixture.item("terminalExposedLeft")->mapToItem(fixture.window->contentItem(), QPointF()).y());
    QTRY_COMPARE(terminal->property("reportedViewportRows").toInt(), 4);
    for (auto *surface : {terminal, fixture.item("terminalDocumentSurfaceRight")}) {
        auto *bar = surface->findChild<QQuickItem *>("documentScrollBar");
        QVERIFY(bar);
        const auto top = bar->mapToItem(surface, QPointF()).y();
        QVERIFY(top >= -.001);
        QVERIFY(top + bar->height() <= surface->height() + .001);
    }
    auto terminalLeaves = [&]() {
        QList<QQuickItem *> result;
        QList<QQuickItem *> pending{terminal, fixture.item("terminalDocumentSurfaceRight")};
        while (!pending.isEmpty()) {
            auto *leaf = pending.takeLast();
            pending.append(leaf->childItems());
            if ((leaf->objectName() == "documentPlainText" || leaf->objectName() == "documentRunText") && leaf->isVisible()
                && !leaf->property("text").toString().isEmpty()) result.append(leaf);
        }
        return result;
    };
    QTRY_VERIFY(!terminalLeaves().isEmpty());
    for (auto *leaf : terminalLeaves()) {
        const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        qInfo() << "terminal leaf" << origin*dpr;
        QVERIFY(qAbs(origin.x()*dpr - qRound64(origin.x()*dpr)) < .001);
        QVERIFY(qAbs(origin.y()*dpr - qRound64(origin.y()*dpr)) < .001);
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
    }
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    capture.save("D:/Code/f4-zoin/.diagnostics/shortened-panels-175.png");
    shell.insert("wide", true);
    shell.insert("widePanel", 1);
    scene.insert("shell", shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!left->isVisible());
    QVERIFY(backdrop->isVisible());
    shell.insert("panelLayout", QVariantMap{{"leftBottomInsetRows", 0}, {"rightBottomInsetRows", 0}});
    scene.insert("shell", shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!backdrop->isVisible());
    QCOMPARE(right->height(), fullHeight);
    shell.insert("wide", false);
    shell.insert("panelLayout", QVariantMap{{"leftBottomInsetRows", 2}, {"rightBottomInsetRows", 4}});
    shell.insert("infoPanels", QVariantList{QVariantMap{{"side", 0}, {"title", "Information"}, {"bottomHint", "Bytes"}}});
    auto preview = quickView(1, true, "shortened-preview", 0, 0, 1);
    preview.insert("bottomHint", "Quick View");
    shell.insert("quickViews", QVariantList{preview});
    scene.insert("shell", shell);
    fixture.shell.setScene(scene);
    for (const auto &name : {"infoPanelFooterText-0", "quickViewFooterText-1"}) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY((leaf = fixture.item(name)) != nullptr && leaf->isVisible());
        QTest::qWait(100);
        const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        qInfo() << name << origin*dpr;
        QVERIFY(qAbs(origin.x()*dpr-qRound64(origin.x()*dpr)) < .001);
        QVERIFY(qAbs(origin.y()*dpr-qRound64(origin.y()*dpr)) < .001);
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
    }
    fixture.window->grabWindow().save("D:/Code/f4-zoin/.diagnostics/shortened-alternate-panels-175.png");
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

    shell.insert(QStringLiteral("wide"), false);
    shell.remove(QStringLiteral("widePanel"));
    scene.insert(QStringLiteral("shell"), shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY_WITH_TIMEOUT(passivePanel->isVisible(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(widePanel->width() < fixture.window->width(), 3000);
}

void F4QuickViewSurfaceTests::terminalScrollBarStaysInsideTheExposedPanelSide()
{
    QVariantMap scene = shellScene();
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("showLeftPanel"), false);
    shell.insert(QStringLiteral("showRightPanel"), true);
    shell.insert(QStringLiteral("panelLayout"), QVariantMap{
        {QStringLiteral("columns"), 100},
        {QStringLiteral("splitColumn"), 40},
        {QStringLiteral("leftBottomInsetRows"), 0},
        {QStringLiteral("rightBottomInsetRows"), 0},
    });
    shell.insert(QStringLiteral("terminal"), QVariantMap{
        {QStringLiteral("id"), QStringLiteral("terminal-exposed-side")},
        {QStringLiteral("kind"), QStringLiteral("terminal")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("scrollAction"), QStringLiteral("terminal.scroll")},
        {QStringLiteral("selectionEnabled"), true},
        {QStringLiteral("windowRows"), visualRows(0, 90)},
        {QStringLiteral("windowStart"), 0},
        {QStringLiteral("windowEnd"), 90},
        {QStringLiteral("viewportStart"), 30},
        {QStringLiteral("viewportSpan"), 24},
        {QStringLiteral("viewportRow"), 30},
        {QStringLiteral("viewportRows"), 24},
        {QStringLiteral("contentExtent"), 10'000},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("windowGeneration"), 1},
    });
    scene.insert(QStringLiteral("shell"), shell);

    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *backdrop = fixture.item(QStringLiteral("terminalBackdrop"));
    QVERIFY(backdrop);
    QTRY_VERIFY_WITH_TIMEOUT(backdrop->isVisible(), 3000);
    QQuickItem *surface = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (surface = fixture.item(QStringLiteral("terminalDocumentSurface")))
            != nullptr,
        3000);
    auto *scrollBar = surface->findChild<QQuickItem *>(
        QStringLiteral("documentScrollBar"));
    QVERIFY(scrollBar);
    QTRY_VERIFY_WITH_TIMEOUT(scrollBar->isVisible(), 3000);

    const qreal splitX = backdrop->width() * 0.4;
    const qreal leftExposedBarRight = scrollBar->mapToItem(
        backdrop, QPointF(scrollBar->width(), 0)).x();
    QVERIFY(qAbs(leftExposedBarRight - splitX) < 0.01);
    QVERIFY(qAbs(backdrop->property("scrollBarRightInset").toReal()
                 - backdrop->width() * 0.6) < 0.01);

    shell.insert(QStringLiteral("showLeftPanel"), true);
    shell.insert(QStringLiteral("showRightPanel"), false);
    scene.insert(QStringLiteral("shell"), shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(backdrop->property("scrollBarRightInset").toReal()) < 0.01,
        3000);
    QTest::qWait(100);
    auto *rightSurface = fixture.item("terminalDocumentSurfaceRight");
    QVERIFY(rightSurface);
    auto *rightBar = rightSurface->findChild<QQuickItem *>("documentScrollBar");
    QVERIFY(rightBar);
    qInfo() << "right bar" << rightBar->mapToItem(backdrop, QPointF(rightBar->width(),0)) << backdrop->width() << rightSurface->width() << rightSurface->property("scrollBarRightInset");
    QTRY_VERIFY_WITH_TIMEOUT(
        qAbs(rightBar->mapToItem(backdrop,
                                 QPointF(rightBar->width(), 0)).x()
             - backdrop->width()) < 0.01,
        3000);
    // Exercise the real terminal backdrop with both panels hidden, not only
    // a standalone document fixture with its embedded flag set.
    shell.insert("showLeftPanel", false);
    shell.insert("showRightPanel", false);
    shell.insert("terminalActive", true);
    scene.insert("shell", shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY((surface = fixture.item("terminalDocumentSurface")) != nullptr);
    QTRY_VERIFY(surface->property("middleAutoScrollAllowed").toBool());
    QTest::qWait(100);
    auto *middle = surface->findChild<QQuickItem *>("documentMiddleButtonArea");
    QVERIFY(middle);
    QTest::mouseClick(fixture.window, Qt::MiddleButton, Qt::NoModifier,
        middle->mapToItem(fixture.window->contentItem(), QPointF(middle->width()/2,middle->height()/2)).toPoint());
    QTRY_VERIFY(surface->property("middleAutoScrollActive").toBool());
    shell.insert("showLeftPanel", true);
    shell.insert("showRightPanel", true);
    shell.insert("terminalActive", false);
    scene.insert("shell", shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!fixture.item("terminalDocumentSurface"));

}

void F4QuickViewSurfaceTests::autocompleteHoverFollowsPopupResize()
{
    QVariantMap scene = shellScene();
    scene.insert("menus", QVariantList{QVariantMap{
        {"id", "autocomplete-menu"}, {"kind", "menu"}, {"role", "autocomplete"},
        {"query", "git st"}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"text", ""}, {"rawText", "git st"}},
                               QVariantMap{{"text", "git status"}, {"rawText", "git status"}},
                               QVariantMap{{"text", "git stash"}, {"rawText", "git stash"}}}},
    }});
    auto shell = scene.value("shell").toMap();
    QVariantMap command{{"visible", true}, {"multiline", true}, {"text", "first"}, {"cursorPosition", 5}};
    shell.insert("commandLine", command);
    scene.insert("shell", shell);
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *row = nullptr;
    QTRY_VERIFY((row = visualItemWithObjectName(fixture.window->contentItem(), QStringLiteral("autocompleteHint-1"))));
    QVERIFY(QTest::qWaitForWindowExposed(fixture.window));
    QTest::qWait(100);
    const QPoint originalPointer = QCursor::pos();
    const auto restorePointer = qScopeGuard([&] { QCursor::setPos(originalPointer); });
    const auto point = row->mapToScene(QPointF(12, 7)).toPoint();
    QCursor::setPos(fixture.window->mapToGlobal(point));
    QTest::mouseMove(fixture.window, point);
    QTRY_COMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 1);
    const qreal offset = fixture.window->mapFromGlobal(QCursor::pos()).y() - row->mapToScene(QPointF()).y();
    fixture.shell.clearActions();
    auto *hoverPopup = visualItemWithObjectName(fixture.window->contentItem(), QStringLiteral("autocompleteOverlay"));
    QVERIFY(hoverPopup);
    // Typing resets selection; delegate-local hover notifications must not
    // retake it when the physical pointer has not moved.
    fixture.window->setProperty("autocompleteSelectedIndex", 0);
    fixture.shell.clearActions();
    const auto stationary = row->mapFromScene(fixture.window->mapFromGlobal(QCursor::pos()));
    QVERIFY(QMetaObject::invokeMethod(hoverPopup, "hoverRow",
        Q_ARG(QVariant, QVariant::fromValue(row)),
        Q_ARG(QVariant, stationary.x()), Q_ARG(QVariant, stationary.y())));
    QCOMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 0);
    QCOMPARE(fixture.shell.actions.size(), 0);
    QCursor::setPos(QCursor::pos() + QPoint(2, 0));
    QTest::mouseMove(fixture.window, fixture.window->mapFromGlobal(QCursor::pos()));
    QTRY_COMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 1);
    fixture.shell.clearActions();
    for (int height : {580, 640, 560, 640}) {
        fixture.window->resize(900, height);
        QTest::qWait(100);
        const qreal newOffset = fixture.window->mapFromGlobal(QCursor::pos()).y() - row->mapToScene(QPointF()).y();
        QVERIFY2(qAbs(newOffset - offset) <= 1, qPrintable(QString::number(newOffset - offset)));
        QCOMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 1);
        // A hover queued before the warp must not select a different row.
        auto *popup = visualItemWithObjectName(fixture.window->contentItem(), QStringLiteral("autocompleteOverlay"));
        auto *other = visualItemWithObjectName(fixture.window->contentItem(), QStringLiteral("autocompleteHint-2"));
        QVERIFY(popup);
        QVERIFY(other);
        const auto stale = other->mapFromScene(point);
        if (!F4PointerRowAnchor::eventIsCurrent(other, stale.x(), stale.y())) {
            QVERIFY(QMetaObject::invokeMethod(popup, "hoverRow",
                Q_ARG(QVariant, QVariant::fromValue(other)),
                Q_ARG(QVariant, stale.x()), Q_ARG(QVariant, stale.y())));
            QCOMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 1);
        }
    }
    // Exercise the actual command-line height publication too.
    for (const auto &text : {QString("first\nsecond"), QString("first"), QString("first\nsecond"), QString("first")}) {
        const qreal previousY = row->mapToScene(QPointF()).y();
        command.insert("text", text);
        command.insert("cursorPosition", text.size());
        shell.insert("commandLine", command);
        scene.insert("shell", shell);
        fixture.shell.setScene(scene);
        QTRY_VERIFY(qAbs(row->mapToScene(QPointF()).y() - previousY) > 1);
        QTest::qWait(100);
        const qreal newOffset = fixture.window->mapFromGlobal(QCursor::pos()).y() - row->mapToScene(QPointF()).y();
        QVERIFY(qAbs(newOffset - offset) <= 1);
        QCOMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 1);
    }
    QCOMPARE(fixture.shell.actions.size(), 0);
}

void F4QuickViewSurfaceTests::autocompleteOutsideClickDismissesWithoutChangingText()
{
    QVariantMap scene = shellScene();
    scene.insert("menus", QVariantList{QVariantMap{
        {"id", "autocomplete-menu"}, {"kind", "menu"}, {"role", "autocomplete"},
        {"query", "git st"}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"text", ""}, {"rawText", "git st"}},
                               QVariantMap{{"text", "git status"}, {"rawText", "git status"}}}},
    }});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *popup = nullptr;
    QTRY_VERIFY((popup = visualItemWithObjectName(fixture.window->contentItem(), QStringLiteral("autocompleteOverlay"))));
    QTRY_VERIFY(popup->width() > 0);
    QVERIFY(QTest::qWaitForWindowExposed(fixture.window));
    QTest::qWait(100);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      QPoint(fixture.window->width() / 2, fixture.window->height() / 2));
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    const auto intent = fixture.shell.actions.last();
    QCOMPARE(intent.value("action").toString(), QString("command.complete"));
    QCOMPARE(intent.value("target").toString(), QString("shell"));
    QVERIFY(!intent.contains("text"));
}

void F4QuickViewSurfaceTests::autocompleteSelectionPreviewsAndRestoresQuery()
{
    QVariantMap scene = shellScene();
    scene.insert("menus", QVariantList{QVariantMap{
        {"id", "autocomplete-menu"}, {"kind", "menu"}, {"role", "autocomplete"},
        {"query", "git st"}, {"selected", 0},
        {"items", QVariantList{
            QVariantMap{{"text", ""}, {"rawText", "git st"}},
            QVariantMap{{"text", "git status"}, {"rawText", "git status"}},
            QVariantMap{{"text", "git stash"}, {"rawText", "git stash"}}}},
    }});
    QuickViewFixture fixture(scene);
    QVERIFY(fixture.window);
    QTRY_COMPARE(fixture.window->property("autocompleteSelectedIndex").toInt(), 0);
    fixture.shell.clearActions();
    fixture.window->setProperty("autocompleteSelectedIndex", 1);
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("command.preview"));
    QCOMPARE(fixture.shell.actions.last().value("text").toString(), QString("git status"));
    fixture.window->setProperty("autocompleteSelectedIndex", 0);
    QTRY_COMPARE(fixture.shell.actions.size(), 2);
    QCOMPARE(fixture.shell.actions.last().value("text").toString(), QString("git st"));
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "completeAutocomplete"));
    QTRY_COMPARE(fixture.shell.actions.size(), 3);
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("command.complete"));
    QCOMPARE(fixture.shell.actions.last().value("text").toString(), QString("git st"));
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
    // A fractional movement inside row 20 must not request that same row.
    QTest::qWait(260);
    QCOMPARE(fixture.shell.actions.size(), 0);
    sendPixelWheel(fixture.window, center.toPoint(), -7);
    QTRY_VERIFY_WITH_TIMEOUT(qAbs(topVisualRow(surface, list) - 21.0) < 0.02,
                             1000);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.shell.actions.size(), 1, 1500);
    const QVariantMap request = fixture.shell.actions.constFirst();
    QCOMPARE(request.value(QStringLiteral("action")).toString(),
             QStringLiteral("quickView.scroll"));
    QCOMPARE(request.value(QStringLiteral("target")).toString(),
             QStringLiteral("quick-view-0"));
    QCOMPARE(request.value(QStringLiteral("contentKey")).toString(),
             QStringLiteral("file-A"));
    QCOMPARE(request.value(QStringLiteral("visualRow")).toInt(), 21);
    QCOMPARE(request.value(QStringLiteral("generation")).toInt(), 5);
    QVERIFY(surface->property("windowRequestPending").toBool());

    QTest::qWait(260);
    QCOMPARE(fixture.shell.actions.size(), 1);

    QVariantMap surfaceMap = qv.value(QStringLiteral("surface")).toMap();
    surfaceMap.insert(QStringLiteral("windowGeneration"), 5);
    surfaceMap.insert(QStringLiteral("viewportStart"), 21);
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
    sendPixelWheel(fixture.window, center.toPoint(), -40);
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

void F4QuickViewSurfaceTests::panelStatusLeavesStayOnPhysicalPixelGrid()
{
    auto scene = shellScene({}, 0);
    auto shell = scene.value("shell").toMap();
    auto panels = shell.value("panels").toList();
    auto status = panels[0].toMap();
    status.insert("selectedFiles", 2);
    status.insert("selectedDirectories", 1);
    status.insert("selectedSize", 1536);
    status.insert("totalFiles", 10);
    status.insert("totalDirectories", 2);
    status.insert("totalSize", 4096);
    status.insert("freeSpace", 1048576);
    status.insert("diskTotalSpace", 4194304);
    status.insert("symlinkTarget", "D:/long/symlink/target/example.txt");
    QuickViewFixture fixture(scene, true);
    QVERIFY(fixture.window);
    const auto dpr = fixture.window->devicePixelRatio();
    if (qEnvironmentVariable("QT_SCALE_FACTOR") == "1.75") QCOMPARE(dpr, 1.75);
    auto *footer = fixture.item("panelStatus-0");
    auto *panelItem = fixture.item("filePanel-0");
    auto *loader = fixture.item("galleryPanelContent-0");
    QVERIFY(footer && panelItem && loader);
    for (const int windowWidth : {1300, 720}) {
        fixture.window->resize(windowWidth, 900);
        for (const bool selected : {false, true}) {
            for (const bool known : {false, true}) {
                status["selectedCount"] = selected ? 3 : 0;
                status["freeSpaceKnown"] = known;
                status["showFileInfo"] = !selected;
                panels[0] = status; shell["panels"] = panels; scene["shell"] = shell;
                fixture.shell.setScene(scene);
                QTest::qWait(80);
                QVERIFY(footer->isVisible());
                QCOMPARE(footer->property("selectionActive").toBool(), selected);
                QVERIFY(!fixture.item("panelStatusSelection-0"));
                for (const auto *metric : {"Files", "Folders", "Size"}) {
                    const auto name = QString("panelStatus%1-0").arg(metric);
                    const auto expected = fixture.window->property(selected ? "gallerySelectionColor" : "mutedText").value<QColor>();
                    QCOMPARE(fixture.item(name)->property("iconColor").value<QColor>(), expected);
                    const auto iconSource = fixture.item(name + "Icon")->property("source").toUrl();
                    QCOMPARE(QColor(QUrlQuery(iconSource).queryItemValue("color")), expected);
                    QCOMPARE(fixture.item(name + "Text")->property("color").value<QColor>(), expected);
                    for (const auto *suffix : {"Icon", "Text"}) {
                        qreal opacity = 1;
                        for (auto *item = fixture.item(name + suffix); item; item = item->parentItem())
                            opacity *= item->opacity();
                        QCOMPARE(opacity, selected ? 0.95 : 1.0);
                    }
                }
                QCOMPARE(fixture.item("panelStatusFiles-0")->property("text").toString(), selected ? "2" : "10");
                QCOMPARE(fixture.item("panelStatusFolders-0")->property("text").toString(), selected ? "1" : "2");
                QVERIFY(qAbs(loader->y()+loader->height()-panelItem->height()) < .01);
                QVERIFY(footer->y()+footer->height() <= panelItem->height());
                QVERIFY(footer->x()+footer->width() <= panelItem->width());
                QVERIFY(footer->x() >= 0 && footer->y() >= loader->y());
                auto *track = fixture.item("panelStatusSpaceTrack-0");
                auto *fill = fixture.item("panelStatusSpaceFill-0");
                QVERIFY(track && fill);
                QCOMPARE(track->isVisible(), known);
                if (known) {
                    QVERIFY(qAbs(fill->width()-track->width()*3/4)*dpr <= 1);
                    const auto occupiedColor = fixture.window->property("controlBorder").value<QColor>();
                    QCOMPARE(fill->property("color").value<QColor>(), occupiedColor);
                    QCOMPARE(track->property("color").value<QColor>(), occupiedColor.darker(160));
                }
                QList<QQuickItem *> pending{footer};
                int leaves = 0;
                while (!pending.isEmpty()) {
                    auto *item = pending.takeLast();
                    pending.append(item->childItems());
                    if (!item->isVisible()) continue;
                    const bool leaf = item->inherits("QQuickText") || item->inherits("QQuickImage");
                    if (!leaf && item != footer && item != track && item != fill) continue;
                    QVERIFY(!item->objectName().isEmpty());
                    const auto origin = item->mapToItem(fixture.window->contentItem(), QPointF());
                    const auto physical = origin*dpr;
                    QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .01 && qAbs(physical.y()-qRound64(physical.y())) < .01,
                        qPrintable(QString("%1 physical %2,%3").arg(item->objectName()).arg(physical.x()).arg(physical.y())));
                    QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin, QPointF(1,0));
                    QCOMPARE(item->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin, QPointF(0,1));
                    if (leaf) {
                        const auto local = item->mapToItem(footer, QPointF());
                        QVERIFY(local.x() >= 0 && local.y() >= 0);
                        QVERIFY(local.x()+item->width() <= footer->width()+1/dpr);
                        QVERIFY(local.y()+item->height() <= footer->height()+1/dpr);
                        if (item->inherits("QQuickImage")) QTRY_COMPARE(item->property("status").toInt(), 1);
                        ++leaves;
                    } else {
                        QVERIFY(qAbs(item->width()*dpr-qRound64(item->width()*dpr)) < .01);
                        QVERIFY(qAbs(item->height()*dpr-qRound64(item->height()*dpr)) < .01);
                    }
                }
                QVERIFY(leaves >= 6);
                const auto capture = fixture.window->grabWindow();
                QVERIFY(!capture.isNull());
                if (known) {
                    const auto origin = track->mapToItem(fixture.window->contentItem(), QPointF()) * dpr;
                    const int y = qFloor(origin.y() + track->height()*dpr/2);
                    QCOMPARE(capture.pixelColor(qFloor(origin.x() + track->width()*dpr/4), y).rgba(),
                        fill->property("color").value<QColor>().rgba());
                    QCOMPARE(capture.pixelColor(qFloor(origin.x() + track->width()*dpr*0.9), y).rgba(),
                        track->property("color").value<QColor>().rgba());
                }
                if (qEnvironmentVariableIsSet("F4_PANEL_STATUS_CAPTURE"))
                    QVERIFY(capture.save(qEnvironmentVariable("F4_PANEL_STATUS_CAPTURE")
                        + QString("-%1-%2-%3.png").arg(windowWidth).arg(selected).arg(known)));
            }
        }
    }
    // Empty/full and missing capacity must be safe without a fabricated ratio.
    for (const int free : {-1048576, 0, 4194304, 8388608}) {
        status["freeSpace"] = free; status["freeSpaceKnown"] = true;
        panels[0] = status; shell["panels"] = panels; scene["shell"] = shell; fixture.shell.setScene(scene);
        QTRY_COMPARE(footer->property("usedFraction").toDouble(), free <= 0 ? 1.0 : 0.0);
        const auto trackWidth = fixture.item("panelStatusSpaceTrack-0")->width();
        QTRY_COMPARE(fixture.item("panelStatusSpaceFill-0")->width(), free <= 0 ? trackWidth : 0.0);
    }
    status["diskTotalSpace"] = 0;
    panels[0] = status; shell["panels"] = panels; scene["shell"] = shell; fixture.shell.setScene(scene);
    QTRY_VERIFY(!fixture.item("panelStatusSpaceTrack-0")->isVisible());
    QCOMPARE(footer->property("usedFraction").toDouble(), 0.0);
}

void F4QuickViewSurfaceTests::sortGroupLeavesStayOnPhysicalPixelGrid()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);
    auto *menu = fixture.window->findChild<QObject *>("panelSortMenu-0");
    QVERIFY(menu);
    QVERIFY(QMetaObject::invokeMethod(menu, "open"));
    QTest::qWait(100);
    auto verifyLeaf = [&](QQuickItem *leaf) {
        QVERIFY(leaf);
        const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        qInfo() << leaf->objectName() << physical;
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < .001 && qAbs(physical.y() - qRound64(physical.y())) < .001,
                 qPrintable(QString("%1 %2,%3").arg(leaf->objectName()).arg(physical.x()).arg(physical.y())));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0)) - origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1)) - origin, QPointF(0,1));
    };
    auto *group = visualItemWithObjectNamePrefix(fixture.window->contentItem(), "panelSortChoice-groups-0");
    QVERIFY(group);
    QList<QQuickItem *> pending{group};
    while (!pending.isEmpty()) {
        auto *leaf = pending.takeLast();
        pending.append(leaf->childItems());
        const QByteArray type = leaf->metaObject()->className();
        if (leaf->isVisible() && (type.startsWith("QQuickText") || type.contains("Image"))) verifyLeaf(leaf);
    }
    QVERIFY(!fixture.window->grabWindow().isNull());
    fixture.window->grabWindow().save("D:/Code/f4-zoin/.diagnostics/qt-upstream-sort.png");
    QVERIFY(QMetaObject::invokeMethod(menu, "close"));
}

void F4QuickViewSurfaceTests::quickSearchPaletteDefaultAndResetArePink()
{
    QuickViewFixture fixture(shellScene(), true, true);
    QVERIFY(fixture.window);
    const QColor pink(QStringLiteral("#c678dd"));
    const QColor initial = fixture.window->property("galleryQuickSearchMatchColor").value<QColor>();
    QVERIFY(fixture.window->setProperty("galleryQuickSearchMatchColor", QColor("#25a244")));
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "resetThemeToDefaults"));
    const QColor reset = fixture.window->property("galleryQuickSearchMatchColor").value<QColor>();
    QCOMPARE(reset, pink);
    QCOMPARE(initial, pink);
}

void F4QuickViewSurfaceTests::commandMenusKeepPanelCursorWhileBlockingInput()
{
    auto scene = shellScene({}, 0);
    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    auto *loader = fixture.item("galleryPanelContent-0");
    QVERIFY(loader);
    QTRY_VERIFY(loader->property("item").value<QObject *>());
    auto *host = loader->property("item").value<QObject *>();
    QVERIFY(host->property("panelActive").toBool());
    QVERIFY(host->property("showCursor").toBool());
    const auto menu = titledUserMenu("Files", true);
    fixture.shell.clearActions();
    for (int cycle = 0; cycle < 2; ++cycle) {
        fixture.shell.setCommandMenus({menu});
        QTRY_VERIFY(visualItemWithObjectNamePrefix(fixture.window->contentItem(),
            "semanticMenuPopup-user-menu"));
        QVERIFY(!host->property("panelActive").toBool());
        QVERIFY2(host->property("showCursor").toBool(),
                 "command menu must suppress input without hiding the panel cursor");
        fixture.shell.setCommandMenus({});
        QTRY_VERIFY(host->property("panelActive").toBool());
        QVERIFY(host->property("showCursor").toBool());
        QCOMPARE(loader->property("item").value<QObject *>(), host);
    }
    // A dialog remains a paint blocker even when its combo menu is present.
    scene.insert("dialogs", QVariantList{QVariantMap{
        {"id", "cursor-blocking-dialog"}, {"kind", "dialog"},
        {"title", "Dialog"}, {"modal", true},
        {"x", 2}, {"y", 2}, {"w", 30}, {"h", 10}}});
    scene.insert("menus", QVariantList{menu});
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!host->property("panelActive").toBool());
    QVERIFY(!host->property("showCursor").toBool());
    for (const auto &action : fixture.shell.actions)
        QVERIFY(action.value("action").toString() != "panel.cursor");
}

void F4QuickViewSurfaceTests::uriBreadcrumbKeepsSchemeTogetherAndNavigates()
{
    QuickViewFixture fixture(shellScene({}, 0), true, true);
    QVERIFY(fixture.window);
    fixture.window->resize(1800, 640);
    auto *control = fixture.item("panelPathTitle-0");
    QVERIFY(control);
    // The software renderer cannot render the optional shader fade mask.
    control->setProperty("breadcrumbMaskEnabled", false);
    const QString path = "registry://HKEY_LOCAL_MACHINE/SOFTWARE";
    control->setProperty("text", path);
    control->setProperty("navigationPath", path);
    QQuickItem *root = nullptr;
    QTRY_VERIFY((root = visualItemWithObjectNamePrefix(control, "pathBreadcrumbRoot-text")));
    QTRY_COMPARE(root->property("text").toString(), QString("registry://"));
    QTest::qWait(100);
    for (int i = -1; i < 2; ++i) {
        const QString id = i < 0 ? "pathBreadcrumbRoot" : QString("pathBreadcrumb-%1").arg(i);
        const QString label = i < 0 ? "registry://" : i == 0 ? "HKEY_LOCAL_MACHINE" : "SOFTWARE";
        auto *text = visualItemWithObjectNamePrefix(control, id + "-text");
        QVERIFY(text);
        QCOMPARE(text->property("text").toString(), label);
        for (const QString &suffix : {"-text", "-separator"}) {
            auto *leaf = visualItemWithObjectNamePrefix(control, id + suffix);
            QVERIFY(leaf);
            if (!leaf->isVisible()) continue;
            const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
            const auto physical = origin * fixture.window->devicePixelRatio();
            QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < .001 && qAbs(physical.y() - qRound64(physical.y())) < .001,
                     qPrintable(QString("%1 %2,%3").arg(leaf->objectName()).arg(physical.x()).arg(physical.y())));
            QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0)) - origin, QPointF(1,0));
            QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1)) - origin, QPointF(0,1));
        }
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
            text->mapToItem(fixture.window->contentItem(), QPointF(text->width()/2, text->height()/2)).toPoint());
        QTRY_COMPARE(fixture.shell.actions.size(), 1);
        QCOMPARE(fixture.shell.actions[0].value("path").toString(), i < 0 ? QString("registry://")
                 : i == 0 ? QString("registry://HKEY_LOCAL_MACHINE") : path);
    }
    QVERIFY(!visualItemWithObjectNamePrefix(control, "pathBreadcrumb-2-text"));
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    capture.save("D:/Code/f4-zoin/.diagnostics/uri-breadcrumb-175.png");
    for (const QString &prefix : {"registry://", "sftp://", "custom+v1.test://"}) {
        control->setProperty("text", prefix);
        control->setProperty("navigationPath", "");
        QCoreApplication::processEvents();
        QCOMPARE(root->property("text").toString(), prefix);
        fixture.shell.clearActions();
        QVERIFY(QMetaObject::invokeMethod(control, "folderClicked", Q_ARG(QVariant, QVariant(""))));
        QCOMPARE(fixture.shell.actions.last().value("path").toString(), prefix);
    }
}

void F4QuickViewSurfaceTests::deviceBreadcrumbLabelsPreserveCanonicalNavigationAt175Percent_data()
{
    QTest::addColumn<QString>("path");
    QTest::addColumn<QString>("title");
    QTest::addColumn<QString>("rootLabel");
    QTest::addColumn<QString>("icon");
    QTest::addColumn<QStringList>("children");
    QTest::newRow("ai-root")
        << QString("ai://") << QString("ai://")
        << QString("AI") << QString("sparkles") << QStringList{};
    QTest::newRow("ai-child")
        << QString("ai://ctx/nested") << QString("ai://ctx/nested")
        << QString("AI") << QString("sparkles") << QStringList{"ctx", "nested"};
    QTest::newRow("android-manager")
        << QString("android://") << QString("Android devices")
        << QString("Android") << QString("android-logo") << QStringList{};
    QTest::newRow("ios-manager")
        << QString("ios://") << QString("Apple mobile devices")
        << QString("iOS") << QString("apple-logo") << QStringList{};
    QTest::newRow("android-child")
        << QString("android://Pixel 3/sdcard/DCIM")
        << QString("android://Pixel 3/sdcard/DCIM")
        << QString("Android") << QString("android-logo")
        << QStringList{"Pixel 3", "sdcard", "DCIM"};
    QTest::newRow("ios-child")
        << QString::fromUtf8("ios://Alexander’s iPhone/DCIM/100APPLE")
        << QString::fromUtf8("ios://Alexander’s iPhone/DCIM/100APPLE")
        << QString("iOS") << QString("apple-logo")
        << QStringList{QString::fromUtf8("Alexander’s iPhone"), "DCIM", "100APPLE"};
}

void F4QuickViewSurfaceTests::deviceBreadcrumbLabelsPreserveCanonicalNavigationAt175Percent()
{
    QFETCH(QString, path);
    QFETCH(QString, title);
    QFETCH(QString, rootLabel);
    QFETCH(QString, icon);
    QFETCH(QStringList, children);
    auto scene = shellScene({}, 0);
    auto shell = scene.value("shell").toMap();
    auto panels = shell.value("panels").toList();
    auto panel = panels[0].toMap();
    panel.insert("path", path);
    panel.insert("title", title);
    panel.insert("pathIcon", icon);
    panels[0] = panel;
    shell.insert("panels", panels);
    scene.insert("shell", shell);
    QuickViewFixture fixture(scene, true, true);
    QVERIFY(fixture.window);
    fixture.window->resize(1800, 640);
    const qreal dpr = fixture.window->devicePixelRatio();
    QVERIFY2(qAbs(dpr - 1.75) < .001, "Run with QT_SCALE_FACTOR=1.75");
    auto *control = fixture.item("panelPathTitle-0");
    QVERIFY(control);
    control->setProperty("breadcrumbMaskEnabled", false);
    auto *content = fixture.window->contentItem();
    QQuickItem *root = nullptr;
    QTRY_VERIFY((root = visualItemWithObjectNamePrefix(control, "pathBreadcrumbRoot-text")));
    QTest::qWait(100);
    QImage capture;
    QTRY_VERIFY_WITH_TIMEOUT(!(capture = fixture.window->grabWindow()).isNull(), 3000);
    QVERIFY(capture.save("/tmp/f4-device-breadcrumb-175.png"));

    const auto verifyLeaf = [content, dpr](QQuickItem *leaf) {
        QVERIFY(leaf);
        QVERIFY(leaf->isVisible());
        const QPointF origin = leaf->mapToItem(content, QPointF());
        const QPointF physical = origin * dpr;
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < .001
                     && qAbs(physical.y() - qRound64(physical.y())) < .001,
                 qPrintable(QString("%1 scene origin = (%2, %3) physical px")
                     .arg(leaf->objectName()).arg(physical.x(), 0, 'f', 6)
                     .arg(physical.y(), 0, 'f', 6)));
        QCOMPARE(leaf->mapToItem(content, QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(leaf->mapToItem(content, QPointF(0, 1)) - origin, QPointF(0, 1));
    };
    auto *driveIcon = fixture.item("panelDriveButtonIcon-0");
    QVERIFY(driveIcon);
    QVERIFY2(driveIcon->property("source").toUrl().toString().contains(icon),
             qPrintable(driveIcon->property("source").toUrl().toString()));
    verifyLeaf(driveIcon);
    QCOMPARE(control->property("navigationPath").toString(), path);
    QCOMPARE(root->property("text").toString(), rootLabel);
    const QString scheme = path.left(path.indexOf("://") + 3);
    for (int index = -1; index < children.size(); ++index) {
        const QString id = index < 0 ? "pathBreadcrumbRoot"
                                     : QString("pathBreadcrumb-%1").arg(index);
        auto *text = visualItemWithObjectNamePrefix(control, id + "-text");
        QVERIFY(text);
        QCOMPARE(text->property("text").toString(), index < 0 ? rootLabel : children[index]);
        verifyLeaf(text);
        auto *separator = visualItemWithObjectNamePrefix(control, id + "-separator");
        QVERIFY(separator);
        if (separator->isVisible())
            verifyLeaf(separator);
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
            text->mapToItem(content, QPointF(text->width() / 2, text->height() / 2)).toPoint());
        QTRY_COMPARE(fixture.shell.actions.size(), 1);
        QCOMPARE(fixture.shell.actions[0].value("action").toString(), QString("panel.navigatePath"));
        QCOMPARE(fixture.shell.actions[0].value("path").toString(),
                 scheme + children.mid(0, index + 1).join('/'));
    }
    QVERIFY(!visualItemWithObjectNamePrefix(control,
        QString("pathBreadcrumb-%1-text").arg(children.size())));
    control->setProperty("editMode", true);
    auto *field = visualItemWithObjectNamePrefix(control, "pathField");
    QVERIFY(field);
    QTRY_COMPARE(field->property("text").toString(), path);
    verifyLeaf(field);
    QTest::qWait(50);
    const auto editCapture = fixture.window->grabWindow();
    QVERIFY(!editCapture.isNull());
    QVERIFY(editCapture.save("/tmp/f4-device-path-editor-175.png"));
    QCOMPARE(control->property("navigationPath").toString(), path);
    if (!QTest::currentTestFailed()) {
        qInfo().noquote() << "[FIX:device-breadcrumb] scheme=" + scheme
                         << "rootLabel=" + rootLabel << "dpr=" << dpr;
    }
}

void F4QuickViewSurfaceTests::pluginPathIconFollowsPanelOnPhysicalGrid()
{
    auto sceneWithIcon = [](const QString &icon) {
        auto scene = shellScene({}, 0);
        auto shell = scene.value("shell").toMap();
        auto panels = shell.value("panels").toList();
        auto panel = panels[0].toMap();
        panel.insert("pathIcon", icon);
        panel.insert("path", "registry://HKEY_LOCAL_MACHINE/SOFTWARE");
        panel.insert("title", "registry://HKEY_LOCAL_MACHINE/SOFTWARE");
        panels[0] = panel; shell.insert("panels", panels); scene.insert("shell", shell);
        return scene;
    };
    QuickViewFixture fixture(sceneWithIcon("blocks"), true, true);
    QVERIFY(fixture.window);
    fixture.window->resize(1800, 640);
    auto *control = fixture.item("panelPathTitle-0");
    QVERIFY(control);
    control->setProperty("breadcrumbMaskEnabled", false);
    for (const QString &name : {"blocks", "android-logo", "apple-logo", "cloud", "network", "archive", "plug", ""}) {
        fixture.shell.setScene(sceneWithIcon(name));
        QTest::qWait(80);
        auto *leaf = fixture.item("panelDriveButtonIcon-0");
        QVERIFY(leaf);
        QVERIFY2(leaf->property("source").toUrl().toString().contains(name.isEmpty() ? "hard-drive" : name),
                 qPrintable(leaf->property("source").toUrl().toString()));
        const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY2(qAbs(physical.x()-qRound64(physical.x()))<.001 && qAbs(physical.y()-qRound64(physical.y()))<.001,
            qPrintable(QString("%1 %2,%3").arg(name).arg(physical.x()).arg(physical.y())));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin,QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin,QPointF(0,1));
        QVERIFY(qAbs(leaf->width()*fixture.window->devicePixelRatio()-qRound64(leaf->width()*fixture.window->devicePixelRatio()))<.001);
        if (name == "blocks" || name == "android-logo") {
            auto capture = fixture.window->grabWindow();
            QVERIFY(!capture.isNull());
            capture.save("D:/Code/f4-zoin/.diagnostics/path-icon-"+name+"-175.png");
        }
    }
}

void F4QuickViewSurfaceTests::semanticTableDialog()
{
    auto scene = shellScene({}, 0);
    QVariantList rows;
    for (int i=0; i<50; ++i) rows.append(QVariantMap{{"cells", QStringList{QString("Command %1").arg(i), "Ctrl+F1", "Shell"}}});
    scene.insert("dialogs", QVariantList{QVariantMap{
        {"id", "hotkeys"}, {"kind", "dialog"}, {"title", "Hotkey configuration"},
        {"x", 2}, {"y", 2}, {"w", 80}, {"h", 25}, {"modal", true},
        {"children", QVariantList{QVariantMap{
            {"id", "keys"}, {"kind", "table"}, {"x", 4}, {"y", 4}, {"w", 74}, {"h", 18},
            {"visible", true}, {"focused", true}, {"quickSearch", true}, {"showHeader", true}, {"cursor", 0},
            {"columns", QVariantList{QVariantMap{{"title", "Command"}, {"width", 30}}, QVariantMap{{"title", "Key"}, {"width", 20}}, QVariantMap{{"title", "Area"}, {"width", 10}}}},
            {"rows", rows}
        }}}
    }});
    QuickViewFixture fixture(scene, true);
    QVERIFY(fixture.window);
    QTRY_VERIFY(visualItemWithText(fixture.window->contentItem(), "Command 0"));
    QTest::qWait(150);
    const auto dpr=fixture.window->devicePixelRatio();
    for (int column = 1; column < 3; ++column) {
        auto *divider = visualItemWithObjectName(fixture.window->contentItem(),
            QString("dialogWidget-keysTableDivider-%1").arg(column));
        QVERIFY(divider);
        const auto origin = divider->mapToItem(fixture.window->contentItem(), QPointF());
        QVERIFY(qAbs(origin.x()*dpr - qRound64(origin.x()*dpr)) < .001);
        QVERIFY(qAbs(origin.y()*dpr - qRound64(origin.y()*dpr)) < .001);
        QCOMPARE(divider->width()*dpr, 1.0);
        QVERIFY(qAbs(divider->height()*dpr - qRound64(divider->height()*dpr)) < .001);
    }

    QList<QQuickItem*> pending{fixture.window->contentItem()};
    int leaves=0;
    while (!pending.isEmpty()) {
        auto *leaf=pending.takeLast(); pending.append(leaf->childItems());
        if (!leaf->objectName().startsWith("dialogWidget-keysTable") || !QByteArray(leaf->metaObject()->className()).startsWith("QQuickText")) continue;
        const auto origin=leaf->mapToItem(fixture.window->contentItem(), QPointF());
        qInfo()<<leaf->objectName()<<origin*dpr;
        QVERIFY(qAbs(origin.x()*dpr-qRound64(origin.x()*dpr))<.001);
        QVERIFY(qAbs(origin.y()*dpr-qRound64(origin.y()*dpr))<.001);
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-origin,QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-origin,QPointF(0,1));
        ++leaves;
    }
    QVERIFY(leaves>=7);
    QVERIFY(fixture.window->grabWindow().save(".diagnostics/hotkey-table-175.png"));
}

void F4QuickViewSurfaceTests::liveSelectionStatusUsesCompactPatches()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);
    auto *footer = fixture.item("panelStatus-0");
    auto *files = fixture.item("panelStatusFiles-0Text");
    auto *loader = fixture.item("galleryPanelContent-0");
    QVERIFY(footer && files && loader);
    QObject *content = loader->property("item").value<QObject *>();
    QVERIFY(content);
    auto *panel = fixture.item("filePanel-0");
    QVERIFY(panel);
    QVariantMap state = panel->property("panel").toMap();
    state.remove("entries");
    state.remove("highlightStyles");
    QSignalSpy scenes(&fixture.shell, &TestShell::sceneChanged);
    const QSizeF contentSize = loader->size();
    QList<qint64> durations;
    for (int selected = 1; selected <= 100; ++selected) {
        state["selectedCount"] = selected;
        state["selectedFiles"] = selected;
        state["selectedSize"] = selected * 1024;
        QElapsedTimer elapsed;
        elapsed.start();
        fixture.shell.deliverCompactPresentation({
            {"type", "scene_patch"}, {"side", 0}, {"panel", state}});
        QCOMPARE(files->property("text").toString(), QString::number(selected));
        QVERIFY(footer->property("selectionActive").toBool());
        QCOMPARE(loader->property("item").value<QObject *>(), content);
        QCOMPARE(loader->size(), contentSize);
        QCOMPARE(scenes.size(), 0);
        durations.append(elapsed.nsecsElapsed());
    }
    std::sort(durations.begin(), durations.end());
    qInfo() << "100 compact status updates, p95 microseconds" << durations.at(94) / 1000.0;
}

void F4QuickViewSurfaceTests::panelStatusFitsLongestRowWithoutWrapping()
{
    QuickViewFixture fixture(shellScene({}, 0), true);
    QVERIFY(fixture.window);
    fixture.window->resize(1300, 900);
    auto *footer = fixture.item("panelStatus-0");
    auto *files = fixture.item("panelStatusFiles-0");
    auto *folders = fixture.item("panelStatusFolders-0");
    auto *size = fixture.item("panelStatusSize-0");
    auto *disk = fixture.item("panelStatusDisk-0");
    QVERIFY(footer && files && folders && size && disk);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr-1.75) > .001) QSKIP("Run with QT_SCALE_FACTOR=1.75");
    QVariantMap state = fixture.item("filePanel-0")->property("panel").toMap();
    state.remove("entries");
    state.remove("highlightStyles");
    state["freeSpaceKnown"] = true;
    state["diskTotalSpace"] = 12.7 * (1LL << 40);
    state["freeSpace"] = 135.0 * (1LL << 30);
    state["totalFiles"] = 3279;
    state["totalDirectories"] = 112;
    state["totalSize"] = 59.4 * (1LL << 30);
    state["selectedFiles"] = 1234567;
    state["selectedDirectories"] = 123456;
    state["selectedSize"] = 999.9 * (1LL << 30);
    QList<qreal> widths;
    for (int scenario = 0; scenario < 3; ++scenario) {
        state["selectedCount"] = scenario == 1 ? 1358023 : 0;
        if (scenario == 2) {
            state["totalFiles"] = 1;
            state["totalDirectories"] = 0;
            state["totalSize"] = 0;
        }
        fixture.shell.deliverCompactPresentation({{"type", "scene_patch"}, {"side", 0}, {"panel", state}});
        QTest::qWait(80);
        const auto origin = files->mapToItem(footer, QPointF());
        const auto sizeOrigin = size->mapToItem(footer, QPointF());
        qInfo() << "status scenario" << scenario << "width" << footer->width()
                << "files physical y" << files->mapToScene(QPointF()).y()*dpr
                << "size physical y" << size->mapToScene(QPointF()).y()*dpr;
        QCOMPARE(sizeOrigin.y(), origin.y());
        QCOMPARE(folders->mapToItem(footer, QPointF()).y(), origin.y());
        QVERIFY(sizeOrigin.x() > folders->mapToItem(footer, QPointF()).x());
        const auto padding = footer->property("padding").toReal();
        const auto summaryWidth = files->property("naturalWidth").toReal()
            + folders->property("naturalWidth").toReal() + size->property("naturalWidth").toReal()
            + 2*footer->property("gap").toReal();
        const auto expectedWidth = 2*padding + qMax(summaryWidth, disk->property("naturalWidth").toReal());
        QVERIFY(qAbs(footer->width()-expectedWidth)*dpr < 1.01);
        widths.append(footer->width());
        for (auto *metric : {files, folders, size, disk}) {
            QVERIFY(metric->width()+.001 >= metric->property("naturalWidth").toReal());
            for (auto *leaf : metric->childItems()) {
                if (!leaf->inherits("QQuickText") && !leaf->inherits("QQuickImage")) continue;
                QVERIFY(!leaf->objectName().isEmpty());
                const auto scene = leaf->mapToItem(fixture.window->contentItem(), QPointF());
                const auto physical = scene*dpr;
                QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .01 && qAbs(physical.y()-qRound64(physical.y())) < .01,
                    qPrintable(QString("%1 physical %2,%3").arg(leaf->objectName()).arg(physical.x()).arg(physical.y())));
                QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0))-scene, QPointF(1,0));
                QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1))-scene, QPointF(0,1));
                if (leaf->inherits("QQuickText")) QVERIFY(!leaf->property("truncated").toBool());
            }
        }
        if (qEnvironmentVariableIsSet("F4_PANEL_STATUS_CAPTURE"))
            QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_PANEL_STATUS_CAPTURE")+QString("-width-%1.png").arg(scenario)));
    }
    QVERIFY(widths[1] > widths[2]);
    // Exercise sizes that land on different fractions of a logical pixel.
    state["freeSpaceKnown"] = false;
    for (int count = 1; count <= 200; ++count) {
        state["totalFiles"] = count * 3279;
        state["totalDirectories"] = count * 112;
        state["totalSize"] = count * (1LL << 30);
        fixture.shell.deliverCompactPresentation({{"type", "scene_patch"}, {"side", 0}, {"panel", state}});
        QTest::qWait(1);
        const auto fileY = files->mapToScene(QPointF()).y()*dpr;
        const auto sizeY = size->mapToScene(QPointF()).y()*dpr;
        QVERIFY2(qAbs(fileY-sizeY) < .01, qPrintable(QString("count %1: files physical y %2, size physical y %3, width %4")
            .arg(count).arg(fileY).arg(sizeY).arg(footer->width())));
    }
}
