#include "DummyQWK.h"
#include "TestExtUiStateController.h"

#include <QAccessible>
#include <QColor>
#include <QCoreApplication>
#include <QElapsedTimer>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QFont>
#include <QGuiApplication>
#include <QImage>
#include <QQuickItem>
#include <QQuickItemGrabResult>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QPointer>
#include <QScopeGuard>
#include <QStyleHints>
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
    void semanticDialogComboBoxFollowsGoOwnedMenuState();
    void semanticOverlayMenuStreamClearsWhileFallbackSurfaceIsHidden();
    void semanticOverlayDialogStreamClearsWithoutStaleOverlay();
    void semanticDialogRowsExpandForNativeControls();
    void environmentManagerDialogUsesExpandedRows();
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
    QVERIFY(fixture.window->grabWindow().save(QString(".diagnostics/queue-dropdown-%1.png").arg(dpr)));
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
    QVERIFY(fixture.window->grabWindow().save(QString(".diagnostics/queue-resume-%1.png").arg(dpr)));
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
    QCOMPARE(combo->property("count").toInt(), 3);
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
    QTRY_COMPARE_WITH_TIMEOUT(frameModel->property("count").toInt(), 1,
                              1000);
    QVERIFY(visualItem(fixture.window->contentItem(),
                       QStringLiteral("semanticDialog-appearance-dialog")));

    // This is the production transition emitted when Go closes a dialog.
    // The QML overlay must retire it even if the native panel remains alive
    // below the overlay and no unrelated scene change follows.
    fixture.shell.overlayState()->applyDialogsState({
        {QStringLiteral("dialogs"), QVariantList{}},
    }, 2);

    QTRY_COMPARE_WITH_TIMEOUT(frameModel->property("count").toInt(), 0,
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
    const auto restoreFont = qScopeGuard([previousFont]() {
        QGuiApplication::setFont(previousFont);
    });
    QFont dialogFont = previousFont;
    dialogFont.setPixelSize(17);
    dialogFont.setWeight(QFont::Medium);
    QGuiApplication::setFont(dialogFont);

    QueueFixture fixture(panelScene());
    QVERIFY(fixture.window);

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
        listText, groupTitle, nestedText, borderlessGroupTitle,
    };
    const QFont initialWindowFont = fixture.window->property("font").value<QFont>();
    QCOMPARE(initialWindowFont, dialogFont);
    for (QQuickItem *leaf : std::as_const(textLeaves))
        QCOMPARE(leaf->property("font").value<QFont>(), initialWindowFont);
    for (QQuickItem *leaf : {buttonText, checkText, radioText, editText,
                             comboText, plainText, listText, nestedText}) {
        QCOMPARE(leaf->property("color").value<QColor>(), firstText);
    }
    QCOMPARE(groupTitle->property("color").value<QColor>(), firstMuted);
    QCOMPARE(borderlessGroupTitle->property("color").value<QColor>(),
             firstMuted);

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
    QCOMPARE(groupBorder->property("testBorderWidth").toReal(), 1.0);
    QCOMPARE(borderlessGroupBorder->property("testBorderWidth").toReal(), 0.0);

    const QPointF nestedInGroup = nestedRoot->mapToItem(groupRoot, QPointF{});
    const qreal expectedNestedX = qRound(2.0
        * fixture.window->property("cw").toReal());
    const qreal expectedNestedY = qMax(2.0 * qMax(22.0, fixture.window->property("ch").toReal()),
                                          editBackground->height() + 8.0);
    QCOMPARE(nestedInGroup.x(), expectedNestedX);
    QCOMPARE(nestedInGroup.y(), expectedNestedY);
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
                              secondMuted, 3000);
    QTRY_COMPARE_WITH_TIMEOUT(
        borderlessGroupTitle->property("color").value<QColor>(),
        secondMuted, 3000);

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
