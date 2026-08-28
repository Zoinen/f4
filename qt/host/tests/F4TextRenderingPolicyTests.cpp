#include "F4TextRenderingPolicy.h"

#include <QGuiApplication>
#include <QQmlComponent>
#include <QQmlEngine>
#include <QQuickItem>
#include <QQuickWindow>
#include <QScopedPointer>
#include <QUrl>
#include <QtTest>

class F4TextRenderingPolicyTests final : public QObject
{
    Q_OBJECT

private slots:
    void defaultIsNative();
    void exposesEveryQtRenderType();
    void updatesExistingQmlTextItems();
    void rejectsUnknownRenderType();
};

void F4TextRenderingPolicyTests::defaultIsNative()
{
    F4TextRenderingPolicy policy;

    QCOMPARE(policy.renderType(),
             int(QQuickWindow::NativeTextRendering));
    QCOMPARE(int(QQuickWindow::textRenderType()),
             int(QQuickWindow::NativeTextRendering));
    QCOMPARE(policy.renderTypeName(), QStringLiteral("NativeRendering"));
}

void F4TextRenderingPolicyTests::exposesEveryQtRenderType()
{
    F4TextRenderingPolicy policy;
    const QVariantList options = policy.options();

#if QT_VERSION >= QT_VERSION_CHECK(6, 8, 0)
    QCOMPARE(options.size(), 3);
#else
    QCOMPARE(options.size(), 2);
#endif

    QVERIFY(policy.setRenderTypeByName(QStringLiteral("QtRendering")));
    QCOMPARE(policy.renderType(), int(QQuickWindow::QtTextRendering));
    QVERIFY(policy.setRenderTypeByName(QStringLiteral("NativeRendering")));
    QCOMPARE(policy.renderType(), int(QQuickWindow::NativeTextRendering));
#if QT_VERSION >= QT_VERSION_CHECK(6, 8, 0)
    QVERIFY(policy.setRenderTypeByName(QStringLiteral("CurveRendering")));
    QCOMPARE(policy.renderType(), int(QQuickWindow::CurveTextRendering));
#endif
}

void F4TextRenderingPolicyTests::updatesExistingQmlTextItems()
{
    F4TextRenderingPolicy policy;
    policy.setRenderType(int(QQuickWindow::NativeTextRendering));

    QQuickWindow window;
    QQmlEngine engine;
    QQmlComponent component(&engine);
    component.setData(
        "import QtQuick\n"
        "Item { Text { objectName: \"label\"; text: \"sample\" } }",
        QUrl(QStringLiteral("F4TextRenderingPolicyTest.qml")));
    QScopedPointer<QObject> object(component.create());
    QVERIFY2(!object.isNull(), qPrintable(component.errorString()));

    auto *rootItem = qobject_cast<QQuickItem *>(object.data());
    QVERIFY(rootItem);
    rootItem->setParentItem(window.contentItem());
    QObject *label = rootItem->findChild<QObject *>(QStringLiteral("label"));
    QVERIFY(label);
    QCOMPARE(label->property("renderType").toInt(),
             int(QQuickWindow::NativeTextRendering));

    policy.setRenderType(int(QQuickWindow::QtTextRendering));
    QCOMPARE(label->property("renderType").toInt(),
             int(QQuickWindow::QtTextRendering));

    policy.setRenderType(int(QQuickWindow::NativeTextRendering));
    QCOMPARE(label->property("renderType").toInt(),
             int(QQuickWindow::NativeTextRendering));
}

void F4TextRenderingPolicyTests::rejectsUnknownRenderType()
{
    F4TextRenderingPolicy policy;
    policy.setRenderType(int(QQuickWindow::NativeTextRendering));

    QVERIFY(!policy.setRenderTypeByName(QStringLiteral("unknown")));
    QCOMPARE(policy.renderType(),
             int(QQuickWindow::NativeTextRendering));
    policy.setRenderType(999);
    QCOMPARE(policy.renderType(),
             int(QQuickWindow::NativeTextRendering));
}

QTEST_MAIN(F4TextRenderingPolicyTests)
#include "F4TextRenderingPolicyTests.moc"
