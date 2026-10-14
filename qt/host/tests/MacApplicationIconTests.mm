#include "F4ApplicationIcon.h"

#include <QGuiApplication>
#include <QIcon>
#include <QPixmap>
#include <QColorSpace>
#include <QtTest>

#import <AppKit/AppKit.h>
#include <Accelerate/Accelerate.h>
#include <optional>

// Qt's exported conversion helper is used by both images and CoreText.
std::optional<vImage_CGImageFormat> qt_mac_cgImageFormatForImage(const QImage &image);

namespace
{
QString fromNSString(NSString *value)
{
    return value ? QString::fromUtf8([value UTF8String]) : QString();
}
}

class MacApplicationIconTests final : public QObject
{
    Q_OBJECT

private slots:
    void appBundleContainsPrebuiltIcon();
    void runtimeFallbackLeavesBundleIconAuthoritative();
    void nativeImageKeepsColorSpaceAlive();
    void nativeImageFormatKeepsColorSpaceAlive();
};

void MacApplicationIconTests::appBundleContainsPrebuiltIcon()
{
    NSString *bundlePath = [NSString stringWithUTF8String:F4_MACOS_APP_BUNDLE];
    NSBundle *bundle = [NSBundle bundleWithPath:bundlePath];
    QVERIFY2(bundle != nil, F4_MACOS_APP_BUNDLE);

    NSDictionary *info = [bundle infoDictionary];
    QCOMPARE(fromNSString([info objectForKey:@"CFBundleIconFile"]),
             QStringLiteral("AppIcon.icns"));
    QVERIFY([info objectForKey:@"CFBundleIconName"] == nil);

    NSFileManager *fileManager = [NSFileManager defaultManager];
    NSString *resourcesPath = [bundle resourcePath];
    for (NSString *fileName in @[@"AppIcon.icns"]) {
        NSString *path = [resourcesPath stringByAppendingPathComponent:fileName];
        BOOL isDirectory = NO;
        const bool exists =
            [fileManager fileExistsAtPath:path isDirectory:&isDirectory];
        const QByteArray failureMessage =
            QStringLiteral("missing %1").arg(fromNSString(path)).toUtf8();
        QVERIFY2(exists, failureMessage.constData());
        QVERIFY(!isDirectory);
        NSDictionary *attributes = [fileManager attributesOfItemAtPath:path
                                                                   error:nil];
        QVERIFY([[attributes objectForKey:NSFileSize] unsignedLongLongValue] > 0);
    }
}

void MacApplicationIconTests::runtimeFallbackLeavesBundleIconAuthoritative()
{
    QVERIFY(F4ApplicationIcon::isBundleManaged());

    QPixmap pixels(4, 4);
    pixels.fill(Qt::magenta);
    const QIcon sentinel(pixels);
    QGuiApplication::setWindowIcon(sentinel);
    const qint64 before = QGuiApplication::windowIcon().cacheKey();

    F4ApplicationIcon::installRuntimeFallback();

    QCOMPARE(QGuiApplication::windowIcon().cacheKey(), before);
}

void MacApplicationIconTests::nativeImageKeepsColorSpaceAlive()
{
    // Custom ICC spaces are not backed by CoreGraphics' shared sRGB cache.
    // Exercise the conversion used when Cocoa creates Qt's settings cursors.
    for (int i = 0; i < 100; ++i) {
        QImage image(32, 32, QImage::Format_ARGB32_Premultiplied);
        image.fill(Qt::white);
        image.setColorSpace(QColorSpace(QColorSpace::Primaries::SRgb, 1.8f + i * 0.001f));
        CGImageRef native = image.toCGImage();
        QVERIFY(native != nullptr);
        QCOMPARE(CGColorSpaceGetModel(CGImageGetColorSpace(native)), kCGColorSpaceModelRGB);
        CGImageRelease(native);
    }
    qInfo("[FIX:mac-color-space] 100 native image conversions retained valid RGB spaces");
}

void MacApplicationIconTests::nativeImageFormatKeepsColorSpaceAlive()
{
    QImage image(32, 32, QImage::Format_ARGB32_Premultiplied);
    image.fill(Qt::white);
    image.setColorSpace(QColorSpace(QColorSpace::Primaries::SRgb, 1.81f));
    auto format = qt_mac_cgImageFormatForImage(image);
    QVERIFY(format.has_value());
    // Evict CoreGraphics' cached ICC spaces; otherwise the cache masks a
    // prematurely released pointer until unrelated allocations occur.
    for (int i = 0; i < 10000; ++i) {
        const QByteArray profile = QColorSpace(QColorSpace::Primaries::SRgb,
                                               1.9f + i * 0.00001f).iccProfile();
        CFDataRef data = CFDataCreate(nullptr,
            reinterpret_cast<const UInt8 *>(profile.constData()), profile.size());
        CGColorSpaceRef space = CGColorSpaceCreateWithICCData(data);
        CGColorSpaceRelease(space);
        CFRelease(data);
    }
    QCOMPARE(CGColorSpaceGetModel(format->colorSpace), kCGColorSpaceModelRGB);
    // The fixed helper transfers one owning reference to its caller.
    CGColorSpaceRelease(format->colorSpace);
}

QTEST_MAIN(MacApplicationIconTests)
#include "MacApplicationIconTests.moc"
