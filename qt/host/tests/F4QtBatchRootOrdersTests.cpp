#include <QQuickItem>
#include <QQuickWindow>
#include <QSGSimpleRectNode>
#include <QSGTransformNode>
#include <QScopeGuard>
#include <QtTest>
#include <QtQuick/private/qquickclipnode_p.h>
#include <QtQuick/private/qquickitem_p.h>
#include <QtQuick/private/qquickwindow_p.h>
#define private public
#include <QtQuick/private/qsgbatchrenderer_p.h>
#undef private

#include <atomic>

namespace {

class DrawingNode final : public QSGTransformNode {
public:
    bool isSubtreeBlocked() const override { return blocked; }
    void setBlocked(bool value) {
        if (blocked == value) return;
        blocked = value;
        markDirty(QSGNode::DirtySubtreeBlocked);
    }
private:
    bool blocked = false;
};

class DrawingPrivate final : public QQuickItemPrivate {
public:
    QSGTransformNode *createTransformNode() override { return new DrawingNode; }
};

class DrawingGate final : public QQuickItem {
public:
    explicit DrawingGate(QQuickItem *parent) : QQuickItem(*new DrawingPrivate, parent) {
        setFlag(ItemHasContents);
    }
    void showDrawing(bool value) { shown = value; update(); }
protected:
    QSGNode *updatePaintNode(QSGNode *, UpdatePaintNodeData *data) override {
        static_cast<DrawingNode *>(data->transformNode)->setBlocked(!shown);
        return nullptr;
    }
private:
    bool shown = true;
};

class ColorTile final : public QQuickItem {
public:
    ColorTile(QQuickItem *parent, const QColor &color) : QQuickItem(parent), color(color) {
        setFlag(ItemHasContents);
    }
protected:
    QSGNode *updatePaintNode(QSGNode *node, UpdatePaintNodeData *) override {
        auto *rect = static_cast<QSGSimpleRectNode *>(node);
        if (!rect) rect = new QSGSimpleRectNode;
        rect->setRect(boundingRect());
        rect->setColor(color);
        return rect;
    }
private:
    QColor color;
};

} // namespace

class F4QtBatchRootOrdersTests : public QObject {
    Q_OBJECT
private slots:
    void opacityInsideBlockedDrawingDoesNotRebuildUnrelatedBatches() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QQuickWindow window;
        window.resize(240, 200);
        window.setColor(Qt::black);
        auto *background = new ColorTile(window.contentItem(), Qt::red);
        background->setSize(QSizeF(240, 200));
        auto *clip = new QQuickItem(window.contentItem());
        clip->setSize(QSizeF(240, 200));
        clip->setClip(true);
        auto *gate = new DrawingGate(clip);
        gate->setSize(clip->size());
        gate->showDrawing(false);
        auto *hidden = new ColorTile(gate, Qt::blue);
        hidden->setPosition(QPointF(20, 20));
        hidden->setSize(QSizeF(80, 40));
        hidden->setOpacity(0);
        std::atomic<int> frames = 0;
        std::atomic<int> completeRebuilds = 0;
        connect(&window, &QQuickWindow::afterSynchronizing, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(QQuickWindowPrivate::get(&window)->renderer);
            if (renderer && renderer->m_rebuild == QSGBatchRenderer::Renderer::FullRebuild)
                ++completeRebuilds;
        }, Qt::DirectConnection);
        connect(&window, &QQuickWindow::afterRendering, &window, [&] { ++frames; }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const auto renderNext = [&] {
            const int previous = frames.load();
            window.update();
            return QTest::qWaitFor([&] { return frames.load() > previous; }, 3000);
        };
        QVERIFY(renderNext());
        const QImage baseline = window.grabWindow();
        QVERIFY(!baseline.isNull());
        QCOMPARE(baseline.pixelColor(qRound(40*window.devicePixelRatio()),
                                    qRound(40*window.devicePixelRatio())), QColor(Qt::red));
        for (int cycle = 0; cycle < 3; ++cycle) {
            int previousRebuilds = completeRebuilds.load();
            hidden->setOpacity(1);
            QVERIFY(renderNext());
            QVERIFY2(completeRebuilds.load() == previousRebuilds,
                     "An opacity change in an already blocked subtree rebuilt unrelated batches");
            QCOMPARE(window.grabWindow(), baseline);
            previousRebuilds = completeRebuilds.load();
            hidden->setOpacity(0);
            QVERIFY(renderNext());
            QCOMPARE(completeRebuilds.load(), previousRebuilds);
            QCOMPARE(window.grabWindow(), baseline);
        }
        gate->showDrawing(true);
        hidden->setOpacity(1);
        QVERIFY(renderNext());
        const QImage revealed = window.grabWindow();
        QCOMPARE(revealed.pixelColor(qRound(40*window.devicePixelRatio()),
                                    qRound(40*window.devicePixelRatio())), QColor(Qt::blue));
        // Visible opacity changes must still update blending and drawing order.
        for (qreal opacity : {0.5, 0.0, 1.0, 0.0, 0.5, 1.0}) {
            hidden->setOpacity(opacity);
            QVERIFY(renderNext());
            const auto rendered = window.grabWindow();
            const auto color = rendered.pixelColor(qRound(40*window.devicePixelRatio()),
                                                   qRound(40*window.devicePixelRatio()));
            QVERIFY(qAbs(color.red()-qRound(255*(1-opacity))) <= 1);
            QCOMPARE(color.green(), 0);
            QVERIFY(qAbs(color.blue()-qRound(255*opacity)) <= 1);
        }
    }

    void repeatedDrawingVisibilityReturnsAncestorReservations() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        const QByteArray previousPadding = qgetenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING");
        qputenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING", "1024");
        const auto restorePadding = qScopeGuard([&] {
            if (previousPadding.isNull()) qunsetenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING");
            else qputenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING", previousPadding);
        });
        QQuickWindow window;
        window.resize(240, 200);
        window.setColor(Qt::black);
        auto *outerClip = new QQuickItem(window.contentItem());
        outerClip->setSize(QSizeF(240, 200));
        outerClip->setClip(true);
        QList<ColorTile *> leaves;
        DrawingGate *foreground = nullptr;
        for (int layer = 0; layer < 2; ++layer) {
            auto *clip = new QQuickItem(outerClip);
            clip->setPosition(QPointF(20, 20));
            clip->setSize(QSizeF(180, 160));
            clip->setClip(true);
            auto *gate = new DrawingGate(clip);
            gate->setSize(clip->size());
            if (layer) foreground = gate;
            for (int row = 0; row < 20; ++row) {
                auto *tile = new ColorTile(gate, layer ? QColor(0, 0, 255, 128) : QColor(Qt::red));
                tile->setObjectName(QStringLiteral("batchOrderTile-%1-%2").arg(layer).arg(row));
                tile->setPosition(layer ? QPointF(40, 40 + row*4) : QPointF(0, row*8));
                tile->setSize(layer ? QSizeF(120, 4) : QSizeF(120, 8));
                leaves.append(tile);
            }
        }
        std::atomic<int> frames = 0;
        std::atomic<int> available = -1;
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(QQuickWindowPrivate::get(&window)->renderer);
            auto *clip = QQuickItemPrivate::get(outerClip)->clipNode();
            if (renderer && clip) {
                if (auto *node = renderer->m_nodes.value(clip))
                    available.store(node->clipInfo()->availableOrders);
            }
            ++frames;
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        QTRY_VERIFY(available.load() >= 0);
        const auto renderNext = [&] {
            const int previous = frames.load();
            window.update();
            return QTest::qWaitFor([&] { return frames.load() > previous; }, 3000);
        };
        QVERIFY(renderNext());
        const int initialOrders = available.load();
        const qreal dpr = window.devicePixelRatio();
        for (auto *leaf : leaves) {
            const QPointF origin = leaf->mapToItem(window.contentItem(), QPointF());
            QVERIFY2(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001, qPrintable(leaf->objectName()));
            QVERIFY2(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001, qPrintable(leaf->objectName()));
            QCOMPARE(leaf->mapToItem(window.contentItem(), QPointF(1, 0))-origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToItem(window.contentItem(), QPointF(0, 1))-origin, QPointF(0, 1));
        }
        const QImage baseline = window.grabWindow();
        QVERIFY(!baseline.isNull());
        QVERIFY(baseline.save("qt-batch-root-orders-before-175.png"));
        for (int cycle = 0; cycle < 8; ++cycle) {
            foreground->showDrawing(false);
            QVERIFY(renderNext());
            foreground->showDrawing(true);
            QVERIFY(renderNext());
            QCOMPARE(available.load(), initialOrders);
            QCOMPARE(window.grabWindow(), baseline);
        }
        const QColor overlap = baseline.pixelColor(qRound(70*dpr), qRound(70*dpr));
        QVERIFY(qAbs(overlap.red()-127) <= 1 && overlap.green() == 0 && qAbs(overlap.blue()-128) <= 1);
        QVERIFY(baseline.save("qt-batch-root-orders-after-175.png"));
    }
};

QTEST_MAIN(F4QtBatchRootOrdersTests)
#include "F4QtBatchRootOrdersTests.moc"
