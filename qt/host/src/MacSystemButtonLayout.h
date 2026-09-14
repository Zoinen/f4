#pragma once

class QQuickWindow;

// Observe native layout for the lifetime of the window, not just its first frame.
void scheduleMacSystemButtonLayout(QQuickWindow *window);
