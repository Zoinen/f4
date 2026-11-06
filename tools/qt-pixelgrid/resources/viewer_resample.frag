#version 450
#extension GL_GOOGLE_include_directive : require

layout(location = 0) in vec2 qt_TexCoord0;
layout(location = 1) flat in int pixelGridAligned;
layout(location = 0) out vec4 fragColor;

// Exact uniform layout of F4's ViewerResampleEffect and original vertex stage.
// Only this diagnostic fragment stage replaces the image-resampling algorithm.
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

#include "pixel_pattern.glsl"

void main()
{
    fragColor = pixelPattern(qt_TexCoord0, ubuf.viewportSize) * ubuf.qt_Opacity;
}
