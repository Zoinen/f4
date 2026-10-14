#pragma once

#include "F4IconProvider.h"

#include <QDir>
#include <QQmlComponent>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickView>
#include <QSemaphore>
#include <QTemporaryDir>
#include <ZoinGallery/GalleryRuntime.h>
#include <ZoinGallery/GallerySession.h>

#include <atomic>
#include <memory>

namespace
{
struct IconResizeQueueGate {
    std::atomic<bool> started{false};
    QSemaphore release;
};

// A separate provider holds Qt's shared Image worker. Actual icon requests
// still go through the unmodified F4IconProvider, including its async flag.
class IconResizeBlockingProvider final : public QQuickImageProvider
{
public:
    explicit IconResizeBlockingProvider(std::shared_ptr<IconResizeQueueGate> gate)
        : QQuickImageProvider(Image, ForceAsynchronousImageLoading),
          m_gate(std::move(gate))
    {
    }

    QImage requestImage(const QString &, QSize *size, const QSize &) override
    {
        m_gate->started.store(true);
        // Bound the wait even if a regression aborts fixture teardown.
        m_gate->release.tryAcquire(1, 10000);
        QImage image(1, 1, QImage::Format_ARGB32);
        image.fill(Qt::transparent);
        if (size) {
            *size = image.size();
        }
        return image;
    }

private:
    std::shared_ptr<IconResizeQueueGate> m_gate;
};

class GalleryIconResizeFixture
{
public:
    GalleryIconResizeFixture()
        : icons(QStringLiteral("resize-icons")),
          component(view.engine())
    {
        view.setColor(Qt::white);
        view.engine()->addImportPath(QStringLiteral(":/"));
        view.setResizeMode(QQuickView::SizeRootObjectToView);
        view.resize(128, 192);
        ZoinGallery::RuntimeOptions options;
        options.persistentCache = false;
        auto *runtime = ZoinGallery::GalleryRuntime::install(view.engine(), options);
        view.rootContext()->setContextProperty(
            "resizeSession", runtime->createExternalSession("icon-resize-fixture"));
        view.engine()->addImageProvider("resize-icons", new F4IconProvider);
        view.engine()->addImageProvider(
            "resize-blocker", new IconResizeBlockingProvider(gate));
        view.rootContext()->setContextProperty("resizeIcons", &icons);
        component.setData(R"QML(
            import QtQuick
            import ZoinGallery as ZG
            ZG.GalleryPanel {
                id: panel
                width: 128; height: 192
                devicePixelRatio: 1.75
                debugLayoutZones: false
                emptyStateEnabled: false
                showCursor: false
                iconProvider: resizeIcons
                session: resizeSession
                property string sourceAtIdentityNotification: "unobserved"
                property string sourceAtRowNotification: "unobserved"
                property bool changingAtIdentityNotification: false
                property bool changingAtRowNotification: false
                property bool inspectThumbnail: false
                function activeFallbackLoader() {
                    const children = fixtureEntry.previewContainerItem.children
                    for (let index = 0; index < children.length; ++index) {
                        if (children[index].modelIconSource !== undefined)
                            return children[index]
                    }
                    return null
                }
                function recordIdentitySource() {
                    // Read only the active Loader item, not a retired leaf
                    // awaiting deferred deletion after the identity reset.
                    if (inspectThumbnail)
                        return fixtureEntry.previewContainerItem.thumbnailItem.source.toString()
                    const loader = panel.activeFallbackLoader()
                    const leaf = loader ? loader.item : null
                    return leaf ? leaf.source.toString() : ""
                }
                Connections {
                    target: fixtureEntry
                    function onVisualIdentityChanged() {
                        panel.sourceAtIdentityNotification = panel.recordIdentitySource()
                        panel.changingAtIdentityNotification = fixtureEntry.visualIdentityChanging === true
                    }
                    function onVisualRowChanged() {
                        panel.sourceAtRowNotification = panel.recordIdentitySource()
                        panel.changingAtRowNotification = fixtureEntry.visualIdentityChanging === true
                    }
                }
                iconResolver.compactPrefix: "qrc:/F4QtHost/icons/lucide"
                iconResolver.largePrefix: "qrc:/F4QtHost/icons/lucide-gallery"
                theme.folderIcon: "#00a080"
                theme.mutedText: "#e03050"
                Image { objectName: "resizeBlockerImage"; visible: false }
                ZG.GalleryEntryDelegateBase {
                    id: fixtureEntry
                    objectName: "resizeEntry"
                    panelRoot: panel
                    anchors.fill: parent
                    viewIndex: 0
                    masonryGeometryReady: true
                    previewRect: Qt.rect(0, 0, width, height)
                    visualRow: ({valid: true, entryId: "folder-A", text: "",
                                 isFolder: true, iconKey: "folder"})
                }
            }
        )QML", QUrl("inline:GalleryIconResize.qml"));
    }

    QQuickItem *icon() const
    {
        auto *entry = view.rootObject()->findChild<QObject *>("resizeEntry");
        auto *preview = entry->property("previewContainerItem").value<QQuickItem *>();
        for (auto *child : preview->childItems()) {
            if (child->property("modelIconSource").isValid()) {
                return child->property("item").value<QQuickItem *>();
            }
        }
        return nullptr;
    }

    QQuickItem *compactImage() const
    {
        auto *entry = view.rootObject()->findChild<QObject *>("resizeEntry");
        auto *preview = entry->property("previewContainerItem").value<QQuickItem *>();
        for (auto *child : preview->childItems()) {
            if (!child->property("sourceColorIconNeeded").isValid()) {
                continue;
            }
            auto *activeItem = child->property("item").value<QQuickItem *>();
            return activeItem ? activeItem->findChild<QQuickItem *>(
                "galleryThumbnailImage-0") : nullptr;
        }
        return nullptr;
    }

    void finishCaptures()
    {
        captures.setAutoRemove(!qEnvironmentVariableIsSet(
            "F4_GALLERY_ICON_RESIZE_KEEP_CAPTURES"));
    }

    QString readyLeafGeometryError(QQuickItem *leaf) const
    {
        if (!leaf || leaf->property("status").toInt() != 1) {
            return QStringLiteral(
                "Ready geometry check requires the actual Ready image leaf");
        }
        const qreal dpr = view.devicePixelRatio();
        const QPointF origin = leaf->mapToItem(view.contentItem(), QPointF());
        const QPointF xUnit = leaf->mapToItem(
            view.contentItem(), QPointF(1, 0)) - origin;
        const QPointF yUnit = leaf->mapToItem(
            view.contentItem(), QPointF(0, 1)) - origin;
        const QList<qreal> physicalValues{
            origin.x() * dpr, origin.y() * dpr,
            leaf->width() * dpr, leaf->height() * dpr};
        for (qreal value : physicalValues) {
            if (qAbs(value - qRound(value)) >= 0.001) {
                return QStringLiteral(
                    "%1 physical origin/size %2,%3 %4x%5 is off grid at DPR %6")
                    .arg(leaf->objectName())
                    .arg(physicalValues[0]).arg(physicalValues[1])
                    .arg(physicalValues[2]).arg(physicalValues[3]).arg(dpr);
            }
        }
        if (qAbs(xUnit.x() - 1) >= 0.001 || qAbs(xUnit.y()) >= 0.001
            || qAbs(yUnit.x()) >= 0.001 || qAbs(yUnit.y() - 1) >= 0.001) {
            return QStringLiteral(
                "%1 inherits a non-unit scene transform: (%2,%3),(%4,%5)")
                .arg(leaf->objectName()).arg(xUnit.x()).arg(xUnit.y())
                .arg(yUnit.x()).arg(yUnit.y());
        }
        return {};
    }

    int capture(const QString &phase, QQuickItem *imageItem = nullptr)
    {
        const QImage image = view.grabWindow();
        image.save(captures.filePath(phase + QStringLiteral(".png")));
        int count = 0;
        cyanPixels = 0;
        redPixels = 0;
        for (int y = 0; y < image.height(); ++y) {
            for (int x = 0; x < image.width(); ++x) {
                const QColor color = image.pixelColor(x, y);
                const bool cyan = color.green() > color.red() + 20
                    && color.blue() > color.red() + 20;
                const bool red = color.red() > color.green() + 30
                    && color.red() > color.blue() + 30;
                if (cyan) {
                    ++cyanPixels;
                }
                if (red) {
                    ++redPixels;
                }
                if (cyan || red) {
                    ++count;
                }
            }
        }
        if (!imageItem) {
            imageItem = icon();
        }
        qInfo() << "[gallery-icon-resize]" << phase
                << "status" << imageItem->property("status")
                << "source" << imageItem->property("source")
                << "sourceSize" << imageItem->property("sourceSize")
                << "paintedPixels" << count
                << "capture" << captures.filePath(phase + QStringLiteral(".png"));
        return count;
    }

    // Keep failed regression captures available outside the build directory.
    QTemporaryDir captures{
        QDir::tempPath() + QStringLiteral("/f4-gallery-icon-resize-XXXXXX")};
    int cyanPixels = 0;
    int redPixels = 0;
    std::shared_ptr<IconResizeQueueGate> gate =
        std::make_shared<IconResizeQueueGate>();
    F4IconSet icons;
    QQuickView view;
    QQmlComponent component;
};
}
