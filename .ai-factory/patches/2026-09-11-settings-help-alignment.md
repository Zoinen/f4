# Settings help alignment and responsive widths

Reproduced description offset 13.5714 DIP from its viewport top. The generic row
layout centered a native paragraph inside its console wrapping height. Added
naturalText layout for the description role: top-aligned paragraph, full live
width, no internal viewport padding. Docked wide help at the heading's top edge.
Child Loader now owns responsive width; assigning width only to its loaded item
allowed Qt Loader sizing to retain an old extent after resize. Hidden scrollbars
no longer reserve width. Existing text/pixel-alignment rendering is retained.

Extended the 175% regression across three sizes and different text/console row
counts: verify origin, usable width, page/help gap including scrollbar, every
visible text/image scene origin and unit transform. Capture each size for visual
inspection. Related dialog tests and portable static build checks run separately.
