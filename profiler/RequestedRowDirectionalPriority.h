#pragma once

#include "GalleryViewportMaterializer.h"

#include <QList>
#include <QSet>
#include <algorithm>

// Keep visible rows first. Within each overscan tier, approach the nearest
// edge in the requested direction before preparing rows behind the camera.
template <typename IndexesInRange>
QList<int> requestedRowDirectionalPriority(
    const ZoinGallery::GalleryViewportWindow &window,
    qreal targetOffset, IndexesInRange indexesInRange)
{
    QList<int> priority = indexesInRange(window.visibleStart, window.visibleEnd);
    QSet<int> seen(priority.cbegin(), priority.cend());
    const auto append = [&](QList<int> indexes, bool reverse = false) {
        if (reverse)
            std::reverse(indexes.begin(), indexes.end());
        for (int index : indexes) {
            if (!seen.contains(index)) {
                seen.insert(index);
                priority.append(index);
            }
        }
    };
    const auto appendTier = [&](qreal begin, qreal end) {
        if (targetOffset > window.visibleStart) {
            append(indexesInRange(window.visibleEnd, end));
            append(indexesInRange(begin, window.visibleStart), true);
        } else if (targetOffset < window.visibleStart) {
            append(indexesInRange(begin, window.visibleStart), true);
            append(indexesInRange(window.visibleEnd, end));
        } else {
            append(indexesInRange(begin, end));
        }
    };
    appendTier(window.delegateStart, window.delegateEnd);
    appendTier(window.metadataStart, window.metadataEnd);
    return priority;
}
