#version 450

layout(location = 0) in vec4 qt_Vertex;
layout(location = 1) in vec2 qt_MultiTexCoord0;
layout(location = 0) out vec2 qt_TexCoord0;
layout(location = 1) flat out int pixelGridAligned;

layout(std140, binding = 0) uniform buf {
    mat4 qt_Matrix;
    float qt_Opacity;
    vec2 viewportSize;
    vec2 checkerboardOffset;
    bool showCheckerboard;
    int checkerboardSize;
    float borderRadius;
    bool intermediate;
    bool pixelAlignedIdentity;
    vec2 sourceExtent;
    bool pixelAligned;
    vec2 itemSize;
    vec4 framebufferRect;
    float framebufferYDirection;
} ubuf;

vec2 framebufferPosition(vec4 clipPosition)
{
    vec2 ndc = clipPosition.xy / clipPosition.w;
    ndc.y *= ubuf.framebufferYDirection;
    return ubuf.framebufferRect.xy
        + (ndc + 1.0) * ubuf.framebufferRect.zw * 0.5;
}

vec2 cardinalDirection(vec2 axis)
{
    float extent = length(axis);
    if (extent < 0.00001)
        return vec2(0.0);
    vec2 direction = axis / extent;
    if (abs(direction.x) > 0.999999 && abs(direction.y) < 0.000001)
        return vec2(sign(direction.x), 0.0);
    if (abs(direction.y) > 0.999999 && abs(direction.x) < 0.000001)
        return vec2(0.0, sign(direction.y));
    return vec2(0.0);
}

void main()
{
    qt_TexCoord0 = qt_MultiTexCoord0;
    pixelGridAligned = 0;
    gl_Position = ubuf.qt_Matrix * qt_Vertex;
    if (!ubuf.pixelAligned || ubuf.intermediate
            || any(lessThanEqual(ubuf.framebufferRect.zw, vec2(0.0)))
            || any(lessThanEqual(ubuf.viewportSize, vec2(0.0))))
        return;

    vec4 originClip = ubuf.qt_Matrix * vec4(0.0, 0.0, 0.0, 1.0);
    vec4 endXClip = ubuf.qt_Matrix * vec4(ubuf.itemSize.x, 0.0, 0.0, 1.0);
    vec4 endYClip = ubuf.qt_Matrix * vec4(0.0, ubuf.itemSize.y, 0.0, 1.0);
    // Perspective and non-cardinal transforms keep continuous sampling.
    if (abs(originClip.w) < 0.00001
            || abs(endXClip.w - originClip.w) > 0.000001
            || abs(endYClip.w - originClip.w) > 0.000001)
        return;
    vec2 origin = framebufferPosition(originClip);
    vec2 axisX = framebufferPosition(endXClip) - origin;
    vec2 axisY = framebufferPosition(endYClip) - origin;
    vec2 directionX = cardinalDirection(axisX);
    vec2 directionY = cardinalDirection(axisY);
    if (length(directionX) < 0.5 || length(directionY) < 0.5
            || abs(dot(directionX, directionY)) > 0.5)
        return;

    // On fractional-scale Wayland, QWaylandScreen's integer output scale can
    // differ from the concrete window/render target DPR in Qt's matrix. In
    // that case replacing the matrix projection with the screen-derived
    // viewportSize would scale and offset the image when motion ends.
    // Keep Qt's projection (and continuous filtering) for such render targets;
    // the regular path below is only safe when both pixel grids coincide.
    vec2 projectedScale = vec2(length(axisX) / ubuf.itemSize.x,
                               length(axisY) / ubuf.itemSize.y);
    vec2 screenScale = ubuf.viewportSize / ubuf.itemSize;
    if (any(greaterThan(abs(projectedScale - screenScale), vec2(0.01))))
        return;

    // The actual framebuffer can differ from rounded logical window size *
    // DPR. Snap the final projected rectangle, using the intended pixel count,
    // so a half-size pyramid is not bilinearly stretched a second time.
    vec2 extent = max(vec2(1.0), floor(ubuf.viewportSize + 0.5));
    vec2 alignedX = directionX * extent.x;
    vec2 alignedY = directionY * extent.y;
    vec2 center = origin + (axisX + axisY) * 0.5;
    vec2 halfParity = mod(abs(alignedX) + abs(alignedY), 2.0) * 0.5;
    center = floor(center - halfParity + 0.5) + halfParity;
    vec2 alignedOrigin = center - (alignedX + alignedY) * 0.5;
    vec2 screenExtent = abs(alignedX) + abs(alignedY);
    bvec2 oversized = greaterThan(screenExtent, ubuf.framebufferRect.zw);
    if (any(oversized)) {
        // The viewer's resting ancestor transforms are unit/cardinal. Undo
        // framebuffer rounding on oversized axes before snapping their origin:
        // correcting only a far-off center would shift the clipped source rows
        // or columns, including during large negative pan offsets.
        vec2 projectedExtent = abs(axisX) + abs(axisY);
        vec2 nominalOrigin = ubuf.framebufferRect.xy
            + (origin - ubuf.framebufferRect.xy) * screenExtent / projectedExtent;
        alignedOrigin = mix(alignedOrigin, floor(nominalOrigin + 0.5), oversized);
    }
    vec2 pixel = alignedOrigin + alignedX * qt_MultiTexCoord0.x
                              + alignedY * qt_MultiTexCoord0.y;
    vec2 ndc = (pixel - ubuf.framebufferRect.xy)
        * 2.0 / ubuf.framebufferRect.zw - 1.0;
    ndc.y *= ubuf.framebufferYDirection;
    gl_Position.xy = ndc * gl_Position.w;
    pixelGridAligned = 1;
}
