#!/usr/bin/env bash
# Drag-out acceptance probe for the Wine GUI backend (unxed/f4#566).
#
# Runs f4.exe --gui=win32 under Wine on the current X display (the Wine job
# starts it inside xvfb-run), then drags the only file of its panel onto
# tools/xdndtarget, a bare XDND drop target on the same display. The target's
# exit code is the verdict (0: the exact path arrived as text/uri-list).
#
# Wine's f4 starts in "/" whatever its working directory or first argument is
# (run 38986161054), so the probe types `cd <folder>` into f4's command line.
# The window is a 100x30 cell screen, so a row is a thirtieth of the window height; the list starts on
# row 3 (screen dump of unxed/f4#566 run 38984108992), the cd lands in the right
# panel (the click that focuses it is at its middle), where ".." is row 3 and the
# file row 4. The probe tries row 4, then its neighbours, and stops at the first
# drop that lands.
#
# usage: wine-xdnd-drag.sh <build dir with f4.exe and xdndtarget> <log dir>
# env:   WINE  the wine loader
set -u

build=${1:?build dir}
logs=${2:?log dir}
: "${WINE:?}"

mkdir -p "$logs" "$build/dragdir"
printf 'xdnd\n' > "$build/dragdir/dragme.txt"
want="$build/dragdir/dragme.txt"

cd "$build/dragdir" || exit 2
"$WINE" "$build/f4.exe" --debug --dump-screen-after 19 --gui=win32 > "$logs/f4.out" 2>&1 &
f4pid=$!
# f4's own account of each gesture (the DND:/WINEX11DRAG: lines of its debug
# log, unxed/f4#566 step C3d): whether the drag started at all, and why not.
# shellcheck disable=SC2329 # runs from the EXIT trap
collect() {
  find "$build" -maxdepth 4 -name debug.log -exec cp -t "$logs" {} + 2>/dev/null || true
  if [ -f "$logs/debug.log" ]; then
    # The per-motion "over" lines (one per 15 ms) would push the end of the drag
    # -- drop, leave, finish -- out of the tail and out of a clipped CI log.
    grep -E 'DND|WINEX11DRAG|XDND|[Dd]rag' "$logs/debug.log" | grep -vE 'DND: over (at|handled)' |
      tail -n 40 > "$logs/drag-lines.txt" || true
  fi
}
trap 'collect; kill "$f4pid" 2>/dev/null || true' EXIT
sleep 12

# The f4 window is the largest visible window that is not the whole screen.
best=0
for id in $(xdotool search --onlyvisible --name . 2>/dev/null); do
  eval "$(xdotool getwindowgeometry --shell "$id" 2>/dev/null)" || continue
  area=$((WIDTH * HEIGHT))
  if [ "$WIDTH" -lt 1280 ] && [ "$area" -gt "$best" ]; then
    best=$area wx=$X wy=$Y ww=$WIDTH wh=$HEIGHT wid=$id
  fi
done
if [ "$best" -eq 0 ]; then
  echo "no visible f4 window" | tee "$logs/verdict.txt"
  exit 1
fi
echo "f4 window $wid at ${wx},${wy} size ${ww}x${wh}" | tee "$logs/window.txt"

# Go into the folder through the command line (the screen dump at 19 s shows
# whether it worked: the row/column layout the guesses below have to hit),
# then give f4 time to redraw.
cw=$((ww / 100))
ch=$((wh / 30))
xdotool mousemove $((wx + 75 * cw)) $((wy + 12 * ch)) click 1
xdotool type --delay 40 "cd $build/dragdir"
xdotool key Return
sleep 9
find "$build" -maxdepth 2 -name vtui.screen.log -exec cp -t "$logs" {} + 2>/dev/null || true

for k in 4 3 5; do
  "$build/xdndtarget" -x 1000 -y 700 -w 250 -h 250 -timeout 8s \
    -want-path "$want" -out "$logs/uri-$k.txt" > "$logs/target-$k.out" 2>&1 &
  tpid=$!
  for _ in $(seq 1 50); do
    grep -q '^READY' "$logs/target-$k.out" 2>/dev/null && break
    sleep 0.1
  done
  sx=$((wx + 56 * cw))
  sy=$((wy + k * ch + ch / 2))
  # No click before the press: two presses 0.3 s apart are a double click, which
  # opens the file (rundll32 in f4's debug log, run 38988877156) instead of
  # arming the drag; the press alone puts the cursor on the row.
  xdotool mousemove "$sx" "$sy" sleep 0.5 mousedown 1 sleep 0.3
  for step in 1 2 3 4 5 6 7 8 9 10; do
    xdotool mousemove $((sx + (1100 - sx) * step / 10)) $((sy + (800 - sy) * step / 10)) sleep 0.1
  done
  xdotool sleep 0.3 mouseup 1
  rc=0
  xdotool getmouselocation > "$logs/mouse-$k.txt" 2>&1 || true
  wait "$tpid" || rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "drop landed from row $k (${sx},${sy})" | tee "$logs/verdict.txt"
    exit 0
  fi
  echo "row $k (${sx},${sy}): target exit $rc" >> "$logs/attempts.txt"
done
echo "no drop landed" | tee "$logs/verdict.txt"
exit 1
