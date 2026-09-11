#include <QQmlProperty>
#include "SemanticOverlayModel.h"
#include "DummyQWK.h"
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
#include <QGuiApplication>
#include <QImage>
#include <QJsonDocument>
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
    void driveDetailsUseMeasuredColumnsOnPhysicalPixelGrid();
    void messageBodyWrapsToGuiWidth();
    void settingsSceneReplayProfile();
    void settingsHelpUpdatePreservesControlIdentity();
    void semanticChildrenModelReportsOnlyChangedRows();
    void pixelAlignmentUsesSettledAncestorTransforms();
    void settingsDecorationsAndViewportStayPixelAligned();
    void settingsListsAndDisabledInputs();
    void settingsCategoryListSelectsDuringMouseDrag();
    void settingsCategoryListSelectsDuringMouseDrag_data();
    void settingsListScrollingAndTouch();
    void settingsCategoryIconsStayPixelAligned();
    void settingsResizeKeepsChromeAndScrollsContent();
    void overlayModelPreservesIdentityAndExitLifecycle();
    void adaptiveChoicesAndFilledFields();
    void multilineDialogEditor();
    void menuBarPressDragReleaseActivatesItem();
    void menuBarPressDragReleaseActivatesItem_data();
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
        QVERIFY(qAbs(capacity->y() + capacity->height()/2 - track->y() - track->height()/2) * fixture.window->devicePixelRatio() <= 1);

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
           "scrollable":true,"scrollTop":0,"contentHeight":32,"children":[
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
        const QPointF origin = item->mapToItem(root, QPointF{});
        const QPointF physical = origin * dpr;
        QVERIFY2(qAbs(physical.x() - qRound(physical.x())) < 0.02,
                 qPrintable(QString("%1 x=%2 px").arg(item->objectName()).arg(physical.x(), 0, 'f', 4)));
        QVERIFY2(qAbs(physical.y() - qRound(physical.y())) < 0.02,
                 qPrintable(QString("%1 y=%2 px").arg(item->objectName()).arg(physical.y(), 0, 'f', 4)));
        QVERIFY(QLineF(item->mapToItem(root, QPointF(1, 0)) - origin, QPointF(1, 0)).length() < 0.0001);
        QVERIFY(QLineF(item->mapToItem(root, QPointF(0, 1)) - origin, QPointF(0, 1)).length() < 0.0001);
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
        "description-textText", "applyButtonText", "okButtonText", "cancelButtonText"
    };
    for (const auto &name : names) {
        QQuickItem *leaf = nullptr;
        QTRY_VERIFY2((leaf = visualItem(root, "dialogWidget-" + name)), qPrintable(name));
        checkGrid(leaf);
    }
    for (const auto &group : {"language", "colors", "font", "titles"}) {
        for (const auto &part : {"GroupBorder", "GroupTitleBackground"}) {
            auto *item = visualItem(root, "dialogWidget-" + QString(group) + part);
            QVERIFY(item);
            checkGrid(item);
            QVERIFY2(qAbs(item->width() * dpr - qRound(item->width() * dpr)) < 0.02, qPrintable(item->objectName() + " width"));
            QVERIFY2(qAbs(item->height() * dpr - qRound(item->height() * dpr)) < 0.02, qPrintable(item->objectName() + " height"));
        }
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


void F4OperationsQueueTests::settingsResizeKeepsChromeAndScrollsContent()
{
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
        children[5] = helpModel;
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
        auto *page = visualItem(root, "dialogWidget-pageViewport");
        QVERIFY(page && page->height() > 50);
        QVERIFY(page->property("contentHeight").toReal() > page->height());
        auto *box = visualItem(root, "dialogWidget-boxRoot");
        auto *field = visualItem(root, "dialogWidget-languageRoot");
        QVERIFY(box && field);
        const auto inset = field->mapToItem(box, QPointF()).y();
        QVERIFY2(inset >= 12 && inset <= 20, qPrintable(QString("group top inset=%1").arg(inset)));
        auto *helpRoot = visualItem(root, "dialogWidget-helpRoot");
        auto *helpText = visualItem(root, "dialogWidget-help-textText");
        QVERIFY(helpRoot && helpText);
        const auto helpOrigin = helpText->mapToItem(helpRoot, QPointF());
        QVERIFY2(qAbs(helpOrigin.y()) < 1, qPrintable(QString("help top inset=%1").arg(helpOrigin.y())));
        QVERIFY(qAbs(helpOrigin.x()) < 1);
        QVERIFY(helpRoot->width() - helpText->width() <= 9);
        if (body->property("wideHelp").toBool()) {
            auto *title = visualItem(root, "dialogWidget-category-titleRoot");
            QVERIFY(title);
            const auto gap = helpRoot->mapToItem(root, QPointF()).x()
                - box->mapToItem(root, QPointF(box->width(), 0)).x();
            QVERIFY2(gap >= 11 && gap <= 30, qPrintable(QString("page/help gap=%1").arg(gap)));
            QVERIFY(qAbs(helpText->mapToItem(root, QPointF()).y() - title->mapToItem(root, QPointF()).y()) < 1);
        }
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
                QVERIFY2(qAbs(physical.x()-qRound64(physical.x())) < .02 && qAbs(physical.y()-qRound64(physical.y())) < .02,
                    qPrintable(QString("%1 at %2,%3").arg(item->objectName()).arg(physical.x()).arg(physical.y())));
                QVERIFY(QLineF(item->mapToItem(root, QPointF(1,0))-origin, QPointF(1,0)).length() < .0001);
                QVERIFY(QLineF(item->mapToItem(root, QPointF(0,1))-origin, QPointF(0,1)).length() < .0001);
            }
            for (auto *child : item->childItems()) self(self, child);
        };
        checkLeaves(checkLeaves, surface);
        if (qEnvironmentVariableIsSet("F4_SETTINGS_RESIZE_CAPTURE"))
            QVERIFY(fixture.window->grabWindow().save(qEnvironmentVariable("F4_SETTINGS_RESIZE_CAPTURE")
                + QString(".%1.png").arg(size.width())));

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
    QVariantMap bar{{"id", "main-menu"}, {"kind", "menu"}, {"active", false}, {"selected", 0},
        {"items", QVariantList{QVariantMap{{"index", 0}, {"text", "Files"}}}}};
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
    QVERIFY(fixture.window->grabWindow().save("D:/Code/f4-zoin/.diagnostics/multiline.png"));
}
