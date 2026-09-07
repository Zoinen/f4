#include "F4ApplicationIcon.h"

#include <QFile>
#include <QGuiApplication>
#include <QIcon>
#include <QScreen>
#include <QDir>
#include <QWindow>
#include <QtTest>
#include <QtEndian>
#include <qt_windows.h>

#include <memory>

class F4WindowsApplicationIconTests final : public QObject
{
    Q_OBJECT
private slots:
    void nativeTaskbarUsesGoIconSize()
    {
        if (QGuiApplication::platformName() != QStringLiteral("windows"))
            QSKIP("Requires a native Windows desktop");
        F4ApplicationIcon::installRuntimeFallback();
        QWindow window;
        // Exercise the mandatory fractional monitor when available; Qt's
        // offscreen scale factor is not the HWND's actual Windows DPI.
        for (QScreen *screen : QGuiApplication::screens()) {
            if (qFuzzyCompare(screen->devicePixelRatio(), 1.75)) {
                window.setScreen(screen);
                window.setPosition(screen->geometry().topLeft() + QPoint(100, 100));
                break;
            }
        }
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const HWND hwnd = reinterpret_cast<HWND>(window.winId());
        const UINT dpi = GetDpiForWindow(hwnd);
        const HICON icon = reinterpret_cast<HICON>(SendMessageW(hwnd, WM_GETICON, ICON_BIG, 0));
        ICONINFO info{};
        QVERIFY(GetIconInfo(icon, &info));
        BITMAP bitmap{};
        GetObjectW(info.hbmColor, sizeof(bitmap), &bitmap);
        DeleteObject(info.hbmColor);
        DeleteObject(info.hbmMask);
        qInfo() << "Native DPI" << dpi << "taskbar icon" << bitmap.bmWidth << bitmap.bmHeight;
        QCOMPARE(bitmap.bmWidth, MulDiv(24, dpi, 96));
        QCOMPARE(bitmap.bmHeight, MulDiv(24, dpi, 96));
        const QImage rendered = QImage::fromHICON(icon).convertToFormat(QImage::Format_ARGB32_Premultiplied);
        const QIcon prepared(QString::fromUtf8(F4_ICON_SOURCE));
        const QImage expected = prepared.pixmap(QSize(bitmap.bmWidth, bitmap.bmHeight), 1.0)
                                    .toImage().convertToFormat(QImage::Format_ARGB32_Premultiplied);
        const QString captureDir = qEnvironmentVariable("F4_ICON_CAPTURE_DIR");
        if (!captureDir.isEmpty()) {
            QVERIFY(QDir().mkpath(captureDir));
            QVERIFY(rendered.save(captureDir + QStringLiteral("/native-taskbar.png")));
            QVERIFY(expected.save(captureDir + QStringLiteral("/prepared-taskbar.png")));
        }
        QCOMPARE(rendered.size(), expected.size());
        // Windows and Qt round alpha premultiplication differently by at
        // most one channel value. No spatial resampling is permitted.
        for (int y = 0; y < rendered.height(); ++y) {
            for (int x = 0; x < rendered.width(); ++x) {
                const QRgb actual = rendered.pixel(x, y);
                const QRgb reference = expected.pixel(x, y);
                QCOMPARE(qAlpha(actual), qAlpha(reference));
                QVERIFY(qAbs(qRed(actual) - qRed(reference)) <= 1);
                QVERIFY(qAbs(qGreen(actual) - qGreen(reference)) <= 1);
                QVERIFY(qAbs(qBlue(actual) - qBlue(reference)) <= 1);
            }
        }

        // Qt overwrites WM_SETICON when the application icon changes; our
        // event handler must restore the prepared native frame afterwards.
        window.setIcon(prepared);
        QTRY_COMPARE(QImage::fromHICON(reinterpret_cast<HICON>(
                         SendMessageW(hwnd, WM_GETICON, ICON_BIG, 0)))
                         .convertToFormat(QImage::Format_ARGB32_Premultiplied), rendered);
        window.destroy();
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        const HWND recreated = reinterpret_cast<HWND>(window.winId());
        QTRY_COMPARE(QImage::fromHICON(reinterpret_cast<HICON>(
                         SendMessageW(recreated, WM_GETICON, ICON_BIG, 0)))
                         .convertToFormat(QImage::Format_ARGB32_Premultiplied), rendered);
    }

    void executableContainsEveryPreparedFrame()
    {
        QFile source(QString::fromUtf8(F4_ICON_SOURCE));
        QVERIFY(source.open(QIODevice::ReadOnly));
        const QByteArray ico = source.readAll();
        const QString host = QString::fromUtf8(F4_ICON_HOST);
        const auto releaseModule = [](HMODULE module) { FreeLibrary(module); };
        const std::unique_ptr<std::remove_pointer_t<HMODULE>, decltype(releaseModule)> module(
            LoadLibraryExW(reinterpret_cast<LPCWSTR>(host.utf16()), nullptr,
                           LOAD_LIBRARY_AS_DATAFILE | LOAD_LIBRARY_AS_IMAGE_RESOURCE),
            releaseModule);
        QVERIFY(module);
        const auto resourceBytes = [&](int type, int id) {
            const HRSRC resource = FindResourceW(module.get(), MAKEINTRESOURCEW(id),
                                                 MAKEINTRESOURCEW(type));
            if (!resource)
                return QByteArray();
            const HGLOBAL data = LoadResource(module.get(), resource);
            return QByteArray(static_cast<const char *>(LockResource(data)),
                              SizeofResource(module.get(), resource));
        };
        const QByteArray group = resourceBytes(14, 101); // RT_GROUP_ICON
        QVERIFY(group.size() >= 6);
        QCOMPARE(group.first(6), ico.first(6));
        const auto count = qFromLittleEndian<quint16>(ico.constData() + 4);
        QCOMPARE(group.size(), 6 + count * 14);
        for (int index = 0; index < count; ++index) {
            const int icoOffset = 6 + index * 16;
            const int groupOffset = 6 + index * 14;
            QCOMPARE(group.mid(groupOffset, 12), ico.mid(icoOffset, 12));
            const auto id = qFromLittleEndian<quint16>(group.constData() + groupOffset + 12);
            const auto length = qFromLittleEndian<quint32>(ico.constData() + icoOffset + 8);
            const auto offset = qFromLittleEndian<quint32>(ico.constData() + icoOffset + 12);
            QCOMPARE(resourceBytes(3, id), ico.mid(offset, length)); // RT_ICON
        }
    }

    void preparedIconSurvivesRuntimePackaging()
    {
        QFile source(QString::fromUtf8(F4_ICON_SOURCE));
        QVERIFY(source.open(QIODevice::ReadOnly));
        QFile resource(QStringLiteral(":/F4QtHost/icons/app/f4.ico"));
        QVERIFY2(resource.open(QIODevice::ReadOnly),
                 "The production runtime ICO must exist at the window-icon URL");
        QCOMPARE(resource.readAll(), source.readAll());

        F4ApplicationIcon::installRuntimeFallback();
        const QIcon icon = QGuiApplication::windowIcon();
        QVERIFY(!icon.isNull());
        const QIcon original(QString::fromUtf8(F4_ICON_SOURCE));
        const QList<int> sizes{16, 24, 28, 30, 32, 36, 42, 48, 56, 64, 128, 256};
        QCOMPARE(icon.availableSizes().size(), sizes.size());
        for (const int size : sizes) {
            QVERIFY(icon.availableSizes().contains(QSize(size, size)));
            QCOMPARE(icon.pixmap(QSize(size, size), 1.0).toImage(),
                     original.pixmap(QSize(size, size), 1.0).toImage());
        }
        // Windows uses 28/56 physical pixels for 16/32 DIP icons at 175%.
        for (const int dip : {16, 32}) {
            const QPixmap pixmap = icon.pixmap(QSize(dip, dip), 1.75);
            QCOMPARE(pixmap.size(), QSize(dip * 7 / 4, dip * 7 / 4));
            QCOMPARE(pixmap.toImage(),
                     original.pixmap(QSize(dip, dip), 1.75).toImage());
        }
        QWindow window;
        window.show();
        QCoreApplication::processEvents();
        QCOMPARE(window.icon().cacheKey(), icon.cacheKey());
    }
};

QTEST_MAIN(F4WindowsApplicationIconTests)
#include "F4WindowsApplicationIconTests.moc"
