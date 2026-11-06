#version 450
#extension GL_GOOGLE_include_directive : require

layout(location = 0) in vec2 qt_TexCoord0;
layout(location = 0) out vec4 fragColor;
layout(std140, binding = 0) uniform buf {
    mat4 qt_Matrix;
    float qt_Opacity;
    vec2 viewportSize;
} ubuf;

#include "pixel_pattern.glsl"

void main()
{
    fragColor = pixelPattern(qt_TexCoord0, ubuf.viewportSize) * ubuf.qt_Opacity;
}
