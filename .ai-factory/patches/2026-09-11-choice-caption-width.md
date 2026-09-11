# Radio caption natural width

Reproduced Startup mode wrapping to two lines in adaptiveChoicesAndFilledFields.
FontMetrics advanceWidth rounded to nearest physical pixel could allocate less
than the Text layout required. Measure the caption's actual implicitWidth and
round upward to the physical pixel grid; retain normal wrapping below the inline
layout threshold. Caption text is explicitly plain text.

Regression covers Startup mode, Terminal renderer and Launch defaults at 12, 13,
14, 16 and 18 pixel font sizes, plus three responsive group widths. At DPR 1.75,
caption/radio/text leaves retain aligned scene origins and unit transforms.
Rendered capture inspected; Settings resize and static QML import smoke passed.
