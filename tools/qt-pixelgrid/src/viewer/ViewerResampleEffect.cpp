#include "ViewerResampleEffect.h"

#include <QDebug>
#include <QQuickWindow>
#include <QSGGeometryNode>
#include <QSGMaterial>
#include <QSGMaterialShader>
#include <QSGTexture>
#include <QSGTextureProvider>
#include <rhi/qrhi.h>

#include <cstddef>
#include <cstring>
#include <memory>
#include <atomic>

namespace {

// std140 layout shared by viewer_resample.vert and viewer_resample.frag.
struct alignas(16) ResampleUniforms {
    float matrix[16];
    float opacity;
    float padding0;
    float viewportSize[2];
    float checkerboardOffset[2];
    qint32 showCheckerboard;
    qint32 checkerboardSize;
    float borderRadius;
    qint32 intermediate;
    qint32 pixelAlignedIdentity;
    float padding1;
    float sourceExtent[2];
    qint32 pixelAligned;
    float padding2;
    float itemSize[2];
    float padding3[2];
    float framebufferRect[4];
    float framebufferYDirection;
};

static_assert(offsetof(ResampleUniforms, opacity) == 64);
static_assert(offsetof(ResampleUniforms, viewportSize) == 72);
static_assert(offsetof(ResampleUniforms, checkerboardOffset) == 80);
static_assert(offsetof(ResampleUniforms, showCheckerboard) == 88);
static_assert(offsetof(ResampleUniforms, checkerboardSize) == 92);
static_assert(offsetof(ResampleUniforms, borderRadius) == 96);
static_assert(offsetof(ResampleUniforms, intermediate) == 100);
static_assert(offsetof(ResampleUniforms, pixelAlignedIdentity) == 104);
static_assert(offsetof(ResampleUniforms, sourceExtent) == 112);
static_assert(offsetof(ResampleUniforms, pixelAligned) == 120);
static_assert(offsetof(ResampleUniforms, itemSize) == 128);
static_assert(offsetof(ResampleUniforms, framebufferRect) == 144);
static_assert(offsetof(ResampleUniforms, framebufferYDirection) == 160);
constexpr qsizetype UniformByteSize = offsetof(ResampleUniforms, framebufferYDirection)
                                     + sizeof(float);

class ResampleMaterial;

class ResampleShader final : public QSGMaterialShader
{
public:
    ResampleShader()
    {
        setShaderFileName(VertexStage,
            QStringLiteral(":/ZoinGallery/resources/viewer_resample.vert.qsb"));
        setShaderFileName(FragmentStage,
            QStringLiteral(":/ZoinGallery/resources/viewer_resample.frag.qsb"));
    }

    bool updateUniformData(RenderState &state, QSGMaterial *newMaterial,
                           QSGMaterial *oldMaterial) override;
    void updateSampledImage(RenderState &state, int binding, QSGTexture **texture,
                            QSGMaterial *newMaterial, QSGMaterial *oldMaterial) override;
};

class ResampleMaterial final : public QSGMaterial
{
public:
    explicit ResampleMaterial(QQuickWindow *window)
    {
        setFlag(Blending | RequiresFullMatrix, true);
        QImage transparent(1, 1, QImage::Format_RGBA8888_Premultiplied);
        transparent.fill(Qt::transparent);
        fallbackTexture.reset(window->createTextureFromImage(transparent));
    }

    QSGMaterialType *type() const override
    {
        static QSGMaterialType type;
        return &type;
    }

    QSGMaterialShader *createShader(QSGRendererInterface::RenderMode) const override
    {
        return new ResampleShader;
    }

    QPointer<QSGTextureProvider> provider;
    std::unique_ptr<QSGTexture> fallbackTexture;
    QSizeF viewportSize;
    QSizeF sourceExtent;
    QSizeF itemSize;
    QVector2D checkerboardOffset;
    bool showCheckerboard = false;
    int checkerboardSize = 4;
    qreal borderRadius = 0;
    bool intermediate = false;
    bool pixelAligned = false;
    bool pixelAlignedIdentity = false;
};

bool ResampleShader::updateUniformData(RenderState &state, QSGMaterial *newMaterial,
                                      QSGMaterial *)
{
    const auto *material = static_cast<ResampleMaterial *>(newMaterial);
    QByteArray *buffer = state.uniformData();
    if (buffer->size() < UniformByteSize)
        return false;

    ResampleUniforms uniforms{};
    const QMatrix4x4 matrix = state.combinedMatrix();
    std::memcpy(uniforms.matrix, matrix.constData(), sizeof(uniforms.matrix));
    uniforms.opacity = state.opacity();
    uniforms.viewportSize[0] = float(material->viewportSize.width());
    uniforms.viewportSize[1] = float(material->viewportSize.height());
    uniforms.checkerboardOffset[0] = material->checkerboardOffset.x();
    uniforms.checkerboardOffset[1] = material->checkerboardOffset.y();
    uniforms.showCheckerboard = material->showCheckerboard;
    uniforms.checkerboardSize = material->checkerboardSize;
    uniforms.borderRadius = float(material->borderRadius);
    uniforms.intermediate = material->intermediate;
    uniforms.pixelAlignedIdentity = material->pixelAlignedIdentity;
    uniforms.sourceExtent[0] = float(material->sourceExtent.width());
    uniforms.sourceExtent[1] = float(material->sourceExtent.height());
    uniforms.pixelAligned = material->pixelAligned;
    uniforms.itemSize[0] = float(material->itemSize.width());
    uniforms.itemSize[1] = float(material->itemSize.height());

    // This is the active draw target, including layer and render-control
    // targets. Nominal DPR and rounded logical window sizes are insufficient.
    const QRect viewport = state.viewportRect();
    uniforms.framebufferRect[0] = float(viewport.x());
    uniforms.framebufferRect[1] = float(viewport.y());
    uniforms.framebufferRect[2] = float(viewport.width());
    uniforms.framebufferRect[3] = float(viewport.height());
    const QRhi *rhi = state.rhi();
    uniforms.framebufferYDirection = rhi->isYUpInFramebuffer() == rhi->isYUpInNDC()
        ? 1.0f : -1.0f;
    if (qEnvironmentVariableIsSet("F4_VIEWER_RESAMPLE_DEBUG")) {
        static std::atomic<int> emitted = 0;
        if (emitted.fetch_add(1, std::memory_order_relaxed) < 24) {
            qInfo().noquote() << QStringLiteral(
                "[FIX:viewer-resample-dpr] targetDpr=%1 viewport=(%2,%3 %4x%5) "
                "requested=%6x%7 item=%8x%9 aligned=%10 intermediate=%11")
                .arg(state.devicePixelRatio())
                .arg(viewport.x()).arg(viewport.y())
                .arg(viewport.width()).arg(viewport.height())
                .arg(material->viewportSize.width())
                .arg(material->viewportSize.height())
                .arg(material->itemSize.width()).arg(material->itemSize.height())
                .arg(material->pixelAligned).arg(material->intermediate);
        }
    }
    std::memcpy(buffer->data(), &uniforms, UniformByteSize);
    return true;
}

void ResampleShader::updateSampledImage(RenderState &state, int binding,
                                       QSGTexture **texture, QSGMaterial *newMaterial,
                                       QSGMaterial *)
{
    if (binding != 1)
        return;
    auto *material = static_cast<ResampleMaterial *>(newMaterial);
    QSGTexture *selected = material->provider ? material->provider->texture() : nullptr;
    if (selected && selected->isAtlasTexture()) {
        selected->commitTextureOperations(state.rhi(), state.resourceUpdateBatch());
        // Qt's atlas texture owns and caches the extracted texture. Use
        // the current update batch so the upload precedes the extraction.
        selected = selected->removedFromAtlas(state.resourceUpdateBatch());
    }
    if (!selected)
        selected = material->fallbackTexture.get();
    if (selected) {
        selected->setFiltering(QSGTexture::Linear);
        selected->setMipmapFiltering(QSGTexture::None);
        selected->setHorizontalWrapMode(QSGTexture::ClampToEdge);
        selected->setVerticalWrapMode(QSGTexture::ClampToEdge);
        selected->commitTextureOperations(state.rhi(), state.resourceUpdateBatch());
    }
    *texture = selected;
}

class ResampleNode final : public QObject, public QSGGeometryNode
{
public:
    explicit ResampleNode(QQuickWindow *window)
        : geometry(QSGGeometry::defaultAttributes_TexturedPoint2D(), 4), material(window)
    {
        geometry.setDrawingMode(QSGGeometry::DrawTriangleStrip);
        setGeometry(&geometry);
        setMaterial(&material);
        setFlag(UsePreprocess, true);
    }

    void setProvider(QSGTextureProvider *provider)
    {
        if (material.provider == provider)
            return;
        QObject::disconnect(textureChanged);
        QObject::disconnect(providerDestroyed);
        material.provider = provider;
        if (provider) {
            textureChanged = QObject::connect(provider, &QSGTextureProvider::textureChanged,
                this, [this] { markDirty(DirtyMaterial); }, Qt::DirectConnection);
            providerDestroyed = QObject::connect(provider, &QObject::destroyed,
                this, [this] { material.provider = nullptr; markDirty(DirtyMaterial); },
                Qt::DirectConnection);
        }
        markDirty(DirtyMaterial);
    }

    void preprocess() override
    {
        QSGTexture *texture = material.provider ? material.provider->texture() : nullptr;
        auto *dynamic = qobject_cast<QSGDynamicTexture *>(texture);
        if (dynamic && dynamic->updateTexture())
            markDirty(DirtyMaterial);
    }

    QSGGeometry geometry;
    ResampleMaterial material;
    QMetaObject::Connection textureChanged;
    QMetaObject::Connection providerDestroyed;
};

} // namespace

ViewerResampleEffect::ViewerResampleEffect(QQuickItem *parent) : QQuickItem(parent)
{
    setFlag(ItemHasContents, true);
}

void ViewerResampleEffect::setSource(QQuickItem *source)
{
    if (m_source == source)
        return;
    QObject::disconnect(m_sourceDestroyed);
    QObject::disconnect(m_sourceWindowChanged);
    m_source = source;
    if (source) {
        m_sourceDestroyed = connect(source, &QObject::destroyed, this, [this] {
            m_source = nullptr;
            emit sourceChanged();
            update();
        });
        m_sourceWindowChanged = connect(source, &QQuickItem::windowChanged,
            this, [this] { update(); });
    }
    emit sourceChanged();
    update();
}

void ViewerResampleEffect::setViewportSize(QSizeF size)
{
    if (m_viewportSize == size)
        return;
    m_viewportSize = size;
    emit viewportSizeChanged();
    update();
}

void ViewerResampleEffect::setSourceExtent(QSizeF size)
{
    if (m_sourceExtent == size)
        return;
    m_sourceExtent = size;
    emit sourceExtentChanged();
    update();
}

void ViewerResampleEffect::setCheckerboardOffset(QVector2D offset)
{
    if (m_checkerboardOffset == offset)
        return;
    m_checkerboardOffset = offset;
    emit checkerboardOffsetChanged();
    update();
}

void ViewerResampleEffect::setShowCheckerboard(bool enabled)
{
    if (m_showCheckerboard == enabled)
        return;
    m_showCheckerboard = enabled;
    emit showCheckerboardChanged();
    update();
}

void ViewerResampleEffect::setCheckerboardSize(int size)
{
    if (m_checkerboardSize == size)
        return;
    m_checkerboardSize = size;
    emit checkerboardSizeChanged();
    update();
}

void ViewerResampleEffect::setBorderRadius(qreal radius)
{
    if (m_borderRadius == radius)
        return;
    m_borderRadius = radius;
    emit borderRadiusChanged();
    update();
}

void ViewerResampleEffect::setIntermediate(bool enabled)
{
    if (m_intermediate == enabled)
        return;
    m_intermediate = enabled;
    emit intermediateChanged();
    update();
}

void ViewerResampleEffect::setPixelAligned(bool enabled)
{
    if (m_pixelAligned == enabled)
        return;
    m_pixelAligned = enabled;
    emit pixelAlignedChanged();
    update();
}

void ViewerResampleEffect::setPixelAlignedIdentity(bool enabled)
{
    if (m_pixelAlignedIdentity == enabled)
        return;
    m_pixelAlignedIdentity = enabled;
    emit pixelAlignedIdentityChanged();
    update();
}

QSGNode *ViewerResampleEffect::updatePaintNode(QSGNode *oldNode, UpdatePaintNodeData *)
{
    auto *node = static_cast<ResampleNode *>(oldNode);
    if (width() <= 0 || height() <= 0 || !window()) {
        delete node;
        return nullptr;
    }
    if (!node)
        node = new ResampleNode(window());
    node->setProvider(m_source && m_source->isTextureProvider()
        ? m_source->textureProvider() : nullptr);

    ResampleMaterial &material = node->material;
    material.viewportSize = m_viewportSize;
    material.sourceExtent = m_sourceExtent;
    material.itemSize = size();
    material.checkerboardOffset = m_checkerboardOffset;
    material.showCheckerboard = m_showCheckerboard;
    material.checkerboardSize = m_checkerboardSize;
    material.borderRadius = m_borderRadius;
    material.intermediate = m_intermediate;
    material.pixelAligned = m_pixelAligned;
    material.pixelAlignedIdentity = m_pixelAlignedIdentity;
    QSGGeometry::updateTexturedRectGeometry(&node->geometry, boundingRect(), QRectF(0, 0, 1, 1));
    node->markDirty(QSGNode::DirtyGeometry | QSGNode::DirtyMaterial);
    return node;
}

void ViewerResampleEffect::geometryChange(const QRectF &newGeometry, const QRectF &oldGeometry)
{
    QQuickItem::geometryChange(newGeometry, oldGeometry);
    if (newGeometry.size() != oldGeometry.size())
        update();
}
