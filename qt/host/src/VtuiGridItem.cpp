#include "VtuiGridItem.h"

#include "QtShellController.h"

#include <QClipboard>
#include <QCoreApplication>
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

#if defined(Q_OS_MACOS)
#include <Carbon/Carbon.h>
#endif

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

bool containsPrintableText(const QString &text)
{
    for (const QChar character : text) {
        if (character.isPrint()) {
            return true;
        }
    }
    return false;
}

#if defined(Q_OS_MACOS)
quint16 macVirtualKeyForQtKey(int key)
{
    if (key >= Qt::Key_A && key <= Qt::Key_Z) {
        static constexpr quint16 ansiLetters[] = {
            kVK_ANSI_A, kVK_ANSI_B, kVK_ANSI_C, kVK_ANSI_D, kVK_ANSI_E,
            kVK_ANSI_F, kVK_ANSI_G, kVK_ANSI_H, kVK_ANSI_I, kVK_ANSI_J,
            kVK_ANSI_K, kVK_ANSI_L, kVK_ANSI_M, kVK_ANSI_N, kVK_ANSI_O,
            kVK_ANSI_P, kVK_ANSI_Q, kVK_ANSI_R, kVK_ANSI_S, kVK_ANSI_T,
            kVK_ANSI_U, kVK_ANSI_V, kVK_ANSI_W, kVK_ANSI_X, kVK_ANSI_Y,
            kVK_ANSI_Z,
        };
        return ansiLetters[key - Qt::Key_A];
    }
    if (key >= Qt::Key_0 && key <= Qt::Key_9) {
        static constexpr quint16 ansiDigits[] = {
            kVK_ANSI_0, kVK_ANSI_1, kVK_ANSI_2, kVK_ANSI_3, kVK_ANSI_4,
            kVK_ANSI_5, kVK_ANSI_6, kVK_ANSI_7, kVK_ANSI_8, kVK_ANSI_9,
        };
        return ansiDigits[key - Qt::Key_0];
    }
    switch (key) {
    case Qt::Key_Space: return kVK_Space;
    case Qt::Key_QuoteLeft: return kVK_ANSI_Grave;
    case Qt::Key_Minus: return kVK_ANSI_Minus;
    case Qt::Key_Equal: return kVK_ANSI_Equal;
    case Qt::Key_BracketLeft: return kVK_ANSI_LeftBracket;
    case Qt::Key_Backslash: return kVK_ANSI_Backslash;
    case Qt::Key_BracketRight: return kVK_ANSI_RightBracket;
    case Qt::Key_Semicolon: return kVK_ANSI_Semicolon;
    case Qt::Key_Apostrophe: return kVK_ANSI_Quote;
    case Qt::Key_Comma: return kVK_ANSI_Comma;
    case Qt::Key_Period: return kVK_ANSI_Period;
    case Qt::Key_Slash: return kVK_ANSI_Slash;
    default: return UINT16_MAX;
    }
}

QString macTextWithoutOption(const QKeyEvent *event,
                             quint32 forwardedScanCode = 0)
{
    if (!event || !event->modifiers().testFlag(Qt::AltModifier)) {
        return {};
    }

    quint16 virtualKey = event->nativeVirtualKey() <= UINT16_MAX
        ? static_cast<quint16>(event->nativeVirtualKey()) : UINT16_MAX;
    if ((virtualKey == 0 || virtualKey == UINT16_MAX)
        && forwardedScanCode > 0 && forwardedScanCode <= UINT16_MAX) {
        virtualKey = static_cast<quint16>(forwardedScanCode);
    }
    if (virtualKey == 0 || virtualKey == UINT16_MAX) {
        virtualKey = macVirtualKeyForQtKey(event->key());
    }
    if (virtualKey == UINT16_MAX) {
        return {};
    }

    TISInputSourceRef source = TISCopyCurrentKeyboardInputSource();
    if (!source) {
        return {};
    }
    const auto *layoutData = static_cast<CFDataRef>(
        TISGetInputSourceProperty(source, kTISPropertyUnicodeKeyLayoutData));
    if (!layoutData) {
        CFRelease(source);
        source = TISCopyCurrentKeyboardLayoutInputSource();
        layoutData = source ? static_cast<CFDataRef>(
            TISGetInputSourceProperty(source,
                                      kTISPropertyUnicodeKeyLayoutData))
                            : nullptr;
    }
    if (!layoutData) {
        if (source) {
            CFRelease(source);
        }
        return {};
    }

    const auto *layout = reinterpret_cast<const UCKeyboardLayout *>(
        CFDataGetBytePtr(layoutData));
    UInt32 deadKeyState = 0;
    UniChar chars[8]{};
    UniCharCount length = 0;
    const UInt32 modifiers = event->modifiers().testFlag(Qt::ShiftModifier)
        ? (shiftKey >> 8) : 0;
    const OSStatus status = UCKeyTranslate(
        layout, virtualKey, kUCKeyActionDown, modifiers,
        LMGetKbdType(), kUCKeyTranslateNoDeadKeysBit, &deadKeyState,
        std::size(chars), &length, chars);
    CFRelease(source);
    if (status != noErr || length == 0) {
        return {};
    }
    return QString::fromUtf16(reinterpret_cast<const char16_t *>(chars),
                              static_cast<qsizetype>(length));
}
#endif
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
    if (QCoreApplication *application = QCoreApplication::instance()) {
        application->installEventFilter(this);
    }

    m_font.setStyleHint(QFont::Monospace);
    m_font.setFamilies({QStringLiteral("Monaco"),
                        QStringLiteral("Menlo"),
                        QStringLiteral("Consolas"),
                        QStringLiteral("DejaVu Sans Mono"),
                        QStringLiteral("monospace")});
#if defined(Q_OS_MACOS)
    m_font.setPixelSize(17);
#else
    m_font.setPixelSize(16);
#endif
    recalculateMetrics();
}

void VtuiGridItem::setFontFamily(const QString &family)
{
    const QString normalized = family.trimmed();
    if (normalized.isEmpty() || m_font.family() == normalized) {
        return;
    }
    m_font.setFamily(normalized);
    recalculateMetrics();
    markDirty();
    emit fontChanged();
}

void VtuiGridItem::setFontPixelSize(int size)
{
    if (size <= 0 || m_font.pixelSize() == size) {
        return;
    }
    m_font.setPixelSize(size);
    recalculateMetrics();
    markDirty();
    emit fontChanged();
}

void VtuiGridItem::setController(QObject *controller)
{
    if (m_controllerObject == controller) {
        return;
    }

    if (m_controller) {
        releaseForwardedKeys();
        disconnect(m_controller, nullptr, this, nullptr);
    }

    m_controllerObject = controller;
    m_controller = qobject_cast<QtShellController *>(controller);
    if (m_controller) {
        connect(m_controller, &QtShellController::messageReceived, this, &VtuiGridItem::handleMessage);
    }

    emit controllerChanged();
}

void VtuiGridItem::setPointerInputEnabled(bool enabled)
{
    if (m_pointerInputEnabled == enabled) {
        return;
    }
    m_pointerInputEnabled = enabled;
    setAcceptedMouseButtons(enabled ? Qt::AllButtons : Qt::NoButton);
    setAcceptHoverEvents(enabled);
    if (!enabled) {
        m_pressedButtonState = 0;
        m_wheelRemainder = 0;
    }
    emit pointerInputEnabledChanged();
}

void VtuiGridItem::setInputMethodForwardingEnabled(bool enabled)
{
    if (m_inputMethodForwardingEnabled == enabled) {
        return;
    }
    m_inputMethodForwardingEnabled = enabled;
    emit inputMethodForwardingEnabledChanged();
}

void VtuiGridItem::setTerminalInputEnabled(bool enabled)
{
    if (m_terminalInputEnabled == enabled) {
        return;
    }
    if (!enabled) {
        // Balance every key-down that reached Go before the modal surface
        // appeared. A later physical release is ignored after the map clears,
        // so neither a stuck commander modifier nor an orphan release can
        // cross a GalleryViewer focus transition.
        releaseForwardedKeys();
    }
    m_terminalInputEnabled = enabled;
    emit terminalInputEnabledChanged();
}

void VtuiGridItem::setRenderingEnabled(bool enabled)
{
    if (m_renderingEnabled == enabled) {
        return;
    }
    m_renderingEnabled = enabled;
    if (enabled) {
        // Frames continue updating the authoritative cell buffer while the
        // semantic surface covers this item. Paint the latest accumulated
        // state exactly once when the fallback grid becomes visible again.
        update();
    }
    emit renderingEnabledChanged();
}

bool VtuiGridItem::forwardKeyToController(int vk, int ch, bool down, int mods,
                                         bool repeat)
{
    if (!m_controller) {
        return false;
    }
    if (down) {
        if (!m_terminalInputEnabled) {
            return false;
        }
        m_controller->sendKeyEvent(vk, ch, true, mods, repeat);
        m_forwardedKeyModifiers.insert(vk, mods);
        return true;
    }

    // Some Qt platform backends expose autorepeat as synthetic release/press
    // pairs. The synthetic release is not a physical key-up: forwarding it
    // would end the held-key burst immediately before its repeat press.
    if (repeat) {
        return true;
    }

    const auto forwarded = m_forwardedKeyModifiers.constFind(vk);
    if (forwarded == m_forwardedKeyModifiers.cend()) {
        return false;
    }
    m_controller->sendKey(vk, 0, false, mods);
    m_forwardedKeyModifiers.remove(vk);
    return true;
}

void VtuiGridItem::releaseForwardedKeys()
{
    if (m_controller) {
        for (auto it = m_forwardedKeyModifiers.cbegin();
             it != m_forwardedKeyModifiers.cend(); ++it) {
            m_controller->sendKey(it.key(), 0, false, it.value());
        }
    }
    m_forwardedKeyModifiers.clear();
}

void VtuiGridItem::sendQtKey(int key, const QString &text, bool down,
                             int modifiers, quint32 nativeScanCode)
{
    sendQtKeyEvent(key, text, down, modifiers, nativeScanCode, false);
}

void VtuiGridItem::sendQtKeyEvent(int key, const QString &text, bool down,
                                  int modifiers, quint32 nativeScanCode,
                                  bool autoRepeat)
{
    if (!m_controller) {
        return;
    }

    const auto qtModifiers = Qt::KeyboardModifiers::fromInt(modifiers);
    if (down && nativeScanCode == 0
        && qtModifiers.testFlag(Qt::AltModifier)) {
        nativeScanCode = m_lastNativeAltScanCode;
    }
    QKeyEvent event(down ? QEvent::KeyPress : QEvent::KeyRelease,
                    key, qtModifiers, nativeScanCode, nativeScanCode, 0,
                    down ? text : QString(), autoRepeat);
    int nativeModifiers = modifiersFromEvent(qtModifiers);
    if (isEnhancedQtKey(key)) {
        nativeModifiers |= EnhancedKey;
    }
    const bool forwarded = forwardKeyToController(
        keyToVk(&event), down ? keyChar(&event) : 0, down, nativeModifiers,
        autoRepeat);
    if (forwarded && down) {
        emit keyboardActivity();
    }
    if (forwarded && down && containsPrintableText(text)) {
        emit commanderTextInputForwarded(text, modifiers);
    }
}

void VtuiGridItem::sendClipboardPaste()
{
    if (!m_terminalInputEnabled || !m_controller) {
        return;
    }
    if (QClipboard *clipboard = QGuiApplication::clipboard()) {
        const QString text = clipboard->text();
        if (!text.isEmpty()) {
            m_controller->sendPaste(text);
            emit keyboardActivity();
            emit commanderTextInputForwarded(text, 0);
        }
    }
}

void VtuiGridItem::sendQtText(const QString &text)
{
    if (m_terminalInputEnabled && m_controller && !text.isEmpty()) {
        m_controller->sendText(text);
        emit keyboardActivity();
        emit commanderTextInputForwarded(text, 0);
    }
}

bool VtuiGridItem::eventFilter(QObject *watched, QEvent *event)
{
#if defined(Q_OS_MACOS)
    // Qt Quick exposes the layout-produced key/text to QML, but not a usable
    // physical key code on every Qt/macOS combination. Capture it from the
    // original application event before GalleryPanelHost forwards the key.
    if (event && event->type() == QEvent::KeyPress) {
        const auto *keyEvent = static_cast<QKeyEvent *>(event);
        if (keyEvent->modifiers().testFlag(Qt::AltModifier)
            && keyEvent->nativeVirtualKey() > 0
            && keyEvent->nativeVirtualKey() <= UINT16_MAX) {
            m_lastNativeAltScanCode = keyEvent->nativeVirtualKey();
        }
    }
#endif
    if (m_terminalInputEnabled && m_inputMethodForwardingEnabled && m_controller
        && event && event->type() == QEvent::InputMethod) {
        auto *inputMethodEvent = static_cast<QInputMethodEvent *>(event);
        if (!inputMethodEvent->commitString().isEmpty()) {
            sendQtText(inputMethodEvent->commitString());
            inputMethodEvent->accept();
            return true;
        }
    }
    return QQuickItem::eventFilter(watched, event);
}

QSGNode *VtuiGridItem::updatePaintNode(QSGNode *oldNode, UpdatePaintNodeData *)
{
    // The semantic UI keeps this item visible as a callable keyboard/IME
    // sink. Opacity alone does not prevent an explicit update() from running
    // this full-cell QPainter/texture-upload path, so suppress visual work
    // independently while retaining every incoming frame in m_cells.
    if (!m_renderingEnabled) {
        return oldNode;
    }

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
        sendClipboardPaste();
        event->accept();
        return;
    }

    if (m_controller) {
        int mods = modifiersFromEvent(event->modifiers());
        if (isEnhancedQtKey(event->key())) {
            mods |= EnhancedKey;
        }
        const bool forwarded = forwardKeyToController(
            keyToVk(event), keyChar(event), true, mods,
            event->isAutoRepeat());
        if (forwarded) {
            emit keyboardActivity();
        }
        if (forwarded && containsPrintableText(event->text())) {
            emit commanderTextInputForwarded(event->text(),
                                             event->modifiers().toInt());
        }
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
        forwardKeyToController(keyToVk(event), 0, false, mods,
                               event->isAutoRepeat());
    }
    event->accept();
}

void VtuiGridItem::inputMethodEvent(QInputMethodEvent *event)
{
    sendQtText(event->commitString());
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
    if (!m_pointerInputEnabled) {
        event->ignore();
        return;
    }
    m_pressedButtonState |= buttonState(event->button());
    sendMouseEvent(event, 0, true);
    forceActiveFocus();
    event->accept();
}

void VtuiGridItem::mouseReleaseEvent(QMouseEvent *event)
{
    if (!m_pointerInputEnabled) {
        event->ignore();
        return;
    }
    sendMouseEvent(event, 0, false);
    m_pressedButtonState &= ~buttonState(event->button());
    event->accept();
}

void VtuiGridItem::mouseMoveEvent(QMouseEvent *event)
{
    if (!m_pointerInputEnabled) {
        event->ignore();
        return;
    }
    sendMouseEvent(event, MouseMoved, event->buttons() != Qt::NoButton);
    event->accept();
}

void VtuiGridItem::wheelEvent(QWheelEvent *event)
{
    if (!m_pointerInputEnabled) {
        event->ignore();
        return;
    }
    if (isTouchpadScroll(event)) {
        m_wheelRemainder = 0;
        event->accept();
        return;
    }

    if (!m_controller) {
        return;
    }

    const int delta = event->angleDelta().y();
    if (delta == 0) {
        event->accept();
        return;
    }

    const QPoint cell = cellForPosition(event->position());
    m_wheelRemainder += delta;
    while (std::abs(m_wheelRemainder) >= 120) {
        const int dir = m_wheelRemainder > 0 ? 1 : -1;
        m_controller->sendWheel(cell.x(), cell.y(), dir, modifiersFromEvent(event->modifiers()));
        m_wheelRemainder -= dir * 120;
    }
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
        ++m_retainedFrameRevision;
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
    if (m_renderingEnabled) {
        update();
    }
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
    if (modifiers.testFlag(Qt::ControlModifier)
#ifdef Q_OS_MACOS
        || modifiers.testFlag(Qt::MetaModifier)
#endif
    ) {
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
#if defined(Q_OS_MACOS)
    const QString textWithoutOption = macTextWithoutOption(event);
    const QString text = textWithoutOption.isEmpty()
        ? event->text() : textWithoutOption;
#else
    const QString text = event->text();
#endif
    if (text.isEmpty()) {
        return 0;
    }
    const char32_t codepoint = text.toUcs4().isEmpty() ? 0 : text.toUcs4().front();
    if (codepoint < 0x20 && event->key() != Qt::Key_Tab && event->key() != Qt::Key_Return && event->key() != Qt::Key_Enter) {
        return 0;
    }
    return static_cast<int>(codepoint);
}

bool VtuiGridItem::isTouchpadScroll(const QWheelEvent *event) const
{
    if (!event->pixelDelta().isNull()) {
        return true;
    }
    if (event->phase() != Qt::NoScrollPhase) {
        return true;
    }
    return event->source() == Qt::MouseEventSynthesizedBySystem
        || event->source() == Qt::MouseEventSynthesizedByQt;
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
