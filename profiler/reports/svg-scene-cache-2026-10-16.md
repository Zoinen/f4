# SVG scene reuse: implementation and verification

Panel SVG scenes now survive delegate recycling. The warm Grid QML trace contains 0 VectorImage creation events, while lightweight presentation wrappers are still recycled.

Each panel retains at most 16 idle scenes plus surfaces currently used by its bounded delegate set. Exact source URLs contain immutable revision keys. Idle scenes use LRU eviction; path/session changes invalidate them. Source pins preserve materialized-file leases while queued or cached. Pending abandoned requests are discarded. Admission allows one new source assignment per animation frame and pauses while the viewer owns the session or the panel is hidden. Existing source acquisition and viewer priorities remain unchanged.

The existing layout/decode pipeline supplies visible and overscan sources. This change does not scan the directory for speculative scene work. It retains direct vector geometry and never caches a rasterized replacement. A single complex uncached SVG can still exceed the frame budget.

## Native frame measurements

Downloads, 183 entries, 160 Hz, DPR 1.75. Warm complete downward sweeps only; four seconds per sweep, QML recording off. The neighboring pane now also shows Downloads and the saved Grid density is 237, unlike the earlier capture. These results are not a controlled before/after comparison against the October 15 report. Frame-swap intervals measure submission pacing, not GPU execution or scanout.

| Mode | Density | p95 ms | p99 ms | Worst ms | ≥9.375 ms |
|---|---:|---:|---:|---:|---:|
| Details | 22.29 | 13.11 | 16.28 | 19.29 | 15.1% |
| Columns | 22.29 | 7.68 | 15.84 | 61.63 | 4.6% |
| Grid | 237.00 | 10.97 | 14.96 | 19.91 | 10.8% |
| Icons | 112.00 | 13.41 | 25.73 | 30.24 | 9.9% |
| Masonry | 202.00 | 11.40 | 16.21 | 18.06 | 9.3% |

The 6.25 ms budget is still missed. This change removes repeated SVG construction; it does not claim to resolve the remaining rendering or delegate/batch churn. Warm Grid caption measurements remained zero.

## Regression and portable checks

- 16 selected Qt regression cases pass: reuse, LRU bound, pending cancellation, changed revision, source-pin retention/release, local/virtual SVG/SVGZ acquisition, viewer visibility/animation, and stale-failure handling.
- All five panel modes pass final scene-space translation and unit-transform checks for vector leaves and caption leaves at DPR 1.75. Rendered captures were saved and inspected, including Details filename and small size text.
- Reusing a scene preserves the same VectorImage object and identical rendered output.
- Portable Windows system-DLL audit and embedded-only QML import test pass. Normal payload regenerated; Go launcher built with CGO_ENABLED=0 and the embedded Qt tag.

## QML attribution trace

Full capture: [cache-verify-grid.qtd](D:/Code/f4-zoin/artifacts/modes-profile/cache-verify-grid.qtd). Parser active-time estimate: 8000.00 ms. Inclusive totals overlap and include presentation-mode setup; they are not steady per-frame costs. Native pacing above comes from separate recording-off sweeps.

| Type | Count | Inclusive ms | ms/animation frame |
|---|---:|---:|---:|
| Binding | 113326 | 1675.26 | 1.309 |
| Javascript | 186926 | 1342.00 | 1.048 |
| Creating | 25364 | 603.44 | 0.471 |
| HandlingSignal | 8258 | 439.06 | 0.343 |
| Compiling | 6 | 0.59 | 0.000 |

## Memory

Short-lived JS allocation churn remains; this trace does not establish a leak. 249,006 object allocations, 27.87 MiB allocated, 71.0% reclaimed, 8.07 MiB live at capture end. Peak GC heap pages: 17.05 MiB.

| Category | Allocations | Total allocated | Reclaimed | Peak live | Live at exit |
|---|---:|---:|---:|---:|---:|
| GC heap pages | 3 | 17.17 MiB | 0.99 MiB | 17.05 MiB | 16.18 MiB |
| Small JS objects | 249006 | 27.87 MiB | 19.80 MiB | 16.58 MiB | 8.07 MiB |

GC heap pages back the QV4 allocator; small/large object counters track allocations within its object accounting. Do not add these categories together. Zero categories omitted.

## Pixmap cache

Requests: 396; loaded: 343; removed: 242. Includes both panes and mode setup. These raster counters do not represent vector-scene geometry. Loaded dimensions below do not reveal an obvious new oversized SVG bitmap; SVGs use no raster fallback.

| Source | Dimensions | Pixels |
|---|---:|---:|
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-916x512-356` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb704ebd5f003655-v1789051989722368100:799344-s799344-916x512-357` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-358422fb44357a4f-v1789051992780984400:795683-s795683-916x512-361` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-916x512-360` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-916x512-356` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb704ebd5f003655-v1789051989722368100:799344-s799344-916x512-357` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-358422fb44357a4f-v1789051992780984400:795683-s795683-916x512-361` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-916x512-360` | 916×512 | 468992 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-acafa8327f8b8b33-v1789051982300703000:674207-s674207-923x508-362` | 923×508 | 468884 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-acafa8327f8b8b33-v1789051982300703000:674207-s674207-923x508-362` | 923×508 | 468884 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1a365f3a7cf06a6d-v1789051982469777600:687802-s687802-916x504-364` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-dd6d7289823242b0-v1789051983202534600:690087-s690087-916x504-365` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-893713db778ff93d-v1789052183361837400:679716-s679716-916x504-366` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e4f4b0f8b1590147-v1789052181430938300:533790-s533790-916x504-367` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-893713db778ff93d-v1789052183361837400:679716-s679716-916x504-366` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e4f4b0f8b1590147-v1789052181430938300:533790-s533790-916x504-367` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1a365f3a7cf06a6d-v1789051982469777600:687802-s687802-916x504-364` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-dd6d7289823242b0-v1789051983202534600:690087-s690087-916x504-365` | 916×504 | 461664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1246daafdad65e23-v1789052005136169600:777537-s777537-907x507-363` | 907×507 | 459849 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1246daafdad65e23-v1789052005136169600:777537-s777537-907x507-363` | 907×507 | 459849 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-680x513-349` | 680×513 | 348840 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-680x513-350` | 680×513 | 348840 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/table.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/text.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/table.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/text.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 514×643 | 330502 |
| `qrc:/F4QtHost/icons/zoin/table.svg` | 514×643 | 330502 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-708x396-354` | 708×396 | 280368 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-708x396-354` | 708×396 | 280368 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2ef176d5acb84b0b-v1789051904603847600:1958221-s1958221-707x396-355` | 707×396 | 279972 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2ef176d5acb84b0b-v1789051904603847600:1958221-s1958221-707x396-355` | 707×396 | 279972 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/F4QtHost/icons/zoin/html.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/F4QtHost/icons/zoin/html.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 514×514 | 264196 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-681x375-338` | 681×375 | 255375 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-681x375-338` | 681×375 | 255375 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e149e1f8ee7d06f6-v1789052181495278500:656823-s656823-676x372-368` | 676×372 | 251472 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e149e1f8ee7d06f6-v1789052181495278500:656823-s656823-676x372-368` | 676×372 | 251472 |
| `qrc:/F4QtHost/icons/zoin/json.svg` | 514×484 | 248776 |
| `qrc:/F4QtHost/icons/zoin/json.svg` | 514×484 | 248776 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-661x369-352` | 661×369 | 243909 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-661x369-353` | 661×369 | 243909 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-614x393-371` | 614×393 | 241302 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-641x361-347` | 641×361 | 231401 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-641x361-347` | 641×361 | 231401 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-450x514-346` | 450×514 | 231300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-508x445-321` | 508×445 | 226060 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-508x445-321` | 508×445 | 226060 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-541x415-294` | 541×415 | 224515 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-541x415-294` | 541×415 | 224515 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-541x415-294` | 541×415 | 224515 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-541x415-294` | 541×415 | 224515 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-587x361-348` | 587×361 | 211907 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-587x361-348` | 587×361 | 211907 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 514×411 | 211254 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 514×411 | 211254 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 514×411 | 211254 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 514×411 | 211254 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 514×411 | 211254 |
| `qrc:/F4QtHost/icons/zoin/folder.svg` | 514×411 | 211254 |
| `qrc:/F4QtHost/icons/zoin/folder-up.svg` | 514×411 | 211254 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 450×450 | 202500 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-488x369-351` | 488×369 | 180072 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-448x394-326` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-448x394-327` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-448x394-328` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-448x394-329` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-448x394-330` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-448x394-334` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-448x394-336` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-448x394-331` | 448×394 | 176512 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-449x393-322` | 449×393 | 176457 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-449x393-323` | 449×393 | 176457 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-449x393-322` | 449×393 | 176457 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-449x393-323` | 449×393 | 176457 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-447x393-324` | 447×393 | 175671 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-447x393-325` | 447×393 | 175671 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-447x393-324` | 447×393 | 175671 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-447x393-325` | 447×393 | 175671 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-416x416-296` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-416x416-296` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-416x416-297` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-416x416-297` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-416x416-296` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-416x416-297` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-416x416-296` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-416x416-297` | 416×416 | 173056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-472x354-293` | 472×354 | 167088 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-472x354-293` | 472×354 | 167088 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-472x354-293` | 472×354 | 167088 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-472x354-293` | 472×354 | 167088 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-431x379-332` | 431×379 | 163349 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-431x379-333` | 431×379 | 163349 |
| `qrc:/F4QtHost/icons/zoin/folder.svg` | 450×360 | 162000 |
| `qrc:/F4QtHost/icons/zoin/folder-up.svg` | 450×360 | 162000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-446x342-196` | 446×342 | 152532 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-446x342-196` | 446×342 | 152532 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-446x342-196` | 446×342 | 152532 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-446x342-196` | 446×342 | 152532 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-446x342-196` | 446×342 | 152532 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-448x338-262` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-448x338-265` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-448x338-265` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-448x338-261` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-448x338-261` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-448x338-265` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-448x338-265` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-448x338-261` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-448x338-261` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-448x338-262` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-448x338-262` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-448x338-262` | 448×338 | 151424 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-448x336-192` | 448×336 | 150528 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-448x336-192` | 448×336 | 150528 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-448x336-192` | 448×336 | 150528 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-390x342-231` | 390×342 | 133380 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-390x342-234` | 390×342 | 133380 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-390x342-235` | 390×342 | 133380 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-389x342-252` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-389x342-241` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-389x342-253` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-389x342-242` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-389x342-243` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-389x342-245` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-389x342-243` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-389x342-245` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-389x342-252` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-389x342-241` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-389x342-253` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-389x342-242` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-389x342-237` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-389x342-236` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-389x342-238` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-389x342-240` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-389x342-232` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-389x342-233` | 389×342 | 133038 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-363x363-310` | 363×363 | 131769 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-363x363-310` | 363×363 | 131769 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-362x362-298` | 362×362 | 131044 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-362x362-298` | 362×362 | 131044 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-362x362-298` | 362×362 | 131044 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-362x362-298` | 362×362 | 131044 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-448x287-250` | 448×287 | 128576 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-448x287-250` | 448×287 | 128576 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-356x356-342` | 356×356 | 126736 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-356x356-344` | 356×356 | 126736 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-221d25a0ceeabe30-v1785516399377774300:48469-s48469-356x356-369` | 356×356 | 126736 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b95a9b4dde03b012-v1784122757378878600:12976-s12976-356x356-370` | 356×356 | 126736 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-356x356-342` | 356×356 | 126736 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-447x275-259` | 447×275 | 122925 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2381e37cf085a5a6-v1785369435180652900:130823-s130823-325x363-345` | 325×363 | 117975 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-342x342-195` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-342x342-197` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-342x342-199` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-342x342-195` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-342x342-197` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-342x342-199` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-342x342-209` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-342x342-256` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-342x342-256` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-342x342-255` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-342x342-223` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-342x342-209` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-342x342-195` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-342x342-197` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-342x342-199` | 342×342 | 116964 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-284x403-307` | 284×403 | 114452 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-284x403-308` | 284×403 | 114452 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-284x403-307` | 284×403 | 114452 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-448x252-260` | 448×252 | 112896 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-447x250-263` | 447×250 | 111750 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-447x250-264` | 447×250 | 111750 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-447x250-263` | 447×250 | 111750 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-447x250-264` | 447×250 | 111750 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-250x445-320` | 250×445 | 111250 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-250x445-320` | 250×445 | 111250 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-249x444-316` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-249x444-317` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-249x444-319` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-249x444-318` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-249x444-319` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-249x444-318` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-249x444-316` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-249x444-317` | 249×444 | 110556 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2381e37cf085a5a6-v1785369435180652900:130823-s130823-306x340-257` | 306×340 | 104040 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2381e37cf085a5a6-v1785369435180652900:130823-s130823-306x340-257` | 306×340 | 104040 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-324x320-193` | 324×320 | 103680 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-324x320-193` | 324×320 | 103680 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-324x320-193` | 324×320 | 103680 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-299x342-258` | 299×342 | 102258 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-299x342-258` | 299×342 | 102258 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-268x379-337` | 268×379 | 101572 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-245x354-295` | 245×354 | 86730 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-245x354-295` | 245×354 | 86730 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-245x354-295` | 245×354 | 86730 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-245x354-295` | 245×354 | 86730 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-219x389-341` | 219×389 | 85191 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-219x389-341` | 219×389 | 85191 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-241x342-246` | 241×342 | 82422 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-241x342-246` | 241×342 | 82422 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-241x342-218` | 241×342 | 82422 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-237x342-194` | 237×342 | 81054 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-237x342-194` | 237×342 | 81054 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-237x342-194` | 237×342 | 81054 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-213x380-335` | 213×380 | 80940 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-203x362-311` | 203×362 | 73486 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-203x362-315` | 203×362 | 73486 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-203x362-311` | 203×362 | 73486 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-203x362-315` | 203×362 | 73486 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-202x363-312` | 202×363 | 73326 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-202x363-313` | 202×363 | 73326 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-202x363-314` | 202×363 | 73326 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-202x363-312` | 202×363 | 73326 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-202x363-313` | 202×363 | 73326 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-202x363-314` | 202×363 | 73326 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-202x360-343` | 202×360 | 72720 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-202x360-343` | 202×360 | 72720 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-280x246-239` | 280×246 | 68880 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-280x246-239` | 280×246 | 68880 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-196x350-309` | 196×350 | 68600 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-196x350-309` | 196×350 | 68600 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-192x342-244` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-192x342-251` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-192x342-254` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-192x342-244` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-192x342-224` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-192x342-225` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-192x342-226` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-192x342-227` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-192x342-228` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-192x342-229` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-192x342-230` | 192×342 | 65664 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c19916a1a2d652a8-v1784122849445675900:107370-s107370-256x256-281` | 256×256 | 65536 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-190x342-220` | 190×342 | 64980 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-190x342-222` | 190×342 | 64980 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-190x342-221` | 190×342 | 64980 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-172x374-339` | 172×374 | 64328 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-172x374-340` | 172×374 | 64328 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-172x374-339` | 172×374 | 64328 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-172x374-340` | 172×374 | 64328 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-166x361-303` | 166×361 | 59926 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-166x361-304` | 166×361 | 59926 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-166x361-303` | 166×361 | 59926 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-166x361-304` | 166×361 | 59926 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-165x362-305` | 165×362 | 59730 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-165x362-305` | 165×362 | 59730 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-161x351-306` | 161×351 | 56511 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-161x351-306` | 161×351 | 56511 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-157x342-211` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-157x342-212` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-157x342-210` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-157x342-273` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-60c9db7c62b48f6-v1784110708116041000:38184619-s38184619-157x342-270` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-157x342-273` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-60c9db7c62b48f6-v1784110708116041000:38184619-s38184619-157x342-270` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-157x342-211` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-157x342-212` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-157x342-210` | 157×342 | 53694 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-55765f1b63b5a6c3-v1789762930268619800:6044312-s6044312-133x100-200` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-15183ee136fdca0a-v1789763015826454800:18770107-s18770107-133x100-201` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-2a07565336cd56c-v1789763020768943900:20447938-s20447938-133x100-203` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-1494bd6f0404e6e8-v1789763026823176300:15240372-s15240372-133x100-208` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-aa89a18d17d3443-v1789763032050824600:12324740-s12324740-133x100-202` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-a2946c6d6b8feef6-v1789764029739910100:3070197-s3070197-133x100-204` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-2a29d907fb2a9733-v1789764034293636700:3205547-s3205547-133x100-205` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-6f012a77922cd459-v1789764038520512000:3459806-s3459806-133x100-206` | 133×100 | 13300 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-f90fba003ed52a20-v1789764042524487100:5449170-s5449170-133x100-207` | 133×100 | 13300 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ?size=32&dpr=1.75&revision=3&strokeWidth=1.5&color=%23ffffffff` | 98×98 | 9604 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ?size=32&dpr=1.75&revision=3&strokeWidth=1.5&color=%23ffffffff` | 98×98 | 9604 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ?size=32&dpr=1.75&revision=3&strokeWidth=1.5&color=%23ffffffff` | 98×98 | 9604 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ?size=32&dpr=1.75&revision=3&strokeWidth=1.5&color=%23ffffffff` | 98×98 | 9604 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ?size=32&dpr=1.75&revision=3&strokeWidth=1.5&color=%23ffffffff` | 98×98 | 9604 |
| `image://f4icons/lucide/cGxheS1maWxsZWQ?size=32&dpr=1.75&revision=3&strokeWidth=1.5&color=%23ffffffff` | 98×98 | 9604 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-5-598633810b77414-v1788039817491105100:335913-s335913-89x101-198` | 89×101 | 8989 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-4ee3957913d1488b-v1788801297632101500:9518093-s9518093-70x100-285` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-4271734d63ada2f5-v1788801485878858700:6587624-s6587624-70x100-284` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-d94ecba6dfb086e6-v1788801483934874300:9301848-s9301848-70x100-286` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-ecddcf07cac4b9d6-v1788801487216597000:9342952-s9342952-70x100-287` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-2b8de27f9c5ca82f-v1788801490159292400:8739863-s8739863-70x100-289` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-40b553b147a0577d-v1788801485345095900:7307765-s7307765-70x100-288` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-82a5f66e19c397fc-v1788803017664872000:9585656-s9585656-70x100-290` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-f977f784798d29dd-v1788803013847689700:10645867-s10645867-70x100-292` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-9fe61eedc3178db9-v1788803008943152700:9721557-s9721557-70x100-291` | 70×100 | 7000 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-5ff985280a7dd490-v1786213748932635500:16021-s16021-91x57-80` | 91×57 | 5187 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-b5bbafae2754558-v1789663522977082300:78892-s78892-62x75-103` | 62×75 | 4650 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-97c71f239501eeca-v1789561835804388900:79029-s79029-55x76-77` | 55×76 | 4180 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-3e5f5f9a54464693-v1789561835955110900:1271736-s1271736-55x76-82` | 55×76 | 4180 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-7d692eb3c671f1f-v1789675970464542200:922483-s922483-42x77-89` | 42×77 | 3234 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-2828bf452c39d00f-v1789676116805322500:803811-s803811-42x77-91` | 42×77 | 3234 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-14449933ffa2b9fd-v1789676116226932600:781744-s781744-42x77-92` | 42×77 | 3234 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-904d8d4c92e032f4-v1786015835416220900:220683-s220683-42x76-79` | 42×76 | 3192 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-3fb6ea7f0361dc9e-v1789675967518861100:712911-s712911-41x75-85` | 41×75 | 3075 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-9a57b6310b83c5c-v1789675967779179600:684030-s684030-41x75-86` | 41×75 | 3075 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-aa0373d161eff005-v1789675970168780700:798633-s798633-41x75-88` | 41×75 | 3075 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-10f7e507d3c54090-v1789676115981693100:705952-s705952-41x75-94` | 41×75 | 3075 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-d2aaa0e00a732a0f-v1789676116108317700:669249-s669249-41x75-93` | 41×75 | 3075 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-17-21dbf164327f00c7-v1789676115819504300:667912-s667912-41x75-95` | 41×75 | 3075 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-8a8b223799f31b2-v1786015839413505900:6096522-s6096522-35x77-87` | 35×77 | 2695 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-7de443931ea86adf-v1783125166391964800:445399-s445399-35x76-84` | 35×76 | 2660 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-cdaed73cd2401f6b-v1789663674222815100:6124633-s6124633-34x77-106` | 34×77 | 2618 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-7f7c8a4f00b1afcc-v1789663667539136100:6729476-s6729476-34x77-109` | 34×77 | 2618 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-c563290fb7b3b7dc-v1789663669832290100:7916733-s7916733-34x77-111` | 34×77 | 2618 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-41b53b4ad3493694-v1789663892638517500:1212099-s1212099-34x76-104` | 34×76 | 2584 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-78d5331c06967f1d-v1789663844464909700:993715-s993715-34x76-114` | 34×76 | 2584 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-8729258a1875f20d-v1789663847121523300:1128765-s1128765-34x76-115` | 34×76 | 2584 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-bd453b5192597c4e-v1786015834069471400:2140284-s2140284-34x75-83` | 34×75 | 2550 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-5310ae27ca68dd75-v1786015828897829200:7439941-s7439941-34x75-90` | 34×75 | 2550 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-f64034dab7101cc9-v1789663668931525900:6760111-s6760111-33x75-108` | 33×75 | 2475 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-21-f7ca98587ed70342-v1789663670604813500:7517938-s7517938-33x75-110` | 33×75 | 2475 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-left-folder-11-9ae34f151eadf006-v1787656176088238000:4876-s4876-90x21-81` | 90×21 | 1890 |
| `image://f4icons/lucide/Z3JpZC0zeDM?size=16&dpr=1.75&revision=3&color=%23ffd7e0ea` | 28×28 | 784 |

## Top 30 inclusive hotspots

| # | ms | Count | Average ms | ms/frame | Type | Source | Detail |
|---|---:|---:|---:|---:|---|---|---|
| 1 | 213.26 | 1360 | 0.157 | 0.167 | Binding | [GalleryEntryPreview.qml:138](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:138) |  |
| 2 | 206.59 | 1006 | 0.205 | 0.161 | Binding | [GalleryEntryDelegateBase.qml:305](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:305) |  |
| 3 | 196.10 | 988 | 0.198 | 0.153 | Binding | [GalleryEntryPreview.qml:173](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:173) |  |
| 4 | 151.64 | 1284 | 0.118 | 0.118 | HandlingSignal | [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) |  |
| 5 | 150.43 | 1284 | 0.117 | 0.118 | Javascript | [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) | onContentYChanged |
| 6 | 128.05 | 1322 | 0.097 | 0.100 | Creating | [GalleryEntryPreview.qml:164](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:164) | QtQuick.Controls.impl/IconImage |
| 7 | 126.75 | 988 | 0.128 | 0.099 | Binding | [GalleryEntryPreview.qml:83](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:83) |  |
| 8 | 73.17 | 1287 | 0.057 | 0.057 | Binding | [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) |  |
| 9 | 72.80 | 6 | 12.134 | 0.057 | Javascript | [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105) | endPresentationStateUpdate |
| 10 | 72.80 | 6 | 12.133 | 0.057 | Javascript | [GalleryViewportController.qml:476](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:476) | endPresentationStateUpdate |
| 11 | 69.02 | 3 | 23.007 | 0.054 | HandlingSignal | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) |  |
| 12 | 68.69 | 3 | 22.897 | 0.054 | Javascript | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) | onCompactPresentationChanged |
| 13 | 68.68 | 3 | 22.894 | 0.054 | Javascript | [ShellSceneStore.qml:443](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:443) | applyCompactPatch |
| 14 | 63.18 | 1239 | 0.051 | 0.049 | Binding | [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) |  |
| 15 | 60.75 | 4 | 15.188 | 0.047 | Javascript | [RetainedGalleryPanelContent.qml:61](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:61) | refreshDescriptor |
| 16 | 59.33 | 340 | 0.174 | 0.046 | Creating | [GalleryViewport.qml:384](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:384) | GalleryEntryDelegateBase |
| 17 | 58.92 | 680 | 0.087 | 0.046 | Creating | [GalleryEntryPreview.qml:211](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:211) | QtQuick/Loader |
| 18 | 58.20 | 680 | 0.086 | 0.045 | Creating | [GalleryEntryDelegateBase.qml:8](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:8) | ZoinGallery.Native/BrickItem |
| 19 | 55.17 | 3 | 18.389 | 0.043 | Javascript | [GalleryPanelHostAdapter.qml:264](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:264) | applyRendererStateTo |
| 20 | 54.62 | 680 | 0.080 | 0.043 | Creating | [GalleryEntryDelegateBase.qml:311](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:311) | QtQuick/Loader |
| 21 | 37.33 | 2 | 18.664 | 0.029 | Binding | [PanelPairSurface.qml:90](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:90) |  |
| 22 | 37.07 | 2 | 18.534 | 0.029 | HandlingSignal | [RetainedGalleryPanelContent.qml:44](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:44) |  |
| 23 | 37.06 | 2 | 18.532 | 0.029 | Javascript | [RetainedGalleryPanelContent.qml:44](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:44) | onLayoutStateChanged |
| 24 | 34.94 | 1419 | 0.025 | 0.027 | Binding | [GalleryEntryPreview.qml:87](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:87) |  |
| 25 | 34.74 | 740 | 0.047 | 0.027 | Binding | [GalleryEntryDelegateBase.qml:51](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:51) |  |
| 26 | 34.26 | 2 | 17.131 | 0.027 | Binding | [RetainedGalleryPanelContent.qml:78](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:78) |  |
| 27 | 33.99 | 2 | 16.995 | 0.027 | HandlingSignal | [GalleryPanelHost.qml:377](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:377) |  |
| 28 | 33.99 | 2 | 16.994 | 0.027 | Javascript | [GalleryPanelHost.qml:377](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:377) | expression for onLayoutStateChanged |
| 29 | 33.98 | 2 | 16.992 | 0.027 | Javascript | [GalleryPanelHostAdapter.qml:442](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:442) | synchronizeLayout |
| 30 | 31.07 | 2 | 15.534 | 0.024 | Javascript | [PanelsSurface.qml:46](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:46) | updatePanelDescriptor |

## Source analysis

### [GalleryEntryPreview.qml:138](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:138)

```qml
                                                : Math.min(width, height)
        readonly property url rasterSource: !Window.window ? "" : lucideSource
            ? preview.entry.sourceColorIconAtSize(modelIconSource, rasterExtent, effectiveIconColor)
            : (systemFileSource ? preview.entry.systemFileFallbackSource(rasterExtent) : "")
        sourceComponent: useAspectRaster ? aspectRasterComponent : tintedRasterComponent
    }

```

This presentation transaction includes nested layout/delegate work during mode setup. Do not attribute its entire inclusive time to steady scrolling. Investigate how many leaves are recreated and whether state propagation can be consolidated.

### [GalleryEntryDelegateBase.qml:305](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:305)

```qml
        shaderThumbnail: entry.masonryMode || entry.gridMode
        activePresentation: entry.visualGeometryReady && !entry.visualIdentityChanging
            && (!entry.folderPreviewRequested
            || entry.visualModel.folderPreviewState !== 2)
        z: 1
    }

```

Delegate creation and geometry/alignment bindings remain active. Preserve compatible visual leaves and trim invalidations without weakening scene-space alignment at rest.

### [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72)

```qml

            function onContentYChanged() {
                const extent = chromeRoot.layout.contentHeight
                verticalScroll.position = extent > 0
                        ? chromeRoot.layout.contentY / extent : 0
            }
            function onContentHeightChanged() { verticalScroll.updateSize() }
```

Scrollbar notifications continue on the scroll path. Gate invisible-axis computations and consolidate forwarding after the larger rendering/delegate costs are addressed.

### [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105)

```qml

    function endPresentationStateUpdate(publishVisibleRange) {
        panelFacade.viewportController.endPresentationStateUpdate(publishVisibleRange)
    }

    function restoreScrollOffset() {
        return panelFacade.viewportController.restoreScrollOffset()
```

This presentation transaction includes nested layout/delegate work during mode setup. Do not attribute its entire inclusive time to steady scrolling. Investigate how many leaves are recreated and whether state propagation can be consolidated.

### [GalleryViewportController.qml:476](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:476)

```qml

    function endPresentationStateUpdate(publishVisibleRange) {
        galleryLayout.endLayoutUpdate()
        if (presentationStateUpdateDepth > 0)
            presentationStateUpdateDepth -= 1
        if (presentationStateUpdateDepth > 0)
            return
```

This presentation transaction includes nested layout/delegate work during mode setup. Do not attribute its entire inclusive time to steady scrolling. Investigate how many leaves are recreated and whether state propagation can be consolidated.

## Next targets

Reduce delegate/media-leaf churn and render-batch rebuilding, then isolate Details render CPU. Preserve the current cache and resting pixel-grid checks. A focused qt-qml-review of GalleryEntryPreview.qml and GalleryEntryDelegateBase.qml can check lifecycle/binding changes before implementation.

> AI assistance has been used to create this output.