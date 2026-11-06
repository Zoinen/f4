#include <QQuickItem>
#include <QQuickWindow>
#include <QSGSimpleRectNode>
#include <QSGMaterial>
#include <QSGFlatColorMaterial>
#include <QSGRenderNode>
#include <QSGClipNode>
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
#include "VectorUploadGateProtocol.h"

namespace {

std::atomic<quintptr> watchedRoot = 0;
std::atomic<quint64> watchedUploads = 0;
std::atomic<bool> watchFullRebuilds = false;
std::atomic<bool> watchAllUploads = false;
std::atomic<int> fullRebuilds = 0;
QtMessageHandler previousMessageHandler = nullptr;

void watchUploads(QtMsgType type, const QMessageLogContext &context, const QString &message) {
    if (watchFullRebuilds && message.contains("rebuild: full")) ++fullRebuilds;
    const auto root = watchedRoot.load();
    if ((root || watchAllUploads.load()) && message.contains("[  upload]")
        && (watchAllUploads.load() || message.contains(QStringLiteral("root: 0x%1").arg(root, 0, 16))))
        ++watchedUploads;
    if (previousMessageHandler) previousMessageHandler(type, context, message);
}

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
    ColorTile(QQuickItem *parent, const QColor &color, bool noBatch = false)
        : QQuickItem(parent), color(color), noBatch(noBatch) {
        setFlag(ItemHasContents);
    }
    void setColor(const QColor &value) { color = value; update(); }
protected:
    QSGNode *updatePaintNode(QSGNode *node, UpdatePaintNodeData *) override {
        auto *rect = static_cast<QSGSimpleRectNode *>(node);
        if (!rect) rect = new QSGSimpleRectNode;
        rect->setRect(boundingRect());
        rect->setColor(color);
        rect->material()->setFlag(QSGMaterial::NoBatching, noBatch);
        return rect;
    }
private:
    QColor color;
    bool noBatch;
};

// One item publishes a late vector-like burst of otherwise static geometry.
class ColdGeometryTile final : public QQuickItem {
public:
    explicit ColdGeometryTile(QQuickItem *parent) : QQuickItem(parent) {
        setFlag(ItemHasContents);
    }
    void setCount(int value) { count = value; update(); }
protected:
    QSGNode *updatePaintNode(QSGNode *root, UpdatePaintNodeData *) override {
        if (!root) root = new QSGNode;
        while (root->firstChild()) delete root->firstChild();
        for (int i = 0; i < count; ++i)
            root->appendChildNode(new QSGSimpleRectNode(boundingRect(), Qt::green));
        return root;
    }
private:
    int count = 0;
};

class EphemeralClip final : public QQuickItem {
public:
    explicit EphemeralClip(QQuickItem *parent) : QQuickItem(parent) { setFlag(ItemHasContents); }
    void cycle() { pending = true; update(); }
protected:
    QSGNode *updatePaintNode(QSGNode *root, UpdatePaintNodeData *) override {
        if (!root) root = new QSGNode;
        if (pending) {
            auto *clip = new QSGClipNode;
            clip->setIsRectangular(true);
            clip->setClipRect(QRectF(0, 0, 4, 4));
            root->appendChildNode(clip);
            clip->appendChildNode(new QSGSimpleRectNode(QRectF(0, 0, 4, 4), Qt::cyan));
            root->removeChildNode(clip);
            delete clip;
            pending = false;
        }
        return root;
    }
private:
    bool pending = false;
};

class NestedClipTile final : public QQuickItem {
public:
    NestedClipTile(QQuickItem *parent, bool stencil) : QQuickItem(parent), stencil(stencil) {
        setFlag(ItemHasContents);
        setSize(QSizeF(96, 96));
    }
protected:
    QSGNode *updatePaintNode(QSGNode *node, UpdatePaintNodeData *) override {
        if (node) return node;
        auto *clip = new QSGClipNode;
        clip->setIsRectangular(!stencil);
        clip->setClipRect(boundingRect());
        if (stencil) {
            auto *geometry = new QSGGeometry(QSGGeometry::defaultAttributes_Point2D(), 4);
            geometry->setDrawingMode(QSGGeometry::DrawTriangleStrip);
            auto *vertices = geometry->vertexDataAsPoint2D();
            vertices[0].set(0, 0);
            vertices[1].set(96, 0);
            vertices[2].set(0, 96);
            vertices[3].set(64, 96);
            clip->setGeometry(geometry);
            clip->setFlag(QSGNode::OwnsGeometry);
        }
        auto *nested = new QSGClipNode;
        nested->setIsRectangular(true);
        nested->setClipRect(QRectF(8, 8, 80, 80));
        clip->appendChildNode(nested);
        nested->appendChildNode(new QSGSimpleRectNode(QRectF(0, 0, 96, 96), Qt::green));
        nested->appendChildNode(new QSGSimpleRectNode(QRectF(24, 24, 48, 48), QColor(0, 0, 255, 128)));
        return clip;
    }
private:
    bool stencil;
};

class UploadGateTile final : public QQuickItem {
public:
    UploadGateTile(QQuickItem *parent, QColor color, int vertices = 16384, int indexes = 6)
        : QQuickItem(parent), color(color), vertices(vertices), indexes(indexes) {
        setFlag(ItemHasContents);
        setSize(QSizeF(16, 16));
    }
protected:
    QSGNode *updatePaintNode(QSGNode *node, UpdatePaintNodeData *) override {
        if (node) return node;
        auto *result = new QSGGeometryNode;
        // A large allocation with only four referenced vertices exercises
        // upload admission without adding expensive overdraw to this fixture.
        auto *geometry = new QSGGeometry(QSGGeometry::defaultAttributes_Point2D(),
                                        vertices, indexes, QSGGeometry::UnsignedIntType);
        geometry->setDrawingMode(QSGGeometry::DrawTriangles);
        auto *vertices = geometry->vertexDataAsPoint2D();
        for (int i = 0; i < geometry->vertexCount(); ++i) vertices[i].set(0, 0);
        vertices[1].set(16, 0);
        vertices[2].set(16, 16);
        vertices[3].set(0, 16);
        const quint32 indices[] = {0, 1, 2, 0, 2, 3};
        auto *indexData = geometry->indexDataAsUInt();
        for (int i = 0; i < indexes; ++i) indexData[i] = indices[i % 6];
        auto *material = new QSGFlatColorMaterial;
        material->setColor(color);
        material->setFlag(QSGMaterial::NoBatching);
        result->setGeometry(geometry);
        result->setMaterial(material);
        result->setFlag(QSGNode::OwnsGeometry);
        result->setFlag(QSGNode::OwnsMaterial);
        return result;
    }
private:
    QColor color;
    int vertices;
    int indexes;
};

class GateRenderNode final : public QSGRenderNode {
public:
    explicit GateRenderNode(std::atomic<int> *calls) : calls(calls) {}
    StateFlags changedStates() const override { return {}; }
    RenderingFlags flags() const override { return BoundedRectRendering; }
    QRectF rect() const override { return QRectF(0, 0, 16, 16); }
    void render(const RenderState *) override { ++*calls; }
private:
    std::atomic<int> *calls;
};

class GateRenderItem final : public QQuickItem {
public:
    GateRenderItem(QQuickItem *parent, std::atomic<int> *calls) : QQuickItem(parent), calls(calls) {
        setFlag(ItemHasContents);
    }
    QSGNode *updatePaintNode(QSGNode *node, UpdatePaintNodeData *) override {
        return node ? node : new GateRenderNode(calls);
    }
private:
    std::atomic<int> *calls;
};

} // namespace

class F4QtBatchRootOrdersTests : public QObject {
    Q_OBJECT
private slots:
    void initTestCase() {
        qputenv("QSG_RENDERER_DEBUG", "render");
        previousMessageHandler = qInstallMessageHandler(watchUploads);
    }
    void cleanupTestCase() { qInstallMessageHandler(previousMessageHandler); }

    void geometryBufferPoolBudget_data() {
        QTest::addColumn<QByteArray>("overrideValue");
        QTest::addColumn<int>("expected");
        QTest::newRow("default-working-set") << QByteArray() << 16 * 1024 * 1024;
        QTest::newRow("explicit-smaller-budget") << QByteArray("2097152") << 2 * 1024 * 1024;
        QTest::newRow("explicit-no-retention") << QByteArray("0") << 0;
    }

    void geometryBufferPoolBudget() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QFETCH(QByteArray, overrideValue);
        QFETCH(int, expected);
        const QByteArray previous = qgetenv("QSG_RENDERER_BUFFER_POOL_LIMIT");
        const auto restore = qScopeGuard([&] {
            if (previous.isNull()) qunsetenv("QSG_RENDERER_BUFFER_POOL_LIMIT");
            else qputenv("QSG_RENDERER_BUFFER_POOL_LIMIT", previous);
        });
        if (overrideValue.isNull()) qunsetenv("QSG_RENDERER_BUFFER_POOL_LIMIT");
        else qputenv("QSG_RENDERER_BUFFER_POOL_LIMIT", overrideValue);
        std::atomic<int> actual = -1;
        QQuickWindow window;
        window.resize(64, 64);
        const auto connection = connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            if (renderer) actual = renderer->m_bufferPoolSizeLimit;
        }, Qt::DirectConnection);
        const auto disconnectRender = qScopeGuard([&] { disconnect(connection); });
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        QTRY_COMPARE(actual.load(), expected);
    }

    void interleavedAlphaBatchesPreservePainterOrderAcrossChanges() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        struct Captures { QList<QImage> images; int initialBatches = 0; };
        const auto capture = [&](bool reference) {
            Captures result;
            std::atomic<int> batches = 0;
            QQuickWindow window;
            window.resize(256, 96);
            window.setColor(Qt::black);
            const auto connection = connect(&window, &QQuickWindow::afterRendering, &window, [&] {
                auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(QQuickWindowPrivate::get(&window)->renderer);
                int count = 0;
                for (int i = 0; renderer && i < renderer->m_alphaBatches.size(); ++i)
                    count += renderer->m_alphaBatches.at(i)->first != nullptr;
                batches = count;
            }, Qt::DirectConnection);
            const auto disconnectRender = qScopeGuard([&] { disconnect(connection); });
            QList<QQuickItem *> backgroundHosts, foregroundHosts;
            QList<ColorTile *> backgrounds, foregrounds;
            const auto add = [&](const QColor &color, const QPointF &position, const QSizeF &size) {
                auto *host = new QQuickItem(window.contentItem());
                host->setSize(QSizeF(256, 96));
                // Independent full-window clips force the reference to retain
                // literal painter order without changing any covered pixels.
                host->setClip(reference);
                auto *leaf = new ColorTile(host, color);
                leaf->setPosition(position);
                leaf->setSize(size);
                leaf->setObjectName("interleavedAlphaLeaf");
                return leaf;
            };
            for (int i = 0; i < 6; ++i) {
                auto *background = add(QColor(192, 64, 32, 192), {8.0 + i * 36, 8}, {32, 32});
                auto *foreground = add(QColor(32, 96, 224, 128), {12.0 + i * 36, 12}, {20, 20});
                backgrounds.append(background); foregrounds.append(foreground);
                backgroundHosts.append(background->parentItem()); foregroundHosts.append(foreground->parentItem());
            }
            window.show();
            if (!QTest::qWaitForWindowExposed(&window)) return result;
            const qreal dpr = window.devicePixelRatio();
            if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR")
                && dpr != qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble()) return result;
            for (auto *leaf : backgrounds + foregrounds) {
                const QPointF origin = leaf->mapToScene({});
                if (qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) > .001
                    || qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) > .001
                    || leaf->mapToScene({1, 0}) - origin != QPointF(1, 0)
                    || leaf->mapToScene({0, 1}) - origin != QPointF(0, 1)) return result;
            }
            result.images.append(window.grabWindow());
            result.initialBatches = batches;
            foregrounds[0]->setX(48); // Earlier text now lies under the next background.
            result.images.append(window.grabWindow());
            backgrounds[1]->setColor(QColor(32, 192, 64, 192));
            foregrounds[2]->setWidth(56);
            result.images.append(window.grabWindow());
            auto *inserted = add(QColor(240, 192, 32, 160), {80, 16}, {64, 16});
            inserted->parentItem()->stackBefore(foregroundHosts[3]);
            result.images.append(window.grabWindow());
            delete backgroundHosts[0];
            result.images.append(window.grabWindow());
            inserted->parentItem()->setVisible(false);
            foregrounds[0]->setX(12);
            foregrounds[2]->setWidth(20);
            result.images.append(window.grabWindow());
            backgroundHosts[2]->setWidth(96);
            backgroundHosts[2]->setClip(true);
            result.images.append(window.grabWindow());
            foregroundHosts[4]->setOpacity(.5);
            delete inserted->parentItem();
            result.images.append(window.grabWindow());
            window.contentItem()->setPosition({4, 4});
            result.images.append(window.grabWindow());
            return result;
        };
        const auto reference = capture(true);
        const auto optimized = capture(false);
        QCOMPARE(reference.images.size(), 9);
        QCOMPARE(optimized.images.size(), reference.images.size());
        for (int phase = 0; phase < reference.images.size(); ++phase) {
            QVERIFY2(!optimized.images[phase].isNull(), qPrintable(QString::number(phase)));
            QVERIFY2(optimized.images[phase] == reference.images[phase],
                     qPrintable(QStringLiteral("Painter order differs in phase %1").arg(phase)));
        }
        // The stock renderer is conservative around already-batched nodes.
        // Isolated batching experiments also assert their intended reduction.
        if (qEnvironmentVariableIsSet("F4_EXPECT_INTERLEAVED_ALPHA_MERGE"))
            QCOMPARE(optimized.initialBatches, 2);
    }

    void coldBranchKeepsUnrelatedBuffersAndOrderRange() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        const auto priorPadding = qgetenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING");
        qunsetenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING");
        const auto restore = qScopeGuard([&] {
            if (!priorPadding.isNull()) qputenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING", priorPadding);
            watchedRoot = 0;
        });
        watchedUploads = 0;
        QQuickWindow window;
        window.resize(256, 128);
        window.setColor(Qt::black);
        auto *stable = new QQuickItem(window.contentItem());
        stable->setSize(QSizeF(96, 96));
        stable->setClip(true);
        auto *red = new ColorTile(stable, Qt::red);
        red->setObjectName("boundedOrderStableLeaf");
        red->setSize(stable->size());
        auto *cold = new QQuickItem(window.contentItem());
        cold->setPosition(QPointF(128, 0));
        cold->setSize(stable->size());
        cold->setClip(true);
        auto *burst = new ColdGeometryTile(cold);
        burst->setObjectName("boundedOrderColdLeaf");
        burst->setSize(cold->size());
        std::atomic<int> orders = 0;
        connect(&window, &QQuickWindow::beforeRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(QQuickWindowPrivate::get(&window)->renderer);
            auto *clip = QQuickItemPrivate::get(stable)->clipNode();
            watchedRoot = renderer && clip ? quintptr(renderer->m_nodes.value(clip)) : 0;
        }, Qt::DirectConnection);
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(QQuickWindowPrivate::get(&window)->renderer);
            orders = renderer->m_nextRenderOrder;
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const auto baseline = window.grabWindow();
        const int initialOrders = orders.load();
        const auto initialUploads = watchedUploads.load();
        QVERIFY(initialUploads > 0);
        const qreal dpr = window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(dpr, qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        for (auto *leaf : {static_cast<QQuickItem *>(red), static_cast<QQuickItem *>(burst)}) {
            const auto origin = leaf->mapToScene(QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(leaf->mapToScene(QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToScene(QPointF(0,1))-origin, QPointF(0,1));
        }
        for (int cycle = 0; cycle < 4; ++cycle) {
            burst->setCount(512);
            const auto image = window.grabWindow();
            QCOMPARE(orders.load(), initialOrders);
            QCOMPARE(watchedUploads.load(), initialUploads);
            QCOMPARE(image.pixelColor(qRound(160*dpr), qRound(32*dpr)), QColor(Qt::green));
            QCOMPARE(image.pixelColor(qRound(224*dpr), qRound(32*dpr)), QColor(Qt::black));
            QCOMPARE(image.pixelColor(qRound(32*dpr), qRound(32*dpr)), QColor(Qt::red));
            if (!cycle) QVERIFY(image.save("bounded-order-cold-175.png"));
            burst->setCount(0);
            QCOMPARE(window.grabWindow(), baseline);
            QCOMPARE(orders.load(), initialOrders);
        }
    }

    void automaticOrderPaddingHasSceneWideBound() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        const auto priorPadding = qgetenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING");
        qunsetenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING");
        const auto restore = qScopeGuard([&] {
            if (!priorPadding.isNull()) qputenv("QSG_BATCHRENDERER_MINIMUM_ORDER_PADDING", priorPadding);
        });
        QQuickWindow window;
        window.resize(64, 64);
        QList<QQuickItem *> leaves;
        for (int i = 0; i < 512; ++i) {
            auto *clip = new QQuickItem(window.contentItem());
            clip->setClip(true);
            clip->setSize(QSizeF(64,64));
            auto *tile = new ColorTile(clip, i == 511 ? Qt::green : Qt::red);
            tile->setObjectName(QStringLiteral("boundedOrderBudgetLeaf-%1").arg(i));
            tile->setSize(clip->size());
            leaves.append(tile);
        }
        std::atomic<int> orders = 0;
        std::atomic<int> elements = 0;
        std::atomic<bool> valid = true;
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(QQuickWindowPrivate::get(&window)->renderer);
            orders = renderer->m_nextRenderOrder;
            elements = renderer->m_opaqueRenderList.size() + renderer->m_alphaRenderList.size();
            for (auto *node : std::as_const(renderer->m_nodes)) {
                if (node->type() != QSGNode::GeometryNodeType) continue;
                auto *e = node->element();
                if (!e || e->removed || !e->root) continue;
                const auto *range = e->root->rootInfo();
                if (e->order <= range->firstOrder || e->order > range->lastOrder) valid = false;
                const qreal inverse = 1.0 / renderer->m_nextRenderOrder;
                // Same float subtraction as Qt's vertex depth conversion.
                const float z0 = 1.0f - float(e->order * inverse);
                const float z1 = 1.0f - float((e->order + 1) * inverse);
                if (z0 == z1 || qRound(z0*16777215) == qRound(z1*16777215)) valid = false;
            }
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const auto image = window.grabWindow();
        QCOMPARE(elements.load(), 512);
        QVERIFY(orders.load() >= 2048);
        QVERIFY(orders.load() <= elements.load() + 262144);
        QVERIFY(valid.load());
        const qreal dpr = window.devicePixelRatio();
        QCOMPARE(image.pixelColor(qRound(32*dpr),qRound(32*dpr)),QColor(Qt::green));
        // Roots beyond the global budget receive no speculative padding.
        // Late geometry there may take the normal full-rebuild fallback safely.
        for (int i = 0; i < 16; ++i) {
            auto *extra = new ColorTile(leaves.last()->parentItem(), Qt::green);
            extra->setObjectName(QStringLiteral("boundedOrderFallbackLeaf-%1").arg(i));
            extra->setSize(QSizeF(64,64));
            leaves.append(extra);
        }
        const auto fallback = window.grabWindow();
        QCOMPARE(elements.load(), 528);
        QVERIFY(orders.load() <= elements.load() + 262144);
        QVERIFY(valid.load());
        QCOMPARE(fallback.pixelColor(qRound(32*dpr),qRound(32*dpr)),QColor(Qt::green));
        for (auto *leaf : leaves) {
            const auto origin = leaf->mapToScene(QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(leaf->mapToScene(QPointF(1,0))-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToScene(QPointF(0,1))-origin, QPointF(0,1));
        }
    }

    void recycledBufferUsesAvailableCapacity_data() {
        QTest::addColumn<QList<int>>("capacities");
        QTest::addColumn<int>("expectedIndex");
        QTest::addColumn<bool>("indexBuffer");
        for (bool index : {false, true}) {
            const QByteArray prefix = index ? "index-" : "vertex-";
            QTest::newRow((prefix + "undersized-before-exact").constData())
                << QList<int>{16, 128, 64} << 2 << index;
            QTest::newRow((prefix + "undersized-before-best").constData())
                << QList<int>{16, 128, 96} << 2 << index;
            QTest::newRow((prefix + "adequate-before-smaller").constData())
                << QList<int>{128, 16, 96} << 2 << index;
            QTest::newRow((prefix + "largest-undersized").constData())
                << QList<int>{16, 32, 48} << 2 << index;
            QTest::newRow((prefix + "preserve-large-for-large-uploads").constData())
                << QList<int>{16, 8192, 48} << 2 << index;
            QTest::newRow((prefix + "allocate-small-with-only-large-pooled").constData())
                << QList<int>{8192, 16384} << -1 << index;
        }
    }

    void recycledBufferUsesAvailableCapacity() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QFETCH(QList<int>, capacities);
        QFETCH(int, expectedIndex);
        QFETCH(bool, indexBuffer);
        QQuickWindow window;
        window.resize(96, 64);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        window.grabWindow();
        QVERIFY(window.rhi());
        const auto api = window.rendererInterface()->graphicsApi();
        if (qEnvironmentVariable("ZOIN_EXPECT_TEST_API") == "d3d11")
            QCOMPARE(api, QSGRendererInterface::Direct3D11);
        if (qEnvironmentVariable("ZOIN_EXPECT_TEST_API") == "opengl")
            QCOMPARE(api, QSGRendererInterface::OpenGL);
        auto *tile = new UploadGateTile(window.contentItem(), Qt::red, 8, 16);
        tile->setObjectName("pooledBufferTile");
        tile->setPosition(QPointF(8, 8));
        std::atomic<bool> injected = false;
        std::atomic<bool> completed = false;
        bool allocationsReady = true;
        bool poolInitiallyEmpty = false;
        bool selectedExpected = false;
        int remainingCount = -1;
        quint32 remainingCost = 0;
        QList<QRhiBuffer *> buffers;
        const auto before = connect(&window, &QQuickWindow::beforeRendering, &window, [&] {
            if (injected.load()) return;
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            auto *pool = indexBuffer ? &renderer->m_iboPool : &renderer->m_vboPool;
            auto *cost = indexBuffer ? &renderer->m_iboPoolCost : &renderer->m_vboPoolCost;
            poolInitiallyEmpty = pool->isEmpty() && *cost == 0;
            if (!poolInitiallyEmpty) {
                completed = true;
                return;
            }
            auto *rhi = window.rhi();
            for (const int capacity : capacities) {
                auto *buffer = rhi->newBuffer(QRhiBuffer::Dynamic,
                    indexBuffer ? QRhiBuffer::IndexBuffer : QRhiBuffer::VertexBuffer, capacity);
                allocationsReady &= buffer->create();
                buffers.append(buffer);
                pool->add(buffer);
                *cost += capacity;
            }
            injected = true;
        }, Qt::DirectConnection);
        const auto after = connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            if (!injected.load() || completed.load()) return;
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            if (renderer->m_opaqueBatches.size() != 1) return;
            const auto *batch = renderer->m_opaqueBatches.at(0);
            const auto &upload = indexBuffer ? batch->ibo : batch->vbo;
            selectedExpected = expectedIndex < 0
                ? upload.buf && !buffers.contains(upload.buf) && upload.buf->size() == 64
                : upload.buf == buffers.at(expectedIndex);
            remainingCount = indexBuffer ? renderer->m_iboPool.size() : renderer->m_vboPool.size();
            remainingCost = indexBuffer ? renderer->m_iboPoolCost : renderer->m_vboPoolCost;
            completed = true;
        }, Qt::DirectConnection);
        const auto cleanup = qScopeGuard([&] { disconnect(before); disconnect(after); });
        window.update();
        const QImage image = window.grabWindow();
        QVERIFY(completed.load());
        QVERIFY(poolInitiallyEmpty);
        QVERIFY(allocationsReady);
        QVERIFY2(selectedExpected, "Resized an undersized buffer despite a better pooled buffer");
        QCOMPARE(remainingCount, capacities.size() - (expectedIndex >= 0 ? 1 : 0));
        quint32 expectedCost = 0;
        for (int i = 0; i < capacities.size(); ++i)
            if (i != expectedIndex) expectedCost += capacities.at(i);
        QCOMPARE(remainingCost, expectedCost);
        const qreal dpr = window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(dpr, qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        const QPointF origin = tile->mapToScene(QPointF());
        QVERIFY(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < .001);
        QVERIFY(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < .001);
        QCOMPARE(tile->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
        QCOMPARE(tile->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
        QCOMPARE(image.pixelColor(qRound(12 * dpr), qRound(12 * dpr)), QColor(Qt::red));
    }

    void vectorUploadRootsAvoidUnrelatedFullRebuilds() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QQuickWindow window;
        window.resize(192, 96);
        window.setColor(Qt::blue);
        auto *outer = new QQuickItem(window.contentItem());
        outer->setSize(QSizeF(192, 96));
        outer->setClip(true);
        auto *sibling = new ColorTile(outer, Qt::green, true);
        sibling->setObjectName(QStringLiteral("vectorRootRetainedSibling"));
        sibling->setPosition(QPointF(96, 16));
        sibling->setSize(QSizeF(32, 32));
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        for (int i = 0; i < 3; ++i) window.grabWindow();
        QQuickItem *gate = nullptr;
        std::atomic<bool> supported = false;
        const auto sync = connect(&window, &QQuickWindow::beforeSynchronizing, &window, [&] {
            const auto *api = static_cast<const F4VectorUploadGateInterface *>(
                window.rendererInterface()->getResource(&window, "f4.vectorUploadGate.v2"));
            supported = api && api->version == 2 && api->size >= sizeof(*api);
            if (supported && gate)
                api->configure(QQuickItemPrivate::get(gate)->itemNode(), 1, true, true, 100, false);
        }, Qt::DirectConnection);
        const auto cleanup = qScopeGuard([&] {
            watchFullRebuilds = false;
            disconnect(sync);
        });
        for (int cycle = 0; cycle < 16; ++cycle) {
            fullRebuilds = 0;
            watchFullRebuilds = true;
            gate = new QQuickItem(outer);
            gate->setSize(QSizeF(8, 8));
            auto *inside = new ColorTile(gate, Qt::red, true);
            inside->setObjectName(QStringLiteral("vectorRootInsertedLeaf"));
            inside->setPosition(QPointF(16, 16));
            inside->setSize(QSizeF(32, 32));
            QImage image;
            for (int i = 0; i < 5; ++i) {
                window.update();
                image = window.grabWindow();
            }
            QVERIFY(supported.load());
            QCOMPARE(image.pixelColor(qRound(24 * window.devicePixelRatio()),
                                      qRound(24 * window.devicePixelRatio())), QColor(Qt::red));
            QCOMPARE(fullRebuilds.load(), 0);
            for (auto *leaf : {inside, sibling}) {
                const QPointF origin = leaf->mapToScene(QPointF());
                const QPointF physical = origin * window.devicePixelRatio();
                QVERIFY(qAbs(physical.x() - qRound(physical.x())) < 0.0001);
                QVERIFY(qAbs(physical.y() - qRound(physical.y())) < 0.0001);
                QCOMPARE(leaf->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
                QCOMPARE(leaf->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
            }
            fullRebuilds = 0;
            delete gate;
            gate = nullptr;
            for (int i = 0; i < 3; ++i) {
                window.update();
                image = window.grabWindow();
            }
            QCOMPARE(image.pixelColor(qRound(24 * window.devicePixelRatio()),
                                      qRound(24 * window.devicePixelRatio())), QColor(Qt::blue));
            QCOMPARE(image.pixelColor(qRound(104 * window.devicePixelRatio()),
                                      qRound(24 * window.devicePixelRatio())), QColor(Qt::green));
            QCOMPARE(fullRebuilds.load(), 0);
        }
    }

    void vectorUploadTransformIsolatesSmallSurfaceWithoutClipping() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QQuickWindow window;
        window.resize(192, 96);
        window.setColor(Qt::blue);
        auto *gate = new QQuickItem(window.contentItem());
        gate->setSize(QSizeF(8, 8));
        gate->setVisible(false);
        auto *inside = new ColorTile(gate, Qt::red);
        inside->setPosition(QPointF(16, 16));
        inside->setSize(QSizeF(32, 32));
        auto *sibling = new ColorTile(window.contentItem(), Qt::red);
        sibling->setPosition(QPointF(96, 16));
        sibling->setSize(QSizeF(32, 32));
        std::atomic<bool> supported = false, cpuComplete = false, present = false, ready = false;
        connect(&window, &QQuickWindow::beforeRendering, &window, [&] {
            auto *api = static_cast<const F4VectorUploadGateInterface *>(
                window.rendererInterface()->getResource(&window, "f4.vectorUploadGate.v2"));
            supported = api && api->version == 2 && api->size >= sizeof(F4VectorUploadGateInterface);
            if (supported)
                api->configure(QQuickItemPrivate::get(gate)->itemNode(), 1,
                               cpuComplete.load(), present.load(), 0, false);
        }, Qt::DirectConnection);
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *api = static_cast<const F4VectorUploadGateInterface *>(
                window.rendererInterface()->getResource(&window, "f4.vectorUploadGate.v2"));
            if (api) ready = api->ready(QQuickItemPrivate::get(gate)->itemNode(), 1);
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const auto baseline = window.grabWindow();
        QVERIFY(supported.load());
        gate->setVisible(true);
        QCOMPARE(window.grabWindow(), baseline);
        QVERIFY(!ready.load());
        cpuComplete = true;
        for (int frame = 0; frame < 16 && !ready.load(); ++frame)
            QCOMPARE(window.grabWindow(), baseline);
        QVERIFY(ready.load());
        present = true;
        const auto complete = window.grabWindow();
        const auto dpr = window.devicePixelRatio();
        QCOMPARE(complete.pixelColor(qRound(24*dpr), qRound(24*dpr)), QColor(Qt::red));
        QCOMPARE(complete.pixelColor(qRound(104*dpr), qRound(24*dpr)), QColor(Qt::red));
    }

    void vectorUploadGateDefersDrawingUntilComplete_data() {
        QTest::addColumn<bool>("clipped");
        QTest::newRow("clip") << true;
        QTest::newRow("transform") << false;
    }

    void vectorUploadGateDefersDrawingUntilComplete() {
        QFETCH(bool, clipped);
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QQuickWindow window;
        window.resize(256, 160);
        window.setColor(Qt::black);
        auto *background = new ColorTile(window.contentItem(), Qt::blue);
        background->setSize(QSizeF(256, 160));
        auto *gate = new QQuickItem(window.contentItem());
        gate->setPosition(QPointF(16, 16));
        gate->setSize(QSizeF(192, 128));
        gate->setClip(clipped);
        gate->setVisible(false);
        std::atomic<int> renderNodeCalls = 0;
        QList<QQuickItem *> leaves;
        for (int i = 0; i < 24; ++i) {
            auto *tile = new UploadGateTile(gate, QColor(200 + i, 0, 0));
            tile->setObjectName(QStringLiteral("uploadGateTile-%1").arg(i));
            tile->setPosition(QPointF((i % 8)*24, (i / 8)*24));
            leaves.append(tile);
        }
        new GateRenderItem(gate, &renderNodeCalls);
        const F4VectorUploadGateInterface *api = nullptr;
        std::atomic<bool> supported = false;
        std::atomic<bool> uploadReady = false;
        std::atomic<bool> present = false;
        std::atomic<bool> cpuComplete = true;
        std::atomic<quint64> generation = 1;
        std::atomic<int> priority = 0;
        std::atomic<bool> staleGenerationReady = false;
        std::atomic<bool> viewerReady = false;
        std::atomic<bool> removedRootReady = false;
        std::atomic<QSGNode *> removedRoot = nullptr;
        QQuickItem *viewerGate = nullptr;
        std::atomic<int> maximumUploadsPerFrame = 0;
        QSGNode *registeredRoot = nullptr;
        connect(&window, &QQuickWindow::beforeRendering, &window, [&] {
            api = static_cast<const F4VectorUploadGateInterface *>(
                window.rendererInterface()->getResource(&window, "f4.vectorUploadGate.v2"));
            supported = api && api->version == 2 && api->size >= sizeof(F4VectorUploadGateInterface);
            if (!supported || !gate) return;
            QSGNode *clip = clipped ? static_cast<QSGNode *>(QQuickItemPrivate::get(gate)->clipNode())
                                    : QQuickItemPrivate::get(gate)->itemNode();
            if (!clip) return;
            registeredRoot = clip;
            api->configure(clip, generation.load(), cpuComplete.load(), present.load(),
                           priority.load(), false);
            if (viewerGate) {
                if (QSGNode *viewerClip = clipped ? static_cast<QSGNode *>(QQuickItemPrivate::get(viewerGate)->clipNode())
                                                   : QQuickItemPrivate::get(viewerGate)->itemNode())
                    api->configure(viewerClip, 1, true, false, 0, false);
            }
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            watchedRoot = renderer ? quintptr(renderer->m_nodes.value(clip)) : 0;
            watchedUploads = 0;
        }, Qt::DirectConnection);
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            if (!supported) return;
            if (removedRoot.load()) removedRootReady = api->ready(removedRoot.load(), 2);
            if (!registeredRoot) return;
            maximumUploadsPerFrame = qMax(maximumUploadsPerFrame.load(), int(watchedUploads.load()));
            uploadReady = api->ready(registeredRoot, generation.load());
            if (generation.load() != 1) staleGenerationReady = api->ready(registeredRoot, 1);
            if (viewerGate) {
                QSGNode *clip = clipped ? static_cast<QSGNode *>(QQuickItemPrivate::get(viewerGate)->clipNode())
                                        : QQuickItemPrivate::get(viewerGate)->itemNode();
                viewerReady = clip && api->ready(clip, 1);
            }
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const QImage backgroundOnly = window.grabWindow();
        QVERIFY(supported.load());
        gate->setVisible(true);
        int frames = 0;
        while (!uploadReady.load() && frames++ < 64) {
            QCOMPARE(window.grabWindow(), backgroundOnly);
        }
        QVERIFY(uploadReady.load());
        QVERIFY(frames >= 2);
        QVERIFY(maximumUploadsPerFrame.load() > 0 && maximumUploadsPerFrame.load() <= 5);
        QCOMPARE(renderNodeCalls.load(), 0);
        qInfo() << "Admission captures" << frames << "maximum uploaded batches per frame"
                << maximumUploadsPerFrame.load();
        QCOMPARE(window.grabWindow(), backgroundOnly);
        present = true;
        const auto completed = window.grabWindow();
        QVERIFY(renderNodeCalls.load() > 0);
        const qreal dpr = window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(dpr, qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        for (auto *leaf : leaves) {
            const auto origin = leaf->mapToItem(window.contentItem(), QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(leaf->mapToItem(window.contentItem(), QPointF(1, 0))-origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToItem(window.contentItem(), QPointF(0, 1))-origin, QPointF(0, 1));
        }
        for (int i = 0; i < 24; ++i)
            QCOMPARE(completed.pixelColor(qRound((24+(i%8)*24)*dpr),
                                          qRound((24+(i/8)*24)*dpr)), QColor(200+i, 0, 0));
        QVERIFY(completed.save("vector-upload-gate-complete-175.png"));
        // Old readiness must not publish another generation, and CPU completion
        // is independently required even if the old buffers are already resident.
        generation = 2;
        cpuComplete = false;
        present = false;
        for (int i = 0; i < 3; ++i) QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(!uploadReady.load());
        QVERIFY(!staleGenerationReady.load());
        cpuComplete = true;
        frames = 0;
        while (!uploadReady.load() && frames++ < 64)
            QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(uploadReady.load());
        present = true;
        QCOMPARE(window.grabWindow(), completed);
        removedRoot = registeredRoot;
        delete gate;
        gate = nullptr;
        registeredRoot = nullptr;
        QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(!removedRootReady.load());
        removedRoot = nullptr;

        // A later viewer request must overtake a panel request still being
        // uploaded. Both remain invisible until independently complete.
        const auto makeGate = [&](int x) {
            auto *item = new QQuickItem(window.contentItem());
            item->setPosition(QPointF(x, 16));
            item->setSize(QSizeF(96, 128));
            item->setClip(clipped);
            for (int i = 0; i < 24; ++i) {
                auto *tile = new UploadGateTile(item, QColor(200+i, 0, 0));
                tile->setPosition(QPointF((i%4)*24, (i/4)*20));
            }
            return item;
        };
        gate = makeGate(16);
        generation = 3;
        present = false;
        priority = 20;
        uploadReady = false;
        QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(!uploadReady.load());
        viewerGate = makeGate(144);
        frames = 0;
        while (!viewerReady.load() && frames++ < 64)
            QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(viewerReady.load());
        QVERIFY(!uploadReady.load());
        // Cancel unfinished panel work. A subsequent frame must not reuse its
        // readiness or try to process batches belonging to the removed subtree.
        delete gate;
        gate = nullptr;
        delete viewerGate;
        viewerGate = nullptr;
        registeredRoot = nullptr;
        QCOMPARE(window.grabWindow(), backgroundOnly);

        // One allocation can exceed the soft byte budget. Admit it alone so it
        // makes progress rather than waiting forever for a larger frame budget.
        gate = new QQuickItem(window.contentItem());
        gate->setSize(QSizeF(96, 96));
        gate->setClip(clipped);
        new UploadGateTile(gate, Qt::red, 262144);
        generation = 4;
        uploadReady = false;
        frames = 0;
        while (!uploadReady.load() && frames++ < 8)
            QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(uploadReady.load());
        delete gate;
        gate = nullptr;
        registeredRoot = nullptr;
        QCOMPARE(window.grabWindow(), backgroundOnly);

        // Valid empty documents have no upload work and must still finish.
        gate = new QQuickItem(window.contentItem());
        gate->setSize(QSizeF(96, 96));
        gate->setClip(clipped);
        generation = 5;
        uploadReady = false;
        frames = 0;
        while (!uploadReady.load() && frames++ < 8)
            QCOMPARE(window.grabWindow(), backgroundOnly);
        QVERIFY(uploadReady.load());
        delete gate;
        gate = nullptr;
        registeredRoot = nullptr;
        QCOMPARE(window.grabWindow(), backgroundOnly);
    }

    void preparedVectorReentryUsesUploadBudget_data() {
        QTest::addColumn<bool>("clipped");
        QTest::addColumn<bool>("rebuildHidden");
        QTest::newRow("clip") << true << false;
        QTest::newRow("transform") << false << false;
        QTest::newRow("hidden-ready-clip") << true << true;
        QTest::newRow("hidden-ready-transform") << false << true;
    }

    void preparedVectorReentryUsesUploadBudget() {
        QFETCH(bool, clipped);
        QFETCH(bool, rebuildHidden);
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QQuickWindow window;
        window.resize(256, 160);
        window.setColor(Qt::blue);
        auto *ancestor = new DrawingGate(window.contentItem());
        ancestor->setSize({256, 160});
        auto *gate = new QQuickItem(ancestor);
        gate->setPosition({16, 16});
        gate->setSize({192, 128});
        gate->setClip(clipped);
        QList<QQuickItem *> leaves;
        for (int i = 0; i < 24; ++i) {
            auto *tile = new UploadGateTile(gate, QColor(200+i, 0, 0));
            tile->setObjectName(QStringLiteral("reentryTile-%1").arg(i));
            tile->setPosition({qreal((i%8)*24), qreal((i/8)*24)});
            leaves.append(tile);
        }
        std::atomic_bool supported{false}, ready{false}, prepared{false}, present{false};
        std::atomic_int maximumUploads{0}, residentBefore{0}, maximumNewResident{0};
        const F4VectorUploadGateInterface *api = nullptr;
        QSGNode *registered = nullptr;
        auto before = connect(&window, &QQuickWindow::beforeRendering, &window, [&] {
            api = static_cast<const F4VectorUploadGateInterface *>(
                window.rendererInterface()->getResource(&window, "f4.vectorUploadGate.v2"));
            supported = api && api->version == 2 && api->size >= sizeof(*api);
            watchedUploads = 0;
            if (!supported) return;
            registered = clipped ? static_cast<QSGNode *>(QQuickItemPrivate::get(gate)->clipNode())
                                 : QQuickItemPrivate::get(gate)->itemNode();
            if (registered)
                api->configure(registered, 1, true, present.load(), 100, prepared.load());
        }, Qt::DirectConnection);
        auto after = connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            if (!supported || !registered) return;
            if (rebuildHidden && watchAllUploads.load()) {
                int resident = 0;
                auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                    QQuickWindowPrivate::get(&window)->renderer);
                if (renderer) for (const auto *batches : {&renderer->m_opaqueBatches, &renderer->m_alphaBatches})
                    for (int i = 0; i < batches->size(); ++i) {
                        auto *batch = batches->at(i);
                        if (!batch->first || !batch->vbo.buf) continue;
                        for (QSGNode *node = batch->first->node; node; node = node->parent())
                            if (node == registered) { ++resident; break; }
                    }
                maximumNewResident = qMax(maximumNewResident.load(), resident - residentBefore.load());
                residentBefore = resident;
            }
            ready = api->ready(registered, 1);
            maximumUploads = qMax(maximumUploads.load(), int(watchedUploads.load()));
        }, Qt::DirectConnection);
        const auto cleanup = qScopeGuard([&] {
            disconnect(before); disconnect(after); watchAllUploads = false;
        });
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        QVERIFY(supported.load());
        const QImage background = window.grabWindow();
        for (int frame = 0; !ready.load() && frame < 64; ++frame)
            QCOMPARE(window.grabWindow(), background);
        QVERIFY(ready.load());
        present = true;
        const QImage complete = window.grabWindow();
        QVERIFY(complete != background);
        prepared = true; // The source remains known after renderer culling.
        if (rebuildHidden) {
            present = false;
            QCOMPARE(window.grabWindow(), background);
            // Keep the registered gate alive and ready while replacing geometry.
            // Native viewport preparation can rebuild a hidden cached surface.
            qDeleteAll(leaves);
            leaves.clear();
            for (int i = 0; i < 24; ++i) {
                auto *tile = new UploadGateTile(gate, QColor(200+i, 0, 0));
                tile->setObjectName(QStringLiteral("reentryTile-%1").arg(i));
                tile->setPosition({qreal((i%8)*24), qreal((i/8)*24)});
                leaves.append(tile);
            }
        } else {
            ancestor->showDrawing(false);
            QCOMPARE(window.grabWindow(), background);
        }
        ready = false;
        maximumUploads = 0;
        watchAllUploads = true;
        ancestor->showDrawing(true);
        int frames = 0;
        do {
            const QImage image = window.grabWindow();
            QVERIFY2(image == background || image == complete,
                "A re-entering vector surface was presented with only some batches uploaded");
            ++frames;
        } while (!ready.load() && frames < 64);
        QVERIFY(ready.load());
        const int observed = rebuildHidden ? maximumNewResident.load() : maximumUploads.load();
        QVERIFY2(observed > 0 && observed <= 5,
            qPrintable(QString("Prepared re-entry uploaded %1 batches in one frame (budget permits at most 5)")
                .arg(observed)));
        QVERIFY(frames >= 2);
        present = true;
        QCOMPARE(window.grabWindow(), complete);
        const qreal dpr = window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(dpr, qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        for (auto *leaf : leaves) {
            const auto origin = leaf->mapToItem(window.contentItem(), QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(leaf->mapToItem(window.contentItem(), {1,0})-origin, QPointF(1,0));
            QCOMPARE(leaf->mapToItem(window.contentItem(), {0,1})-origin, QPointF(0,1));
        }
        QVERIFY(complete.save("prepared-vector-reentry-complete-175.png"));
    }

    void visibleReadyGeometryBurstUsesBudget_data() {
        QTest::addColumn<bool>("clipped");QTest::newRow("clip")<<true;QTest::newRow("transform")<<false;
    }
    void visibleReadyGeometryBurstUsesBudget() {
        QFETCH(bool,clipped);
        if(!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))QSKIP("native renderer required");
        QQuickWindow window;window.resize(256,160);window.setColor(Qt::blue);
        auto *gate=new QQuickItem(window.contentItem());gate->setPosition({16,16});gate->setSize({192,128});gate->setClip(clipped);
        auto *seed=new UploadGateTile(gate,Qt::green,8,6);seed->setObjectName("lateBurstSeed");
        const F4VectorUploadGateInterface *api=nullptr;QSGNode *registered=nullptr;
        std::atomic_bool ready=false;std::atomic_int maximumUploads=0;
        auto before=connect(&window,&QQuickWindow::beforeRendering,&window,[&]{
            watchedUploads=0;api=static_cast<const F4VectorUploadGateInterface*>(window.rendererInterface()->getResource(&window,"f4.vectorUploadGate.v2"));
            if(!api)return;
            registered=clipped?static_cast<QSGNode*>(QQuickItemPrivate::get(gate)->clipNode()):QQuickItemPrivate::get(gate)->itemNode();
            if(registered)api->configure(registered,1,true,true,100,true);
        },Qt::DirectConnection);
        auto after=connect(&window,&QQuickWindow::afterRendering,&window,[&]{
            if(api&&registered)ready=api->ready(registered,1);
            maximumUploads=qMax(maximumUploads.load(),int(watchedUploads.load()));
        },Qt::DirectConnection);
        auto cleanup=qScopeGuard([&]{disconnect(before);disconnect(after);watchAllUploads=false;});
        window.show();QVERIFY(QTest::qWaitForWindowExposed(&window));
        for(int i=0;i<16&&!ready.load();++i)window.grabWindow();QVERIFY(ready.load());
        maximumUploads=0;watchAllUploads=true;QList<QQuickItem*> leaves;
        for(int i=0;i<24;++i){auto *tile=new UploadGateTile(gate,QColor(200+i,0,0));tile->setObjectName(QString("lateBurstLeaf-%1").arg(i));tile->setPosition({qreal((i%8)*24),qreal((i/8)*24)});leaves.append(tile);}
        window.grabWindow();
        for(int i=0;i<64&&!ready.load();++i)window.grabWindow();
        QVERIFY(ready.load());
        QVERIFY2(maximumUploads.load()>0&&maximumUploads.load()<=5,qPrintable(QString("Ready+present root uploaded %1 batches in one frame, budget permits at most 5").arg(maximumUploads.load())));
        QCOMPARE(window.devicePixelRatio(),1.75);
        for(auto *leaf:leaves){auto origin=leaf->mapToItem(window.contentItem(),QPointF());QVERIFY(qAbs(origin.x()*1.75-qRound(origin.x()*1.75))<.001);QVERIFY(qAbs(origin.y()*1.75-qRound(origin.y()*1.75))<.001);QCOMPARE(leaf->mapToScene({1,0})-leaf->mapToScene({}),QPointF(1,0));QCOMPARE(leaf->mapToScene({0,1})-leaf->mapToScene({}),QPointF(0,1));}
    }

    void repeatedClipLifecycle_data() {
        QTest::addColumn<bool>("stencil");
        QTest::newRow("scissor") << false;
        QTest::newRow("stencil") << true;
    }

    void repeatedClipLifecycle() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QFETCH(bool, stencil);
        QQuickWindow window;
        window.resize(544, 272);
        window.setColor(Qt::black);
        QList<QQuickItem *> parents;
        for (int side = 0; side < 2; ++side) {
            auto *parent = new QQuickItem(window.contentItem());
            parent->setPosition(QPointF(16 + side*256, 16));
            parent->setSize(QSizeF(240, 240));
            parent->setClip(true);
            parents.append(parent);
            for (int i = 0; i < 128; ++i) {
                auto *tile = new ColorTile(parent, side ? Qt::yellow : Qt::red);
                tile->setSize(parent->size());
            }
            auto *stable = new QQuickItem(parent);
            stable->setPosition(QPointF(16, 16));
            stable->setSize(QSizeF(80, 80));
            stable->setClip(true);
            auto *blue = new ColorTile(stable, QColor(0, 0, 255, 128));
            blue->setSize(stable->size());
        }
        std::atomic<bool> validRanges = true;
        std::atomic<int> maximumOrders = 0;
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            if (!renderer) return;
            maximumOrders = qMax(maximumOrders.load(), renderer->m_nextRenderOrder);
            for (auto *node : std::as_const(renderer->m_nodes)) {
                if (node->type() == QSGNode::ClipNodeType || node->isBatchRoot) {
                    const auto *info = node->rootInfo();
                    if (info->availableOrders < 0 || info->firstOrder < 0
                        || info->lastOrder < info->firstOrder) validRanges = false;
                    if (info->parentRoot) {
                        const auto *outer = info->parentRoot->rootInfo();
                        if (info->firstOrder < outer->firstOrder || info->lastOrder > outer->lastOrder)
                            validRanges = false;
                    }
                } else if (node->type() == QSGNode::GeometryNodeType) {
                    const auto *element = node->element();
                    if (element && !element->removed && element->root) {
                        const auto *info = element->root->rootInfo();
                        if (element->order <= info->firstOrder || element->order > info->lastOrder)
                            validRanges = false;
                    }
                }
            }
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const QImage baseline = window.grabWindow();
        QVERIFY(!baseline.isNull());
        const qreal dpr = window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(dpr, qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        QImage initialLeft;
        QImage initialRight;
        int warmedOrderBound = 0;
        for (int cycle = 0; cycle < 32; ++cycle) {
            auto *branch = new NestedClipTile(parents[0], stencil);
            branch->setPosition(QPointF(128, 128));
            const QImage left = window.grabWindow();
            if (!cycle) initialLeft = left;
            else QCOMPARE(left, initialLeft);
            // This point is inside the rectangular clip but outside the trapezoid.
            QCOMPARE(left.pixelColor(qRound(228*dpr), qRound(224*dpr)),
                     QColor(stencil ? Qt::red : Qt::green));
            branch->setParentItem(parents[1]);
            const QImage right = window.grabWindow();
            if (!cycle) initialRight = right;
            else QCOMPARE(right, initialRight);
            branch->setVisible(false);
            QCOMPARE(window.grabWindow(), baseline);
            branch->setVisible(true);
            QCOMPARE(window.grabWindow(), initialRight);
            branch->setParentItem(parents[0]);
            QCOMPARE(window.grabWindow(), initialLeft);
            delete branch;
            QCOMPARE(window.grabWindow(), baseline);
            QVERIFY2(validRanges.load(), "Clip lifecycle produced an invalid drawing-order interval");
            if (cycle == 1) warmedOrderBound = maximumOrders.load();
            if (cycle > 1) QCOMPARE(maximumOrders.load(), warmedOrderBound);
        }
        QVERIFY(initialLeft.save(stencil ? "retained-stencil-lifecycle-175.png"
                                        : "retained-scissor-lifecycle-175.png"));
    }

    void unchangedSubrootKeepsUploadedGeometry_data() {
        QTest::addColumn<bool>("unmerged");
        QTest::addColumn<bool>("moveRange");
        QTest::addColumn<bool>("forceFull");
        QTest::addColumn<bool>("multipleElements");
        QTest::addColumn<bool>("interleavedMembers");
        QTest::newRow("merged") << false << false << false << false << false;
        QTest::newRow("unmerged") << true << false << false << false << false;
        QTest::newRow("merged-order-change") << false << true << false << false << false;
        QTest::newRow("unmerged-order-change") << true << true << false << false << false;
        QTest::newRow("merged-full-order-change") << false << true << true << false << false;
        QTest::newRow("unmerged-full-order-change") << true << true << true << false << false;
        QTest::newRow("multi-unmerged-order-change") << true << true << false << true << false;
        QTest::newRow("multi-unmerged-full-order-change") << true << true << true << true << false;
        QTest::newRow("interleaved-unmerged-order-change") << true << true << false << true << true;
        QTest::newRow("interleaved-unmerged-full-order-change") << true << true << true << true << true;
    }

    void unchangedSubrootKeepsUploadedGeometry() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QFETCH(bool, unmerged);
        QFETCH(bool, moveRange);
        QFETCH(bool, forceFull);
        QFETCH(bool, multipleElements);
        QFETCH(bool, interleavedMembers);
        watchedRoot = 0;
        watchedUploads = 0;
        QQuickWindow window;
        window.resize(320, 240);
        window.setColor(Qt::black);
        auto *parent = new QQuickItem(window.contentItem());
        parent->setSize(QSizeF(240, 200));
        parent->setClip(true);
        for (int i = 0; i < 100; ++i) {
            auto *red = new ColorTile(parent, Qt::red);
            red->setSize(parent->size());
        }
        auto *stable = new QQuickItem(parent);
        stable->setPosition(QPointF(32, 32));
        stable->setSize(QSizeF(80, 80));
        stable->setClip(true);
        auto *blue = new ColorTile(stable, QColor(0, 0, 255, 128), unmerged);
        blue->setObjectName("retainedBlueLeaf");
        blue->setSize(QSizeF(48, 48));
        // Distinct materials keep the unmerged rows in single-element batches.
        auto *overlap = new ColorTile(stable, QColor(0, 0, unmerged && !multipleElements ? 254 : 255, 128), unmerged);
        overlap->setObjectName("retainedOverlapLeaf");
        overlap->setPosition(QPointF(16, 16));
        overlap->setSize(QSizeF(48, 48));
        ColorTile *unchangedMiddle = nullptr;
        if (interleavedMembers) {
            auto *middle = new ColorTile(stable, QColor(0, 255, 0, 128), true);
            unchangedMiddle = middle;
            middle->setObjectName("unchangedInterleavedLeaf");
            middle->setPosition(QPointF(72, 72));
            middle->setSize(QSizeF(8, 8));
            middle->stackBefore(overlap);
            const qreal dpr = window.devicePixelRatio();
            const auto origin = middle->mapToScene(QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(middle->mapToScene(QPointF(1, 0))-origin, QPointF(1, 0));
            QCOMPARE(middle->mapToScene(QPointF(0, 1))-origin, QPointF(0, 1));
        }
        auto *lastRoot = new QQuickItem(window.contentItem());
        lastRoot->setPosition(QPointF(256, 0));
        lastRoot->setSize(QSizeF(64, 64));
        lastRoot->setClip(true);
        auto *yellow = new ColorTile(lastRoot, Qt::yellow);
        yellow->setSize(lastRoot->size());
        auto *transient = new QQuickItem(parent);
        transient->setPosition(QPointF(160, 80));
        transient->setSize(QSizeF(32, 32));
        transient->setClip(true);
        auto *ephemeral = new EphemeralClip(parent);
        ephemeral->setSize(QSizeF(4, 4));
        std::atomic<quint64> uploads = 0;
        std::atomic<bool> merged = false;
        std::atomic<int> memberCount = 0;
        std::atomic<int> sceneOrders = 0;
        std::atomic<bool> validRanges = true;
        std::atomic<bool> requestFull = false;
        connect(&window, &QQuickWindow::beforeRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            auto *clip = QQuickItemPrivate::get(stable)->clipNode();
            watchedRoot = renderer && clip ? quintptr(renderer->m_nodes.value(clip)) : 0;
            if (renderer && requestFull.exchange(false))
                renderer->m_rebuild = QSGBatchRenderer::Renderer::FullRebuild;
        }, Qt::DirectConnection);
        connect(&window, &QQuickWindow::afterRendering, &window, [&] {
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            auto *paint = QQuickItemPrivate::get(blue)->paintNode;
            if (!renderer || !paint) return;
            uploads = watchedUploads.load();
            sceneOrders = renderer->m_nextRenderOrder;
            for (auto *node : std::as_const(renderer->m_nodes)) {
                if (node->type() == QSGNode::ClipNodeType || node->isBatchRoot) {
                    const auto *info = node->rootInfo();
                    if (info->availableOrders < 0 || info->firstOrder < 0
                        || info->lastOrder < info->firstOrder) validRanges = false;
                    if (info->parentRoot) {
                        const auto *outer = info->parentRoot->rootInfo();
                        if (info->firstOrder < outer->firstOrder || info->lastOrder > outer->lastOrder)
                            validRanges = false;
                    }
                } else if (node->type() == QSGNode::GeometryNodeType) {
                    const auto *element = node->element();
                    if (element && !element->removed && element->root) {
                        const auto *info = element->root->rootInfo();
                        if (element->order <= info->firstOrder || element->order > info->lastOrder)
                            validRanges = false;
                    }
                }
            }
            auto *shadow = renderer->m_nodes.value(paint);
            if (shadow && shadow->element() && shadow->element()->batch) {
                auto *batch = shadow->element()->batch;
                merged = batch->merged;
                int count = 0;
                for (auto *element = batch->first; element; element = element->nextInBatch) ++count;
                memberCount = count;
            }
        }, Qt::DirectConnection);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const auto before = window.grabWindow();
        QVERIFY(!before.isNull());
        qInfo() << "Actual DPR" << window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(window.devicePixelRatio(), qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        QCOMPARE(merged.load(), !unmerged);
        if (multipleElements) QCOMPARE(memberCount.load(), 2);
        const auto initialUploads = uploads.load();
        QVERIFY(initialUploads > 0);
        auto *added = new ColorTile(parent, Qt::green);
        added->setPosition(QPointF(160, 0));
        added->setSize(QSizeF(32, 32));
        if (moveRange) added->stackBefore(stable);
        requestFull = forceFull;
        const auto after = window.grabWindow();
        if (moveRange && !unmerged) QVERIFY(uploads.load() > initialUploads);
        else QCOMPARE(uploads.load(), initialUploads);
        const qreal dpr = window.devicePixelRatio();
        const QRect retainedRect(qRound(32*dpr), qRound(32*dpr), qRound(80*dpr), qRound(80*dpr));
        QCOMPARE(after.copy(retainedRect), before.copy(retainedRect));
        QCOMPARE(after.pixelColor(qRound(168*dpr), qRound(8*dpr)), QColor(Qt::green));
        for (auto *leaf : {static_cast<QQuickItem *>(blue), static_cast<QQuickItem *>(overlap),
                           static_cast<QQuickItem *>(added), static_cast<QQuickItem *>(yellow)}) {
            const auto origin = leaf->mapToScene(QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(leaf->mapToScene(QPointF(1, 0))-origin, QPointF(1, 0));
            QCOMPARE(leaf->mapToScene(QPointF(0, 1))-origin, QPointF(0, 1));
        }
        QVERIFY(after.save(unmerged ? "retained-unmerged-root-175.png" : "retained-merged-root-175.png"));
        if (unchangedMiddle) {
            const auto beforeOutsideMutation = uploads.load();
            unchangedMiddle->setPosition(QPointF(8, 8));
            unchangedMiddle->setSize(QSizeF(48, 48));
            unchangedMiddle->update();
            const QColor composite = window.grabWindow().pixelColor(qRound(60*dpr), qRound(60*dpr));
            QVERIFY(uploads.load() > beforeOutsideMutation);
            QVERIFY(qAbs(composite.red() - 31) <= 2);
            QVERIFY(qAbs(composite.green() - 64) <= 2);
            QVERIFY(qAbs(composite.blue() - 160) <= 2);
            unchangedMiddle->setPosition(QPointF(72, 72));
            unchangedMiddle->setSize(QSizeF(8, 8));
            unchangedMiddle->update();
            QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
        }
        if (multipleElements) {
            const auto beforeInterleaving = uploads.load();
            auto *interleaved = new ColorTile(stable, QColor(0, 255, 0, 128), true);
            interleaved->setObjectName("retainedInterleavedLeaf");
            interleaved->setPosition(QPointF(8, 8));
            interleaved->setSize(QSizeF(48, 48));
            interleaved->stackBefore(overlap);
            const QImage interleavedImage = window.grabWindow();
            QVERIFY(uploads.load() > beforeInterleaving);
            const QColor composite = interleavedImage.pixelColor(qRound(60*dpr), qRound(60*dpr));
            QVERIFY(qAbs(composite.red() - 31) <= 2);
            QVERIFY(qAbs(composite.green() - 64) <= 2);
            QVERIFY(qAbs(composite.blue() - 160) <= 2);
            const auto origin = interleaved->mapToScene(QPointF());
            QVERIFY(qAbs(origin.x()*dpr-qRound(origin.x()*dpr)) < .001);
            QVERIFY(qAbs(origin.y()*dpr-qRound(origin.y()*dpr)) < .001);
            QCOMPARE(interleaved->mapToScene(QPointF(1, 0))-origin, QPointF(1, 0));
            QCOMPARE(interleaved->mapToScene(QPointF(0, 1))-origin, QPointF(0, 1));
            delete interleaved;
            QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
            const auto beforeReorder = uploads.load();
            overlap->stackBefore(blue);
            QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
            QVERIFY(uploads.load() > beforeReorder);
            blue->stackBefore(overlap);
            QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
        }
        const auto retainedUploads = uploads.load();
        blue->setWidth(40);
        blue->update();
        const auto resized = window.grabWindow();
        QVERIFY(uploads.load() > retainedUploads);
        QCOMPARE(resized.pixelColor(qRound(76*dpr), qRound(40*dpr)), QColor(Qt::red));
        blue->setWidth(48);
        blue->update();
        QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
        blue->setColor(QColor(0, 0, 255, 255));
        QCOMPARE(window.grabWindow().pixelColor(qRound(40*dpr), qRound(40*dpr)), QColor(Qt::blue));
        blue->setColor(QColor(0, 0, 255, 128));
        QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
        blue->setColor(QColor(0, 255, 0, 128));
        const auto recolored = window.grabWindow().pixelColor(qRound(40*dpr), qRound(40*dpr));
        QVERIFY(qAbs(recolored.red()-127) <= 1 && qAbs(recolored.green()-128) <= 1);
        QCOMPARE(recolored.blue(), 0);
        blue->setColor(QColor(0, 0, 255, 128));
        QCOMPARE(window.grabWindow().copy(retainedRect), before.copy(retainedRect));
        delete overlap;
        const auto single = window.grabWindow().pixelColor(qRound(60*dpr), qRound(60*dpr));
        QVERIFY(qAbs(single.red()-127) <= 1 && qAbs(single.blue()-128) <= 1);
        QCOMPARE(single.green(), 0);
        const auto beforeGrowthUploads = uploads.load();
        const auto beforeGrowthOrders = sceneOrders.load();
        for (int i = 0; i < 16; ++i) {
            auto *temporary = new ColorTile(transient, Qt::yellow);
            temporary->setSize(transient->size());
        }
        const auto withTransient = window.grabWindow();
        QCOMPARE(uploads.load(), beforeGrowthUploads);
        QCOMPARE(sceneOrders.load(), beforeGrowthOrders);
        QVERIFY(validRanges.load());
        QCOMPARE(withTransient.pixelColor(qRound(168*dpr), qRound(88*dpr)), QColor(Qt::yellow));
        const auto beforeRemovalUploads = uploads.load();
        const auto beforeRemovalOrders = sceneOrders.load();
        delete transient;
        const auto withoutTransient = window.grabWindow();
        QCOMPARE(uploads.load(), beforeRemovalUploads);
        QCOMPARE(sceneOrders.load(), beforeRemovalOrders);
        QCOMPARE(withoutTransient.copy(retainedRect), withTransient.copy(retainedRect));
        QCOMPARE(withoutTransient.pixelColor(qRound(168*dpr), qRound(88*dpr)), QColor(Qt::red));
        QVERIFY(validRanges.load());
        const auto beforeEphemeralUploads = uploads.load();
        const auto beforeEphemeralOrders = sceneOrders.load();
        ephemeral->cycle();
        QCOMPARE(window.grabWindow(), withoutTransient);
        QCOMPARE(uploads.load(), beforeEphemeralUploads);
        QCOMPARE(sceneOrders.load(), beforeEphemeralOrders);
    }

    void clipBelowUninitializedBatchRoot_data() {
        QTest::addColumn<bool>("fullRebuild");
        QTest::newRow("partial-rebuild") << false;
        QTest::newRow("full-rebuild") << true;
    }

    void clipBelowUninitializedBatchRoot() {
        if (!qEnvironmentVariableIsSet("ZOIN_TEST_NATIVE_RENDERER"))
            QSKIP("Requires a native Qt Quick renderer");
        QFETCH(bool, fullRebuild);
        QQuickWindow window;
        window.resize(240, 200);
        auto *background = new ColorTile(window.contentItem(), Qt::red);
        background->setObjectName("lazyBatchRootBackground");
        background->setSize(QSizeF(240, 200));
        std::atomic<bool> completed = false;
        std::atomic<bool> registered = false;
        const auto connection = connect(&window, &QQuickWindow::afterSynchronizing, &window, [&] {
            if (completed.load()) return;
            auto *renderer = static_cast<QSGBatchRenderer::Renderer *>(
                QQuickWindowPrivate::get(&window)->renderer);
            QSGTransformNode transform;
            renderer->rootNode()->appendChildNode(&transform);
            auto *parent = renderer->m_nodes.value(&transform);
            // Qt initializes transform-root data lazily. A revealed clip may
            // arrive after the root flag is set and before that initialization.
            parent->isBatchRoot = true;
            renderer->m_rebuild = fullRebuild
                ? QSGBatchRenderer::Renderer::FullRebuild
                : QSGBatchRenderer::Renderer::BuildBatches;
            auto *clip = new QSGClipNode;
            clip->setIsRectangular(true);
            clip->setClipRect(QRectF(0, 0, 80, 40));
            transform.appendChildNode(clip);
            registered = renderer->m_nodes.contains(clip);
            renderer->rootNode()->removeChildNode(&transform);
            completed = true;
        }, Qt::DirectConnection);
        const auto disconnectProbe = qScopeGuard([&] { disconnect(connection); });
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        QTRY_VERIFY(completed.load());
        QVERIFY(registered.load());
        const qreal dpr = window.devicePixelRatio();
        QCOMPARE(dpr, qreal(1.75));
        const QPointF origin = background->mapToItem(window.contentItem(), QPointF{});
        QCOMPARE(origin * dpr, QPointF(0, 0));
        QCOMPARE(background->mapToItem(window.contentItem(), {1, 0}) - origin, QPointF(1, 0));
        QCOMPARE(background->mapToItem(window.contentItem(), {0, 1}) - origin, QPointF(0, 1));
        const QImage rendered = window.grabWindow();
        QVERIFY(!rendered.isNull());
        QCOMPARE(rendered.pixelColor(qRound(40*dpr), qRound(40*dpr)), QColor(Qt::red));
        QVERIFY(rendered.save(QStringLiteral("lazy-batch-root-%1-175.png").arg(fullRebuild)));
    }

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
