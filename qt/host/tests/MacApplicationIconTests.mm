#include "F4ApplicationIcon.h"

#include <QGuiApplication>
#include <QIcon>
#include <QPixmap>
#include <QtTest>

#import <AppKit/AppKit.h>

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

QTEST_MAIN(MacApplicationIconTests)
#include "MacApplicationIconTests.moc"
