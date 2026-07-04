#include "VtuiGridItem.h"

#include "QtShellController.h"

#include <QClipboard>
#include <QFontMetricsF>
#include <QGuiApplication>
#include <QInputMethodEvent>
#include <QKeyEvent>
#include <QKeySequence>
#include <QPainter>
#include <QQuickWindow>
#include <QSGSimpleTextureNode>
#include <QSGTexture>
#include <QWheelEvent>

#include <algorithm>
#include <cmath>

namespace
{
constexpr quint64 IsFgRGB = 0x0100;
constexpr quint64 IsBgRGB = 0x0200;
constexpr quint64 ForegroundIntensity = 0x0008;
constexpr quint64 BackgroundIntensity = 0x0080;
constexpr quint64 ForegroundDim = 0x1000;
constexpr quint64 CommonLvbStrikeout = 0x2000;
constexpr quint64 CommonLvbReverse = 0x4000;
constexpr quint64 CommonLvbUnderscore = 0x8000;
constexpr quint64 WideCharFiller = ~quint64(0);

constexpr int LeftAltPressed = 0x0002;
constexpr int LeftCtrlPressed = 0x0008;
constexpr int ShiftPressed = 0x0010;
constexpr int EnhancedKey = 0x0100;

constexpr int FromLeft1stButtonPressed = 0x0001;
constexpr int RightmostButtonPressed = 0x0002;
constexpr int FromLeft2ndButtonPressed = 0x0004;
constexpr int MouseMoved = 0x0001;

QVector<quint32> defaultPalette()
{
    return {
        0x000000, 0x800000, 0x008000, 0x808000, 0x000080, 0x800080, 0x008080, 0xc0c0c0,
        0x808080, 0xff0000, 0x00ff00, 0xffff00, 0x0000ff, 0xff00ff, 0x00ffff, 0xffffff,
    };
}

QColor colorFromRgb(quint32 rgb)
{
    return QColor(static_cast<int>((rgb >> 16) & 0xff),
                  static_cast<int>((rgb >> 8) & 0xff),
                  static_cast<int>(rgb & 0xff));
}

QString glyphForCell(quint64 ch)
{
    if (ch == 0 || ch == WideCharFiller) {
        return QString();
    }
    if (ch > 0x10ffff || (ch >= 0xd800 && ch <= 0xdfff)) {
        return QStringLiteral(" ");
    }
    char32_t codepoint = static_cast<char32_t>(ch);
    return QString::fromUcs4(&codepoint, 1);
}

bool isEnhancedQtKey(int key)
{
    switch (key) {
    case Qt::Key_Left:
    case Qt::Key_Up:
    case Qt::Key_Right:
    case Qt::Key_Down:
    case Qt::Key_Insert:
    case Qt::Key_Delete:
    case Qt::Key_Home:
    case Qt::Key_End:
    case Qt::Key_PageUp:
    case Qt::Key_PageDown:
        return true;
    default:
        return false;
    }
}
}

VtuiGridItem::VtuiGridItem(QQuickItem *parent)
    : QQuickItem(parent)
    , m_palette(defaultPalette())
{
    setFlag(ItemHasContents, true);
    setFlag(ItemAcceptsInputMethod, true);
    setAcceptedMouseButtons(Qt::AllButtons);
    setAcceptHoverEvents(true);
    setFocus(true);

    m_font.setStyleHint(QFont::Monospace);
    m_font.setFamilies({QStringLiteral("Menlo"),
                        QStringLiteral("Consolas"),
                        QStringLiteral("DejaVu Sans Mono"),
                        QStringLiteral("monospace")});
    m_font.setPixelSize(18);
    recalculateMetrics();
}

void VtuiGridItem::setController(QObject *controller)
{
    if (m_controllerObject == controller) {
        return;
    }

    if (m_controller) {
        disconnect(m_controller, nullptr, this, nullptr);
    }

    m_controllerObject = controller;
    m_controller = qobject_cast<QtShellController *>(controller);
    if (m_controller) {
        connect(m_controller, &QtShellController::messageReceived, this, &VtuiGridItem::handleMessage);
    }

    emit controllerChanged();
}

QSGNode *VtuiGridItem::updatePaintNode(QSGNode *oldNode, UpdatePaintNodeData *)
{
    auto *node = static_cast<QSGSimpleTextureNode *>(oldNode);
    if (!node) {
        node = new QSGSimpleTextureNode;
    }

    if (!window()) {
        return node;
    }

    const qreal devicePixelRatio = currentDevicePixelRatio();
    if (m_imageDirty || node->rect() != boundingRect() || !qFuzzyCompare(m_lastDevicePixelRatio, devicePixelRatio)) {
        if (QSGTexture *oldTexture = node->texture()) {
            node->setOwnsTexture(false);
            delete oldTexture;
        }

        QImage image = renderImage();
        QSGTexture *texture = window()->createTextureFromImage(image);
        node->setTexture(texture);
        node->setOwnsTexture(true);
        node->setRect(boundingRect());
        m_lastDevicePixelRatio = devicePixelRatio;
        m_imageDirty = false;
    }

    return node;
}

void VtuiGridItem::geometryChange(const QRectF &newGeometry, const QRectF &oldGeometry)
{
    QQuickItem::geometryChange(newGeometry, oldGeometry);
    if (newGeometry.size() != oldGeometry.size()) {
        maybeSendResize();
        markDirty();
    }
}

void VtuiGridItem::keyPressEvent(QKeyEvent *event)
{
    if (event->matches(QKeySequence::Paste)) {
        if (QClipboard *clipboard = QGuiApplication::clipboard()) {
            if (m_controller) {
                m_controller->sendPaste(clipboard->text());
            }
        }
        event->accept();
        return;
    }

    if (m_controller) {
        int mods = modifiersFromEvent(event->modifiers());
        if (isEnhancedQtKey(event->key())) {
            mods |= EnhancedKey;
        }
        m_controller->sendKey(keyToVk(event), keyChar(event), true, mods);
    }
    event->accept();
}

void VtuiGridItem::keyReleaseEvent(QKeyEvent *event)
{
    if (m_controller) {
        int mods = modifiersFromEvent(event->modifiers());
        if (isEnhancedQtKey(event->key())) {
            mods |= EnhancedKey;
        }
        m_controller->sendKey(keyToVk(event), 0, false, mods);
    }
    event->accept();
}

void VtuiGridItem::inputMethodEvent(QInputMethodEvent *event)
{
    if (m_controller && !event->commitString().isEmpty()) {
        m_controller->sendText(event->commitString());
    }
    event->accept();
}

QVariant VtuiGridItem::inputMethodQuery(Qt::InputMethodQuery query) const
{
    if (query == Qt::ImCursorRectangle) {
        return QRectF(m_cursorX * m_cellWidth, m_cursorY * m_cellHeight, m_cellWidth, m_cellHeight);
    }
    return QQuickItem::inputMethodQuery(query);
}

void VtuiGridItem::mousePressEvent(QMouseEvent *event)
{
    m_pressedButtonState |= buttonState(event->button());
    sendMouseEvent(event, 0, true);
    forceActiveFocus();
    event->accept();
}

void VtuiGridItem::mouseReleaseEvent(QMouseEvent *event)
{
    sendMouseEvent(event, 0, false);
    m_pressedButtonState &= ~buttonState(event->button());
    event->accept();
}

void VtuiGridItem::mouseMoveEvent(QMouseEvent *event)
{
    sendMouseEvent(event, MouseMoved, event->buttons() != Qt::NoButton);
    event->accept();
}

void VtuiGridItem::wheelEvent(QWheelEvent *event)
{
    if (!m_controller) {
        return;
    }

    const int delta = event->angleDelta().y();
    if (delta == 0) {
        return;
    }

    const QPoint cell = cellForPosition(event->position());
    m_controller->sendWheel(cell.x(), cell.y(), delta > 0 ? 1 : -1, modifiersFromEvent(event->modifiers()));
    event->accept();
}

void VtuiGridItem::handleMessage(const QVariantMap &message)
{
    const QString type = message.value(QStringLiteral("type")).toString();

    if (type == QStringLiteral("palette")) {
        const QVariantList colors = message.value(QStringLiteral("colors")).toList();
        if (!colors.isEmpty()) {
            m_palette.resize(colors.size());
            for (qsizetype i = 0; i < colors.size(); ++i) {
                m_palette[i] = colors[i].toUInt();
            }
            markDirty();
        }
        return;
    }

    if (type == QStringLiteral("frame")) {
        const int cols = message.value(QStringLiteral("width")).toInt();
        const int rows = message.value(QStringLiteral("height")).toInt();
        if (cols > 0 && rows > 0 && (cols != m_cols || rows != m_rows)) {
            resizeGrid(cols, rows);
        }

        if (message.value(QStringLiteral("full")).toBool() && m_cols > 0 && m_rows > 0) {
            std::fill(m_cells.begin(), m_cells.end(), Cell{});
        }

        const QVariantList cells = message.value(QStringLiteral("cells")).toList();
        for (const QVariant &entryVariant : cells) {
            const QVariantList entry = entryVariant.toList();
            if (entry.size() < 3) {
                continue;
            }
            const int index = entry[0].toInt();
            if (index < 0 || index >= m_cells.size()) {
                continue;
            }
            m_cells[index].ch = entry[1].toULongLong();
            m_cells[index].attr = entry[2].toULongLong();
        }
        markDirty();
        return;
    }

    if (type == QStringLiteral("cursor")) {
        m_cursorX = message.value(QStringLiteral("x")).toInt();
        m_cursorY = message.value(QStringLiteral("y")).toInt();
        m_cursorVisible = message.value(QStringLiteral("visible")).toBool();
        m_cursorShape = message.value(QStringLiteral("shape")).toInt();
        markDirty();
        return;
    }

    if (type == QStringLiteral("clipboard_set")) {
        if (QClipboard *clipboard = QGuiApplication::clipboard()) {
            clipboard->setText(message.value(QStringLiteral("text")).toString());
        }
        return;
    }

    if (type == QStringLiteral("quit")) {
        QGuiApplication::quit();
    }
}

void VtuiGridItem::recalculateMetrics()
{
    const QFontMetricsF metrics(m_font);
    m_cellWidth = std::ceil(metrics.horizontalAdvance(QStringLiteral("W")));
    m_cellHeight = std::ceil(metrics.height() + 1.0);
    m_ascent = metrics.ascent();
    emit metricsChanged();
}

void VtuiGridItem::resizeGrid(int cols, int rows)
{
    m_cols = cols;
    m_rows = rows;
    m_cells.resize(cols * rows);
    markDirty();
}

void VtuiGridItem::markDirty()
{
    m_imageDirty = true;
    update();
}

qreal VtuiGridItem::currentDevicePixelRatio() const
{
    if (window()) {
        return std::max<qreal>(1, window()->effectiveDevicePixelRatio());
    }
    return 1;
}

QImage VtuiGridItem::renderImage() const
{
    const qreal devicePixelRatio = currentDevicePixelRatio();
    const int pixelWidth = std::max(1, static_cast<int>(std::ceil(width() * devicePixelRatio)));
    const int pixelHeight = std::max(1, static_cast<int>(std::ceil(height() * devicePixelRatio)));
    QImage image(pixelWidth, pixelHeight, QImage::Format_RGBA8888_Premultiplied);
    image.setDevicePixelRatio(devicePixelRatio);
    image.fill(Qt::black);

    QPainter painter(&image);
    painter.setFont(m_font);
    painter.setRenderHint(QPainter::TextAntialiasing, true);
    painter.setRenderHint(QPainter::Antialiasing, false);

    for (int y = 0; y < m_rows; ++y) {
        for (int x = 0; x < m_cols; ++x) {
            const int index = y * m_cols + x;
            if (index < 0 || index >= m_cells.size()) {
                continue;
            }

            const Cell &cell = m_cells[index];
            const QRectF rect(x * m_cellWidth, y * m_cellHeight, m_cellWidth + 1, m_cellHeight + 1);
            painter.fillRect(rect, backgroundColor(cell.attr));

            const QString glyph = glyphForCell(cell.ch);
            if (!glyph.isEmpty()) {
                painter.setPen(foregroundColor(cell.attr));
                painter.drawText(QPointF(rect.left(), rect.top() + m_ascent), glyph);
            }

            if ((cell.attr & CommonLvbUnderscore) != 0) {
                painter.fillRect(QRectF(rect.left(), rect.bottom() - 2, rect.width(), 1), foregroundColor(cell.attr));
            }
            if ((cell.attr & CommonLvbStrikeout) != 0) {
                painter.fillRect(QRectF(rect.left(), rect.top() + rect.height() / 2, rect.width(), 1), foregroundColor(cell.attr));
            }
        }
    }

    if (m_cursorVisible && m_cursorX >= 0 && m_cursorY >= 0 && m_cursorX < m_cols && m_cursorY < m_rows) {
        const QRectF cursorRect(m_cursorX * m_cellWidth, m_cursorY * m_cellHeight, m_cellWidth, m_cellHeight);
        if (m_cursorShape == 1) {
            painter.fillRect(cursorRect, QColor(255, 255, 255, 100));
            painter.setPen(QColor(255, 255, 255, 220));
            painter.drawRect(cursorRect.adjusted(0.5, 0.5, -0.5, -0.5));
        } else {
            painter.fillRect(QRectF(cursorRect.left(), cursorRect.bottom() - 3, cursorRect.width(), 2),
                             QColor(255, 255, 255, 220));
        }
    }

    return image;
}

QColor VtuiGridItem::foregroundColor(quint64 attr) const
{
    quint32 rgb = (attr & IsFgRGB) != 0
        ? static_cast<quint32>((attr >> 16) & 0xffffff)
        : static_cast<quint32>(indexedColor(static_cast<int>((attr >> 16) & 0xff)).rgb() & 0xffffff);

    if ((attr & CommonLvbReverse) != 0) {
        return backgroundColor(attr & ~CommonLvbReverse);
    }

    QColor color = colorFromRgb(rgb);
    if ((attr & ForegroundDim) != 0) {
        color = color.darker(180);
    } else if ((attr & ForegroundIntensity) != 0) {
        color = color.lighter(125);
    }
    return color;
}

QColor VtuiGridItem::backgroundColor(quint64 attr) const
{
    quint32 rgb = (attr & IsBgRGB) != 0
        ? static_cast<quint32>((attr >> 40) & 0xffffff)
        : static_cast<quint32>(indexedColor(static_cast<int>((attr >> 40) & 0xff)).rgb() & 0xffffff);

    if ((attr & CommonLvbReverse) != 0) {
        return foregroundColor(attr & ~CommonLvbReverse);
    }

    QColor color = colorFromRgb(rgb);
    if ((attr & BackgroundIntensity) != 0) {
        color = color.lighter(125);
    }
    return color;
}

QColor VtuiGridItem::indexedColor(int index) const
{
    if (index >= 0 && index < m_palette.size()) {
        return colorFromRgb(m_palette[index]);
    }
    return index == 0 ? QColor(Qt::black) : QColor(Qt::white);
}

QPoint VtuiGridItem::cellForPosition(const QPointF &position) const
{
    const int x = std::clamp(static_cast<int>(position.x() / std::max<qreal>(1, m_cellWidth)), 0, std::max(0, m_cols - 1));
    const int y = std::clamp(static_cast<int>(position.y() / std::max<qreal>(1, m_cellHeight)), 0, std::max(0, m_rows - 1));
    return QPoint(x, y);
}

int VtuiGridItem::modifiersFromEvent(Qt::KeyboardModifiers modifiers) const
{
    int result = 0;
    if (modifiers.testFlag(Qt::ShiftModifier)) {
        result |= ShiftPressed;
    }
    if (modifiers.testFlag(Qt::ControlModifier)) {
        result |= LeftCtrlPressed;
    }
    if (modifiers.testFlag(Qt::AltModifier)) {
        result |= LeftAltPressed;
    }
    return result;
}

int VtuiGridItem::buttonState(Qt::MouseButton button) const
{
    switch (button) {
    case Qt::LeftButton:
        return FromLeft1stButtonPressed;
    case Qt::RightButton:
        return RightmostButtonPressed;
    case Qt::MiddleButton:
        return FromLeft2ndButtonPressed;
    default:
        return 0;
    }
}

int VtuiGridItem::keyToVk(const QKeyEvent *event) const
{
    const int key = event->key();
    if (key >= Qt::Key_A && key <= Qt::Key_Z) {
        return 0x41 + (key - Qt::Key_A);
    }
    if (key >= Qt::Key_0 && key <= Qt::Key_9) {
        return 0x30 + (key - Qt::Key_0);
    }
    if (key >= Qt::Key_F1 && key <= Qt::Key_F24) {
        return 0x70 + (key - Qt::Key_F1);
    }

    switch (key) {
    case Qt::Key_Backspace: return 0x08;
    case Qt::Key_Tab: return 0x09;
    case Qt::Key_Return:
    case Qt::Key_Enter: return 0x0d;
    case Qt::Key_Escape: return 0x1b;
    case Qt::Key_Space: return 0x20;
    case Qt::Key_PageUp: return 0x21;
    case Qt::Key_PageDown: return 0x22;
    case Qt::Key_End: return 0x23;
    case Qt::Key_Home: return 0x24;
    case Qt::Key_Left: return 0x25;
    case Qt::Key_Up: return 0x26;
    case Qt::Key_Right: return 0x27;
    case Qt::Key_Down: return 0x28;
    case Qt::Key_Insert: return 0x2d;
    case Qt::Key_Delete: return 0x2e;
    case Qt::Key_Shift: return 0x10;
    case Qt::Key_Control: return 0x11;
    case Qt::Key_Alt: return 0x12;
    case Qt::Key_CapsLock: return 0x14;
    case Qt::Key_NumLock: return 0x90;
    case Qt::Key_ScrollLock: return 0x91;
    case Qt::Key_Semicolon: return 0xba;
    case Qt::Key_Equal: return 0xbb;
    case Qt::Key_Comma: return 0xbc;
    case Qt::Key_Minus: return 0xbd;
    case Qt::Key_Period: return 0xbe;
    case Qt::Key_Slash: return 0xbf;
    case Qt::Key_QuoteLeft: return 0xc0;
    case Qt::Key_BracketLeft: return 0xdb;
    case Qt::Key_Backslash: return 0xdc;
    case Qt::Key_BracketRight: return 0xdd;
    case Qt::Key_Apostrophe: return 0xde;
    default:
        return 0;
    }
}

int VtuiGridItem::keyChar(const QKeyEvent *event) const
{
    if (event->text().isEmpty()) {
        return 0;
    }
    const QString text = event->text();
    const char32_t codepoint = text.toUcs4().isEmpty() ? 0 : text.toUcs4().front();
    if (codepoint < 0x20 && event->key() != Qt::Key_Tab && event->key() != Qt::Key_Return && event->key() != Qt::Key_Enter) {
        return 0;
    }
    return static_cast<int>(codepoint);
}

void VtuiGridItem::sendMouseEvent(QMouseEvent *event, int flags, bool down)
{
    if (!m_controller) {
        return;
    }
    const QPoint cell = cellForPosition(event->position());
    const int button = flags == MouseMoved ? m_pressedButtonState : buttonState(event->button());
    m_controller->sendMouse(cell.x(), cell.y(), button, flags, down, modifiersFromEvent(event->modifiers()));
}

void VtuiGridItem::maybeSendResize()
{
    if (!m_controller || m_cellWidth <= 0 || m_cellHeight <= 0) {
        return;
    }

    const int cols = std::max(1, static_cast<int>(std::floor(width() / m_cellWidth)));
    const int rows = std::max(1, static_cast<int>(std::floor(height() / m_cellHeight)));
    if (cols == m_lastSentCols && rows == m_lastSentRows) {
        return;
    }

    m_lastSentCols = cols;
    m_lastSentRows = rows;
    m_controller->sendResize(cols, rows);
}
