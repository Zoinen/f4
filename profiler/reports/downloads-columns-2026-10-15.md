# Downloads scrolling: Columns

Scrolling misses the 160 Hz budget; vector-scene construction and rendering are the principal targets.

Full QML trace: [columns](D:/Code/f4-zoin/artifacts/modes-profile/qml-columns.qtd). Profile mode: full. Parser active-time estimate: 4013.00 ms; this is not capture wall time. Native frame statistics below use the complete warm downward sweep, without QML event recording.

160 Hz budget: 6.25 ms. Native frame-swap intervals: p50 6.23, p95 11.08, p99 17.73, maximum 84.40 ms; 31/575 intervals ≥9.375 ms, 4 >33 ms.

Isolated scrolling QML exclusive time: 0.617 ms/frame. Clock correlation p95 error: 0.0170 ms. Masonry correlation has up to one frame of uncertainty; do not attribute individual Masonry spikes precisely.

## Full-trace event summary

Inclusive/nested totals overlap and include mode setup; they are not independent frame costs.

| Event | Count | Inclusive ms | ms/frame |
|---|---:|---:|---:|
| Javascript | 205903 | 5422.49 | 8.446 |
| Binding | 143434 | 1796.98 | 2.799 |
| HandlingSignal | 3429 | 1586.85 | 2.472 |
| Creating | 24505 | 704.19 | 1.097 |
| Compiling | 18 | 51.28 | 0.080 |

QML animation counters describe animation activity, not actual presented-frame intervals. Native frame swaps are the pacing measure used here.

## Memory

Allocation churn is visible; this short trace does not establish a leak. JS objects: 316,073 allocations, 41.17 MiB allocated, 41.8% reclaimed, 23.94 MiB live at capture end. Peak GC heap pages: 27.05 MiB.

| Category | Allocations | Total allocated | Reclaimed | Peak live | Live at exit |
|---|---:|---:|---:|---:|---:|
| GC heap pages | 59 | 27.05 MiB | 0.74 MiB | 27.05 MiB | 26.31 MiB |
| Small JS objects | 316,073 | 41.17 MiB | 17.22 MiB | 27.04 MiB | 23.94 MiB |

GC heap pages back the QV4 allocator; small/large JS objects are allocations within its object accounting. These categories must not be added together. All-zero categories are omitted.

## Pixmap cache

Load requests: 160; loaded: 158; removed: 8. These include the whole window and mode setup. Raster pixmaps do not account for vector scene geometry. Dimensions below permit checking oversized loads against each surface; source/native frames are not automatically wasteful.

| Source | Dimensions | Pixels |
|---|---:|---:|
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-646x356-281` | 646×356 | 229976 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-590x378-284` | 590×378 | 223020 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-477x418-263` | 477×418 | 199386 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-432x378-265` | 432×378 | 163296 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-432x378-266` | 432×378 | 163296 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3524d55d2c72140e-v1784046257271230000:8896730-s8896730-430x378-268` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d518f2ebfd7d9858-v1784046250584785800:7924155-s7924155-430x378-269` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-34219f55058c6590-v1784046536858183200:22207648-s22207648-430x378-270` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6d80472d596eeb3e-v1784046543430051200:23449165-s23449165-430x378-271` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-30b07a601e68f26c-v1784046541813037100:22073270-s22073270-430x378-272` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fe8d674a8b0f948c-v1784046542164321000:21569251-s21569251-430x378-276` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f58a217345aaa026-v1784046539506353800:22088051-s22088051-430x378-280` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ab72794de88f22e1-v1784046460751070200:3345953-s3345953-430x378-273` | 430×378 | 162540 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ad51260f0603c0d4-v1784046261064991000:8450668-s8450668-429x377-264` | 429×377 | 161733 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-751dc0fd23ed32e2-v1784046256476180400:8008060-s8008060-429x377-267` | 429×377 | 161733 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-ed79727fb57ff5c1-v1784046251626275600:8028027-s8028027-409x358-274` | 409×358 | 146422 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-765c79d8dee7580d-v1784046255973426400:7608901-s7608901-409x358-277` | 409×358 | 146422 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b95a9b4dde03b012-v1784122757378878600:12976-s12976-338x338-315` | 338×338 | 114244 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-221d25a0ceeabe30-v1785516399377774300:48469-s48469-338x338-313` | 338×338 | 114244 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b95a9b4dde03b012-v1784122757378878600:12976-s12976-338x338-315` | 338×338 | 114244 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-234x417-260` | 234×417 | 97578 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-234x417-259` | 234×417 | 97578 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-234x417-258` | 234×417 | 97578 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-234x417-261` | 234×417 | 97578 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-234x417-262` | 234×417 | 97578 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-253x359-278` | 253×359 | 90827 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-207x369-285` | 207×369 | 76383 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-202x360-279` | 202×360 | 72720 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35917994ed6a0a0c-v1784045356275755500:98572-s98572-280x246-275` | 280×246 | 68880 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c19916a1a2d652a8-v1784122849445675900:107370-s107370-256x256-314` | 256×256 | 65536 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-189x337-257` | 189×337 | 63693 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-163x355-282` | 163×355 | 57865 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-163x355-283` | 163×355 | 57865 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-175x112-62` | 175×112 | 19600 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3b57924670087300-v1788455513456249400:8499105-s8499105-175x112-62` | 175×112 | 19600 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-56d46523b9f947ad-v1788723964680304400:26986-s26986-149x112-5` | 149×112 | 16688 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1767a7ce256e4da8-v1784068958903843400:1358552-s1358552-146x112-4` | 146×112 | 16352 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-35226e09609937e8-v1784046391174270000:13537769-s13537769-128x112-38` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d1f22cbf49b2990-v1784046400707440400:13195558-s13195558-128x112-36` | 128×112 | 14336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-18a789a6abf5a13c-v1784046394632663500:13259276-s13259276-128x112-37` | 128×112 | 14336 |
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
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6332eacf7409fb86-v1785446568636646300:138020-s138020-113x112-19` | 113×112 | 12656 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9be450bcdb6b34f-v1784681103848923300:254861-s254861-112x112-6` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8847696d975874c0-v1784681229234881400:1214684-s1214684-112x112-14` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-7b9b996deb5a5847-v1787578312912958800:1135103-s1135103-112x112-2` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b0ca93f3b430ccf2-v1787578652310440400:2723610-s2723610-112x112-7` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-db265e4d30c59784-v1785535483299059400:77453-s77453-112x112-17` | 112×112 | 12544 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-c7e054392918617-v1791814264851785800:436432-s436432-79x112-12` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-40f9d5481c62cde6-v1791814265159347100:324253-s324253-79x112-20` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-79x112-61` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e1a6dd28bb556873-v1791816474589389500:314015-s314015-79x112-61` | 79×112 | 8848 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7ce7722ef0998d7-v1785521202263723500:30378-s30378-77x112-18` | 77×112 | 8624 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-37e3f23d014420c9-v1784044904085362100:1825195-s1825195-63x112-35` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb354f88a3abca80-v1784067002024621500:1640853-s1640853-63x112-52` | 63×112 | 7056 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b253c63a49bab346-v1784045098486615200:1067668-s1067668-62x112-21` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b9dfed014b3063ac-v1784045053667629700:1123128-s1123128-62x112-9` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-24e18b39beff0fab-v1784045111239826500:1133061-s1133061-62x112-13` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-70a8ed88a936a63c-v1784044986429070600:437558-s437558-62x112-29` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8fa2cd6b81fb9da5-v1784045004431980300:570569-s570569-62x112-30` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f21665c1b739d11e-v1784045028810675500:596505-s596505-62x112-31` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-9b5eda74cf70a4-v1784044979481490400:548930-s548930-62x112-32` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6723138049a8edd3-v1784044977380512800:485804-s485804-62x112-33` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8ae9bcd8a0b5795a-v1784044979769918100:616496-s616496-62x112-34` | 62×112 | 6944 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f8527acefde5dc7b-v1784655557043099600:3741192-s3741192-62x110-22` | 62×110 | 6820 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-596e77beb32d0810-v1788201560897421900:5594121-s5594121-51x112-15` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2e5823c00b722c58-v1788201873801794200:3285308-s3285308-51x112-8` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a0104909214f3aeb-v1788202635380252700:825115-s825115-51x112-16` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-6e49f4077be8f1ef-v1788202753316886300:825115-s825115-51x112-11` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fbf310087e42ca5f-v1788202593111164800:825115-s825115-51x112-1` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2db92b043bd91fe6-v1788447618585546800:2740719-s2740719-51x112-10` | 51×112 | 5712 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d0cd3ba56ae87b4f-v1784233436917450100:2513656-s2513656-51x112-3` | 51×112 | 5712 |
| `qrc:/F4QtHost/icons/zoin/text.svg` | 36×45 | 1620 |
| `qrc:/F4QtHost/icons/zoin/html.svg` | 36×36 | 1296 |
| `qrc:/F4QtHost/icons/zoin/json.svg` | 36×34 | 1224 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-28x28-83` | 28×28 | 784 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-28x28-82` | 28×28 | 784 |
| `image://f4icons/lucide/Y29sdW1ucy0y?size=16&dpr=1.75&revision=3&color=%23ffd7e0ea` | 28×28 | 784 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e483c29faf3b6db2-v1789572330743896400:1325988-s1325988-28x28-83` | 28×28 | 784 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-48c36c9a2d417772-v1785448486978980300:30790-s30790-28x28-82` | 28×28 | 784 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2381e37cf085a5a6-v1785369435180652900:130823-s130823-25x28-80` | 25×28 | 700 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-f9d5c9a92566ea9c-v1785369437748362200:117235-s117235-24x28-81` | 24×28 | 672 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-23006e6f88ba9bac-v1786565933722006200:9302945-s9302945-27x21-86` | 27×21 | 567 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-13141bfcdc7cb9cc-v1786564757558397900:7126336-s7126336-27x21-85` | 27×21 | 567 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cd58567a32ff9556-v1786565933722006200:44882-s44882-27x21-90` | 27×21 | 567 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-27x17-79` | 27×17 | 459 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d5028b27c7baa217-v1785516969840189400:395230721-s395230721-27x17-79` | 27×17 | 459 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-15x28-84` | 15×28 | 420 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-15x28-74` | 15×28 | 420 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-255394c391babf03-v1784739102385224800:1644880-s1644880-15x28-74` | 15×28 | 420 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-a51a2b952948c627-v1786743380975636500:8561108-s8561108-15x28-84` | 15×28 | 420 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-27x15-71` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-d069df292a6980c7-v1789051907680071200:456828-s456828-27x15-71` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-acafa8327f8b8b33-v1789051982300703000:674207-s674207-27x15-99` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1a365f3a7cf06a6d-v1789051982469777600:687802-s687802-27x15-98` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-dd6d7289823242b0-v1789051983202534600:690087-s690087-27x15-102` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-893713db778ff93d-v1789052183361837400:679716-s679716-27x15-101` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e4f4b0f8b1590147-v1789052181430938300:533790-s533790-27x15-103` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-e149e1f8ee7d06f6-v1789052181495278500:656823-s656823-27x15-104` | 27×15 | 405 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-26x15-94` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-26x15-77` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-984c2e22aa8a029e-v1785516985488341600:745856157-s745856157-26x15-77` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-cb4d2ea17147aa57-v1786564674664732600:6444277-s6444277-26x15-91` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-908282ec06bbbc56-v1786564675195132200:6333629-s6333629-26x15-87` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-2ef176d5acb84b0b-v1789051904603847600:1958221-s1958221-26x15-92` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-8c7b6dc8a7f8dc51-v1789051990733052400:810982-s810982-26x15-94` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-216a4ab0cdf1f546-v1789051989726868500:809398-s809398-26x15-93` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-fb704ebd5f003655-v1789051989722368100:799344-s799344-26x15-95` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-358422fb44357a4f-v1789051992780984400:795683-s795683-26x15-96` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-695f9c6c65e46a7c-v1789052003684039800:785999-s785999-26x15-97` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1246daafdad65e23-v1789052005136169600:777537-s777537-26x15-100` | 26×15 | 390 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-60c9db7c62b48f6-v1784110708116041000:38184619-s38184619-12x28-106` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-12x28-105` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-12x28-72` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-12x28-73` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-b7a13496e35f7d46-v1784233917055999900:2295421-s2295421-12x28-72` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-1d997d947ccc91fe-v1791813179678280300:288267-s288267-12x28-73` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-3af9100ccc7cbe7e-v1783677820299848100:26477999-s26477999-12x28-105` | 12×28 | 336 |
| `image://f4-zoingallery-thumbnails/zg-thumb-f4-right-60c9db7c62b48f6-v1784110708116041000:38184619-s38184619-12x28-106` | 12×28 | 336 |

## Full-trace top 30 inclusive hotspots

| # | ms | Count | Average ms | ms/frame | Type | Source | Detail |
|---|---:|---:|---:|---:|---|---|---|
| 1 | 500.76 | 6 | 83.461 | 0.780 | Javascript | [GalleryPanelFacade.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelFacade.qml:105) | endPresentationStateUpdate |
| 2 | 500.76 | 6 | 83.460 | 0.780 | Javascript | [GalleryViewportController.qml:476](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewportController.qml:476) | endPresentationStateUpdate |
| 3 | 349.56 | 2 | 174.782 | 0.544 | HandlingSignal | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) |  |
| 4 | 349.41 | 2 | 174.707 | 0.544 | Javascript | [ShellSceneStore.qml:532](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:532) | onCompactPresentationChanged |
| 5 | 349.41 | 2 | 174.704 | 0.544 | Javascript | [ShellSceneStore.qml:443](D:/Code/f4-zoin/qt/host/qml/ShellSceneStore.qml:443) | applyCompactPatch |
| 6 | 344.66 | 3 | 114.886 | 0.537 | Javascript | [RetainedGalleryPanelContent.qml:61](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:61) | refreshDescriptor |
| 7 | 333.48 | 3 | 111.160 | 0.519 | Javascript | [GalleryPanelHostAdapter.qml:264](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:264) | applyRendererStateTo |
| 8 | 230.71 | 3 | 76.902 | 0.359 | Javascript | [GalleryPanel.qml:578](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:578) | applyPresentationMode |
| 9 | 230.53 | 3 | 76.842 | 0.359 | Javascript | [GalleryViewport.qml:45](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:45) | applyPresentationState |
| 10 | 228.37 | 3 | 76.122 | 0.356 | Binding | [RetainedGalleryPanelContent.qml:76](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:76) |  |
| 11 | 225.53 | 1 | 225.525 | 0.351 | HandlingSignal | [PanelsSurface.qml:74](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:74) |  |
| 12 | 225.52 | 1 | 225.523 | 0.351 | Javascript | [PanelsSurface.qml:74](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:74) | onRightPanelPresentationOverrideChanged |
| 13 | 225.52 | 1 | 225.521 | 0.351 | Javascript | [PanelsSurface.qml:46](D:/Code/f4-zoin/qt/host/qml/PanelsSurface.qml:46) | updatePanelDescriptor |
| 14 | 225.09 | 1 | 225.094 | 0.351 | Binding | [PanelPairSurface.qml:89](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:89) |  |
| 15 | 222.98 | 3 | 74.328 | 0.347 | HandlingSignal | [GalleryPanel.qml:671](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:671) |  |
| 16 | 222.98 | 3 | 74.327 | 0.347 | Javascript | [GalleryPanel.qml:671](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanel.qml:671) | expression for onPresentationModeChanged |
| 17 | 221.30 | 1 | 221.302 | 0.345 | HandlingSignal | [RetainedGalleryPanelContent.qml:43](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:43) |  |
| 18 | 221.30 | 1 | 221.301 | 0.345 | Javascript | [RetainedGalleryPanelContent.qml:43](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:43) | onPanelChanged |
| 19 | 221.30 | 1 | 221.298 | 0.345 | Javascript | [RetainedGalleryPanelContent.qml:17](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:17) | activatePanel |
| 20 | 218.24 | 3 | 72.746 | 0.340 | HandlingSignal | [GalleryPanelHost.qml:373](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:373) |  |
| 21 | 218.23 | 3 | 72.744 | 0.340 | Javascript | [GalleryPanelHost.qml:373](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:373) | expression for onPanelChanged |
| 22 | 218.22 | 3 | 72.739 | 0.340 | Javascript | [GalleryPanelHostAdapter.qml:431](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHostAdapter.qml:431) | synchronizePanel |
| 23 | 192.26 | 57560 | 0.003 | 0.299 | Binding | [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) |  |
| 24 | 163.24 | 448 | 0.364 | 0.254 | Creating | [GalleryEntryPreview.qml:211](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:211) | QtQuick/Loader |
| 25 | 139.83 | 36 | 3.884 | 0.218 | Creating | [GalleryVectorImage.qml:86](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVectorImage.qml:86) | QtQuick.VectorImage/VectorImage |
| 26 | 123.73 | 2 | 61.864 | 0.193 | Binding | [PanelPairSurface.qml:90](D:/Code/f4-zoin/qt/host/qml/PanelPairSurface.qml:90) |  |
| 27 | 123.38 | 2 | 61.692 | 0.192 | HandlingSignal | [RetainedGalleryPanelContent.qml:44](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:44) |  |
| 28 | 123.38 | 2 | 61.689 | 0.192 | Javascript | [RetainedGalleryPanelContent.qml:44](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:44) | onLayoutStateChanged |
| 29 | 115.79 | 2 | 57.895 | 0.180 | Binding | [RetainedGalleryPanelContent.qml:78](D:/Code/f4-zoin/qt/host/qml/RetainedGalleryPanelContent.qml:78) |  |
| 30 | 115.49 | 2 | 57.744 | 0.180 | HandlingSignal | [GalleryPanelHost.qml:377](D:/Code/f4-zoin/qt/host/qml/GalleryPanelHost.qml:377) |  |

## Isolated scrolling hotspots

| Source | Detail | Count | Exclusive ms | Maximum inclusive ms |
|---|---|---:|---:|---:|
| [GalleryPanelChrome.qml:72](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:72) | onContentYChanged | 584 | 75.57 | 0.55 |
| [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) | expression for iconSceneOrigin | 52771 | 62.03 | 0.05 |
| [GalleryEntryDelegateBase.qml:76](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryDelegateBase.qml:76) |  | 52771 | 49.98 | 0.95 |
| [GalleryPanelChrome.qml:105](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryPanelChrome.qml:105) |  | 584 | 25.73 | 0.36 |
| [GalleryVectorImage.qml:86](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryVectorImage.qml:86) | QtQuick.VectorImage/VectorImage | 4 | 7.62 | 12.54 |
| [GalleryEntryPreview.qml:164](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:164) | QtQuick.Controls.impl/IconImage | 278 | 7.26 | 0.14 |
| :0 |  | 2 | 5.88 | 5.32 |
| [GalleryEntryPreview.qml:173](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:173) |  | 225 | 5.78 | 0.12 |
| [ColumnsEntryDelegate.qml:17](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:17) | pixelOffset | 484 | 5.42 | 0.04 |
| [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) |  | 584 | 5.39 | 0.04 |
| [GalleryCursorController.qml:111](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:111) | indexIntersectsViewport | 584 | 5.12 | 0.04 |
| [GalleryEntryPreview.qml:99](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:99) |  | 192 | 4.79 | 0.14 |
| [GalleryViewport.qml:349](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryViewport.qml:349) | expression for onContentYChanged | 584 | 3.92 | 0.12 |
| [GalleryCursorController.qml:176](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:176) | updateVisualCursorForViewport | 584 | 3.73 | 0.06 |
| [GalleryCursorController.qml:88](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:88) |  | 584 | 3.19 | 0.05 |
| [GalleryGroupHeaders.qml:12](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryGroupHeaders.qml:12) | expression for model | 584 | 2.82 | 0.02 |
| [GallerySelectionController.qml:62](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GallerySelectionController.qml:62) | refreshHoveredIndex | 588 | 1.95 | 0.02 |
| [GalleryEntryPreview.qml:285](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryEntryPreview.qml:285) | QtQuick/Text | 172 | 1.92 | 0.06 |
| [ColumnsEntryDelegate.qml:50](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:50) | QtQuick/Text | 106 | 1.80 | 0.12 |
| [GalleryCursorController.qml:88](D:/Code/f4-zoin/third_party/ZoinGallery/qml/GalleryCursorController.qml:88) | expression for cursorPixelGridViewportOrigin | 584 | 1.76 | 0.01 |

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

### [ColumnsEntryDelegate.qml:17](D:/Code/f4-zoin/third_party/ZoinGallery/qml/ColumnsEntryDelegate.qml:17)

```qml

    function pixelOffset(item) {
        // The row observes its complete ancestor chain once. Labels only
        // need their local chain plus that shared scene-space origin.
        const origin = content.entry.iconSceneOrigin
        let revision = origin.x + origin.y
        for (let parent = item.parent; parent && parent !== content.entry; parent = parent.parent) {
```

This work repeats on the scroll path. Reduce repeated state propagation or preserve prepared objects; validate against the native frame timeline before changing it.

## Next steps

1. Bound and preprepare vector scene creation; reuse nearby surfaces by revision.
2. Reduce entering-row/column delegate and media-leaf churn, then measure render-batch rebuilding.
3. Investigate Details render-thread CPU and driver cost separately from QML.
4. Trim residual alignment notifications and hidden-scrollbar updates after the larger stalls.

A focused qt-qml-review of GalleryVectorImage.qml, GalleryEntryPreview.qml and GalleryEntryDelegateBase.qml can check lifecycle and binding changes before implementation.

See the master report for capture scope, native CPU evidence and renderer experiments.

> AI assistance has been used to create this output.