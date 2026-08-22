#include "F4IconProvider.h"

#include <QAbstractFileIconProvider>
#include <QCoreApplication>
#include <QDir>
#include <QEvent>
#include <QFileInfo>
#include <QGuiApplication>
#include <QIconEngine>
#include <QPainter>
#include <QPalette>
#include <QScreen>
#include <QSignalSpy>
#include <QUrlQuery>
#include <QtTest>

#include <cmath>
#include <limits>
#include <memory>

namespace
{
struct BackendRecord {
    QString requestedName;
    QString localPath;
    QString fileName;
    bool directory = false;
    int namedRequests = 0;
    int fileRequests = 0;
    QSize renderedLogicalSize;
    qreal renderedDevicePixelRatio = 0;
    int scaledPixmapRequests = 0;
};

class RecordingIconEngine final : public QIconEngine
{
public:
    explicit RecordingIconEngine(std::shared_ptr<BackendRecord> record)
        : m_record(std::move(record))
    {
    }

    RecordingIconEngine *clone() const override
    {
        return new RecordingIconEngine(m_record);
    }

    void paint(QPainter *painter,
               const QRect &rect,
               QIcon::Mode,
               QIcon::State) override
    {
        if (!painter) {
            return;
        }
        painter->fillRect(rect, QColor(24, 110, 180));
    }

    QPixmap pixmap(const QSize &size,
                   QIcon::Mode,
                   QIcon::State) override
    {
        return makePixmap(size, 1.0);
    }

    QPixmap scaledPixmap(const QSize &size,
                         QIcon::Mode,
                         QIcon::State,
                         qreal scale) override
    {
        m_record->renderedLogicalSize = size;
        m_record->renderedDevicePixelRatio = scale;
        ++m_record->scaledPixmapRequests;
        return makePixmap(size, scale);
    }

private:
    static QSize physicalSize(const QSize &logicalSize, qreal scale)
    {
        return QSize(std::max(1, qRound(logicalSize.width() * scale)),
                     std::max(1, qRound(logicalSize.height() * scale)));
    }

    static QPixmap makePixmap(const QSize &logicalSize, qreal scale)
    {
        QPixmap pixmap(physicalSize(logicalSize, scale));
        pixmap.fill(Qt::transparent);
        pixmap.setDevicePixelRatio(scale);

        // Deliberately multicolor: a provider regression that recolors native
        // icons as masks is caught without relying on any installed theme.
        QPainter painter(&pixmap);
        const QRect left(0, 0, pixmap.width() / 2, pixmap.height());
        const QRect right(left.right() + 1, 0,
                          pixmap.width() - left.width(), pixmap.height());
        painter.fillRect(left, QColor(220, 42, 52));
        painter.fillRect(right, QColor(35, 112, 214));
        return pixmap;
    }

    std::shared_ptr<BackendRecord> m_record;
};

class RecordingBackend final : public F4IconProviderBackend
{
public:
    explicit RecordingBackend(std::shared_ptr<BackendRecord> record)
        : m_record(std::move(record))
    {
    }

    QIcon iconForName(const QString &name) const override
    {
        m_record->requestedName = name;
        ++m_record->namedRequests;
        return QIcon(new RecordingIconEngine(m_record));
    }

    QIcon iconForFile(const QString &localPath,
                      const QString &fileName,
                      bool directory) const override
    {
        m_record->localPath = localPath;
        m_record->fileName = fileName;
        m_record->directory = directory;
        ++m_record->fileRequests;
        return QIcon(new RecordingIconEngine(m_record));
    }

private:
    std::shared_ptr<BackendRecord> m_record;
};

class NullBackend final : public F4IconProviderBackend
{
public:
    QIcon iconForName(const QString &) const override { return {}; }
    QIcon iconForFile(const QString &, const QString &, bool) const override
    {
        return {};
    }
};
}

class F4IconProviderTests final : public QObject
{
    Q_OBJECT

private slots:
    void requestsAlwaysRunOffTheGuiThread();
    void routeValuesRoundTripWithoutUrlAmbiguity();
    void routeValuesRejectNonCanonicalInput();
    void normalizationIsDeterministic();
    void lucideSourcesAndFileClassification();
    void largeLucideRouteRendersAVisibleDprAwareFallback();
    void iconSetPropertiesAndRevision();
    void systemFileRoutePreservesMetadataAndColor();
    void systemNamedRoutePreservesName();
    void requestedPhysicalSizeOverridesStaleDevicePixelRatio();
    void nativePlatformFileIconSmoke();
    void renderingUsesLogicalSizeAndDevicePixelRatio_data();
    void renderingUsesLogicalSizeAndDevicePixelRatio();
};

void F4IconProviderTests::requestsAlwaysRunOffTheGuiThread()
{
    F4IconProvider provider(std::make_unique<NullBackend>());
    QVERIFY(provider.flags().testFlag(
        QQmlImageProviderBase::ForceAsynchronousImageLoading));
}

void F4IconProviderTests::routeValuesRoundTripWithoutUrlAmbiguity()
{
    const QStringList values{
        QString(),
        QStringLiteral("-"),
        QStringLiteral("/tmp/a file#with?delimiters&more.png"),
        QStringLiteral("C:\\Users\\A B\\иконка 🖥️.txt"),
        QStringLiteral("//server/share/folder/.."),
    };

    for (const QString &value : values) {
        const QString encoded = F4IconProvider::encodeRouteValue(value);
        QVERIFY2(!encoded.contains(u'/'), qPrintable(encoded));
        QVERIFY2(!encoded.contains(u'+'), qPrintable(encoded));
        QVERIFY2(!encoded.contains(u'='), qPrintable(encoded));

        bool ok = false;
        QCOMPARE(F4IconProvider::decodeRouteValue(encoded, &ok), value);
        QVERIFY(ok);
    }
}

void F4IconProviderTests::routeValuesRejectNonCanonicalInput()
{
    const QStringList invalid{
        QString(),
        QStringLiteral("a"),
        QStringLiteral("Zg=="),
        QStringLiteral("Zg%3D%3D"),
        QStringLiteral("not/a/token"),
        QStringLiteral("_w"), // invalid UTF-8
    };
    for (const QString &encoded : invalid) {
        bool ok = true;
        QCOMPARE(F4IconProvider::decodeRouteValue(encoded, &ok), QString());
        QVERIFY2(!ok, qPrintable(encoded));
    }
}

void F4IconProviderTests::normalizationIsDeterministic()
{
    QCOMPARE(F4IconProvider::normalizedLogicalSize(-20), 1);
    QCOMPARE(F4IconProvider::normalizedLogicalSize(24), 24);
    QCOMPARE(F4IconProvider::normalizedLogicalSize(9000), 1024);

    QCOMPARE(F4IconProvider::normalizedDevicePixelRatio(0), 1.0);
    QCOMPARE(F4IconProvider::normalizedDevicePixelRatio(
                 std::numeric_limits<qreal>::quiet_NaN()), 1.0);
    QCOMPARE(F4IconProvider::normalizedDevicePixelRatio(0.25), 0.5);
    QCOMPARE(F4IconProvider::normalizedDevicePixelRatio(1.25), 1.25);
    QCOMPARE(F4IconProvider::normalizedDevicePixelRatio(99), 8.0);
    QCOMPARE(F4IconProvider::physicalSize(17, 1.25), QSize(21, 21));

    QCOMPARE(F4IconProvider::normalizedIconName(u" terminal.svg "),
             QStringLiteral("square-terminal"));
    QCOMPARE(F4IconProvider::normalizedIconName(u"FOLDER_KANBAN"),
             QStringLiteral("folder-kanban"));
    QCOMPARE(F4IconProvider::normalizedIconName(u"not-an-icon"),
             QStringLiteral("file"));

    QCOMPARE(F4IconSet::normalizedSetName(u" SYSTEM "),
             QStringLiteral("system"));
    QCOMPARE(F4IconSet::normalizedSetName(u"native"),
             QStringLiteral("lucide"));

    F4IconSet validProvider(QStringLiteral(" My_Icons "));
    QCOMPARE(validProvider.providerId(), QStringLiteral("my_icons"));
    F4IconSet invalidProvider(QStringLiteral("contains/a/slash"));
    QCOMPARE(invalidProvider.providerId(), F4IconSet::defaultProviderId());
}

void F4IconProviderTests::lucideSourcesAndFileClassification()
{
    QCOMPARE(F4IconProvider::lucideFileIconName(u"anything", true),
             QStringLiteral("folder"));
    QCOMPARE(F4IconProvider::lucideFileIconName(u"..", true),
             QStringLiteral("folder-up"));
    QCOMPARE(F4IconProvider::lucideFileIconName(u"photo.JPEG", false),
             QStringLiteral("image"));
    QCOMPARE(F4IconProvider::lucideFileIconName(u"source.QML", false),
             QStringLiteral("file-code"));
    QCOMPARE(F4IconProvider::lucideFileIconName(u"manual.pdf", false),
             QStringLiteral("book-open"));
    QCOMPARE(F4IconProvider::lucideFileIconName(u"unknown.bin", false),
             QStringLiteral("file"));

    QCOMPARE(F4IconProvider::lucideSource(u"folder", 16).toString(),
             QStringLiteral("qrc:/F4QtHost/icons/lucide/folder.svg"));
    QCOMPARE(F4IconProvider::lucideSource(u"folder", 128).toString(),
             QStringLiteral("qrc:/F4QtHost/icons/lucide-gallery/folder.svg"));

    F4IconSet icons;
    QCOMPARE(icons.iconSource(QStringLiteral("terminal"), 20, 2.0).toString(),
             QStringLiteral("qrc:/F4QtHost/icons/lucide/square-terminal.svg"));
    const QUrl gallerySource = icons.fileIconSource(
        QString(), QStringLiteral("a.zip"), false, 64, 2.0);
    QCOMPARE(gallerySource.scheme(), QStringLiteral("image"));
    QCOMPARE(gallerySource.host(), F4IconSet::defaultProviderId());
    QVERIFY(gallerySource.path().startsWith(QStringLiteral("/lucide/")));
    // A Lucide category icon depends only on its route, size, DPR and palette
    // revision. Per-file mtimes used to make every identical folder icon a
    // unique texture and defeated the Qt Quick image cache.
    const QUrl versionedGallerySource = icons.fileIconSource(
        QStringLiteral("/tmp/folder"), QStringLiteral("folder"), true,
        128, 2.0, 123456);
    const QUrlQuery galleryQuery(versionedGallerySource);
    QVERIFY(!galleryQuery.hasQueryItem(QStringLiteral("version")));
}

void F4IconProviderTests::largeLucideRouteRendersAVisibleDprAwareFallback()
{
    F4IconProvider provider(std::make_unique<NullBackend>());
    F4IconSet icons(QStringLiteral("test-icons"));
    const QUrl source = icons.fileIconSource(
        QString(), QStringLiteral("archive.zip"), false, 128, 2.0);

    const QImage image = provider.requestImage(
        F4IconProvider::routeId(source), nullptr, {});
    QCOMPARE(image.size(), QSize(256, 256));

    QColor stroke;
    bool foundStroke = false;
    for (int y = 0; y < image.height() && !foundStroke; ++y) {
        for (int x = 0; x < image.width(); ++x) {
            const QColor pixel = image.pixelColor(x, y);
            if (pixel.alpha() > 192) {
                stroke = pixel;
                foundStroke = true;
                break;
            }
        }
    }
    QVERIFY(foundStroke);
    QVERIFY2(qGray(stroke.rgb()) >= 90,
             qPrintable(QStringLiteral("fallback stroke was too dark: %1")
                            .arg(stroke.name(QColor::HexArgb))));
}

void F4IconProviderTests::iconSetPropertiesAndRevision()
{
    F4IconSet icons(QStringLiteral("test-icons"));
    QSignalSpy setSpy(&icons, &F4IconSet::iconSetChanged);
    QSignalSpy revisionSpy(&icons, &F4IconSet::revisionChanged);

    QCOMPARE(icons.iconSet(), F4IconSet::Lucide);
    QCOMPARE(icons.name(), QStringLiteral("lucide"));
    QVERIFY(!icons.system());
    QVERIFY(!icons.fileIconsAreFullColor());
    QCOMPARE(icons.revision(), qulonglong(1));

    icons.setName(QStringLiteral("SyStEm"));
    QCOMPARE(icons.iconSet(), F4IconSet::System);
    QCOMPARE(icons.name(), QStringLiteral("system"));
    QVERIFY(icons.system());
    QVERIFY(icons.fileIconsAreFullColor());
    QCOMPARE(icons.revision(), qulonglong(2));
    QCOMPARE(setSpy.count(), 1);
    QCOMPARE(revisionSpy.count(), 1);

    icons.setName(QStringLiteral("system"));
    QCOMPARE(setSpy.count(), 1);
    QCOMPARE(revisionSpy.count(), 1);

    icons.refresh();
    QCOMPARE(icons.revision(), qulonglong(3));
    QCOMPARE(revisionSpy.count(), 2);

    const qulonglong revisionBeforeThemeChange = icons.revision();
    QEvent themeChange(QEvent::ThemeChange);
    QCoreApplication::sendEvent(qGuiApp, &themeChange);
    QVERIFY(icons.revision() > revisionBeforeThemeChange);
    QVERIFY(revisionSpy.count() >= 3);

    const QUrl url = icons.fileIconSource(
        QStringLiteral("/tmp/archive.zip"), QStringLiteral("archive.zip"),
        false, 20, 1.25, 77);
    QCOMPARE(url.scheme(), QStringLiteral("image"));
    QCOMPARE(url.host(), QStringLiteral("test-icons"));
    const QUrlQuery query(url);
    QCOMPARE(query.queryItemValue(QStringLiteral("size")), QStringLiteral("20"));
    QCOMPARE(query.queryItemValue(QStringLiteral("dpr")), QStringLiteral("1.25"));
    QCOMPARE(query.queryItemValue(QStringLiteral("revision")),
             QString::number(icons.revision()));
    QCOMPARE(query.queryItemValue(QStringLiteral("version")), QStringLiteral("77"));
    QCOMPARE(query.queryItemValue(QStringLiteral("fallback")),
             QStringLiteral("archive"));
}

void F4IconProviderTests::systemFileRoutePreservesMetadataAndColor()
{
    const auto record = std::make_shared<BackendRecord>();
    F4IconProvider provider(std::make_unique<RecordingBackend>(record));
    F4IconSet icons(QStringLiteral("test-icons"));
    icons.setIconSet(F4IconSet::System);

    const QString path = QStringLiteral("/tmp/A file #?/фото.png");
    const QString name = QStringLiteral("фото #1.png");
    const QUrl source = icons.fileIconSource(path, name, false, 20, 1.25, 9);

    QSize reportedSize;
    const QImage image = provider.requestImage(
        F4IconProvider::routeId(source), &reportedSize, {});
    QVERIFY(!image.isNull());
    QCOMPARE(image.size(), QSize(25, 25));
    QCOMPARE(reportedSize, image.size());
    QCOMPARE(record->fileRequests, 1);
    QCOMPARE(record->namedRequests, 0);
    QCOMPARE(record->localPath, path);
    QCOMPARE(record->fileName, name);
    QVERIFY(!record->directory);

    // A true native icon must retain independent color channels.
    QCOMPARE(image.pixelColor(2, image.height() / 2), QColor(220, 42, 52));
    QCOMPARE(image.pixelColor(image.width() - 2, image.height() / 2),
             QColor(35, 112, 214));
}

void F4IconProviderTests::systemNamedRoutePreservesName()
{
    const auto record = std::make_shared<BackendRecord>();
    F4IconProvider provider(std::make_unique<RecordingBackend>(record));
    F4IconSet icons(QStringLiteral("test-icons"));
    icons.setIconSet(F4IconSet::System);

    const QUrl source = icons.iconSource(QStringLiteral("terminal.svg"), 18, 2);
    const QImage image = provider.requestImage(F4IconProvider::routeId(source),
                                               nullptr, {});
    QVERIFY(!image.isNull());
    QCOMPARE(record->namedRequests, 1);
    QCOMPARE(record->fileRequests, 0);
    QCOMPARE(record->requestedName, QStringLiteral("square-terminal"));
}

void F4IconProviderTests::requestedPhysicalSizeOverridesStaleDevicePixelRatio()
{
    const auto record = std::make_shared<BackendRecord>();
    F4IconProvider provider(std::make_unique<RecordingBackend>(record));
    F4IconSet icons(QStringLiteral("test-icons"));
    icons.setIconSet(F4IconSet::System);

    // The URL can have been created while the window was on a 1x screen.
    // A later 32px texture request for a 16-DIP icon must select the 2x
    // representation instead of scaling a stale 1x native pixmap.
    const QUrl source = icons.iconSource(QStringLiteral("folder"), 16, 1.0);
    const QImage image = provider.requestImage(
        F4IconProvider::routeId(source), nullptr, QSize(32, 32));

    QVERIFY(!image.isNull());
    QCOMPARE(image.size(), QSize(32, 32));
    QCOMPARE(record->renderedLogicalSize, QSize(16, 16));
    QCOMPARE(record->renderedDevicePixelRatio, qreal(2));
}

void F4IconProviderTests::nativePlatformFileIconSmoke()
{
    const QString platform = QGuiApplication::platformName();
    if (platform == QStringLiteral("offscreen")
        || platform == QStringLiteral("minimal")) {
        QSKIP("Native file icons require the real desktop QPA plugin");
    }

    const QString homePath = QDir::homePath();
    QAbstractFileIconProvider nativeSource;
    const QIcon nativeIcon = nativeSource.icon(QFileInfo(homePath));
    QVERIFY2(!nativeIcon.isNull(), qPrintable(
        QStringLiteral("%1 did not return a native home-folder icon")
            .arg(platform)));

    F4IconProvider provider;
    F4IconSet icons(QStringLiteral("test-icons"));
    icons.setIconSet(F4IconSet::System);
    const QUrl source = icons.fileIconSource(
        homePath, QFileInfo(homePath).fileName(), true, 64,
        QGuiApplication::primaryScreen()
            ? QGuiApplication::primaryScreen()->devicePixelRatio()
            : qreal(1));
    const QImage image = provider.requestImage(
        F4IconProvider::routeId(source), nullptr, {});
    QVERIFY(!image.isNull());
}

void F4IconProviderTests::renderingUsesLogicalSizeAndDevicePixelRatio_data()
{
    QTest::addColumn<int>("logicalSize");
    QTest::addColumn<qreal>("devicePixelRatio");
    QTest::addColumn<QSize>("expectedPhysicalSize");

    QTest::newRow("one-x") << 16 << qreal(1.0) << QSize(16, 16);
    QTest::newRow("fractional") << 17 << qreal(1.25) << QSize(21, 21);
    QTest::newRow("retina") << 20 << qreal(2.0) << QSize(40, 40);
    QTest::newRow("large-retina") << 128 << qreal(2.0) << QSize(256, 256);
}

void F4IconProviderTests::renderingUsesLogicalSizeAndDevicePixelRatio()
{
    QFETCH(int, logicalSize);
    QFETCH(qreal, devicePixelRatio);
    QFETCH(QSize, expectedPhysicalSize);

    const auto record = std::make_shared<BackendRecord>();
    F4IconProvider provider(std::make_unique<RecordingBackend>(record));
    F4IconSet icons(QStringLiteral("test-icons"));
    icons.setIconSet(F4IconSet::System);

    const QUrl source = icons.iconSource(QStringLiteral("folder"),
                                         logicalSize,
                                         devicePixelRatio);
    QSize reportedSize;
    const QImage image = provider.requestImage(
        F4IconProvider::routeId(source), &reportedSize, {});

    QVERIFY(!image.isNull());
    QCOMPARE(record->scaledPixmapRequests, 1);
    QCOMPARE(record->renderedLogicalSize, QSize(logicalSize, logicalSize));
    QVERIFY(qFuzzyCompare(record->renderedDevicePixelRatio,
                          devicePixelRatio));
    QCOMPARE(image.size(), expectedPhysicalSize);
    QCOMPARE(reportedSize, expectedPhysicalSize);
    QCOMPARE(image.devicePixelRatio(), 1.0);
}

QTEST_MAIN(F4IconProviderTests)

#include "F4IconProviderTests.moc"
