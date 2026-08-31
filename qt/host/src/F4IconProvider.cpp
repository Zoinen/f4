#include "F4IconProvider.h"

#include <QAbstractFileIconProvider>
#include <QByteArray>
#include <QColor>
#include <QEvent>
#include <QFileInfo>
#include <QGuiApplication>
#include <QMimeDatabase>
#include <QMimeType>
#include <QPainter>
#include <QPalette>
#include <QPixmap>
#include <QSet>
#include <QSvgRenderer>
#include <QUrlQuery>

#include <algorithm>
#include <cmath>
#include <limits>
#include <utility>

#ifdef Q_OS_WIN
#ifndef NOMINMAX
#define NOMINMAX
#endif
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#include <objbase.h>
// wingdi.h (pulled in transitively) #defines DocumentProperties to the ANSI/
// wide entry point; QIcon::ThemeIcon::DocumentProperties below is an
// unrelated enumerator that must not be macro-substituted.
#undef DocumentProperties
#endif

namespace
{

#ifdef Q_OS_WIN
// QAbstractFileIconProvider's Windows backend queries the shell
// (SHGetFileInfo/IExtractIcon), which needs COM initialized on the calling
// thread. Qt Quick's asynchronous image-loader threads are not COM
// initialized by default: the very first shell query on a fresh worker
// thread silently auto-initializes COM apartment-style and can hand back the
// generic "unknown file" icon instead of the real one, while a later query
// on that now-initialized thread returns the correct icon. That is why a
// folder can flash a file glyph for a single frame right after the fast
// generic pass already showed the correct folder icon. Keep one apartment
// alive for the lifetime of each worker thread that reaches here.
struct ComApartmentGuard
{
    ComApartmentGuard()
    {
        const HRESULT result = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
        // S_FALSE means COM was already initialized on this thread (for
        // instance by Qt itself); only balance the calls this guard made.
        initialized = (result == S_OK || result == S_FALSE);
    }
    ~ComApartmentGuard()
    {
        if (initialized) {
            CoUninitialize();
        }
    }
    bool initialized = false;
};

void ensureComInitializedForCurrentThread()
{
    thread_local const ComApartmentGuard guard;
    Q_UNUSED(guard);
}
#endif

constexpr int MinLogicalIconSize = 1;
constexpr int MaxLogicalIconSize = 1024;
constexpr qreal MinDevicePixelRatio = 0.5;
constexpr qreal MaxDevicePixelRatio = 8.0;
constexpr int LargeLucideIconThreshold = 48;

const QSet<QString> &lucideIconNames()
{
    static const QSet<QString> names{
        QStringLiteral("archive"),
        QStringLiteral("arrow-down"),
        QStringLiteral("arrow-down-a-z"),
        QStringLiteral("arrow-down-wide-narrow"),
        QStringLiteral("arrow-up"),
        QStringLiteral("binary"),
        QStringLiteral("book-open"),
        QStringLiteral("check"),
        QStringLiteral("circle-check"),
        QStringLiteral("circle-play"),
        QStringLiteral("circle-question-mark"),
        QStringLiteral("circle-x"),
        QStringLiteral("chevron-down"),
        QStringLiteral("chevron-right"),
        QStringLiteral("clock-3"),
        QStringLiteral("cloud"),
        QStringLiteral("columns-2"),
        QStringLiteral("columns-3"),
        QStringLiteral("copy"),
        QStringLiteral("database"),
        QStringLiteral("eye"),
        QStringLiteral("file"),
        QStringLiteral("file-clock"),
        QStringLiteral("file-code"),
        QStringLiteral("file-cog"),
        QStringLiteral("file-lock"),
        QStringLiteral("file-pen-line"),
        QStringLiteral("file-plus"),
        QStringLiteral("file-text"),
        QStringLiteral("file-type"),
        QStringLiteral("flask-conical"),
        QStringLiteral("folder"),
        QStringLiteral("folder-clock"),
        QStringLiteral("folder-input"),
        QStringLiteral("folder-kanban"),
        QStringLiteral("folder-lock"),
        QStringLiteral("folder-plus"),
        QStringLiteral("folder-root"),
        QStringLiteral("folder-up"),
        QStringLiteral("git-fork"),
        QStringLiteral("globe"),
        QStringLiteral("grid-3x3"),
        QStringLiteral("hard-drive"),
        QStringLiteral("house"),
        QStringLiteral("image"),
        QStringLiteral("images"),
        QStringLiteral("layout-dashboard"),
        QStringLiteral("languages"),
        QStringLiteral("library"),
        QStringLiteral("list"),
        QStringLiteral("list-checks"),
        QStringLiteral("list-restart"),
        QStringLiteral("list-tree"),
        QStringLiteral("loader-circle"),
        QStringLiteral("locate-fixed"),
        QStringLiteral("log-out"),
        QStringLiteral("maximize-2"),
        QStringLiteral("menu"),
        QStringLiteral("monitor"),
        QStringLiteral("music"),
        QStringLiteral("network"),
        QStringLiteral("panel-left"),
        QStringLiteral("panel-right"),
        QStringLiteral("panels-top-left"),
        QStringLiteral("palette"),
        QStringLiteral("pencil"),
        QStringLiteral("plug"),
        QStringLiteral("plus"),
        QStringLiteral("refresh-cw"),
        QStringLiteral("rotate-ccw"),
        QStringLiteral("rows-3"),
        QStringLiteral("rows-4"),
        QStringLiteral("save"),
        QStringLiteral("search"),
        QStringLiteral("sparkles"),
        QStringLiteral("space"),
        QStringLiteral("square-terminal"),
        QStringLiteral("text-wrap"),
        QStringLiteral("trash-2"),
        QStringLiteral("triangle-alert"),
        QStringLiteral("video"),
        QStringLiteral("x"),
    };
    return names;
}

const QSet<QString> &streamlineIconNames()
{
    static const QSet<QString> names{
        QStringLiteral("android-logo"),
        QStringLiteral("apple-logo"),
    };
    return names;
}

bool isStreamlineIconName(QStringView name)
{
    return streamlineIconNames().contains(name.toString());
}

QUrl streamlineSource(const QString &iconName)
{
    return QUrl(QStringLiteral("qrc:/F4QtHost/icons/streamline/%1.svg")
                    .arg(iconName));
}

QIcon firstThemeIcon(std::initializer_list<QString> names)
{
    for (const QString &name : names) {
        QIcon icon = QIcon::fromTheme(name);
        if (!icon.isNull()) {
            return icon;
        }
    }
    return {};
}

class QtIconProviderBackend final : public F4IconProviderBackend
{
public:
    QIcon iconForName(const QString &rawName) const override
    {
        const QString name = F4IconProvider::normalizedIconName(rawName);
        if (name == QStringLiteral("folder")
            || name == QStringLiteral("folder-kanban")) {
            QIcon icon = name == QStringLiteral("folder-kanban")
                ? firstThemeIcon({QStringLiteral("system-file-manager")})
                : QIcon();
            return icon.isNull()
                ? m_fileIcons.icon(QAbstractFileIconProvider::Folder)
                : icon;
        }
        if (name == QStringLiteral("folder-up")) {
            return QIcon::fromTheme(QIcon::ThemeIcon::GoUp,
                                    m_fileIcons.icon(QAbstractFileIconProvider::Folder));
        }
        if (name == QStringLiteral("folder-lock")) {
            QIcon icon = firstThemeIcon({QStringLiteral("folder-locked"),
                                         QStringLiteral("folder-private")});
            return icon.isNull()
                ? m_fileIcons.icon(QAbstractFileIconProvider::Folder)
                : icon;
        }
        if (name == QStringLiteral("file")) {
            return m_fileIcons.icon(QAbstractFileIconProvider::File);
        }

        QIcon icon;
        if (name == QStringLiteral("square-terminal")) {
            icon = firstThemeIcon({QStringLiteral("utilities-terminal")});
        } else if (name == QStringLiteral("file-code")) {
            icon = firstThemeIcon({QStringLiteral("accessories-text-editor"),
                                   QStringLiteral("text-x-script")});
            if (icon.isNull()) {
                icon = QIcon::fromTheme(QIcon::ThemeIcon::DocumentNew);
            }
        } else if (name == QStringLiteral("file-text")) {
            icon = firstThemeIcon({QStringLiteral("text-x-generic")});
        } else if (name == QStringLiteral("file-clock")) {
            icon = QIcon::fromTheme(QIcon::ThemeIcon::DocumentOpenRecent);
        } else if (name == QStringLiteral("file-cog")) {
            icon = QIcon::fromTheme(QIcon::ThemeIcon::DocumentProperties);
        } else if (name == QStringLiteral("file-lock")) {
            icon = QIcon::fromTheme(QIcon::ThemeIcon::DialogPassword);
        } else if (name == QStringLiteral("image")) {
            icon = QIcon::fromTheme(QIcon::ThemeIcon::InsertImage);
        } else if (name == QStringLiteral("video")) {
            icon = QIcon::fromTheme(QIcon::ThemeIcon::CameraVideo);
        } else if (name == QStringLiteral("music")) {
            icon = QIcon::fromTheme(QIcon::ThemeIcon::MultimediaPlayer);
        } else if (name == QStringLiteral("globe")) {
            icon = firstThemeIcon({QStringLiteral("applications-internet"),
                                   QStringLiteral("network-workgroup")});
        } else if (name == QStringLiteral("library")
                   || name == QStringLiteral("book-open")) {
            icon = firstThemeIcon({QStringLiteral("accessories-dictionary"),
                                   QStringLiteral("emblem-documents")});
        } else if (name == QStringLiteral("database")) {
            icon = firstThemeIcon({QStringLiteral("server-database"),
                                   QStringLiteral("drive-harddisk")});
        } else if (name == QStringLiteral("archive")) {
            icon = firstThemeIcon({QStringLiteral("package-x-generic"),
                                   QStringLiteral("application-x-archive")});
        }

        // A platform or Freedesktop theme can know a name that is not in the
        // common mapping above. The Lucide resource remains the final fallback.
        if (icon.isNull()) {
            icon = QIcon::fromTheme(name);
        }
        return icon;
    }

    QIcon iconForFile(const QString &localPath,
                      const QString &fileName,
                      bool directory) const override
    {
        if (!localPath.isEmpty()) {
            const QFileInfo info(localPath);
            // For a declared directory, avoid asking the MIME provider to
            // treat a temporarily missing path as an ordinary file. Existing
            // paths still go through the native provider so custom folder and
            // executable icons, aliases, and overlays are retained.
            if (!directory || info.exists()) {
                QIcon icon = m_fileIcons.icon(info);
                if (!icon.isNull()) {
                    return icon;
                }
            }
        }

        if (directory) {
            return m_fileIcons.icon(QAbstractFileIconProvider::Folder);
        }

        QString probeName = fileName;
        if (probeName.isEmpty()) {
            probeName = QFileInfo(localPath).fileName();
        }
        if (!probeName.isEmpty()) {
            const QMimeType mime = m_mimeDatabase.mimeTypeForFile(
                probeName, QMimeDatabase::MatchExtension);
            if (mime.isValid()) {
                QIcon icon;
                if (!mime.iconName().isEmpty()) {
                    icon = QIcon::fromTheme(mime.iconName());
                }
                if (icon.isNull() && !mime.genericIconName().isEmpty()) {
                    icon = QIcon::fromTheme(mime.genericIconName());
                }
                if (!icon.isNull()) {
                    return icon;
                }
            }
        }
        return m_fileIcons.icon(QAbstractFileIconProvider::File);
    }

private:
    mutable QAbstractFileIconProvider m_fileIcons;
    mutable QMimeDatabase m_mimeDatabase;
};

QString normalizedProviderId(QString providerId)
{
    providerId = providerId.trimmed().toLower();
    if (providerId.isEmpty()) {
        return F4IconSet::defaultProviderId();
    }
    for (const QChar character : std::as_const(providerId)) {
        if (!character.isLetterOrNumber()
            && character != u'-' && character != u'_') {
            return F4IconSet::defaultProviderId();
        }
    }
    return providerId;
}

QString resourceFileName(const QUrl &url)
{
    return url.scheme() == QStringLiteral("qrc")
        ? QStringLiteral(":") + url.path()
        : url.toString();
}

QImage imageAtPhysicalSize(const QPixmap &pixmap, const QSize &targetSize)
{
    if (pixmap.isNull() || targetSize.width() <= 0
        || targetSize.height() <= 0) {
        return {};
    }

    QImage source = pixmap.toImage();
    // QQuickImageProvider receives physical sourceSize pixels. Qt Quick owns
    // their device-independent interpretation, so do not let the QImage's DPR
    // apply a second scale when it becomes a scene-graph texture.
    source.setDevicePixelRatio(1.0);
    if (source.size() == targetSize) {
        return source;
    }

    const QImage scaled = source.scaled(targetSize,
                                        Qt::KeepAspectRatio,
                                        Qt::SmoothTransformation);
    QImage result(targetSize, QImage::Format_ARGB32_Premultiplied);
    result.fill(Qt::transparent);
    QPainter painter(&result);
    painter.drawImage((targetSize.width() - scaled.width()) / 2,
                      (targetSize.height() - scaled.height()) / 2,
                      scaled);
    return result;
}

QColor fallbackTintColor()
{
    QColor color(154, 167, 181);
    if (qGuiApp) {
        const QColor paletteText = qGuiApp->palette().color(QPalette::Text);
        if (paletteText.isValid() && paletteText.alpha() != 0
            && qGray(paletteText.rgb()) >= 96) {
            color = paletteText;
        }
    }
    return color;
}

void tintMask(QImage &image, const QColor &requestedColor)
{
    if (image.isNull()) {
        return;
    }
    image = image.convertToFormat(QImage::Format_ARGB32_Premultiplied);
    image.setDevicePixelRatio(1.0);
    const QColor color = requestedColor.isValid()
        ? requestedColor
        : fallbackTintColor();
    for (int y = 0; y < image.height(); ++y) {
        auto *scanline = reinterpret_cast<QRgb *>(image.scanLine(y));
        for (int x = 0; x < image.width(); ++x) {
            const int alpha = qAlpha(scanline[x]) * color.alpha() / 255;
            scanline[x] = qRgba(color.red() * alpha / 255,
                                color.green() * alpha / 255,
                                color.blue() * alpha / 255,
                                alpha);
        }
    }
}

QImage renderLucideImage(const QString &iconName, const QSize &targetSize,
                         int logicalSize, const QColor &tint)
{
    if (targetSize.width() <= 0 || targetSize.height() <= 0) {
        return {};
    }
    const QUrl source = F4IconProvider::lucideSource(iconName, logicalSize);
    QSvgRenderer renderer(resourceFileName(source));
    if (!renderer.isValid()) {
        return {};
    }

    QImage image(targetSize, QImage::Format_ARGB32_Premultiplied);
    image.fill(Qt::transparent);
    QPainter painter(&image);
    if (!painter.isActive()) {
        return {};
    }
    renderer.render(&painter, QRectF(QPointF(0, 0), QSizeF(targetSize)));
    painter.end();
    tintMask(image, tint);
    return image;
}

QPixmap paletteTintedMask(QPixmap pixmap)
{
    if (pixmap.isNull()) {
        return {};
    }

    // Lucide resources use currentColor. When they are loaded directly by
    // QIcon's SVG engine that resolves to black, whereas a system file icon is
    // intentionally shown by QML as an untinted, full-color Image. Tint only
    // this final fallback here so it remains legible with a dark palette while
    // leaving native multicolor icons completely untouched.
    // The QML frontend uses a deliberately dark surface independently of the
    // desktop's light/dark setting. Start with its muted foreground so a
    // missing native icon never becomes an invisible black Lucide fallback
    // under a light OS palette. A sufficiently bright application text color
    // still follows live palette/theme changes.
    const qreal devicePixelRatio = pixmap.devicePixelRatio();
    QImage tinted = pixmap.toImage().convertToFormat(
        QImage::Format_ARGB32_Premultiplied);
    tintMask(tinted, fallbackTintColor());
    QPixmap result = QPixmap::fromImage(tinted);
    result.setDevicePixelRatio(devicePixelRatio);
    return result;
}

QString queryValue(const QUrlQuery &query, const QString &name)
{
    return query.queryItemValue(name, QUrl::FullyDecoded);
}
}

F4IconProvider::F4IconProvider(std::unique_ptr<F4IconProviderBackend> backend,
                               bool testPatternEnabled)
    : QQuickImageProvider(
          QQuickImageProvider::Image,
          QQmlImageProviderBase::ForceAsynchronousImageLoading)
    , m_backend(backend ? std::move(backend)
                        : std::make_unique<QtIconProviderBackend>())
    , m_testPatternEnabled(testPatternEnabled)
{
}

F4IconProvider::~F4IconProvider() = default;

QImage F4IconProvider::requestImage(const QString &id,
                                    QSize *size,
                                    const QSize &requestedSize)
{
    if (size) {
        *size = {};
    }

    const QUrl routeUrl = QUrl::fromEncoded(
        QByteArrayLiteral("f4icon://provider/") + id.toUtf8(),
        QUrl::StrictMode);
    if (!routeUrl.isValid()) {
        return {};
    }

    QString routePath = routeUrl.path();
    if (routePath.startsWith(u'/')) {
        routePath.remove(0, 1);
    }
    const qsizetype separator = routePath.indexOf(u'/');
    if (separator <= 0 || separator == routePath.size() - 1) {
        return {};
    }
    const QString route = routePath.left(separator);
    const QString encodedPrimary = routePath.mid(separator + 1);
    bool primaryOk = false;
    const QString primary = decodeRouteValue(encodedPrimary, &primaryOk);
    if (!primaryOk) {
        return {};
    }

    const QUrlQuery query(routeUrl);
    bool logicalOk = false;
    const int logicalSize = normalizedLogicalSize(
        queryValue(query, QStringLiteral("size")).toInt(&logicalOk));
    const qreal devicePixelRatio = normalizedDevicePixelRatio(
        queryValue(query, QStringLiteral("dpr")).toDouble());
    const int effectiveLogicalSize = logicalOk ? logicalSize : 16;
    QSize targetSize = requestedSize;
    if (targetSize.width() <= 0 && targetSize.height() > 0) {
        targetSize.setWidth(targetSize.height());
    } else if (targetSize.height() <= 0 && targetSize.width() > 0) {
        targetSize.setHeight(targetSize.width());
    }
    // QSize(0, 0) is valid according to QSize::isValid(), but it cannot back a
    // QImage paint device. Hidden/prewarmed QML Images can transiently request
    // exactly that size; render their logical fallback instead of flooding
    // stderr from QPainter::begin on a null image.
    if (targetSize.width() <= 0 || targetSize.height() <= 0) {
        targetSize = physicalSize(effectiveLogicalSize, devicePixelRatio);
    }

    if (m_testPatternEnabled) {
        targetSize.setWidth(std::max(1, targetSize.width()));
        targetSize.setHeight(std::max(1, targetSize.height()));

        // Diagnostic image for checking icon geometry and DPR handling: a
        // solid white box with an exact one-physical-pixel black border.
        QImage testImage(targetSize, QImage::Format_ARGB32_Premultiplied);
        testImage.fill(Qt::white);
        for (int x = 0; x < targetSize.width(); ++x) {
            testImage.setPixelColor(x, 0, Qt::black);
            testImage.setPixelColor(x, targetSize.height() - 1, Qt::black);
        }
        for (int y = 0; y < targetSize.height(); ++y) {
            testImage.setPixelColor(0, y, Qt::black);
            testImage.setPixelColor(targetSize.width() - 1, y, Qt::black);
        }
        testImage.setDevicePixelRatio(devicePixelRatio);
        if (size) {
            *size = QSize(effectiveLogicalSize, effectiveLogicalSize);
        }
        return testImage;
    }

    // requestedSize is the physical texture size Qt Quick needs on the
    // current screen. Derive the render scale from it so moving a window
    // between screens with different DPRs cannot leave a Gallery/model URL
    // tied to the primary screen's scale. The URL DPR remains the fallback
    // for direct callers and requests without an explicit source size.
    qreal renderDevicePixelRatio = devicePixelRatio;
    if (requestedSize.isValid() && effectiveLogicalSize > 0) {
        renderDevicePixelRatio = normalizedDevicePixelRatio(
            qreal(std::max(targetSize.width(), targetSize.height()))
            / qreal(effectiveLogicalSize));
    }

    // The default Lucide Gallery route is served from asynchronous Qt Quick
    // image-loader threads. QIcon's SVG engine renders through QPixmap, which
    // is GUI-thread-bound on Windows and produced null paint devices during
    // startup. Render the immutable resource directly into a QImage instead;
    // this is thread-safe and also avoids serializing generic file icons on
    // the native icon-provider mutex.
    if (route == QStringLiteral("lucide")) {
        const QString iconName = normalizedIconName(primary);
        const QColor requestedTint(
            queryValue(query, QStringLiteral("color")));
        QImage image = renderLucideImage(iconName, targetSize,
                                        effectiveLogicalSize,
                                        requestedTint.isValid()
                                            ? requestedTint
                                            : fallbackTintColor());
        if (size && !image.isNull()) {
            *size = image.size();
        }
        return image;
    }

    QString fallbackName;
    QIcon icon;
    {
        // Native theme engines and their caches are not uniformly documented
        // as thread-safe. QQuickImageProvider may call us from loader threads,
        // so serialize both lookup and QPixmap generation.
        std::lock_guard lock(m_mutex);
#ifdef Q_OS_WIN
        ensureComInitializedForCurrentThread();
#endif
        if (route == QStringLiteral("theme")) {
            fallbackName = normalizedIconName(primary);
            icon = m_backend->iconForName(primary);
        } else if (route == QStringLiteral("file")) {
            bool fileNameOk = false;
            const QString fileName = decodeRouteValue(
                queryValue(query, QStringLiteral("name")), &fileNameOk);
            if (!fileNameOk) {
                return {};
            }
            const bool directory = queryValue(query, QStringLiteral("dir"))
                                       == QStringLiteral("1");
            fallbackName = normalizedIconName(
                queryValue(query, QStringLiteral("fallback")));
            if (fallbackName == QStringLiteral("file")) {
                fallbackName = lucideFileIconName(fileName, directory);
            }
            icon = m_backend->iconForFile(primary, fileName, directory);
        } else {
            return {};
        }

        QPixmap pixmap;
        if (!icon.isNull()) {
            pixmap = icon.pixmap(QSize(effectiveLogicalSize,
                                       effectiveLogicalSize),
                                 renderDevicePixelRatio,
                                 QIcon::Normal,
                                 QIcon::Off);
        }
        if (pixmap.isNull()) {
            const QUrl fallback = lucideSource(fallbackName,
                                               effectiveLogicalSize);
            QIcon fallbackIcon(resourceFileName(fallback));
            pixmap = fallbackIcon.pixmap(
                QSize(effectiveLogicalSize, effectiveLogicalSize),
                renderDevicePixelRatio,
                QIcon::Normal,
                QIcon::Off);
            pixmap = paletteTintedMask(std::move(pixmap));
        }

        QImage image = imageAtPhysicalSize(pixmap, targetSize);
        if (size && !image.isNull()) {
            *size = image.size();
        }
        return image;
    }
}

QString F4IconProvider::encodeRouteValue(QStringView value)
{
    const QByteArray utf8 = value.toString().toUtf8();
    if (utf8.isEmpty()) {
        return QStringLiteral("-");
    }
    return QString::fromLatin1(utf8.toBase64(
        QByteArray::Base64UrlEncoding | QByteArray::OmitTrailingEquals));
}

QString F4IconProvider::decodeRouteValue(QStringView value, bool *ok)
{
    const QString encoded = value.toString();
    if (encoded == QStringLiteral("-")) {
        if (ok) {
            *ok = true;
        }
        return {};
    }

    bool valid = !encoded.isEmpty();
    for (const QChar character : encoded) {
        if (!character.isLetterOrNumber()
            && character != u'-' && character != u'_') {
            valid = false;
            break;
        }
    }
    if (encoded.size() % 4 == 1) {
        valid = false;
    }

    const QByteArray decodedBytes = valid
        ? QByteArray::fromBase64(encoded.toLatin1(),
                                 QByteArray::Base64UrlEncoding)
        : QByteArray();
    const QString decoded = QString::fromUtf8(decodedBytes);
    valid = valid
        && decoded.toUtf8() == decodedBytes
        && encodeRouteValue(decoded) == encoded;
    if (ok) {
        *ok = valid;
    }
    return valid ? decoded : QString();
}

QString F4IconProvider::routeId(const QUrl &url)
{
    const QString withoutPrefix = url.toString(
        QUrl::RemoveScheme | QUrl::RemoveAuthority);
    return withoutPrefix.startsWith(u'/')
        ? withoutPrefix.mid(1)
        : withoutPrefix;
}

int F4IconProvider::normalizedLogicalSize(int logicalSize)
{
    return std::clamp(logicalSize, MinLogicalIconSize, MaxLogicalIconSize);
}

qreal F4IconProvider::normalizedDevicePixelRatio(qreal devicePixelRatio)
{
    if (!std::isfinite(devicePixelRatio) || devicePixelRatio <= 0) {
        return 1.0;
    }
    return std::clamp(devicePixelRatio,
                      MinDevicePixelRatio,
                      MaxDevicePixelRatio);
}

QSize F4IconProvider::physicalSize(int logicalSize, qreal devicePixelRatio)
{
    const int normalizedSize = normalizedLogicalSize(logicalSize);
    const qreal normalizedRatio = normalizedDevicePixelRatio(devicePixelRatio);
    const int pixels = std::max(1, qRound(normalizedSize * normalizedRatio));
    return QSize(pixels, pixels);
}

QString F4IconProvider::normalizedIconName(QStringView rawName)
{
    QString name = rawName.toString().trimmed().toLower();
    if (name.endsWith(QStringLiteral(".svg"))) {
        name.chop(4);
    }
    name.replace(u'_', u'-');
    if (name == QStringLiteral("terminal")) {
        name = QStringLiteral("square-terminal");
    } else if (name == QStringLiteral("directory")) {
        name = QStringLiteral("folder");
    } else if (name == QStringLiteral("up")) {
        name = QStringLiteral("folder-up");
    } else if (name == QStringLiteral("code")) {
        name = QStringLiteral("file-code");
    } else if (name == QStringLiteral("text")) {
        name = QStringLiteral("file-text");
    }
    return lucideIconNames().contains(name) || isStreamlineIconName(name)
        ? name
        : QStringLiteral("file");
}

QString F4IconProvider::lucideFileIconName(QStringView rawFileName,
                                           bool directory)
{
    const QString fileName = rawFileName.toString();
    if (directory) {
        return fileName == QStringLiteral("..")
            ? QStringLiteral("folder-up")
            : QStringLiteral("folder");
    }

    const QString suffix = QFileInfo(fileName).suffix().toLower();
    static const QSet<QString> archiveExtensions{
        QStringLiteral("7z"), QStringLiteral("bz2"), QStringLiteral("cab"),
        QStringLiteral("cpio"), QStringLiteral("gz"), QStringLiteral("jar"),
        QStringLiteral("lha"), QStringLiteral("lzh"), QStringLiteral("rar"),
        QStringLiteral("rpm"), QStringLiteral("tar"), QStringLiteral("tgz"),
        QStringLiteral("xz"), QStringLiteral("zip"), QStringLiteral("zst"),
    };
    static const QSet<QString> imageExtensions{
        QStringLiteral("avif"), QStringLiteral("bmp"), QStringLiteral("dds"),
        QStringLiteral("gif"), QStringLiteral("heic"), QStringLiteral("heif"),
        QStringLiteral("ico"), QStringLiteral("jpeg"), QStringLiteral("jpg"),
        QStringLiteral("jxl"), QStringLiteral("png"), QStringLiteral("qoi"),
        QStringLiteral("svg"), QStringLiteral("tif"), QStringLiteral("tiff"),
        QStringLiteral("webp"),
    };
    static const QSet<QString> audioExtensions{
        QStringLiteral("aac"), QStringLiteral("aiff"), QStringLiteral("alac"),
        QStringLiteral("flac"), QStringLiteral("m4a"), QStringLiteral("mid"),
        QStringLiteral("midi"), QStringLiteral("mp3"), QStringLiteral("ogg"),
        QStringLiteral("opus"), QStringLiteral("wav"), QStringLiteral("wma"),
    };
    static const QSet<QString> videoExtensions{
        QStringLiteral("3gp"), QStringLiteral("avi"), QStringLiteral("flv"),
        QStringLiteral("m4v"), QStringLiteral("mkv"), QStringLiteral("mov"),
        QStringLiteral("mp4"), QStringLiteral("mpeg"), QStringLiteral("mpg"),
        QStringLiteral("ogv"), QStringLiteral("webm"), QStringLiteral("wmv"),
    };
    static const QSet<QString> codeExtensions{
        QStringLiteral("bash"), QStringLiteral("c"), QStringLiteral("cc"),
        QStringLiteral("cmake"), QStringLiteral("cpp"), QStringLiteral("cs"),
        QStringLiteral("css"), QStringLiteral("cxx"), QStringLiteral("go"),
        QStringLiteral("h"), QStringLiteral("hpp"), QStringLiteral("html"),
        QStringLiteral("java"), QStringLiteral("js"), QStringLiteral("jsx"),
        QStringLiteral("kt"), QStringLiteral("lua"), QStringLiteral("m"),
        QStringLiteral("mm"), QStringLiteral("php"), QStringLiteral("pl"),
        QStringLiteral("py"), QStringLiteral("qml"), QStringLiteral("rb"),
        QStringLiteral("rs"), QStringLiteral("sh"), QStringLiteral("swift"),
        QStringLiteral("ts"), QStringLiteral("tsx"), QStringLiteral("vue"),
        QStringLiteral("xml"), QStringLiteral("zig"), QStringLiteral("zsh"),
    };
    static const QSet<QString> textExtensions{
        QStringLiteral("csv"), QStringLiteral("diff"), QStringLiteral("ini"),
        QStringLiteral("log"), QStringLiteral("md"), QStringLiteral("nfo"),
        QStringLiteral("patch"), QStringLiteral("rst"), QStringLiteral("tex"),
        QStringLiteral("toml"), QStringLiteral("tsv"), QStringLiteral("txt"),
        QStringLiteral("yaml"), QStringLiteral("yml"),
    };
    static const QSet<QString> bookExtensions{
        QStringLiteral("azw"), QStringLiteral("azw3"), QStringLiteral("djvu"),
        QStringLiteral("epub"), QStringLiteral("fb2"), QStringLiteral("mobi"),
        QStringLiteral("pdf"),
    };
    static const QSet<QString> databaseExtensions{
        QStringLiteral("db"), QStringLiteral("db3"), QStringLiteral("mdb"),
        QStringLiteral("sqlite"), QStringLiteral("sqlite3"),
    };
    static const QSet<QString> configurationExtensions{
        QStringLiteral("cfg"), QStringLiteral("conf"), QStringLiteral("json"),
        QStringLiteral("plist"), QStringLiteral("properties"),
    };
    static const QSet<QString> lockedExtensions{
        QStringLiteral("asc"), QStringLiteral("gpg"), QStringLiteral("key"),
        QStringLiteral("pem"), QStringLiteral("pfx"), QStringLiteral("p12"),
    };
    static const QSet<QString> temporaryExtensions{
        QStringLiteral("bak"), QStringLiteral("old"), QStringLiteral("tmp"),
    };

    if (archiveExtensions.contains(suffix)) return QStringLiteral("archive");
    if (imageExtensions.contains(suffix)) return QStringLiteral("image");
    if (audioExtensions.contains(suffix)) return QStringLiteral("music");
    if (videoExtensions.contains(suffix)) return QStringLiteral("video");
    if (codeExtensions.contains(suffix)) return QStringLiteral("file-code");
    if (bookExtensions.contains(suffix)) return QStringLiteral("book-open");
    if (databaseExtensions.contains(suffix)) return QStringLiteral("database");
    if (configurationExtensions.contains(suffix)) return QStringLiteral("file-cog");
    if (lockedExtensions.contains(suffix)) return QStringLiteral("file-lock");
    if (temporaryExtensions.contains(suffix)) return QStringLiteral("file-clock");
    if (textExtensions.contains(suffix)) return QStringLiteral("file-text");
    return QStringLiteral("file");
}

QUrl F4IconProvider::lucideSource(QStringView rawName, int logicalSize)
{
    const QString iconName = normalizedIconName(rawName);
    if (isStreamlineIconName(iconName)) {
        return streamlineSource(iconName);
    }
    const QString group = normalizedLogicalSize(logicalSize)
            >= LargeLucideIconThreshold
        ? QStringLiteral("lucide-gallery")
        : QStringLiteral("lucide");
    return QUrl(QStringLiteral("qrc:/F4QtHost/icons/%1/%2.svg")
                    .arg(group, iconName));
}

F4IconSet::F4IconSet(QObject *parent)
    : F4IconSet(defaultProviderId(), parent)
{
}

F4IconSet::F4IconSet(QString providerId, QObject *parent)
    : QObject(parent)
    , m_providerId(normalizedProviderId(std::move(providerId)))
{
    if (qGuiApp) {
        qGuiApp->installEventFilter(this);
    }
}

QString F4IconSet::name() const
{
    return m_iconSet == System
        ? QStringLiteral("system")
        : QStringLiteral("lucide");
}

void F4IconSet::setIconSet(IconSet iconSet)
{
    if (iconSet != Lucide && iconSet != System) {
        iconSet = Lucide;
    }
    if (m_iconSet == iconSet) {
        return;
    }
    m_iconSet = iconSet;
    emit iconSetChanged();
    advanceRevision();
}

void F4IconSet::setName(const QString &setName)
{
    setIconSet(normalizedSetName(setName) == QStringLiteral("system")
                   ? System
                   : Lucide);
}

QUrl F4IconSet::iconSource(const QString &rawName,
                           int logicalSize,
                           qreal devicePixelRatio) const
{
    const QString iconName = F4IconProvider::normalizedIconName(rawName);
    // These two drive-menu symbols are deliberately bundled brand marks, not
    // names delegated to a platform icon theme.  Keep their source stable
    // when the user switches between the Lucide and System icon sets.
    if (isStreamlineIconName(iconName)) {
        return streamlineSource(iconName);
    }
    if (m_iconSet == Lucide) {
        return F4IconProvider::lucideSource(iconName, logicalSize);
    }
    return systemIconSource(QStringLiteral("theme"),
                            F4IconProvider::encodeRouteValue(iconName),
                            logicalSize,
                            devicePixelRatio,
                            0);
}

QUrl F4IconSet::rasterizedLucideSource(const QString &rawName,
                                       int logicalSize,
                                       qreal devicePixelRatio) const
{
    const QString iconName = F4IconProvider::normalizedIconName(rawName);
    return systemIconSource(QStringLiteral("lucide"),
                            F4IconProvider::encodeRouteValue(iconName),
                            logicalSize,
                            devicePixelRatio,
                            0);
}

QUrl F4IconSet::rasterizedLucideSource(const QString &rawName,
                                       int logicalSize,
                                       qreal devicePixelRatio,
                                       const QColor &tint) const
{
    if (!tint.isValid()) {
        return rasterizedLucideSource(rawName, logicalSize,
                                     devicePixelRatio);
    }

    const QString iconName = F4IconProvider::normalizedIconName(rawName);
    return systemIconSource(
        QStringLiteral("lucide"),
        F4IconProvider::encodeRouteValue(iconName),
        logicalSize,
        devicePixelRatio,
        0,
        {{QStringLiteral("color"), tint.name(QColor::HexArgb)}});
}

QUrl F4IconSet::fileIconSource(const QString &localPath,
                               const QString &fileName,
                               bool directory,
                               int logicalSize,
                               qreal devicePixelRatio,
                               qulonglong version) const
{
    const QString fallback = F4IconProvider::lucideFileIconName(fileName,
                                                                directory);
    if (m_iconSet == Lucide) {
        if (F4IconProvider::normalizedLogicalSize(logicalSize)
            >= LargeLucideIconThreshold) {
            return systemIconSource(
                QStringLiteral("lucide"),
                F4IconProvider::encodeRouteValue(fallback),
                logicalSize,
                devicePixelRatio,
                0);
        }
        return F4IconProvider::lucideSource(fallback, logicalSize);
    }
    return systemIconSource(
        QStringLiteral("file"),
        F4IconProvider::encodeRouteValue(localPath),
        logicalSize,
        devicePixelRatio,
        version,
        {{QStringLiteral("name"), F4IconProvider::encodeRouteValue(fileName)},
         {QStringLiteral("dir"), directory ? QStringLiteral("1")
                                            : QStringLiteral("0")},
         {QStringLiteral("fallback"), fallback}});
}

void F4IconSet::refresh()
{
    advanceRevision();
}

QString F4IconSet::defaultProviderId()
{
    return QStringLiteral("f4icons");
}

QString F4IconSet::normalizedSetName(QStringView rawName)
{
    return rawName.toString().trimmed().compare(
               QStringLiteral("system"), Qt::CaseInsensitive) == 0
        ? QStringLiteral("system")
        : QStringLiteral("lucide");
}

bool F4IconSet::eventFilter(QObject *watched, QEvent *event)
{
    if (event
        && (event->type() == QEvent::ThemeChange
            || (watched == qGuiApp
                && event->type() == QEvent::ApplicationPaletteChange))) {
        refresh();
    }
    return QObject::eventFilter(watched, event);
}

QUrl F4IconSet::systemIconSource(
    const QString &route,
    const QString &encodedValue,
    int logicalSize,
    qreal devicePixelRatio,
    qulonglong version,
    const QList<QPair<QString, QString>> &extraQuery) const
{
    QUrl url;
    url.setScheme(QStringLiteral("image"));
    url.setHost(m_providerId);
    url.setPath(QStringLiteral("/%1/%2").arg(route, encodedValue));

    QUrlQuery query;
    query.addQueryItem(QStringLiteral("size"), QString::number(
        F4IconProvider::normalizedLogicalSize(logicalSize)));
    query.addQueryItem(QStringLiteral("dpr"), QString::number(
        F4IconProvider::normalizedDevicePixelRatio(devicePixelRatio), 'g', 12));
    query.addQueryItem(QStringLiteral("revision"), QString::number(m_revision));
    if (version != 0) {
        query.addQueryItem(QStringLiteral("version"), QString::number(version));
    }
    for (const auto &[name, value] : extraQuery) {
        query.addQueryItem(name, value);
    }
    url.setQuery(query);
    return url;
}

void F4IconSet::advanceRevision()
{
    if (m_revision == std::numeric_limits<qulonglong>::max()) {
        m_revision = 1;
    } else {
        ++m_revision;
    }
    emit revisionChanged();
}
