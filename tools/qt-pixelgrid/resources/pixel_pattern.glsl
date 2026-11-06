// Integrate a one-physical-pixel-wide stroke over the fragment footprint.
// This preserves the stroke's width while revealing fractional translation as
// partial coverage. A hard step/floor would conceal the phase error.
float intervalCoverage(float center, float footprint, float lo, float hi)
{
    return clamp((min(center + footprint * 0.5, hi)
                  - max(center - footprint * 0.5, lo)) / footprint, 0.0, 1.0);
}

float boxCoverage(vec2 p, vec2 footprint, vec2 lo, vec2 hi)
{
    return intervalCoverage(p.x, footprint.x, lo.x, hi.x)
        * intervalCoverage(p.y, footprint.y, lo.y, hi.y);
}

vec4 pixelPattern(vec2 uv, vec2 size)
{
    vec2 p = uv * size;
    vec2 footprint = max(fwidth(p), vec2(0.00001));
    float frame = boxCoverage(p, footprint, vec2(4.0), size - 4.0)
        - boxCoverage(p, footprint, vec2(5.0), size - 5.0);
    float stripes = 0.0;
    for (int i = 0; i < 19; ++i) {
        float x = 12.0 + float(i) * 4.0;
        stripes += boxCoverage(p, footprint, vec2(x, 16.0), vec2(x + 1.0, 80.0));
    }
    for (int i = 0; i < 16; ++i) {
        float y = 16.0 + float(i) * 4.0;
        stripes += boxCoverage(p, footprint, vec2(104.0, y), vec2(180.0, y + 1.0));
    }
    vec3 color = vec3(32.0, 40.0, 48.0) / 255.0;
    color = mix(color, vec3(0.0, 191.0, 255.0) / 255.0, frame);
    color = mix(color, vec3(1.0), clamp(stripes, 0.0, 1.0));
    return vec4(color, 1.0);
}
