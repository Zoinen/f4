# D3D11 offset-only binding snapshot reuse

This isolated candidate is not promoted. The normal application's executable,
SDK headers, SDK archives and embedded payload were not changed. Constant
160 Hz without spikes remains unverified.

## Motivation and implementation

Scheduler CPU stacks identify D3D11 ResourceBatches and StageUniformBufferBatches
copies/destructors among the render-thread hotspots. Qt 6.11.1 retains a new
binding snapshot for each dynamic-offset bind, although offsets themselves are
stored separately in each command.

The prototype reuses the immediately preceding retained snapshot only when
pipeline, SRB identity, SRB generation and resource generations are unchanged,
dynamic offsets are present, and the retain pool is nonempty. It preserves
every command and per-command offset. All other binds retain a new snapshot.
No SDK header, persistent cache or object layout changes are needed.

The source audit covers resetCommands, resetCachedState, external rendering,
finish, pass changes and graphics/compute transitions. Command execution reads
the retained snapshots through const references and computes offsets separately.
The same executable can select the original retain operation with the presence
of F4_PROFILE_DISABLE_D3D_BINDING_REUSE.

## Regression evidence and limits

The baseline regression fails with 128 retained snapshots instead of one.
The replacement implementation passes 9 tests, including independent offsets
for 128 commands and boundaries for SRB change, rebuild, pipeline change,
command-list reset, compute transition and cached-state reset. These tests
exercise actual Qt command recording with synthetic resource batches; they do
not establish rendered GPU correctness. Resource recreation and GPU readback
coverage would still be required before production promotion.

The isolated host passes import isolation and compiled-host loading (4 tests)
and the Windows system-DLL dependency audit. Receipts pin all linked inputs.

## Live result

Same-exe control/enabled and reversed enabled/control comparisons use 187 entries
in Downloads, Masonry density 152, a 2194x1186 window, DPR 1.75 and 160 Hz.
All cold/warm captures retain focus. QML mask improvement is enabled in both
arms; rejected texture experiments, detailed D3D timers, material micro-probes
and scheduler collection are disabled. The prior vector prototype remains fixed
in the diagnostic base and is not promoted by this experiment.

Preparation p99, control -> enabled, milliseconds:

| Order | Cold 0 | Cold 1 | Warm 0 | Warm 1 |
| --- | ---: | ---: | ---: | ---: |
| Control then enabled | 2.3227 -> 2.2636 | 1.9026 -> 1.7903 | 1.8471 -> 1.8169 | 1.9448 -> 1.8405 |
| Enabled then control | 2.1483 -> 2.4178 | 1.6631 -> 1.7162 | 1.7838 -> 2.2679 | 1.7146 -> 1.8785 |

The first comparison improves, but all four reversed preparation p99 values
worsen. Render-to-frame-end p99 improves in all four first blocks but only one
of four reversed blocks. One enabled warm block still has a counted displayed
interval over 9.375 ms. The strict motion gates remain unchanged.

This proves the synthetic recording case avoids copies, not a reproducible
end-to-end performance gain. Current captures do not count actual reuse hits
in the live scene or isolate their cost; do not attribute all measured timing
differences to this operation. Leave the candidate out of the normal app.

Evidence: artifacts/D3DBindingReuseTest.cpp,
artifacts/d3d-binding-before-test.txt, artifacts/d3d-binding-expanded-test.txt,
artifacts/build-d3d-binding-reuse.py,
artifacts/binding-reuse-build-receipt.json,
artifacts/analyze-binding-reuse-pairs.py,
artifacts/binding-reuse-pairs-analysis.json and case files beginning
real-scroll-binding-reuse under artifacts/modes-profile.
