# Vector upload admission: real scrolling and root lifetime

## Workload and controls

Fresh diagnostic host launches use the same current gallery sources and
different Qt Quick libraries: A is the integrated stable-interval renderer,
B is the v2 admission checkpoint, and C adds interval ownership for registered
transform roots. Output names include upload-control, upload-candidate and
upload-interval respectively. The ordinary worktree executable is used as the
Go parent with an explicit diagnostic host override.

The guarded workload is C:/Users/xs/Downloads, 184 entries, density 152,
scroll extent 4161.142857 logical pixels, actual DPR 1.75, active window and
160 Hz display. Wheel events repeat every 30 ms, 120 units each; each direction
runs approximately four seconds. Caption measurements remain zero in all
seven captures. No build, test, sampler, UI Automation observer, renderer debug
log or extra diagnostic scope runs during these captures. View/persistence
state is restored before every process restart.

PresentMon display-change intervals are the headline, not the harness's native
frame period. Long intervals below mean greater than 9.375 ms. The 160 Hz frame
target is 6.25 ms; crossing neither p99 nor this long threshold proves the
all-frame objective. Cold below means new native scene/GPU work after a fresh
process, not an assertion that disk/source caches were purged.

## Measured display cadence

| Run | Down p99 ms | Down maximum ms | Down long | Up p99 ms | Up long |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 normal | 18.7436 | 37.1540 | 22 | 6.2965 | 2 |
| B1 admission | 12.5383 | 24.9929 | 16 | 6.3481 | 4 |
| B2 admission | 18.7206 | 32.5852 | 16 | 6.2731 | 2 |
| A2 normal | 12.4942 | 18.7538 | 13 | 6.2809 | 3 |
| C1 admission + root intervals | 12.4961 | 18.7339 | 9 | 6.3581 | 4 |
| A3 normal | 12.5025 | 31.2454 | 13 | 6.3320 | 4 |
| C2 admission + root intervals | 24.9751 | 37.4476 | 43 | 12.6593 | 30 |

B's first improvement does not reproduce against A2. C1 reduces the count,
but C2 is substantially worse, including its return scroll. No verified cause
justifies excluding C2. Neither variant establishes reliable frame-tail gains,
and no claim of constant 160 Hz or successful rollout follows from these data.

Raw local evidence is under artifacts/modes-profile with case names
upload-real-{a1,b1,b2,a2,c1,a3,c2}-wheel-masonry. Each case has JSON, PresentMon
CSV and a display-summary JSON. The compact combined evidence is
artifacts/upload-real-ab-summary.json. These generated traces remain outside
the source checkpoint.

## Reproduced and fixed root-lifetime cost

The v2 prototype forces registered transforms into independent Qt batch roots
to prevent pending SVG geometry from sharing batches with unrelated content.
Qt's ordinary transform-root creation/removal requests a full rebuild. A new
native fixture first measures this directly: inserting a small vector root
inside a stable clip produces one full rebuild where zero is expected.

The experimental successor gives registered vector roots the same owned
padding reservation and safe return machinery as the integrated clip-root
path. Assigned ancestors fund a partial rebuild when capacity permits. Unsafe
cases keep the original full rebuild. This preserves batch isolation without
adding clipping. The source patch is archived in
profiler/experiments/vector-upload-root-intervals-2026-10-18 and passes ordinary
git apply --check against the normalized bases.

The final regression repeats 16 insertion/removal cycles, requires no full
rebuild during the cycles, checks the visible red overflow and retained green
sibling, and verifies actual leaf origins/unit transforms at DPR 1.75. It has
stable object names for both leaves. This is synthetic lifecycle evidence;
it does not prove that all real scrolling roots avoid full rebuilds.

## Validation and boundary

- Before: vector-upload-root-rebuild-before-175.txt has 2 passes and 1 failure,
  specifically actual full rebuild count 1 versus expected 0.
- Final native suite, including 16-cycle regression: 14 passes, no failures on
  default Windows RHI and OpenGL, in vector-upload-root-cycles-native-175.txt
  and vector-upload-root-cycles-opengl-175.txt.
- Real vector consumer with the revised renderer: 26 focused SVG results pass
  at 175%, including explicit native capability/readiness assertions for both
  swap paths (vector-upload-root-interval-gallery-175.txt).
- After relinking the gallery test against normal Qt, the same two swap paths
  explicitly assert capability absence: 4 results pass, no failures
  (vector-upload-gallery-capability-absent-175.txt).

The displayed test captures include observation overhead; swap timing messages
are not clean latency comparisons. Small-caption rendered checks, transient
native failure/retry policy and full portable gate rollout remain unfinished.
The current normal dependency recipe does not enable admission. The ordinary
worktree app is restored after measurement with diagnostic overrides cleared,
using the already-integrated retention payload, not an experimental host.

## Next investigation

Keep the root-lifetime regression and experimental patch. Attribute the
remaining real native preparation/upload work and C2's return-scroll stalls
in a separate diagnostic run. Distinguish unbudgeted updates of already-ready
surfaces, oversized single allocations, group discovery/preparation cost and
frame scheduling gaps before selecting another change. Admission's two-ms
upload-loop limit excludes material preparation and cannot bound one driver
call. The measured lack of a repeatable improvement must remain part of the
decision, rather than promoting the most favorable capture.

The 160 Hz objective remains unfinished and active.
