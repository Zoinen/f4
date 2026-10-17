# Downloads scrolling: Grid

Scrolling misses the 160 Hz budget; vector-scene construction and rendering are the principal targets.

Full QML trace: [grid](D:/Code/f4-zoin/artifacts/modes-profile/qml-grid.qtd). Profile mode: full. Parser active-time estimate: 4000.00 ms; this is not capture wall time. Native frame statistics below use the complete warm downward sweep, without QML event recording.

160 Hz budget: 6.25 ms. Native frame-swap intervals: p50 6.24, p95 12.88, p99 24.29, maximum 74.62 ms; 46/568 intervals ≥9.375 ms, 2 >33 ms.

Isolated scrolling QML exclusive time: 0.757 ms/frame. Clock correlation p95 error: 0.0158 ms. Masonry correlation has up to one frame of uncertainty; do not attribute individual Masonry spikes precisely.

## Full-trace event summary

Inclusive/nested totals overlap and include mode setup; they are not independent frame costs.

| Event | Count | Inclusive ms | ms/frame |
|---|---:|---:|---:|
| Javascript | 176713 | 4789.87 | 7.484 |
| Binding | 111239 | 1933.74 | 3.021 |
| HandlingSignal | 3858 | 1470.04 | 2.297 |
| Creating | 20968 | 605.42 | 0.946 |
| Compiling | 30 | 47.31 | 0.074 |

QML animation counters describe animation activity, not actual presented-frame intervals. Native frame swaps are the pacing measure used here.

## Memory

Allocation churn is visible; this short trace does not establish a leak. JS objects: 231,608 allocations, 42.82 MiB allocated, 54.4% reclaimed, 19.50 MiB live at capture end. Peak GC heap pages: 34.71 MiB.

| Category | Allocations | Total allocated | Reclaimed | Peak live | Live at exit |
|---|---:|---:|---:|---:|---:|
| GC heap pages | 137 | 34.71 MiB | 1.11 MiB | 34.71 MiB | 33.60 MiB |
| Small JS objects | 231,608 | 42.82 MiB | 23.31 MiB | 34.64 MiB | 19.50 MiB |

GC heap pages back the QV4 allocator; small/large JS objects are allocations within its object accounting. These categories must not be added together. All-zero categories are omitted.

## Pixmap cache

Load requests: 592; loaded: 504; removed: 197. These include the whole window and mode setup. Raster pixmaps do not account for vector scene geometry. Dimensions below permit checking oversized loads against each surface; source/native frames are not automatically wasteful.

| Source | Dimensions | Pixels |
|---|---:|---:|
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-221d25a0ceeabe30-v1785516399377774300:48469-s48469-338x338-313` | 338×338 | 114244 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-221d25a0ceeabe30-v1785516399377774300:48469-s48469-338x338-313` | 338×338 | 114244 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b95a9b4dde03b012-v1784122757378878600:12976-s12976-338x338-315` | 338×338 | 114244 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c19916a1a2d652a8-v1784122849445675900:107370-s107370-256x256-314` | 256×256 | 65536 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 226×283 | 63958 |
| `qrc:/F4QtHost/icons/zoin/text.svg` | 226×283 | 63958 |
| `qrc:/F4QtHost/icons/zoin/table.svg` | 226×283 | 63958 |
| `qrc:/F4QtHost/icons/zoin/html.svg` | 226×226 | 51076 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 226×226 | 51076 |
| `qrc:/F4QtHost/icons/zoin/default.svg` | 198×248 | 49104 |
| `qrc:/F4QtHost/icons/zoin/table.svg` | 198×248 | 49104 |
| `qrc:/F4QtHost/icons/zoin/json.svg` | 226×213 | 48138 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 226×181 | 40906 |
| `qrc:/ZoinGallery/resources/FileIcon.svg` | 198×198 | 39204 |
| `qrc:/F4QtHost/icons/zoin/folder-up.svg` | 198×158 | 31284 |
| `qrc:/F4QtHost/icons/zoin/folder.svg` | 198×158 | 31284 |
| `qrc:/F4QtHost/icons/zoin/zip.svg` | 198×158 | 31284 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-197x111-115` | 197×111 | 21867 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-197x111-115` | 197×111 | 21867 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-197x110-124` | 197×110 | 21670 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-197x110-124` | 197×110 | 21670 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-197x110-125` | 197×110 | 21670 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-197x110-125` | 197×110 | 21670 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2ef176d5acb84b0b-v1789051904603847600:1958221-s1958221-196x110-129` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2ef176d5acb84b0b-v1789051904603847600:1958221-s1958221-196x110-129` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-196x110-130` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-196x110-130` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-196x110-131` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-196x110-131` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb704ebd5f003655-v1789051989722368100:799344-s799344-196x110-132` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb704ebd5f003655-v1789051989722368100:799344-s799344-196x110-132` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-358422fb44357a4f-v1789051992780984400:795683-s795683-196x110-133` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-358422fb44357a4f-v1789051992780984400:795683-s795683-196x110-133` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-196x110-134` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-196x110-134` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1246daafdad65e23-v1789052005136169600:777537-s777537-196x110-135` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1246daafdad65e23-v1789052005136169600:777537-s777537-196x110-135` | 196×110 | 21560 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-196x108-111` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-acafa8327f8b8b33-v1789051982300703000:674207-s674207-196x108-136` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1a365f3a7cf06a6d-v1789051982469777600:687802-s687802-196x108-137` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-dd6d7289823242b0-v1789051983202534600:690087-s690087-196x108-138` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-893713db778ff93d-v1789052183361837400:679716-s679716-196x108-139` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e4f4b0f8b1590147-v1789052181430938300:533790-s533790-196x108-140` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e149e1f8ee7d06f6-v1789052181495278500:656823-s656823-196x108-141` | 196×108 | 21168 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-182x112-116` | 182×112 | 20384 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-175x112-62` | 175×112 | 19600 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-175x112-62` | 175×112 | 19600 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-148x112-122` | 148×112 | 16576 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-148x112-123` | 148×112 | 16576 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-148x112-126` | 148×112 | 16576 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-127x112-39` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-127x112-40` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-127x112-41` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-127x112-42` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-127x112-43` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-127x112-44` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-127x112-45` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-127x112-46` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-127x112-47` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-127x112-48` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-127x112-49` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-127x112-50` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-127x112-51` | 127×112 | 14224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-112x112-118` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-112x112-119` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2381e37cf085a5a6-v1785369435180652900:130823-s130823-100x112-120` | 100×112 | 11200 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-98x112-121` | 98×112 | 10976 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-79x112-61` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-79x112-61` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-63x112-114` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-63x112-117` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-5-598633810b77414-v1788039817491105100:335913-s335913-76x86-222` | 76×86 | 6536 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-51x112-112` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-51x112-113` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-51x112-127` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-60c9db7c62b48f6-v1784110708116041000:38184619-s38184619-51x112-128` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-55765f1b63b5a6c3-v1789762930268619800:6044312-s6044312-71x54-316` | 71×54 | 3834 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-15183ee136fdca0a-v1789763015826454800:18770107-s18770107-71x54-317` | 71×54 | 3834 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-5ff985280a7dd490-v1786213748932635500:16021-s16021-68x43-56` | 68×43 | 2924 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-2a07565336cd56c-v1789763020768943900:20447938-s20447938-55x42-27` | 55×42 | 2310 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-7-1494bd6f0404e6e8-v1789763026823176300:15240372-s15240372-55x42-28` | 55×42 | 2310 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-21-b5bbafae2754558-v1789663522977082300:78892-s78892-35x41-67` | 35×41 | 1435 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-97c71f239501eeca-v1789561835804388900:79029-s79029-31x43-53` | 31×43 | 1333 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-3e5f5f9a54464693-v1789561835955110900:1271736-s1271736-30x42-55` | 30×42 | 1260 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-4ee3957913d1488b-v1788801297632101500:9518093-s9518093-29x42-57` | 29×42 | 1218 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-4271734d63ada2f5-v1788801485878858700:6587624-s6587624-29x42-58` | 29×42 | 1218 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-d94ecba6dfb086e6-v1788801483934874300:9301848-s9301848-29x42-59` | 29×42 | 1218 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-15-ecddcf07cac4b9d6-v1788801487216597000:9342952-s9342952-29x42-60` | 29×42 | 1218 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-11-904d8d4c92e032f4-v1786015835416220900:220683-s220683-24x44-54` | 24×44 | 1056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-17-3fb6ea7f0361dc9e-v1789675967518861100:712911-s712911-24x43-63` | 24×43 | 1032 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-17-9a57b6310b83c5c-v1789675967779179600:684030-s684030-24x43-64` | 24×43 | 1032 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-17-aa0373d161eff005-v1789675970168780700:798633-s798633-24x43-65` | 24×43 | 1032 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-17-7d692eb3c671f1f-v1789675970464542200:922483-s922483-24x43-66` | 24×43 | 1032 |
| `qrc:/F4QtHost/icons/zoin/svg.svg` | 32×32 | 1024 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-28x28-83` | 28×28 | 784 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-28x28-82` | 28×28 | 784 |
| `image://f4icons/lucide/Z3JpZC0zeDM?size=16&dpr=1.75&revision=3&color=%23ffd7e0ea` | 28×28 | 784 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-21-41b53b4ad3493694-v1789663892638517500:1212099-s1212099-18x42-68` | 18×42 | 756 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-21-f64034dab7101cc9-v1789663668931525900:6760111-s6760111-18x42-69` | 18×42 | 756 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-folder-21-cdaed73cd2401f6b-v1789663674222815100:6124633-s6124633-18x42-70` | 18×42 | 756 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2381e37cf085a5a6-v1785369435180652900:130823-s130823-25x28-80` | 25×28 | 700 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-24x28-81` | 24×28 | 672 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-27x17-79` | 27×17 | 459 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-15x28-74` | 15×28 | 420 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-15x28-84` | 15×28 | 420 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-893713db778ff93d-v1789052183361837400:679716-s679716-27x15-101` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-27x15-71` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-26x15-93` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-26x15-97` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1246daafdad65e23-v1789052005136169600:777537-s777537-26x15-100` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-26x15-77` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-12x28-105` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-12x28-72` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-12x28-73` | 12×28 | 336 |

## Full-trace top 30 inclusive hotspots

| # | ms | Count | Average ms | ms/frame | Type | Source | Detail |
|---|---:|---:|---:|---:|---|---|---|
| 1 | 447.42 | 6 | 74.570 | 0.699 | Javascript | [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105) | endPresentationStateUpdate |
| 2 | 447.41 | 6 | 74.568 | 0.699 | Javascript | [GalleryViewportController.qml:476](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:476) | endPresentationStateUpdate |
| 3 | 380.25 | 2 | 190.126 | 0.594 | HandlingSignal | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) |  |
| 4 | 380.15 | 2 | 190.073 | 0.594 | Javascript | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) | onCompactPresentationChanged |
| 5 | 380.14 | 2 | 190.071 | 0.594 | Javascript | [ShellSceneStore.qml:443](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:443) | applyCompactPatch |
| 6 | 375.80 | 3 | 125.265 | 0.587 | Javascript | [RetainedGalleryPanelContent.qml:61](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:61) | refreshDescriptor |
| 7 | 364.60 | 3 | 121.534 | 0.570 | Javascript | [GalleryPanelHostAdapter.qml:264](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:264) | applyRendererStateTo |
| 8 | 258.04 | 2 | 129.021 | 0.403 | Binding | [PanelPairSurface.qml:90](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:90) |  |
| 9 | 257.79 | 2 | 128.893 | 0.403 | HandlingSignal | [RetainedGalleryPanelContent.qml:44](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:44) |  |
| 10 | 257.78 | 2 | 128.891 | 0.403 | Javascript | [RetainedGalleryPanelContent.qml:44](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:44) | onLayoutStateChanged |
| 11 | 248.64 | 2 | 124.322 | 0.388 | Binding | [RetainedGalleryPanelContent.qml:78](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:78) |  |
| 12 | 248.34 | 2 | 124.170 | 0.388 | HandlingSignal | [GalleryPanelHost.qml:377](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:377) |  |
| 13 | 248.34 | 2 | 124.169 | 0.388 | Javascript | [GalleryPanelHost.qml:377](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:377) | expression for onLayoutStateChanged |
| 14 | 248.34 | 2 | 124.168 | 0.388 | Javascript | [GalleryPanelHostAdapter.qml:442](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:442) | synchronizeLayout |
| 15 | 153.20 | 372 | 0.412 | 0.239 | Creating | [GalleryEntryPreview.qml:211](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:211) | QtQuick/Loader |
| 16 | 130.90 | 36 | 3.636 | 0.205 | Creating | [GalleryVectorImage.qml:86](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVectorImage.qml:86) | QtQuick.VectorImage/VectorImage |
| 17 | 123.07 | 1274 | 0.097 | 0.192 | Binding | [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) |  |
| 18 | 121.95 | 1 | 121.955 | 0.191 | HandlingSignal | [PanelsSurface.qml:74](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:74) |  |
| 19 | 121.95 | 1 | 121.953 | 0.191 | Javascript | [PanelsSurface.qml:74](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:74) | onRightPanelPresentationOverrideChanged |
| 20 | 121.95 | 1 | 121.951 | 0.191 | Javascript | [PanelsSurface.qml:46](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:46) | updatePanelDescriptor |
| 21 | 121.53 | 3 | 40.511 | 0.190 | Binding | [RetainedGalleryPanelContent.qml:76](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:76) |  |
| 22 | 121.50 | 1 | 121.501 | 0.190 | Binding | [PanelPairSurface.qml:89](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:89) |  |
| 23 | 120.57 | 3 | 40.189 | 0.188 | Javascript | [GalleryPanel.qml:578](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:578) | applyPresentationMode |
| 24 | 120.27 | 3 | 40.091 | 0.188 | Javascript | [GalleryViewport.qml:45](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:45) | applyPresentationState |
| 25 | 118.03 | 1 | 118.034 | 0.184 | HandlingSignal | [RetainedGalleryPanelContent.qml:43](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:43) |  |
| 26 | 118.03 | 1 | 118.033 | 0.184 | Javascript | [RetainedGalleryPanelContent.qml:43](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:43) | onPanelChanged |
| 27 | 118.03 | 1 | 118.030 | 0.184 | Javascript | [RetainedGalleryPanelContent.qml:17](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:17) | activatePanel |
| 28 | 117.21 | 3 | 39.069 | 0.183 | HandlingSignal | [GalleryPanel.qml:671](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:671) |  |
| 29 | 117.20 | 3 | 39.067 | 0.183 | Javascript | [GalleryPanel.qml:671](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:671) | expression for onPresentationModeChanged |
| 30 | 116.45 | 3 | 38.817 | 0.182 | HandlingSignal | [GalleryPanelHost.qml:373](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:373) |  |

## Isolated scrolling hotspots

| Source | Detail | Count | Exclusive ms | Maximum inclusive ms |
|---|---|---:|---:|---:|
| [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) | onContentYChanged | 573 | 72.92 | 0.40 |
| [GalleryVectorImage.qml:86](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVectorImage.qml:86) | QtQuick.VectorImage/VectorImage | 18 | 40.51 | 17.50 |
| [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) | expression for iconSceneOrigin | 16080 | 26.34 | 0.06 |
| :0 |  | 9 | 26.07 | 5.88 |
| [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) |  | 573 | 24.76 | 0.31 |
| [GalleryEntryPreview.qml:164](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:164) | QtQuick.Controls.impl/IconImage | 416 | 19.87 | 0.66 |
| [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) |  | 16080 | 18.58 | 0.86 |
| [GalleryEntryPreview.qml:173](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:173) |  | 312 | 14.90 | 0.46 |
| [GalleryEntryDelegateBase.qml:185](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:185) |  | 312 | 7.61 | 0.32 |
| [GalleryPanel.qml:316](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:316) |  | 573 | 6.91 | 0.04 |
| [GridEntryDelegate.qml:15](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GridEntryDelegate.qml:15) | QtQuick/Text | 220 | 5.38 | 0.15 |
| [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) |  | 573 | 5.20 | 0.09 |
| [GalleryCursorController.qml:111](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:111) | indexIntersectsViewport | 573 | 5.15 | 0.06 |
| [GalleryCursorController.qml:88](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:88) |  | 573 | 4.46 | 0.07 |
| [GalleryEntryDelegateBase.qml:8](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:8) | ZoinGallery.Native/BrickItem | 208 | 3.68 | 0.35 |
| [GalleryEntryPreview.qml:240](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:240) | QtQuick/Image | 132 | 3.59 | 0.09 |
| [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349) | expression for onContentYChanged | 573 | 3.58 | 0.16 |
| [GalleryEntryPreview.qml:285](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:285) | QtQuick/Text | 208 | 3.37 | 0.15 |
| [GalleryCursorController.qml:176](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:176) | updateVisualCursorForViewport | 573 | 2.88 | 0.06 |
| [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) | expression for model | 573 | 2.80 | 0.02 |

## Source analysis

### [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72)

```qml

            function onContentYChanged() {
                const extent = chromeRoot.layout.contentHeight
                verticalScroll.position = extent > 0
                        ? chromeRoot.layout.contentY / extent : 0
            }
            function onContentHeightChanged() { verticalScroll.updateSize() }
```

Scrollbar forwarding and position bindings execute throughout scrolling, including computations for hidden scrollbars. Gate irrelevant work and consolidate forwarding; this is a secondary cost.

### [GalleryVectorImage.qml:86](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVectorImage.qml:86)

```qml
    }
    VectorImage {
        id: vectorImage
        objectName: "galleryVectorSurface"
        x: Math.round((root.width - width) * root.devicePixelRatio / 2) / root.devicePixelRatio
        y: Math.round((root.height - height) * root.devicePixelRatio / 2) / root.devicePixelRatio
        width: Math.round(root.fittedWidth * root.devicePixelRatio) / root.devicePixelRatio
```

VectorImage constructs scalable scene geometry on entry. asynchronousShapes does not remove all synchronous source/scene construction. Prepare bounded nearby vector surfaces before they enter view and reuse them by immutable source revision; preserve direct vector rendering.

### [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76)

```qml
        && !panelRoot.cursorPixelAlignmentSuspended
    readonly property point iconSceneOrigin: {
        if (!iconPixelAlignmentActive || detailsMode)
            return Qt.point(0, 0)
        // mapToItem() does not register dependencies on ancestor geometry.
        // Track geometry through the complete chain so host
        // panel placement invalidates the correction without any scrolling.
```

The expensive scene-space alignment walk is already suspended during animation, but per-leaf bindings still reevaluate frequently. Reduce invalidation/subscriptions while retaining complete scene-space alignment at rest.

### [GalleryEntryPreview.qml:164](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:164)

```qml
        id: tintedRasterComponent
        IconImage {
            id: fallbackRaster
            objectName: preview.activePresentation
                        ? "galleryFallbackIcon-" + preview.entry.viewIndex : ""
            readonly property color effectiveIconColor:
                fallbackIcon.effectiveIconColor
```

Loader/media leaves are recreated as reusable delegates change identity. Preserve compatible leaves or amortize creation while preventing stale-source display and keeping acquisition priority.

### [GalleryPanel.qml:316](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:316)

```qml
        cursorState.cursorChromeTargetRect
    readonly property point cursorPixelGridViewportOrigin:
        cursorState.cursorPixelGridViewportOrigin
    readonly property bool cursorPixelAlignmentSuspended:
        cursorState.cursorPixelAlignmentSuspended

    GalleryCursorController {
```

Viewport-origin forwarding invalidates dependent visual bindings each frame. Share a minimal scroll-phase state and avoid notifying consumers that do not need continuous updates.

## Next steps

1. Bound and preprepare vector scene creation; reuse nearby surfaces by revision.
2. Reduce entering-row/column delegate and media-leaf churn, then measure render-batch rebuilding.
3. Investigate Details render-thread CPU and driver cost separately from QML.
4. Trim residual alignment notifications and hidden-scrollbar updates after the larger stalls.

A focused qt-qml-review of GalleryVectorImage.qml, GalleryEntryPreview.qml and GalleryEntryDelegateBase.qml can check lifecycle and binding changes before implementation.

See the master report for capture scope, native CPU evidence and renderer experiments.

> AI assistance has been used to create this output.