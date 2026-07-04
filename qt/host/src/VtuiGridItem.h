#pragma once

#include <QFont>
#include <QImage>
#include <QVariantMap>
#include <QVector>
#include <QtQmlIntegration/qqmlintegration.h>
#include <QtQuick/QQuickItem>

class QtShellController;
class QInputMethodEvent;
class QKeyEvent;
class QMouseEvent;
class QWheelEvent;

class VtuiGridItem : public QQuickItem
{
    Q_OBJECT
    QML_ELEMENT
    Q_PROPERTY(QObject *controller READ controller WRITE setController NOTIFY controllerChanged)
    Q_PROPERTY(qreal cellWidth READ cellWidth NOTIFY metricsChanged)
    Q_PROPERTY(qreal cellHeight READ cellHeight NOTIFY metricsChanged)

public:
    explicit VtuiGridItem(QQuickItem *parent = nullptr);

    QObject *controller() const { return m_controllerObject; }
    void setController(QObject *controller);

    qreal cellWidth() const { return m_cellWidth; }
    qreal cellHeight() const { return m_cellHeight; }

signals:
    void controllerChanged();
    void metricsChanged();

protected:
    QSGNode *updatePaintNode(QSGNode *oldNode, UpdatePaintNodeData *) override;
    void geometryChange(const QRectF &newGeometry, const QRectF &oldGeometry) override;
    void keyPressEvent(QKeyEvent *event) override;
    void keyReleaseEvent(QKeyEvent *event) override;
    void inputMethodEvent(QInputMethodEvent *event) override;
    QVariant inputMethodQuery(Qt::InputMethodQuery query) const override;
    void mousePressEvent(QMouseEvent *event) override;
    void mouseReleaseEvent(QMouseEvent *event) override;
    void mouseMoveEvent(QMouseEvent *event) override;
    void wheelEvent(QWheelEvent *event) override;

private slots:
    void handleMessage(const QVariantMap &message);

private:
    struct Cell {
        quint64 ch = ' ';
        quint64 attr = 0;
    };

    void recalculateMetrics();
    void resizeGrid(int cols, int rows);
    void markDirty();
    qreal currentDevicePixelRatio() const;
    QImage renderImage() const;
    QColor foregroundColor(quint64 attr) const;
    QColor backgroundColor(quint64 attr) const;
    QColor indexedColor(int index) const;
    QPoint cellForPosition(const QPointF &position) const;
    int modifiersFromEvent(Qt::KeyboardModifiers modifiers) const;
    int buttonState(Qt::MouseButton button) const;
    int keyToVk(const QKeyEvent *event) const;
    int keyChar(const QKeyEvent *event) const;
    void sendMouseEvent(QMouseEvent *event, int flags, bool down);
    void maybeSendResize();

    QObject *m_controllerObject = nullptr;
    QtShellController *m_controller = nullptr;
    QVector<Cell> m_cells;
    QVector<quint32> m_palette;
    QFont m_font;
    qreal m_cellWidth = 10;
    qreal m_cellHeight = 20;
    qreal m_ascent = 15;
    int m_cols = 0;
    int m_rows = 0;
    int m_lastSentCols = 0;
    int m_lastSentRows = 0;
    int m_cursorX = 0;
    int m_cursorY = 0;
    int m_cursorShape = 0;
    bool m_cursorVisible = false;
    int m_pressedButtonState = 0;
    qreal m_lastDevicePixelRatio = 0;
    bool m_imageDirty = true;
};
