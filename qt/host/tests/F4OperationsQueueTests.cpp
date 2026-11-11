#include "PointerRowAnchor.h"
#include <QQmlProperty>
#include "SemanticOverlayModel.h"
#include "DummyQWK.h"
#include <ZoinGallery/GalleryRuntime.h>
#include "TestExtUiStateController.h"
#include "SemanticChildrenModel.h"
#include "F4IconProvider.h"
#include <QAbstractItemModelTester>
#include <QPersistentModelIndex>

#include <QAccessible>
#include <QColor>
#include <QCoreApplication>
#include <QElapsedTimer>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QFile>
#include <QDir>
#include <QFont>
#include <QFontMetricsF>
#include <QGuiApplication>
#include <QImage>
#include <QJsonDocument>
#include <QPainter>
#include <QQuickItem>
#include <QQuickItemGrabResult>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QPointer>
#include <QScopeGuard>
#include <QStyleHints>
#include <QSvgRenderer>
#include <QTemporaryDir>
#include <QUrl>
#include <QUrlQuery>
#include <QVariantList>
#include <QVariantMap>
#include <QWheelEvent>
#include <QtQml>
#include <QtTest>

#include <cmath>
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
    Q_PROPERTY(bool panelClipboardPasteEnabled MEMBER m_panelClipboardPasteEnabled)

public:
    Q_INVOKABLE QPoint pointerScreenPosition() const { return QCursor::pos(); }
    Q_INVOKABLE bool pointerEventIsCurrent(QQuickItem *item, qreal x, qreal y) {
        return F4PointerRowAnchor::eventIsCurrent(item, x, y);
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
    Q_INVOKABLE void sendQtKeyEvent(int key, const QString &text, bool down,
                                    int modifiers, int, bool repeat)
    {
        forwardedKeys.append({{"key", key}, {"text", text}, {"down", down},
                              {"modifiers", modifiers}, {"repeat", repeat}});
    }
    QVector<QVariantMap> forwardedKeys;
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
    bool m_panelClipboardPasteEnabled = false;
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
    Q_INVOKABLE QObject *sessionForPanel(const QString &, int) const { return nullptr; }
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

QVariantMap dialogControlsScene(bool focused, bool dropdownOnly = false)
{
    QVariantMap scene = dialogScene();
    QVariantList dialogs = scene.value(QStringLiteral("dialogs")).toList();
    QVariantMap dialog = dialogs.constFirst().toMap();
    QVariantList children = dialog.value(QStringLiteral("children")).toList();

    for (qsizetype index = 0; index < children.size(); ++index) {
        QVariantMap child = children.at(index).toMap();
        const QString kind = child.value(QStringLiteral("kind")).toString();
        if (kind == QStringLiteral("checkbox")) {
            child.insert(QStringLiteral("state"), focused ? 1 : 0);
            child.insert(QStringLiteral("focused"), focused);
        } else if (kind == QStringLiteral("radioGroup")) {
            child.insert(QStringLiteral("selected"), focused ? 1 : 0);
            child.insert(QStringLiteral("focused"), focused);
        }
        children[index] = child;
    }
    children.append(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("appearance-edit")},
        {QStringLiteral("kind"), QStringLiteral("edit")},
        {QStringLiteral("text"), QStringLiteral("C:\\Windows")},
        {QStringLiteral("focused"), focused},
        {QStringLiteral("selectionActive"), focused},
        {QStringLiteral("cursor"), 10},
        {QStringLiteral("x"), 2},
        {QStringLiteral("y"), 12},
        {QStringLiteral("w"), 32},
        {QStringLiteral("h"), 2},
    });
    children.append(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("appearance-combo")},
        {QStringLiteral("kind"), QStringLiteral("comboBox")},
        {QStringLiteral("text"), QStringLiteral("Second option")},
        {QStringLiteral("selected"), 1},
        {QStringLiteral("focused"), focused},
        {QStringLiteral("dropdownOnly"), dropdownOnly},
        {QStringLiteral("items"), QVariantList{
             QVariantMap{{QStringLiteral("text"),
                          QStringLiteral("First option")}},
             QVariantMap{{QStringLiteral("text"),
                          QStringLiteral("Second option")}},
             QVariantMap{{QStringLiteral("text"),
                          QStringLiteral("Third option")}},
         }},
        {QStringLiteral("x"), 2},
        {QStringLiteral("y"), 15},
        {QStringLiteral("w"), 32},
        {QStringLiteral("h"), 2},
    });
    children.append(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("appearance-apply")},
        {QStringLiteral("kind"), QStringLiteral("button")},
        {QStringLiteral("text"), QStringLiteral("Apply")},
        {QStringLiteral("focused"), focused},
        {QStringLiteral("x"), 2},
        {QStringLiteral("y"), 18},
        {QStringLiteral("w"), 16},
        {QStringLiteral("h"), 2},
    });
    children.append(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("appearance-list")},
        {QStringLiteral("kind"), QStringLiteral("listBox")},
        {QStringLiteral("focused"), focused},
        {QStringLiteral("items"), QVariantList{
             QStringLiteral("First row"), QStringLiteral("Second row")}},
        {QStringLiteral("cursor"), 0},
        {QStringLiteral("x"), 38},
        {QStringLiteral("y"), 2},
        {QStringLiteral("w"), 22},
        {QStringLiteral("h"), 6},
    });
    children.append(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("appearance-group")},
        {QStringLiteral("kind"), QStringLiteral("group")},
        {QStringLiteral("title"), QStringLiteral("Advanced")},
        {QStringLiteral("bordered"), true},
        {QStringLiteral("x"), 38},
        {QStringLiteral("y"), 12},
        {QStringLiteral("w"), 22},
        {QStringLiteral("h"), 7},
        {QStringLiteral("children"), QVariantList{
             QVariantMap{
                 {QStringLiteral("id"),
                  QStringLiteral("appearance-group-label")},
                 {QStringLiteral("kind"), QStringLiteral("text")},
                 {QStringLiteral("text"), QStringLiteral("Nested option")},
                 {QStringLiteral("x"), 40},
                 {QStringLiteral("y"), 14},
                 {QStringLiteral("w"), 18},
                 {QStringLiteral("h"), 1},
             },
         }},
    });
    children.append(QVariantMap{
        {QStringLiteral("id"), QStringLiteral("appearance-borderless-group")},
        {QStringLiteral("kind"), QStringLiteral("group")},
        {QStringLiteral("title"), QStringLiteral("Borderless")},
        {QStringLiteral("bordered"), false},
        {QStringLiteral("x"), 38},
        {QStringLiteral("y"), 21},
        {QStringLiteral("w"), 22},
        {QStringLiteral("h"), 4},
        {QStringLiteral("children"), QVariantList{}},
    });

    dialog.insert(QStringLiteral("children"), children);
    dialogs[0] = dialog;
    scene.insert(QStringLiteral("dialogs"), dialogs);
    return scene;
}

QVariantMap dialogComboMenu(int selected)
{
    return {
        {QStringLiteral("id"), QStringLiteral("appearance-combo-menu")},
        {QStringLiteral("kind"), QStringLiteral("menu")},
        {QStringLiteral("role"), QStringLiteral("vmenu")},
        {QStringLiteral("active"), true},
        {QStringLiteral("selected"), selected},
        {QStringLiteral("ownerId"), QStringLiteral("appearance-combo")},
        {QStringLiteral("presentation"), QStringLiteral("dropdown")},
        {QStringLiteral("top"), 0},
        {QStringLiteral("x"), 20},
        {QStringLiteral("y"), 17},
        {QStringLiteral("w"), 32},
        {QStringLiteral("h"), 5},
        {QStringLiteral("viewHeight"), 3},
        {QStringLiteral("items"), QVariantList{
             QVariantMap{
                 {QStringLiteral("index"), 0},
                 {QStringLiteral("text"), QStringLiteral("First option")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("header"), false},
                 {QStringLiteral("disabled"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 1},
                 {QStringLiteral("text"), QStringLiteral("Second option")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("header"), false},
                 {QStringLiteral("disabled"), false},
             },
             QVariantMap{
                 {QStringLiteral("index"), 2},
                 {QStringLiteral("text"), QStringLiteral("Third option")},
                 {QStringLiteral("separator"), false},
                 {QStringLiteral("header"), false},
                 {QStringLiteral("disabled"), false},
             },
         }},
    };
}

QVariantMap dialogComboMenuScene(int selected,
                                 const QString &presentation = {})
{
    QVariantMap scene = dialogControlsScene(true, true);
    scene.insert(QStringLiteral("menus"),
                 QVariantList{dialogComboMenu(selected)});
    if (!presentation.isEmpty())
        scene.insert(QStringLiteral("presentation"), presentation);
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
    F4IconSet productionIcons;
    QQmlApplicationEngine engine;
    QQuickWindow *window = nullptr;

    explicit QueueFixture(const QVariantMap &scene, bool usesQwk = false, bool realIcons = false)
    {
        shell.setScene(scene);
        engine.addImportPath(QStringLiteral(":"));
        engine.rootContext()->setContextProperty(QStringLiteral("qtShell"),
                                                  &shell);
        engine.rootContext()->setContextProperty(QStringLiteral("qtGallery"),
                                                  &gallery);
        engine.rootContext()->setContextProperty(QStringLiteral("qtIcons"),
                                                  realIcons ? static_cast<QObject *>(&productionIcons) : &icons);
        if (realIcons)
            engine.addImageProvider(F4IconSet::defaultProviderId(), new F4IconProvider);
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4GuiFontFamily"), QStringLiteral("Monaco"));
        engine.rootContext()->setContextProperty(
            QStringLiteral("f4GuiFontPixelSize"), 13);
        engine.rootContext()->setContextProperty(QStringLiteral("f4UsesQwk"),
                                                  usesQwk);
        DummyQWK::registerTypes(&engine);
        ZoinGallery::GalleryRuntime::registerTypes();
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
    void searchMessageStaysInDocumentAndOnlyBlocksTab();
    void helpDialogRendersAndRoutesNavigation();
    void driveDetailsUseMeasuredColumnsOnPhysicalPixelGrid();
    void messageBodyWrapsToGuiWidth();
    void settingsSceneReplayProfile();
    void viewerSearchSceneReplayProfile();
    void settingsHelpUpdatePreservesControlIdentity();
    void semanticChildrenModelReportsOnlyChangedRows();
    void pixelAlignmentUsesSettledAncestorTransforms();
    void settingsDecorationsAndViewportStayPixelAligned();
    void settingsLocalScrollSurvivesLateLayout_data();
    void settingsLocalScrollSurvivesLateLayout();
    void fileGroupsOverviewKeepsScrollAndRendersPreview();
    void checkableGroupHeaderRendersAndToggles();
    void fileGroupSectionHeadersStayPixelAligned();
    void dialogWheelAnimatesAndPassesThroughShortLists();
    void fileGroupConditionCheckboxesAreHeadingsAt175Percent();
    void fileGroupNameAndMaskUseCompactRows();
    void fileGroupEditorMatrixAndPinnedPreview();
    void fileGroupEditorCombinesAppearanceInRightColumn();
    void nestedFileGroupEditorBlocksParentNavigation();
    void settingsListsAndDisabledInputs();
    void settingsCategoryListSelectsDuringMouseDrag();
    void settingsCategoryWheelScrollsWithoutSelecting();
    void settingsCategoryListSelectsDuringMouseDrag_data();
    void settingsListScrollingAndTouch();
    void semanticTableWheelPreservesMagnitude();
    void settingsCategoryIconsStayPixelAligned();
    void settingsSearchToolsUseIcons();
    void settingsHeaderShowsActiveSection();
    void settingsEmbeddedTableFillsContent();
    void commandLineFocusKeepsPanelImagePaste();
    void textContextMenusUseSharedStyle();
    void dropdownMenusFitAndScroll_data();
    void dropdownMenusFitAndScroll();
    void longMenusRevealKeyboardSelection();
    void commandLineUndoRedoUsesCoreState();
    void commandLineSelectAllDoesNotSendPanelShortcut();
    void commandAutocompleteUsesRenderedPromptWidth();
    void semanticMenuClearsDisabledHover();
    void platformDialogControls();
    void settingsFooterUsesPlatformButtonOrder();
    void settingsResizeKeepsChromeAndScrollsContent_data();
    void settingsResizeKeepsChromeAndScrollsContent();
    void overlayModelPreservesIdentityAndExitLifecycle();
    void adaptiveChoicesAndFilledFields();
    void multilineDialogEditor();
    void menuBarPressDragReleaseActivatesItem();
    void menuBarPressDragReleaseActivatesItem_data();
    void menuBarClosingDoesNotFlashFirstRow();
    void menuBarClosingDoesNotFlashFirstRow_data();
    void settingsHoverExplainsWithoutFocus();
    void settingsRadiosExpandAndStayPixelAligned();
    void queueDropdownKeepsPanelsAndAlignsLeaves();
    void consoleModeRestoresQueueWorkspace();
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
    void dialogOpenRestoresGlobalKeyboardSinkAndSemanticControlFocus();
    void semanticDialogKeyboardFocusFramesAreDistinctAndThemeLive();
    void semanticChoiceFocusTracksCursorAndHasEqualInsets();
    void fileGroupIconChoicesShowPixelAlignedArtwork();
    void semanticDialogComboBoxFollowsGoOwnedMenuState();
    void dropdownWaitsForItsTimestampAnchor();
    void semanticOverlayMenuStreamClearsWhileFallbackSurfaceIsHidden();
    void semanticOverlayDialogStreamClearsWithoutStaleOverlay();
    void menuChildDialogFollowsGoStackOrder();
    void semanticDialogRowsExpandForNativeControls();
    void commandPaletteInputHasCompactGapAndLucidePrompt();
    void environmentManagerDialogUsesExpandedRows();
    void semanticListCheckboxesAndDimming();
    void semanticListRepeatPreservesRowsAndImmediateCursor();
    void semanticDialogKeyHintsStayVisibleAndPixelAligned();
    void environmentProfilesFitDuringResize();
    void environmentProfilesHeldResizeKeepsMouseGrab();
    void semanticInlineLabelsKeepControlsClose();
    void semanticDialogControlsUseWindowFontAndStayPixelAligned();
    void errorMessagesWrapToNativeWidth();
    void semanticDialogEditShowsRemoteAndNativeSelection();
    void semanticDialogEditSelectionWaitsForSemanticFocus();
    void dialogTextCursorBlinkSettlesAndFocusStopsIt();
};

void F4OperationsQueueTests::initTestCase()
{
    QQuickStyle::setStyle(QStringLiteral("Basic"));
    QGuiApplication::styleHints()->setCursorFlashTime(0);
    qmlRegisterType<TestGrid>("F4QtHost", 1, 0, "VtuiGridItem");
}

void F4OperationsQueueTests::searchMessageStaysInDocumentAndOnlyBlocksTab()
{
    for (const QString &kind : {QStringLiteral("editor"), QStringLiteral("viewer")}) {
        auto scene = documentScene();
        auto surface = scene.value("surface").toMap();
        surface.insert("kind", kind);
        scene.insert("surface", surface);
        // Model the document as the active workspace. Pressing the active
        // tab navigates to the previous workspace; this check deliberately
        // switches to the inactive Commander tab through the modal overlay.
        auto workspaces = scene.value("workspaceTabs").toMap();
        auto tabs = workspaces.value("tabs").toList();
        auto commanderTab = tabs[0].toMap();
        commanderTab.insert("active", false);
        tabs[0] = commanderTab;
        auto documentTab = tabs[1].toMap();
        documentTab.insert("id", "document-tab");
        documentTab.insert("surfaceKind", kind);
        documentTab.insert("active", true);
        documentTab.insert("text", "report.txt");
        tabs[1] = documentTab;
        workspaces.insert("tabs", tabs);
        scene.insert("workspaceTabs", workspaces);
        scene.insert("dialogs", QVariantList{QVariantMap{
            {"id", "search-message"}, {"kind", "dialog"}, {"title", "Search"},
            {"x", 20}, {"y", 8}, {"w", 26}, {"h", 7},
            {"children", QVariantList{
                QVariantMap{{"id", "search-message-body"}, {"kind", "text"},
                    {"text", "Pattern not found."}, {"x", 22}, {"y", 10}, {"w", 18}, {"h", 1}},
                QVariantMap{{"id", "search-message-ok"}, {"kind", "button"},
                    {"text", "Ok"}, {"x", 30}, {"y", 12}, {"w", 6}, {"h", 1}}
            }}
        }});
        QueueFixture fixture(scene);
        QVERIFY(fixture.window);
        auto *document = visualItem(fixture.window->contentItem(), "persistentDocumentLayer");
        QTRY_VERIFY(document && document->isVisible());
        QQuickItem *dialog = nullptr;
        QQuickItem *button = nullptr;
        QTRY_VERIFY((dialog = visualItem(fixture.window->contentItem(), "semanticDialog-search-message")) &&
                    (button = visualItem(fixture.window->contentItem(), "dialogWidget-search-message-okButton")));
        const auto buttonCenter = button->mapToItem(dialog, QPointF(button->width()/2, 0));
        QVERIFY2(qAbs(buttonCenter.x() - dialog->width()/2) * fixture.window->devicePixelRatio() <= 1.01,
                 qPrintable(QString("button center %1, dialog center %2").arg(buttonCenter.x()).arg(dialog->width()/2)));
        auto *backdrop = visualItem(fixture.window->contentItem(), "dialogTabBackdrop");
        QVERIFY(backdrop);
        const auto backdropTop = backdrop->mapToScene(QPointF()).y();
        QVERIFY(backdropTop >= fixture.window->property("menuBarHeight").toReal());
        auto *title = visualItem(fixture.window->contentItem(), "semanticDialogTitle");
        auto *body = visualItem(fixture.window->contentItem(), "dialogWidget-search-message-bodyText");
        auto *label = visualItem(fixture.window->contentItem(), "dialogWidget-search-message-okButtonText");
        for (auto *leaf : {title, body, label}) {
            QVERIFY(leaf);
            const auto origin = leaf->mapToScene(QPointF());
            const auto physical = origin * fixture.window->devicePixelRatio();
            QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
                qPrintable(QString("%1 at %2,%3 px").arg(leaf->objectName()).arg(physical.x()).arg(physical.y())));
            QCOMPARE(leaf->mapToScene(QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToScene(QPointF(0,1))-origin, QPointF(0,1));
        }
        QVERIFY(fixture.window->grabWindow().save(QString("artifacts/search-dialog-%1-175.png").arg(kind)));
        fixture.shell.actions.clear();
        auto *tab = visualItem(fixture.window->contentItem(), "commander-tab");
        QVERIFY(tab && tab->isVisible());
        // The title-bar controls can leave Commander outside the clipped tab
        // viewport even though isVisible() is true. Reveal it before clicking.
        QQuickItem *tabViewport = tab->parentItem();
        while (tabViewport && !tabViewport->property("contentX").isValid())
            tabViewport = tabViewport->parentItem();
        QVERIFY(tabViewport);
        QVERIFY(tabViewport->setProperty("contentX", 0));
        const QPoint clickPosition = itemCenter(tab);
        QVERIFY(QRectF(QPointF(), tabViewport->size()).contains(
            tabViewport->mapFromScene(clickPosition)));
        QVERIFY(QRect(QPoint(), fixture.window->size()).contains(clickPosition));
        if (qEnvironmentVariableIsSet("F4_TRACE_TEST_INPUT"))
            qInfo() << "[FIX:search-tab]" << kind << "click" << clickPosition
                    << "viewport" << tabViewport->size();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                         clickPosition);
        QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(),
            [](const QVariantMap &action) {
                return action.value("action").toString() == "workspace.activate"
                    && action.value("target").toString() == "commander-tab";
            }));
    }
}

void F4OperationsQueueTests::consoleModeRestoresQueueWorkspace()
{
    auto scene=queueScene(queueModel({task(1,"Running",35)},1,true));
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QTRY_VERIFY(fixture.window->property("queueDropdownOpen").toBool());
    scene.insert("presentation","text");
    fixture.shell.actions.clear();
    fixture.shell.setScene(scene);
    QTest::qWait(150);
    QVERIFY(!fixture.window->property("queueDropdownOpen").toBool());
    QVERIFY(!fixture.item("operationsQueueButton")->isVisible());
    // Console mode renders its own tabs in the grid. QML must neither cover
    // them with a popup nor redirect activation of the console queue workspace.
    QVERIFY(fixture.item("vtuiGrid")->property("renderingEnabled").toBool());
    auto *popup=fixture.window->findChild<QObject *>("operationsQueueDropdown");
    QVERIFY(popup && !popup->property("visible").toBool());
    for (bool queueActive : {false,true,false,true}) {
        scene.insert("workspaceTabs",workspaceTabs(queueActive,false));
        fixture.shell.setScene(scene);
        QTest::qWait(50);
        QVERIFY(!fixture.window->property("queueDropdownOpen").toBool());
        QVERIFY(!popup->property("visible").toBool());
        for(const auto &action:fixture.shell.actions) QVERIFY(action.value("action")!="workspace.activate");
    }
    scene.remove("presentation");
    fixture.shell.setScene(scene);
    QTRY_VERIFY(fixture.window->property("queueDropdownOpen").toBool());
    QVERIFY(fixture.item("operationsQueueButton")->isVisible());
    QTRY_VERIFY(!fixture.item("queue-tab"));
}

void F4OperationsQueueTests::queueDropdownKeepsPanelsAndAlignsLeaves()
{
    auto scene=panelScene();
    auto running = task(1,"Running",35);
    running.insert("pausable",true);
    scene.insert("operationsQueue",queueModel({running,task(2,"Error",20)},1,true));
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QVERIFY(!fixture.item("queue-tab"));
    auto *button=fixture.item("operationsQueueButton");
    QVERIFY(button);
    // A copy progress publication can arrive between native press and release.
    QTest::mousePress(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));
    running.insert("progress",36);
    scene.insert("operationsQueue",queueModel({running,task(2,"Error",20)},1,true));
    fixture.shell.setScene(scene);
    QTest::qWait(150);
    QVERIFY2(button->hasActiveFocus(),"Queue progress stole focus from the pressed trigger");
    QTest::mouseRelease(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));

    QTRY_VERIFY(fixture.window->property("queueDropdownOpen").toBool());
    auto *surface=fixture.item("operationsQueueSurface");
    QVERIFY(surface && surface->isVisible());
    QVERIFY(fixture.item("persistentPanelsLayer")->isVisible());
    for (const auto &action : fixture.shell.actions) QVERIFY(action.value("action")!="workspace.activate");
    QTest::qWait(150);
    const qreal dpr=fixture.window->devicePixelRatio();
    QList<QQuickItem *> pending{surface,button};
    int leaves=0;
    while (!pending.isEmpty()) {
        auto *item=pending.takeLast(); pending.append(item->childItems());
        if (!item->isVisible()) continue;
        const QString type=item->metaObject()->className();
        if (!type.startsWith("QQuickText") && !type.startsWith("QQuickImage") && !type.startsWith("QQuickIconImage")) continue;
        if (item->property("text").isValid() && item->property("text").toString().isEmpty()) continue;
        QVERIFY2(!item->objectName().isEmpty(),qPrintable(type));
        const auto origin=item->mapToScene(QPointF());
        const auto physical=origin*dpr;
        QVERIFY2(qAbs(physical.x()-qRound(physical.x()))<0.001 && qAbs(physical.y()-qRound(physical.y()))<0.001,
            qPrintable(QString("%1 %2 physical=(%3,%4)").arg(item->objectName(),type).arg(physical.x()).arg(physical.y())));
        QCOMPARE(item->mapToScene(QPointF(1,0))-origin,QPointF(1,0));
        QCOMPARE(item->mapToScene(QPointF(0,1))-origin,QPointF(0,1));
        ++leaves;
    }
    QVERIFY(leaves>10);
    QVERIFY(!fixture.window->grabWindow().isNull());
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(fixture.window->grabWindow().save(QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath(QString("queue-dropdown-%1.png").arg(dpr))));
    auto *pause = fixture.item("operationsQueuePauseButton");
    QVERIFY(pause && pause->isEnabled());
    QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(pause));
    QCOMPARE(fixture.shell.actions.last().value("action").toString(),QString("queue.pause"));
    QCOMPARE(fixture.shell.actions.last().value("taskId").toInt(),1);
    running.insert("state","Paused"); running.insert("stateClass","paused");
    running.insert("pausable",false); running.insert("resumable",true);
    scene.insert("operationsQueue",queueModel({running,task(2,"Error",20)},1,true));
    fixture.shell.setScene(scene);
    QTRY_COMPARE(pause->property("text").toString(),QString("Resume"));
    QTest::qWait(150);
    QList<QQuickItem *> resumedLeaves{pause};
    while (!resumedLeaves.isEmpty()) {
        auto *leaf = resumedLeaves.takeLast(); resumedLeaves.append(leaf->childItems());
        const QString type = leaf->metaObject()->className();
        if (!leaf->isVisible() || (!type.startsWith("QQuickText") && !type.startsWith("QQuickImage"))) continue;
        QVERIFY(!leaf->objectName().isEmpty());
        const auto origin = leaf->mapToScene(QPointF());
        const auto physical = origin*dpr;
        QVERIFY(qAbs(physical.x()-qRound(physical.x()))<0.001 && qAbs(physical.y()-qRound(physical.y()))<0.001);
        QCOMPARE(leaf->mapToScene(QPointF(1,0))-origin,QPointF(1,0));
        QCOMPARE(leaf->mapToScene(QPointF(0,1))-origin,QPointF(0,1));
    }
    QVERIFY(!fixture.window->grabWindow().isNull());
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(fixture.window->grabWindow().save(QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath(QString("queue-resume-%1.png").arg(dpr))));
    QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(pause));
    QCOMPARE(fixture.shell.actions.last().value("action").toString(),QString("queue.resume"));
    QTest::keyClick(fixture.window,Qt::Key_Escape);
    QTRY_VERIFY(!fixture.window->property("queueDropdownOpen").toBool());
    QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));
    QTRY_VERIFY(fixture.window->property("queueDropdownOpen").toBool());
    QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,QPoint(10,600));
    QTRY_VERIFY(!fixture.window->property("queueDropdownOpen").toBool());
    auto *popup = fixture.window->findChild<QObject *>("operationsQueueDropdown");
    QVERIFY(popup);
    // Model native title-bar delivery: the press starts on the trigger,
    // outside dismissal occurs, then the trigger receives its release/click.
    QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));
    QTRY_VERIFY(popup->property("visible").toBool());
    QVERIFY(QMetaObject::invokeMethod(button,"pressed"));
    QVERIFY(QMetaObject::invokeMethod(popup,"close"));
    QVERIFY(QMetaObject::invokeMethod(button,"clicked"));
    QTRY_VERIFY(!popup->property("visible").toBool());
    QTRY_VERIFY(!fixture.window->property("queueDropdownOpen").toBool());
    for (int attempt=0; attempt<8; ++attempt) {
        QTest::mousePress(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));
        running.insert("progress",40+attempt);
        scene.insert("operationsQueue",queueModel({running,task(2,"Error",20)},1,true));
        fixture.shell.setScene(scene);
        QTest::qWait(110);
        QVERIFY(button->hasActiveFocus());
        QTest::mouseRelease(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));
        QTRY_VERIFY(popup->property("visible").toBool());
        pause->forceActiveFocus();
        running.insert("progress",50+attempt);
        scene.insert("operationsQueue",queueModel({running,task(2,"Error",20)},1,true));
        fixture.shell.setScene(scene);
        QTest::qWait(110);
        QVERIFY(pause->hasActiveFocus());
        QVERIFY(popup->property("visible").toBool());
        QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,itemCenter(button));
        QTRY_VERIFY(!popup->property("visible").toBool());
        QTRY_VERIFY(!fixture.window->property("queueDropdownOpen").toBool());
    }
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

    const QList<QPair<QString, QString>> typedWorkspaceIcons{
        {QStringLiteral("panels"), QStringLiteral("panels-top-left")},
        {QStringLiteral("terminal"), QStringLiteral("square-terminal")},
        {QStringLiteral("viewer"), QStringLiteral("file-text")},
        {QStringLiteral("editor"), QStringLiteral("file-pen-line")},
        {QStringLiteral("imageViewer"), QStringLiteral("image")},
    };
    for (const auto &[surfaceKind, expectedIcon] : typedWorkspaceIcons) {
        const QVariantMap tab{{QStringLiteral("surfaceKind"), surfaceKind}};
        QVariant iconName;
        QVERIFY(QMetaObject::invokeMethod(
            fixture.window, "workspaceTabIconName",
            Q_RETURN_ARG(QVariant, iconName), Q_ARG(QVariant, tab)));
        QCOMPARE(iconName.toString(), expectedIcon);
    }
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
    QVERIFY(!visualItemWithText(workspaceBar, QStringLiteral("Queue")));
    QVERIFY(fixture.item(QStringLiteral("operationsQueueButton")));
    QVariant naturalTabWidth;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "preferredWorkspaceTabWidth",
        Q_RETURN_ARG(QVariant, naturalTabWidth),
        Q_ARG(QVariant, 180)));
    QCOMPARE(naturalTabWidth.toInt(), 226);
    QVariant cappedTabWidth;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "preferredWorkspaceTabWidth",
        Q_RETURN_ARG(QVariant, cappedTabWidth),
        Q_ARG(QVariant, 400)));
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
    QueueFixture fixture(panelScene(), true);
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
    int newTabActions=0;
    for (const auto &action : fixture.shell.actions) {
        if (action.value("action")!="workspace.new") continue;
        QCOMPARE(action.value("target"),QVariant("workspace-new"));
        ++newTabActions;
    }
    QCOMPARE(newTabActions,1);
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
    items[0] = task(1, QStringLiteral("Scanning"), 0);
    const QVariantMap queue = queueModel(items, 1, true);
    fixture.shell.setScene(queueScene(queue));

    QQuickItem *queueSurface = nullptr;
    QQuickItem *queueList = nullptr;
    QObject *queueBusy = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (queueSurface = fixture.item(QStringLiteral("operationsQueueSurface"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (queueList = fixture.item(QStringLiteral("operationsQueueList"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(queueList->property("contentHeight").toReal()
                             > queueList->height(), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (queueBusy = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("operationsQueueBusy-1"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(queueBusy->property("running").toBool(), 1000);

    // Retained surfaces must not leave a hidden indeterminate animation
    // running below a modal/menu layer. It resumes when the queue can present
    // frames again because the operation itself remains active.
    QVariantMap queueWithDialog = queueScene(queue);
    queueWithDialog.insert(QStringLiteral("dialogs"),
                           dialogScene().value(QStringLiteral("dialogs")));
    fixture.shell.setScene(queueWithDialog);
    QTRY_VERIFY_WITH_TIMEOUT(
        !queueBusy->property("running").toBool(), 1000);
    fixture.shell.setScene(queueScene(queue));
    QTRY_VERIFY_WITH_TIMEOUT(queueBusy->property("running").toBool(), 1000);

    queueList->setProperty("contentY", 180.0);
    QVERIFY(QMetaObject::invokeMethod(queueList, "flick",
                                      Qt::DirectConnection,
                                      Q_ARG(qreal, 0.0),
                                      Q_ARG(qreal, -900.0)));
    QTRY_VERIFY_WITH_TIMEOUT(queueList->property("flicking").toBool(), 1000);

    fixture.window->setProperty("queueDropdownOpen",false);
    fixture.shell.setScene(panelScene());
    QTRY_VERIFY_WITH_TIMEOUT(panelPair->isVisible(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        !queueSurface->property("interactionActive").toBool(), 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        !queueBusy->property("running").toBool(), 1000);
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
    QTRY_VERIFY_WITH_TIMEOUT(queueBusy->property("running").toBool(), 1000);
    QCOMPARE(fixture.item(QStringLiteral("operationsQueueList")), queueList);
    QCOMPARE(queueList->property("contentY").toReal(), frozenQueueY);

    fixture.window->setProperty("queueDropdownOpen",false);
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

    fixture.window->setProperty("queueDropdownOpen",false);
    fixture.shell.setScene(documentScene());
    QTRY_VERIFY_WITH_TIMEOUT(document->isVisible(), 1000);
    QCOMPARE(fixture.item(QStringLiteral("documentList")), documentList);
    QCOMPARE(documentList->property("contentY").toReal(), frozenDocumentY);
    QCOMPARE(fixture.item(QStringLiteral("persistentPanelPair")), panelPair);
}

void F4OperationsQueueTests::terminalModeKeepsPersistentPanelsSurfaceVisible()
{
    QVariantMap scene = terminalScene();
    QueueFixture fixture(scene);
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
    QCOMPARE(panelPair->opacity(), 0.0);

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

    QPointer<QQuickItem> terminalSurface =
        fixture.item(QStringLiteral("terminalDocumentSurface"));
    QVERIFY(terminalSurface && terminalSurface->isVisible());
    const QSizeF exposedSize(terminalSurface->width(), terminalSurface->height());
    QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    shell.insert(QStringLiteral("terminalActive"), false);
    scene.insert(QStringLiteral("shell"), shell);
    fixture.shell.setScene(scene);
    QTRY_COMPARE(panelPair->opacity(), 1.0);
    QTRY_VERIFY(!terminal->isVisible());
    QVERIFY(terminalSurface);
    QVERIFY(!terminalSurface->isVisible());
    QVERIFY(terminalSurface->height() > 0.0);

    shell.insert(QStringLiteral("terminalActive"), true);
    scene.insert(QStringLiteral("shell"), shell);
    fixture.shell.setScene(scene);
    QTRY_VERIFY(terminalSurface && terminalSurface->isVisible());
    QCOMPARE(fixture.item(QStringLiteral("terminalDocumentSurface")), terminalSurface.data());
    QCOMPARE(QSizeF(terminalSurface->width(), terminalSurface->height()), exposedSize);
}

void F4OperationsQueueTests::panelLoadingPulseIsDelayedLocalAndDoesNotMoveRendererButton()
{
    QueueFixture fixture(loadingPanelScene(false));
    QVERIFY(fixture.window);

    QQuickItem *path = nullptr;
    QQuickItem *panelView = nullptr;
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
        (panelView = fixture.item(QStringLiteral("filePanel-0"))), 3000);
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
    const qreal dpr = fixture.window->devicePixelRatio();
    QCOMPARE(path->property("leadingInset").toReal(), qRound(4 * dpr) / dpr);
    QCOMPARE(path->property("breadcrumbFontPixelSize").toReal(), 13.0);
    QCOMPARE(path->property("pathTextColor").value<QColor>(),
             QColor(QStringLiteral("#e8edf2")));
    QCOMPARE(path->property("pathHoveredColor").value<QColor>(),
             QColor(QStringLiteral("#2a3745")));
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

    const auto panelActions = [&fixture]() {
        QVector<QVariantMap> result;
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            // The standalone Qt viewport negotiation is session state and may
            // complete while this panel-only interaction is being exercised.
            if (action.value(QStringLiteral("action")).toString()
                != QStringLiteral("document.viewport")) {
                result.append(action);
            }
        }
        return result;
    };

    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(sortButton));
    QTRY_VERIFY_WITH_TIMEOUT(sortMenu->property("opened").toBool(), 1000);
    QVERIFY(panelActions().isEmpty());
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
    QTRY_COMPARE_WITH_TIMEOUT(panelActions().size(), 1, 1000);
    const QVector<QVariantMap> sortActions = panelActions();
    QCOMPARE(sortActions.constFirst().value(QStringLiteral("action"))
                 .toString(),
             QStringLiteral("panel.sort"));
    QCOMPARE(sortActions.constFirst().value(QStringLiteral("side"))
                 .toInt(),
             0);
    QCOMPARE(sortActions.constFirst().value(QStringLiteral("mode"))
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
    QTRY_VERIFY_WITH_TIMEOUT(
        panelView->property("loadingIndicatorPulseRunning").toBool(), 500);
    const QString firstFrame = pulse->property("text").toString();
    QVERIFY(!firstFrame.isEmpty());
    QCOMPARE(path->property("text").toString(),
             QStringLiteral("/Users/zoin/Documents"));
    QCOMPARE(renderer->x(), rendererX);
    QTRY_VERIFY_WITH_TIMEOUT(
        pulse->property("text").toString() != firstFrame, 500);

    // The panel object is deliberately retained below Viewer/Editor. Loading
    // may continue, but its invisible pulse must stop requesting frames.
    fixture.shell.setScene(documentScene());
    QTRY_VERIFY_WITH_TIMEOUT(
        !panelView->property("loadingIndicatorPulseRunning").toBool(), 500);
    QVERIFY(!panelView->property("loadingIndicatorDelayRunning").toBool());
    QVERIFY(!pulse->isVisible());

    fixture.shell.setScene(loadingPanelScene(true));
    QTRY_VERIFY_WITH_TIMEOUT(pulse->isVisible(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        panelView->property("loadingIndicatorPulseRunning").toBool(), 500);

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

    // An open menu is a stable surface. Allow the Basic style's bounded
    // opening/hover transitions to finish, then require the scene graph to
    // sleep while the menu remains visible and untouched.
    QTest::qWait(350);
    QSignalSpy idleMenuFrames(fixture.window, &QQuickWindow::frameSwapped);
    QVERIFY(idleMenuFrames.isValid());
    QTest::qWait(160);
    QCOMPARE(idleMenuFrames.size(), 0);

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
    QTRY_COMPARE(qobject_cast<QAbstractItemModel *>(frameModel)->rowCount(), 1);
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
    // Retain the legacy rectangular-control assertions on every test host.
    QVERIFY(dialog->setProperty("macWindowControls", false));
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
    QVERIFY(!maximizeButton->isVisible());
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
    QVERIFY(!maximizeButton->isVisible());

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

void F4OperationsQueueTests::dialogOpenRestoresGlobalKeyboardSinkAndSemanticControlFocus()
{
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);

    auto *grid = fixture.window->findChild<TestGrid *>();
    QQuickItem *panelFocusOwner = fixture.item(
        QStringLiteral("semanticOverlayHost"));
    QVERIFY(grid);
    QVERIFY(panelFocusOwner);

    // Reproduce a native gallery panel owning Qt keyboard focus at the exact
    // moment a semantic dialog arrives on its independent protocol stream.
    panelFocusOwner->forceActiveFocus();
    QTRY_VERIFY_WITH_TIMEOUT(panelFocusOwner->hasActiveFocus(), 1000);
    QVERIFY(!grid->hasActiveFocus());

    const QVariantMap focusedScene = dialogControlsScene(true);
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"),
         focusedScene.value(QStringLiteral("dialogs")).toList()},
    }, 100);

    // The grid remains the raw-key forwarding sink. The actual control focus
    // is Go-owned and must be represented by the semantic focus frame rather
    // than by letting the covered panel retain Qt focus.
    QTRY_VERIFY_WITH_TIMEOUT(grid->hasActiveFocus(), 1000);
    QQuickItem *checkBox = nullptr;
    QQuickItem *checkFocusFrame = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkBox = visualItem(fixture.window->contentItem(), QStringLiteral(
             "dialogWidget-appearance-checkboxCheckBox"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkFocusFrame = visualItem(fixture.window->contentItem(),
             QStringLiteral(
                 "dialogWidget-appearance-checkboxCheckBoxFocusFrame"))),
        3000);
    QVERIFY(checkBox->property("semanticFocus").toBool());
    QTRY_COMPARE_WITH_TIMEOUT(
        checkFocusFrame->property("testBorderColor").value<QColor>(),
        fixture.window->property("dialogAccent").value<QColor>(), 1000);
    QTRY_COMPARE_WITH_TIMEOUT(
        checkFocusFrame->property("testBorderWidth").toReal(),
        fixture.window->property("separatorWidth").toReal(), 1000);
}

void F4OperationsQueueTests::semanticChoiceFocusTracksCursorAndHasEqualInsets()
{
    const auto sceneForFocus = [](int focusIndex) {
        auto scene = dialogScene();
        auto dialogs = scene.value("dialogs").toList();
        auto dialog = dialogs[0].toMap();
        auto children = dialog.value("children").toList();
        for (auto &entry : children) {
            auto child = entry.toMap();
            // Semantic coordinates are absolute, including the frame origin.
            child["x"] = 20;
            child["y"] = child.value("y").toInt() + 3;
            child["focused"] = true;
            if (child.value("kind") == "radioGroup") {
                child["focusIndex"] = focusIndex;
                child["selected"] = 0;
            }
            entry = child;
        }
        dialog["children"] = children;
        dialogs[0] = dialog;
        scene["dialogs"] = dialogs;
        return scene;
    };
    QueueFixture fixture(sceneForFocus(0));
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    const QString check = "dialogWidget-appearance-checkboxCheckBox";
    const QString radio = "dialogWidget-appearance-navigationRadio-";
    const auto item = [root](const QString &name) { return visualItem(root, name); };
    QTRY_VERIFY(item(radio + "1Text"));
    QTest::qWait(150);
    const qreal dpr = fixture.window->devicePixelRatio();
    for (const auto &name : {check, radio + "0", radio + "1"}) {
        auto *frame = item(name + "FocusFrame");
        auto *indicator = item(name + "Indicator");
        QVERIFY(frame);
        QVERIFY(indicator);
        const auto f = frame->mapToItem(root, QPointF{}) * dpr;
        const auto i = indicator->mapToItem(root, QPointF{}) * dpr;
        const qreal left = i.x() - f.x();
        const qreal top = i.y() - f.y();
        const qreal bottom = f.y() + frame->height() * dpr
                             - i.y() - indicator->height() * dpr;
        const auto details = QString("%1 focus gaps: left=%2 top=%3 bottom=%4 physical px")
                                 .arg(name).arg(left).arg(top).arg(bottom);
        QVERIFY2(left > 0 && qAbs(left - top) < 0.001
                 && qAbs(left - bottom) < 0.001, qPrintable(details));
        for (const auto &suffix : {QString("Text"), QString("Indicator"), QString("FocusFrame")}) {
            auto *leaf = item(name + suffix);
            QVERIFY(leaf);
            const auto origin = leaf->mapToItem(root, QPointF{});
            QVERIFY2(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001,
                     qPrintable(leaf->objectName()));
            QVERIFY2(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001,
                     qPrintable(leaf->objectName()));
            QCOMPARE(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1));
        }
    }
    for (int focusIndex : {1, 0, 1}) {
        fixture.shell.setScene(sceneForFocus(focusIndex));
        QTRY_COMPARE(item(radio + "0")->property("semanticFocus").toBool(), focusIndex == 0);
        QTRY_COMPARE(item(radio + "1")->property("semanticFocus").toBool(), focusIndex == 1);
        QVERIFY(item(radio + "0")->property("checked").toBool());
        QVERIFY(!item(radio + "1")->property("checked").toBool());
    }
    QImage capture;
    QTRY_VERIFY(!(capture = fixture.window->grabWindow()).isNull());
    if (qEnvironmentVariableIsSet("F4_DIALOG_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_DIALOG_CAPTURE")));
}

void F4OperationsQueueTests::semanticDialogKeyboardFocusFramesAreDistinctAndThemeLive()
{
    QueueFixture fixture(dialogControlsScene(false));
    QVERIFY(fixture.window);

    QQuickItem *const rootItem = fixture.window->contentItem();
    QVERIFY(rootItem);
    QQuickItem *checkBox = nullptr;
    QQuickItem *checkFocusFrame = nullptr;
    QQuickItem *radioButton = nullptr;
    QQuickItem *radioFocusFrame = nullptr;
    QQuickItem *listBox = nullptr;
    QQuickItem *listFocusFrame = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkBox = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-checkboxCheckBox"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkFocusFrame = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-checkboxCheckBoxFocusFrame"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioButton = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-navigationRadio-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioFocusFrame = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-navigationRadio-0FocusFrame"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (listBox = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-listListBox"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (listFocusFrame = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-listListFocusFrame"))),
        3000);

    const QColor normalBorder(QStringLiteral("#334455"));
    const QColor firstAccent(QStringLiteral("#4a90e2"));
    QVERIFY(fixture.window->setProperty("controlBorder", normalBorder));
    QVERIFY(fixture.window->setProperty("dialogAccent", firstAccent));

    // Selection and focus are independent. The selected radio button must not
    // look keyboard-focused until the semantic model says it is focused.
    QVERIFY(radioButton->property("checked").toBool());
    QVERIFY(checkBox->setProperty("checked", true));
    for (QQuickItem *frame : {checkFocusFrame, radioFocusFrame,
                              listFocusFrame}) {
        QTRY_COMPARE_WITH_TIMEOUT(
            frame->property("testBorderColor").value<QColor>(),
            normalBorder, 1000);
        QCOMPARE(frame->property("testBorderWidth").toReal(), 0.0);
    }
    QImage unfocusedFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(unfocusedFrame = fixture.window->grabWindow()).isNull(), 3000);

    QVERIFY(checkBox->setProperty("semanticFocus", true));
    QVERIFY(radioButton->setProperty("semanticFocus", true));
    QVERIFY(listBox->setProperty("semanticFocus", true));
    const qreal separatorWidth =
        fixture.window->property("separatorWidth").toReal();
    for (QQuickItem *frame : {checkFocusFrame, radioFocusFrame,
                              listFocusFrame}) {
        QTRY_COMPARE_WITH_TIMEOUT(
            frame->property("testBorderColor").value<QColor>(),
            firstAccent, 1000);
        QTRY_COMPARE_WITH_TIMEOUT(
            frame->property("testBorderWidth").toReal(),
            separatorWidth, 1000);
    }

    const QColor changedAccent(QStringLiteral("#2fbcff"));
    QVERIFY(fixture.window->setProperty("dialogAccent", changedAccent));
    for (QQuickItem *frame : {checkFocusFrame, radioFocusFrame,
                              listFocusFrame}) {
        QTRY_COMPARE_WITH_TIMEOUT(
            frame->property("testBorderColor").value<QColor>(),
            changedAccent, 1000);
    }

    QImage focusedFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(focusedFrame = fixture.window->grabWindow()).isNull(), 3000);
    QVERIFY(focusedFrame != unfocusedFrame);

    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required for the physical-pixel gate");
    for (QQuickItem *frame : {checkFocusFrame, radioFocusFrame,
                              listFocusFrame}) {
        const QPointF origin = frame->mapToItem(rootItem, QPointF{});
        const QPointF physicalOrigin = origin * dpr;
        const qreal physicalWidth = frame->width() * dpr;
        const qreal physicalHeight = frame->height() * dpr;
        const QString details = QStringLiteral(
            "%1 geometry is (%2,%3) %4x%5 physical px")
                                    .arg(frame->objectName())
                                    .arg(physicalOrigin.x(), 0, 'f', 6)
                                    .arg(physicalOrigin.y(), 0, 'f', 6)
                                    .arg(physicalWidth, 0, 'f', 6)
                                    .arg(physicalHeight, 0, 'f', 6);
        QVERIFY2(qAbs(physicalOrigin.x()
                      - qRound(physicalOrigin.x())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physicalOrigin.y()
                      - qRound(physicalOrigin.y())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physicalWidth - qRound(physicalWidth)) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physicalHeight - qRound(physicalHeight)) < 0.001,
                 qPrintable(details));
    }
}

void F4OperationsQueueTests::dropdownWaitsForItsTimestampAnchor()
{
    auto menu = dialogComboMenu(1);
    menu["id"] = "timestamp-menu";
    menu["ownerId"] = "timestamp";
    auto items = menu.value("items").toList();
    const QStringList timestamps{"Modified", "Created", "Accessed"};
    for (qsizetype i = 0; i < items.size(); ++i) {
        auto item = items[i].toMap();
        item["text"] = timestamps[i];
        items[i] = item;
    }
    menu["items"] = items;
    auto scene = dialogControlsScene(true, true);
    auto dialogs = scene.value("dialogs").toList();
    auto dialog = dialogs[0].toMap();
    auto children = dialog.value("children").toList();
    QVariantMap timestamp;
    for (qsizetype i = 0; i < children.size(); ++i) {
        if (children[i].toMap().value("id") != "appearance-combo")
            continue;
        timestamp = children.takeAt(i).toMap();
        break;
    }
    QVERIFY(!timestamp.isEmpty());
    timestamp["id"] = "timestamp";
    timestamp["text"] = "Created";
    timestamp["x"] = 32;
    dialog["children"] = children;
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QTRY_VERIFY(fixture.window->isActive());
    auto *root = fixture.window->contentItem();
    scene["menus"] = QVariantList{menu};
    fixture.shell.setScene(scene);
    QQuickItem *popup = nullptr;
    QTRY_VERIFY((popup = visualItem(root, "semanticMenuPopup-timestamp-menu")));
    QTest::qWait(40);
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto premature = popup->mapToItem(root, QPointF{}) * dpr;
    QVERIFY2(!popup->isVisible(), qPrintable(QString("Unanchored popup painted at (%1, %2) physical px")
        .arg(premature.x()).arg(premature.y())));

    children.append(timestamp);
    dialog["children"] = children;
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    fixture.shell.setScene(scene);
    QQuickItem *combo = nullptr;
    QTRY_VERIFY((combo = visualItem(root, "dialogWidget-timestampComboBox")));
    QTRY_VERIFY(popup->isVisible());
    const auto firstX = popup->mapToItem(root, QPointF{}).x();
    auto *overlay = popup->parentItem();
    const auto anchor = combo->mapToItem(root, QPointF{});
    const qreal anchorX = qRound(anchor.x() * dpr) / dpr;
    const qreal anchorY = qRound(anchor.y() * dpr) / dpr;
    QVERIFY(qAbs((firstX + overlay->property("menuEdgeInset").toReal() - anchorX) * dpr) < 0.02);
    const qreal progress = overlay->property("revealProgress").toReal();
    const qreal openTop = overlay->property("dropdownOpenTop").toReal();
    QVERIFY(qAbs((popup->mapToItem(root, QPointF{}).y()
        - (anchorY + (openTop - anchorY) * progress)) * dpr) < 0.02);
    QTRY_VERIFY(overlay->property("dropdownOpenSettled").toBool());
    QCOMPARE(popup->mapToItem(root, QPointF{}).x(), firstX);
    for (const auto &name : {"semanticMenuItemText-timestamp-menu-0", "semanticMenuItemText-timestamp-menu-1",
                             "semanticMenuItemText-timestamp-menu-2", "semanticDropdownChevronUp-timestamp-menu"}) {
        auto *leaf = visualItem(root, name);
        QVERIFY2(leaf && leaf->isVisible(), name);
        const auto origin = leaf->mapToItem(root, QPointF{});
        const auto physical = origin * dpr;
        QVERIFY(qAbs(physical.x() - qRound(physical.x())) < 0.02);
        QVERIFY(qAbs(physical.y() - qRound(physical.y())) < 0.02);
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
    }
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(fixture.window->grabWindow().save(
            QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("timestamp-dropdown-175.png")));
}

void F4OperationsQueueTests::fileGroupIconChoicesShowPixelAlignedArtwork()
{
    QVariantMap scene = dialogControlsScene(true);
    QVariantList dialogs = scene.value("dialogs").toList();
    QVariantMap dialog = dialogs.first().toMap();
    QVariantList children = dialog.value("children").toList();
    for (QVariant &value : children) {
        QVariantMap control = value.toMap();
        if (control.value("id") != "appearance-combo")
            continue;
        control.insert("dropdownOnly", false);
        control.insert("text", "Automatic");
        control.insert("iconTheme", "lucide");
        control.insert("iconReference", "asset:images");
        control.insert("items", QVariantList{
            QVariantMap{{"text", "Automatic"}, {"details", QVariantMap{
                {"iconTheme", "lucide"}, {"iconReference", "asset:images"}}}},
            QVariantMap{{"text", "File"}, {"details", QVariantMap{
                {"iconTheme", "lucide"}, {"iconReference", "asset:file"}}}},
        });
        value = control;
    }
    dialog.insert("children", children);
    dialogs[0] = dialog;
    scene.insert("dialogs", dialogs);
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required for the physical-pixel gate");
    QQuickItem *const root = fixture.window->contentItem();
    const QString prefix = QStringLiteral("dialogWidget-appearance-comboComboBox");
    QQuickItem *combo = nullptr;
    QQuickItem *closedIcon = nullptr;
    QTRY_VERIFY(combo = visualItem(root, prefix));
    QTRY_VERIFY(closedIcon = visualItem(root, prefix + "IconPreview"));
    QTRY_COMPARE(closedIcon->property("status").toInt(), 1); // Image.Ready
    QObject *const popup = combo->property("popup").value<QObject *>();
    QVERIFY(popup);
    QVERIFY(QMetaObject::invokeMethod(popup, "open"));
    QTRY_VERIFY(popup->property("visible").toBool());

    QList<QQuickItem *> leaves{closedIcon};
    for (const QString &suffix : {QStringLiteral("Text"), QStringLiteral("Indicator"),
                                  QStringLiteral("PopupItemText-0"), QStringLiteral("PopupItemText-1"),
                                  QStringLiteral("PopupItemIcon-0"), QStringLiteral("PopupItemIcon-1")}) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY(leaf = visualItem(root, prefix + suffix));
        if (suffix.startsWith("PopupItemIcon"))
            QTRY_COMPARE(leaf->property("status").toInt(), 1);
        leaves.append(leaf);
    }
    for (QQuickItem *leaf : std::as_const(leaves)) {
        const QPointF origin = leaf->mapToItem(root, QPointF{});
        const QPointF physical = origin * dpr;
        const QString detail = QStringLiteral("%1 physical origin (%2, %3)")
            .arg(leaf->objectName()).arg(physical.x()).arg(physical.y());
        QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.001
                 && qAbs(physical.y() - qRound(physical.y())) < 0.001, qPrintable(detail));
        QCOMPARE(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1));
    }
    QImage frame;
    QTRY_VERIFY(!(frame = fixture.window->grabWindow()).isNull());
    const QString capture = qEnvironmentVariable("F4_ICON_CHOICES_CAPTURE");
    if (!capture.isEmpty())
        QVERIFY(frame.save(capture));
}

void F4OperationsQueueTests::semanticDialogComboBoxFollowsGoOwnedMenuState()
{
    QueueFixture fixture(dialogControlsScene(true, true));
    QVERIFY(fixture.window);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.window->isActive(), 3000);

    auto *grid = fixture.window->findChild<TestGrid *>();
    QQuickItem *combo = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (combo = visualItem(fixture.window->contentItem(), QStringLiteral(
             "dialogWidget-appearance-comboComboBox"))),
        3000);
    QVERIFY(grid);
    QTRY_VERIFY_WITH_TIMEOUT(grid->hasActiveFocus(), 1000);
    QVERIFY(combo->property("semanticFocus").toBool());
    QVERIFY(!combo->hasActiveFocus());
    QCOMPARE(combo->property("count").toInt(), 0);
    QVERIFY(combo->property("externallyOwnedPopup").toBool());
    QVERIFY(!combo->property("editable").toBool());

    const QPointer<QObject> nativePopup =
        combo->property("popup").value<QObject *>();
    QVERIFY(nativePopup);
    const auto comboActions = [&fixture]() {
        QVector<QVariantMap> result;
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("target")).toString()
                == QStringLiteral("appearance-combo")) {
                result.append(action);
            }
        }
        return result;
    };

    fixture.shell.clearActions();
    QSignalSpy externalPopupSpy(combo, SIGNAL(externalPopupRequested()));
    QVERIFY(externalPopupSpy.isValid());
    const QPoint comboVisiblePoint = combo->mapToScene(
        QPointF(combo->width() * 0.75, combo->height() / 2.0)).toPoint();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      comboVisiblePoint);
    QTRY_COMPARE_WITH_TIMEOUT(externalPopupSpy.size(), 1, 1000);
    QTRY_COMPARE_WITH_TIMEOUT(comboActions().size(), 1, 1000);
    QCOMPARE(comboActions().constFirst().value(QStringLiteral("action")).toString(),
             QStringLiteral("control.open"));
    QVERIFY(!nativePopup->property("visible").toBool());

    // Neither physical Enter key belongs to QML. The global key sink remains
    // focused and forwards it to Go, while the local ComboBox popup stays
    // closed until Go publishes its VMenu.
    for (const auto key : {Qt::Key_Return, Qt::Key_Enter}) {
        fixture.shell.clearActions();
        QTest::keyClick(fixture.window, key);
        QTest::qWait(30);
        QVERIFY(!nativePopup->property("visible").toBool());
        QCOMPARE(comboActions().size(), 0);
        QVERIFY(grid->hasActiveFocus());
    }

    // This is the response produced by ComboBox.ProcessKey in Go. QML must
    // present that semantic menu above its owning full-window dialog rather
    // than opening a second, Qt-owned popup beneath or outside the Go stack.
    // Compare actual glyph pixels, not just the Text item's origin: internal
    // padding and external margins can have different raster rounding.
    fixture.shell.setScene(dialogComboMenuScene(1));
    QVariant overlayFrames;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "overlayFrames",
        Q_RETURN_ARG(QVariant, overlayFrames)));
    const QVariantList frames = overlayFrames.toList();
    QCOMPARE(frames.size(), 2);
    QCOMPARE(frames.at(0).toMap().value(QStringLiteral("kind")).toString(),
             QStringLiteral("dialog"));
    QCOMPARE(frames.at(1).toMap().value(QStringLiteral("kind")).toString(),
             QStringLiteral("menu"));

    QQuickItem *const rootItem = fixture.window->contentItem();
    QPointer<QQuickItem> semanticPopup;
    QPointer<QQuickItem> semanticList;
    QTRY_VERIFY_WITH_TIMEOUT(
        (semanticPopup = visualItem(rootItem, QStringLiteral(
             "semanticMenuPopup-appearance-combo-menu"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (semanticList = visualItem(rootItem, QStringLiteral(
             "semanticMenuList-appearance-combo-menu"))),
        3000);
    QPointer<QQuickItem> semanticMenuOverlay = semanticPopup->parentItem();
    QVERIFY(semanticMenuOverlay);
    QVERIFY(semanticMenuOverlay->property("dropdownMode").toBool());
    QCOMPARE(semanticMenuOverlay->property("semanticSelectedIndex").toInt(),
             1);
    QCOMPARE(semanticMenuOverlay->property("visualSelectedIndex").toInt(),
             1);
    QTRY_COMPARE_WITH_TIMEOUT(semanticList->property("currentIndex").toInt(),
                              1, 1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        semanticMenuOverlay->property("dropdownOpenSettled").toBool(), 1000);
    QCOMPARE(semanticMenuOverlay->property("revealProgress").toReal(), 1.0);
    QVERIFY(!semanticMenuOverlay->property("dropdownAnimationRunning").toBool());

    QQuickItem *selectedRow = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (selectedRow = visualItem(rootItem, QStringLiteral(
             "semanticMenuItem-appearance-combo-menu-1"))),
        1000);
    const QPointF comboTop = combo->mapToItem(rootItem, QPointF{});
    const QPointF selectedTop = selectedRow->mapToItem(rootItem, QPointF{});
    const qreal dpr = fixture.window->devicePixelRatio();
    const qreal snappedComboTop = qRound(comboTop.y() * dpr) / dpr;
    QVERIFY2(qAbs((selectedTop.x() - comboTop.x()) * dpr) < 0.01,
             qPrintable(QStringLiteral(
                 "selected dropdown row changed physical x: expanded=%1 "
                 "collapsed=%2")
                            .arg(selectedTop.x())
                            .arg(comboTop.x())));
    QVERIFY2(qAbs(selectedTop.y() - snappedComboTop) < 0.01,
             qPrintable(QStringLiteral(
                 "selected dropdown row moved from its control: %1 vs %2")
                             .arg(selectedTop.y()).arg(snappedComboTop)));
    QVERIFY2(qAbs((selectedRow->width() - combo->width()) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "selected dropdown row width does not match the collapsed "
                 "control: row=%1 combo=%2")
                             .arg(selectedRow->width()).arg(combo->width())));
    QVERIFY2(qAbs((selectedRow->height() - combo->height()) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "selected dropdown row height does not match the collapsed "
                 "control: row=%1 combo=%2")
                             .arg(selectedRow->height()).arg(combo->height())));
    const qreal menuEdgeInset = semanticMenuOverlay->property(
        "menuEdgeInset").toReal();
    const QPointF popupTopBeforeWidth = semanticPopup->mapToItem(
        rootItem, QPointF{});
    QVERIFY2(qAbs((semanticPopup->width()
                   - combo->width() - 2 * menuEdgeInset) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "dropdown frame width did not include both presentation insets: "
                 "popup=%1 combo=%2 inset=%3")
                            .arg(semanticPopup->width())
                            .arg(combo->width())
                            .arg(menuEdgeInset)));
    QVERIFY2(qAbs((popupTopBeforeWidth.x()
                   - (comboTop.x() - menuEdgeInset)) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "dropdown frame x did not compensate its presentation inset: "
                 "popup=%1 combo=%2 inset=%3")
                            .arg(popupTopBeforeWidth.x())
                            .arg(comboTop.x())
                            .arg(menuEdgeInset)));
    QVERIFY2(qAbs((popupTopBeforeWidth.x() + menuEdgeInset - comboTop.x())
                  * dpr) < 0.01,
             qPrintable(QStringLiteral(
                 "dropdown frame left edge changed physical x: expanded=%1 "
                 "collapsed=%2")
                            .arg(popupTopBeforeWidth.x() + menuEdgeInset)
                            .arg(comboTop.x())));
    const qreal selectedRight = selectedRow->mapToItem(
        rootItem, QPointF(selectedRow->width(), 0)).x();
    const qreal popupRight = popupTopBeforeWidth.x() + semanticPopup->width();
    QVERIFY2(qAbs((popupRight - selectedRight - menuEdgeInset) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "dropdown frame lost its trailing presentation inset: "
                 "popupRight=%1 selectedRight=%2 inset=%3")
                            .arg(popupRight)
                            .arg(selectedRight)
                            .arg(menuEdgeInset)));
    const QPointF popupTop = semanticPopup->mapToItem(rootItem, QPointF{});
    QVERIFY(popupTop.y() < comboTop.y());
    QVERIFY(popupTop.y() + semanticPopup->height()
            > comboTop.y() + combo->height());
    QVERIFY(!nativePopup->property("visible").toBool());
    QVERIFY(grid->hasActiveFocus());

    QQuickItem *selectedText = nullptr;
    QQuickItem *comboText = nullptr;
    QQuickItem *comboIndicator = nullptr;
    QQuickItem *expandedChevron = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (selectedText = visualItem(rootItem, QStringLiteral(
             "semanticMenuItemText-appearance-combo-menu-1"))),
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (comboText = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-comboComboBoxText"))),
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (comboIndicator = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-comboComboBoxIndicator"))),
        1000);
    const QPointF comboTextTop = comboText->mapToItem(rootItem, QPointF{});
    const auto closedGrab = comboText->grabToImage();
    QVERIFY(closedGrab);
    QTRY_VERIFY_WITH_TIMEOUT(!closedGrab->image().isNull(), 1000);
    const auto openedGrab = selectedText->grabToImage();
    QVERIFY(openedGrab);
    QTRY_VERIFY_WITH_TIMEOUT(!openedGrab->image().isNull(), 1000);
    QVERIFY2(openedGrab->image() == closedGrab->image(),
             "Opening the dropdown changed rasterized text (padding/layout rounding)");
    const QPointF selectedTextTop = selectedText->mapToItem(rootItem, QPointF{});
    const qreal comboGlyphX = comboTextTop.x()
            + comboText->property("leftPadding").toReal();
    QCOMPARE(selectedText->property("font").value<QFont>(),
             comboText->property("font").value<QFont>());
    const QPointF comboTextGlyphOrigin = comboText->mapToItem(
        rootItem, QPointF(comboText->property("leftPadding").toReal(), 0));
    const QPointF selectedTextGlyphOrigin = selectedText->mapToItem(
        rootItem, QPointF(selectedText->property("leftPadding").toReal(), 0));
    QVERIFY2(qAbs((selectedTextGlyphOrigin.x()
                   - comboTextGlyphOrigin.x()) * dpr) < 0.01,
             qPrintable(QStringLiteral(
                 "selected dropdown text changed physical x: expanded=%1 "
                 "collapsed=%2")
                            .arg(selectedTextGlyphOrigin.x())
                            .arg(comboTextGlyphOrigin.x())));
    QVERIFY2(qAbs((selectedTextTop.y() - comboTextTop.y()) * dpr) < 0.01,
             qPrintable(QStringLiteral(
                 "selected dropdown text changed physical y: expanded=%1 "
                 "collapsed=%2")
                            .arg(selectedTextTop.y())
                            .arg(comboTextTop.y())));
    QVERIFY2(qAbs((selectedTextGlyphOrigin.x() - comboGlyphX) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "selected dropdown text moved horizontally: popup=%1 combo=%2 "
                 "combo-item=%3 padding=%4")
                            .arg(selectedTextTop.x())
                            .arg(comboGlyphX)
                            .arg(comboTextTop.x())
                            .arg(comboText->property("leftPadding").toReal())));
    QTRY_VERIFY_WITH_TIMEOUT(
        (expandedChevron = visualItem(rootItem, QStringLiteral(
             "semanticDropdownChevronUp-appearance-combo-menu"))),
        1000);
    const QPointF comboIndicatorTop = comboIndicator->mapToItem(
        rootItem, QPointF{});
    const QPointF expandedChevronTop = expandedChevron->mapToItem(
        rootItem, QPointF{});
    QVERIFY2(qAbs((expandedChevronTop.x() - comboIndicatorTop.x()) * dpr)
                 < 0.01,
             qPrintable(QStringLiteral(
                 "expanded dropdown chevron changed physical x: expanded=%1 "
                 "collapsed=%2")
                            .arg(expandedChevronTop.x())
                            .arg(comboIndicatorTop.x())));
    QVERIFY2(qAbs((expandedChevronTop.y() - comboIndicatorTop.y()) * dpr)
                 < 0.01,
             qPrintable(QStringLiteral(
                 "expanded dropdown chevron changed physical y: expanded=%1 "
                 "collapsed=%2")
                            .arg(expandedChevronTop.y())
                            .arg(comboIndicatorTop.y())));
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required for the dropdown pixel gate");
    const auto verifyPixelAlignedLeaf = [rootItem, dpr](QQuickItem *leaf) {
        const QPointF origin = leaf->mapToItem(rootItem, QPointF{});
        const QPointF physical = origin * dpr;
        const QString details = QStringLiteral(
            "%1 physical origin is (%2, %3)")
                                    .arg(leaf->objectName())
                                    .arg(physical.x(), 0, 'f', 6)
                                    .arg(physical.y(), 0, 'f', 6);
        QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physical.y() - qRound(physical.y())) < 0.001,
                 qPrintable(details));
        const QPointF xAxis = leaf->mapToItem(rootItem, QPointF(1, 0)) - origin;
        const QPointF yAxis = leaf->mapToItem(rootItem, QPointF(0, 1)) - origin;
        QVERIFY2(qAbs(xAxis.x() - 1.0) < 0.001
                     && qAbs(xAxis.y()) < 0.001
                     && qAbs(yAxis.x()) < 0.001
                     && qAbs(yAxis.y() - 1.0) < 0.001,
                 qPrintable(QStringLiteral("%1 has a non-translation transform")
                                .arg(leaf->objectName())));
    };
    for (QQuickItem *leaf : {selectedRow, comboText, selectedText, comboIndicator, expandedChevron})
        verifyPixelAlignedLeaf(leaf);
    const QPointF physicalPopupTop = popupTop * dpr;
    const qreal physicalPopupWidth = semanticPopup->width() * dpr;
    const qreal physicalPopupHeight = semanticPopup->height() * dpr;
    QVERIFY(qAbs(physicalPopupTop.x() - qRound(physicalPopupTop.x())) < 0.001);
    QVERIFY(qAbs(physicalPopupTop.y() - qRound(physicalPopupTop.y())) < 0.001);
    QVERIFY(qAbs(physicalPopupWidth - qRound(physicalPopupWidth)) < 0.001);
    QVERIFY(qAbs(physicalPopupHeight - qRound(physicalPopupHeight)) < 0.001);
    QVERIFY(expandedChevron->property("rasterizedIconSource").toString()
                .contains(QStringLiteral("dpr=1.75")));
    QImage expandedFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(expandedFrame = fixture.window->grabWindow()).isNull(), 3000);

    // Once the bounded transition finishes, this surface must be completely
    // idle: no timer or perpetual animation may keep presenting frames.
    QTest::qWait(350);
    QSignalSpy idleDropdownFrames(fixture.window, &QQuickWindow::frameSwapped);
    QVERIFY(idleDropdownFrames.isValid());
    QTest::qWait(160);
    QCOMPARE(idleDropdownFrames.size(), 0);

    const auto menuActions = [&fixture]() {
        QVector<QVariantMap> result;
        for (const QVariantMap &action : std::as_const(fixture.shell.actions)) {
            if (action.value(QStringLiteral("target")).toString()
                == QStringLiteral("appearance-combo-menu")) {
                result.append(action);
            }
        }
        return result;
    };
    QQuickItem *firstRow = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (firstRow = visualItem(rootItem, QStringLiteral(
             "semanticMenuItem-appearance-combo-menu-0"))),
        1000);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      itemCenter(firstRow));
    QTRY_COMPARE_WITH_TIMEOUT(menuActions().size(), 2, 1000);
    QCOMPARE(menuActions().at(0).value(QStringLiteral("action")).toString(),
             QStringLiteral("menu.select"));
    QCOMPARE(menuActions().at(0).value(QStringLiteral("index")).toInt(), 0);
    QCOMPARE(menuActions().at(1).value(QStringLiteral("action")).toString(),
             QStringLiteral("menu.activate"));
    QCOMPARE(menuActions().at(1).value(QStringLiteral("index")).toInt(), 0);

    QQuickItem *const overlayHost = fixture.item(
        QStringLiteral("semanticOverlayHost"));
    QVERIFY(overlayHost);
    QObject *const frameRepeater = overlayHost->findChild<QObject *>(
        QStringLiteral("semanticOverlayRepeater"));
    QVERIFY(frameRepeater);
    QQuickItem *dialogLoader = nullptr;
    QQuickItem *menuLoader = nullptr;
    QVERIFY(QMetaObject::invokeMethod(
        frameRepeater, "itemAt", Q_RETURN_ARG(QQuickItem *, dialogLoader),
        Q_ARG(int, 0)));
    QVERIFY(QMetaObject::invokeMethod(
        frameRepeater, "itemAt", Q_RETURN_ARG(QQuickItem *, menuLoader),
        Q_ARG(int, 1)));
    QVERIFY(dialogLoader);
    QVERIFY(menuLoader);
    QVERIFY(menuLoader->z() > dialogLoader->z());

    // Selection and closure continue to be projections of Go state.
    fixture.shell.setScene(dialogComboMenuScene(0));
    QTRY_COMPARE_WITH_TIMEOUT(semanticList->property("currentIndex").toInt(),
                              0, 1000);
    fixture.shell.setScene(dialogControlsScene(true, true));
    QVERIFY(!semanticMenuOverlay.isNull());
    QVERIFY(semanticMenuOverlay->property("closing").toBool());
    QCOMPARE(semanticMenuOverlay->property("closingSelectedIndex").toInt(), 0);
    QVERIFY(semanticMenuOverlay->property("dropdownAnimationRunning").toBool());
    QVERIFY(visualItem(rootItem, QStringLiteral(
        "semanticMenuPopup-appearance-combo-menu")));
    QTRY_VERIFY_WITH_TIMEOUT(
        semanticMenuOverlay.isNull()
            && !visualItem(rootItem, QStringLiteral(
                   "semanticMenuPopup-appearance-combo-menu")),
        1000);

    // Text presentation owns the same Go menu in the fallback grid. Hidden
    // native controls must never register an application-wide Enter handler
    // or open a GUI popup on top of console mode.
    fixture.shell.setScene(dialogComboMenuScene(1, QStringLiteral("text")));
    QVariant fallback;
    QVERIFY(QMetaObject::invokeMethod(
        fixture.window, "needsFallbackGrid",
        Q_RETURN_ARG(QVariant, fallback)));
    QVERIFY(fallback.toBool());
    for (const auto key : {Qt::Key_Return, Qt::Key_Enter}) {
        fixture.shell.clearActions();
        QTest::keyClick(fixture.window, key);
        QTest::qWait(30);
        QCOMPARE(comboActions().size(), 0);
        QVERIFY(grid->hasActiveFocus());
    }
}

void F4OperationsQueueTests::semanticOverlayMenuStreamClearsWhileFallbackSurfaceIsHidden()
{
    QVariantMap scene = dialogControlsScene(true, true);
    scene.insert(QStringLiteral("presentation"), QStringLiteral("text"));
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    // In console/text presentation the native surface is hidden, but the
    // overlay model still receives the same Go-owned menu stream.  Closing
    // that stream must retire the retained dropdown before a later GUI
    // presentation can make the hidden QML surface visible again.
    fixture.shell.applyCommandMenus(QVariantList{dialogComboMenu(1)});
    QTRY_VERIFY_WITH_TIMEOUT(
        visualItem(fixture.window->contentItem(), QStringLiteral(
            "semanticMenuPopup-appearance-combo-menu")),
        1000);
    fixture.shell.applyCommandMenus({});
    QTRY_VERIFY_WITH_TIMEOUT(
        !visualItem(fixture.window->contentItem(), QStringLiteral(
            "semanticMenuPopup-appearance-combo-menu")),
        1000);
}

void F4OperationsQueueTests::semanticOverlayDialogStreamClearsWithoutStaleOverlay()
{
    QueueFixture fixture(dialogControlsScene(true));
    QVERIFY(fixture.window);

    QQuickItem *const overlayHost = fixture.item(
        QStringLiteral("semanticOverlayHost"));
    QVERIFY(overlayHost);
    QObject *const frameModel = overlayHost->findChild<QObject *>(
        QStringLiteral("semanticOverlayFrameModel"));
    QVERIFY(frameModel);
    QTRY_COMPARE_WITH_TIMEOUT(qobject_cast<QAbstractItemModel *>(frameModel)->rowCount(), 1,
                              1000);
    QVERIFY(visualItem(fixture.window->contentItem(),
                       QStringLiteral("semanticDialog-appearance-dialog")));

    // This is the production transition emitted when Go closes a dialog.
    // The QML overlay must retire it even if the native panel remains alive
    // below the overlay and no unrelated scene change follows.
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), QVariantList{}},
    }, 2);

    QTRY_COMPARE_WITH_TIMEOUT(qobject_cast<QAbstractItemModel *>(frameModel)->rowCount(), 0,
                              1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        !visualItem(fixture.window->contentItem(),
                    QStringLiteral("semanticDialog-appearance-dialog")),
        1000);
}

void F4OperationsQueueTests::semanticInlineLabelsKeepControlsClose()
{
    auto scene = dialogScene();
    auto dialogs = scene.value("dialogs").toList();
    auto dialog = dialogs[0].toMap();
    dialog["title"] = "Additional panel settings";
    dialog["children"] = QVariantList{
        QVariantMap{{"id", "workers-label"}, {"kind", "text"}, {"x", 20}, {"y", 6}, {"w", 38}, {"h", 1},
                    {"text", "Parallel apply workers (0 = Unlimited):"}},
        QVariantMap{{"id", "workers"}, {"kind", "edit"}, {"x", 59}, {"y", 6}, {"w", 10}, {"h", 1}, {"text", "32"}},
        QVariantMap{{"id", "mode-label"}, {"kind", "text"}, {"x", 20}, {"y", 8}, {"w", 25}, {"h", 1},
                    {"text", "Default operation mode:"}},
        QVariantMap{{"id", "mode"}, {"kind", "comboBox"}, {"x", 46}, {"y", 8}, {"w", 20}, {"h", 1},
                    {"items", QVariantList{"Queue", "Parallel"}}, {"text", "Queue"}, {"selected", 0}},
    };
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QTRY_VERIFY(visualItem(root, "dialogWidget-modeComboBox"));
    for (int size : {18, 14}) {
        auto font = fixture.window->property("font").value<QFont>();
        font.setPixelSize(size);
        fixture.window->setProperty("font", font);
        QTest::qWait(150);
        for (const auto &id : {QString("workers"), QString("mode")}) {
            auto *label = visualItem(root, "dialogWidget-" + id + "-labelText");
            auto *control = visualItem(root, "dialogWidget-" + id + (id == "workers" ? "Edit" : "ComboBox"));
            QVERIFY(label); QVERIFY(control);
            const qreal gap = control->mapToItem(root, QPointF{}).x()
                            - label->mapToItem(root, QPointF{}).x()
                            - label->property("contentWidth").toReal();
            QVERIFY2(qAbs(gap - 12) < 1, qPrintable(QString("%1 gap=%2").arg(id).arg(gap)));
        }
        const qreal dpr = fixture.window->devicePixelRatio();
        for (const auto &name : {"semanticDialogTitle", "dialogWidget-workers-labelText", "dialogWidget-mode-labelText",
                                 "dialogWidget-workersEditTextInput", "dialogWidget-modeComboBoxText", "dialogWidget-modeComboBoxIndicator"}) {
            auto *leaf = visualItem(root, name);
            QVERIFY2(leaf, name);
            const auto origin = leaf->mapToItem(root, QPointF{});
            QVERIFY2(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001, name);
            QVERIFY2(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001, name);
            QCOMPARE(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1));
        }
    }
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_DIALOG_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_DIALOG_CAPTURE")));
}

void F4OperationsQueueTests::menuChildDialogFollowsGoStackOrder()
{
    auto scene = dialogScene();
    auto dialog = scene.value("dialogs").toList().first().toMap();
    dialog["stackOrder"] = 2;
    dialog["title"] = "Plugin hotkey";
    dialog["children"] = QVariantList{};
    auto menu = dialogComboMenu(0);
    menu["id"] = "plugins-menu";
    menu["title"] = "Plugins";
    menu["presentation"] = "window";
    menu["ownerId"] = "";
    menu["stackOrder"] = 1;
    scene["dialogs"] = QVariantList{dialog};
    scene["menus"] = QVariantList{menu};
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QVariant result;
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "overlayFrames",
                                      Q_RETURN_ARG(QVariant, result)));
    QCOMPARE(result.toList().last().toMap().value("id"), dialog.value("id"));
    auto *repeater = fixture.item("semanticOverlayHost")
                         ->findChild<QObject *>("semanticOverlayRepeater");
    QVERIFY(repeater);
    QQuickItem *lower = nullptr;
    QQuickItem *upper = nullptr;
    QVERIFY(QMetaObject::invokeMethod(repeater, "itemAt", Q_RETURN_ARG(QQuickItem *, lower), Q_ARG(int, 0)));
    QVERIFY(QMetaObject::invokeMethod(repeater, "itemAt", Q_RETURN_ARG(QQuickItem *, upper), Q_ARG(int, 1)));
    QVERIFY(lower && upper);
    QVERIFY(upper->z() > lower->z());
    auto *surface = visualItem(upper, "semanticDialog-" + dialog.value("id").toString());
    QVERIFY(surface);
    auto *title = visualItem(surface, "semanticDialogTitle");
    QVERIFY(title);
    auto *closeButton = visualItem(surface, "dialogCloseButton");
    QVERIFY(closeButton);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(closeButton));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(),
        [&dialog](const QVariantMap &action) {
            return action.value("target") == dialog.value("id")
                && action.value("action").toString() == QStringLiteral("dialog.close");
        }));
    const auto origin = title->mapToScene(QPointF{});
    const auto dpr = fixture.window->devicePixelRatio();
    QVERIFY(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001);
    QVERIFY(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001);
    QCOMPARE(title->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
    QCOMPARE(title->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_DIALOG_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_DIALOG_CAPTURE")));

    // A menu subsequently opened by that dialog must become the top layer.
    menu["stackOrder"] = 3;
    scene["menus"] = QVariantList{menu};
    fixture.shell.setScene(scene);
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "overlayFrames",
                                      Q_RETURN_ARG(QVariant, result)));
    QCOMPARE(result.toList().last().toMap().value("id"), menu.value("id"));
    // Closing the child restores the retained menu, without recreating it.
    scene["dialogs"] = QVariantList{};
    fixture.shell.setScene(scene);
    QVERIFY(QMetaObject::invokeMethod(fixture.window, "overlayFrames",
                                      Q_RETURN_ARG(QVariant, result)));
    QCOMPARE(result.toList().size(), 1);
}

void F4OperationsQueueTests::environmentProfilesHeldResizeKeepsMouseGrab()
{
    auto scene = dialogScene();
    auto dialogs = scene.value("dialogs").toList();
    auto dialog = dialogs.first().toMap();
    dialog["title"] = "Environment profiles";
    dialog["paneSplit"] = QVariantMap{{"x", 45}, {"leftTitle", "Profiles"},
        {"rightTitle", "Profile details"}, {"active", "left"}};
    dialog["children"] = QVariantList{
        QVariantMap{{"id", "env-profiles"}, {"kind", "listBox"}, {"x", 20}, {"y", 6},
            {"w", 24}, {"h", 8}, {"items", QVariantList{"Default", "Development"}}, {"selected", 0}},
        QVariantMap{{"id", "env-details"}, {"kind", "edit"}, {"x", 47}, {"y", 6},
            {"w", 30}, {"h", 1}, {"text", "PATH=C:\\Tools"}, {"readOnly", true}}};
    dialog["keyHints"] = QVariantList{
        QVariantMap{{"action", "envman.add"}, {"key", "Ins"}, {"text", "Add"}},
        QVariantMap{{"action", "envman.delete"}, {"key", "Del"}, {"text", "Delete"}},
        QVariantMap{{"action", "envman.edit"}, {"key", "F4"}, {"text", "Edit"}},
        QVariantMap{{"action", "envman.copy"}, {"key", "F5"}, {"text", "Copy"}},
        QVariantMap{{"action", "envman.toggle"}, {"key", "Space"}, {"text", "Toggle"}},
        QVariantMap{{"action", "envman.moveUp"}, {"key", "Ctrl+Up"}, {"text", "Move"}},
        QVariantMap{{"action", "envman.moveDown"}, {"key", "Ctrl+Down"}, {"text", "Move"}},
        QVariantMap{{"action", "envman.settings"}, {"key", "F2"}, {"text", "Settings"}}};
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    if (qEnvironmentVariable("QT_SCALE_FACTOR") == "1.75")
        QCOMPARE(fixture.window->devicePixelRatio(), 1.75);
    auto *root = fixture.window->contentItem();
    QQuickItem *surface = nullptr;
    QTRY_VERIFY((surface = visualItem(root, "semanticDialog-appearance-dialog")));
    const QStringList corners{"dialogResizeBottomRight", "dialogResizeTopLeft",
        "dialogResizeTopRight", "dialogResizeBottomLeft"};
    for (int cornerIndex = 0; cornerIndex < corners.size(); ++cornerIndex) {
        QVERIFY(QMetaObject::invokeMethod(surface, "setUserGeometry",
            Q_ARG(QVariant, QVariant(120.0)), Q_ARG(QVariant, QVariant(100.0)),
            Q_ARG(QVariant, QVariant(700.0)), Q_ARG(QVariant, QVariant(500.0))));
        QTest::qWait(40);
        auto *handle = visualItem(surface, corners[cornerIndex]);
        QVERIFY(handle);
        const auto press = handle->mapToScene(QPointF(handle->width()/2, handle->height()/2)).toPoint();
        const qreal initialWidth = surface->width();
        const qreal initialHeight = surface->height();
        const int horizontal = (cornerIndex == 1 || cornerIndex == 3) ? 1 : -1;
        const int vertical = (cornerIndex == 1 || cornerIndex == 2) ? 1 : -1;
        fixture.shell.clearActions();
        QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, press);
        QVERIFY(handle->property("pressed").toBool());
        QPoint lastPoint = press;
        // Large inward steps cross the read-only pane and footer while held.
        // Reverse direction without releasing to catch stale drag ownership.
        for (int step : {1, 2, 3, 4, 5, 6, 5, 4}) {
            lastPoint = press + QPoint(horizontal * step * 25, vertical * step * 20);
            QTest::mouseMove(fixture.window, lastPoint, 10);
            QTRY_VERIFY(qAbs(surface->width() - (initialWidth - step * 25)) < 1.0);
            QTRY_VERIFY(qAbs(surface->height() - (initialHeight - step * 20)) < 1.0);
            QVERIFY(handle->property("pressed").toBool());
            for (const auto &action : std::as_const(fixture.shell.actions))
                QVERIFY(action.value("action").toString() != "dialog.geometry");
        }
        QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, lastPoint);
        QVERIFY(!handle->property("pressed").toBool());
        int commits = 0;
        for (const auto &action : std::as_const(fixture.shell.actions))
            if (action.value("action").toString() == "dialog.geometry") ++commits;
        QCOMPARE(commits, 1);
        const QSizeF releasedSize(surface->width(), surface->height());
        QTest::mouseMove(fixture.window, lastPoint + QPoint(30, 30), 10);
        QCOMPARE(QSizeF(surface->width(), surface->height()), releasedSize);
    }
}

void F4OperationsQueueTests::semanticListRepeatPreservesRowsAndImmediateCursor()
{
    auto scene = panelScene();
    QVariantList items, states;
    for (int i = 0; i < 40; ++i) {
        items.append(QString("Profile %1").arg(i));
        states.append(QVariantMap{{"checkable", true}, {"checked", i % 2 == 0}});
    }
    QVariantMap list{{"id", "repeat-list"}, {"kind", "listBox"}, {"x", 2}, {"y", 3},
        {"w", 50}, {"h", 15}, {"items", items}, {"itemStates", states}, {"cursor", 0}, {"focused", true}};
    QVariantMap dialog{{"id", "repeat-dialog"}, {"kind", "dialog"}, {"title", "Profiles"},
        {"x", 1}, {"y", 1}, {"w", 60}, {"h", 22}, {"children", QVariantList{list}},
        {"keyHints", QVariantList{QVariantMap{{"key", "F4"}, {"text", "Edit"},
            {"action", "envman.edit"}, {"icon", "pencil"}}}}};
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QTRY_VERIFY(visualItem(root, "dialogWidget-repeat-listListItemText-0"));
    QTest::qWait(100);
    QPointer<QQuickItem> first = visualItem(root, "dialogWidget-repeat-listListItemText-0");
    QPointer<QQuickItem> hint = visualItem(root, "semanticDialogHintButton-0");
    QElapsedTimer timer; timer.start();
    int invisibleCursorFrames = 0;
    for (int step = 0; step < 60; ++step) {
        const int cursor = 1 + step % 5;
        list["cursor"] = cursor;
        dialog["children"] = QVariantList{list};
        scene["dialogs"] = QVariantList{dialog};
        fixture.shell.setScene(scene);
        QCoreApplication::processEvents();
        QVERIFY2(first && first == visualItem(root, "dialogWidget-repeat-listListItemText-0"),
            "Cursor-only snapshots recreated unchanged list rows");
        QVERIFY2(hint && hint == visualItem(root, "semanticDialogHintButton-0"),
            "Cursor-only snapshots recreated unchanged footer buttons");
        auto *text = visualItem(root, QString("dialogWidget-repeat-listListItemText-%1").arg(cursor));
        QVERIFY(text);
        if (text->parentItem()->property("color").value<QColor>()
            != fixture.window->property("selectedBg").value<QColor>())
            ++invisibleCursorFrames;
    }
    qInfo() << "60 cursor snapshots, retained rows/footer, immediate highlight:" << timer.elapsed() << "ms";
    QCOMPARE(invisibleCursorFrames, 0);
    QVERIFY(!fixture.window->grabWindow().isNull());
    if (qEnvironmentVariableIsSet("F4_LIST_REPEAT_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_LIST_REPEAT_CAPTURE")));
}

void F4OperationsQueueTests::environmentProfilesFitDuringResize()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"env-resize","kind":"dialog","layout":"environmentProfiles","title":"Environment profiles","x":8,"y":3,"w":96,"h":30,
      "paneSplit":{"x":40,"leftTitle":"Profiles","rightTitle":"Profile details","active":"left"},
      "keyHints":[{"key":"Ins","text":"Add","icon":"plus","action":"envman.add"},{"key":"Del","text":"Delete","icon":"trash-2","action":"envman.delete"},
        {"key":"F4","text":"Edit","icon":"pencil","action":"envman.edit"},{"key":"F5","text":"Copy","icon":"copy","action":"envman.copy"},
        {"key":"Space","text":"Toggle","icon":"circle-check","action":"envman.toggle"},{"key":"Ctrl+Up","text":"Move","icon":"arrow-up","action":"envman.moveUp"},
        {"key":"Ctrl+Down","text":"Move","icon":"arrow-down","action":"envman.moveDown"},{"key":"F2","text":"Settings","icon":"file-cog","action":"envman.settings"}],
      "children":[
        {"id":"profiles","layoutRole":"profiles","kind":"listBox","x":10,"y":5,"w":28,"h":24,"items":["Default","Development"],"selected":0,
         "itemStates":[{"checkable":true,"checked":true},{"checkable":true,"checked":false,"dimmed":true}]},
        {"id":"name-label","layoutRole":"name-label","kind":"text","x":42,"y":5,"w":8,"h":1,"text":"Name:"},
        {"id":"name","layoutRole":"name","kind":"edit","x":42,"y":6,"w":60,"h":1,"text":"Development"},
        {"id":"enabled","layoutRole":"enabled","kind":"checkbox","x":42,"y":8,"w":20,"h":1,"text":"Enabled","checked":true},
        {"id":"variables-label","layoutRole":"variables-label","kind":"text","x":42,"y":10,"w":40,"h":1,"text":"Variables (NAME=value):"},
        {"id":"variables","layoutRole":"variables","kind":"multiLineEdit","x":42,"y":11,"w":60,"h":18,"text":"PATH=C:\\Tools\nBUILD=debug","readOnly":true},
        {"id":"add","layoutRole":"add","kind":"button","x":10,"y":31,"w":18,"h":1,"text":"Add profile"},
        {"id":"save","layoutRole":"save","kind":"button","x":65,"y":31,"w":18,"h":1,"text":"Save","visible":false},
        {"id":"cancel","layoutRole":"cancel","kind":"button","x":84,"y":31,"w":18,"h":1,"text":"Cancel","visible":false}
      ]})").toVariant().toMap();
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    fixture.window->resize(1300, 950);
    auto *root = fixture.window->contentItem();
    QQuickItem *surface = nullptr;
    QTRY_VERIFY((surface = visualItem(root, "semanticDialog-env-resize")));
    const auto dpr = fixture.window->devicePixelRatio();
    if (qEnvironmentVariable("QT_SCALE_FACTOR") == "1.75")
        QCOMPARE(dpr, 1.75);
    for (const bool editing : {false, true}) {
        if (editing) dialog["keyHints"] = QVariantList{
            QVariantMap{{"key","Ctrl+Enter"},{"text","Save"},{"icon","save"},{"action","envman.save"}},
            QVariantMap{{"key","Esc"},{"text","Cancel"},{"icon","x"},{"action","envman.cancel"}}};
        auto children = dialog["children"].toList();
        for (auto &value : children) {
            auto child = value.toMap();
            const auto role = child["layoutRole"].toString();
            if (role == "add") child["visible"] = !editing;
            if (role == "save" || role == "cancel") child["visible"] = editing;
            value = child;
        }
        dialog["children"] = children;
        scene["dialogs"] = QVariantList{dialog};
        fixture.shell.setScene(scene);
        for (const auto size : {QSize(850, 700), QSize(600, 420), QSize(320, 160), QSize(950, 760)}) {
            QVERIFY(QMetaObject::invokeMethod(surface, "setUserGeometry", Q_ARG(QVariant, 30), Q_ARG(QVariant, 50),
                Q_ARG(QVariant, size.width()), Q_ARG(QVariant, size.height())));
            QTest::qWait(100);
            QVERIFY(!fixture.window->grabWindow().isNull());
            auto *scroll = visualItem(surface, "dialogBodyScrollBar");
            QVERIFY(!scroll || !scroll->isVisible());
            auto *left = visualItem(surface, "dialogWidget-profilesRoot");
            auto *right = visualItem(surface, "dialogWidget-variablesRoot");
            QVERIFY(left && right);
            auto *nameLabel = visualItem(surface, "dialogWidget-name-labelRoot");
            auto *name = visualItem(surface, "dialogWidget-nameRoot");
            auto *enabled = visualItem(surface, "dialogWidget-enabledRoot");
            QVERIFY(nameLabel && name && enabled);
            auto *nameText = visualItem(nameLabel, "dialogWidget-name-labelText");
            QVERIFY(nameText);
            QVERIFY2(!nameText->property("truncated").toBool(), qPrintable(QString("Name caption truncated: width=%1 implicit=%2 font=%3 measure=%4")
                .arg(nameText->width()).arg(nameText->implicitWidth()).arg(nameText->property("font").value<QFont>().toString())
                .arg(visualItem(surface, "environmentNameMeasure")->implicitWidth())));
            const auto labelCenter = nameLabel->mapToScene(QPointF(0, nameLabel->height()/2));
            const auto nameCenter = name->mapToScene(QPointF(0, name->height()/2));
            const auto enabledCenter = enabled->mapToScene(QPointF(0, enabled->height()/2));
            QVERIFY2(qAbs(labelCenter.y()-nameCenter.y())*dpr <= 1
                     && qAbs(enabledCenter.y()-nameCenter.y())*dpr <= 1,
                qPrintable(QString("name row physical centers: label=%1 input=%2 enabled=%3")
                    .arg(labelCenter.y()*dpr).arg(nameCenter.y()*dpr).arg(enabledCenter.y()*dpr)));
            QVERIFY(labelCenter.x()+nameLabel->width() <= nameCenter.x());
            QVERIFY(nameCenter.x()+name->width() <= enabledCenter.x());
            QVERIFY(name->width() >= 32);
            const auto leftBottom = left->mapToScene(QPointF(0,left->height())).y();
            const auto rightBottom = right->mapToScene(QPointF(0,right->height())).y();
            QVERIFY2(qAbs(leftBottom-rightBottom) < .001,
                qPrintable(QString("pane bottoms %1 / %2").arg(leftBottom).arg(rightBottom)));
            QVERIFY2(right->height() >= 31, qPrintable(QString("requested=%1x%2 actual=%3x%4 rightHeight=%5 minimum=%6")
                .arg(size.width()).arg(size.height()).arg(surface->width()).arg(surface->height()).arg(right->height()).arg(surface->property("minimumDialogHeight").toReal())));
            auto *action = visualItem(surface, editing ? "dialogWidget-saveRoot" : "dialogWidget-addRoot");
            QVERIFY(action && action->isVisible());
            QVERIFY(action->mapToScene(QPointF()).y() >= leftBottom);
            const auto checkLeaves = [&](auto &&self, QQuickItem *item) -> void {
                if (item->isVisible() && (item->inherits("QQuickText") || item->inherits("QQuickTextInput")
                    || item->inherits("QQuickTextEdit") || item->inherits("QQuickImage"))) {
                    QVERIFY2(!item->objectName().isEmpty(), item->metaObject()->className());
                    const auto origin = item->mapToItem(root, QPointF());
                    const auto physical = origin*dpr;
                    QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
                        qPrintable(QString("%1 physical=(%2,%3)").arg(item->objectName()).arg(physical.x()).arg(physical.y())));
                    QVERIFY(QLineF(item->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0)).length() < .0001);
                    QVERIFY(QLineF(item->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1)).length() < .0001);
                }
                for (auto *child : item->childItems()) self(self,child);
            };
            checkLeaves(checkLeaves,surface);
            if (qEnvironmentVariableIsSet("F4_ENVMAN_RESIZE_CAPTURE"))
                QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_ENVMAN_RESIZE_CAPTURE")
                    + QString("-%1-%2.png").arg(editing).arg(size.width())));
            const auto previous = right->height();
            // A source-cell geometry acknowledgement must not reset native allocation.
            dialog["h"] = size.height()/20;
            scene["dialogs"] = QVariantList{dialog}; fixture.shell.setScene(scene);
            QTest::qWait(40);
            QCOMPARE(right->height(),previous);
        }
    }
}

void F4OperationsQueueTests::semanticDialogKeyHintsStayVisibleAndPixelAligned()
{
    auto scene = dialogScene();
    auto dialogs = scene.value("dialogs").toList();
    auto dialog = dialogs.first().toMap();
    const QVariantList hints{
        QVariantMap{{"icon", "file-cog"}, {"action", "envman.settings"}, {"key", "F2"}, {"text", "Settings"}},
        QVariantMap{{"icon", "pencil"}, {"action", "envman.edit"}, {"key", "F4"}, {"text", "Edit"}},
        QVariantMap{{"icon", "plus"}, {"action", "envman.new"}, {"key", "Insert"}, {"text", "New profile"}},
        QVariantMap{{"icon", "trash-2"}, {"action", "envman.remove"}, {"key", "Delete"}, {"text", "Remove"}},
        QVariantMap{{"icon", "check"}, {"action", "envman.apply"}, {"key", "Enter"}, {"text", "Apply"}},
        QVariantMap{{"icon", "x"}, {"action", "dialog.close"}, {"key", "Escape"}, {"text", "Close"}},
        QVariantMap{{"icon", "arrow-up"}, {"action", "envman.move-up"}, {"key", QString::fromUtf8("Ctrl+↑")}, {"text", "Move"}},
        QVariantMap{{"icon", "arrow-down"}, {"action", "envman.move-down"}, {"key", QString::fromUtf8("Ctrl+↓")}, {"text", "Move"}}};
    dialog["title"] = "Environment Manager";
    dialog["children"] = QVariantList{
        QVariantMap{{"id", "env-profiles"}, {"kind", "listBox"}, {"x", 20}, {"y", 6}, {"w", 24}, {"h", 8},
                    {"items", QVariantList{"Default", "Development"}}, {"selected", 0}},
        QVariantMap{{"id", "env-details"}, {"kind", "edit"}, {"x", 47}, {"y", 6}, {"w", 30}, {"h", 1},
                    {"text", "PATH=C:\\Tools"}}
    };
    dialog["keyHints"] = hints;
    dialog["paneSplit"] = QVariantMap{{"x", 45}, {"leftTitle", "Profiles"}, {"rightTitle", "Profile details"}, {"active", "left"}};
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *footer = nullptr;
    QTRY_VERIFY((footer = visualItem(root, "semanticDialogKeyHints")));
    auto *surface = visualItem(root, "semanticDialog-appearance-dialog");
    auto *body = visualItem(surface, "dialogBody");
    QVERIFY(surface); QVERIFY(body);
    auto *leftTitle = visualItem(root, "semanticDialogLeftPaneTitle");
    auto *rightTitle = visualItem(root, "semanticDialogRightPaneTitle");
    auto *divider = visualItem(root, "semanticDialogPaneDivider");
    QVERIFY(leftTitle); QVERIFY(rightTitle); QVERIFY(divider);
    const qreal dpr = fixture.window->devicePixelRatio();
    for (const int width : {640, 320}) {
        surface->setProperty("userGeometrySet", true);
        surface->setProperty("userWidth", width);
        surface->setProperty("userHeight", 260);
        QTest::qWait(100);
        QVERIFY(!fixture.window->grabWindow().isNull());
        QTest::qWait(50);
        QVERIFY(footer->isVisible());
        const auto bodyBottom = body->mapToScene(QPointF(0, body->height())).y();
        const auto footerTop = footer->mapToScene(QPointF{}).y();
        QVERIFY2(bodyBottom <= footerTop + .001,
                 qPrintable(QString("bodyBottom=%1 footerTop=%2").arg(bodyBottom, 0, 'f', 6).arg(footerTop, 0, 'f', 6)));
        const auto footerOrigin = footer->mapToScene(QPointF{});
        body->setProperty("contentY", 50);
        QCOMPARE(footer->mapToScene(QPointF{}), footerOrigin);
        QVERIFY(!fixture.window->grabWindow().isNull());
        QTest::qWait(50);
        for (int i = 0; i < hints.size(); ++i) {
            auto *button = visualItem(root, QString("semanticDialogHintButton-%1").arg(i));
            QVERIFY2(button, "Footer hints must reuse clickable F-bar buttons");
            auto *key = visualItem(root, QString("semanticDialogHintKey-%1").arg(i));
            auto *label = visualItem(root, QString("semanticDialogHintLabel-%1").arg(i));
            QVERIFY(key && label);
            QVERIFY(key->mapToScene(QPointF{}).x() > label->mapToScene(QPointF{}).x());
            auto *separator = visualItem(root, QString("semanticDialogHintSeparator-%1").arg(i));
            QVERIFY(separator);
            const auto buttonOrigin = button->mapToScene(QPointF{}) * dpr;
            const auto separatorOrigin = separator->mapToScene(QPointF{}) * dpr;
            for (const auto value : {buttonOrigin.x(), buttonOrigin.y(), button->width()*dpr,
                                     button->height()*dpr, separatorOrigin.x(), separatorOrigin.y(),
                                     separator->width()*dpr, separator->height()*dpr})
                QVERIFY2(qAbs(value-qRound(value)) < .001,
                         qPrintable(QString("button %1 physical=%2").arg(i).arg(value, 0, 'f', 6)));
            fixture.shell.clearActions();
            const auto center = button->mapToScene(QPointF(button->width()/2, button->height()/2)).toPoint();
            QTest::mouseMove(fixture.window, center);
            QCOMPARE(key->property("color").value<QColor>(), fixture.window->property("dialogAccent").value<QColor>());
            QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, center);
            QTRY_COMPARE(fixture.shell.actions.size(), 1);
            QCOMPARE(fixture.shell.actions.last().value("action"), hints[i].toMap().value("action"));
            QCOMPARE(fixture.shell.actions.last().value("target").toString(), QString("appearance-dialog"));
            for (const auto *kind : {"Key", "Label", "Icon"}) {
                const auto name = QString("semanticDialogHint%1-%2").arg(kind).arg(i);
                auto *leaf = visualItem(root, name);
                QVERIFY2(leaf, qPrintable(name));
                QVERIFY(leaf->isVisible());
                if (QString(kind) == "Icon")
                    QTRY_COMPARE(leaf->property("status").toInt(), 1);
                QVERIFY2(!leaf->property("truncated").toBool(), qPrintable(name));
                const auto origin = leaf->mapToItem(root, QPointF{});
                const auto physical = origin * dpr;
                const auto details = QString("%1 physical origin=(%2,%3)").arg(name).arg(physical.x(), 0, 'f', 6).arg(physical.y(), 0, 'f', 6);
                QVERIFY2(qAbs(physical.x()-qRound(physical.x())) < .001, qPrintable(details));
                QVERIFY2(qAbs(physical.y()-qRound(physical.y())) < .001, qPrintable(details));
                QCOMPARE(leaf->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0));
                QCOMPARE(leaf->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1));
                const auto local = leaf->mapToItem(footer, QPointF{});
                QVERIFY(local.x() >= 0 && local.y() >= 0);
                QVERIFY(local.x()+leaf->width() <= footer->width()+.001);
                QVERIFY(local.y()+leaf->height() <= footer->height()+.001);
            }
        }
        for (auto *leaf : {leftTitle, rightTitle, visualItem(root, "dialogWidget-env-detailsEditTextInput"),
                           visualItem(root, "dialogWidget-env-profilesListItemText-0"),
                           visualItem(root, "dialogWidget-env-profilesListItemText-1")}) {
            QVERIFY(leaf);
            const auto origin = leaf->mapToScene(QPointF{});
            for (const auto value : {origin.x()*dpr, origin.y()*dpr})
                QVERIFY2(qAbs(value-qRound(value)) < .001, qPrintable(QString("%1 physical=%2").arg(leaf->objectName()).arg(value, 0, 'f', 6)));
            QCOMPARE(leaf->mapToScene(QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToScene(QPointF(0,1))-origin, QPointF(0,1));
        }
        QVERIFY(divider->mapToScene(QPointF(0, divider->height())).y() <= footer->mapToScene(QPointF{}).y());
        for (auto *item : {footer, visualItem(root, "semanticDialogKeyHintsSeparator"), divider}) {
            QVERIFY(item);
            const auto origin = item->mapToScene(QPointF{}) * dpr;
            for (const auto value : {origin.x(), origin.y(), item->width()*dpr, item->height()*dpr})
                QVERIFY2(qAbs(value-qRound(value)) < .001, qPrintable(QString::number(value, 'f', 6)));
        }
    }
    surface->setProperty("userWidth", 640);
    body->setProperty("contentY", 0);
    QTest::qWait(100);
    auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_DIALOG_HINTS_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_DIALOG_HINTS_CAPTURE")));
    dialog["paneSplit"] = QVariantMap{{"x", 45}, {"leftTitle", "Profiles"}, {"rightTitle", "Editing profile"}, {"active", "right"}};
    dialog["keyHints"] = QVariantList{
        QVariantMap{{"icon", "save"}, {"action", "envman.save"}, {"key", "Ctrl+Enter"}, {"text", "Save"}},
        QVariantMap{{"icon", "x"}, {"action", "envman.cancel"}, {"key", "Esc"}, {"text", "Cancel"}, {"disabled", true}}};
    dialogs[0] = dialog; scene["dialogs"] = dialogs; fixture.shell.setScene(scene);
    QTRY_COMPARE(visualItem(root, "semanticDialogHintLabel-0")->property("text").toString(), QString("Save"));
    QTRY_COMPARE(visualItem(root, "semanticDialogHintLabel-1")->property("text").toString(), QString("Cancel"));
    QTRY_VERIFY(!visualItem(root, "semanticDialogHintLabel-2"));
    QVERIFY(!fixture.window->grabWindow().isNull());
    for (int i = 0; i < 2; ++i) {
        for (const auto *kind : {"Key", "Label", "Icon"}) {
            auto *leaf = visualItem(root, QString("semanticDialogHint%1-%2").arg(kind).arg(i));
            QVERIFY(leaf);
            const auto origin = leaf->mapToItem(root, QPointF{});
            for (const auto value : {origin.x()*dpr, origin.y()*dpr})
                QVERIFY2(qAbs(value-qRound(value)) < .001,
                         qPrintable(QString("%1 physical=%2").arg(leaf->objectName()).arg(value, 0, 'f', 6)));
            QCOMPARE(leaf->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1));
        }
    }
    auto *disabledButton = visualItem(root, "semanticDialogHintButton-1");
    QVERIFY(disabledButton);
    QVERIFY(!disabledButton->isEnabled());
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     disabledButton->mapToScene(QPointF(disabledButton->width()/2, disabledButton->height()/2)).toPoint());
    for (const auto &action : fixture.shell.actions)
        QVERIFY(action.value("action").toString() != QString("envman.cancel"));
    QTRY_COMPARE(rightTitle->property("text").toString(), QString("Editing profile"));
    QTRY_COMPARE(rightTitle->property("color").value<QColor>(), fixture.window->property("dialogAccent").value<QColor>());
    dialog.remove("keyHints"); dialogs[0] = dialog; scene["dialogs"] = dialogs; fixture.shell.setScene(scene);
    QTRY_VERIFY(!footer->isVisible());
    QTRY_COMPARE(footer->height(), 0);
}

void F4OperationsQueueTests::semanticListCheckboxesAndDimming()
{
    auto scene = dialogScene();
    auto dialog = scene.value("dialogs").toList().first().toMap();
    QVariantMap list{{"id", "profiles"}, {"kind", "listBox"}, {"x", 20}, {"y", 6},
                     {"w", 35}, {"h", 8}, {"cursor", 1}, {"focused", true},
                     {"items", QVariantList{"Enabled & profile", "Inactive profile", "--------", "[x] literal name"}},
                     {"itemStates", QVariantList{
                         QVariantMap{{"checkable", true}, {"checked", true}},
                         QVariantMap{{"checkable", true}, {"checked", false}, {"dimmed", true}},
                         QVariantMap{}, QVariantMap{{"checkable", true}, {"checked", true}}}}};
    const auto publish = [&] {
        dialog["children"] = QVariantList{list};
        scene["dialogs"] = QVariantList{dialog};
    };
    publish();
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *checked = nullptr;
    QTRY_VERIFY((checked = visualItem(root, "dialogWidget-profilesListCheck-0")));
    auto *unchecked = visualItem(root, "dialogWidget-profilesListCheck-1");
    auto *separator = visualItem(root, "dialogWidget-profilesListCheck-2");
    QVERIFY(unchecked); QVERIFY(separator);
    QVERIFY(checked->property("checked").toBool());
    QVERIFY(!unchecked->property("checked").toBool());
    QVERIFY(!separator->isVisible());
    auto *dimText = visualItem(root, "dialogWidget-profilesListItemText-1");
    QCOMPARE(visualItem(root, "dialogWidget-profilesListItemText-0")->property("text").toString(), QString("Enabled & profile"));
    QCOMPARE(visualItem(root, "dialogWidget-profilesListItemText-3")->property("text").toString(), QString("[x] literal name"));
    QCOMPARE(dimText->opacity(), .75);
    QCOMPARE(unchecked->opacity(), .75);
    QCOMPARE(dimText->parentItem()->opacity(), 1.0);
    QTest::qWait(150);
    QCOMPARE(dimText->parentItem()->property("color").value<QColor>(), fixture.window->property("selectedBg").value<QColor>());
    QVERIFY(!fixture.window->grabWindow().isNull());
    const qreal dpr = fixture.window->devicePixelRatio();
    for (int row : {0, 1, 2, 3}) {
        QStringList leaves{QString("dialogWidget-profilesListItemText-%1").arg(row)};
        if (row != 2) {
            const auto base = QString("dialogWidget-profilesListCheck-%1").arg(row);
            leaves << base + "Indicator" << base + "CheckMark";
        }
        for (const auto &name : leaves) {
            auto *leaf = visualItem(root, name);
            QVERIFY2(leaf, qPrintable(name));
            const auto origin = leaf->mapToItem(root, QPointF{});
            const auto physical = origin * dpr;
            QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .001 && qAbs(physical.y()-qRound64(physical.y())) < .001,
                     qPrintable(QString("%1 physical=(%2,%3)").arg(name).arg(physical.x()).arg(physical.y())));
            QCOMPARE(leaf->mapToItem(root, QPointF(1, 0))-origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToItem(root, QPointF(0, 1))-origin, QPointF(0, 1));
            if (name.endsWith("Indicator")) {
                QVERIFY(qAbs(leaf->width()*dpr-qRound64(leaf->width()*dpr)) < .001);
                QVERIFY(qAbs(leaf->height()*dpr-qRound64(leaf->height()*dpr)) < .001);
                const auto bounds = leaf->mapRectToItem(dimText->parentItem()->parentItem(), leaf->boundingRect());
                QVERIFY(bounds.top() >= 0);
                QVERIFY(bounds.bottom() <= dimText->parentItem()->parentItem()->height());
            }
        }
    }
    const auto toggleCount = [&] {
        int count = 0;
        for (const auto &action : fixture.shell.actions)
            if (action.value("action") == "control.toggle") ++count;
        return count;
    };
    const auto checkboxPoint = itemCenter(unchecked);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, checkboxPoint);
    QTRY_COMPARE(toggleCount(), 1);
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.last().value("index").toInt(), 1);
    QVERIFY(!unchecked->property("checked").toBool()); // Go acknowledges state changes.
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(dimText));
    QCOMPARE(toggleCount(), 0);
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("control.select"));
    fixture.shell.clearActions();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, checkboxPoint);
    QTest::mouseMove(fixture.window, checkboxPoint + QPoint(0, 35), 30);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, checkboxPoint + QPoint(0, 35));
    QCOMPARE(toggleCount(), 0);
    if (qEnvironmentVariableIsSet("F4_LIST_CHECK_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_LIST_CHECK_CAPTURE")));
    auto states = list.value("itemStates").toList();
    states[1] = QVariantMap{{"checkable", true}, {"checked", true}, {"dimmed", false}};
    list["itemStates"] = states;
    publish(); fixture.shell.setScene(scene);
    QTRY_VERIFY(visualItem(root, "dialogWidget-profilesListCheck-1")->property("checked").toBool());
    QCOMPARE(visualItem(root, "dialogWidget-profilesListItemText-1")->opacity(), 1.0);
    for (const auto &flag : {"disabled", "readOnly"}) {
        list[flag] = true;
        publish(); fixture.shell.setScene(scene);
        QCoreApplication::processEvents();
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, checkboxPoint);
        QVERIFY(fixture.shell.actions.isEmpty());
        list.remove(flag);
    }
}

void F4OperationsQueueTests::environmentManagerDialogUsesExpandedRows()
{
    QVariantMap scene = dialogScene();
    QVariantList dialogs = scene.value("dialogs").toList();
    QVariantMap dialog = dialogs.first().toMap();
    dialog.insert("title", "Environment Manager settings");
    dialog.insert("h", 16);
    const auto control = [](const char *id, const char *kind, int x, int row,
                            int width, const char *text) -> QVariantMap {
        return {{"id", id}, {"kind", kind}, {"x", 18 + x}, {"y", 3 + row},
                {"w", width}, {"h", 1}, {"text", text}, {"hotkey", QString(id) == "env-save" ? "s" : ""}};
    };
    dialog.insert("children", QVariantList{
        control("env-label", "text", 2, 2, 60, "Ignored variables (comma-separated):"),
        control("env-edit", "edit", 2, 3, 60, "PATH, TEMP"),
        control("env-check", "checkbox", 2, 5, 60, "Always edit profiles in the f4 editor"),
        control("env-prefix", "text", 2, 7, 60, "Command prefix: envman"),
        control("env-import", "button", 5, 13, 32, "Import from Far Manager 3..."),
        control("env-save", "button", 39, 13, 9, "Save"),
        control("env-cancel", "button", 50, 13, 11, "&Cancel"),
    });
    dialogs[0] = dialog;
    scene.insert("dialogs", dialogs);
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    fixture.shell.setScene(scene);
    QQuickItem *edit = nullptr;
    QTRY_VERIFY((edit = visualItem(fixture.window->contentItem(), "dialogWidget-env-editEdit")));
    auto *label = visualItem(fixture.window->contentItem(), "dialogWidget-env-labelText");
    auto *save = visualItem(fixture.window->contentItem(), "dialogWidget-env-saveButton");
    auto *cancel = visualItem(fixture.window->contentItem(), "dialogWidget-env-cancelButton");
    QVERIFY(label); QVERIFY(save); QVERIFY(cancel);
    const auto accent = fixture.window->property("dialogAccent").value<QColor>().name();
    QTRY_COMPARE(visualItem(fixture.window->contentItem(), "dialogWidget-env-saveButtonText")->property("text").toString(),
                 QString("<font color=\"%1\">S</font>ave").arg(accent));
    QTRY_COMPARE(visualItem(fixture.window->contentItem(), "dialogWidget-env-cancelButtonText")->property("text").toString(),
                 QString("<font color=\"%1\">C</font>ancel").arg(accent));
    QVERIFY(edit->mapToScene(QPointF{}).y()
            >= label->mapToScene(QPointF(0, label->height())).y() + 2);
    QCOMPARE(save->mapToScene(QPointF{}).y(), cancel->mapToScene(QPointF{}).y());
    auto *dialogItem = visualItem(fixture.window->contentItem(), "semanticDialog-appearance-dialog");
    QVERIFY(dialogItem);
    const qreal compactWidth = dialogItem->width();
    const qreal compactHeight = dialogItem->height();
    QVERIFY(compactHeight < 350);
    // Even five blank rows become one 16-DIP section gap.
    auto *prefix = visualItem(fixture.window->contentItem(), "dialogWidget-env-prefixText");
    QVERIFY(prefix);
    const qreal sectionGap = save->mapToScene(QPointF{}).y()
                             - prefix->mapToScene(QPointF(0, prefix->height())).y();
    QVERIFY2(sectionGap >= 16 && sectionGap < 24, qPrintable(QString::number(sectionGap)));
    auto shifted = dialog.value("children").toList();
    for (auto &entry : shifted) {
        auto child = entry.toMap();
        child["x"] = child.value("x").toInt() + 7;
        child["y"] = child.value("y").toInt() + 4;
        entry = child;
    }
    dialog["children"] = shifted;
    dialog["w"] = 100;
    dialog["h"] = 40;
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    fixture.shell.setScene(scene);
    QTRY_COMPARE(dialogItem->width(), compactWidth);
    QTRY_COMPARE(dialogItem->height(), compactHeight);
    QTest::qWait(150);
    const qreal dpr = fixture.window->devicePixelRatio();
    for (const QString &name : {QStringLiteral("env-labelText"), QStringLiteral("env-editEditTextInput"),
                               QStringLiteral("env-checkCheckBoxText"), QStringLiteral("env-prefixText"),
                               QStringLiteral("env-importButtonText"), QStringLiteral("env-saveButtonText"),
                               QStringLiteral("env-cancelButtonText"), QStringLiteral("semanticDialogTitle")}) {
        auto *leaf = visualItem(fixture.window->contentItem(), name == "semanticDialogTitle" ? name : "dialogWidget-" + name);
        QVERIFY2(leaf, qPrintable(name));
        const QPointF origin = leaf->mapToItem(fixture.window->contentItem(), QPointF{});
        QVERIFY2(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < 0.001, qPrintable(name));
        QVERIFY2(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < 0.001, qPrintable(name));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0, 1)) - origin, QPointF(0, 1));
    }
    auto *title = visualItem(fixture.window->contentItem(), "semanticDialogTitle");
    auto *header = visualItem(fixture.window->contentItem(), "dialogMoveHandle");
    QVERIFY(title); QVERIFY(header);
    const qreal titleCenter = title->mapToScene(QPointF(0, title->height() / 2)).y();
    const qreal headerCenter = header->mapToScene(QPointF(0, header->height() / 2)).y();
    QVERIFY2(qAbs(titleCenter - headerCenter) * dpr <= 0.501,
             qPrintable(QString("title center=%1 header center=%2 physical px")
                        .arg(titleCenter * dpr).arg(headerCenter * dpr)));
    // This assertion covers the existing non-macOS rectangular hover chrome.
    QVERIFY(dialogItem->setProperty("macWindowControls", false));
    QCoreApplication::processEvents();
    auto *closeButton = visualItem(fixture.window->contentItem(), "dialogCloseButton");
    QVERIFY(closeButton);
    const qreal closeRight = closeButton->mapToScene(QPointF(closeButton->width(), 0)).x();
    const qreal dialogRight = dialogItem->mapToScene(QPointF(dialogItem->width(), 0)).x();
    QVERIFY2((dialogRight - closeRight) * dpr >= 1,
             qPrintable(QString("close right=%1 dialog right=%2 physical px")
                        .arg(closeRight * dpr).arg(dialogRight * dpr)));
    // Exercise the hover paint deterministically even on the offscreen platform.
    QVERIFY(closeButton->setProperty("backgroundColor", QColor("#c42b1c")));
    auto *closeBackground = visualItem(closeButton, "dialogCloseBackground");
    QVERIFY(closeBackground);
    const qreal highlightRight = closeBackground->mapToScene(QPointF(closeBackground->width(), 0)).x() * dpr;
    const qreal innerRight = qRound(header->mapToScene(QPointF(header->width(), 0)).x() * dpr);
    QVERIFY2(qAbs(highlightRight - innerRight) < 0.001,
             qPrintable(QString("highlight right=%1 border inner edge=%2 physical px").arg(highlightRight).arg(innerRight)));
    QVERIFY(closeBackground->property("topRightRadius").toReal() > 0);
    auto *closeIcon = visualItem(closeButton, "titleBarButtonIcon");
    QVERIFY(closeIcon);
    // The platform style switch schedules a native alignment pass at polish.
    QTRY_VERIFY(qAbs(closeIcon->mapToScene(QPointF{}).x() * dpr
                     - qRound(closeIcon->mapToScene(QPointF{}).x() * dpr)) < 0.001);
    QTRY_VERIFY(qAbs(closeIcon->mapToScene(QPointF{}).y() * dpr
                     - qRound(closeIcon->mapToScene(QPointF{}).y() * dpr)) < 0.001);
    const auto iconOrigin = closeIcon->mapToScene(QPointF{});
    QVERIFY2(qAbs(iconOrigin.x() * dpr - qRound(iconOrigin.x() * dpr)) < 0.001, qPrintable(QString::number(iconOrigin.x() * dpr, 'f', 6)));
    QVERIFY(qAbs(iconOrigin.y() * dpr - qRound(iconOrigin.y() * dpr)) < 0.001);
    QCOMPARE(closeIcon->mapToScene(QPointF(1, 0)) - iconOrigin, QPointF(1, 0));
    QCOMPARE(closeIcon->mapToScene(QPointF(0, 1)) - iconOrigin, QPointF(0, 1));
    save = visualItem(fixture.window->contentItem(), "dialogWidget-env-saveButton");
    QVERIFY(save);
    auto *saveBackground = visualItem(save, "dialogWidget-env-saveButtonBackground");
    QVERIFY(saveBackground);
    const auto normalFill = saveBackground->property("color").value<QColor>();
    QVERIFY(save->setProperty("semanticFocus", true));
    QTRY_COMPARE(saveBackground->property("testBorderColor").value<QColor>(),
                 fixture.window->property("dialogAccent").value<QColor>());
    QTest::qWait(120);
    QCOMPARE(saveBackground->property("color").value<QColor>(), normalFill);
    const QImage rendered = fixture.window->grabWindow();
    QVERIFY(!rendered.isNull());
    if (!qEnvironmentVariable("F4_DIALOG_CAPTURE").isEmpty())
        QVERIFY(rendered.save(qEnvironmentVariable("F4_DIALOG_CAPTURE")));
}

void F4OperationsQueueTests::commandPaletteInputHasCompactGapAndLucidePrompt()
{
    const auto dialog = QJsonDocument::fromJson(R"({
        "id":"palette","kind":"dialog","title":"Command Palette",
        "x":0,"y":0,"w":90,"h":38,"showClose":true,"children":[
          {"id":"prompt","kind":"text","text":">","x":2,"y":2,"w":1,"h":1},
          {"id":"query","kind":"edit","text":"","x":4,"y":2,"w":84,"h":1},
          {"id":"commands","kind":"table","x":2,"y":4,"w":86,"h":30,
           "showHeader":true,"columns":[{"title":"Command","width":60}],
           "items":["Help"],"selected":0}
        ]})").object().toVariantMap();
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    auto scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    auto *root = fixture.window->contentItem();
    QQuickItem *edit = nullptr;
    QQuickItem *table = nullptr;
    QQuickItem *icon = nullptr;
    QTRY_VERIFY((edit = visualItem(root, "dialogWidget-queryRoot")));
    QTRY_VERIFY((table = visualItem(root, "dialogWidget-commandsRoot")));
    QTRY_VERIFY((icon = visualItem(root, "dialogWidget-promptTextIcon")));
    const qreal gap = table->mapToScene(QPointF()).y()
        - edit->mapToScene(QPointF(0, edit->height())).y();
    QVERIFY2(gap >= 0 && gap <= 28, qPrintable(QString("input/table gap=%1").arg(gap)));
    QVERIFY(icon->isVisible());
    QVERIFY(icon->property("source").toUrl().toString().contains("chevron-right"));
    auto *prompt = visualItem(root, "dialogWidget-promptText");
    QVERIFY(prompt);
    QCOMPARE(prompt->property("text").toString(), QString());
    QVERIFY(edit->mapToScene(QPointF()).x()
            >= icon->mapToScene(QPointF(icon->width(), 0)).x() + 8);
}

void F4OperationsQueueTests::semanticDialogRowsExpandForNativeControls()
{
    QVariantMap scene = dialogControlsScene(false);
    QVariantList dialogs = scene.value(QStringLiteral("dialogs")).toList();
    QVariantMap dialog = dialogs.constFirst().toMap();
    QVariantList children = dialog.value(QStringLiteral("children")).toList();
    for (qsizetype index = 0; index < children.size(); ++index) {
        QVariantMap child = children.at(index).toMap();
        const QString id = child.value(QStringLiteral("id")).toString();
        if (id == QStringLiteral("appearance-edit")
            || id == QStringLiteral("appearance-combo")
            || id == QStringLiteral("appearance-apply")) {
            child.insert(QStringLiteral("h"), 1);
            children[index] = child;
        }
    }
    for (qsizetype i = 0; i < children.size(); ++i) {
        QVariantMap child = children[i].toMap();
        const QString id = child.value(QStringLiteral("id")).toString();
        const int row = id == "appearance-label" ? 7 : id == "appearance-edit" ? 8
                      : id == "appearance-combo" ? 9 : id == "appearance-apply" ? 10 : -1;
        if (row >= 0) child.insert(QStringLiteral("y"), dialog.value(QStringLiteral("y")).toInt() + 1 + row);
        children[i] = child;
    }
    dialog.insert(QStringLiteral("children"), children);
    dialogs[0] = dialog;
    scene.insert(QStringLiteral("dialogs"), dialogs);

    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.window->isActive(), 3000);
    fixture.shell.setScene(scene);

    QQuickItem *const visualRoot = fixture.window->contentItem();
    QVERIFY(visualRoot);
    QQuickItem *editRoot = nullptr;
    QQuickItem *comboRoot = nullptr;
    QQuickItem *buttonRoot = nullptr;
    QQuickItem *labelRoot = nullptr;
    QQuickItem *edit = nullptr;
    QQuickItem *combo = nullptr;
    QQuickItem *button = nullptr;
    QQuickItem *editCursorArea = nullptr;
    QQuickItem *comboCursorArea = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (editRoot = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-editRoot"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (comboRoot = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-comboRoot"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (buttonRoot = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-applyRoot"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (labelRoot = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-labelRoot"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (edit = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-editEdit"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (combo = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-comboComboBox"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (button = visualItem(visualRoot,
             QStringLiteral("dialogWidget-appearance-applyButton"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (editCursorArea = visualItem(visualRoot, QStringLiteral(
             "dialogWidget-appearance-editEditTextCursorArea"))), 3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (comboCursorArea = visualItem(visualRoot, QStringLiteral(
             "dialogWidget-appearance-comboComboBoxEditableCursorArea"))),
        3000);
    QVERIFY(editCursorArea->property("hoverEnabled").toBool());
    QVERIFY(comboCursorArea->property("hoverEnabled").toBool());
    QCOMPARE(editCursorArea->property("cursorShape").toInt(),
             int(Qt::IBeamCursor));
    QCOMPARE(comboCursorArea->property("cursorShape").toInt(),
             int(Qt::IBeamCursor));

    const qreal cellHeight = fixture.window->property("ch").toReal();
    const qreal semanticHeight = qMax<qreal>(22.0, qRound(cellHeight));
    const qreal dialogControlHeight =
        fixture.window->property("dialogControlHeight").toReal();
    const qreal dpr = fixture.window->devicePixelRatio();
    struct ControlGeometry {
        QQuickItem *root;
        QQuickItem *control;
        int relativeRow;
        const char *description;
    };
    const QList<ControlGeometry> controls{
        {editRoot, edit, 8, "edit"},
        {comboRoot, combo, 9, "combo box"},
        {buttonRoot, button, 10, "button"},
    };
    qreal previousBottom = labelRoot->y() + labelRoot->height();
    for (const ControlGeometry &entry : controls) {
        const qreal semanticTop = qRound(entry.relativeRow * cellHeight);
        const QString geometry = QStringLiteral(
            "%1 visual y=%2 height=%3; semantic y=%4 height=%5")
                                     .arg(QString::fromLatin1(entry.description))
                                     .arg(entry.root->y(), 0, 'f', 6)
                                     .arg(entry.root->height(), 0, 'f', 6)
                                     .arg(semanticTop, 0, 'f', 6)
                                     .arg(semanticHeight, 0, 'f', 6);
        QVERIFY2(entry.root->height() > semanticHeight,
                 qPrintable(geometry));
        QCOMPARE(entry.root->height(), dialogControlHeight);
        QCOMPARE(entry.control->height(), entry.root->height());
        QVERIFY2(entry.root->y() >= previousBottom + 2, qPrintable(geometry));
        previousBottom = entry.root->y() + entry.root->height();
    }

    QImage rendered;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(rendered = fixture.window->grabWindow()).isNull(), 3000);
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required for the physical-pixel gate");


    for (const ControlGeometry &entry : controls) {
        const QPointF origin = entry.root->mapToItem(visualRoot, QPointF{});
        const QPointF physical = origin * dpr;
        const qreal physicalWidth = entry.root->width() * dpr;
        const qreal physicalHeight = entry.root->height() * dpr;
        const QString details = QStringLiteral(
            "%1 root physical geometry is (%2, %3) %4x%5")
                                    .arg(QString::fromLatin1(entry.description))
                                    .arg(physical.x(), 0, 'f', 6)
                                    .arg(physical.y(), 0, 'f', 6)
                                    .arg(physicalWidth, 0, 'f', 6)
                                    .arg(physicalHeight, 0, 'f', 6);
        QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physical.y() - qRound(physical.y())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physicalWidth - qRound(physicalWidth)) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physicalHeight - qRound(physicalHeight)) < 0.001,
                 qPrintable(details));
    }

    QList<QQuickItem *> leaves;
    for (const QString &name : {
             QStringLiteral("dialogWidget-appearance-labelText"),
             QStringLiteral("dialogWidget-appearance-editEditTextInput"),
             QStringLiteral("dialogWidget-appearance-comboComboBoxText"),
             QStringLiteral("dialogWidget-appearance-comboComboBoxIndicator"),
             QStringLiteral("dialogWidget-appearance-applyButtonText")}) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY_WITH_TIMEOUT((leaf = visualItem(visualRoot, name)), 3000);
        leaves.append(leaf);
    }
    for (QQuickItem *leaf : std::as_const(leaves)) {
        const QPointF origin = leaf->mapToItem(visualRoot, QPointF{});
        const QPointF physical = origin * dpr;
        const QString details = QStringLiteral(
            "%1 physical origin is (%2, %3)")
                                    .arg(leaf->objectName())
                                    .arg(physical.x(), 0, 'f', 6)
                                    .arg(physical.y(), 0, 'f', 6);
        QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physical.y() - qRound(physical.y())) < 0.001,
                 qPrintable(details));

        const QPointF xAxis = leaf->mapToItem(visualRoot, QPointF(1, 0)) - origin;
        const QPointF yAxis = leaf->mapToItem(visualRoot, QPointF(0, 1)) - origin;
        QVERIFY2(qAbs(xAxis.x() - 1.0) < 0.001
                     && qAbs(xAxis.y()) < 0.001
                     && qAbs(yAxis.x()) < 0.001
                     && qAbs(yAxis.y() - 1.0) < 0.001,
                 qPrintable(QStringLiteral("%1 has a non-translation transform")
                                .arg(leaf->objectName())));
    }
}

void F4OperationsQueueTests::errorMessagesWrapToNativeWidth()
{
    auto scene = dialogScene();
    auto dialogs = scene.value("dialogs").toList();
    auto dialog = dialogs[0].toMap();
    dialog["title"] = "Deletion Errors";
    const QString message = QStringLiteral("Skipped 'photo & notes.png': unlinkat D:/Code/f4-qt-drag-drop/.diagnostics/visual-drag/test/photo & notes.png: The process cannot access the file because it is being used by another process.");
    dialog["children"] = QVariantList{QVariantMap{
        {"id", "error-list"}, {"kind", "listBox"}, {"x", 20}, {"y", 7},
        {"w", 50}, {"h", 12}, {"wrapText", true}, {"readOnly", true},
        {"items", QVariantList{message, QStringLiteral("Second complete error message.")}}
    }};
    dialogs[0] = dialog;
    scene["dialogs"] = dialogs;
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *root = fixture.window->contentItem();
    QQuickItem *first = nullptr;
    QTRY_VERIFY((first = visualItem(root, QStringLiteral("dialogWidget-error-listListItemText-0"))));
    QQuickItem *second = nullptr;
    QTRY_VERIFY((second = visualItem(root, QStringLiteral("dialogWidget-error-listListItemText-1"))));
    QCOMPARE(first->property("text").toString(), message);
    QTRY_VERIFY(first->property("lineCount").toInt() > 1);
    const int wideLines = first->property("lineCount").toInt();
    auto *control = visualItem(root, QStringLiteral("dialogWidget-error-listRoot"));
    QVERIFY(control);
    QVERIFY(control->setProperty("maximumWidth", 180.0));
    QTRY_VERIFY(first->property("lineCount").toInt() > wideLines);
    QTest::qWait(100);
    for (auto *leaf : {first, second}) {
        const QPointF origin = leaf->mapToItem(root, QPointF());
        const qreal dpr = fixture.window->devicePixelRatio();
        QVERIFY(qAbs(origin.x()*dpr - qRound(origin.x()*dpr)) < 0.001);
        QVERIFY(qAbs(origin.y()*dpr - qRound(origin.y()*dpr)) < 0.001);
        QCOMPARE(leaf->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1));
        QVERIFY(leaf->height() >= leaf->property("contentHeight").toReal());
    }
    const QImage capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (!qEnvironmentVariable("F4_DIALOG_CAPTURE").isEmpty())
        QVERIFY(capture.save(qEnvironmentVariable("F4_DIALOG_CAPTURE")));
}

void F4OperationsQueueTests::semanticDialogControlsUseWindowFontAndStayPixelAligned()
{
    const QFont previousFont = QGuiApplication::font();
    QFont dialogFont = previousFont;
    dialogFont.setPixelSize(17);

    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    // GUI preferences own the interface font; the application font is untouched.
    auto *typography = fixture.window->property("typography").value<QObject *>();
    QVERIFY(typography);
    QVERIFY(typography->setProperty("uiFamily", dialogFont.family()));
    QVERIFY(typography->setProperty("uiSize", dialogFont.pixelSize()));
    QTRY_COMPARE(fixture.window->property("font").value<QFont>(), dialogFont);
    QCOMPARE(QGuiApplication::font(), previousFont);

    const QColor firstText(QStringLiteral("#d4e5f6"));
    const QColor firstMuted(QStringLiteral("#8091a2"));
    const QColor firstControl(QStringLiteral("#172839"));
    const QColor firstPressed(QStringLiteral("#294a5b"));
    const QColor firstBorder(QStringLiteral("#3b5c6d"));
    const QColor firstAccent(QStringLiteral("#4d7e8f"));
    for (const auto &entry : {
             qMakePair("textColor", firstText),
             qMakePair("mutedText", firstMuted),
             qMakePair("controlBg", firstControl),
             qMakePair("controlPressedBg", firstPressed),
             qMakePair("controlBorder", firstBorder),
             qMakePair("dialogAccent", firstAccent),
         }) {
        QVERIFY(fixture.window->setProperty(entry.first, entry.second));
    }

    fixture.shell.setScene(dialogControlsScene(false));

    QQuickItem *button = nullptr;
    QQuickItem *buttonContent = nullptr;
    QQuickItem *buttonIcon = nullptr;
    QQuickItem *buttonText = nullptr;
    QQuickItem *checkBox = nullptr;
    QQuickItem *checkFocusFrame = nullptr;
    QQuickItem *checkMark = nullptr;
    QQuickItem *checkText = nullptr;
    QQuickItem *radioButton = nullptr;
    QQuickItem *radioFocusFrame = nullptr;
    QQuickItem *radioIndicator = nullptr;
    QQuickItem *radioMark = nullptr;
    QQuickItem *radioText = nullptr;
    QQuickItem *edit = nullptr;
    QQuickItem *editCursor = nullptr;
    QQuickItem *editText = nullptr;
    QQuickItem *combo = nullptr;
    QQuickItem *comboText = nullptr;
    QQuickItem *comboIndicator = nullptr;
    QQuickItem *plainText = nullptr;
    QQuickItem *listText = nullptr;
    QQuickItem *listBox = nullptr;
    QQuickItem *listFocusFrame = nullptr;
    QQuickItem *groupRoot = nullptr;
    QQuickItem *groupTitle = nullptr;
    QQuickItem *nestedRoot = nullptr;
    QQuickItem *nestedText = nullptr;
    QQuickItem *borderlessGroupTitle = nullptr;
    QQuickItem *const rootItem = fixture.window->contentItem();
    QVERIFY(rootItem);
    QTRY_VERIFY_WITH_TIMEOUT(
        (button = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-applyButton"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (buttonContent = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-applyButtonContent"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (buttonIcon = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-applyButtonIcon"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (buttonText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-applyButtonText"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkBox = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-checkboxCheckBox"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkFocusFrame = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-checkboxCheckBoxFocusFrame"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-checkboxCheckBoxText"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (checkMark = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-checkboxCheckBoxCheckMark"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioButton = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-navigationRadio-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioFocusFrame = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-navigationRadio-0FocusFrame"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-navigationRadio-0Text"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioIndicator = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-navigationRadio-0Indicator"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (radioMark = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-navigationRadio-0SelectionMark"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (edit = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-editEdit"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (editText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-editEditTextInput"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (combo = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-comboComboBox"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (comboText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-comboComboBoxText"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (comboIndicator = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-comboComboBoxIndicator"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (plainText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-labelText"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (listText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-listListItemText-0"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (listBox = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-listListBox"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (listFocusFrame = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-listListFocusFrame"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (groupRoot = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-groupRoot"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (groupTitle = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-groupGroupTitle"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (nestedRoot = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-group-labelRoot"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (nestedText = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-group-labelText"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (borderlessGroupTitle = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-borderless-groupGroupTitle"))),
        3000);

    QList<QQuickItem *> textLeaves{
        buttonText, checkText, radioText, editText, comboText, plainText,
        listText, nestedText,
    };
    const QFont initialWindowFont = fixture.window->property("font").value<QFont>();
    QCOMPARE(initialWindowFont, dialogFont);
    for (QQuickItem *leaf : std::as_const(textLeaves))
        QCOMPARE(leaf->property("font").value<QFont>(), initialWindowFont);
    for (QQuickItem *leaf : {buttonText, checkText, radioText, editText,
                             comboText, plainText, listText, nestedText}) {
        QCOMPARE(leaf->property("color").value<QColor>(), firstText);
    }
    for (auto *heading : {groupTitle, borderlessGroupTitle}) {
        const auto font = heading->property("font").value<QFont>();
        QCOMPARE(font.family(), initialWindowFont.family());
        QCOMPARE(font.pixelSize(), initialWindowFont.pixelSize());
        QVERIFY(font.bold());
    }
    QCOMPARE(groupTitle->property("color").value<QColor>(), firstText);
    QCOMPARE(borderlessGroupTitle->property("color").value<QColor>(),
             firstText);

    QQuickItem *const buttonBackground = visualItem(rootItem,
        QStringLiteral("dialogWidget-appearance-applyButtonBackground"));
    QQuickItem *const checkIndicator = visualItem(rootItem,
        QStringLiteral("dialogWidget-appearance-checkboxCheckBoxIndicator"));
    QQuickItem *const editBackground = visualItem(rootItem,
        QStringLiteral("dialogWidget-appearance-editEditBackground"));
    QQuickItem *const comboBackground = visualItem(rootItem,
        QStringLiteral("dialogWidget-appearance-comboComboBoxBackground"));
    QQuickItem *const groupBorder = visualItem(rootItem,
        QStringLiteral("dialogWidget-appearance-groupGroupBorder"));
    QQuickItem *const borderlessGroupBorder = visualItem(rootItem,
        QStringLiteral("dialogWidget-appearance-borderless-groupGroupBorder"));
    QVERIFY(buttonBackground);
    QVERIFY(checkIndicator);
    QVERIFY(checkFocusFrame);
    QVERIFY(radioFocusFrame);
    QVERIFY(editBackground);
    QVERIFY(comboBackground);
    QVERIFY(listBox);
    QVERIFY(listFocusFrame);
    QVERIFY(groupBorder);
    QVERIFY(borderlessGroupBorder);
    QCOMPARE(buttonBackground->property("color").value<QColor>(), firstControl);
    QCOMPARE(checkIndicator->property("color").value<QColor>(), firstControl);
    QCOMPARE(buttonBackground->property("testBorderColor").value<QColor>(),
             firstBorder);
    QCOMPARE(comboBackground->property("testBorderColor").value<QColor>(),
             firstBorder);
    QCOMPARE(checkFocusFrame->property("testBorderColor").value<QColor>(),
             firstBorder);
    QCOMPARE(checkFocusFrame->property("testBorderWidth").toReal(), 0.0);
    QCOMPARE(radioFocusFrame->property("testBorderColor").value<QColor>(),
             firstBorder);
    QCOMPARE(radioFocusFrame->property("testBorderWidth").toReal(), 0.0);
    QCOMPARE(listFocusFrame->property("testBorderColor").value<QColor>(),
             firstBorder);
    QCOMPARE(listFocusFrame->property("testBorderWidth").toReal(), 0.0);
    QCOMPARE(groupBorder->property("testBorderColor").value<QColor>(),
             firstBorder);
    QCOMPARE(groupBorder->property("testBorderWidth").toReal(), 0.0);
    const auto groupFill = groupBorder->property("color").value<QColor>();
    const auto dialogFill = fixture.window->property("dialogBg").value<QColor>();
    QVERIFY(groupFill.redF() > dialogFill.redF());
    QCOMPARE(groupFill.alpha(), 255);
    QVERIFY(groupBorder->property("radius").toReal() >= 9);
    QCOMPARE(borderlessGroupBorder->property("testBorderWidth").toReal(), 0.0);

    const QPointF nestedInGroup = nestedRoot->mapToItem(groupRoot, QPointF{});
    const qreal expectedNestedX = qRound(2.0
        * fixture.window->property("cw").toReal());
    QCOMPARE(nestedInGroup.x(), expectedNestedX);
    QVERIFY(qAbs(nestedInGroup.y() - groupBorder->y() - 12) < 1);
    QVERIFY(groupTitle->mapToItem(groupRoot, QPointF{}).y() + groupTitle->height() <= groupBorder->y() - 3);
    QVERIFY(nestedInGroup.x() >= 0.0);
    QVERIFY(nestedInGroup.y() >= 0.0);
    QVERIFY(nestedInGroup.x() + nestedRoot->width() <= groupRoot->width());
    QVERIFY(nestedInGroup.y() + nestedRoot->height() <= groupRoot->height());

    QImage normalFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(normalFrame = fixture.window->grabWindow()).isNull(), 3000);
    if (!qEnvironmentVariable("F4_DIALOG_CAPTURE").isEmpty())
        QVERIFY(normalFrame.save(qEnvironmentVariable("F4_DIALOG_CAPTURE")));

    QFont updatedFont = initialWindowFont;
    updatedFont.setPixelSize(19);
    updatedFont.setItalic(!updatedFont.italic());
    QVERIFY(fixture.window->setProperty("font", updatedFont));
    for (QQuickItem *leaf : std::as_const(textLeaves)) {
        QTRY_COMPARE_WITH_TIMEOUT(leaf->property("font").value<QFont>(),
                                  updatedFont, 3000);
    }

    const QColor secondText(QStringLiteral("#f1dac2"));
    const QColor secondMuted(QStringLiteral("#a98b7c"));
    const QColor secondControl(QStringLiteral("#253647"));
    const QColor secondPressed(QStringLiteral("#476879"));
    const QColor secondBorder(QStringLiteral("#698a9b"));
    const QColor secondAccent(QStringLiteral("#8bacbd"));
    for (const auto &entry : {
             qMakePair("textColor", secondText),
             qMakePair("mutedText", secondMuted),
             qMakePair("controlBg", secondControl),
             qMakePair("controlPressedBg", secondPressed),
             qMakePair("controlBorder", secondBorder),
             qMakePair("dialogAccent", secondAccent),
         }) {
        QVERIFY(fixture.window->setProperty(entry.first, entry.second));
    }
    QVERIFY(button->setProperty("semanticFocus", true));
    QVERIFY(checkBox->setProperty("semanticFocus", true));
    QVERIFY(checkBox->setProperty("checked", true));
    QVERIFY(radioButton->setProperty("semanticFocus", true));
    QVERIFY(radioButton->setProperty("checked", true));
    QVERIFY(edit->setProperty("semanticFocus", true));
    QVERIFY(edit->setProperty("remoteCursorVisible", true));
    QVERIFY(combo->setProperty("semanticFocus", true));
    QVERIFY(listBox->setProperty("semanticFocus", true));
    QCoreApplication::processEvents();
    QTRY_VERIFY_WITH_TIMEOUT(
        (editCursor = visualItem(rootItem,
             QStringLiteral("dialogWidget-appearance-editEditCursor"))),
        3000);

    QTRY_COMPARE_WITH_TIMEOUT(buttonBackground->property("color").value<QColor>(),
                              secondControl, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        buttonBackground->property("testBorderColor").value<QColor>(),
        secondAccent, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(checkIndicator->property("color").value<QColor>(),
                              secondAccent, 3000);
    const QUrl checkIconSource = checkMark->property("source").toUrl();
    QVERIFY(checkIconSource.isValid());
    QCOMPARE(checkIconSource.fileName(), QStringLiteral("check.svg"));
    QCOMPARE(QUrlQuery(checkIconSource).queryItemValue(QStringLiteral("size")),
             QStringLiteral("12"));
    QCOMPARE(QUrlQuery(checkIconSource).queryItemValue(QStringLiteral("dpr")),
             QString::number(fixture.window->devicePixelRatio()));
    QCOMPARE(checkMark->property("opticalVerticalOffset").toReal()
                 * fixture.window->devicePixelRatio(),
             1.0);
    QCOMPARE(checkMark->property("sourceSize").toSize(), QSize(12, 12));
    QTRY_COMPARE_WITH_TIMEOUT(
        editBackground->property("testBorderColor").value<QColor>(),
        secondAccent, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        comboBackground->property("testBorderColor").value<QColor>(),
        secondAccent, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        checkFocusFrame->property("testBorderColor").value<QColor>(),
        secondAccent, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        checkFocusFrame->property("testBorderWidth").toReal(),
        fixture.window->property("separatorWidth").toReal(), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        radioFocusFrame->property("testBorderColor").value<QColor>(),
        secondAccent, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        radioFocusFrame->property("testBorderWidth").toReal(),
        fixture.window->property("separatorWidth").toReal(), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        listFocusFrame->property("testBorderColor").value<QColor>(),
        secondAccent, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        listFocusFrame->property("testBorderWidth").toReal(),
        fixture.window->property("separatorWidth").toReal(), 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        groupBorder->property("testBorderColor").value<QColor>(),
        secondBorder, 3000);
    for (QQuickItem *leaf : {buttonText, checkText, radioText, editText,
                             comboText, plainText, listText, nestedText}) {
        QTRY_COMPARE_WITH_TIMEOUT(leaf->property("color").value<QColor>(),
                                  secondText, 3000);
    }
    QTRY_COMPARE_WITH_TIMEOUT(groupTitle->property("color").value<QColor>(),
                              secondText, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        borderlessGroupTitle->property("color").value<QColor>(),
        secondText, 3000);

    QObject *const popup = combo->property("popup").value<QObject *>();
    QVERIFY(popup);
    QVERIFY(QMetaObject::invokeMethod(popup, "open"));
    QTRY_VERIFY_WITH_TIMEOUT(popup->property("visible").toBool(), 3000);
    QQuickItem *popupText = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (popupText = visualItem(rootItem, QStringLiteral(
             "dialogWidget-appearance-comboComboBoxPopupItemText-1"))),
        3000);
    QCOMPARE(popupText->property("font").value<QFont>(), updatedFont);
    QCOMPARE(popupText->property("color").value<QColor>(), secondText);
    textLeaves.append(popupText);

    const QPointF buttonContentCenter = buttonContent->mapToItem(
        button, QPointF(buttonContent->width() / 2.0,
                        buttonContent->height() / 2.0));
    const QPointF buttonTextCenter = buttonText->mapToItem(
        button, QPointF(buttonText->width() / 2.0, buttonText->height() / 2.0));
    const QPointF buttonCenter(button->width() / 2.0, button->height() / 2.0);
    const qreal dpr = fixture.window->devicePixelRatio();
    QVERIFY(!buttonIcon->isVisible());
    QCOMPARE(buttonIcon->width(), 0.0);
    QCOMPARE(buttonIcon->height(), 0.0);
    QVERIFY2(qAbs((buttonContentCenter.x() - buttonCenter.x()) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "button content is horizontally off-center by %1 physical px "
                 "(button %2x%3, content %4x%5 at %6,%7)")
                            .arg((buttonContentCenter.x() - buttonCenter.x()) * dpr,
                                 0, 'f', 6)
                            .arg(button->width()).arg(button->height())
                            .arg(buttonContent->width()).arg(buttonContent->height())
                            .arg(buttonContent->x()).arg(buttonContent->y())));
    QVERIFY2(qAbs((buttonContentCenter.y() - buttonCenter.y()) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "button content is vertically off-center by %1 physical px")
                            .arg((buttonContentCenter.y() - buttonCenter.y()) * dpr,
                                 0, 'f', 6)));
    QVERIFY2(qAbs((buttonTextCenter.x() - buttonCenter.x()) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "button text is horizontally off-center by %1 physical px "
                 "(content %2x%3; text %4x%5 at %6,%7; icon %8x%9 at %10,%11, "
                 "visible=%12, source=%13)")
                            .arg((buttonTextCenter.x() - buttonCenter.x()) * dpr,
                                 0, 'f', 6)
                            .arg(buttonContent->width()).arg(buttonContent->height())
                            .arg(buttonText->width()).arg(buttonText->height())
                            .arg(buttonText->x()).arg(buttonText->y())
                            .arg(buttonIcon->width()).arg(buttonIcon->height())
                            .arg(buttonIcon->x()).arg(buttonIcon->y())
                            .arg(buttonIcon->isVisible())
                            .arg(buttonIcon->property("source").toString())));
    QVERIFY2(qAbs((buttonTextCenter.y() - buttonCenter.y()) * dpr) <= 0.51,
             qPrintable(QStringLiteral(
                 "button text is vertically off-center by %1 physical px")
                            .arg((buttonTextCenter.y() - buttonCenter.y()) * dpr,
                                 0, 'f', 6)));

    QImage focusedFrame;
    QTRY_VERIFY_WITH_TIMEOUT(
        !(focusedFrame = fixture.window->grabWindow()).isNull(), 3000);
    QVERIFY(focusedFrame != normalFrame);
    QCOMPARE(fixture.window->property("font").value<QFont>(), updatedFont);
    QCOMPARE(QGuiApplication::font(), previousFont);

    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale invocation required for the physical-pixel gate");

    QList<QQuickItem *> visualLeaves = textLeaves;
    visualLeaves.append(checkIndicator);
    visualLeaves.append(checkMark);
    visualLeaves.append(radioIndicator);
    visualLeaves.append(radioMark);
    visualLeaves.append(editCursor);
    visualLeaves.append(comboIndicator);
    visualLeaves.append(checkFocusFrame);
    visualLeaves.append(radioFocusFrame);
    visualLeaves.append(listFocusFrame);
    for (QQuickItem *leaf : std::as_const(visualLeaves)) {
        const QPointF origin = leaf->mapToItem(rootItem, QPointF{});
        const QPointF physical = origin * dpr;
        const QString details = QStringLiteral(
            "%1 physical origin is (%2, %3)")
                                    .arg(leaf->objectName())
                                    .arg(physical.x(), 0, 'f', 6)
                                    .arg(physical.y(), 0, 'f', 6);
        QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.001,
                 qPrintable(details));
        QVERIFY2(qAbs(physical.y() - qRound(physical.y())) < 0.001,
                 qPrintable(details));

        const QPointF xAxis = leaf->mapToItem(rootItem, QPointF(1, 0)) - origin;
        const QPointF yAxis = leaf->mapToItem(rootItem, QPointF(0, 1)) - origin;
        QVERIFY2(qAbs(xAxis.x() - 1.0) < 0.001
                     && qAbs(xAxis.y()) < 0.001
                     && qAbs(yAxis.x()) < 0.001
                     && qAbs(yAxis.y() - 1.0) < 0.001,
                 qPrintable(QStringLiteral("%1 has a non-translation transform")
                                .arg(leaf->objectName())));
    }
}

void F4OperationsQueueTests::dialogTextCursorBlinkSettlesAndFocusStopsIt()
{
    QueueFixture fixture(dialogControlsScene(true));
    QVERIFY(fixture.window);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.window->isActive(), 3000);
    QCOMPARE(QGuiApplication::styleHints()->cursorFlashTime(), 0);

    auto *edit = visualItem(
        fixture.window->contentItem(),
        QStringLiteral("dialogWidget-appearance-editEdit"));
    QQuickItem *cursor = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (cursor = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("dialogWidget-appearance-editEditCursor"))),
        3000);
    QVERIFY(edit);
    QTRY_VERIFY_WITH_TIMEOUT(cursor->isVisible(), 3000);
    QVERIFY(cursor->setProperty("blinkInterval", 20));
    QVERIFY(QMetaObject::invokeMethod(cursor, "restartBlink"));
    QTRY_VERIFY_WITH_TIMEOUT(
        cursor->property("blinkTimerRunning").toBool(), 500);
    QTRY_VERIFY_WITH_TIMEOUT(
        !cursor->property("blinkTimerRunning").toBool(), 500);
    QVERIFY(cursor->property("blinkOn").toBool());

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
             "dialog surface never reached frame quiescence");
    settledFrames.clear();
    QTest::qWait(700);
    QCOMPARE(settledFrames.size(), 0);

    auto *grid = fixture.window->findChild<TestGrid *>();
    QVERIFY(grid);
    emit grid->keyboardActivity();
    QTRY_VERIFY_WITH_TIMEOUT(
        cursor->property("blinkTimerRunning").toBool(), 500);

    QVERIFY(edit->setProperty("semanticFocus", false));
    QTRY_VERIFY_WITH_TIMEOUT(
        !cursor->property("blinkTimerRunning").toBool(), 500);
    QVERIFY(cursor->property("blinkOn").toBool());
    QTRY_COMPARE(cursor->opacity(), 0.0);
    // Native pointer focus may remain in the previous read-only input while
    // the semantic dialog has already moved to another control with Tab.
    auto *nativeInput = visualItem(fixture.window->contentItem(),
        QStringLiteral("dialogWidget-appearance-editEditTextInput"));
    QVERIFY(nativeInput);
    nativeInput->forceActiveFocus();
    QTRY_VERIFY(nativeInput->hasActiveFocus());
    QVERIFY(!edit->property("editingFocus").toBool());
    QTRY_COMPARE(cursor->opacity(), 0.0);
    QVERIFY(!cursor->property("blinkTimerRunning").toBool());
}

void F4OperationsQueueTests::semanticDialogEditShowsRemoteAndNativeSelection()
{
    QVariantMap scene = dialogControlsScene(true);
    QVariantList dialogs = scene.value(QStringLiteral("dialogs")).toList();
    QVariantMap dialog = dialogs.constFirst().toMap();
    QVariantList children = dialog.value(QStringLiteral("children")).toList();
    for (qsizetype index = 0; index < children.size(); ++index) {
        QVariantMap child = children.at(index).toMap();
        if (child.value(QStringLiteral("id")).toString()
            == QStringLiteral("appearance-edit")) {
            child.insert(QStringLiteral("selectionStart"), 1);
            child.insert(QStringLiteral("selectionEnd"), 4);
            // Keep the test control inside the dialog's native geometry. The
            // fixture's frame starts at column 18, while the shared scene
            // places most sample controls at column 2 for the grid renderer.
            child.insert(QStringLiteral("x"), 20);
            children[index] = child;
            break;
        }
    }
    dialog.insert(QStringLiteral("children"), children);
    dialogs[0] = dialog;
    scene.insert(QStringLiteral("dialogs"), dialogs);

    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *const root = fixture.window->contentItem();
    QVERIFY(root);
    QQuickItem *editInput = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (editInput = visualItem(root, QStringLiteral(
             "dialogWidget-appearance-editEditTextInput"))),
        3000);

    // A Go-owned/read-only field must still expose Qt's native selection
    // surface.  The semantic focus click is delivered by TextInput itself;
    // no left-button overlay is allowed to swallow the drag.
    QVERIFY(editInput->property("readOnly").toBool());
    QVERIFY(editInput->property("selectByMouse").toBool());
    QCOMPARE(editInput->property("selectedText").toString(),
             QStringLiteral(":\\W"));
    QCOMPARE(editInput->property("selectionStart").toInt(), 1);
    QCOMPARE(editInput->property("selectionEnd").toInt(), 4);

    // Replace the remote selection with a real pointer drag.  The selection
    // must be painted by TextInput immediately, without waiting for a Go
    // round trip.
    const QPoint startPoint = fixture.window->mapFromGlobal(
        editInput->mapToGlobal(QPointF(2, editInput->height() / 2.0))).toPoint();
    const QPoint endPoint = fixture.window->mapFromGlobal(
        editInput->mapToGlobal(QPointF(editInput->width() - 2,
                                       editInput->height() / 2.0))).toPoint();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      startPoint);
    QTest::mouseMove(fixture.window, endPoint, 100);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier,
                        endPoint);
    QVERIFY2(editInput->property("selectedText").toString().size() > 0,
             "native TextInput drag did not produce a visible selection");
    QTRY_VERIFY_WITH_TIMEOUT(!fixture.shell.actions.isEmpty()
        && fixture.shell.actions.constLast().value("action") == "control.select", 1000);
    const QVariantMap selectionAction = fixture.shell.actions.constLast();
    QCOMPARE(selectionAction.value("target").toString(), QString("appearance-edit"));
    const QString selectedValue = editInput->property("text").toString();
    QCOMPARE(selectionAction.value("cursor").toInt(), selectedValue.left(
        editInput->property("cursorPosition").toInt()).toUcs4().size());

    auto *keySink = qobject_cast<TestGrid *>(fixture.window->property("focusTarget").value<QQuickItem *>());
    QVERIFY(keySink);
    for (const auto &[key, modifiers] : QList<QPair<Qt::Key, Qt::KeyboardModifiers>>{
             {Qt::Key_Delete, Qt::ControlModifier},
             {Qt::Key_Backspace, Qt::ControlModifier},
             {Qt::Key_Left, Qt::ControlModifier | Qt::ShiftModifier},
             {Qt::Key_Backspace, Qt::NoModifier}}) {
        editInput->forceActiveFocus();
        const int firstKey = keySink->forwardedKeys.size();
        QTest::keyClick(fixture.window, key, modifiers);
        QVERIFY(keySink->forwardedKeys.size() >= firstKey + 2);
        bool found = false;
        for (int i = firstKey; i < keySink->forwardedKeys.size(); ++i) {
            const auto &message = keySink->forwardedKeys[i];
            if (message.value("key").toInt() == key && message.value("down").toBool()) {
                QCOMPARE(message.value("modifiers").toInt(), int(modifiers));
                found = true;
            }
        }
        QVERIFY(found);
        QCOMPARE(editInput->property("text").toString(), selectedValue);
    }
    fixture.shell.clearActions();
    const QPoint wordPoint = fixture.window->mapFromGlobal(editInput->mapToGlobal(
        QPointF(45, editInput->height() / 2))).toPoint();
    QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier, wordPoint);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, wordPoint);
    QTRY_VERIFY(!fixture.shell.actions.isEmpty()
        && fixture.shell.actions.constLast().value("action") == "control.select");
    QVERIFY(!editInput->property("selectedText").toString().isEmpty());
    const auto wordSelection = fixture.shell.actions.constLast();
    QVERIFY(wordSelection.value("anchor") != wordSelection.value("cursor"));
    const int beforeBackspace = keySink->forwardedKeys.size();
    QTest::keyClick(fixture.window, Qt::Key_Backspace);
    QVERIFY(keySink->forwardedKeys.size() >= beforeBackspace + 2);

    // A press in the field's visual margin must behave like a press on the
    // text itself.  The native TextInput is inset by the control's padding,
    // so without the explicit margin hit areas this common I-beam starting
    // position never establishes an anchor and the drag produces no range.
    QQuickItem *const editControl = visualItem(
        root, QStringLiteral("dialogWidget-appearance-editEdit"));
    QVERIFY(editControl);
    QQuickItem *const leftMargin = visualItem(
        root, QStringLiteral(
            "dialogWidget-appearance-editEditLeftMarginSelectionArea"));
    QQuickItem *const rightMargin = visualItem(
        root, QStringLiteral(
            "dialogWidget-appearance-editEditRightMarginSelectionArea"));
    QVERIFY(leftMargin);
    QVERIFY(rightMargin);

    const qreal controlCenterY = editControl->height() / 2.0;
    const QPoint leftMarginStart = fixture.window->mapFromGlobal(
        editControl->mapToGlobal(QPointF(2, controlCenterY))).toPoint();
    const QPoint textMiddle = fixture.window->mapFromGlobal(
        editInput->mapToGlobal(QPointF(editInput->width() / 2.0,
                                       editInput->height() / 2.0))).toPoint();
    const QPoint textInputLeft = fixture.window->mapFromGlobal(
        editInput->mapToGlobal(QPointF(0, 0))).toPoint();
    const QPoint textInputRight = fixture.window->mapFromGlobal(
        editInput->mapToGlobal(QPointF(editInput->width(), 0))).toPoint();
    QVERIFY(leftMarginStart.x() < textInputLeft.x());
    QVERIFY(QMetaObject::invokeMethod(editInput, "deselect"));
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      leftMarginStart);
    QTest::mouseMove(fixture.window, textMiddle, 100);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier,
                        textMiddle);
    QVERIFY2(editInput->property("selectedText").toString().size() > 0,
             "drag beginning in the left text-field margin did not select");

    const QPoint rightMarginStart = fixture.window->mapFromGlobal(
        editControl->mapToGlobal(QPointF(editControl->width() - 2,
                                          controlCenterY))).toPoint();
    const QPoint textStart = fixture.window->mapFromGlobal(
        editInput->mapToGlobal(QPointF(2, editInput->height() / 2.0))).toPoint();
    QVERIFY(rightMarginStart.x() > textInputRight.x());
    QVERIFY(QMetaObject::invokeMethod(editInput, "deselect"));
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      rightMarginStart);
    QTest::mouseMove(fixture.window, textStart, 100);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier,
                        textStart);
    QVERIFY2(editInput->property("selectedText").toString().size() > 0,
             "drag beginning in the right text-field margin did not select");

    // Margin clicks must retain the same word/line gesture semantics as the
    // native TextInput.  In particular, the hit target is intentionally just
    // outside the text, so these clicks exercise the forwarding area rather
    // than the input itself.
    QVERIFY(QMetaObject::invokeMethod(editInput, "deselect"));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      leftMarginStart);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      leftMarginStart);
    QCOMPARE(editInput->property("selectedText").toString(),
             QStringLiteral("C"));

    QVERIFY(QMetaObject::invokeMethod(editInput, "deselect"));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      rightMarginStart);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      rightMarginStart);
    QCOMPARE(editInput->property("selectedText").toString(),
             QStringLiteral("Windows"));

    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                      rightMarginStart);
    QCOMPARE(editInput->property("selectedText").toString(),
             QStringLiteral("C:\\Windows"));
    QCOMPARE(editInput->property("selectionStart").toInt(), 0);
    QCOMPARE(editInput->property("selectionEnd").toInt(), 10);
}

void F4OperationsQueueTests::semanticDialogEditSelectionWaitsForSemanticFocus()
{
    QVariantMap scene = dialogControlsScene(false);
    QVariantList dialogs = scene.value(QStringLiteral("dialogs")).toList();
    QVariantMap dialog = dialogs.constFirst().toMap();
    QVariantList children = dialog.value(QStringLiteral("children")).toList();
    for (qsizetype index = 0; index < children.size(); ++index) {
        QVariantMap child = children.at(index).toMap();
        if (child.value(QStringLiteral("id")).toString()
            == QStringLiteral("appearance-edit")) {
            child.insert(QStringLiteral("selectionStart"), 1);
            child.insert(QStringLiteral("selectionEnd"), 4);
            child.insert(QStringLiteral("x"), 20);
            children[index] = child;
            break;
        }
    }
    dialog.insert(QStringLiteral("children"), children);
    dialogs[0] = dialog;
    scene.insert(QStringLiteral("dialogs"), dialogs);

    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *editInput = nullptr;
    QQuickItem *editField = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (editInput = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("dialogWidget-appearance-editEditTextInput"))),
        3000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (editField = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("dialogWidget-appearance-editEdit"))),
        1000);

    // New semantic edits may carry an initial SelectAll range, but that range
    // is dormant until Go activates the control.  The Qt field must therefore
    // remain visually unselected while the dialog is only being displayed.
    QCOMPARE(editField->property("remoteSelectionActivated").toBool(),
             false);
    QCOMPARE(editInput->property("selectedText").toString(), QString());

    QVariantMap focusedScene = dialogControlsScene(true);
    QVariantList focusedDialogs = focusedScene.value(
        QStringLiteral("dialogs")).toList();
    QVariantMap focusedDialog = focusedDialogs.constFirst().toMap();
    QVariantList focusedChildren = focusedDialog.value(
        QStringLiteral("children")).toList();
    for (qsizetype index = 0; index < focusedChildren.size(); ++index) {
        QVariantMap child = focusedChildren.at(index).toMap();
        if (child.value(QStringLiteral("id")).toString()
            == QStringLiteral("appearance-edit")) {
            child.insert(QStringLiteral("selectionStart"), 1);
            child.insert(QStringLiteral("selectionEnd"), 4);
            child.insert(QStringLiteral("x"), 20);
            focusedChildren[index] = child;
            break;
        }
    }
    focusedDialog.insert(QStringLiteral("children"), focusedChildren);
    focusedDialogs[0] = focusedDialog;
    focusedScene.insert(QStringLiteral("dialogs"), focusedDialogs);
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), focusedScene.value(
             QStringLiteral("dialogs")).toList()},
    }, 2);

    editInput = nullptr;
    editField = nullptr;
    QTRY_VERIFY_WITH_TIMEOUT(
        (editInput = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("dialogWidget-appearance-editEditTextInput"))),
        1000);
    QTRY_VERIFY_WITH_TIMEOUT(
        (editField = visualItem(
             fixture.window->contentItem(),
             QStringLiteral("dialogWidget-appearance-editEdit"))),
        1000);
    QTRY_COMPARE_WITH_TIMEOUT(
        editField->property("remoteSelectionActivated").toBool(), true,
        1000);
    QTRY_COMPARE_WITH_TIMEOUT(editInput->property("selectedText").toString(),
                              QStringLiteral(":\\W"), 1000);
}

QTEST_MAIN(F4OperationsQueueTests)

#include "F4OperationsQueueTests.moc"

void F4OperationsQueueTests::driveDetailsUseMeasuredColumnsOnPhysicalPixelGrid()
{
    auto scene = panelScene();
    auto menu = dialogComboMenu(0);
    menu.insert("id", "drives"); menu.remove("ownerId"); menu.remove("presentation");
    QVariantList rows;
    const QStringList icons{"hard-drive", "usb-flash-drive", "disc", "network", "memory-stick", "folder-symlink", "database", "circle-question-mark"};
    for (int i = 0; i < icons.size(); ++i) {
        QVariantMap details{{"isDrive", "true"}, {"name", QString(QChar('C' + i)) + ":"}, {"label", "Work disk"}, {"icon", icons[i]},
            {"filesystem", "NTFS"}, {"total", "2 TB"}, {"free", "1 TB"}, {"network", "server"}};
        if (i != 7) details.insert("usedFraction", i == 0 ? "0.5" : i == 1 ? "0" : "1");
        rows.append(QVariantMap{{"index", i}, {"separator", false}, {"header", false}, {"text", "padded console text"}, {"details", details}});
    }
    rows.append(QVariantMap{{"index", 8}, {"separator", false}, {"header", false}, {"text", "Ordinary menu item"}});
    rows.append(QVariantMap{{"index", 9}, {"separator", false}, {"header", false},
        {"details", QVariantMap{{"isDrive", "false"}, {"name", "A very long virtual provider title that must not widen the drive caption column"}, {"icon", "blocks"}}}});
    menu.insert("items", rows); scene.insert("menus", QVariantList{menu});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *label = nullptr;
    QTRY_VERIFY((label = visualItem(fixture.window->contentItem(), "semanticMenuDetail-drives-0-name")) && label->isVisible());
    QTest::qWait(150);
    auto checkGrid = [&](QQuickItem *leaf) {
        QVERIFY(leaf);
        const auto origin = leaf->mapToItem(fixture.window->contentItem(), QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY2(qAbs(physical.x() - qRound64(physical.x())) < .001 && qAbs(physical.y() - qRound64(physical.y())) < .001,
                 qPrintable(QString("%1 %2,%3").arg(leaf->objectName()).arg(physical.x()).arg(physical.y())));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(1,0)) - origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(fixture.window->contentItem(), QPointF(0,1)) - origin, QPointF(0,1));
    };
    qreal widestCaption = 0;
    for (int i = 0; i < icons.size(); ++i) {
        auto *caption = visualItem(fixture.window->contentItem(), QString("semanticMenuDetail-drives-%1-name").arg(i));
        QVERIFY(caption);
        widestCaption = qMax(widestCaption, caption->property("implicitWidth").toReal());
    }
    for (int i = 0; i < icons.size(); ++i) {
        const auto prefix = QString("semanticMenuDetail-drives-%1-").arg(i);
        for (const QString &field : {"name", "filesystem", "capacity"})
            checkGrid(visualItem(fixture.window->contentItem(), prefix + field));
        auto *icon = visualItem(fixture.window->contentItem(), QString("semanticMenuItemIcon-drives-%1").arg(i));
        checkGrid(icon);
        QCOMPARE(icon->property("semanticIconName").toString(), icons[i]);
        auto *track = visualItem(fixture.window->contentItem(), QString("semanticMenuCapacity-drives-%1").arg(i));
        QVERIFY(track); checkGrid(track);
        QCOMPARE(track->isVisible(), i != 7);
        auto *fill = visualItem(fixture.window->contentItem(), track->objectName() + "-used");
        checkGrid(fill);
        QVERIFY(qAbs(fill->width() * fixture.window->devicePixelRatio() - qRound64(fill->width() * fixture.window->devicePixelRatio())) < .001);
        QCOMPARE(fill->property("color").value<QColor>(), fixture.window->property("dialogAccent").value<QColor>());
        QCOMPARE(fill->opacity(), 1.0);
        QVERIFY(fill->width() <= track->width());
        if (i == 1) QCOMPARE(fill->width(), 0.0);
        if (i == 2) QCOMPARE(fill->width(), track->width());
        auto *fs = visualItem(fixture.window->contentItem(), prefix + "filesystem");
        QVERIFY(fs->x() > track->x() + track->width());
        auto *capacity = visualItem(fixture.window->contentItem(), prefix + "capacity");
        QVERIFY(capacity);
        QVERIFY(!capacity->property("truncated").toBool());
        const auto sizeColor = fixture.window->property("textColor").value<QColor>().name();
        QCOMPARE(capacity->property("text").toString(),
                 QString("<font color=\"%1\">1 TB</font> free of <font color=\"%1\">2 TB</font>").arg(sizeColor));
        QCOMPARE(capacity->property("color").value<QColor>(), fixture.window->property("mutedText").value<QColor>());
        QVERIFY(fs->x() >= capacity->x() + capacity->width());
        auto *caption = visualItem(fixture.window->contentItem(), prefix + "name");
        QVERIFY(caption);
        QVERIFY(!caption->property("truncated").toBool());
        // Only glyph measurement, a rounding allowance, and a 16 DIP gap.
        const qreal captionGap = track->x() - caption->x() - widestCaption;
        QVERIFY2(captionGap >= 16 && captionGap < 21, qPrintable(QString::number(captionGap)));
        const qreal labelGap = capacity->x() - track->x() - track->width();
        QVERIFY(labelGap >= 7 && labelGap <= 9);
        QVERIFY(labelGap < captionGap);
        QVERIFY(labelGap < fs->x() - capacity->x() - capacity->width());
        auto *row = visualItem(fixture.window->contentItem(), QString("semanticMenuItem-drives-%1").arg(i));
        auto *ordinary = visualItem(fixture.window->contentItem(), "semanticMenuItem-drives-8");
        QVERIFY(row && ordinary);
        QCOMPARE(row->height(), ordinary->height());
        auto *ordinaryText = visualItem(fixture.window->contentItem(), "semanticMenuItemText-drives-8");
        auto *driveName = visualItem(fixture.window->contentItem(), prefix + "name");
        QVERIFY(ordinaryText && driveName);
        checkGrid(ordinaryText);
        QCOMPARE(driveName->mapToItem(fixture.window->contentItem(), QPointF()).x(),
                 ordinaryText->mapToItem(fixture.window->contentItem(), QPointF()).x());
        const QRectF ink = QFontMetricsF(capacity->property("font").value<QFont>())
                               .tightBoundingRect(QStringLiteral("Ag"));
        const qreal inkCenter = capacity->y() + capacity->property("baselineOffset").toReal()
                                + ink.y() + ink.height() / 2;
        QVERIFY(qAbs(inkCenter - track->y() - track->height()/2)
                    * fixture.window->devicePixelRatio() <= 1);

    }
    auto *virtualCaption = visualItem(fixture.window->contentItem(), "semanticMenuDetail-drives-9-name");
    QVERIFY(virtualCaption);
    checkGrid(virtualCaption);
    QVERIFY(!virtualCaption->property("truncated").toBool());
    QVERIFY(label->property("text").toString().contains("C: (Work disk)"));
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_DRIVE_MENU_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_DRIVE_MENU_CAPTURE")));
}

void F4OperationsQueueTests::messageBodyWrapsToGuiWidth()
{
    auto scene = dialogScene();
    auto dialog = scene.value("dialogs").toList().first().toMap();
    const QString body = QString("Heading\n\n") + QString("A long message with <literal> & text. ").repeated(12);
    dialog.insert("children", QVariantList{
        QVariantMap{{"id", "message-body"}, {"kind", "text"}, {"text", body}, {"wrapText", true}, {"x", 20}, {"y", 4}, {"w", 60}, {"h", 1}},
        QVariantMap{{"id", "message-ok"}, {"kind", "button"}, {"text", "OK"}, {"x", 46}, {"y", 6}, {"w", 8}, {"h", 1}}});
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *bodyItem = nullptr;
    QTRY_VERIFY((bodyItem = visualItem(root, "dialogWidget-message-bodyText")) && bodyItem->isVisible());
    QTest::qWait(150);
    QCOMPARE(bodyItem->property("text").toString(), body);
    QVERIFY(!bodyItem->property("truncated").toBool());
    QVERIFY(bodyItem->property("lineCount").toInt() > 3);
    auto *button = visualItem(root, "dialogWidget-message-okButtonText");
    QVERIFY(button);
    QVERIFY(button->mapToItem(root, QPointF()).y() > bodyItem->mapToItem(root, QPointF(0, bodyItem->height())).y());
    for (auto *leaf : {bodyItem, button, visualItem(root, "semanticDialogTitle")}) {
        QVERIFY(leaf);
        const auto origin = leaf->mapToItem(root, QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .001 && qAbs(physical.y()-qRound64(physical.y())) < .001, qPrintable(leaf->objectName()));
        QCOMPARE(leaf->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1));
    }
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_MESSAGE_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_MESSAGE_CAPTURE")));
}


void F4OperationsQueueTests::fileGroupsOverviewKeepsScrollAndRendersPreview()
{
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"settings-center","kind":"dialog","layout":"settings","title":"Settings","x":0,"y":0,"w":100,"h":30,"children":[
        {"id":"categories","kind":"table","layoutRole":"navigation","columns":[{"title":"Settings","width":20}],"rows":[{"cells":["File groups"]}],"x":1,"y":2,"w":20,"h":20},
        {"id":"settings-page","kind":"group","layoutRole":"content","fileGroupsOverview":true,"x":23,"y":2,"w":75,"h":24,"children":[
          {"id":"fg-theme","kind":"comboBox","layoutRole":"overview-theme","text":"Zoin","items":["Zoin"],"x":23,"y":2,"w":25,"h":1},
          {"id":"fg-table","kind":"table","layoutRole":"overview-table","sortable":false,"x":23,"y":4,"w":75,"h":20,"cursor":1,"columns":[{"title":"Preview","width":20},{"title":"Group","width":20},{"title":"Conditions","width":30}],"rows":[]},
          {"id":"fg-actions","kind":"group","layoutRole":"overview-actions","x":23,"y":25,"w":75,"h":1,"children":[{"id":"fg-edit","kind":"button","text":"Edit F4","x":23,"y":25,"w":10,"h":1}]}
        ]},
        {"id":"ok","kind":"button","text":"OK","layoutRole":"accept","x":88,"y":28,"w":10,"h":1}
      ]})").toVariant().toMap();
    auto children = dialog.value("children").toList();
    auto page = children[1].toMap();
    auto controls = page.value("children").toList();
    auto table = controls[1].toMap();
    auto actions = controls[2].toMap();
    QVariantList buttons;
    const QStringList labels{"Add", "Edit F4", "Duplicate", "Delete", "Up", "Down", "More", "Restore standard set"};
    for (int i = 0; i < labels.size(); ++i) {
        buttons.append(QVariantMap{{"id", QString("fg-action-%1").arg(i)}, {"kind", "button"}, {"text", labels[i]}});
    }
    actions.insert("children", buttons);
    controls[2] = actions;
    controls.append(QVariantMap{{"id", "fg-colors"}, {"kind", "comboBox"},
        {"layoutRole", "overview-color-theme"}, {"text", "Colors: Current"},
        {"items", QStringList{"Colors: Current", "Colors: Zoin"}}, {"cursor", 0}});
    QVariantList rows;
    for (int i = 0; i < 60; ++i) {
        rows.append(QVariantMap{{"id",QString::number(i)}, {"separator",i == 0 ? "Universal" : ""},
            {"cells",QStringList{"sample.txt",QString::number(i),"*.txt"}},
            {"appearance",QVariantMap{{"tintIcon",true},{"cursor",QVariantMap{{"foreground","#fedcba"},{"background","#123456"}}}}}});
    }
    table.insert("rows",rows); controls[1]=table; page.insert("children",controls); children[1]=page; dialog.insert("children",children);
    auto scene=panelScene(); scene.insert("dialogs",QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root=fixture.window->contentItem();
    QQuickItem *restore = nullptr;
    QTRY_VERIFY((restore = visualItem(root, "dialogWidget-fg-action-7Button")));
    QCOMPARE(restore->property("text").toString(), QString("Restore standard set"));
    QVERIFY(restore->width() >= 160);
    QVERIFY(restore->y() + restore->height() <= restore->parentItem()->height() + 1);
    QQuickItem *colorTheme = nullptr;
    QQuickItem *iconTheme = nullptr;
    QTRY_VERIFY((colorTheme = visualItem(root, "dialogWidget-fg-colorsComboBox")));
    QTRY_VERIFY((iconTheme = visualItem(root, "dialogWidget-fg-themeComboBox")));
    QVERIFY(colorTheme->width() > 120);
    QVERIFY(colorTheme->mapToItem(root, QPointF()).x()
        >= iconTheme->mapToItem(root, QPointF(iconTheme->width(), 0)).x());
    QQuickItem *list=nullptr;
    QTRY_VERIFY((list=visualItem(root,"dialogWidget-fg-tableTableRows")));
    QVERIFY(list->height()>200);
    QQuickItem *cell=nullptr;
    QTRY_VERIFY((cell=visualItem(root,"dialogWidget-fg-tableTableCell-1-0")));
    QCOMPARE(cell->property("color"), fixture.window->property("galleryFileTextColor"));
    auto *icon=visualItem(root,"dialogWidget-fg-tableTableItemIcon-1");
    QVERIFY(icon);
    QCOMPARE(icon->property("color").value<QColor>(),QColor("#fedcba"));
    const auto *otherCell=visualItem(root,"dialogWidget-fg-tableTableCell-2-0");
    QVERIFY(otherCell);
    QCOMPARE(cell->x(),otherCell->x()); // Empty icon gutters still align.
    QVERIFY(list->setProperty("contentY",240.0));
    QCoreApplication::processEvents();
    const auto position=list->property("contentY").toReal();
    auto changedRow=rows[1].toMap(); changedRow.insert("marked",true); rows[1]=changedRow;
    table.insert("rows",rows); controls[1]=table; page.insert("children",controls); children[1]=page; dialog.insert("children",children);
    scene.insert("dialogs",QVariantList{dialog});fixture.shell.setScene(scene);
    QTest::qWait(120);
    QCOMPARE(list->property("contentY").toReal(),position);
}

void F4OperationsQueueTests::nestedFileGroupEditorBlocksParentNavigation()
{
    const auto makeDialog = [](const QString &id, bool child) {
        return QVariantMap{{"id", id}, {"kind", "dialog"}, {"layout", "settings"},
            {"title", child ? "File group" : "Settings"}, {"settingsSectionsOnly", child},
            {"x", 0}, {"y", 0}, {"w", 100}, {"h", 30},
            {"children", QVariantList{
                QVariantMap{{"id", id + "-categories"}, {"kind", "table"},
                    {"layoutRole", "navigation"}, {"x", 1}, {"y", 2}, {"w", 20}, {"h", 20},
                    {"showHeader", false}, {"cursor", 0},
                    {"columns", QVariantList{QVariantMap{{"title", "Sections"}, {"width", 20}}}},
                    {"rows", QVariantList{
                        QVariantMap{{"cells", QStringList{"Conditions"}}},
                        QVariantMap{{"cells", QStringList{"Appearance"}}},
                        QVariantMap{{"cells", QStringList{"Sorting"}}}}}},
                QVariantMap{{"id", id + "-page"}, {"kind", "group"}, {"layoutRole", "content"},
                    {"x", 23}, {"y", 2}, {"w", 75}, {"h", 20}}}}};
    };
    auto scene = panelScene();
    const auto parent = makeDialog("parent-settings", false);
    const auto child = makeDialog("filegroup-child", true);
    scene.insert("dialogs", QVariantList{parent, child});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *childRow = nullptr;
    QTRY_VERIFY((childRow = visualItem(root, "dialogWidget-filegroup-child-categoriesTableCell-1-0")));
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(childRow));
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    for (const auto &action : std::as_const(fixture.shell.actions)) {
        QVERIFY2(action.value("target") != "parent-settings-categories", "child pointer reached parent Settings");
    }
    QVERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "filegroup-child-categories" && action.value("index").toInt() == 1;
    }));
    fixture.shell.clearActions();
    QTest::keyClick(fixture.window, Qt::Key_Down);
    for (const auto &action : std::as_const(fixture.shell.actions))
        QVERIFY(action.value("target") != "parent-settings-categories");

    scene.insert("dialogs", QVariantList{parent});
    fixture.shell.setScene(scene);
    QQuickItem *parentRow = nullptr;
    QTRY_VERIFY((parentRow = visualItem(root, "dialogWidget-parent-settings-categoriesTableCell-1-0")));
    QTRY_VERIFY(parentRow->isEnabled());
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(parentRow));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "parent-settings-categories";
    }));
}

void F4OperationsQueueTests::fileGroupEditorCombinesAppearanceInRightColumn()
{
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"id:filegroup-editor:appearance","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"File group","x":2,"y":2,"w":125,"h":34,"children":[
        {"id":"sample","kind":"group","layoutRole":"editor-preview","fileGroupSample":true,
         "previewName":"example.txt","previewCatalog":"lucide","items":["Normal","Marked","Cursor","Marked cursor"],
         "previewStyle":{"marker":"! ","iconSource":"plugin","icon":"qrc:/F4QtHost/icons/lucide/images.svg",
           "tintIcon":true,"normal":{"foreground":"#ff0000"},
           "cursor":{"foreground":"#00ff00","fileNameForeground":"#00ff00"}}},
        {"id":"page","kind":"group","layoutRole":"content","scrollable":true,"contentHeight":50,"x":4,"y":5,"w":118,"h":24,
         "children":[
          {"id":"identity","kind":"group","title":"Group name","bordered":true,"x":4,"y":5,"w":118,"h":3,
           "children":[{"id":"name","kind":"edit","text":"Images","x":6,"y":6,"w":70,"h":1}]},
          {"id":"sort","kind":"group","title":"Sort priority","bordered":true,"x":4,"y":26,"w":118,"h":7,
           "children":[
             {"id":"sort-label","kind":"text","text":"Sort group number","x":6,"y":27,"w":40,"h":1},
             {"id":"sort-number","kind":"edit","text":"30","x":6,"y":28,"w":10,"h":1},
             {"id":"sort-toggle","kind":"button","text":"Hide existing groups","x":18,"y":28,"w":25,"h":1},
             {"id":"sort-list","kind":"table","x":6,"y":29,"w":112,"h":4,"showHeader":true,"sortable":false,
              "columns":[{"title":"Group","minWidth":24},{"title":"Sort number","width":12}],
              "rows":[{"cells":["PDFs","20"]},{"cells":["Images","100"]}]}]},
          {"id":"highlight","kind":"group","layoutRole":"section-header","sectionID":"filegroup-Highlighting","title":"Highlighting","x":4,"y":9,"w":118,"h":1},
          {"id":"colors","kind":"group","layoutRole":"filegroup-colors","x":4,"y":10,"w":118,"h":8,"children":[
            {"id":"color0","kind":"edit","colorMatrixCell":true},
            {"id":"color1","kind":"edit","colorMatrixCell":true},
            {"id":"color2","kind":"edit","colorMatrixCell":true},
            {"id":"color3","kind":"edit","colorMatrixCell":true}]},
          {"id":"rules","kind":"group","layoutRole":"filegroup-highlight","title":"Highlighting rules","bordered":true,
           "x":4,"y":18,"w":118,"h":5,"children":[
             {"id":"prefix-label","kind":"text","text":"Visual prefix","x":6,"y":19,"w":25,"h":1},
             {"id":"prefix","kind":"edit","text":"! ","x":6,"y":20,"w":60,"h":1},
             {"id":"continue","kind":"checkbox","layoutRole":"highlighting-option","text":"Continue highlighting",
              "x":6,"y":21,"w":54,"h":1,"state":1},
             {"id":"defaults","kind":"checkbox","layoutRole":"highlighting-option-last","text":"Override attribute defaults",
              "x":64,"y":21,"w":54,"h":1,"state":1}]},
          {"id":"icons","kind":"group","layoutRole":"section-header","sectionID":"filegroup-Icons","title":"Icons","x":4,"y":20,"w":118,"h":1},
          {"id":"artwork","kind":"group","layoutRole":"filegroup-icons","title":"Artwork","bordered":true,"x":4,"y":22,"w":118,"h":3,
           "children":[{"id":"catalog","kind":"comboBox","text":"Automatic","iconTheme":"lucide","iconReference":"asset:images",
                        "x":6,"y":23,"w":24,"h":1}]}
         ]},
        {"id":"ok","kind":"button","layoutRole":"accept","text":"OK","x":100,"y":32,"w":10,"h":1}]
    })").toVariant().toMap();
    auto controls = dialog.value("children").toList();
    auto page = controls[1].toMap();
    auto groups = page.value("children").toList();
    const QStringList icons{"plus", "trash-2", "arrow-up", "arrow-down"};
    for (int side = 0; side < 2; ++side) {
        const QString id = side == 0 ? "regular" : "alternative";
        QVariantList buttons;
        for (int i = 0; i < icons.size(); ++i) {
            buttons.append(QVariantMap{{"id", id + QString::number(i)}, {"kind", "button"},
                {"iconName", icons[i]}, {"disabled", i > 0}, {"text", icons[i]}, {"y", 9}});
        }
        const QVariantMap bar{{"id", id + "-actions"}, {"kind", "group"},
            {"layoutRole", "association-buttons"}, {"children", buttons},
            {"x", 6}, {"y", 9}, {"w", 60}, {"h", 1}};
        groups.insert(1 + side, QVariantMap{{"id", id}, {"kind", "group"},
            {"layoutRole", "association-" + id}, {"title", side == 0 ? "Open (Enter)" : "Alternate open"},
            {"bordered", true}, {"x", 4}, {"y", 8}, {"w", 118}, {"h", 4},
            {"children", QVariantList{bar}}});
    }
    page.insert("children", groups);
    controls[1] = page;
    dialog.insert("children", controls);
    auto scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *list = nullptr;
    QTRY_VERIFY(list = visualItem(root, "fileGroupPreviewList"));
    QCOMPARE(list->property("count").toInt(), 4);
    auto *name = visualItem(root, "dialogWidget-nameEditTextInput");
    auto *changeIcon = visualItem(root, "fileGroupChangeIcon");
    QVERIFY(name && changeIcon);
    QVERIFY(name->mapToScene(QPointF{}).x() < list->mapToScene(QPointF{}).x());
    auto *heading = visualItem(root, "dialogWidget-highlightGroupTitle");
    QVERIFY(heading);
    QVERIFY(heading->mapToScene(QPointF{}).y() < list->mapToScene(QPointF{}).y());
    QVERIFY(!visualItem(root, "fileGroupPinnedSample-0"));
    QVERIFY(!visualItem(root, "dialogWidget-catalogComboBox"));

    QStringList leaves{"dialogWidget-nameEditTextInput", "dialogWidget-highlightGroupTitle",
                       "fileGroupChangeIconText"};
    for (int index = 0; index < 4; ++index) {
        leaves << QString("fileGroupPreviewState-%1").arg(index)
               << QString("fileGroupPreviewName-%1").arg(index)
               << QString("fileGroupPreviewIcon-%1").arg(index)
               << QString("dialogWidget-color%1ColorEditButtonText").arg(index);
    }
    for (const auto &id : {"continue", "defaults"}) {
        const QString prefix = QString("dialogWidget-%1CheckBox").arg(id);
        leaves << prefix + "Text" << prefix + "Indicator" << prefix + "CheckMark";
    }
    leaves << "dialogWidget-prefix-labelText" << "dialogWidget-prefixEditTextInput";
    auto *firstOption = visualItem(root, "dialogWidget-continueCheckBox");
    auto *secondOption = visualItem(root, "dialogWidget-defaultsCheckBox");
    QVERIFY(firstOption && secondOption);
    QCOMPARE(firstOption->mapToScene(QPointF{}).y(), secondOption->mapToScene(QPointF{}).y());
    QVERIFY(firstOption->mapToScene(QPointF(firstOption->width(), 0)).x()
            < secondOption->mapToScene(QPointF{}).x());
    QTRY_VERIFY(!fixture.window->grabWindow().isNull());
    for (int i = 0; i < 4; ++i) {
        QQuickItem *icon = nullptr;
        QQuickItem *name = nullptr;
        QTRY_VERIFY(icon = visualItem(root, QString("fileGroupPreviewIcon-%1").arg(i)));
        QTRY_VERIFY(name = visualItem(root, QString("fileGroupPreviewName-%1").arg(i)));
        QCOMPARE(name->property("text").toString(), QString("! example.txt"));
        if (i == 0) {
            QCOMPARE(name->property("color"), fixture.window->property("galleryFileTextColor"));
            QCOMPARE(icon->property("color").value<QColor>(), QColor("#ff0000"));
        } else if (i == 2) {
            QCOMPARE(name->property("color").value<QColor>(), QColor("#00ff00"));
        }
        QVERIFY2(icon->isVisible(), qPrintable(icon->property("source").toUrl().toString()));
        QCOMPARE(icon->property("status").toInt(), 1); // Image.Ready
        QVERIFY(icon->mapToScene(QPointF(icon->width(), 0)).x() + 4
                <= name->mapToScene(QPointF{}).x());
    }
    auto *regular = visualItem(root, "dialogWidget-regularGroupTitle");
    auto *alternative = visualItem(root, "dialogWidget-alternativeGroupTitle");
    QVERIFY(regular && alternative);
    QVERIFY(regular->mapToScene(QPointF{}).x() < alternative->mapToScene(QPointF{}).x());
    QCOMPARE(regular->mapToScene(QPointF{}).y(), alternative->mapToScene(QPointF{}).y());
    leaves << regular->objectName() << alternative->objectName();
    for (const auto &id : {"regular", "alternative"}) {
        for (int i = 0; i < icons.size(); ++i) {
            const QString name = QString("dialogWidget-%1%2Button").arg(id).arg(i);
            auto *button = visualItem(root, name);
            auto *icon = visualItem(root, name + "Icon");
            QVERIFY(button && icon);
            QCOMPARE(button->isEnabled(), i == 0);
            QVERIFY(!icon->property("source").toUrl().isEmpty());
            QTRY_COMPARE(icon->property("status").toInt(), 1); // Image.Ready
            QVERIFY(icon->width() > 0 && icon->height() > 0);
            leaves << icon->objectName();
        }
    }
    auto *sortNumber = visualItem(root, "dialogWidget-sort-numberEditTextInput");
    auto *sortButton = visualItem(root, "dialogWidget-sort-toggleButton");
    QVERIFY(sortNumber && sortButton);
    QVERIFY(sortNumber->mapToScene(QPointF(sortNumber->width(), 0)).x()
            < sortButton->mapToScene(QPointF{}).x());
    auto *sortTable = visualItem(root, "dialogWidget-sort-listTableBackground");
    QVERIFY(sortTable && sortButton->mapToScene(QPointF(0, sortButton->height())).y()
            <= sortTable->mapToScene(QPointF{}).y());
    leaves << "dialogWidget-sortGroupTitle" << "dialogWidget-sort-labelText"
           << "dialogWidget-sort-numberEditTextInput" << "dialogWidget-sort-toggleButtonText"
           << "dialogWidget-sort-listTableHeader-0" << "dialogWidget-sort-listTableHeader-1"
           << "dialogWidget-sort-listTableCell-0-0" << "dialogWidget-sort-listTableCell-0-1"
           << "dialogWidget-sort-listTableCell-1-0" << "dialogWidget-sort-listTableCell-1-1";
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto verifyLeaves = [&](const QStringList &names) {
        QTRY_VERIFY(!fixture.window->grabWindow().isNull());
        for (const QString &name : names) {
            auto *leaf = visualItem(root, name);
            QVERIFY2(leaf && leaf->isVisible(), qPrintable(name));
            if (qAbs(dpr - 1.75) >= 0.001)
                continue;
            const auto origin = leaf->mapToItem(root, QPointF{});
            const auto physical = origin * dpr;
            const auto detail = QString("%1 at (%2, %3)").arg(name).arg(physical.x()).arg(physical.y());
            QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.001
                     && qAbs(physical.y() - qRound(physical.y())) < 0.001, qPrintable(detail));
            QCOMPARE(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1));
        }
    };
    verifyLeaves(leaves);
    const QString captureDir = qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR");
    if (!captureDir.isEmpty())
        QVERIFY(fixture.window->grabWindow().save(QDir(captureDir).filePath("file-group-appearance-175.png")));

    auto *colorEdit = visualItem(root, "dialogWidget-color0ColorEditButton");
    QVERIFY(colorEdit);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(colorEdit));
    QTRY_VERIFY(visualItem(root, "dialogWidget-color0ColorPickerTitle"));
    QTest::keyClick(fixture.window, Qt::Key_Escape);
    QTRY_VERIFY(!visualItem(root, "dialogWidget-color0ColorPickerTitle"));

    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(changeIcon));
    QTRY_VERIFY(visualItem(root, "dialogWidget-catalogComboBox"));
    verifyLeaves({"dialogWidget-catalogComboBoxText", "dialogWidget-catalogComboBoxIconPreview",
                  "dialogWidget-catalogComboBoxIndicator"});
}

void F4OperationsQueueTests::fileGroupEditorMatrixAndPinnedPreview()
{
    auto dialog=QJsonDocument::fromJson(R"({
      "id":"group-editor-matrix","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"File group","x":4,"y":2,"w":90,"h":28,"children":[
        {"id":"pinned","kind":"group","layoutRole":"editor-preview","fileGroupSample":true,
         "previewName":"example.txt","previewCatalog":"zoin","items":["Normal","Marked","Cursor","Marked cursor"],
         "previewStyle":{"tintIcon":true,"normal":{"foreground":"#112233","background":"#eeeedd"},
           "selected":{"foreground":"#223344","background":"#ddddcc"},
           "cursor":{"foreground":"#334455","background":"#ccccbb"},
           "selectedCursor":{"foreground":"#445566","background":"#bbbbaa"}}},
        {"id":"matrix-page","kind":"group","layoutRole":"content","scrollable":true,"contentHeight":100,"x":8,"y":8,"w":80,"h":15,
         "children":[{"id":"colours","kind":"group","title":"Colors","bordered":true,"x":8,"y":8,"w":78,"h":12,"children":[
           {"id":"nlabel","kind":"text","text":"Normal","x":10,"y":10,"w":12,"h":1},
           {"id":"n","kind":"edit","colorMatrixCell":true,"text":"foreground:#112233 | B_WHITE","colorComponents":{"foreground":"foreground:#112233","background":"B_WHITE"},"x":23,"y":10,"w":60,"h":1,"colorPreview":{"foreground":"#112233","background":"#eeeedd"}},
           {"id":"slabel","kind":"text","text":"Marked","x":10,"y":12,"w":12,"h":1},
           {"id":"s","kind":"edit","colorMatrixCell":true,"text":"","x":23,"y":12,"w":60,"h":1},
           {"id":"clabel","kind":"text","text":"Cursor","x":47,"y":10,"w":12,"h":1},
           {"id":"c","kind":"edit","colorMatrixCell":true,"text":"foreground:#334455","x":60,"y":10,"w":24,"h":1},
           {"id":"sclabel","kind":"text","text":"Marked cursor","x":47,"y":12,"w":12,"h":1},
           {"id":"sc","kind":"edit","colorMatrixCell":true,"text":"foreground:#445566","x":60,"y":12,"w":24,"h":1}]},
           {"id":"tail","kind":"text","text":"Additional properties","x":10,"y":100,"w":60,"h":1}]},
        {"id":"ok","kind":"button","layoutRole":"accept","text":"OK","x":60,"y":27,"w":10,"h":1},
        {"id":"cancel","kind":"button","layoutRole":"cancel","text":"Cancel","x":73,"y":27,"w":10,"h":1}]
    })").toVariant().toMap();
    auto allChildren=dialog.value("children").toList();
    auto page=allChildren[1].toMap();
    auto properties=page.value("children").toList();
    auto colours=properties[0].toMap();
    auto colourFields=colours.value("children").toList();
    for(int i=0;i<colourFields.size();++i) {
        auto field=colourFields[i].toMap();
        if(field.value("colorMatrixCell").toBool()) {
            field.insert("w",16);
            field.insert("colorMatrixColumnWidth",34);
            field.insert("items",QStringList{"Normal","Marked","Cursor","Marked cursor"});
            colourFields[i]=field;
        }
    }
    colours.insert("children",colourFields);properties[0]=colours;
    for(int i=0;i<16;++i)
        properties.append(QVariantMap{{"id",QString("extra-%1").arg(i)},{"kind","edit"},
            {"text","Additional property"},{"x",10},{"y",25+i*2},{"w",60},{"h",1}});
    page.insert("children",properties);allChildren[1]=page;dialog.insert("children",allChildren);
    auto scene=panelScene();scene.insert("dialogs",QVariantList{dialog});
    QueueFixture fixture(scene);QVERIFY(fixture.window);
    auto *root=fixture.window->contentItem();
    QQuickItem *sample=nullptr;
    QTRY_VERIFY((sample=visualItem(root,"fileGroupPinnedSample-0")));
    QCOMPARE(sample->property("color").value<QColor>(),QColor("#eeeedd"));
    for (int i=0;i<4;++i) {
        auto *tile=visualItem(root,QString("fileGroupPinnedSample-%1").arg(i));
        QVERIFY(tile && tile->isVisible() && tile->width()>20 && tile->height()>10);
        auto *icon=visualItem(root,QString("fileGroupPinnedIcon-%1").arg(i));
        QVERIFY(icon);
        const QStringList colours{"#112233","#223344","#334455","#445566"};
        QCOMPARE(icon->property("color").value<QColor>(),QColor(colours[i]));
    }
    auto *viewport=visualItem(root,"dialogWidget-matrix-pageViewport");
    QVERIFY(viewport);
    auto *swatch=visualItem(root,"dialogWidget-nColorSwatch");
    QVERIFY(swatch && swatch->isVisible());
    QCOMPARE(swatch->property("color").value<QColor>(),QColor("#eeeedd"));
    auto *first=visualItem(root,"dialogWidget-nColorSwatch");
    auto *last=visualItem(root,"dialogWidget-scColorSwatch");
    QVERIFY(first && last);
    auto *marked=visualItem(root,"dialogWidget-sColorSwatch");
    auto *cursor=visualItem(root,"dialogWidget-cColorSwatch");
    QVERIFY(marked && cursor);
    QCOMPARE(first->mapToScene(QPointF()).x(),marked->mapToScene(QPointF()).x());
    QCOMPARE(cursor->mapToScene(QPointF()).x(),last->mapToScene(QPointF()).x());
    QCOMPARE(first->mapToScene(QPointF()).y(),cursor->mapToScene(QPointF()).y());
    QCOMPARE(marked->mapToScene(QPointF()).y(),last->mapToScene(QPointF()).y());
    auto *leftSwatch=visualItem(root,"dialogWidget-nColorSwatch");
    auto *cursorLabel=visualItem(root,"dialogWidget-clabelText");
    QVERIFY(leftSwatch && cursorLabel);
    QVERIFY(leftSwatch->mapToScene(QPointF(leftSwatch->width(), 0)).x()
        < cursorLabel->mapToScene(QPointF{}).x());
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) < 0.001) {
        for (const auto &id : {"n", "s", "c", "sc"}) {
            for (const auto &suffix : {"labelText", "ColorEditButtonText", "ColorSwatchText"}) {
                const auto name = QString("dialogWidget-%1%2").arg(id, suffix);
                auto *leaf = visualItem(root, name);
                QVERIFY2(leaf && leaf->isVisible(), qPrintable(name));
                const auto origin = leaf->mapToItem(root, QPointF{});
                const auto physical = origin * dpr;
                QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.02
                         && qAbs(physical.y() - qRound(physical.y())) < 0.02, qPrintable(name));
                QVERIFY(QLineF(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
                QVERIFY(QLineF(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
            }
        }
        if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
            QVERIFY(fixture.window->grabWindow().save(
                QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-color-matrix-175.png")));
    }
    QVERIFY(last->mapToScene(QPointF()).y()-first->mapToScene(QPointF()).y()<200);
    QVERIFY(swatch->width() > 65 && swatch->height() >= 39);
    auto *inherited = visualItem(root, "dialogWidget-sColorSwatchText");
    QVERIFY(inherited);
    QCOMPARE(inherited->property("text").toString(), QString("Inherited"));
    QVERIFY(!visualItem(root, "dialogWidget-sEditTextInput"));
    auto *edit = visualItem(root, "dialogWidget-nColorEditButton");
    QVERIFY(edit);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(edit));
    QQuickItem *apply = nullptr;
    QTRY_VERIFY((apply = visualItem(root, "dialogWidget-nColorApply")) && apply->isVisible());
    if (qAbs(dpr - 1.75) < 0.001) {
        for (const auto &suffix : {"ColorPickerTitle", "ColorTextChannelText", "ColorBackgroundChannelText",
                                   "ColorHexTextInput", "ColorInheritText", "ColorApplyText", "ColorCancelText"}) {
            const auto name = QString("dialogWidget-n%1").arg(suffix);
            auto *leaf = visualItem(root, name);
            QVERIFY2(leaf && leaf->isVisible(), qPrintable(name));
            const auto origin = leaf->mapToItem(root, QPointF{});
            const auto physical = origin * dpr;
            QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.02
                     && qAbs(physical.y() - qRound(physical.y())) < 0.02, qPrintable(name));
            QVERIFY(QLineF(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
            QVERIFY(QLineF(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
        }
        if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
            QVERIFY(fixture.window->grabWindow().save(
                QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-color-picker-175.png")));
    }
    auto *preset = visualItem(root, "dialogWidget-nColorPreset-12");
    QVERIFY(preset);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(preset));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(apply));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "n" && action.value("action") == "control.setText"
            && action.value("text") == "foreground:#ff0000 | B_WHITE";
    }));
    QTRY_VERIFY(!visualItem(root, "dialogWidget-nColorApply"));
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(edit));
    QTRY_VERIFY(visualItem(root, "dialogWidget-nColorCancel"));
    auto *backgroundChannel = visualItem(root, "dialogWidget-nColorBackgroundChannel");
    auto *inherit = visualItem(root, "dialogWidget-nColorInherit");
    QVERIFY(backgroundChannel && inherit);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(backgroundChannel));
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(inherit));
    auto *placeholder = visualItem(root, "dialogWidget-nColorHexPlaceholder");
    QVERIFY(placeholder && placeholder->isVisible());
    if (qAbs(dpr - 1.75) < 0.001) {
        auto physical = placeholder->mapToItem(root, QPointF{}) * dpr;
        const auto aligned = [&] {
            physical = placeholder->mapToItem(root, QPointF{}) * dpr;
            return qAbs(physical.x() - qRound(physical.x())) < 0.02
                && qAbs(physical.y() - qRound(physical.y())) < 0.02;
        };
        if (!aligned())
            qInfo() << "Newly revealed hex placeholder, before scene polish:" << physical;
        QTRY_VERIFY2(aligned(), qPrintable(QString("Hex placeholder scene position (%1, %2) px")
            .arg(physical.x()).arg(physical.y())));
        const auto origin = placeholder->mapToItem(root, QPointF{});
        QVERIFY(QLineF(placeholder->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
        QVERIFY(QLineF(placeholder->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
        if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
            QVERIFY(fixture.window->grabWindow().save(
                QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-color-inherit-175.png")));
    }
    QTest::keyClick(fixture.window, Qt::Key_Escape);
    QTRY_VERIFY(!visualItem(root, "dialogWidget-nColorCancel"));
    QVERIFY(std::none_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("action") == "control.setText";
    }));
    const auto samplePosition=sample->mapToScene(QPointF());
    QVERIFY(viewport->property("contentHeight").toReal()>viewport->height());
    QVERIFY(viewport->setProperty("contentY",150.0));
    QCoreApplication::processEvents();
    QCOMPARE(sample->mapToScene(QPointF()),samplePosition);
    auto children=dialog.value("children").toList();
    auto preview=children[0].toMap();
    auto style=preview.value("previewStyle").toMap();
    style.insert("normal",QVariantMap{{"foreground","#abcdef"},{"background","#332211"}});
    preview.insert("previewStyle",style);children[0]=preview;dialog.insert("children",children);
    scene.insert("dialogs",QVariantList{dialog});fixture.shell.setScene(scene);
    QTRY_COMPARE(sample->property("color").value<QColor>(),QColor("#332211"));
    QCOMPARE(sample->mapToScene(QPointF()),samplePosition);
    QCOMPARE(viewport->property("contentY").toReal(),150.0);
    preview.insert("previewCatalog","system");children[0]=preview;dialog.insert("children",children);
    scene.insert("dialogs",QVariantList{dialog});fixture.shell.setScene(scene);
    auto *nativeIcon=visualItem(root,"fileGroupPinnedIcon-0");
    QVERIFY(nativeIcon);
    QTRY_COMPARE(nativeIcon->property("color").value<QColor>(),QColor(Qt::transparent));
}

void F4OperationsQueueTests::checkableGroupHeaderRendersAndToggles()
{
    const auto dialog=QJsonDocument::fromJson(R"({
      "id":"group-editor","kind":"dialog","title":"File group","x":10,"y":3,"w":70,"h":20,
      "children":[{"id":"appearance","kind":"group","title":"Highlighting","bordered":true,"x":12,"y":6,"w":65,"h":8,
        "headerControl":{"id":"enabled","kind":"checkbox","text":"Highlighting","state":1,"visible":true,"x":14,"y":6,"w":20,"h":1},
        "children":[{"id":"normal","kind":"edit","text":"foreground:#123456","x":14,"y":9,"w":55,"h":1}]}]
    })").toVariant().toMap();
    auto scene=panelScene();scene.insert("dialogs",QVariantList{dialog});
    QueueFixture fixture(scene);QVERIFY(fixture.window);
    QQuickItem *checkbox=nullptr;
    QTRY_VERIFY((checkbox=visualItem(fixture.window->contentItem(),"dialogWidget-enabledCheckBox")));
    QVERIFY(checkbox->isVisible());
    QVERIFY(checkbox->width()>20);
    QCOMPARE(checkbox->property("checked").toBool(),true);
    auto *caption=visualItem(fixture.window->contentItem(),"dialogWidget-enabledCheckBoxText");
    QVERIFY(caption);
    QVERIFY(!caption->property("truncated").toBool());
    QVERIFY(caption->property("font").value<QFont>().bold());
    QCOMPARE(caption->property("color").value<QColor>(), fixture.window->property("textColor").value<QColor>());
    auto *surface = visualItem(fixture.window->contentItem(), "dialogWidget-appearanceGroupBorder");
    QVERIFY(surface);
    QCOMPARE(surface->property("testBorderWidth").toReal(), 0.0);
    QVERIFY(checkbox->mapToItem(surface, QPointF{}).y() + checkbox->height() <= -3);
    fixture.shell.clearActions();
    const auto position=checkbox->mapToScene(QPointF(10,checkbox->height()/2)).toPoint();
    QTest::mouseClick(fixture.window,Qt::LeftButton,Qt::NoModifier,position);
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    const bool toggled=std::any_of(fixture.shell.actions.cbegin(),fixture.shell.actions.cend(),[](const QVariantMap &a){return a.value("target").toString()==QStringLiteral("enabled");});
    QVERIFY2(toggled,qPrintable(QJsonDocument::fromVariant(QVariant::fromValue(fixture.shell.actions)).toJson()));
}

static void verifyGroupCheckBoxFocusFrame(QQuickItem *root, const QString &name, qreal dpr)
{
    auto *caption = visualItem(root, name + "Text");
    auto *frame = visualItem(root, name + "FocusFrame");
    QVERIFY(caption && frame);
    QTRY_VERIFY(frame->property("testBorderWidth").toReal() > 0);
    QVERIFY(!caption->property("truncated").toBool());
    const QFontMetricsF font(caption->property("font").value<QFont>());
    const qreal textEnd = caption->mapToItem(root, QPointF(
        caption->property("leftPadding").toReal() + font.horizontalAdvance(caption->property("text").toString()), 0)).x();
    const qreal frameEnd = frame->mapToItem(root, QPointF(frame->width(), 0)).x();
    const qreal gap = (frameEnd - textEnd) * dpr;
    QVERIFY2(gap >= 2 && gap <= 5, qPrintable(QString("Focus frame ends at %1 px, %2 px after the label")
        .arg(frameEnd * dpr).arg(gap)));
    const auto origin = frame->mapToItem(root, QPointF{});
    const auto physical = origin * dpr;
    QVERIFY(qAbs(physical.x() - qRound(physical.x())) < .02 && qAbs(physical.y() - qRound(physical.y())) < .02);
    QVERIFY(qAbs(frameEnd * dpr - qRound(frameEnd * dpr)) < .02);
}

void F4OperationsQueueTests::fileGroupSectionHeadersStayPixelAligned()
{
    const auto dialog = QJsonDocument::fromJson(R"({
      "id":"sections-editor","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"File group","x":4,"y":2,"w":85,"h":24,"children":[
        {"id":"sections-page","kind":"group","layoutRole":"content","scrollable":true,
         "contentHeight":14,"x":8,"y":5,"w":75,"h":14,"children":[
          {"id":"conditions","kind":"group","layoutRole":"section-header","title":"Matching conditions","iconName":"list-checks",
           "x":8,"y":5,"w":73,"h":1},
          {"id":"matching","kind":"group","title":"Filename matching","bordered":true,
           "x":8,"y":6,"w":73,"h":4,"children":[
             {"id":"mask-label","kind":"text","text":"Filename masks","x":10,"y":7,"w":60,"h":1},
             {"id":"mask","kind":"edit","text":"*.bash","x":10,"y":8,"w":60,"h":1}]},
          {"id":"highlighting","kind":"group","layoutRole":"section-header","title":"Highlighting","iconName":"palette","collapsible":true,
           "x":8,"y":11,"w":73,"h":1,
           "headerControl":{"id":"feature-toggle","kind":"checkbox","text":"Highlighting","state":1,"focused":true,
             "x":10,"y":11,"w":20,"h":1}},
          {"id":"icons","kind":"group","layoutRole":"section-header","title":"Icons","iconName":"images",
           "x":8,"y":12,"w":73,"h":1,
           "headerControl":{"id":"icons-toggle","kind":"checkbox","text":"Icons","state":1}},
          {"id":"sorting","kind":"group","layoutRole":"section-header","title":"Sorting","iconName":"arrow-down-a-z",
           "x":8,"y":13,"w":73,"h":1,
           "headerControl":{"id":"sorting-toggle","kind":"checkbox","text":"Sorting","state":1}},
          {"id":"associations","kind":"group","layoutRole":"section-header","title":"Associations","iconName":"plug",
           "x":8,"y":14,"w":73,"h":1,
           "headerControl":{"id":"associations-toggle","kind":"checkbox","text":"Associations","state":1}},
          {"id":"reset","kind":"group","layoutRole":"section-header","title":"Reset","iconName":"rotate-ccw",
           "x":8,"y":15,"w":73,"h":1}
         ]},
        {"id":"cancel","kind":"button","layoutRole":"cancel","text":"Cancel","x":73,"y":24,"w":10,"h":1}
      ]})").toVariant().toMap();
    auto scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale required for the section header pixel check");

    const QStringList leaves{
        "conditionsGroupTitle", "conditionsSectionRule", "conditionsSectionIcon", "matchingGroupTitle",
        "mask-labelText", "maskEditTextInput", "feature-toggleCheckBoxText",
        "feature-toggleCheckBoxIndicator", "feature-toggleCheckBoxCheckMark", "feature-toggleCheckBoxLeadingIcon",
        "icons-toggleCheckBoxText", "icons-toggleCheckBoxIndicator", "icons-toggleCheckBoxCheckMark", "icons-toggleCheckBoxLeadingIcon",
        "sorting-toggleCheckBoxText", "sorting-toggleCheckBoxIndicator", "sorting-toggleCheckBoxCheckMark", "sorting-toggleCheckBoxLeadingIcon",
        "associations-toggleCheckBoxText", "associations-toggleCheckBoxIndicator", "associations-toggleCheckBoxCheckMark",
        "associations-toggleCheckBoxLeadingIcon", "highlightingSectionRule", "iconsSectionRule", "sortingSectionRule",
        "associationsSectionRule", "resetGroupTitle", "resetSectionRule", "resetSectionIcon", "highlightingSectionDisclosureIcon"
    };
    for (const auto &name : leaves) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY((leaf = visualItem(root, "dialogWidget-" + name)));
        QVERIFY2(leaf->isVisible(), qPrintable(name));
        const auto aligned = [&] {
            const auto origin = leaf->mapToItem(root, QPointF{}) * dpr;
            return qAbs(origin.x() - qRound(origin.x())) < 0.02
                && qAbs(origin.y() - qRound(origin.y())) < 0.02;
        };
        QTRY_VERIFY2(aligned(), qPrintable(name));
        const auto origin = leaf->mapToItem(root, QPointF{});
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
    }
    const QList<QPair<QString, QString>> sectionIcons{
        {"conditionsSectionIcon", "list-checks"}, {"feature-toggleCheckBoxLeadingIcon", "palette"},
        {"icons-toggleCheckBoxLeadingIcon", "images"}, {"sorting-toggleCheckBoxLeadingIcon", "arrow-down-a-z"},
        {"associations-toggleCheckBoxLeadingIcon", "plug"}, {"resetSectionIcon", "rotate-ccw"}
    };
    for (const auto &[leaf, icon] : sectionIcons) {
        auto *image = visualItem(root, "dialogWidget-" + leaf);
        QVERIFY(QFile::exists(":/F4QtHost/icons/lucide/" + icon + ".svg"));
        QVERIFY(image && image->property("source").toUrl().scheme() == "image");
        QTRY_COMPARE(image->property("status").toInt(), 1); // Image.Ready
        QCOMPARE(QSize(qRound(image->width() * dpr), qRound(image->height() * dpr)), QSize(28, 28));
    }
    for (const auto &section : {"conditions", "highlighting", "icons", "sorting", "associations", "reset"}) {
        auto *rule = visualItem(root, "dialogWidget-" + QString(section) + "SectionRule");
        auto *surface = visualItem(root, "dialogWidget-" + QString(section) + "GroupBorder");
        QVERIFY(surface && !surface->isVisible());
        QVERIFY(rule->width() > 100);
        QVERIFY(qAbs(rule->height() * dpr - 1) < 0.02);
    }
    auto *toggle = visualItem(root, "dialogWidget-feature-toggleCheckBox");
    auto *rule = visualItem(root, "dialogWidget-highlightingSectionRule");
    auto *caption = visualItem(root, "dialogWidget-feature-toggleCheckBoxText");
    QVERIFY(toggle && caption);
    QVERIFY(!caption->property("truncated").toBool());
    auto *indicator = visualItem(root, "dialogWidget-feature-toggleCheckBoxIndicator");
    auto *icon = visualItem(root, "dialogWidget-feature-toggleCheckBoxLeadingIcon");
    QVERIFY(indicator && icon);
    QVERIFY(indicator->mapToItem(root, QPointF(indicator->width(), 0)).x() < icon->mapToItem(root, QPointF{}).x());
    QVERIFY(icon->mapToItem(root, QPointF(icon->width(), 0)).x()
        < caption->mapToItem(root, QPointF(caption->property("leftPadding").toReal(), 0)).x());
    QVERIFY(rule->mapToItem(root, QPointF{}).x() > toggle->mapToItem(root, QPointF(toggle->width(), 0)).x());
    verifyGroupCheckBoxFocusFrame(root, "dialogWidget-feature-toggleCheckBox", dpr);
    auto *disclosure = visualItem(root, "dialogWidget-highlightingSectionDisclosure");
    QVERIFY(disclosure);
    QVERIFY(rule->mapToItem(root, QPointF(rule->width(), 0)).x() < disclosure->mapToItem(root, QPointF{}).x());
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(disclosure));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "highlighting" && action.value("action") == "control.toggleSection";
    }));
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(capture.save(QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-sections-175.png")));
    for (const auto &[leaf, name] : sectionIcons) {
        auto *image = visualItem(root, "dialogWidget-" + leaf);
        const QPoint origin = (image->mapToItem(root, QPointF{}) * dpr).toPoint();
        const QRect physicalRect(origin, QSize(28, 28));
        QVERIFY(capture.rect().contains(physicalRect));
        const auto actual = capture.copy(physicalRect);
        const QColor tint(QUrlQuery(image->property("source").toUrl()).queryItemValue("color"));
        QVERIFY(tint.isValid());
        QSvgRenderer svg(":/F4QtHost/icons/lucide/" + name + ".svg");
        QVERIFY(svg.isValid());
        QImage mask(physicalRect.size(), QImage::Format_ARGB32_Premultiplied);
        mask.fill(Qt::transparent);
        {
            QPainter painter(&mask);
            painter.setRenderHint(QPainter::Antialiasing);
            svg.render(&painter, QRectF(mask.rect()));
            painter.setCompositionMode(QPainter::CompositionMode_SourceIn);
            painter.fillRect(mask.rect(), tint);
        }
        QImage expected(physicalRect.size(), QImage::Format_ARGB32_Premultiplied);
        expected.fill(actual.pixelColor(0, 0));
        {
            QPainter painter(&expected);
            painter.drawImage(QPoint(), mask);
        }
        int mismatch = 0;
        for (int y = 0; y < expected.height(); ++y) {
            for (int x = 0; x < expected.width(); ++x) {
                const auto a = actual.pixelColor(x, y);
                const auto b = expected.pixelColor(x, y);
                if (qMax(qAbs(a.red() - b.red()), qMax(qAbs(a.green() - b.green()), qAbs(a.blue() - b.blue()))) > 1)
                    ++mismatch;
            }
        }
        if (mismatch && qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR")) {
            const QDir captures(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR"));
            actual.save(captures.filePath("file-group-" + name + "-actual.png"));
            expected.save(captures.filePath("file-group-" + name + "-expected.png"));
        }
        QVERIFY2(mismatch == 0, qPrintable(QString("%1 at physical (%2, %3) differs from the 28 px SVG raster at %4 pixels")
            .arg(name).arg(origin.x()).arg(origin.y()).arg(mismatch)));
    }
}

void F4OperationsQueueTests::fileGroupConditionCheckboxesAreHeadingsAt175Percent()
{
    const auto dialog = QJsonDocument::fromJson(R"({
      "id":"condition-editor","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"File group","x":2,"y":2,"w":126,"h":24,"children":[
        {"id":"condition-page","kind":"group","layoutRole":"content","scrollable":true,
         "contentHeight":16,"x":4,"y":5,"w":120,"h":16,"children":[
          {"id":"attributes","kind":"group","layoutRole":"condition-group","title":"Attributes",
           "bordered":true,"x":4,"y":5,"w":118,"h":3,
           "headerControl":{"id":"attributes-toggle","kind":"checkbox","text":"Attributes",
                            "state":1,"x":6,"y":5,"w":19,"h":1},
           "children":[
             {"id":"required-label","kind":"text","text":"Required attributes","x":26,"y":6,"w":19,"h":1},
             {"id":"required","kind":"comboBox","dropdownOnly":true,"text":"None",
              "x":46,"y":6,"w":21,"h":1},
             {"id":"excluded-label","kind":"text","text":"Excluded attributes","x":70,"y":6,"w":19,"h":1},
             {"id":"excluded","kind":"comboBox","dropdownOnly":true,"text":"Directory",
              "x":90,"y":6,"w":22,"h":1}]},
          {"id":"size","kind":"group","layoutRole":"condition-group","title":"Size",
           "bordered":true,"x":4,"y":8,"w":118,"h":3,
           "headerControl":{"id":"size-toggle","kind":"checkbox","text":"Size",
                            "state":1,"x":6,"y":8,"w":13,"h":1},
           "children":[
             {"id":"min-label","kind":"text","text":"Minimum bytes","x":26,"y":9,"w":14,"h":1},
             {"id":"min","kind":"edit","text":"1024","x":41,"y":9,"w":10,"h":1},
             {"id":"max-label","kind":"text","text":"Maximum bytes","x":70,"y":9,"w":14,"h":1},
             {"id":"max","kind":"edit","text":"","x":85,"y":9,"w":10,"h":1}]},
          {"id":"timestamps","kind":"group","layoutRole":"condition-group","title":"Date and time",
           "bordered":true,"x":4,"y":11,"w":118,"h":6,
           "headerControl":{"id":"date-toggle","kind":"checkbox","text":"Date and time",
                            "state":1,"x":6,"y":11,"w":15,"h":1},
           "children":[
             {"id":"date-kind-label","kind":"text","text":"Timestamp","x":26,"y":12,"w":9,"h":1},
             {"id":"date-kind","kind":"comboBox","dropdownOnly":true,"text":"Modified",
              "x":36,"y":12,"w":24,"h":1},
             {"id":"relative","kind":"checkbox","text":"Relative dates","state":1,
              "x":72,"y":12,"w":40,"h":1},
             {"id":"after-label","kind":"text","text":"After","x":26,"y":13,"w":5,"h":1},
             {"id":"after","kind":"edit","text":"2d","x":32,"y":13,"w":24,"h":1},
             {"id":"before-label","kind":"text","text":"Before","x":72,"y":13,"w":6,"h":1},
             {"id":"before","kind":"edit","text":"","x":79,"y":13,"w":24,"h":1}]}
         ]}
      ]})").toVariant().toMap();
    auto scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    fixture.window->resize(1500, 780);
    auto *root = fixture.window->contentItem();
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale required for the condition layout pixel check");

    const QStringList leaves{
        "attributes-toggleCheckBoxText", "attributes-toggleCheckBoxIndicator",
        "attributes-toggleCheckBoxCheckMark", "required-labelText", "requiredComboBoxText",
        "requiredComboBoxIndicator", "excluded-labelText", "excludedComboBoxText",
        "excludedComboBoxIndicator", "size-toggleCheckBoxText",
        "size-toggleCheckBoxIndicator", "size-toggleCheckBoxCheckMark", "min-labelText", "max-labelText",
        "minEditTextInput", "maxEditTextInput",
        "date-toggleCheckBoxText", "date-toggleCheckBoxIndicator", "date-toggleCheckBoxCheckMark",
        "date-kind-labelText", "date-kindComboBoxText", "date-kindComboBoxIndicator",
        "relativeCheckBoxText", "relativeCheckBoxIndicator", "relativeCheckBoxCheckMark",
        "after-labelText", "afterEditTextInput",
        "before-labelText", "beforeEditTextInput"
    };
    for (const auto &name : leaves) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY2((leaf = visualItem(root, "dialogWidget-" + name)), qPrintable(name));
        QVERIFY2(leaf->isVisible(), qPrintable(name));
        QTRY_VERIFY2(!leaf->property("truncated").toBool(), qPrintable(name));
        const auto aligned = [&] {
            const auto origin = leaf->mapToItem(root, QPointF{}) * dpr;
            return qAbs(origin.x() - qRound(origin.x())) < 0.02
                && qAbs(origin.y() - qRound(origin.y())) < 0.02;
        };
        QTRY_VERIFY2(aligned(), qPrintable(name));
        const auto origin = leaf->mapToItem(root, QPointF{});
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
    }
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(capture.save(QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-conditions-175.png")));
    const QList<QPair<QString, QString>> groups{
        {"attributes", "attributes-toggle"}, {"size", "size-toggle"}, {"timestamps", "date-toggle"}
    };
    for (const auto &[group, header] : groups) {
        auto *checkbox = visualItem(root, "dialogWidget-" + header + "CheckBox");
        auto *body = visualItem(root, "dialogWidget-" + group + "GroupBorder");
        QVERIFY(checkbox && body && body->isVisible());
        const qreal headerBottom = checkbox->mapToItem(root, QPointF(0, checkbox->height())).y();
        const qreal bodyTop = body->mapToItem(root, QPointF{}).y();
        QVERIFY2(headerBottom <= bodyTop, qPrintable(QString("%1: header bottom %2 px, body top %3 px")
            .arg(header).arg(headerBottom * dpr).arg(bodyTop * dpr)));
    }
    const QList<QPair<QString, QString>> rows{
        {"requiredComboBox", "excludedComboBox"}, {"minEdit", "maxEdit"}
    };
    for (const auto &[leftName, rightName] : rows) {
        auto *left = visualItem(root, "dialogWidget-" + leftName);
        auto *right = visualItem(root, "dialogWidget-" + rightName);
        QVERIFY(left && right);
        QVERIFY(right->mapToItem(root, QPointF{}).x() > left->mapToItem(root, QPointF(left->width(), 0)).x());
        QVERIFY(qAbs(itemCenter(left).y() - itemCenter(right).y()) <= 2);
    }
}

void F4OperationsQueueTests::fileGroupNameAndMaskUseCompactRows()
{
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"id:filegroup-editor:compact","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"File group","x":2,"y":2,"w":126,"h":32,"children":[
        {"id":"compact-page","kind":"group","layoutRole":"content","scrollable":true,
         "contentHeight":10,"x":4,"y":5,"w":120,"h":10,"children":[
          {"id":"identity","kind":"group","title":"Group name",
           "bordered":true,"x":4,"y":5,"w":118,"h":3,
           "headerControl":{"id":"enabled","kind":"checkbox","text":"Group name",
                            "state":1,"focused":true,"x":6,"y":5,"w":14,"h":1},
           "children":[
             {"id":"name","kind":"edit","text":"Source code","x":6,"y":6,"w":113,"h":1}]},
          {"id":"conditions","kind":"group","layoutRole":"section-header","title":"Matching conditions",
           "bordered":false,"x":4,"y":8,"w":118,"h":1},
          {"id":"matching","kind":"group","layoutRole":"condition-group","title":"Name","conditionColumn":true,
           "bordered":true,"x":4,"y":9,"w":57,"h":9,
           "headerControl":{"id":"matching-toggle","kind":"checkbox","text":"Name",
                            "state":1,"x":6,"y":9,"w":22,"h":1},
           "children":[
             {"id":"mask-label","kind":"text","text":"File name masks","focusTarget":"mask","x":6,"y":10,"w":15,"h":1},
             {"id":"mask","kind":"multiLineEdit",
              "text":"*.bash, *.go, /alpha,beta/, *.a-long-extension, *.another-extension, *.last-extension",
              "maskChips":["*.bash","*.go","/alpha,beta/","*.a-long-extension","*.another-extension","*.last-extension"],
              "x":6,"y":11,"w":52,"h":3},
             {"id":"syntax","kind":"radioGroup","choicePresentation":"buttons","title":"Mask syntax",
              "items":["far","legacy-glob"],"selected":0,"compactChoices":true,
              "x":6,"y":14,"w":19,"h":2},
             {"id":"case","kind":"radioGroup","choicePresentation":"buttons","title":"Case matching",
              "items":["insensitive","sensitive","platform"],"selected":0,"compactChoices":true,
              "x":28,"y":14,"w":30,"h":3}]},
          {"id":"attributes","kind":"group","layoutRole":"condition-group","conditionColumn":true,"title":"Attributes",
           "bordered":true,"x":63,"y":9,"w":57,"h":3,
           "headerControl":{"id":"attributes-toggle","kind":"checkbox","text":"Attributes",
                            "state":1,"x":65,"y":9,"w":19,"h":1},
           "children":[
             {"id":"required-label","kind":"text","text":"Required attributes","x":65,"y":10,"w":25,"h":1},
             {"id":"required","kind":"comboBox","dropdownOnly":true,"text":"None",
              "x":65,"y":11,"w":25,"h":1},
             {"id":"excluded-label","kind":"text","text":"Excluded attributes","x":92,"y":10,"w":25,"h":1},
             {"id":"excluded","kind":"comboBox","dropdownOnly":true,"text":"Directory",
              "x":92,"y":11,"w":25,"h":1}]},
          {"id":"size","kind":"group","layoutRole":"condition-group","conditionColumn":true,"title":"Size",
           "bordered":true,"x":63,"y":12,"w":57,"h":3,
           "headerControl":{"id":"size-toggle","kind":"checkbox","text":"Size",
                            "state":1,"x":65,"y":12,"w":13,"h":1},
           "children":[
             {"id":"min-label","kind":"text","text":"Minimum bytes","x":65,"y":13,"w":14,"h":1},
             {"id":"min","kind":"edit","text":"1024","x":80,"y":13,"w":10,"h":1},
             {"id":"max-label","kind":"text","text":"Maximum bytes","x":92,"y":13,"w":14,"h":1},
             {"id":"max","kind":"edit","text":"","x":108,"y":13,"w":10,"h":1}]},
          {"id":"timestamps","kind":"group","layoutRole":"condition-group","conditionColumn":true,"title":"Date and time",
           "bordered":true,"x":63,"y":15,"w":57,"h":6,
           "headerControl":{"id":"date-toggle","kind":"checkbox","text":"Date and time",
                            "state":1,"x":65,"y":15,"w":15,"h":1},
           "children":[
             {"id":"date-kind-label","kind":"text","text":"Timestamp","x":65,"y":16,"w":15,"h":1},
             {"id":"date-kind","kind":"comboBox","dropdownOnly":true,"text":"Modified",
              "x":65,"y":17,"w":15,"h":1},
             {"id":"relative","kind":"checkbox","text":"Relative dates","state":1,
              "x":81,"y":17,"w":16,"h":1},
             {"id":"after-label","kind":"text","text":"After","x":98,"y":16,"w":9,"h":1},
             {"id":"after","kind":"edit","text":"2d","x":98,"y":17,"w":9,"h":1},
             {"id":"before-label","kind":"text","text":"Before","x":108,"y":16,"w":10,"h":1},
             {"id":"before","kind":"edit","text":"","x":108,"y":17,"w":10,"h":1}]}
         ]}
      ]})").toVariant().toMap();
    auto scene = panelScene();
    auto initial = dialog;
    initial["w"] = 60;
    scene.insert("dialogs", QVariantList{initial});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    fixture.window->resize(1500, 920);
    auto *root = fixture.window->contentItem();
    auto *surface = visualItem(root, "semanticDialog-id:filegroup-editor:compact");
    QVERIFY(surface);
    const auto initialGeometry = [&]() -> QVariantMap {
        for (const auto &action : fixture.shell.actions) {
            if (action.value("target") == dialog.value("id") && action.value("action") == "dialog.geometry")
                return action;
        }
        return {};
    };
    QTRY_VERIFY(!initialGeometry().isEmpty());
    QVERIFY(!surface->isVisible());
    dialog["w"] = initialGeometry().value("w");
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    QTRY_VERIFY(surface->isVisible());
    const qreal dpr = fixture.window->devicePixelRatio();
    if (qAbs(dpr - 1.75) >= 0.001)
        QSKIP("175% scale required for the file group pixel check");

    QStringList leaves{
        "enabledCheckBoxText", "enabledCheckBoxIndicator", "enabledCheckBoxCheckMark",
        "nameEditTextInput", "conditionsGroupTitle",
        "matching-toggleCheckBoxText", "matching-toggleCheckBoxIndicator", "matching-toggleCheckBoxCheckMark",
        "mask-labelText", "maskMultiLineChipText-0", "maskMultiLineChipText-1",
        "maskMultiLineChipText-2", "maskMultiLineChipText-3", "maskMultiLineChipText-4", "maskMultiLineChipText-5",
        "syntaxChoiceCaption", "syntaxOption-0Text", "syntaxOption-1Text",
        "caseChoiceCaption", "caseOption-0Text", "caseOption-1Text", "caseOption-2Text",
        "attributes-toggleCheckBoxText", "attributes-toggleCheckBoxIndicator", "attributes-toggleCheckBoxCheckMark",
        "required-labelText", "requiredComboBoxText", "requiredComboBoxIndicator",
        "excluded-labelText", "excludedComboBoxText", "excludedComboBoxIndicator",
        "size-toggleCheckBoxText", "size-toggleCheckBoxIndicator", "size-toggleCheckBoxCheckMark",
        "min-labelText", "max-labelText", "minEditTextInput", "maxEditTextInput",
        "date-toggleCheckBoxText", "date-toggleCheckBoxIndicator", "date-toggleCheckBoxCheckMark",
        "date-kind-labelText", "date-kindComboBoxText", "date-kindComboBoxIndicator",
        "relativeCheckBoxText", "relativeCheckBoxIndicator", "relativeCheckBoxCheckMark",
        "after-labelText", "afterEditTextInput", "before-labelText", "beforeEditTextInput"
    };
    for (const auto &name : leaves) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY2((leaf = visualItem(root, "dialogWidget-" + name)), qPrintable(name));
        QVERIFY2(leaf->isVisible(), qPrintable(name));
        const auto aligned = [&] {
            const auto origin = leaf->mapToItem(root, QPointF{}) * dpr;
            return qAbs(origin.x() - qRound(origin.x())) < 0.02
                && qAbs(origin.y() - qRound(origin.y())) < 0.02;
        };
        QTRY_VERIFY2(aligned(), qPrintable(name));
        const auto origin = leaf->mapToItem(root, QPointF{});
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.001);
        QVERIFY(QLineF(leaf->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.001);
    }
    auto *enabled = visualItem(root, "dialogWidget-enabledCheckBox");
    auto *name = visualItem(root, "dialogWidget-nameEdit");
    auto *mask = visualItem(root, "dialogWidget-maskMultiLine");
    auto *maskLabel = visualItem(root, "dialogWidget-mask-labelText");
    auto *maskEditor = visualItem(root, "dialogWidget-maskMultiLineTextEdit");
    auto *chips = visualItem(root, "dialogWidget-maskMultiLineChips");
    auto *syntax = visualItem(root, "dialogWidget-syntaxOption-0");
    auto *legacy = visualItem(root, "dialogWidget-syntaxOption-1");
    auto *matchCase = visualItem(root, "dialogWidget-caseOption-0");
    auto *caseCaption = visualItem(root, "dialogWidget-caseChoiceCaption");
    auto *conditions = visualItem(root, "dialogWidget-conditionsGroupTitle");
    auto *matching = visualItem(root, "dialogWidget-matching-toggleCheckBox");
    auto *matchingBody = visualItem(root, "dialogWidget-matchingGroupBorder");
    QVERIFY(enabled && name && conditions && matching && matchingBody && mask && syntax && legacy && matchCase && caseCaption);
    QVERIFY(itemCenter(name).y() < itemCenter(conditions).y());
    QVERIFY(itemCenter(conditions).y() < itemCenter(matching).y());
    QVERIFY(matching->mapToItem(root, QPointF(0, matching->height())).y()
            <= matchingBody->mapToItem(root, QPointF{}).y());
    auto *identityBox = visualItem(root, "dialogWidget-identityGroupBorder");
    QVERIFY(identityBox);
    QVERIFY(enabled->mapToItem(root, QPointF(0, enabled->height())).y()
            <= identityBox->mapToItem(root, QPointF{}).y());
    QVERIFY(name->mapToItem(root, QPointF{}).y() >= identityBox->mapToItem(root, QPointF{}).y());
    QVERIFY(!visualItem(root, "dialogWidget-name-labelText"));
    verifyGroupCheckBoxFocusFrame(root, "dialogWidget-enabledCheckBox", dpr);
    QVERIFY(qAbs(itemCenter(matchCase).y() - itemCenter(syntax).y()) <= 1);
    QVERIFY(matchCase->mapToItem(root, QPointF{}).x()
            >= legacy->mapToItem(root, QPointF(legacy->width(), 0)).x());
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(capture.save(QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-name-mask-175.png")));
    QVERIFY(maskLabel && maskEditor && chips);
    QVERIFY(maskLabel->mapToItem(root, QPointF{}).y() < mask->mapToItem(root, QPointF{}).y());
    const qreal gapAbove = caseCaption->mapToItem(root, QPointF{}).y()
        - mask->mapToItem(root, QPointF(0, mask->height())).y();
    const qreal gapBelow = matchCase->mapToItem(root, QPointF{}).y()
        - caseCaption->mapToItem(root, QPointF(0, caseCaption->height())).y();
    QVERIFY(gapAbove >= 9);
    QVERIFY(gapBelow <= 7 && gapBelow >= 4);
    auto *optionText = visualItem(root, "dialogWidget-caseOption-0Text");
    QVERIFY(optionText);
    const qreal horizontalInset = optionText->mapToItem(matchCase, QPointF{}).x();
    const qreal verticalInset = optionText->mapToItem(matchCase, QPointF{}).y();
    QVERIFY(horizontalInset >= 8);
    QVERIFY(verticalInset >= 7);
    QVERIFY(mask->height() > 2 * syntax->height());
    QVERIFY(chips->isVisible() && !maskEditor->isVisible());
    auto *attributes = visualItem(root, "dialogWidget-attributesGroupBorder");
    auto *size = visualItem(root, "dialogWidget-sizeGroupBorder");
    auto *dates = visualItem(root, "dialogWidget-timestampsGroupBorder");
    QVERIFY(attributes && size && dates);
    QVERIFY(qAbs(attributes->mapToScene(QPointF{}).y() - matchingBody->mapToScene(QPointF{}).y()) < 0.02);
    QVERIFY(attributes->mapToScene(QPointF{}).x() > matchingBody->mapToScene(QPointF(matchingBody->width(), 0)).x());
    const auto bottom = [](QQuickItem *item) { return item->mapToScene(QPointF(0, item->height())).y(); };
    QVERIFY(qAbs(bottom(matchingBody) - bottom(dates)) < 0.02);
    for (const auto &[upper, heading] : QList<QPair<QQuickItem *, QString>>{
             {attributes, "size"}, {size, "timestamps"}}) {
        auto *next = visualItem(root, "dialogWidget-" + heading + "GroupTitleBackground");
        const qreal gap = next ? next->mapToScene(QPointF{}).y() - bottom(upper) : -1;
        QVERIFY2(gap >= 7 && gap <= 9, qPrintable(QString::number(gap)));
    }
    const auto pairedControls = [&](const QString &firstId, const QString &secondId) {
        auto *first = visualItem(root, "dialogWidget-" + firstId);
        auto *second = visualItem(root, "dialogWidget-" + secondId);
        return first && second && qAbs(itemCenter(first).y() - itemCenter(second).y()) <= 2
            && itemCenter(first).x() < itemCenter(second).x();
    };
    QVERIFY(pairedControls("date-kindComboBox", "relativeCheckBox"));
    QVERIFY(pairedControls("relativeCheckBox", "afterEdit"));
    QVERIFY(pairedControls("afterEdit", "beforeEdit"));
    QCOMPARE(visualItem(root, "dialogWidget-maskMultiLineChipText-2")->property("text").toString(), QString("/alpha,beta/"));
    QVERIFY(syntax->property("highlighted").toBool());
    QVERIFY(!legacy->property("highlighted").toBool());
    QCOMPARE(legacy->property("text").toString(), QString("legacy-glob"));
    QCOMPARE(matchCase->property("text").toString(), QString("insensitive"));
    const QStringList cornerNames{"topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius"};
    for (const auto &[id, count] : QList<QPair<QString, int>>{{"syntax", 2}, {"case", 3}}) {
        auto *outline = visualItem(root, "dialogWidget-" + id + "ChoiceOutline");
        QVERIFY(outline && outline->isVisible());
        const auto edge = [&](QQuickItem *item, qreal x, qreal y) {
            return item->mapToItem(root, QPointF(x, y)) * dpr;
        };
        const auto outlineTopLeft = edge(outline, 0, 0);
        const auto outlineBottomRight = edge(outline, outline->width(), outline->height());
        for (const auto &point : {outlineTopLeft, outlineBottomRight}) {
            QVERIFY(qAbs(point.x() - qRound(point.x())) < 0.02);
            QVERIFY(qAbs(point.y() - qRound(point.y())) < 0.02);
        }
        qreal previousRight = outlineTopLeft.x();
        for (int i = 0; i < count; ++i) {
            const auto optionName = "dialogWidget-" + id + "Option-" + QString::number(i);
            auto *option = visualItem(root, optionName);
            auto *background = visualItem(root, optionName + "Background");
            QVERIFY(option && background);
            const auto topLeft = edge(option, 0, 0);
            const auto bottomRight = edge(option, option->width(), option->height());
            QVERIFY2(qAbs(topLeft.x() - previousRight) < 0.02,
                     qPrintable(QString("%1: %2 physical pixels between segments").arg(optionName).arg(topLeft.x() - previousRight)));
            QCOMPARE(topLeft.y(), outlineTopLeft.y());
            QCOMPARE(bottomRight.y(), outlineBottomRight.y());
            for (qsizetype corner = 0; corner < cornerNames.size(); ++corner) {
                const bool outer = (corner % 2 == 0) ? i == 0 : i == count - 1;
                const auto radius = background->property(cornerNames[corner].toLatin1().constData());
                QVERIFY(radius.isValid());
                QCOMPARE(radius.toReal() > 0, outer);
            }
            previousRight = bottomRight.x();
        }
        QVERIFY(qAbs(previousRight - outlineBottomRight.x()) < 0.02);
    }
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(legacy));
    QVERIFY(std::ranges::any_of(fixture.shell.actions, [](const QVariantMap &action) {
        return action.value("target") == "syntax" && action.value("action") == "control.select"
            && action.value("index").toInt() == 1;
    }));
    QList<QPointF> positions;
    for (const auto &leaf : leaves)
        positions.append(visualItem(root, "dialogWidget-" + leaf)->mapToItem(root, QPointF{}));
    auto page = dialog.value("children").toList().first().toMap();
    auto groups = page.value("children").toList();
    auto matchingGroup = groups[2].toMap();
    auto heading = matchingGroup.value("headerControl").toMap();
    auto fields = matchingGroup.value("children").toList();
    auto maskField = fields[1].toMap();
    const auto blankSpace = mask->mapToItem(root, QPointF(3, mask->height() - 3)).toPoint();
    const auto maskFocusRequests = [&] {
        return std::ranges::count_if(fixture.shell.actions, [](const QVariantMap &action) {
            return action.value("target") == "mask" && action.value("action") == "control.focus";
        });
    };
    auto *firstChip = visualItem(root, "dialogWidget-maskMultiLineChipText-0");
    QVERIFY(firstChip);
    for (const auto &point : {itemCenter(maskLabel), itemCenter(firstChip), blankSpace}) {
        const auto previousRequests = maskFocusRequests();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, point);
        QCOMPARE(maskFocusRequests(), previousRequests + 1);
    }
    const auto updateMaskScene = [&] {
        fields[1] = maskField;
        matchingGroup["children"] = fields;
        groups[2] = matchingGroup;
        page["children"] = groups;
        dialog["children"] = QVariantList{page};
        scene.insert("dialogs", QVariantList{dialog});
        fixture.shell.setScene(scene);
    };
    maskField["focused"] = true;
    updateMaskScene();
    QTRY_VERIFY(maskEditor->isVisible() && !chips->isVisible());
    QTRY_VERIFY(maskEditor->property("lineCount").toInt() > 1);
    QCOMPARE(maskEditor->property("text"), maskField["text"]);
    const auto editorOrigin = maskEditor->mapToItem(root, QPointF{});
    QVERIFY(QLineF(maskEditor->mapToItem(root, QPointF(1, 0)) - editorOrigin, QPointF(1, 0)).length() < 0.001);
    QVERIFY(QLineF(maskEditor->mapToItem(root, QPointF(0, 1)) - editorOrigin, QPointF(0, 1)).length() < 0.001);
    QVERIFY(qAbs(editorOrigin.x() * dpr - qRound(editorOrigin.x() * dpr)) < 0.02);
    QVERIFY(qAbs(editorOrigin.y() * dpr - qRound(editorOrigin.y() * dpr)) < 0.02);
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(fixture.window->grabWindow().save(
            QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-matching-editor-175.png")));
    maskField["focused"] = false;
    heading["state"] = 0;
    matchingGroup["headerControl"] = heading;
    updateMaskScene();

    QTRY_VERIFY(!mask->isEnabled() && !syntax->isEnabled() && !matchCase->isEnabled()
                && matching->isEnabled());
    for (const auto &id : {"mask", "syntax", "case", "name", "enabled"})
        QVERIFY(!visualItem(root, QString("dialogWidget-%1RootReset").arg(id)));
    const auto disabledRequests = maskFocusRequests();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(maskLabel));
    QCOMPARE(maskFocusRequests(), disabledRequests);
    const auto mutedText = fixture.window->property("mutedText").value<QColor>();
    QVERIFY(mutedText.isValid());
    for (const QString &label : {QString("mask-labelText"), QString("syntaxChoiceCaption"), QString("caseChoiceCaption")}) {
        auto *leaf = visualItem(root, "dialogWidget-" + label);
        QVERIFY(leaf);
        QTRY_COMPARE(leaf->property("color").value<QColor>(), mutedText);
        QCOMPARE(leaf->opacity(), 0.55);
    }
    for (qsizetype i = 0; i < leaves.size(); ++i) {
        if (leaves[i] == "matching-toggleCheckBoxCheckMark")
            continue;
        auto *leaf = visualItem(root, "dialogWidget-" + leaves[i]);
        QVERIFY2(leaf && leaf->isVisible(), qPrintable(leaves[i]));
        QVERIFY2(QLineF(positions[i], leaf->mapToItem(root, QPointF{})).length() < 0.001,
                 qPrintable(leaves[i]));
    }
    if (qEnvironmentVariableIsSet("F4_QUEUE_CAPTURE_DIR"))
        QVERIFY(fixture.window->grabWindow().save(
            QDir(qEnvironmentVariable("F4_QUEUE_CAPTURE_DIR")).filePath("file-group-matching-disabled-175.png")));
}

void F4OperationsQueueTests::settingsLocalScrollSurvivesLateLayout_data()
{
    QTest::addColumn<int>("initialScroll");
    QTest::addColumn<bool>("pixelWheel");
    QTest::newRow("mouse-wheel") << 0 << false;
    QTest::newRow("touchpad-restored-page") << 20 << true;
}

void F4OperationsQueueTests::settingsLocalScrollSurvivesLateLayout()
{
    QFETCH(int, initialScroll);
    QFETCH(bool, pixelWheel);
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"scroll-editor","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"File group","x":4,"y":2,"w":90,"h":28,"children":[
        {"id":"scroll-page","kind":"group","layoutRole":"content","scrollable":true,
         "scrollTop":0,"contentHeight":100,"x":8,"y":8,"w":80,"h":15,"children":[
          {"id":"first","kind":"edit","text":"Group name","x":10,"y":8,"w":60,"h":1},
          {"id":"last","kind":"edit","text":"Last property","x":10,"y":100,"w":60,"h":1}]},
        {"id":"ok","kind":"button","layoutRole":"accept","text":"OK","x":60,"y":27,"w":10,"h":1},
        {"id":"cancel","kind":"button","layoutRole":"cancel","text":"Cancel","x":73,"y":27,"w":10,"h":1}]
    })").toVariant().toMap();
    auto children = dialog.value("children").toList();
    auto page = children[0].toMap();
    page.insert("scrollTop", initialScroll);
    auto fields = page.value("children").toList();
    for (int i = 0; i < 35; ++i)
        fields.append(QVariantMap{{"id", QString("property-%1").arg(i)}, {"kind", "edit"},
            {"text", "Property"}, {"x", 10}, {"y", 12 + i * 2}, {"w", 60}, {"h", 1}});
    page.insert("children", fields);
    children[0] = page;
    dialog.insert("children", children);
    auto scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *viewport = nullptr;
    QTRY_VERIFY((viewport = visualItem(root, "dialogWidget-scroll-pageViewport")));
    QTRY_VERIFY(viewport->property("contentHeight").toReal() > viewport->height() + 300);
    if (initialScroll > 0)
        QTRY_VERIFY(viewport->property("contentY").toReal() > 0);
    QVERIFY(viewport->setProperty("contentY", 180.0));
    // First-open layout settles after input begins (e.g. controls/fonts load).
    // Go still advertises scrollTop=0 until the deferred native scroll commit.
    fields.append(QVariantMap{{"id", "late-property"}, {"kind", "edit"},
        {"text", "Late property"}, {"x", 10}, {"y", 110}, {"w", 60}, {"h", 1}});
    page.insert("children", fields);
    page.insert("contentHeight", 110);
    children[0] = page;
    dialog.insert("children", children);
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    QTest::qWait(150);
    QCOMPARE(viewport->property("contentY").toReal(), 180.0);
    auto *controller = visualItem(root, "dialogWidget-scroll-pageViewportController");
    QVERIFY(controller);
    fixture.shell.clearActions();
    QVERIFY(QMetaObject::invokeMethod(controller, "commitScroll"));
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    const auto scrollAction = fixture.shell.actions.constLast();
    QCOMPARE(scrollAction.value("action").toString(), QStringLiteral("control.scroll"));
    const int earlierScroll = scrollAction.value("value").toInt();
    QVERIFY(earlierScroll != initialScroll);

    // Wheel input can advance the page while the core is still publishing
    // its acknowledgement of the previous position.
    const qreal laterPosition = 280.0;
    QVERIFY(viewport->setProperty("contentY", laterPosition));
    page.insert("scrollTop", earlierScroll);
    children[0] = page;
    dialog.insert("children", children);
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    QTRY_COMPARE(controller->property("semanticScroll").toInt(), earlierScroll);
    QTest::qWait(50);
    QCOMPARE(viewport->property("contentY").toReal(), laterPosition);

    // An explicit keyboard scroll still applies.
    page.insert("scrollTop", 40);
    page.insert("scrollRequest", QVariantMap{{"revision", 1}, {"top", 40}});
    children[0] = page;
    dialog.insert("children", children);
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    const qreal expected = qRound(controller->property("maximumScroll").toReal() * 40
                                  / controller->property("semanticMaximum").toReal()
                                  * fixture.window->devicePixelRatio()) / fixture.window->devicePixelRatio();
    // Flickable.pixelAligned rounds the logical scroll offset; native leaves
    // receive their separate physical-pixel translation in the content item.
    QTRY_VERIFY(qAbs(viewport->property("contentY").toReal() - expected) <= 1.0);

    const auto point = itemCenter(viewport);
    const qreal bottom = controller->property("maximumScroll").toReal();
    const auto scrollDown = [&] {
        if (pixelWheel) {
            sendPixelWheel(fixture.window, point, -120);
        } else {
            QWheelEvent event(point, fixture.window->mapToGlobal(point), {}, QPoint(0, -120),
                              Qt::NoButton, Qt::NoModifier, Qt::NoScrollPhase, false);
            QCoreApplication::sendEvent(fixture.window, &event);
        }
    };
    for (int i = 0; i < 60 && viewport->property("contentY").toReal() < bottom - 1; ++i) {
        scrollDown();
        QTest::qWait(20);
    }
    for (int i = 0; i < 3; ++i)
        scrollDown();
    QVERIFY2(viewport->property("contentY").toReal() >= bottom - 1,
             qPrintable(QString("Wheel stopped at %1 of %2").arg(viewport->property("contentY").toReal()).arg(bottom)));
    QTest::qWait(180);
    page.insert("scrollTop", 0);
    children[0] = page;
    dialog.insert("children", children);
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    QTRY_COMPARE(controller->property("semanticScroll").toInt(), 0);
    QTest::qWait(180);
    QVERIFY(viewport->property("contentY").toReal() >= bottom - 1);
}

void F4OperationsQueueTests::settingsDecorationsAndViewportStayPixelAligned()
{
    // Same nested, absolute-coordinate contract as settingsViewport.SemanticNode.
    auto dialog = QJsonDocument::fromJson(R"({
        "id":"settings-center","kind":"dialog","title":"Settings",
        "x":0,"y":0,"w":100,"h":27,"showClose":true,"children":[
          {"id":"search-label","kind":"text","text":"Search:","x":2,"y":1,"w":24,"h":1},
          {"id":"search","kind":"edit","text":"","x":2,"y":2,"w":24,"h":1},
          {"id":"category-title","kind":"text","text":"Appearance & language","x":29,"y":1,"w":67,"h":1},
          {"id":"categories","kind":"listBox","items":["Appearance & language","Startup & profile"],"x":2,"y":4,"w":24,"h":17},
          {"id":"settings-page","kind":"group","x":29,"y":3,"w":67,"h":15,
           "scrollable":true,"scrollTop":0,"contentHeight":38,"children":[
             {"id":"language","kind":"group","title":"Language","bordered":true,"x":29,"y":3,"w":64,"h":5,"children":[
               {"id":"interface-label","kind":"text","text":"Interface language","x":31,"y":4,"w":19,"h":1},
               {"id":"interface","kind":"comboBox","items":["English"],"text":"English","selected":0,"x":51,"y":4,"w":40,"h":1},
               {"id":"help-label","kind":"text","text":"Help language","x":31,"y":5,"w":14,"h":1},
               {"id":"help","kind":"comboBox","items":["English"],"text":"English","selected":0,"x":46,"y":5,"w":45,"h":1},
               {"id":"local","kind":"checkbox","text":"Use local translations","x":31,"y":6,"w":60,"h":1}
             ]},
             {"id":"colors","kind":"group","title":"Colors","bordered":true,"x":29,"y":10,"w":64,"h":4,"children":[
               {"id":"theme-label","kind":"text","text":"Color theme","x":31,"y":11,"w":12,"h":1},
               {"id":"theme","kind":"comboBox","items":["Modern"],"text":"Modern","selected":0,"x":44,"y":11,"w":47,"h":1},
               {"id":"contrast","kind":"checkbox","text":"Correct low contrast","state":1,"x":31,"y":12,"w":60,"h":1}
             ]},
             {"id":"font","kind":"group","title":"Font","bordered":true,"x":29,"y":16,"w":64,"h":4,"children":[
               {"id":"font-label","kind":"text","text":"Graphical font","x":31,"y":17,"w":15,"h":1},
               {"id":"font-edit","kind":"edit","text":"Monospace","x":47,"y":17,"w":44,"h":1}
             ]},
             {"id":"titles","kind":"group","title":"Titles and menus","bordered":true,"x":29,"y":29,"w":64,"h":5,"children":[
               {"id":"title-label","kind":"text","text":"Window title template","x":31,"y":30,"w":60,"h":1},
               {"id":"title-edit","kind":"edit","text":"f4 %Ver %Platform","x":31,"y":31,"w":60,"h":1}
             ]},
             {"id":"presentation","kind":"group","title":"Presentation","bordered":true,"x":29,"y":35,"w":64,"h":4,"children":[
               {"id":"inherit-terminal-history","kind":"checkbox","text":"Inherit terminal history in new tabs","state":1,"x":31,"y":36,"w":60,"h":1}
             ]}
           ]},
          {"id":"description","kind":"group","x":29,"y":19,"w":67,"h":4,"scrollable":true,"scrollTop":0,"contentHeight":3,"children":[
            {"id":"description-text","kind":"text","text":"Select a setting to read what it does. This explanation wraps to the available GUI width, without terminal line breaks.","wrapText":true,"x":29,"y":19,"w":65,"h":3}
          ]},
          {"id":"apply","kind":"button","text":"Apply","x":62,"y":25,"w":10,"h":1},
          {"id":"ok","kind":"button","text":"Ok","x":74,"y":25,"w":8,"h":1},
          {"id":"cancel","kind":"button","text":"Cancel","x":84,"y":25,"w":12,"h":1}
        ]})").toVariant().toMap();
    QVariantMap scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QQuickItem *root = fixture.window->contentItem();
    QQuickItem *viewport = nullptr;
    QTRY_VERIFY((viewport = visualItem(root, "dialogWidget-settings-pageViewport")));
    QVERIFY(viewport->clip());
    QVERIFY(viewport->property("contentHeight").toReal() > viewport->height());
    // List entries still use mnemonic markup; passive captions use plain text.
    auto *categoryEntry = visualItem(root, "dialogWidget-categoriesListItemText-0");
    QVERIFY(categoryEntry);
    QCOMPARE(categoryEntry->property("textFormat").toInt(), 4); // Text.StyledText
    QVERIFY(!fixture.window->grabWindow().isNull());
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto checkGrid = [&](QQuickItem *item) {
        // Cocoa may return the preceding frame from grabWindow. Alignment
        // settles in afterAnimating, so observe the geometry after that pass.
        const auto aligned = [&](bool horizontal) {
            const auto point = item->mapToItem(root, QPointF{}) * dpr;
            const auto value = horizontal ? point.x() : point.y();
            return qAbs(value - qRound(value)) < 0.02;
        };
        const QPointF origin = item->mapToItem(root, QPointF{});
        const QPointF physical = origin * dpr;
        QTRY_VERIFY2(aligned(true),
                 qPrintable(QString("%1 x=%2 px").arg(item->objectName()).arg(physical.x(), 0, 'f', 4)));
        QTRY_VERIFY2(aligned(false),
                 qPrintable(QString("%1 y=%2 px").arg(item->objectName()).arg(physical.y(), 0, 'f', 4)));
        const auto settledOrigin = item->mapToItem(root, QPointF{});
        QVERIFY(QLineF(item->mapToItem(root, QPointF(1, 0)) - settledOrigin, QPointF(1, 0)).length() < 0.0001);
        QVERIFY(QLineF(item->mapToItem(root, QPointF(0, 1)) - settledOrigin, QPointF(0, 1)).length() < 0.0001);
    };
    const QStringList names{
        "search-labelText", "searchEditTextInput", "category-titleText",
        "categoriesListItemText-0", "categoriesListItemText-1",
        "languageGroupTitle", "interface-labelText", "interfaceComboBoxText",
        "help-labelText", "helpComboBoxText", "localCheckBoxText",
        "interfaceComboBoxIndicator", "helpComboBoxIndicator", "localCheckBoxIndicator",
        "colorsGroupTitle", "theme-labelText", "themeComboBoxText", "contrastCheckBoxText",
        "themeComboBoxIndicator", "contrastCheckBoxIndicator", "contrastCheckBoxCheckMark",
        "fontGroupTitle", "font-labelText", "font-editEditTextInput",
        "titlesGroupTitle", "title-labelText", "title-editEditTextInput",
        "presentationGroupTitle", "inherit-terminal-historyCheckBoxText",
        "inherit-terminal-historyCheckBoxIndicator", "inherit-terminal-historyCheckBoxCheckMark",
        "description-textText", "applyButtonText", "okButtonText", "cancelButtonText"
    };
    for (const auto &name : names) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY2((leaf = visualItem(root, "dialogWidget-" + name)), qPrintable(name));
        checkGrid(leaf);
    }
    for (const auto &group : {"language", "colors", "font", "titles", "presentation"}) {
        for (const auto &part : {"GroupBorder", "GroupTitleBackground"}) {
            auto *item = visualItem(root, "dialogWidget-" + QString(group) + part);
            QVERIFY(item);
            checkGrid(item);
            QVERIFY2(qAbs(item->width() * dpr - qRound(item->width() * dpr)) < 0.02, qPrintable(item->objectName() + " width"));
            QVERIFY2(qAbs(item->height() * dpr - qRound(item->height() * dpr)) < 0.02, qPrintable(item->objectName() + " height"));
        }
    }
    const QList<QPair<QString, QStringList>> groupFields{
        {"language", {"interface-label", "interface", "help-label", "help", "local"}},
        {"colors", {"theme-label", "theme", "contrast"}},
        {"font", {"font-label", "font-edit"}},
        {"titles", {"title-label", "title-edit"}},
        {"presentation", {"inherit-terminal-history"}}
    };
    for (const auto &[group, fields] : groupFields) {
        auto *surface = visualItem(root, "dialogWidget-" + group + "GroupBorder");
        qreal top = std::numeric_limits<qreal>::max(), bottom = 0;
        for (const auto &field : fields) {
            auto *item = visualItem(root, "dialogWidget-" + field + "Root");
            QVERIFY(item);
            const auto rect = item->mapRectToItem(surface, QRectF(0, 0, item->width(), item->height()));
            top = qMin(top, rect.top());
            bottom = qMax(bottom, rect.bottom());
        }
        const auto bottomPadding = surface->height() - bottom;
        QVERIFY2(qAbs(top - bottomPadding) <= 1 / dpr,
            qPrintable(QString("%1 padding top=%2 bottom=%3").arg(group).arg(top).arg(bottomPadding)));
        QVERIFY(qAbs(top - 12) <= 1 / dpr);
    }
    QCOMPARE(visualItem(root, "dialogWidget-category-titleText")->property("text").toString(), QString("Appearance & language"));
    auto *firstLabel = visualItem(root, "dialogWidget-interface-labelText");
    auto *firstControl = visualItem(root, "dialogWidget-interfaceComboBox");
    QVERIFY(firstControl->mapToItem(root, QPointF{}).x()
            >= firstLabel->mapToItem(root, QPointF{}).x() + firstLabel->property("contentWidth").toReal());
    QImage capture;
    QTRY_VERIFY(!(capture = fixture.window->grabWindow()).isNull());
    if (!qEnvironmentVariable("F4_SETTINGS_CAPTURE").isEmpty())
        QVERIFY(capture.save(qEnvironmentVariable("F4_SETTINGS_CAPTURE")));
    const qreal fixedHelpY = visualItem(root, "dialogWidget-descriptionRoot")->mapToItem(root, QPointF{}).y();
    // A fractional scroll position must not blur resting text descendants.
    QVERIFY(viewport->setProperty("contentY", 137.3));
    QCoreApplication::processEvents();
    // Hover/help updates can publish the same semantic page while the local
    // touchpad position has not yet been committed to the core.
    const qreal localScroll = viewport->property("contentY").toReal();
    fixture.shell.setScene(scene);
    QTest::qWait(150);
    QCOMPARE(viewport->property("contentY").toReal(), localScroll);
    QVERIFY(!fixture.window->grabWindow().isNull());
    for (const auto &name : names)
        checkGrid(visualItem(root, "dialogWidget-" + name));
    QCOMPARE(visualItem(root, "dialogWidget-descriptionRoot")->mapToItem(root, QPointF{}).y(), fixedHelpY);
    fixture.shell.clearActions();
    auto *controller = visualItem(root, "dialogWidget-settings-pageViewportController");
    QVERIFY(controller);
    QVERIFY(QMetaObject::invokeMethod(controller, "commitScroll"));
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.constLast().value("target").toString(), QString("settings-page"));
    QCOMPARE(fixture.shell.actions.constLast().value("action").toString(), QString("control.scroll"));
    QVERIFY(fixture.shell.actions.constLast().value("value").toInt() > 0);
    QVERIFY(viewport->setProperty("contentY", viewport->property("contentHeight").toReal() - viewport->height()));
    QCoreApplication::processEvents();
    QVERIFY(!fixture.window->grabWindow().isNull());
    for (const auto &name : names)
        checkGrid(visualItem(root, "dialogWidget-" + name));
    QTRY_VERIFY(!(capture = fixture.window->grabWindow()).isNull());
    if (!qEnvironmentVariable("F4_SETTINGS_CAPTURE").isEmpty())
        QVERIFY(capture.save(qEnvironmentVariable("F4_SETTINGS_CAPTURE") + ".scrolled.png"));
}


void F4OperationsQueueTests::settingsResizeKeepsChromeAndScrollsContent_data()
{
    QTest::addColumn<bool>("recordOnly");
    QTest::newRow("full-settings") << false;
    QTest::newRow("single-connection") << true;
}

void F4OperationsQueueTests::settingsResizeKeepsChromeAndScrollsContent()
{
    QFETCH(bool, recordOnly);
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({
        "id":"resizable-settings","kind":"dialog","layout":"settings","title":"Settings","x":0,"y":0,"w":110,"h":50,
        "children":[
          {"id":"search-label","kind":"text","layoutRole":"search-label","text":"Search:","x":2,"y":1,"w":25,"h":1},
          {"id":"search","kind":"edit","layoutRole":"search","x":2,"y":2,"w":25,"h":1},
          {"id":"categories","kind":"table","layoutRole":"navigation","x":2,"y":4,"w":25,"h":43,"showHeader":false,"columns":[{"width":0}],"rows":[{"cells":["Appearance"]},{"cells":["Editor"]}],"itemIcons":["palette","file-pen-line"]},
          {"id":"category-title","kind":"text","layoutRole":"content-title","text":"Appearance","x":29,"y":1,"w":50,"h":1},
          {"id":"page","kind":"group","layoutRole":"content","scrollable":true,"x":29,"y":3,"w":50,"h":43,"contentHeight":70,"children":[
            {"id":"box","kind":"group","bordered":true,"title":"Language","x":29,"y":3,"w":49,"h":5,"children":[
              {"id":"language","kind":"edit","text":"English","x":31,"y":4,"w":40,"h":1}]},
            {"id":"last","kind":"edit","text":"Last setting","x":31,"y":68,"w":40,"h":1}]},
          {"id":"help","kind":"group","layoutRole":"description","scrollable":true,"x":82,"y":1,"w":26,"h":45,"contentHeight":6,"children":[{"id":"help-text","kind":"text","wrapText":true,"text":"Description of the selected setting.","x":82,"y":1,"w":26,"h":3}]},
          {"id":"apply","kind":"button","layoutRole":"apply","text":"Apply","x":78,"y":48,"w":9,"h":1},
          {"id":"ok","kind":"button","layoutRole":"accept","text":"OK","x":88,"y":48,"w":9,"h":1},
          {"id":"cancel","kind":"button","layoutRole":"cancel","text":"Cancel","x":98,"y":48,"w":9,"h":1}
        ]})").toVariant().toMap();
    auto children = dialog.value("children").toList();
    if (recordOnly) {
        for (int index = 0; index < 4; ++index) {
            auto child = children[index].toMap();
            child.insert("visible", false);
            children[index] = child;
        }
    }
    auto pageModel = children[4].toMap();
    auto settings = pageModel.value("children").toList();
    for (int row = 10; row < 65; row += 2)
        settings.append(QVariantMap{{"id", QString("setting-%1").arg(row)}, {"kind", "edit"},
            {"text", QString("Setting %1").arg(row)}, {"x", 31}, {"y", row}, {"w", 40}, {"h", 1}});
    pageModel.insert("children", settings);
    children[4] = pageModel;
    dialog.insert("children", children);
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    fixture.window->resize(1500, 1000);
    QTest::qWait(100);
    auto *root = fixture.window->contentItem();
    auto *surface = visualItem(root, "semanticDialog-resizable-settings");
    QVERIFY(surface);
    for (const auto &size : {QSize(680, 520), QSize(1250, 830), QSize(730, 610)}) {
        auto helpModel = children[5].toMap();
        auto helpChildren = helpModel.value("children").toList();
        auto paragraph = helpChildren[0].toMap();
        paragraph.insert("h", size.width() > 1000 ? 18 : 3);
        paragraph.insert("text", size.width() > 1000
            ? "Help language\n\nChoose the built-in help language independently of the interface language.\n\nTakes effect: live"
            : "Description of the selected setting.");
        helpChildren[0] = paragraph;
        helpModel.insert("children", helpChildren);
        helpModel.insert("title", size.width() > 1000 ? "Help language" : "");
        helpModel.insert("helpTiming", size.width() > 1000 ? "Takes effect: live" : "");
        children[5] = helpModel;
        auto sectionTitle = children[3].toMap();
        sectionTitle.insert("text", size.width() > 1000 ? "Editor" : "Appearance");
        children[3] = sectionTitle;
        dialog.insert("children", children);
        scene.insert("dialogs", QVariantList{dialog});
        fixture.shell.setScene(scene);
        QVERIFY(QMetaObject::invokeMethod(surface, "setUserGeometry", Q_ARG(QVariant, 30), Q_ARG(QVariant, 50),
            Q_ARG(QVariant, size.width()), Q_ARG(QVariant, size.height())));
        QTest::qWait(80);
        QVERIFY(!fixture.window->grabWindow().isNull());
        auto *button = visualItem(root, "dialogWidget-cancelButton");
        QVERIFY(button);
        const auto buttonRect = button->mapRectToItem(surface, QRectF(0, 0, button->width(), button->height()));
        QVERIFY2(buttonRect.bottom() <= surface->height()-8 && buttonRect.top() >= surface->height()-100,
                 qPrintable(QString("footer bottom=%1, dialog height=%2").arg(buttonRect.bottom()).arg(surface->height())));
        auto *body = visualItem(root, "settingsDialogBody");
        QVERIFY(body && body->isVisible());
        QCOMPARE(body->property("hasNavigation").toBool(), !recordOnly);
        auto *windowTitle = visualItem(root, "semanticDialogTitle");
        auto *contentTitle = visualItem(root, "dialogWidget-category-titleRoot");
        QVERIFY(windowTitle && contentTitle);
        QCOMPARE(windowTitle->property("text").toString(), recordOnly
            ? QString("Settings") : sectionTitle.value("text").toString());
        QVERIFY(!contentTitle->isVisible());
        if (!recordOnly)
            QCOMPARE(body->property("pageY").toReal(), body->property("gap").toReal());
        if (recordOnly) {
            QCOMPARE(body->property("mainX").toReal(), 0.0);
            QCOMPARE(body->property("pageY").toReal(), 0.0);
        }
        auto *page = visualItem(root, "dialogWidget-pageViewport");
        QVERIFY(page && page->height() > 50);
        QVERIFY(page->property("contentHeight").toReal() > page->height());
        auto *box = visualItem(root, "dialogWidget-boxRoot");
        auto *field = visualItem(root, "dialogWidget-languageRoot");
        QVERIFY(box && field);
        auto *groupSurface = visualItem(root, "dialogWidget-boxGroupBorder");
        auto *groupHeading = visualItem(root, "dialogWidget-boxGroupTitle");
        QVERIFY(groupSurface && groupHeading);
        QCOMPARE(groupSurface->property("testBorderWidth").toReal(), 0.0);
        auto *headingRow = visualItem(root, "dialogWidget-boxGroupTitleBackground");
        QVERIFY(headingRow);
        const auto captionGap = -headingRow->mapToItem(groupSurface, QPointF()).y() - headingRow->height();
        QVERIFY2(qAbs(captionGap - 4) < 1, qPrintable(QString("caption gap=%1").arg(captionGap)));
        const auto inset = field->mapToItem(groupSurface, QPointF()).y();
        QVERIFY2(qAbs(inset - 12) < 1, qPrintable(QString("group top inset=%1").arg(inset)));
        auto *helpRoot = visualItem(root, "dialogWidget-helpRoot");
        auto *helpText = visualItem(root, "dialogWidget-help-textText");
        QVERIFY(helpRoot && helpText);
        const auto helpOrigin = helpText->mapToItem(helpRoot, QPointF());
        QVERIFY2(helpOrigin.y() >= 15, qPrintable(QString("help top inset=%1").arg(helpOrigin.y())));
        const bool sideHelp = body->property("wideHelp").toBool();
        QVERIFY(helpOrigin.x() >= (sideHelp ? 15 : 65));
        QVERIFY(helpRoot->width() - helpText->width() >= (sideHelp ? 30 : 80));
        auto *card = visualItem(helpRoot, "settingsHelpCard");
        auto *info = visualItem(helpRoot, "settingsHelpIcon");
        auto *heading = visualItem(helpRoot, "settingsHelpTitle");
        auto *timing = visualItem(helpRoot, "settingsHelpTiming");
        QVERIFY(card && info && heading && timing);
        QCOMPARE(card->property("stacked").toBool(), sideHelp);
        if (sideHelp) {
            QVERIFY(heading->mapToItem(card, QPointF()).y() >= info->y() + info->height() + 11);
            QVERIFY(qAbs(heading->mapToItem(card, QPointF()).x() - info->x()) < 1);
        }
        QVERIFY(card->property("radius").toReal() >= 9);
        QTRY_COMPARE(info->property("status").toInt(), 1);
        QCOMPARE(F4IconProvider::decodeRouteValue(info->property("source").toUrl().path().section('/', -1)), QString("info"));
        QCOMPARE(heading->property("text").toString(), helpModel.value("title").toString());
        QCOMPARE(timing->property("text").toString(), helpModel.value("helpTiming").toString());
        QVERIFY(!helpText->property("text").toString().contains("Takes effect:"));
        QVERIFY(timing->property("color").value<QColor>().lightnessF()
            < helpText->property("color").value<QColor>().lightnessF());
        QCOMPARE(body->property("wideHelp").toBool(), body->property("mainWidth").toReal() >= 720);
        if (body->property("wideHelp").toBool()) {
            auto *bar = visualItem(root, "dialogWidget-pageViewportScrollBar");
            QVERIFY(bar);
            const auto barRect = bar->mapRectToItem(root, QRectF(0, 0, bar->width(), bar->height()));
            QVERIFY(qAbs(helpRoot->mapToItem(root, QPointF()).x() - barRect.right() - 4) < 1);
            QVERIFY(qAbs(barRect.left() - box->mapToItem(root, QPointF(box->width(), 0)).x() - 16) < 1);
            const auto gap = helpRoot->mapToItem(root, QPointF()).x()
                - box->mapToItem(root, QPointF(box->width(), 0)).x();
            QVERIFY2(qAbs(gap - (16 + bar->width() + 4)) < 1,
                qPrintable(QString("page/help gap=%1").arg(gap)));
            QVERIFY(qAbs(helpRoot->mapToItem(root, QPointF()).y()
                - page->mapToItem(root, QPointF()).y()) <= body->property("gap").toReal());
            const auto helpRect = helpRoot->mapRectToItem(surface, QRectF(0, 0, helpRoot->width(), helpRoot->height()));
            QVERIFY(qAbs(surface->width() - helpRect.right() - 12) < 1);
        } else {
            QVERIFY(helpRoot->mapToItem(root, QPointF()).y()
                >= page->mapToItem(root, QPointF(0, page->height())).y());
        }
        const auto footerMargin = surface->height() - buttonRect.bottom();
        QVERIFY2(qAbs(footerMargin - 12) < 1, qPrintable(QString("footer margin=%1 body bottom=%2 button height=%3 control height=%4")
            .arg(footerMargin).arg(body->mapToItem(surface, QPointF(0, body->height())).y())
            .arg(button->height()).arg(body->property("controlHeight").toReal())));
        const auto footerOrigin = button->mapToItem(root, QPointF());
        auto *search = visualItem(root, "dialogWidget-searchEditTextInput");
        QVERIFY(search);
        const auto searchOrigin = search->mapToItem(root, QPointF());
        page->setProperty("contentY", 31.3);
        QCoreApplication::processEvents();
        QVERIFY(!fixture.window->grabWindow().isNull());
        QCOMPARE(button->mapToItem(root, QPointF()), footerOrigin);
        QCOMPARE(search->mapToItem(root, QPointF()), searchOrigin);
        const auto checkLeaves = [&](auto &&self, QQuickItem *item) -> void {
            if (item->isVisible() && item->objectName().startsWith("dialogWidget-")
                && (item->inherits("QQuickText") || item->inherits("QQuickTextInput") || item->inherits("QQuickImage"))) {
                const auto origin = item->mapToItem(root, QPointF());
                const auto physical = origin * fixture.window->devicePixelRatio();
                const auto aligned = [&]() {
                    const auto point = item->mapToItem(root, QPointF()) * fixture.window->devicePixelRatio();
                    return qAbs(point.x()-qRound64(point.x())) < .02 && qAbs(point.y()-qRound64(point.y())) < .02;
                };
                QTRY_VERIFY2(aligned(),
                    qPrintable(QString("%1 at %2,%3").arg(item->objectName()).arg(physical.x()).arg(physical.y())));
                const auto settledOrigin = item->mapToItem(root, QPointF());
                QVERIFY(QLineF(item->mapToItem(root, QPointF(1,0))-settledOrigin, QPointF(1,0)).length() < .0001);
                QVERIFY(QLineF(item->mapToItem(root, QPointF(0,1))-settledOrigin, QPointF(0,1)).length() < .0001);
            }
            for (auto *child : item->childItems()) self(self, child);
        };
        checkLeaves(checkLeaves, surface);
        if (qEnvironmentVariableIsSet("F4_SETTINGS_RESIZE_CAPTURE"))
            QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_SETTINGS_RESIZE_CAPTURE")
                + QString(".%1.png").arg(size.width())));

    }
    // A short page must retain balanced gutters when its scrollbar disappears.
    pageModel.insert("children", QVariantList{settings.first()});
    children[4] = pageModel;
    dialog.insert("children", children);
    scene.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(scene);
    QVERIFY(QMetaObject::invokeMethod(surface, "setUserGeometry", Q_ARG(QVariant, 30), Q_ARG(QVariant, 50),
        Q_ARG(QVariant, 1250), Q_ARG(QVariant, 830)));
    auto *shortPageBar = visualItem(root, "dialogWidget-pageViewportScrollBar");
    QTRY_VERIFY(shortPageBar && !shortPageBar->isVisible());
    auto *shortBox = visualItem(root, "dialogWidget-boxRoot");
    auto *shortHelp = visualItem(root, "dialogWidget-helpRoot");
    auto *shortBody = visualItem(root, "settingsDialogBody");
    QVERIFY(shortBox && shortHelp && shortBody);
    const auto rightSpace = shortHelp->mapToItem(surface, QPointF()).x()
        - shortBox->mapToItem(surface, QPointF(shortBox->width(), 0)).x();
    const auto leftSpace = shortBox->mapToItem(surface, QPointF()).x()
        - (recordOnly ? 0 : shortBody->property("navigationWidth").toReal());
    if (!recordOnly)
        QVERIFY2(qAbs(rightSpace - leftSpace) < 1,
            qPrintable(QString("no-scrollbar gutters left=%1 right=%2").arg(leftSpace).arg(rightSpace)));
    if (!recordOnly) {
        auto *body = visualItem(root, "settingsDialogBody");
        auto *title = visualItem(root, "semanticDialogTitle");
        QVERIFY(body && title);
        body->setProperty("selectedNativePage", "gui");
        QTRY_COMPARE(title->property("text").toString(), QString("GUI"));
        body->setProperty("selectedNativePage", "theme");
        QTRY_COMPARE(title->property("text").toString(), QString("Theme creator"));
        body->setProperty("selectedNativePage", "");
        QTRY_COMPARE(title->property("text").toString(), QString("Appearance"));
    }
    if (qEnvironmentVariableIsSet("F4_SETTINGS_RESIZE_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_SETTINGS_RESIZE_CAPTURE")));
}

static QVariantMap settingsControlsScene()
{
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"settings-controls","kind":"dialog","title":"Settings","x":0,"y":0,"w":90,"h":26,"children":[
        {"id":"list","kind":"listBox","x":2,"y":2,"w":26,"h":5,"items":["First","Second","Third","Fourth","Fifth","Sixth","Seventh","Eighth","Ninth","Tenth"]},
        {"id":"table","kind":"table","x":32,"y":2,"w":50,"h":5,"showHeader":true,"columns":[{"title":"Setting","width":20},{"title":"Value","width":20}],"rows":[{"cells":["First","One"]},{"cells":["Second","Two"]},{"cells":["Third","Three"]},{"cells":["Fourth","Four"]},{"cells":["Fifth","Five"]},{"cells":["Sixth","Six"]}]},
        {"id":"enabled-edit","kind":"edit","x":2,"y":8,"w":26,"h":1,"text":"Editable"},
        {"id":"disabled-label","kind":"text","x":32,"y":8,"w":12,"h":1,"text":"Disabled input","explainTarget":"disabled-edit"},
        {"id":"disabled-edit","kind":"edit","x":46,"y":8,"w":36,"h":1,"text":"Unavailable value","disabled":true,"explainTarget":"disabled-edit"},
        {"id":"enabled-combo","kind":"comboBox","x":2,"y":10,"w":26,"h":1,"text":"Enabled","items":["Enabled"],"dropdownOnly":true},
        {"id":"disabled-combo","kind":"comboBox","x":32,"y":10,"w":50,"h":1,"text":"Unavailable choice","items":["Unavailable choice"],"dropdownOnly":true,"disabled":true,"explainTarget":"disabled-combo"},
        {"id":"radios","kind":"radioGroup","x":32,"y":12,"w":50,"h":1,"items":["First choice","Second choice with a caption that wraps naturally within the available width","Unavailable choice"],"selected":0,"focusIndex":1,"focused":true,"wrapText":true,"disabledItems":[2],"explainTarget":"radios"},
        {"id":"after-radios","kind":"checkbox","x":32,"y":13,"w":50,"h":1,"text":"After the choices","explainTarget":"after-radios"},
        {"id":"ok","kind":"button","x":36,"y":23,"w":10,"h":1,"text":"OK"}
      ]})").toVariant().toMap();
    auto scene = panelScene();
    scene.insert("dialogs", QVariantList{dialog});
    return scene;
}

void F4OperationsQueueTests::dialogWheelAnimatesAndPassesThroughShortLists()
{
    auto dialog = QJsonDocument::fromJson(R"({
      "id":"wheel-dialog","kind":"dialog","layout":"settings","settingsSectionsOnly":true,
      "title":"Wheel","x":0,"y":0,"w":76,"h":18,"children":[
        {"id":"wheel-page","kind":"group","layoutRole":"content","scrollable":true,
         "contentHeight":65,"x":2,"y":4,"w":70,"h":12,"children":[
          {"id":"short-table","kind":"table","x":2,"y":4,"w":60,"h":5,"showHeader":false,
           "columns":[{"title":"Application"}],"rows":[{"cells":["One"]}]},
          {"id":"empty-list","kind":"listBox","x":2,"y":9,"w":60,"h":3,"items":[]},
          {"id":"tail","kind":"edit","x":2,"y":63,"w":55,"h":1,"text":"End"}
        ]}
      ]})").toVariant().toMap();
    auto children = dialog.value("children").toList();
    auto pageModel = children.first().toMap();
    auto fields = pageModel.value("children").toList();
    for (int i = 0; i < 24; ++i)
        fields.append(QVariantMap{{"id", QString("property-%1").arg(i)}, {"kind", "edit"},
            {"text", "Property"}, {"x", 2}, {"y", 12 + i * 2}, {"w", 60}, {"h", 1}});
    pageModel["children"] = fields;
    children[0] = pageModel;
    dialog["children"] = children;
    auto scene = panelScene();
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    auto *page = visualItem(root, "dialogWidget-wheel-pageViewport");
    auto *table = visualItem(root, "dialogWidget-short-tableTableRows");
    auto *list = visualItem(root, "dialogWidget-empty-listListView");
    QVERIFY(page && table && list);
    QTRY_VERIFY(page->property("contentHeight").toReal() > page->height() + 200);
    QVERIFY(table->property("contentHeight").toReal() <= table->height());
    QVERIFY(list->property("contentHeight").toReal() <= list->height());
    QVERIFY(page->mapRectToItem(root, page->boundingRect()).contains(itemCenter(table)));
    QVERIFY(page->mapRectToItem(root, page->boundingRect()).contains(itemCenter(list)));
    const auto scroll = [page] { return page->property("contentY").toReal(); };
    const auto wheel = [&](QQuickItem *target, QPoint pixels, QPoint angles) {
        const QPoint pos = itemCenter(target);
        QWheelEvent event(pos, fixture.window->mapToGlobal(pos), pixels, angles,
                          Qt::NoButton, Qt::NoModifier, Qt::NoScrollPhase, false);
        QCoreApplication::sendEvent(fixture.window, &event);
    };
    fixture.shell.clearActions();
    wheel(table, {}, {0, -120});
    wheel(table, {}, {0, -120});
    QCOMPARE(scroll(), 0.0);
    QTRY_VERIFY(scroll() > 0);
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "wheel-page" && action.value("action") == "control.scroll";
    }));
    QVERIFY(std::none_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "short-table";
    }));
    // Masonry advances by the wheel's angle units in logical pixels.
    QVERIFY(qAbs(scroll() - 240) <= 1 / fixture.window->devicePixelRatio());
    QVERIFY(page->property("pixelAligned").toBool());
    QVERIFY(page->setProperty("contentY", 0));
    wheel(list, {0, -20}, {});
    QVERIFY(scroll() > 0);
    QVERIFY(qAbs(scroll() - 20) <= 1 / fixture.window->devicePixelRatio());
}

void F4OperationsQueueTests::semanticTableWheelPreservesMagnitude()
{
    QueueFixture fixture(settingsControlsScene());
    QVERIFY(fixture.window);
    auto *view = visualItem(fixture.window->contentItem(), "dialogWidget-tableTableRows");
    QVERIFY(view);
    const QPoint position = itemCenter(view);
    fixture.shell.clearActions();
    const auto sendWheel = [&](int angle) {
        QWheelEvent wheel(position, fixture.window->mapToGlobal(position), {}, QPoint(0, angle),
                          Qt::NoButton, Qt::NoModifier, Qt::NoScrollPhase, false);
        QCoreApplication::sendEvent(fixture.window, &wheel);
    };
    sendWheel(-360);
    QCOMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.constLast().value("action").toString(), QString("control.scroll"));
    QCOMPARE(fixture.shell.actions.constLast().value("target").toString(), QString("table"));
    const int lines = qMax(1, QGuiApplication::styleHints()->wheelScrollLines());
    QCOMPARE(fixture.shell.actions.constLast().value("delta").toInt(), 3 * lines);
    fixture.shell.clearActions();
    for (int i = 0; i < 12; ++i) sendWheel(10);
    int total = 0;
    for (const auto &action : fixture.shell.actions) total += action.value("delta").toInt();
    QCOMPARE(total, -lines);
}

void F4OperationsQueueTests::settingsCategoryWheelScrollsWithoutSelecting()
{
    auto scene = settingsControlsScene();
    auto dialog = scene.value("dialogs").toList().first().toMap();
    auto table = dialog.value("children").toList()[1].toMap();
    table.insert("layoutRole", "navigation");
    table.insert("cursor", 2);
    QVariantList rows;
    for (int i = 0; i < 60; ++i)
        rows.append(QVariantMap{{"cells", QStringList{QString("Category %1").arg(i)}}});
    table["rows"] = rows;
    dialog.insert("layout", "settings");
    dialog.insert("children", QVariantList{table});
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *view = visualItem(fixture.window->contentItem(), "dialogWidget-tableTableRows");
    QVERIFY(view);
    QTest::qWait(100);
    const QPoint position = itemCenter(view);
    const auto sendWheel = [&](QPoint pixels, QPoint angle, Qt::ScrollPhase phase) {
        QWheelEvent wheel(position, fixture.window->mapToGlobal(position), pixels, angle,
                          Qt::NoButton, Qt::NoModifier, phase, false);
        QCoreApplication::sendEvent(fixture.window, &wheel);
    };
    view->setProperty("contentY", 0);
    fixture.shell.clearActions();
    sendWheel({}, QPoint(0, -120), Qt::NoScrollPhase);
    QTRY_VERIFY(view->property("contentY").toReal() > 0);
    const qreal afterMouse = view->property("contentY").toReal();
    sendWheel(QPoint(0, -48), {}, Qt::ScrollBegin);
    sendWheel(QPoint(0, -48), {}, Qt::ScrollUpdate);
    sendWheel({}, {}, Qt::ScrollEnd);
    QTRY_VERIFY(view->property("contentY").toReal() > afterMouse);
    QCOMPARE(view->property("currentIndex").toInt(), 2);
    const qreal savedScroll = view->property("contentY").toReal();
    dialog["title"] = "Settings updated";
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTest::qWait(30);
    QCOMPARE(view->property("contentY").toReal(), savedScroll);
    for (const auto &action : fixture.shell.actions)
        QVERIFY(action.value("target") != table.value("id"));
    auto *root = fixture.window->contentItem();
    auto *surface = visualItem(root, "semanticDialog-" + dialog.value("id").toString());
    auto *bar = visualItem(root, "dialogWidget-tableTableScrollBar");
    auto *body = visualItem(root, "settingsDialogBody");
    auto *sidebar = visualItem(root, "settingsSidebarBackground");
    QVERIFY(surface && bar && body && sidebar);
    const qreal sidebarRight = body->property("navigationWidth").toReal();
    auto *divider = visualItem(root, "settingsSidebarSeparator");
    auto *headerSeparator = visualItem(root, "dialogHeaderSeparator");
    QVERIFY(divider && divider->isVisible() && headerSeparator);
    QCOMPARE(divider->x(), sidebarRight);
    QCOMPARE(divider->y(), qreal(0));
    QCOMPARE(divider->height(), sidebar->height());
    QCOMPARE(divider->width(), headerSeparator->height());
    QCOMPARE(divider->property("color"), headerSeparator->property("color"));
    QCOMPARE(divider->opacity(), headerSeparator->opacity());
    const qreal barRight = bar->mapToItem(body, QPointF(bar->width(), 0)).x();
    QVERIFY(qAbs(barRight - sidebarRight) < 1 / fixture.window->devicePixelRatio());
    const qreal sidebarBottom = sidebar->mapToItem(surface, QPointF(0, sidebar->height())).y();
    QVERIFY(sidebarBottom <= surface->height() - 1 / fixture.window->devicePixelRatio());
    const qreal listBottom = view->mapToItem(surface, QPointF(0, view->height())).y();
    QVERIFY(listBottom < surface->height() - 1 / fixture.window->devicePixelRatio());
    const qreal searchBottom = body->property("searchY").toReal() + body->property("searchHeight").toReal();
    const qreal topGap = body->property("navigationY").toReal() - searchBottom;
    QVERIFY(qAbs(sidebarBottom - listBottom - topGap) <= 1 / fixture.window->devicePixelRatio());
}

void F4OperationsQueueTests::settingsCategoryListSelectsDuringMouseDrag_data()
{
    QTest::addColumn<bool>("table");
    QTest::newRow("listBox") << false;
    QTest::newRow("category-table") << true;
}

void F4OperationsQueueTests::settingsCategoryListSelectsDuringMouseDrag()
{
    QFETCH(bool, table);
    auto scene = settingsControlsScene();
    if (table) {
        auto dialog = scene.value("dialogs").toList().first().toMap();
        auto children = dialog.value("children").toList();
        auto list = children[0].toMap();
        QVariantList rows;
        for (const auto &caption : list.value("items").toList())
            rows.append(QVariantMap{{"cells", QVariantList{caption}}});
        list.insert("kind", "table");
        list.insert("columns", QVariantList{QVariantMap{{"width", 0}}});
        list.insert("rows", rows);
        list.insert("showHeader", false);
        list.insert("h", 6);
        list.remove("items");
        children[0] = list;
        dialog.insert("children", children);
        scene.insert("dialogs", QVariantList{dialog});
    }
    const QString viewName = table ? "TableRows" : "ListView";
    const QString pointerName = table ? "TablePointer" : "ListPointer";
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    fixture.window->resize(800, 540);
    auto *root = fixture.window->contentItem();
    QQuickItem *view = nullptr;
    QTRY_VERIFY((view = visualItem(root, "dialogWidget-list" + viewName)));
    auto *body = visualItem(root, "dialogBody");
    auto *dialog = visualItem(root, "semanticDialog-settings-controls");
    QVERIFY(body && dialog);
    QVERIFY(QMetaObject::invokeMethod(dialog, "setUserGeometry",
        Q_ARG(QVariant, dialog->x()), Q_ARG(QVariant, dialog->y()),
        Q_ARG(QVariant, dialog->width()), Q_ARG(QVariant, 250)));
    QVERIFY(!fixture.window->grabWindow().isNull());
    QVERIFY(body->property("contentHeight").toReal() > body->height());
    const qreal bodyY = body->property("contentY").toReal();
    QPointer<QQuickItem> pointerOwner = visualItem(root, "dialogWidget-list" + pointerName);
    QVERIFY(pointerOwner);
    auto *dragHandler = pointerOwner->findChild<QObject *>(pointerOwner->objectName() + "Drag");
    QVERIFY(dragHandler);
    QVector<int> selected;
    QObject::connect(&fixture.shell, &TestShell::uiActionSent, fixture.window,
        [&](const QVariantMap &action) {
            if (action.value("target") != "list" || action.value("action") != "control.select") return;
            const int index = action.value("index").toInt();
            selected.append(index);
            // Acknowledge each selection through the same full snapshot path as
            // Go. The pointer grab must survive replacement of item delegates.
            auto dialogs = scene.value("dialogs").toList();
            auto dialog = dialogs[0].toMap();
            auto children = dialog.value("children").toList();
            auto list = children[0].toMap();
            list.insert("cursor", index);
            list.insert("focused", true);
            children[0] = list;
            dialog.insert("children", children);
            dialogs[0] = dialog;
            scene.insert("dialogs", dialogs);
            fixture.shell.setScene(scene);
        }, Qt::QueuedConnection);
    const auto rowPoint = [&](int index) {
        const auto suffix = table ? "TableCell-" + QString::number(index) + "-0"
                                  : "ListItemText-" + QString::number(index);
        auto *label = visualItem(root, "dialogWidget-list" + suffix);
        return label ? label->mapToItem(root, QPointF(20, label->height() / 2)).toPoint() : QPoint{};
    };
    QPoint pointer = rowPoint(1);
    QVERIFY(!pointer.isNull());
    bool pressed = true;
    const auto release = qScopeGuard([&] {
        if (pressed) QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, pointer);
    });
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, pointer);
    QTRY_COMPARE(selected, QVector<int>({1}));
    QCOMPARE(view->property("currentIndex").toInt(), 1);
    for (int index : {3, 2, 0}) {
        pointer = rowPoint(index);
        QVERIFY(!pointer.isNull());
        QTest::mouseMove(fixture.window, pointer, 30);
        QTRY_COMPARE(selected.constLast(), index);
        QVERIFY(pointerOwner);
        QCOMPARE(pointerOwner.data(), visualItem(root, "dialogWidget-list" + pointerName));
        QVERIFY(dragHandler->property("active").toBool());
        QCOMPARE(body->property("contentY").toReal(), bodyY);
        QVERIFY(!body->property("dragging").toBool());
        QVERIFY(!view->property("dragging").toBool());
    }
    const int changes = selected.size();
    QTest::mouseMove(fixture.window, pointer + QPoint(4, 0), 20);
    QCoreApplication::processEvents();
    QCOMPARE(selected.size(), changes);
    // Like the captured TUI list, follow Y when X leaves the list. Moving
    // vertically outside the viewport must not select or scroll anything.
    pointer = rowPoint(3);
    pointer.setX(view->mapToItem(root, QPointF(view->width() + 30, 0)).toPoint().x());
    QTest::mouseMove(fixture.window, pointer, 20);
    QTRY_COMPARE(selected.constLast(), 3);
    const int outsideChanges = selected.size();
    pointer = view->mapToItem(root, QPointF(30, -10)).toPoint();
    QTest::mouseMove(fixture.window, pointer, 20);
    QCoreApplication::processEvents();
    QCOMPARE(selected.size(), outsideChanges);
    QCOMPARE(body->property("contentY").toReal(), bodyY);
    pointer = rowPoint(2);
    QTest::mouseMove(fixture.window, pointer, 20);
    QTRY_COMPARE(selected.constLast(), 2);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, pointer);
    pressed = false;
    const int finalChanges = selected.size();
    QTest::mouseMove(fixture.window, rowPoint(1), 20);
    QCoreApplication::processEvents();
    QCOMPARE(selected.size(), finalChanges);
    for (const auto &action : std::as_const(fixture.shell.actions))
        QVERIFY(action.value("action") != "control.activate");
    if (table) {
        QTest::mouseDClick(fixture.window, Qt::LeftButton, Qt::NoModifier, rowPoint(1));
        QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
            return action.value("action") == "control.activate" && action.value("index") == 1;
        }));
    }
    qInfo() << "[FIX:listbox-drag] selection sequence" << selected
            << "dialog scroll" << body->property("contentY");
}

void F4OperationsQueueTests::settingsListScrollingAndTouch()
{
    QueueFixture fixture(settingsControlsScene());
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *view = nullptr;
    QTRY_VERIFY((view = visualItem(root, "dialogWidget-listListView")));
    QTest::qWait(100); // settle initial dialog geometry/focus before targeting a wheel event
    QVERIFY(!fixture.window->grabWindow().isNull());
    auto *body = visualItem(root, "dialogBody");
    const qreal bodyY = body->property("contentY").toReal();
    const auto point = [&](qreal y) { return view->mapToItem(root, QPointF(30, y)).toPoint(); };
    const auto resetScroll = [&] {
        QMetaObject::invokeMethod(view, "cancelFlick");
        view->setProperty("contentY", 0);
        QCoreApplication::processEvents();
        fixture.window->grabWindow();
        fixture.shell.clearActions();
    };
    const auto wheelPosition = itemCenter(view);
    QWheelEvent wheel(wheelPosition, fixture.window->mapToGlobal(wheelPosition), {}, QPoint(0, -120),
                      Qt::NoButton, Qt::NoModifier, Qt::NoScrollPhase, false);
    QCoreApplication::sendEvent(fixture.window, &wheel);
    QTRY_VERIFY(view->property("contentY").toReal() > 0);
    QCOMPARE(body->property("contentY").toReal(), bodyY);
    resetScroll();
    auto *bar = visualItem(root, "dialogWidget-listListScrollBar");
    auto *handle = bar->property("contentItem").value<QQuickItem *>();
    QVERIFY(handle && handle->isVisible());
    const auto start = itemCenter(handle);
    const auto end = start + QPoint(0, 35);
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, start);
    QTest::mouseMove(fixture.window, end, 30);
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, end);
    QTRY_VERIFY(view->property("contentY").toReal() > 0);
    QVERIFY(fixture.shell.actions.isEmpty());
    QCOMPARE(body->property("contentY").toReal(), bodyY);
    resetScroll();
    auto *touch = QTest::createTouchDevice();
    QTest::touchEvent(fixture.window, touch).press(0, point(30), fixture.window).commit();
    QVERIFY(fixture.shell.actions.isEmpty());
    QTest::touchEvent(fixture.window, touch).release(0, point(30), fixture.window).commit();
    QTRY_COMPARE(fixture.shell.actions.size(), 1);
    QCOMPARE(fixture.shell.actions.first().value("action").toString(), QString("control.select"));
    QCOMPARE(fixture.shell.actions.first().value("index").toInt(), 1);
    resetScroll();
    QTest::touchEvent(fixture.window, touch).press(0, point(90), fixture.window).commit();
    for (int y : {70, 45, 20}) {
        QTest::qWait(20);
        QTest::touchEvent(fixture.window, touch).move(0, point(y), fixture.window).commit();
    }
    QTest::touchEvent(fixture.window, touch).release(0, point(20), fixture.window).commit();
    QTRY_VERIFY(view->property("contentY").toReal() > 0);
    QVERIFY(fixture.shell.actions.isEmpty());
    QCOMPARE(body->property("contentY").toReal(), bodyY);
    resetScroll();
    // Disabled/read-only lists must reject selection without mutating the model.
    for (const auto &flag : {"readOnly", "disabled"}) {
        auto scene = settingsControlsScene();
        auto dialog = scene.value("dialogs").toList().first().toMap();
        auto children = dialog.value("children").toList();
        auto list = children[0].toMap();
        list.insert(flag, true);
        children[0] = list;
        dialog.insert("children", children);
        scene.insert("dialogs", QVariantList{dialog});
        fixture.shell.setScene(scene);
        QCoreApplication::processEvents();
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, point(30));
        QVERIFY(fixture.shell.actions.isEmpty());
    }
}

void F4OperationsQueueTests::dropdownMenusFitAndScroll_data()
{
    QTest::addColumn<QString>("kind");
    QTest::newRow("sort") << QString("sort");
    QTest::newRow("renderer") << QString("renderer");
    QTest::newRow("text") << QString("text");
}

void F4OperationsQueueTests::dropdownMenusFitAndScroll()
{
    QFETCH(QString, kind);
    auto scene = panelScene();
    auto shell = scene["shell"].toMap();
    shell["commandLine"] = QVariantMap{{"visible", true}, {"focused", true}, {"text", "draft"}};
    scene["shell"] = shell;
    QueueFixture fixture(scene, false, true);
    fixture.window->setHeight(220);
    QTest::qWait(60);
    auto *root = fixture.window->contentItem();
    const QString name = kind == "text" ? "commandLineEditMenu" : kind == "sort" ? "panelSortMenu-0" : "panelRendererMenu-0";
    auto *anchor = visualItem(root, kind == "text" ? "commandLineFocusArea" : kind == "sort" ? "panelSortButton-0" : "panelRendererButton-0");
    QVERIFY(anchor);
    QTest::mouseClick(fixture.window, kind == "text" ? Qt::RightButton : Qt::LeftButton, Qt::NoModifier, itemCenter(anchor));
    auto *popup = fixture.window->findChild<QObject *>(name);
    QVERIFY(popup);
    QTRY_VERIFY(popup->property("opened").toBool());
    QTest::qWait(60);
    QVERIFY2(popup->property("height").toReal() <= fixture.window->height() - 12 + 1,
        qPrintable(QString("%1 height=%2 viewport=%3").arg(name).arg(popup->property("height").toReal()).arg(fixture.window->height())));
    QVERIFY(popup->property("y").toReal() >= 0);
    QVERIFY(popup->property("y").toReal() + popup->property("height").toReal() <= fixture.window->height() + 1);
    auto *bar = visualItem(root, name + "ScrollBar");
    QVERIFY(bar && bar->isVisible());
    QVERIFY(bar->property("size").toReal() < 1);
    auto *view = visualItem(root, name + "Viewport");
    QVERIFY(view);
    const qreal before = view->property("contentY").toReal();
    const auto position = itemCenter(view);
    QWheelEvent wheel(position, fixture.window->mapToGlobal(position), {}, QPoint(0, -120),
        Qt::NoButton, Qt::NoModifier, Qt::NoScrollPhase, false);
    QCoreApplication::sendEvent(fixture.window, &wheel);
    QTRY_VERIFY(view->property("contentY").toReal() > before);
    QMetaObject::invokeMethod(view, "cancelFlick");
    view->setProperty("contentY", 0);
    const QPointingDevice pad("dropdown touchpad", 9876, QInputDevice::DeviceType::TouchPad,
        QPointingDevice::PointerType::Finger,
        QInputDevice::Capability::Position | QInputDevice::Capability::PixelScroll,
        2, 0);
    for (const auto phase : {Qt::ScrollBegin, Qt::ScrollUpdate, Qt::ScrollEnd}) {
        QWheelEvent touchpad(position, fixture.window->mapToGlobal(position),
            phase == Qt::ScrollUpdate ? QPoint(0, -40) : QPoint(), {},
            Qt::NoButton, Qt::NoModifier, phase, false, Qt::MouseEventSynthesizedBySystem, &pad);
        QCoreApplication::sendEvent(fixture.window, &touchpad);
    }
    QTRY_VERIFY(view->property("contentY").toReal() > 0);
    bar->setProperty("position", 1.0 - bar->property("size").toReal());
    QTRY_VERIFY(view->property("contentY").toReal() + view->height() >= view->property("contentHeight").toReal() - 2);
    if (kind == "renderer") {
        for (int i = 0; i < 20; ++i) QTest::keyClick(fixture.window, Qt::Key_Down);
        QTRY_VERIFY(popup->property("keyboardIndex").toInt() >= 0);
    }
    fixture.window->setHeight(720);
    QTest::qWait(100);
    QVERIFY(!bar->isVisible() || bar->property("size").toReal() < 1);
}

void F4OperationsQueueTests::longMenusRevealKeyboardSelection()
{
    QueueFixture fixture(panelScene(), false, true);
    fixture.window->setHeight(260);
    QQmlComponent component(&fixture.engine);
    component.setData(R"(
        import QtQuick
        import QtQuick.Controls
        import QtQuick.Controls.Basic as T
        import QtQml.Models
        F4Menu {
            id: menu
            objectName: "longMenu"
            popupType: Popup.Item
            width: 240
            y: 240
            Instantiator {
                model: 80
                delegate: T.MenuItem {
                    required property int index
                    text: "Entry " + index
                    width: menu.availableWidth
                    implicitHeight: 31
                    hoverEnabled: true
                }
                onObjectAdded: (index, object) => menu.insertItem(index, object)
                onObjectRemoved: (index, object) => menu.removeItem(object)
            }
        }
    )", QUrl("qrc:/F4QtHost/qml/OverflowTest.qml"));
    QTRY_VERIFY2(component.isReady(), qPrintable(component.errorString()));
    auto *menu = component.createWithInitialProperties({{"parent", QVariant::fromValue(fixture.window->contentItem())}});
    QVERIFY2(menu, qPrintable(component.errorString()));
    menu->setParent(fixture.window);
    QVERIFY(QMetaObject::invokeMethod(menu, "open"));
    QTRY_VERIFY(menu->property("opened").toBool());
    QVERIFY(menu->property("height").toReal() <= fixture.window->height() - 12 + 1);
    auto *view = menu->property("contentItem").value<QQuickItem *>();
    QVERIFY(view);
    QTRY_VERIFY(view->property("contentHeight").toReal() > view->height());
    auto *bar = visualItem(fixture.window->contentItem(), "longMenuScrollBar");
    QVERIFY(bar && bar->isVisible());
    for (int i = 0; i < 80; ++i) QTest::keyClick(fixture.window, Qt::Key_Down);
    QTRY_COMPARE(menu->property("currentIndex").toInt(), 79);
    QTRY_VERIFY(view->property("contentY").toReal() + view->height() >= view->property("contentHeight").toReal() - 2);
    QVERIFY(QMetaObject::invokeMethod(menu, "close"));
}

void F4OperationsQueueTests::textContextMenusUseSharedStyle()
{
    auto scene = panelScene();
    auto shell = scene.value("shell").toMap();
    shell["panels"] = QVariantList{filePanel(0, false)};
    shell["commandLine"] = QVariantMap{{"visible", true}, {"focused", true}, {"text", "echo test"},
        {"selectionStart", 0}, {"selectionEnd", 4}, {"cursorPosition", 4}};
    scene["shell"] = shell;
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    auto *area = visualItem(root, "commandLineFocusArea");
    QVERIFY(area && area->isVisible());
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(area));
    auto *menu = fixture.window->findChild<QObject *>("commandLineEditMenu");
    QVERIFY2(menu, "command line did not create its text context menu");
    QTRY_VERIFY(menu->property("visible").toBool());
    QTest::qWait(60); // Native popup polish must settle before hit testing rows.
    auto *copy = visualItem(root, "commandLineEditMenu-copy");
    QVERIFY(copy && copy->isEnabled());
    auto *icon = visualItem(root, "commandLineEditMenu-copyIcon");
    QVERIFY(icon);
    QTRY_COMPARE(icon->property("status").toInt(), 1);
    for (const QString &action : {QString("undo"), QString("redo"), QString("cut"), QString("paste"), QString("selectAll")}) {
        auto *actionIcon = visualItem(root, "commandLineEditMenu-" + action + "Icon");
        QVERIFY(actionIcon);
        const QString requested = F4IconProvider::decodeRouteValue(actionIcon->property("source").toUrl().path().section('/', -1));
        QVERIFY2(requested != "file", qPrintable("missing edit icon: " + action));
    }
    auto *background = visualItem(root, "commandLineEditMenuBackground");
    QVERIFY(background);
    QCOMPARE(background->property("color"), fixture.window->property("controlBg"));
    auto *disabledUndo = visualItem(root, "commandLineEditMenu-undo");
    QVERIFY(disabledUndo && !disabledUndo->isEnabled());
    const auto hover = [&](QQuickItem *row) {
        const auto position = itemCenter(row);
        QCursor::setPos(fixture.window->mapToGlobal(position));
        QTest::mouseMove(fixture.window, position, 30);
    };
    hover(copy);
    QTRY_VERIFY(copy->property("highlighted").toBool());
    hover(disabledUndo);
    QTRY_VERIFY(!copy->property("highlighted").toBool());
    QVERIFY(!disabledUndo->property("highlighted").toBool());
    auto *paste = visualItem(root, "commandLineEditMenu-paste");
    QVERIFY(paste && paste->isEnabled());
    hover(paste);
    QTRY_VERIFY(paste->property("highlighted").toBool());
    hover(disabledUndo);
    QTRY_VERIFY(!paste->property("highlighted").toBool());
    QTest::qWait(60); // Settle Cocoa's native cursor warp before keyboard input.
    QTest::keyClick(fixture.window, Qt::Key_Down);
    QTRY_VERIFY(menu->property("currentIndex").toInt() >= 0);
    QTest::qWait(60);
    QVERIFY(menu->property("currentIndex").toInt() >= 0);
    auto *grid = qobject_cast<TestGrid *>(visualItem(root, "vtuiGrid"));
    QVERIFY(grid);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(copy));
    QTRY_COMPARE(grid->forwardedKeys.size(), 2);
    QCOMPARE(grid->forwardedKeys[0].value("key").toInt(), int(Qt::Key_C));
    QCOMPARE(grid->forwardedKeys[0].value("modifiers").toInt(), int(Qt::ControlModifier));
    QTRY_VERIFY(!menu->property("visible").toBool());
    auto *path = visualItem(root, "panelPathTitle-0");
    QVERIFY(path);
    path->setProperty("editMode", true);
    auto *field = visualItem(path, "pathField");
    QVERIFY(field && field->isVisible());
    const int windows = QGuiApplication::topLevelWindows().size();
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(field));
    auto *pathMenu = fixture.window->findChild<QObject *>("pathEditMenu-0");
    QVERIFY(pathMenu);
    QTRY_VERIFY(pathMenu->property("visible").toBool());
    QCOMPARE(QGuiApplication::topLevelWindows().size(), windows);
    QVERIFY(!menu->property("visible").toBool());
    QVERIFY(QMetaObject::invokeMethod(pathMenu, "close"));
    QQmlComponent component(&fixture.engine);
    component.loadUrl(QUrl("qrc:/F4QtHost/qml/F4TextField.qml"));
    QTRY_VERIFY2(component.isReady(), qPrintable(component.errorString()));
    auto *local = qobject_cast<QQuickItem *>(component.createWithInitialProperties({
        {"hostWindow", QVariant::fromValue(fixture.window)}, {"objectName", "localField"},
        {"x", 30}, {"y", 140}, {"width", 260}, {"height", 32}, {"text", "abcdef"}}));
    QVERIFY2(local, qPrintable(component.errorString()));
    local->setParent(root);
    local->setParentItem(root);
    QVERIFY(QMetaObject::invokeMethod(local, "select", Q_ARG(QVariant, 1), Q_ARG(QVariant, 4)));
    auto *localArea = visualItem(local, "localFieldTextCursorArea");
    QVERIFY(localArea);
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(localArea));
    QTest::qWait(60); // Let Menu lay out its lazily created rows before clicking.
    auto *remove = visualItem(root, "localFieldEditMenu-delete");
    QVERIFY(remove && remove->isEnabled());
    if (qEnvironmentVariableIsSet("F4_TRACE_TEST_INPUT")) {
        qInfo() << "context delete at" << itemCenter(remove) << "window" << fixture.window->size()
                << "selection" << local->property("selectionStart") << local->property("selectionEnd");
        QVERIFY(fixture.window->grabWindow().save("/Users/zoin/.codex/worktrees/qt-run-current/f4/artifacts/text-context-menu-175.png"));
    }
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(remove));
    QTRY_COMPARE(local->property("text").toString(), QString("aef"));
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(localArea));
    QTest::qWait(60);
    auto *undo = visualItem(root, "localFieldEditMenu-undo");
    QVERIFY(undo && undo->isEnabled());
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(undo));
    QTRY_COMPARE(local->property("text").toString(), QString("abcdef"));
    local->setProperty("readOnly", true);
    QVERIFY(QMetaObject::invokeMethod(local, "selectAll"));
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(localArea));
    QTest::qWait(60);
    QVERIFY(!visualItem(root, "localFieldEditMenu-cut")->isEnabled());
    QVERIFY(visualItem(root, "localFieldEditMenu-copy")->isEnabled());
}

void F4OperationsQueueTests::commandLineSelectAllDoesNotSendPanelShortcut()
{
    auto scene = panelScene();
    auto shell = scene["shell"].toMap();
    const QString text = QString::fromUtf8("echo 😀\nsecond line");
    shell["commandLine"] = QVariantMap{{"visible", true}, {"focused", true}, {"text", text}};
    scene["shell"] = shell;
    QueueFixture fixture(scene, false, true);
    auto *root = fixture.window->contentItem();
    auto *area = visualItem(root, "commandLineFocusArea");
    QVERIFY(area);
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(area));
    QTest::qWait(60);
    auto *select = visualItem(root, "commandLineEditMenu-selectAll");
    QVERIFY(select && select->isEnabled());
    fixture.shell.clearActions();
    auto *grid = qobject_cast<TestGrid *>(visualItem(root, "vtuiGrid"));
    QVERIFY(grid);
    grid->forwardedKeys.clear();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(select));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [&](const auto &action) {
        return action.value("action") == "commandLine.select"
            && action.value("anchor").toInt() == 0
            && action.value("cursorPosition").toInt() == text.size();
    }));
    QVERIFY(grid->forwardedKeys.isEmpty());
}

void F4OperationsQueueTests::commandAutocompleteUsesRenderedPromptWidth()
{
    auto scene = panelScene();
    auto shell = scene["shell"].toMap();
    shell["commandLine"] = QVariantMap{{"visible", true}, {"focused", true},
        {"prompt", "user@host:~/directory$"}, {"inputX", 65}, {"text", "pro"}};
    scene["shell"] = shell;
    scene["menus"] = QVariantList{QVariantMap{{"id", "command-hints"}, {"kind", "menu"},
        {"role", "autocomplete"}, {"query", "pro"}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"text", "process --help"}}}}}};
    QueueFixture fixture(scene, false, true);
    auto *root = fixture.window->contentItem();
    auto *input = visualItem(root, "commandLineInputViewport");
    QVERIFY(input);
    QQuickItem *popup = nullptr;
    QTRY_VERIFY((popup = visualItem(root, "autocompleteHintsPanel")));
    QTRY_VERIFY(std::abs(popup->mapToScene(QPointF()).x() + 12
        - input->mapToScene(QPointF()).x()) < 1);
    fixture.window->setWidth(480);
    QTRY_VERIFY(std::abs(popup->mapToScene(QPointF()).x() + 12
        - input->mapToScene(QPointF()).x()) < 1);
}

void F4OperationsQueueTests::commandLineUndoRedoUsesCoreState()
{
    auto scene = panelScene();
    auto shell = scene["shell"].toMap();
    auto command = QVariantMap{{"visible", true}, {"focused", true}, {"text", "draft"},
        {"canUndo", true}, {"canRedo", false}};
    shell["commandLine"] = command;
    scene["shell"] = shell;
    QueueFixture fixture(scene, false, true);
    auto *root = fixture.window->contentItem();
    auto *area = visualItem(root, "commandLineFocusArea");
    QVERIFY(area);
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(area));
    QTest::qWait(60);
    auto *undo = visualItem(root, "commandLineEditMenu-undo");
    auto *redo = visualItem(root, "commandLineEditMenu-redo");
    QVERIFY(undo && undo->isEnabled() && redo && !redo->isEnabled());
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(undo));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("action") == "commandLine.undo";
    }));
    command["canUndo"] = false;
    command["canRedo"] = true;
    command["text"] = "";
    shell["commandLine"] = command;
    scene["shell"] = shell;
    fixture.shell.setScene(scene);
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(area));
    QTest::qWait(60);
    undo = visualItem(root, "commandLineEditMenu-undo");
    redo = visualItem(root, "commandLineEditMenu-redo");
    QVERIFY(undo && !undo->isEnabled() && redo && redo->isEnabled());
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(redo));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("action") == "commandLine.redo";
    }));
    QQmlComponent component(&fixture.engine);
    component.loadUrl(QUrl("qrc:/F4QtHost/qml/DialogTextField.qml"));
    QTRY_VERIFY2(component.isReady(), qPrintable(component.errorString()));
    auto *field = qobject_cast<QQuickItem *>(component.createWithInitialProperties({
        {"hostWindow", QVariant::fromValue(fixture.window)}, {"objectName", "remoteUndoField"},
        {"widget", QVariantMap{{"id", "core-edit"}, {"text", "draft"}, {"focused", true},
            {"canUndo", true}, {"canRedo", false}}},
        {"x", 30}, {"y", 140}, {"width", 260}, {"height", 32}}));
    QVERIFY2(field, qPrintable(component.errorString()));
    field->setParent(root);
    field->setParentItem(root);
    auto *fieldArea = visualItem(field, "remoteUndoFieldTextCursorArea");
    QVERIFY(fieldArea);
    QTest::mouseClick(fixture.window, Qt::RightButton, Qt::NoModifier, itemCenter(fieldArea));
    QTest::qWait(60);
    auto *fieldUndo = visualItem(root, "remoteUndoFieldEditMenu-undo");
    QVERIFY(fieldUndo && fieldUndo->isEnabled());
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(fieldUndo));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target") == "core-edit" && action.value("action") == "control.undo";
    }));
}

void F4OperationsQueueTests::semanticMenuClearsDisabledHover()
{
    auto scene = panelScene();
    auto menu = dialogComboMenu(0);
    menu["ownerId"] = "";
    menu["presentation"] = "";
    auto entries = menu["items"].toList();
    auto disabled = entries[1].toMap();
    disabled["disabled"] = true;
    entries[1] = disabled;
    menu["items"] = entries;
    scene["menus"] = QVariantList{menu};
    QueueFixture fixture(scene);
    auto *root = fixture.window->contentItem();
    QQuickItem *popup = nullptr;
    QTRY_VERIFY((popup = visualItem(root, "semanticMenuPopup-appearance-combo-menu")));
    auto *overlay = popup->parentItem();
    auto *first = visualItem(root, "semanticMenuItem-appearance-combo-menu-0");
    auto *inactive = visualItem(root, "semanticMenuItem-appearance-combo-menu-1");
    auto *last = visualItem(root, "semanticMenuItem-appearance-combo-menu-2");
    QVERIFY(first && inactive && last);
    const auto moveTo = [&](QQuickItem *row) {
        const auto position = itemCenter(row);
        QCursor::setPos(fixture.window->mapToGlobal(position));
        QTest::mouseMove(fixture.window, position, 30);
    };
    moveTo(first);
    moveTo(inactive);
    QTRY_COMPARE(overlay->property("visualSelectedIndex").toInt(), -1);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(inactive));
    for (const auto &action : std::as_const(fixture.shell.actions)) {
        QVERIFY(action.value("action") != "menu.activate");
        QVERIFY(action.value("action") != "menu.select" || action.value("index").toInt() != 1);
    }
    moveTo(last);
    QTRY_COMPARE(overlay->property("visualSelectedIndex").toInt(), 2);
    moveTo(inactive);
    QTRY_COMPARE(overlay->property("visualSelectedIndex").toInt(), -1);
    // A delayed pointer acknowledgement must not repaint the old row.
    menu["selected"] = 2;
    scene["menus"] = QVariantList{menu};
    fixture.shell.setScene(scene);
    QTRY_COMPARE(overlay->property("visualSelectedIndex").toInt(), -1);
    // A core-owned keyboard selection must replace the pointer-only suppression.
    menu["selected"] = 0;
    scene["menus"] = QVariantList{menu};
    fixture.shell.setScene(scene);
    QTRY_COMPARE(overlay->property("visualSelectedIndex").toInt(), 0);
}

void F4OperationsQueueTests::commandLineFocusKeepsPanelImagePaste()
{
    auto scene = panelScene();
    auto shell = scene.value("shell").toMap();
    shell["panels"] = QVariantList{filePanel(0, false), filePanel(1, false)};
    shell["commandLine"] = QVariantMap{{"visible", true}, {"focused", true}, {"text", ""}};
    scene["shell"] = shell;
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *grid = visualItem(fixture.window->contentItem(), "vtuiGrid");
    QVERIFY(grid);
    QTRY_VERIFY(grid->property("panelClipboardPasteEnabled").toBool());
    for (const QString &state : {QString("terminalBusy"), QString("terminalActive")}) {
        shell[state] = true;
        scene["shell"] = shell;
        fixture.shell.setScene(scene);
        QTRY_VERIFY(!grid->property("panelClipboardPasteEnabled").toBool());
        shell[state] = false;
    }
    scene["shell"] = shell;
    scene["dialogs"] = QVariantList{QVariantMap{{"id", "modal"}, {"kind", "dialog"},
        {"title", "Dialog"}, {"x", 1}, {"y", 1}, {"w", 30}, {"h", 10}}};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!grid->property("panelClipboardPasteEnabled").toBool());
    scene["dialogs"] = QVariantList{};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(grid->property("panelClipboardPasteEnabled").toBool());
}

void F4OperationsQueueTests::settingsEmbeddedTableFillsContent()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({
        "id":"hotkey-settings","kind":"dialog","layout":"settings","title":"Settings","x":0,"y":0,"w":100,"h":30,
        "children":[
          {"id":"categories","kind":"table","layoutRole":"navigation","showHeader":false,"columns":[{"width":0}],"rows":[{"cells":["Hotkey Configurator"]}]},
          {"id":"section-title","kind":"text","layoutRole":"content-title","text":"Hotkey Configurator"},
          {"id":"page","kind":"group","layoutRole":"content","fullPage":true,"x":28,"y":3,"w":70,"h":24,"children":[
            {"id":"hotkey-table","kind":"table","x":28,"y":3,"w":70,"h":21,"showHeader":true,"sortable":true,"quickSearch":true,"columns":[{"title":"Command","width":20},{"title":"Chord","width":10},{"title":"Area","width":10},{"title":"Condition","width":10},{"title":"Description","width":20}],"rows":[{"cells":["Open","F3","Shell","","View file"]}]},
            {"id":"hotkey-assign","kind":"button","x":28,"y":26,"w":12,"h":1,"text":"Assign"},
            {"id":"hotkey-unbind","kind":"button","x":41,"y":26,"w":12,"h":1,"text":"Unbind"}
          ]}
        ]})").toVariant().toMap();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    auto *body = visualItem(root, "settingsDialogBody");
    auto *page = visualItem(root, "dialogWidget-pageRoot");
    auto *table = visualItem(root, "dialogWidget-hotkey-tableRoot");
    auto *assign = visualItem(root, "dialogWidget-hotkey-assignButton");
    QVERIFY(body && page && table && table->isVisible() && assign);
    QTRY_VERIFY(qAbs(page->width() - body->property("mainWidth").toReal()) < 1);
    QTRY_VERIFY(qAbs(table->width() - page->width()) < 1);
    QVERIFY(table->height() > 100);
    QVERIFY(assign->mapToItem(page, QPointF()).y() >= table->height());
    QVERIFY(assign->mapToItem(page, QPointF(0, assign->height())).y() <= page->height() + 1);
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(assign));
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &action) {
        return action.value("target").toString() == "hotkey-assign"
            && action.value("action").toString() == "control.activate";
    }));
}

void F4OperationsQueueTests::settingsHeaderShowsActiveSection()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({
        "id":"section-title-settings","kind":"dialog","layout":"settings","title":"Settings","x":0,"y":0,"w":100,"h":30,
        "children":[
          {"id":"categories","kind":"table","layoutRole":"navigation","showHeader":false,"columns":[{"width":0}],"rows":[{"cells":["Appearance"]}]},
          {"id":"section-title","kind":"text","layoutRole":"content-title","text":"Appearance"},
          {"id":"page","kind":"group","layoutRole":"content","children":[]}
        ]})").toVariant().toMap();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    auto *title = visualItem(root, "semanticDialogTitle");
    auto *body = visualItem(root, "settingsDialogBody");
    auto *contentTitle = visualItem(root, "dialogWidget-section-titleRoot");
    QVERIFY(title && body && contentTitle);
    QTRY_COMPARE(title->property("text").toString(), QString("Appearance"));
    QVERIFY(!contentTitle->isVisible());
    auto children = dialog.value("children").toList();
    auto section = children[1].toMap();
    section["text"] = "File groups";
    children[1] = section;
    dialog["children"] = children;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_COMPARE(title->property("text").toString(), QString("File groups"));
    body->setProperty("selectedNativePage", "gui");
    QTRY_COMPARE(title->property("text").toString(), QString("GUI"));
    body->setProperty("selectedNativePage", "theme");
    QTRY_COMPARE(title->property("text").toString(), QString("Theme creator"));
    body->setProperty("selectedNativePage", "");
    QTRY_COMPARE(title->property("text").toString(), QString("File groups"));
}

void F4OperationsQueueTests::settingsSearchToolsUseIcons()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({"kind":"dialog","id":"settings-search","layout":"settings","title":"Settings",
      "x":0,"y":0,"w":100,"h":35,"showClose":true,"showZoom":true,"children":[
      {"id":"search-label","kind":"text","layoutRole":"search-label","text":"Search:"},
      {"id":"search","kind":"edit","layoutRole":"search","text":"font","focused":true},
      {"id":"clear","kind":"button","layoutRole":"search-clear","text":"Clear search"},
      {"id":"previous","kind":"button","layoutRole":"search-previous","text":"Previous match"},
      {"id":"next","kind":"button","layoutRole":"search-next","text":"Next match"},
      {"id":"matches","kind":"text","layoutRole":"search-matches","text":"Matches: 1"},
      {"id":"categories","kind":"table","layoutRole":"navigation","cursor":0,"showHeader":false,
       "columns":[{"width":0}],"rows":[{"cells":["Appearance"]}],"itemIcons":["palette"],"itemStates":[{"dimmed":true}]}
    ]})").object().toVariantMap();
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene, false, true);
    auto *root = fixture.window->contentItem();
    auto *searchLabel = visualItem(root, "dialogWidget-search-labelText");
    auto *matchesLabel = visualItem(root, "dialogWidget-matchesText");
    QVERIFY(!searchLabel || !searchLabel->isVisible());
    QVERIFY(matchesLabel);
    auto *searchField = visualItem(root, "dialogWidget-searchEdit");
    QVERIFY(searchField);
    QCOMPARE(searchField->property("placeholderText").toString(), QString("Search"));
    QVERIFY(searchField->property("leadingIconSource").toUrl().isEmpty());
    QVERIFY(searchField->property("leadingInset").toReal() > 0);
    auto *surface = visualItem(root, "semanticDialog-settings-search");
    auto *body = visualItem(root, "settingsDialogBody");
    auto *sidebar = visualItem(root, "settingsSidebarBackground");
    QVERIFY(surface && body && sidebar);
    QCOMPARE(body->x(), 0.0);
    QVERIFY(sidebar->isVisible());
    auto *row = visualItem(root, "dialogWidget-categoriesTableRow-0");
    QVERIFY(row);
    QVERIFY(qAbs(row->height() - 32) <= 1 / fixture.window->devicePixelRatio());
    QVERIFY(qAbs(row->property("radius").toReal() - 8) <= 1 / fixture.window->devicePixelRatio());
    auto *badge = visualItem(root, "settingsCategoryIconBadge-0");
    QVERIFY(badge && badge->isVisible());
    QVERIFY(qAbs(badge->width() - 20) <= 1 / fixture.window->devicePixelRatio());
    QVERIFY(qAbs(badge->x() - 9) <= 1 / fixture.window->devicePixelRatio());
    auto *categoryCaption = visualItem(root, "dialogWidget-categoriesTableCell-0-0");
    QVERIFY(categoryCaption);
    QVERIFY(qAbs(categoryCaption->x() - 36) <= 1 / fixture.window->devicePixelRatio());
    auto *fieldBackground = visualItem(root, "dialogWidget-searchEditBackground");
    QVERIFY(fieldBackground);
    QVERIFY(qAbs(searchField->height() - 28) <= 1 / fixture.window->devicePixelRatio());
    QVERIFY(qAbs(fieldBackground->property("radius").toReal() - searchField->height()/2) <= 1 / fixture.window->devicePixelRatio());
    auto *searchRing = visualItem(root, "dialogWidget-searchEditFocusRing");
    QVERIFY(searchRing && searchRing->isVisible());
    QVERIFY(qAbs(searchField->property("focusRingWidth").toReal() - 3) <= 1 / fixture.window->devicePixelRatio());
    QVERIFY(badge->property("color").value<QColor>() != fixture.window->property("textColor").value<QColor>());
    for (auto *leaf : {matchesLabel}) {
        const QPointF origin = leaf->mapToItem(root, QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY(qAbs(physical.x() - qRound64(physical.x())) < .02);
        QVERIFY(qAbs(physical.y() - qRound64(physical.y())) < .02);
        QCOMPARE(leaf->mapToItem(root, QPointF(1,0)) - origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(root, QPointF(0,1)) - origin, QPointF(0,1));
    }
    const QStringList ids{"clear", "previous", "next"};
    const QStringList icons{"x", "chevron-up", "chevron-down"};
    for (int i = 0; i < ids.size(); ++i) {
        auto *button = visualItem(root, "dialogWidget-" + ids[i] + "Button");
        QVERIFY(button);
        const QRectF fieldRect(0, 0, searchField->width(), searchField->height());
        QVERIFY(fieldRect.contains(button->mapToItem(searchField, QPointF(button->width()/2, button->height()/2))));
        QVERIFY(button->property("flat").toBool());
        auto *background = visualItem(root, button->objectName() + "Background");
        QVERIFY(background);
        QCOMPARE(button->width(), button->height());
        QVERIFY(qAbs(background->property("radius").toReal() - button->height()/2)
                <= 1 / fixture.window->devicePixelRatio());
        QCOMPARE(button->property("text").toString(), QString());
        QVERIFY(!button->property("toolTipText").toString().isEmpty());
        auto *icon = visualItem(root, button->objectName() + "Icon");
        QVERIFY(icon);
        QTRY_COMPARE(icon->property("status").toInt(), 1);
        QCOMPARE(F4IconProvider::decodeRouteValue(icon->property("source").toUrl().path().section('/', -1)), icons[i]);
        QTRY_VERIFY(qAbs(icon->mapToItem(root, QPointF()).x() * fixture.window->devicePixelRatio()
                         - qRound64(icon->mapToItem(root, QPointF()).x() * fixture.window->devicePixelRatio())) < .02);
        QTRY_VERIFY(qAbs(icon->mapToItem(root, QPointF()).y() * fixture.window->devicePixelRatio()
                         - qRound64(icon->mapToItem(root, QPointF()).y() * fixture.window->devicePixelRatio())) < .02);
        const QPointF origin = icon->mapToItem(root, QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY(qAbs(physical.x() - qRound64(physical.x())) < .02);
        QVERIFY(qAbs(physical.y() - qRound64(physical.y())) < .02);
        QCOMPARE(icon->mapToItem(root, QPointF(1,0)) - origin, QPointF(1,0));
        QCOMPARE(icon->mapToItem(root, QPointF(0,1)) - origin, QPointF(0,1));
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(button));
        QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [&](const auto &action) {
            return action.value("target").toString() == ids[i] && action.value("action").toString() == "control.activate";
        }));
    }
    auto *label = visualItem(root, "dialogWidget-categoriesTableCell-0-0");
    auto *categoryIcon = visualItem(root, "dialogWidget-categoriesTableItemIcon-0");
    QVERIFY(label && categoryIcon);
    QCOMPARE(label->property("color"), fixture.window->property("mutedText"));
    const auto mutedSource = categoryIcon->property("source").toUrl();
    for (auto *leaf : {label, categoryIcon}) {
        const QPointF origin = leaf->mapToItem(root, QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY(qAbs(physical.x() - qRound64(physical.x())) < .02);
        QVERIFY(qAbs(physical.y() - qRound64(physical.y())) < .02);
        QCOMPARE(leaf->mapToItem(root, QPointF(1,0)) - origin, QPointF(1,0));
        QCOMPARE(leaf->mapToItem(root, QPointF(0,1)) - origin, QPointF(0,1));
    }
    const auto capture = fixture.window->grabWindow();
    QVERIFY(!capture.isNull());
    if (qEnvironmentVariableIsSet("F4_SETTINGS_SEARCH_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_SETTINGS_SEARCH_CAPTURE")));
    auto children = dialog.value("children").toList();
    auto categories = children.last().toMap();
    categories["itemStates"] = QVariantList{QVariantMap{{"dimmed", false}}};
    children.last() = categories;
    dialog["children"] = children;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_COMPARE(label->property("color"), fixture.window->property("textColor"));
    QTRY_VERIFY(categoryIcon->property("source").toUrl() != mutedSource);
    const qreal searchWidth = searchField->width();
    children = dialog.value("children").toList();
    for (auto &childValue : children) {
        auto child = childValue.toMap();
        if (child.value("layoutRole") == "search") child["text"] = "";
        if (QStringList{"search-clear", "search-previous", "search-next", "search-matches"}
                .contains(child.value("layoutRole").toString())) child["visible"] = false;
        childValue = child;
    }
    dialog["children"] = children;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!searchField->property("leadingIconSource").toUrl().isEmpty());
    QCOMPARE(searchField->property("leadingInset").toReal(), 0.0);
    QCOMPARE(searchField->property("trailingInset").toReal(), 0.0);
    QCOMPARE(searchField->width(), searchWidth);
}

void F4OperationsQueueTests::platformDialogControls()
{
    auto scene = dialogScene();
    auto dialog = scene.value("dialogs").toList().first().toMap();
    dialog["showClose"] = true;
    dialog["showZoom"] = true;
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene, false, true);
    auto *root = fixture.window->contentItem();
    auto *surface = visualItem(root, "semanticDialog-" + dialog.value("id").toString());
    QVERIFY(surface);
    auto *close = visualItem(surface, "dialogCloseButton");
    auto *zoom = visualItem(surface, "dialogMaximizeButton");
    auto *minimize = visualItem(surface, "dialogMinimizeButton");
    QVERIFY(close && zoom && minimize);
    for (bool mac : {true, false}) {
        QVERIFY(surface->setProperty("macWindowControls", mac));
        QCoreApplication::processEvents();
        QCOMPARE(minimize->isVisible(), mac);
        QVERIFY(!minimize->isEnabled());
        const qreal closeX = close->mapToItem(surface, QPointF()).x();
        const qreal zoomX = zoom->mapToItem(surface, QPointF()).x();
        if (mac) {
            QVERIFY(closeX < zoomX);
            QVERIFY(closeX < surface->width() / 2);
            auto *circle = visualItem(close, "dialogCloseBackground");
            QVERIFY(circle);
            QCOMPARE(circle->property("color").value<QColor>(), QColor("#ff5f57"));
            QVERIFY(qAbs(circle->width() - 14) <= 1 / fixture.window->devicePixelRatio());
            QVERIFY(circle->property("antialiasing").toBool());
            QVERIFY(circle->property("gradient").value<QJSValue>().toQObject());
            QVERIFY(qAbs(QQmlProperty::read(circle, "border.width").toReal()
                         * fixture.window->devicePixelRatio() - 1) < .001);
            QTRY_VERIFY2(qAbs(circle->mapToItem(close, QPointF(circle->width()/2, circle->height()/2)).y()
                              - close->height()/2) <= 1 / fixture.window->devicePixelRatio(),
                         qPrintable(QString("circle y=%1 height=%2 parent=%3 buttonHeight=%4")
                             .arg(circle->y()).arg(circle->height()).arg(circle->parentItem()->height()).arg(close->height())));
        } else {
            QVERIFY(closeX > zoomX);
            QVERIFY(closeX > surface->width() / 2);
        }
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(zoom));
        QTRY_VERIFY(surface->property("maximized").toBool());
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(zoom));
        QTRY_VERIFY(!surface->property("maximized").toBool());
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(close));
        QTRY_VERIFY(!fixture.shell.actions.isEmpty());
        QCOMPARE(fixture.shell.actions.constLast().value("action").toString(), QString("dialog.close"));
        QCOMPARE(fixture.shell.actions.constLast().value("target"), dialog.value("id"));
    }
}

void F4OperationsQueueTests::settingsFooterUsesPlatformButtonOrder()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({"kind":"dialog","id":"native-button-order",
      "layout":"settings","title":"Settings","showClose":true,"children":[
      {"id":"order-apply","kind":"button","layoutRole":"apply","text":"Apply"},
      {"id":"order-ok","kind":"button","layoutRole":"accept","text":"OK"},
      {"id":"order-cancel","kind":"button","layoutRole":"cancel","text":"Cancel"}]})").object().toVariantMap();
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene, false, true);
    auto *root = fixture.window->contentItem();
    auto *box = visualItem(root, "settingsButtonBox");
    QVERIFY(box);
    QTRY_COMPARE(box->property("count").toInt(), 3);
#ifdef Q_OS_MACOS
    if (QGuiApplication::platformName() == "cocoa")
        QCOMPARE(box->property("buttonLayout").toInt(), 1); // Native Cocoa policy.
#endif
    const QStringList ids{"apply", "ok", "cancel"};
    const auto visualOrder = [&]() {
        QList<QPair<qreal, QString>> positions;
        for (const auto &id : ids) {
            auto *button = visualItem(root, "dialogWidget-order-" + id + "Button");
            if (button && button->isVisible())
                positions.append({button->mapToItem(box, QPointF()).x(), id});
        }
        std::sort(positions.begin(), positions.end());
        QStringList order;
        for (const auto &position : positions) order.append(position.second);
        return order;
    };
    for (const auto &layout : QList<QPair<int, QStringList>>{
             {0, {"ok", "cancel", "apply"}}, // WinLayout
             {1, {"apply", "cancel", "ok"}}, // MacLayout
             {2, {"ok", "apply", "cancel"}}, // KdeLayout
             {3, {"apply", "cancel", "ok"}}}) { // GnomeLayout
        QVERIFY(box->setProperty("buttonLayout", layout.first));
        QTRY_COMPARE(visualOrder(), layout.second);
        for (const auto &id : ids) {
            auto *button = visualItem(root, "dialogWidget-order-" + id + "Button");
            QVERIFY(button);
            fixture.shell.clearActions();
            QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, itemCenter(button));
            const auto activations = [&]() {
                QStringList targets;
                for (const auto &action : fixture.shell.actions)
                    if (action.value("action").toString() == "control.activate")
                        targets.append(action.value("target").toString());
                return targets;
            };
            QTRY_COMPARE(activations(), (QStringList{"order-" + id}));
        }
    }
    dialog["settingsSectionsOnly"] = true;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_COMPARE(box->property("count").toInt(), 2);
    QTRY_COMPARE(visualOrder(), (QStringList{"cancel", "ok"}));
}

void F4OperationsQueueTests::settingsCategoryIconsStayPixelAligned()
{
    const QStringList icons{"palette", "circle-play", "panels-top-left", "columns-2", "hard-drive", "copy",
        "file-pen-line", "file-code", "keyboard", "square-terminal", "clock-3", "file-type", "menu", "network",
        "file-text", "refresh-cw", "plug", "sparkles", "file-cog"};
    const QStringList captions{"Appearance & language", "Startup & profile", "Workspaces & saving", "Panels",
        "Drive chooser", "File operations", "Editor & viewer", "Syntax highlighting", "Keyboard & shortcuts",
        "Terminal & environment", "History & bookmarks", "File associations", "User menus & macros",
        "Network & connections", "Metadata & reports", "Updates", "Plugins", "AI", "Contributed settings"};
    QVariantList rows;
    for (const auto &caption : captions)
        rows.append(QVariantMap{{"cells", QStringList{caption}}});
    auto scene = panelScene();
    QVariantMap table{{"id", "categories"}, {"kind", "table"}, {"x", 2}, {"y", 2}, {"w", 34}, {"h", 28},
        {"showHeader", false}, {"cursor", 8}, {"columns", QVariantList{QVariantMap{{"width", 0}}}},
        {"rows", rows}, {"itemIcons", icons}};
    QVariantMap list{{"id", "sample-list"}, {"kind", "listBox"}, {"x", 39}, {"y", 2}, {"w", 25}, {"h", 12},
        {"items", QStringList{"Appearance", "Plain row", "Keyboard"}}, {"itemIcons", QStringList{"palette", "", "keyboard"}}};
    scene.insert("dialogs", QVariantList{QVariantMap{{"id", "category-icons"}, {"kind", "dialog"}, {"title", "Settings"},
        {"x", 0}, {"y", 0}, {"w", 67}, {"h", 32}, {"children", QVariantList{table, list}}}});
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    fixture.window->resize(900, 850);
    auto *root = fixture.window->contentItem();
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto checkGrid = [&](QQuickItem *item) {
        QVERIFY(item);
        const QPointF origin = item->mapToItem(root, QPointF());
        const QPointF physical = origin * dpr;
        QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
            qPrintable(QString("%1 at (%2, %3) px").arg(item->objectName()).arg(physical.x()).arg(physical.y())));
        QVERIFY(QLineF(item->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0)).length() < .0001);
        QVERIFY(QLineF(item->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1)).length() < .0001);
    };
    for (const auto &size : {QSize(900, 850), QSize(753, 797)}) {
        fixture.window->resize(size);
        QTest::qWait(40);
        auto *selectedRow = visualItem(root, "dialogWidget-categoriesTableRow-8");
        QVERIFY(selectedRow);
        QVERIFY(selectedRow->property("radius").toReal() >= 3);
        QTest::qWait(100);
        QVERIFY(!fixture.window->grabWindow().isNull());
        for (int i = 0; i < icons.size(); ++i) {
            auto *icon = visualItem(root, "dialogWidget-categoriesTableItemIcon-" + QString::number(i));
            auto *label = visualItem(root, "dialogWidget-categoriesTableCell-" + QString::number(i) + "-0");
            checkGrid(icon);
            checkGrid(label);
            QVERIFY(icon && label);
            QCOMPARE(icon->property("status").toInt(), 1); // Image.Ready: embedded asset and raster provider.
            QCOMPARE(F4IconProvider::decodeRouteValue(icon->property("source").toUrl().path().section('/', -1)), icons[i]);
            QVERIFY(QFile::exists(":/F4QtHost/icons/lucide/" + icons[i] + ".svg"));
            QVERIFY(!label->property("truncated").toBool());
            QVERIFY(label->mapToItem(root, QPointF()).x()
                    >= icon->mapToItem(root, QPointF(icon->width(), 0)).x() + 7);
            QVERIFY(qAbs(icon->width()*dpr-qRound64(icon->width()*dpr)) < .02);
            QVERIFY(qAbs(icon->height()*dpr-qRound64(icon->height()*dpr)) < .02);
        }
        for (int i = 0; i < 3; ++i) {
            checkGrid(visualItem(root, "dialogWidget-sample-listListItemText-" + QString::number(i)));
            auto *icon = visualItem(root, "dialogWidget-sample-listListItemIcon-" + QString::number(i));
            QVERIFY(icon);
            QCOMPARE(icon->isVisible(), i != 1);
            if (i != 1) {
                checkGrid(icon);
                QCOMPARE(icon->property("status").toInt(), 1);
            }
        }
    }
    if (qEnvironmentVariableIsSet("F4_SETTINGS_CATEGORIES_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_SETTINGS_CATEGORIES_CAPTURE")));
}

void F4OperationsQueueTests::settingsListsAndDisabledInputs()
{
    QueueFixture fixture(settingsControlsScene());
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *list = nullptr;
    QTRY_VERIFY((list = visualItem(root, "dialogWidget-listListBox")));
    QTest::qWait(700); // past Basic ScrollBar's idle fade
    auto *inputBg = visualItem(root, "dialogWidget-enabled-editEditBackground");
    QVERIFY(inputBg);
    for (const auto &name : {"listListBackground", "tableTableBackground"}) {
        auto *background = visualItem(root, "dialogWidget-" + QString(name));
        QVERIFY2(background, name);
        QCOMPARE(background->property("color"), inputBg->property("color"));
    }
    for (const auto &name : {"listListScrollBar", "tableTableScrollBar"}) {
        auto *bar = visualItem(root, "dialogWidget-" + QString(name));
        QVERIFY2(bar, name);
        QVERIFY(bar->isVisible());
        QVERIFY(bar->property("size").toReal() < 1);
        auto *handle = bar->property("contentItem").value<QQuickItem *>();
        QVERIFY(handle && handle->isVisible());
        QVERIFY(handle->opacity() > .99);
    }
    for (const auto &name : {"disabled-editEdit", "disabled-comboComboBox"}) {
        auto *control = visualItem(root, "dialogWidget-" + QString(name));
        QVERIFY2(control, name);
        QVERIFY2(!control->isEnabled(), name);
        fixture.shell.clearActions();
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                         control->mapToItem(root, QPointF(control->width()/2, control->height()/2)).toPoint());
        for (const auto &action : std::as_const(fixture.shell.actions))
            QCOMPARE(action.value("action").toString(), QString("control.explain"));
    }
    QVERIFY(visualItem(root, "dialogWidget-disabled-editEditTextInput")->property("color")
            != visualItem(root, "dialogWidget-enabled-editEditTextInput")->property("color"));
    QVERIFY(visualItem(root, "dialogWidget-disabled-comboComboBoxText")->property("color")
            != visualItem(root, "dialogWidget-enabled-comboComboBoxText")->property("color"));
    // Replace overflowing content with a short result (e.g. filtered settings).
    auto compact = settingsControlsScene();
    auto dialog = compact.value("dialogs").toList().first().toMap();
    auto children = dialog.value("children").toList();
    auto listModel = children[0].toMap();
    listModel.insert("items", QStringList{"Only result"});
    children[0] = listModel;
    auto tableModel = children[1].toMap();
    tableModel.insert("rows", QVariantList{tableModel.value("rows").toList().first()});
    children[1] = tableModel;
    dialog.insert("children", children);
    compact.insert("dialogs", QVariantList{dialog});
    fixture.shell.setScene(compact);
    QTRY_VERIFY(!visualItem(root, "dialogWidget-listListScrollBar")->isVisible());
    QTRY_VERIFY(!visualItem(root, "dialogWidget-tableTableScrollBar")->isVisible());
}

void F4OperationsQueueTests::settingsHoverExplainsWithoutFocus()
{
    QueueFixture fixture(settingsControlsScene());
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    const QList<QPair<QString, QString>> targets{
        {"disabled-labelText", "disabled-edit"}, {"disabled-editEdit", "disabled-edit"},
        {"disabled-comboComboBox", "disabled-combo"}, {"after-radiosCheckBox", "after-radios"},
        {"radiosRadio-0", "radios"}, {"radiosRadio-2", "radios"}};
    for (const auto &target : targets) {
        QQuickItem *item = nullptr;
        QTRY_VERIFY((item = visualItem(root, "dialogWidget-" + target.first)));
        QTest::mouseMove(fixture.window, QPoint(1,1), 40);
        auto *focusedInput = visualItem(root, "dialogWidget-enabled-comboComboBox");
        QVERIFY(focusedInput);
        focusedInput->forceActiveFocus();
        QCoreApplication::processEvents();
        QCOMPARE(fixture.window->activeFocusItem(), focusedInput);
        fixture.shell.clearActions();
        QTest::mouseMove(fixture.window,
            item->mapToItem(root, QPointF(item->width()/2, item->height()/2)).toPoint(), 50);
        QTRY_VERIFY2(!fixture.shell.actions.isEmpty(), qPrintable(target.first));
        for (const auto &action : std::as_const(fixture.shell.actions)) {
            QCOMPARE(action.value("action").toString(), QString("control.explain"));
            QCOMPARE(action.value("target").toString(), target.second);
        }
        QCOMPARE(fixture.window->activeFocusItem(), focusedInput);
        if (target.first.startsWith("radios"))
            QCOMPARE(fixture.shell.actions.constLast().value("index").toInt(), target.first.right(1).toInt());
    }
}

void F4OperationsQueueTests::settingsRadiosExpandAndStayPixelAligned()
{
    QueueFixture fixture(settingsControlsScene());
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *radio = nullptr;
    QTRY_VERIFY((radio = visualItem(root, "dialogWidget-radiosRadio-0")));
    QTest::qWait(150);
    const qreal dpr = fixture.window->devicePixelRatio();
    const auto checkGrid = [&](QQuickItem *item) {
        QVERIFY(item);
        const QPointF origin = item->mapToItem(root, QPointF());
        const QPointF physical = origin * dpr;
        QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
                 qPrintable(QString("%1 at (%2, %3) px").arg(item->objectName()).arg(physical.x()).arg(physical.y())));
        QVERIFY(QLineF(item->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0)).length() < .0001);
        QVERIFY(QLineF(item->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1)).length() < .0001);
    };
    for (int i=0; i<3; ++i) {
        const QString prefix = "dialogWidget-radiosRadio-" + QString::number(i);
        for (const auto &part : {"Text", "Indicator", "SelectionMark", "FocusFrame"})
            checkGrid(visualItem(root, prefix + part));
        auto *choice = visualItem(root, prefix);
        QVERIFY2(choice->height() >= 24, qPrintable(QString("choice %1 height=%2").arg(i).arg(choice->height())));
        QVERIFY(!visualItem(root, prefix+"Text")->property("truncated").toBool());
    }
    auto *wrappedText = visualItem(root, "dialogWidget-radiosRadio-1Text");
    auto *wrappedFrame = visualItem(root, "dialogWidget-radiosRadio-1FocusFrame");
    QVERIFY(wrappedText->property("lineCount").toInt() > 1);
    QVERIFY(wrappedFrame->mapToItem(root, QPointF()).y() < wrappedText->mapToItem(root, QPointF()).y());
    QVERIFY(wrappedFrame->mapToItem(root, QPointF(0, wrappedFrame->height())).y()
            > wrappedText->mapToItem(root, QPointF(0, wrappedText->height())).y());
    QVERIFY(!visualItem(root, "dialogWidget-radiosRadio-2")->isEnabled());
    auto *last = visualItem(root, "dialogWidget-radiosRadio-2");
    auto *after = visualItem(root, "dialogWidget-after-radiosCheckBoxText");
    QVERIFY(after->mapToItem(root,QPointF()).y() >= last->mapToItem(root,QPointF(0,last->height())).y());
    for (const auto &name : {"disabled-labelText", "enabled-editEditTextInput", "disabled-editEditTextInput",
            "enabled-comboComboBoxText", "disabled-comboComboBoxText", "enabled-comboComboBoxIndicator",
            "disabled-comboComboBoxIndicator", "after-radiosCheckBoxText", "after-radiosCheckBoxIndicator",
            "okButtonText", "listListItemText-0", "listListItemText-1", "tableTableHeader-0", "tableTableHeader-1",
            "tableTableCell-0-0", "tableTableCell-0-1", "tableTableCell-1-0", "tableTableCell-1-1"})
        checkGrid(visualItem(root, "dialogWidget-" + QString(name)));
    for (const auto &name : {"listListView", "tableTableRows"}) {
        auto *view = visualItem(root, "dialogWidget-" + QString(name));
        QVERIFY(view && view->setProperty("contentY", 11.3));
    }
    QCoreApplication::processEvents();
    QVERIFY(!fixture.window->grabWindow().isNull());
    for (const auto &name : {"listListItemText-1", "tableTableCell-1-0", "tableTableCell-1-1"})
        checkGrid(visualItem(root, "dialogWidget-" + QString(name)));
    for (const auto &name : {"listListScrollBar", "tableTableScrollBar"}) {
        auto *bar = visualItem(root, "dialogWidget-" + QString(name));
        checkGrid(bar);
        auto *handle = bar->property("contentItem").value<QQuickItem *>();
        QVERIFY(handle && !handle->childItems().isEmpty());
        auto *shape = handle->childItems().first();
        checkGrid(shape);
        QVERIFY2(qAbs(shape->width()*dpr-qRound64(shape->width()*dpr)) < .02, name);
        QVERIFY2(qAbs(shape->height()*dpr-qRound64(shape->height()*dpr)) < .02, name);
    }
    fixture.window->setWidth(520);
    QTest::qWait(120);
    QVERIFY2(after->mapToItem(root,QPointF()).y() >= last->mapToItem(root,QPointF(0,last->height())).y(),
             "narrow dialog overlaps the following setting");
    for (int i=0; i<3; ++i) {
        const QString prefix = "dialogWidget-radiosRadio-" + QString::number(i);
        for (const auto &part : {"Text", "Indicator", "SelectionMark", "FocusFrame"})
            checkGrid(visualItem(root, prefix + part));
    }
    QImage capture;
    QTRY_VERIFY(!(capture=fixture.window->grabWindow()).isNull());
    if (qEnvironmentVariableIsSet("F4_SETTINGS_CONTROLS_CAPTURE"))
        QVERIFY(capture.save(qEnvironmentVariable("F4_SETTINGS_CONTROLS_CAPTURE")));
}


// Replay the actual settings owner exports, not a reduced synthetic dialog.
// Export with TestSettingsProfileFixtures (F4_SETTINGS_PROFILE_DIR).
// Run with F4_SETTINGS_REPLAY_DIR; ordinary regression runs skip this benchmark.
void F4OperationsQueueTests::viewerSearchSceneReplayProfile()
{
    const auto directory = qEnvironmentVariable("F4_SEARCH_REPLAY_DIR");
    if (directory.isEmpty()) QSKIP("Opt-in viewer search profile");
    const auto load = [&](const QString &name) {
        QFile file(QDir(directory).filePath(name + ".json"));
        if (!file.open(QIODevice::ReadOnly)) return QVariantMap{};
        return QJsonDocument::fromJson(file.readAll()).object().toVariantMap();
    };
    QueueFixture fixture(load("initial"));
    QVERIFY(fixture.window);
    fixture.window->resize(2194, 1186);
    const auto settle = [&] {
        QCoreApplication::sendPostedEvents(nullptr, QEvent::DeferredDelete);
        QCoreApplication::processEvents();
        fixture.window->grabWindow();
    };
    settle();
    qulonglong searchRevision = 1000;
    for (int cycle=0; cycle<4; ++cycle) {
        for (int hit=0; hit<39; ++hit) {
            for (const auto &phase : {QString("progress-%1").arg(hit,2,10,QChar('0')), QString("hit-%1").arg(hit,2,10,QChar('0'))}) {
                if (qEnvironmentVariableIsSet("F4_SEARCH_REPLAY_SKIP_PROGRESS") && phase.startsWith("progress-")) continue;
                auto scene=load(phase);
                QVERIFY(!scene.isEmpty());
                QElapsedTimer timer; timer.start();
                if (qEnvironmentVariableIsSet("F4_SEARCH_REPLAY_INCREMENTAL")) {
                    ++searchRevision;
                    fixture.shell.overlayState()->applyDialogsState(QVariantMap{{"dialogs", scene.value("dialogs")}}, searchRevision);
                    fixture.shell.surfaceRegistry()->applyDocument(scene.value("surface").toMap(), searchRevision);
                } else {
                    fixture.shell.setScene(scene);
                }
                const auto apply=timer.nsecsElapsed();
                settle();
                qInfo().noquote() << QString("SEARCH_REPLAY cycle=%1 hit=%2 phase=%3 apply_ms=%4 frame_ms=%5")
                    .arg(cycle).arg(hit).arg(phase).arg(apply/1e6,0,'f',3).arg(timer.nsecsElapsed()/1e6,0,'f',3);
            }
        }
    }
}

void F4OperationsQueueTests::settingsSceneReplayProfile()
{
    const auto directory = qEnvironmentVariable("F4_SETTINGS_REPLAY_DIR");
    if (directory.isEmpty())
        QSKIP("Opt-in real Settings scene profiling");
    QHash<QString, QVariantMap> scenes;
    for (const auto &name : {"appearance", "hover-a", "hover-b", "startup", "operations", "editor", "resized"}) {
        QFile file(QDir(directory).filePath(QString::fromLatin1(name) + ".json"));
        QVERIFY2(file.open(QIODevice::ReadOnly), qPrintable(file.errorString()));
        auto scene = panelScene();
        scene.insert("dialogs", QVariantList{QJsonDocument::fromJson(file.readAll()).object().toVariantMap()});
        scenes.insert(QString::fromLatin1(name), scene);
    }
    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);
    fixture.window->resize(1500, 1100);
    const auto settle = [&] {
        for (int i = 0; i < 3; ++i) {
            QCoreApplication::sendPostedEvents(nullptr, QEvent::DeferredDelete);
            QCoreApplication::processEvents();
        }
        fixture.window->grabWindow();
    };
    settle();
    const auto leaves = [&] {
        QList<QPointer<QQuickItem>> result;
        const auto visit = [&](auto &&self, QQuickItem *item) -> void {
            if (item->objectName().startsWith("dialogWidget-"))
                result.append(item);
            for (auto *child : item->childItems()) self(self, child);
        };
        visit(visit, fixture.window->contentItem());
        return result;
    };
    const int repetitions = qMax(1, qEnvironmentVariableIntValue("F4_SETTINGS_REPLAY_REPETITIONS"));
    const auto measure = [&](const char *operation, const auto &change) {
        const auto previous = leaves();
        QElapsedTimer timer;
        timer.start();
        change();
        const auto applyNs = timer.nsecsElapsed();
        settle();
        const auto totalNs = timer.nsecsElapsed();
        int removed = 0;
        for (const auto &item : previous) if (item.isNull()) ++removed;
        qInfo().noquote() << QString("SETTINGS_PROFILE %1 apply_ms=%2 frame_ms=%3 removed=%4 items=%5")
            .arg(operation).arg(applyNs / 1e6, 0, 'f', 3).arg(totalNs / 1e6, 0, 'f', 3)
            .arg(removed).arg(leaves().size());
    };
    measure("open", [&] { fixture.shell.setScene(scenes["appearance"]); });
    for (int i = 0; i < repetitions; ++i) {
        measure("hover-a", [&] { fixture.shell.setScene(scenes["hover-a"]); });
        measure("hover-b", [&] { fixture.shell.setScene(scenes["hover-b"]); });
    }
    for (int i = 0; i < repetitions; ++i) {
        for (const auto &name : {"startup", "operations", "editor", "appearance"})
            measure(name, [&] { fixture.shell.setScene(scenes[QString::fromLatin1(name)]); });
    }
    fixture.shell.setScene(scenes["editor"]);
    settle();
    for (int i = 0; i < repetitions; ++i) {
        measure("resize-native-small", [&] { fixture.window->resize(1100, 800); });
        measure("resize-native-large", [&] { fixture.window->resize(1500, 1100); });
        measure("resize-semantic-large", [&] { fixture.shell.setScene(scenes["resized"]); });
        measure("resize-semantic-small", [&] { fixture.shell.setScene(scenes["editor"]); });
    }
    if (qEnvironmentVariableIsSet("F4_SETTINGS_REPLAY_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_SETTINGS_REPLAY_CAPTURE")));
}


void F4OperationsQueueTests::settingsHelpUpdatePreservesControlIdentity()
{
    auto scene = settingsControlsScene();
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QTest::qWait(40);
    // Use the complete set of control leaves, including nested group controls.
    QHash<QString, QPointer<QQuickItem>> items;
    const auto collect = [&](auto &&self, QQuickItem *item) -> void {
        if (item->objectName().startsWith("dialogWidget-"))
            items.insert(item->objectName(), item);
        for (auto *child : item->childItems()) self(self, child);
    };
    collect(collect, fixture.window->contentItem());
    QVERIFY(items.size() > 20);
    auto dialogs = scene.value("dialogs").toList();
    auto dialog = dialogs[0].toMap();
    auto children = dialog.value("children").toList();
    // An independent explanatory paragraph updates in the same dialog snapshot.
    children.append(QVariantMap{{"kind","text"},{"id","perf-help"},{"x",3},{"y",25},
                                {"w",30},{"h",1},{"text","Changed help paragraph"}});
    dialog.insert("children", children);
    dialogs[0] = dialog;
    scene.insert("dialogs", dialogs);
    fixture.shell.setScene(scene);
    QCoreApplication::sendPostedEvents(nullptr, QEvent::DeferredDelete);
    QCoreApplication::processEvents();
    for (auto it = items.cbegin(); it != items.cend(); ++it) {
        QVERIFY2(!it.value().isNull(), qPrintable("Recreated on help update: " + it.key()));
        QCOMPARE(visualItem(fixture.window->contentItem(), it.key()), it.value().data());
    }
}



void F4OperationsQueueTests::semanticChildrenModelReportsOnlyChangedRows()
{
    SemanticChildrenModel model;
    QAbstractItemModelTester tester(&model, QAbstractItemModelTester::FailureReportingMode::QtTest);
    const QVariantMap label{{"id","label"},{"kind","text"},{"x",2},{"y",1},{"w",5},{"h",1},{"text","Name:"}};
    const QVariantMap edit{{"id","edit"},{"kind","edit"},{"x",8},{"y",1},{"text","before"}};
    QVariantList widgets{label,edit,QVariantMap{{"id","help"},{"kind","text"},{"text","Explanation"}}};
    model.setWidgets(widgets);
    QPersistentModelIndex editIndex(model.index(1));
    QSignalSpy changed(&model, &QAbstractItemModel::dataChanged);
    QSignalSpy reset(&model, &QAbstractItemModel::modelReset);
    QCOMPARE(model.data(editIndex,Qt::UserRole+1).toMap().value("text").toString(), QString("Name:"));
    auto help=widgets[2].toMap(); help["text"]="Updated explanation"; widgets[2]=help;
    model.setWidgets(widgets);
    QCOMPARE(changed.size(),1);
    QCOMPARE(changed[0][0].value<QModelIndex>().row(),2);
    QCOMPARE(editIndex.row(),1);
    QCOMPARE(reset.size(),0);
    changed.clear();
    model.setWidgets(widgets);
    QVERIFY(changed.isEmpty());
    // Reorder and remove while preserving the persistent index for the edit.
    widgets.move(1,0);
    model.setWidgets(widgets);
    QVERIFY(editIndex.isValid());
    QCOMPARE(editIndex.row(),0);
    widgets.removeLast();
    model.setWidgets(widgets);
    QVERIFY(editIndex.isValid());
    // A new control kind with the same ID needs a different visual delegate.
    auto replacement=widgets[0].toMap(); replacement["kind"]="checkbox"; widgets[0]=replacement;
    model.setWidgets(widgets);
    QVERIFY(!editIndex.isValid());
    QCOMPARE(reset.size(),0);
}

void F4OperationsQueueTests::pixelAlignmentUsesSettledAncestorTransforms()
{
    QQmlEngine engine;
    QQmlComponent component(&engine);
    component.setData(R"(
        import QtQuick
        import QtQuick.Window
        import F4QtHost 1.0
        Window {
            id: window; width: 260; height: 200; color: "#18202a"
            property real shift: 0.3
            Item {
                x: 12.7 + window.shift; y: 10.3; width: 199.5; height: 177.1
                transform: Translate { x: window.shift; y: window.shift * 3 }
                Item {
                    id: group; anchors.centerIn: parent; width: 190.3; height: 159.3
                    transform: Translate {
                        x: group.ScenePixelAlignment.offset.x
                        y: group.ScenePixelAlignment.offset.y
                    }
                    Text {
                        id: caption; objectName: "alignmentCaption"
                        anchors.horizontalCenter: parent.horizontalCenter; y: 25.3
                        font.pixelSize: 17; text: "Settings"; color: "white"
                        renderType: Text.NativeRendering
                        transform: Translate { x: caption.ScenePixelAlignment.offset.x; y: caption.ScenePixelAlignment.offset.y }
                    }
                    Text {
                        id: secondary; objectName: "alignmentSecondary"
                        anchors.centerIn: parent; font.pixelSize: 11
                        text: "Small explanation"; color: "#abb8c5"; renderType: Text.NativeRendering
                        transform: Translate { x: secondary.ScenePixelAlignment.offset.x; y: secondary.ScenePixelAlignment.offset.y }
                    }
                    Image {
                        id: icon; objectName: "alignmentIcon"
                        x: 34.1; y: 118.9; width: 16; height: 16
                        source: "qrc:/F4QtHost/icons/lucide/check.svg"
                        transform: Translate { x: icon.ScenePixelAlignment.offset.x; y: icon.ScenePixelAlignment.offset.y }
                    }
                }
            }
        }
    )",QUrl("qrc:/alignment-regression.qml"));
    QScopedPointer<QObject> object(component.create());
    QVERIFY2(object, qPrintable(component.errorString()));
    auto *window=qobject_cast<QQuickWindow *>(object.data());
    QVERIFY(window);
    window->show();
    for (const qreal shift : {0.3, 14.17, -0.33, 1.75, 0.0}) {
        window->setProperty("shift",shift);
        QCoreApplication::processEvents();
        const auto capture=window->grabWindow();
        QVERIFY(!capture.isNull());
        for (const auto &name : {"alignmentCaption","alignmentSecondary","alignmentIcon"}) {
            auto *leaf=visualItem(window->contentItem(),QString::fromLatin1(name));
            QVERIFY(leaf);
            const auto origin=leaf->mapToItem(window->contentItem(),QPointF{});
            const auto physical=origin*window->devicePixelRatio();
            QVERIFY2(qAbs(physical.x()-qRound(physical.x())) < .001
                     && qAbs(physical.y()-qRound(physical.y())) < .001,
                     qPrintable(QString("%1 (%2, %3) physical px").arg(name).arg(physical.x()).arg(physical.y())));
            QCOMPARE(leaf->mapToItem(window->contentItem(),QPointF(1,0))-origin,QPointF(1,0));
            QCOMPARE(leaf->mapToItem(window->contentItem(),QPointF(0,1))-origin,QPointF(0,1));
        }
        if (qEnvironmentVariableIsSet("F4_PIXEL_ALIGNMENT_CAPTURE"))
            QVERIFY(capture.save(qEnvironmentVariable("F4_PIXEL_ALIGNMENT_CAPTURE")));
    }
}

void F4OperationsQueueTests::overlayModelPreservesIdentityAndExitLifecycle()
{
    SemanticOverlayModel model;
    const QVariantMap dialog{{"id", "settings"}, {"kind", "dialog"}};
    const QVariantMap menu{{"id", "choice"}, {"kind", "menu"}, {"presentation", "dropdown"}};
    model.setFrames({dialog, menu});
    QPersistentModelIndex dialogIndex(model.index(0));
    QPersistentModelIndex menuIndex(model.index(1));
    QSignalSpy reset(&model, &QAbstractItemModel::modelReset);
    model.setFrames({dialog});
    QCOMPARE(model.rowCount(), 2);
    QVERIFY(model.data(menuIndex, Qt::UserRole + 1).toBool());
    model.setFrames({menu, dialog});
    QCOMPARE(menuIndex.row(), 0);
    QCOMPARE(dialogIndex.row(), 1);
    QVERIFY(!model.data(menuIndex, Qt::UserRole + 1).toBool());
    model.finishExit("menu||choice"); // An old animation cannot remove a reopened menu.
    QCOMPARE(model.rowCount(), 2);
    model.setFrames({dialog});
    model.finishExit("menu||choice");
    QCOMPARE(model.rowCount(), 1);
    QVERIFY(dialogIndex.isValid());
    QCOMPARE(dialogIndex.row(), 0);
    QVERIFY(!menuIndex.isValid());
    QCOMPARE(reset.size(), 0);
    model.setFrames({});
    QCOMPARE(model.rowCount(), 0);
}

void F4OperationsQueueTests::menuBarPressDragReleaseActivatesItem()
{
    QFETCH(int, releaseIndex);
    auto scene = panelScene();
    // F9 exposes the panel-level bar; inactive bars are deliberately hidden.
    // Pressing another caption must open its menu before release.
    QVariantMap bar{{"id", "main-menu"}, {"kind", "menu"}, {"active", true}, {"selected", 1},
        {"items", QVariantList{QVariantMap{{"index", 0}, {"text", "Files"}},
                              QVariantMap{{"index", 1}, {"text", "Other"}}}}};
    scene.insert("menuBar", bar);
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QTest::qWait(80);
    auto *root = fixture.window->contentItem();
    auto *caption = visualItem(root, "semanticMenuBarItem-0");
    QVERIFY(caption);
    const auto start = caption->mapToScene(QPointF(caption->width()/2, caption->height()/2)).toPoint();
    fixture.shell.clearActions();
    QTest::mousePress(fixture.window, Qt::LeftButton, Qt::NoModifier, start);
    QVERIFY2(!fixture.shell.actions.isEmpty(), "Menu must open before mouse release");
    QVERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(),
        [](const QVariantMap &action) { return action.value("action") == "menuBar.toggle"; }));
    bar.insert("active", true);
    bar.insert("selected", 0);
    scene.insert("menuBar", bar);
    scene.insert("menus", QVariantList{QVariantMap{{"id", "files-menu"}, {"kind", "menu"},
        {"role", "vmenu"}, {"menuBarSubmenu", true}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"index", 0}, {"text", "Open"}},
            QVariantMap{{"index", 1}, {"text", "Edit"}},
            QVariantMap{{"index", 2}, {"text", "Disabled"}, {"disabled", true}},
            QVariantMap{{"index", 3}, {"separator", true}}}}}});
    fixture.shell.setScene(scene);
    QTest::qWait(50);
    auto *row = visualItem(root, QString("semanticMenuItem-files-menu-%1").arg(qMax(0, releaseIndex)));
    QVERIFY(row);
    auto end = row->mapToScene(QPointF(row->width()/2, row->height()/2)).toPoint();
    if (releaseIndex == -1) end = QPoint(fixture.window->width()-10, fixture.window->height()-10);
    if (releaseIndex == -2) end = start;
    fixture.shell.clearActions();
    QCursor::setPos(fixture.window->mapToGlobal(end));
    QTest::mouseMove(fixture.window, end);
    QTest::qWait(30);
    if (releaseIndex == 1) QVERIFY(fixture.window->property("menuBarPointerHasSelectedItem").toBool());
    QTest::mouseRelease(fixture.window, Qt::LeftButton, Qt::NoModifier, end);
    int activations = 0;
    for (const auto &action : std::as_const(fixture.shell.actions)) {
        if (action.value("action") != "menuBar.itemActivate") continue;
        ++activations;
        QCOMPARE(action.value("index").toInt(), 1);
        QCOMPARE(action.value("menuIndex").toInt(), 0);
    }
    QCOMPARE(activations, releaseIndex == 1 ? 1 : 0);
}

void F4OperationsQueueTests::menuBarPressDragReleaseActivatesItem_data()
{
    QTest::addColumn<int>("releaseIndex");
    QTest::newRow("choose") << 1;
    QTest::newRow("disabled") << 2;
    QTest::newRow("separator") << 3;
    QTest::newRow("outside") << -1;
    QTest::newRow("click-opens") << -2;
}

void F4OperationsQueueTests::menuBarClosingDoesNotFlashFirstRow_data()
{
    QTest::addColumn<QString>("closeRoute");
    QTest::addColumn<int>("selectedIndex");
    QTest::newRow("outside") << QStringLiteral("outside") << 1;
    QTest::newRow("caption") << QStringLiteral("caption") << 1;
    QTest::newRow("activate") << QStringLiteral("activate") << 1;
    QTest::newRow("inactive-before-popup-removal") << QStringLiteral("inactive") << 1;
    QTest::newRow("caption-without-hover") << QStringLiteral("caption") << -1;
    QTest::newRow("inactive-without-hover") << QStringLiteral("inactive") << -1;
}

void F4OperationsQueueTests::menuBarClosingDoesNotFlashFirstRow()
{
    QFETCH(QString, closeRoute);
    QFETCH(int, selectedIndex);
    auto scene = panelScene();
    QVariantMap bar{{"id", "main-menu"}, {"kind", "menu"}, {"active", true}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"index", 0}, {"text", "Files"}}}}};
    scene.insert("menuBar", bar);
    scene.insert("menus", QVariantList{QVariantMap{{"id", "files-menu"}, {"kind", "menu"},
        {"role", "vmenu"}, {"menuBarSubmenu", true}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"index", 0}, {"text", "Open"}},
            QVariantMap{{"index", 1}, {"text", "Edit"}}}}}});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *popup = nullptr;
    QQuickItem *row = nullptr;
    QTRY_VERIFY((popup = visualItem(root, "semanticMenuPopup-files-menu")) && popup->isVisible());
    QTRY_VERIFY((row = visualItem(root, "semanticMenuItem-files-menu-1")));
    auto *overlay = popup->parentItem();
    QVERIFY(overlay);
    const auto rowPoint = row->mapToScene(QPointF(row->width()/2, row->height()/2)).toPoint();
    QTest::mouseMove(fixture.window, QPoint(fixture.window->width()-12, fixture.window->height()-12));
    QVERIFY(fixture.window->setProperty("menuBarOpenedByPointer", true));
    if (selectedIndex >= 0) {
        QCursor::setPos(fixture.window->mapToGlobal(rowPoint));
        QTest::mouseMove(fixture.window, rowPoint);
        QTest::qWait(20);
        QCursor::setPos(fixture.window->mapToGlobal(rowPoint + QPoint(3, 0)));
        QTest::mouseMove(fixture.window, rowPoint + QPoint(3, 0));
    }
    QTRY_COMPARE_WITH_TIMEOUT(overlay->property("visualSelectedIndex").toInt(), selectedIndex, 1000);
    QCOMPARE(overlay->property("semanticSelectedIndex").toInt(), 0);
    fixture.shell.clearActions();

    if (closeRoute == "inactive") {
        // Chrome and command menus are separate publications. Go can mark
        // the bar inactive before its popup is removed from the overlay model.
        bar.insert("active", false);
        scene.insert("menuBar", bar);
        fixture.shell.setScene(scene);
    } else {
        QPoint clickPoint = rowPoint;
        if (closeRoute == "outside")
            clickPoint = QPoint(fixture.window->width()-12, fixture.window->height()-12);
        if (closeRoute == "caption") {
            auto *caption = visualItem(root, "semanticMenuBarItem-0");
            QVERIFY(caption);
            clickPoint = caption->mapToScene(QPointF(caption->width()/2, caption->height()/2)).toPoint();
        }
        QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier, clickPoint);
        QVERIFY(!fixture.shell.actions.isEmpty());
    }
    // Deliberately withhold Go's popup-removal acknowledgement. No visible
    // frame may switch from the hovered row back to stale semantic row zero.
    QCoreApplication::processEvents();
    QVERIFY(!fixture.window->grabWindow().isNull());
    QVERIFY2(!popup->isVisible() || overlay->property("visualSelectedIndex").toInt() == selectedIndex,
             "Closing menu repainted stale semantic first-row selection");
    fixture.shell.applyCommandMenus({});
    QTRY_VERIFY(!visualItem(root, "semanticMenuPopup-files-menu"));
}

void F4OperationsQueueTests::helpDialogRendersAndRoutesNavigation()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({"id":"help","kind":"dialog","layout":"help",
      "title":"Help: Contents [topic]","searchHint":"Search: type  Next: F3  Previous: Shift+F3","topic":"Contents","w":76,"h":30,"canGoBack":true,"showClose":true,
      "scrollTop":0,"pageRows":20,"totalRows":100,"stickyRows":1,"helpLines":[
      {"index":0,"centered":true,"spans":[{"text":"Help heading","bold":true,"link":-1}]},
      {"index":1,"centered":false,"spans":[{"text":"Next ","bold":false,"link":0,"selected":true,"searchMatch":true,"searchSelected":false},{"text":"topic","bold":false,"link":0,"selected":true,"searchMatch":true,"searchSelected":true}]},
      {"index":2,"centered":false,"spans":[{"text":"<literal> & text","bold":false,"link":-1}]}
    ]})").object().toVariantMap();
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene, false, true);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QTRY_VERIFY(visualItem(root, "semanticDialog-help"));
    if (qEnvironmentVariable("QT_SCALE_FACTOR") == "1.75")
        QCOMPARE(fixture.window->devicePixelRatio(), 1.75);
    auto *title = visualItem(root, "semanticDialogTitle");
    auto *link = visualItem(root, "semanticHelpLine-1");
    QVERIFY(title && link);
    QCOMPARE(title->property("text").toString(), QString("Help: Contents [topic]"));
    auto *searchHint = visualItem(root, "semanticHelpSearchHint");
    QVERIFY(searchHint && searchHint->isVisible());
    QVERIFY(searchHint->property("text").toString().contains("Shift+F3"));
    QVERIFY(link->property("text").toString().contains("#ffff00"));
    QTest::mouseMove(fixture.window, QPoint(5,5));
    QTest::qWait(60);
    const auto backgroundCapture = fixture.window->grabWindow();
    const auto backgroundPoint = link->mapToScene(QPointF(15,1)) * fixture.window->devicePixelRatio();
    const QColor selectedBackground = fixture.window->property("selectedBg").value<QColor>();
    QCOMPARE(backgroundCapture.pixelColor(backgroundPoint.toPoint()), selectedBackground);
    QVERIFY(link->property("text").toString().contains("href"));
    QVERIFY(visualItem(root, "semanticHelpLine-2")->property("text").toString().contains("&lt;literal&gt;"));
    const auto normalLinkText = link->property("text").toString();
    fixture.shell.clearActions();
    QTest::mouseMove(fixture.window, link->mapToScene(QPointF(15,link->height()/2)).toPoint());
    QTRY_VERIFY(link->property("text").toString() != normalLinkText);
    const auto hoverCapture = fixture.window->grabWindow();
    QVERIFY(!hoverCapture.isNull());
    if (qEnvironmentVariableIsSet("F4_HELP_CAPTURE"))
        QVERIFY(hoverCapture.save(qEnvironmentVariable("F4_HELP_CAPTURE") + ".hover.png"));
    for (const auto &action : std::as_const(fixture.shell.actions))
        QVERIFY2(!action.value("action").toString().startsWith("help.")
                     || action.value("action") == "help.viewport",
                 qPrintable(QJsonDocument::fromVariant(action).toJson()));
    QTest::mouseMove(fixture.window, QPoint(5,5));
    QTRY_COMPARE(link->property("text").toString(), normalLinkText);
    for (const QSize size : {QSize(1024,720), QSize(853,617)}) {
        fixture.window->resize(size);
        QTest::qWait(100);
        QVERIFY(!fixture.window->grabWindow().isNull());
        for (const auto *name : {"semanticDialogTitle", "dialogBackButtonIcon", "semanticHelpSearchHint",
                                "semanticHelpLine-0", "semanticHelpLine-1", "semanticHelpLine-2"}) {
            auto *leaf = visualItem(root, name);
            QVERIFY(leaf && leaf->isVisible());
            const auto origin = leaf->mapToItem(root, QPointF());
            const auto physical = origin * fixture.window->devicePixelRatio();
            QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
                qPrintable(QString("%1 at %2,%3").arg(name).arg(physical.x()).arg(physical.y())));
            const auto extent = QSizeF(leaf->width(), leaf->height()) * fixture.window->devicePixelRatio();
            QVERIFY2(qAbs(extent.width()-qRound64(extent.width())) < .02 && qAbs(extent.height()-qRound64(extent.height())) < .02,
                qPrintable(QString("%1 physical extent %2x%3").arg(name).arg(extent.width()).arg(extent.height())));
            QVERIFY(QLineF(leaf->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0)).length()<.001);
            QVERIFY(QLineF(leaf->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1)).length()<.001);
        }
        for (const auto *name : {"semanticDialog-help", "dialogMoveHandle", "dialogHeaderSeparator",
                                "dialogHeaderFill", "dialogBackButtonBackground", "dialogCloseBackground",
                                "semanticHelpBody", "semanticHelpScrollBarHandle"}) {
            auto *item = visualItem(root, name);
            QVERIFY(item);
            const auto p = item->mapToItem(root, QPointF()) * fixture.window->devicePixelRatio();
            QVERIFY2(qAbs(p.x()-qRound64(p.x())) < .02 && qAbs(p.y()-qRound64(p.y())) < .02,
                qPrintable(QString("%1 at %2,%3").arg(name).arg(p.x()).arg(p.y())));
            QVERIFY(qAbs(item->width()*fixture.window->devicePixelRatio()-qRound64(item->width()*fixture.window->devicePixelRatio())) < .02);
            QVERIFY(qAbs(item->height()*fixture.window->devicePixelRatio()-qRound64(item->height()*fixture.window->devicePixelRatio())) < .02);
        }
        auto *closeButton = visualItem(root, "dialogCloseButton");
        QVERIFY(closeButton);
        auto *closeIcon = visualItem(closeButton, "titleBarButtonIcon");
        QVERIFY(closeIcon);
        const auto origin = closeIcon->mapToItem(root, QPointF());
        const auto physical = origin * fixture.window->devicePixelRatio();
        QVERIFY(qAbs(physical.x()-qRound64(physical.x())) < .02);
        QVERIFY(qAbs(physical.y()-qRound64(physical.y())) < .02);
        QVERIFY(QLineF(closeIcon->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0)).length()<.001);
        QVERIFY(QLineF(closeIcon->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1)).length()<.001);
    }
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     link->mapToScene(QPointF(15, link->height()/2)).toPoint());
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("help.activate"));
    QCOMPARE(fixture.shell.actions.last().value("index").toInt(), 0);
    QCOMPARE(fixture.shell.actions.last().value("topic").toString(), QString("Contents"));
    auto *back = visualItem(root, "dialogBackButton");
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     back->mapToScene(QPointF(back->width()/2,back->height()/2)).toPoint());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("help.back"));
    auto *body = visualItem(root, "semanticHelpBody");
    const auto wheelPoint = body->mapToScene(QPointF(20,30)).toPoint();
    const auto scrollDelta = [&] {
        int total = 0;
        for (const auto &action : std::as_const(fixture.shell.actions))
            if (action.value("action") == "help.scroll") total += action.value("delta").toInt();
        return total;
    };
    fixture.shell.clearActions();
    sendPixelWheel(fixture.window, wheelPoint, -40);
    const int combinedDelta = scrollDelta();
    QVERIFY(combinedDelta > 0);
    sendPixelWheel(fixture.window, wheelPoint, 40);
    fixture.shell.clearActions();
    for (int i = 0; i < 40; ++i) sendPixelWheel(fixture.window, wheelPoint, -1);
    QCOMPARE(scrollDelta(), combinedDelta);
    sendPixelWheel(fixture.window, wheelPoint, -90);
    QTRY_COMPARE(fixture.shell.actions.last().value("action").toString(), QString("help.scroll"));
    QVERIFY(fixture.shell.actions.last().value("delta").toInt() > 0);
    if (qEnvironmentVariableIsSet("F4_HELP_CAPTURE"))
        QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_HELP_CAPTURE")));
    auto *close = visualItem(root, "dialogCloseButton");
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     close->mapToScene(QPointF(close->width()/2,close->height()/2)).toPoint());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("dialog.close"));
    // Back is a common-dialog capability, not another help-specific header.
    dialog = QVariantMap{{"id", "ordinary"}, {"kind", "dialog"}, {"title", "Ordinary dialog"},
                         {"showClose", true}, {"backAction", "navigation.back"}, {"canGoBack", true}};
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(visualItem(root, "semanticDialog-ordinary"));
    QTRY_VERIFY(!visualItem(root, "semanticDialog-help"));
    auto *ordinary = visualItem(root, "semanticDialog-ordinary");
    back = visualItem(ordinary, "dialogBackButton");
    QVERIFY(back && back->isVisible());
    // Let the newly loaded dialog complete its layout before mapping the click.
    QTest::qWait(100);
    QVERIFY(!fixture.window->grabWindow().isNull());
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
                     back->mapToScene(QPointF(back->width()/2,back->height()/2)).toPoint());
    QCOMPARE(fixture.shell.actions.last().value("action").toString(), QString("navigation.back"));
    title = visualItem(ordinary, "semanticDialogTitle");
    const qreal titleWithBack = title->x();
    dialog["canGoBack"] = false;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!back->isVisible());
    QVERIFY(title->x() < titleWithBack);
    dialog["canGoBack"] = true;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(back->isVisible());
    QTRY_COMPARE(title->x(), titleWithBack);
    dialog.remove("backAction");
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QTRY_VERIFY(!back->isVisible());
    qInfo() << "[FIX:help-dialog] shared chrome, optional Back and hover checked at DPR"
            << fixture.window->devicePixelRatio();
}

void F4OperationsQueueTests::adaptiveChoicesAndFilledFields()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({"id":"adaptive","kind":"dialog","w":100,"h":25,"children":[
      {"id":"box","kind":"group","bordered":true,"x":2,"y":2,"w":90,"h":15,"children":[
        {"id":"modes","kind":"radioGroup","title":"Terminal renderer","fillWidth":true,"wrapText":true,"x":4,"y":3,"w":86,"h":3,"items":["ANSI","Windows console"],"selected":0},
        {"id":"path","kind":"edit","fillWidth":true,"x":4,"y":7,"w":86,"h":1,"text":"Profile directory"},
        {"id":"copy","kind":"button","fillWidth":true,"x":4,"y":9,"w":86,"h":1,"text":"Copy profile"}]}]})").toVariant().toMap();
    scene.insert("dialogs", QVariantList{dialog});
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    QTest::qWait(60);
    auto *root = fixture.window->contentItem();
    auto *box = visualItem(root, "dialogWidget-boxRoot");
    auto *modes = visualItem(root, "dialogWidget-modesRoot");
    QVERIFY(box && modes);
    for (const auto width : {700., 330., 180.}) {
        box->setWidth(width);
        auto *metrics = box->property("dialogLayout").value<QObject *>();
        QVERIFY(metrics);
        QQmlProperty::write(metrics, "width", width + 2 * metrics->property("contentPadding").toReal());
        QTest::qWait(60);
        QVERIFY(!fixture.window->grabWindow().isNull());
        for (const auto &id : {"path", "copy"}) {
            auto *field = visualItem(root, QString("dialogWidget-%1Root").arg(id));
            QVERIFY(field);
            const auto rect = field->mapRectToItem(box, QRectF(0,0,field->width(),field->height()));
            QVERIFY2(qAbs(rect.left() - (box->width()-rect.right())) < 1,
                qPrintable(QString("%1 left=%2 right=%3").arg(id).arg(rect.left()).arg(box->width()-rect.right())));
        }
        if (width == 700) {
            const auto original = modes->property("widget").toMap();
            const auto originalFont = fixture.window->property("font").value<QFont>();
            for (const auto &title : {"Startup mode", "Terminal renderer", "Launch defaults"}) {
                for (const int pixels : {12, 13, 14, 16, 18}) {
                    auto font = originalFont;
                    font.setPixelSize(pixels);
                    fixture.window->setProperty("font", font);
                    auto data = original;
                    data["title"] = title;
                    modes->setProperty("widget", data);
                    QTest::qWait(20);
                    auto *label = visualItem(root, "dialogWidget-modesChoiceCaption");
                    QVERIFY(label);
                    QCOMPARE(label->property("lineCount").toInt(), 1);
                    QVERIFY2(label->width() + .001 >= label->implicitWidth(),
                        qPrintable(QString("%1 font=%2 allocated=%3 natural=%4")
                            .arg(title).arg(pixels).arg(label->width()).arg(label->implicitWidth())));
                }
            }
            fixture.window->setProperty("font", originalFont);
            modes->setProperty("widget", original);
            QTest::qWait(30);
        }
        auto *a = visualItem(root, "dialogWidget-modesRadio-0");
        auto *b = visualItem(root, "dialogWidget-modesRadio-1");
        auto *caption = visualItem(root, "dialogWidget-modesChoiceCaption");
        QVERIFY(a && b && caption);
        const auto pa = a->mapToItem(modes, QPointF());
        const auto pb = b->mapToItem(modes, QPointF());
        auto *path = visualItem(root, "dialogWidget-pathRoot");
        QVERIFY(path->mapToItem(box, QPointF()).y() >= b->mapToItem(box, QPointF(0,b->height())).y());

        if (width == 700) { QVERIFY(pa.x() > 100); QVERIFY(qAbs(pa.y()-pb.y()) < 1); }
        else if (width == 330) { QVERIFY(pa.y() >= caption->height()); QVERIFY(qAbs(pa.y()-pb.y()) < 1); }
        else { QVERIFY(pb.y() > pa.y()); QVERIFY(qAbs(pa.x()-pb.x()) < 1); }
        const auto check = [&](auto &&self, QQuickItem *item) -> void {
            if (item->isVisible() && (item->inherits("QQuickText") || item->inherits("QQuickTextInput"))
                && item->objectName().startsWith("dialogWidget-")) {
                const auto origin = item->mapToItem(root, QPointF());
                const auto p = origin * fixture.window->devicePixelRatio();
                QVERIFY2(qAbs(p.x()-qRound64(p.x())) < .02 && qAbs(p.y()-qRound64(p.y())) < .02,
                    qPrintable(QString("%1 %2,%3").arg(item->objectName()).arg(p.x()).arg(p.y())));
                QVERIFY(QLineF(item->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0)).length()<.001);
                QVERIFY(QLineF(item->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1)).length()<.001);
            }
            for (auto *child : item->childItems()) self(self,child);
        };
        check(check,box);
        if (qEnvironmentVariableIsSet("F4_ADAPTIVE_CAPTURE"))
            QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_ADAPTIVE_CAPTURE")+QString::number(width)+".png"));
    }
}

void F4OperationsQueueTests::multilineDialogEditor()
{
    auto scene = panelScene();
    auto dialog = QJsonDocument::fromJson(R"({"kind":"dialog","id":"multiline","title":"Commands","x":2,"y":2,"w":65,"h":20,"children":[
      {"kind":"multiLineEdit","id":"command","x":4,"y":4,"w":45,"h":4,"visible":true,"focused":true,"text":"echo 😀\necho second\nthird\nfourth\nfifth\nsixth","cursor":8,"selectionActive":true,"selectionStart":5,"selectionEnd":8}
    ]})").object().toVariantMap();
    scene["dialogs"] = QVariantList{dialog};
    QueueFixture fixture(scene);
    QVERIFY(fixture.window);
    auto *root = fixture.window->contentItem();
    QQuickItem *edit = nullptr;
    QTRY_VERIFY((edit = visualItem(root, "dialogWidget-commandMultiLineTextEdit")));
    QTRY_COMPARE(edit->property("selectedText").toString(), QString::fromUtf8("😀\ne"));
    QVERIFY(edit->property("readOnly").toBool());
    QVERIFY(edit->property("text").toString().contains("\n"));
    auto *scroll = visualItem(root, "dialogWidget-commandMultiLineVerticalScroll");
    QVERIFY(scroll); QTRY_VERIFY(scroll->isVisible());
    QTest::qWait(100);
    fixture.shell.clearActions();
    QTest::mouseClick(fixture.window, Qt::LeftButton, Qt::NoModifier,
        edit->mapToScene(QPointF(10,10)).toPoint());
    QTRY_VERIFY(!fixture.shell.actions.isEmpty());
    QTRY_VERIFY(std::any_of(fixture.shell.actions.cbegin(), fixture.shell.actions.cend(), [](const auto &a) {
        return a.value("action") == "control.select" && a.value("target") == "command";
    }));
    auto *viewport = visualItem(root, "dialogWidget-commandMultiLineViewport");
    QVERIFY(viewport);
    viewport->setProperty("contentY", 13.3);
    QTest::qWait(80);
    const auto origin = edit->mapToItem(root, QPointF());
    const auto physical = origin * fixture.window->devicePixelRatio();
    QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
        qPrintable(QString("TextEdit physical origin %1,%2").arg(physical.x()).arg(physical.y())));
    QVERIFY(QLineF(edit->mapToItem(root,QPointF(1,0))-origin,QPointF(1,0)).length()<.001);
    QVERIFY(QLineF(edit->mapToItem(root,QPointF(0,1))-origin,QPointF(0,1)).length()<.001);
    QTemporaryDir captureDirectory(QDir::tempPath() + "/f4-multiline-XXXXXX");
    QVERIFY2(captureDirectory.isValid(), "Could not create multiline capture directory");
    const auto capturePath = captureDirectory.filePath("multiline.png");
    QVERIFY2(fixture.window->grabWindow().save(capturePath), qPrintable(capturePath));
    auto children = dialog.value("children").toList();
    auto inputWidget = children.constFirst().toMap();
    inputWidget["focused"] = false;
    children[0] = inputWidget;
    dialog["children"] = children;
    scene["dialogs"] = QVariantList{dialog};
    fixture.shell.setScene(scene);
    QQuickItem *caret = nullptr;
    QTRY_VERIFY((caret = visualItem(root, "dialogWidget-commandMultiLineCursor")));
    QTRY_COMPARE(caret->opacity(), 0.0);
}
