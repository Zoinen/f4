# Review queue

Doubts, shortcuts and things deliberately left alone, written down here so
they can be reviewed in one sitting instead of being rediscovered one at a
time. Nothing here blocks anything; anything that grows a plan of its own
moves out of this file.

## The keepalive interval is not the one the host was configured with

A minute is a guess that fits the common defaults. A host with a shorter
`ClientAliveInterval`, or a NAT with a thirty second table, drops the session
anyway, and there is nothing in the protocol that would let the client ask. A
setting is the obvious answer and should wait until a host that needs it turns
up.

## Finding a session dead is still all that happens

The keepalive marks a session broken and stops. That moves the discovery
earlier, which is worth having on its own, but the user still has to open the
site again. Picking up where it was is the rest of step 14.
## Two archives share one state namespace

`vfsStateNamespace` names a remote site by the title the panel already shows and
everything else by its Go type. That tells a local file from an archive member
and one host from another, but two different archives are one namespace: the
same path inside two zips shares a saved position. Naming an archive by the file
it lives in needs the path of that file, which a mounted VFS does not expose
today.

## A site renamed is a site forgotten

The key holds `user@host` as it was typed. Connecting to the same machine by IP
one day and by name the next gives two namespaces, and every saved position
starts again. A host key would be the honest identifier; it is also one the user
never sees, so the panel would show one thing and the state file another.

## AsyncBuffer can hand out a short read without saying so

`AsyncBuffer.Read` clips what it copies to the length of the chunk it found
and returns no error when the result is shorter than asked for. A chunk is
stored short whenever the underlying `ReadAt` returned fewer bytes together
with `io.EOF`, which for a remote file is not only the end of the file.
Anything that advances by `len(data)` then loses its alignment silently, and
the line offsets it records are wrong from that point on. Not observed in the
wild; found while reading the code for Step 13a.

## A chunk that failed to load is dropped without a trace

`AsyncBuffer.fetchChunk` logs the failure and clears the fetching flag, so the
next read retries. That is the right behaviour for a hiccup and the wrong one
for a host that is gone: nothing counts the retries and nothing tells the
user.

## A mouse click still cancels a pending restore unconditionally

`ProcessMouse` drops the restore whenever a button is down. A click does place
the cursor, so that is defensible, but it is not the same test the keyboard now
applies, and a click that lands on the cursor's own position cancels the jump
for nothing.

## Automated Layout Verification

An automated layout validation suite has been implemented in `f4/dialog_layouts_test.go`.
It acts as a single source of truth by iterating over all actions from `action_registry.go` and verifying their layout constraints (overlaps, border collisions, and multi-language overflows) across all available translation packs.

- When creating or modifying dialogs, developers no longer need to write custom test logic; the layout test suite automatically discovers them as long as they are registered as standard actions.
## File Association Test Race Condition Fixed
We resolved a test-flake race condition in `TestFileAssociation_PickerRunsChosenCommand` where asynchronous panel refreshes (`f4/file_associations_dispatch_test.go`) would overwrite manually-mocked panel entries with actual repository root files (such as `.git`). This was fixed by physically creating the mock files and directories within the temporary test directory on disk before triggering the refresh.

## The indexer's batch size is a constant nobody has measured

500 line offsets per batch, 64 KB per read. Both were picked to keep the UI
thread from being flooded by a local file, and neither has been measured
against a link where a read is a round trip.### Postponed far2l Color Porting (Roadmap / Future Tasks)

The following color entities and features from `far2l` have been explicitly postponed during the initial alignment phase and remain on our future roadmap:
- **Explicit Disabled Colors:** Transition to dedicated disabled color slots from themes/palette instead of using the dynamically computed `DimColor` fallback.
- **Granular Lists & Comboboxes:** Support custom coloring for `Dialog.List.*` and `Dialog.Combo.*` sub-elements.
- **Default Buttons:** Map and apply styling for `Dialog.DefaultButton.*` elements.
- **Editor & Viewer Selection:** Map and support dedicated selection slots `Editor.Text.Selected` / `Viewer.Text.Selected` (currently falls back to standard text selection).
- **Secondary Widgets:** Support `Clock`, `Panel.ScreensNumber`, and `Panel.DragText` slots.
