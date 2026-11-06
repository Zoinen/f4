# Profiling captures

Keep attribution captures separate from clean delivery and motion checks.
Reports live in `reports/`, evidence indexes in `experiments/`, and raw traces in
`traces/` or in the external locations recorded by their manifests.

## Windows desktop observer guard

The Computer Use helper can contribute animated desktop visuals even when no
UI Automation or screenshot calls occur during a timed gesture. The 2026-10-31
DWM trace attributed eight active WinRT animations to that helper. Releasing this
task's client through the supported Node REPL reset caused its helper to exit;
DWM's median draw-call count changed from 29 to 1 in the measured controls.
See `reports/masonry-dwm-observer-isolation-2026-10-31.md` for evidence and limits.

For a clean Windows delivery capture:

1. Prepare and activate the owned diagnostic window before timing.
2. Release this task's Computer Use client using its supported JavaScript-kernel
   reset tool. Do not kill a shared helper or another task's processes.
3. Run the guard before timing and retain its receipt:

   ```powershell
   python profiler/capture_environment.py --output <before-receipt.json>
   ```

4. Keep the application's foreground/display guards and complete warm-up or
   cold-cache admission checks. Do not reconnect Computer Use during timing.
5. Run the guard again after timing, with a separate receipt. These checks are
   point-in-time observations; they do not prove the entire interval was free
   of all observers or replace frame, motion, and cache evidence.

If a helper remains active for another client, leave it alone. An intentional
observer control may pass `--allow-observer`, but its receipt remains marked
unclean. A query failure or unsupported platform never becomes a clean result.
This guard does not terminate processes, change settings, activate windows, or
capture the desktop.

Test the guard with:

```powershell
python -m unittest discover -s profiler -p test_capture_environment.py
```

Desktop duplication, screenshot capture, kernel tracing, GPU tracking, and
other observers can introduce their own costs. Name them in each capture;
the helper guard only covers the specific process identified in this study.
