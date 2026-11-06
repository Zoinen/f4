#version 450

layout(location = 0) out vec4 fragColor;
layout(std140, binding = 0) uniform buf {
    mat4 qt_Matrix;
    float qt_Opacity;
} ubuf;

void main()
{
    fragColor = vec4(0.0, 1.0, 0.0, 1.0) * ubuf.qt_Opacity;
}
