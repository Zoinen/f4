#pragma once

#include <QFont>
#include <QHash>
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
    Q_PROPERTY(QString fontFamily READ fontFamily WRITE setFontFamily NOTIFY fontChanged)
    Q_PROPERTY(int fontPixelSize READ fontPixelSize WRITE setFontPixelSize NOTIFY fontChanged)
    Q_PROPERTY(qreal cellWidth READ cellWidth NOTIFY metricsChanged)
    Q_PROPERTY(qreal cellHeight READ cellHeight NOTIFY metricsChanged)
    Q_PROPERTY(bool pointerInputEnabled READ pointerInputEnabled
               WRITE setPointerInputEnabled NOTIFY pointerInputEnabledChanged)
    Q_PROPERTY(bool inputMethodForwardingEnabled READ inputMethodForwardingEnabled
               WRITE setInputMethodForwardingEnabled
               NOTIFY inputMethodForwardingEnabledChanged)
    Q_PROPERTY(bool terminalInputEnabled READ terminalInputEnabled
               WRITE setTerminalInputEnabled NOTIFY terminalInputEnabledChanged)

public:
    explicit VtuiGridItem(QQuickItem *parent = nullptr);

    QObject *controller() const { return m_controllerObject; }
    void setController(QObject *controller);
    QString fontFamily() const { return m_font.family(); }
    void setFontFamily(const QString &family);
    int fontPixelSize() const { return m_font.pixelSize(); }
    void setFontPixelSize(int size);

    qreal cellWidth() const { return m_cellWidth; }
    qreal cellHeight() const { return m_cellHeight; }
    bool pointerInputEnabled() const { return m_pointerInputEnabled; }
    void setPointerInputEnabled(bool enabled);
    bool inputMethodForwardingEnabled() const { return m_inputMethodForwardingEnabled; }
    void setInputMethodForwardingEnabled(bool enabled);
    bool terminalInputEnabled() const { return m_terminalInputEnabled; }
    void setTerminalInputEnabled(bool enabled);

    // Forward commander-owned shortcuts without moving focus back to the
    // hidden compatibility grid used by semantic QML surfaces.
    Q_INVOKABLE void sendQtKey(int key,
                               const QString &text,
                               bool down,
                               int modifiers,
                               quint32 nativeScanCode = 0);
    // Semantic QML surfaces do not give this item keyboard focus, so their
    // standard paste shortcuts must explicitly use the same clipboard path
    // as keyPressEvent instead of sending a literal Ctrl/Cmd+V to Go.
    Q_INVOKABLE void sendClipboardPaste();
    Q_INVOKABLE void sendQtText(const QString &text);

signals:
    void controllerChanged();
    void fontChanged();
    void metricsChanged();
    void pointerInputEnabledChanged();
    void inputMethodForwardingEnabledChanged();
    void terminalInputEnabledChanged();
    // Emitted synchronously whenever text has actually been sent through the
    // terminal protocol. Semantic surfaces use this to bridge the short gap
    // before the next authoritative scene reflects command/fast-find input.
    void commanderTextInputForwarded(const QString &text, int modifiers);
    // Drives native caret blink timing independently of scene round trips.
    // Auto-repeat emits this for every accepted key press.
    void keyboardActivity();

protected:
    bool eventFilter(QObject *watched, QEvent *event) override;
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
    bool isTouchpadScroll(const QWheelEvent *event) const;
    bool forwardKeyToController(int vk, int ch, bool down, int mods,
                                bool repeat = false);
    void releaseForwardedKeys();
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
    int m_wheelRemainder = 0;
    int m_cursorX = 0;
    int m_cursorY = 0;
    int m_cursorShape = 0;
    bool m_cursorVisible = false;
    int m_pressedButtonState = 0;
    qreal m_lastDevicePixelRatio = 0;
    bool m_imageDirty = true;
    bool m_pointerInputEnabled = true;
    bool m_inputMethodForwardingEnabled = false;
    bool m_terminalInputEnabled = true;
    quint32 m_lastNativeAltScanCode = 0;
    QHash<int, int> m_forwardedKeyModifiers;
};
