# Design Document: VTML Browser for f4

**Version:** 0.1 (Draft)  
**Status:** Concept / Ready for implementation planning  
**Author:** Design phase for f4  
**Date:** 2026-08  

## 1. Vision & Philosophy

We are building a **special-purpose hypertext browser** inside f4 — not a general-purpose web browser.

It is deliberately constrained, elegant, and native to the console.

### Core Principles

- **Return to pure hypertext.** Markdown was always closer to the original idea of hypertext than modern HTML+CSS+JS monstrosities.
- **Constraints create beauty.** Like classic console applications, BBS, FIDO, and early Twitter, strict limits prevent ugly interfaces and force clarity.
- **Native experience.** Navigation should feel like the existing Far/f4 help system (keyboard-first, topic links, history).
- **Lightning-fast & offline-capable.** Must work comfortably over rotten GPRS from a forest. No heavy assets, no layout thrashing.
- **Graceful degradation is mandatory.** A page must remain fully usable without JavaScript.
- **No CSS. No complex design language.** Visual style is inherited from vtui/f4 palette and the existing Help viewer aesthetics.
- **95% of useful sites could be written in this format.** Documentation, tools, dashboards, forms, simple apps, AI-assisted interfaces.

### What this is *not*

- Not a full HTML browser (intentionally deferred, and only as a possible future whitelist-based mobile-view mode).
- Not a replacement for modern web apps that require rich graphics or complex client-side state.
- Not a place for arbitrary JS embedded in content.

### The bigger picture

This is the missing half-step that BBS and FIDO never took. Combined with later AI panel integration, it becomes a natural home for lightweight tools and human-readable interfaces that live inside the file manager.

## 2. Goals (MVP)

1. Load and render local and remote `.md` / `.vtml` documents.
2. Full hyperlink navigation (same-document anchors + cross-document links) with history (Back/Forward).
3. Extend Markdown with a minimal, elegant form system based on vtui controls.
4. Support external JavaScript files (via QuickJS / qjs + wazero) that can enhance forms and build interactive TUI elements on top of vtui.
5. Strict separation: JS never lives inside the Markdown/VTML source.
6. Graceful degradation: forms and content remain usable when JS is disabled or fails.
7. Extremely lightweight: minimal dependencies, sandboxed JS, predictable resource use.

## 3. Non-Goals (MVP)

- Rendering arbitrary HTML / CSS.
- Full DOM, event model, or browser APIs.
- Inline `<script>` or JS inside Markdown.
- Complex styling, animations, or pixel-perfect layout control.
- Network protocols beyond simple HTTP(S) GET/POST for documents and form submission (and local files / VFS).
- Persistent storage / cookies / localStorage in the first version (can be added later in a controlled way).

## 4. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        f4 Browser Window                     │
│  (reuses / extends vtui HelpView + HelpEngine)               │
├──────────────────────┬──────────────────────────────────────┤
│   VTML Parser        │   Document Model                      │
│   (MD + extensions)  │   (AST / blocks + form widgets)       │
├──────────────────────┼──────────────────────────────────────┤
│   Renderer           │   Link / Navigation Manager           │
│   (vtui widgets)     │   (history, anchors, cross-doc)       │
├──────────────────────┼──────────────────────────────────────┤
│   Form Engine        │   JS Runtime (optional)               │
│   (vtui Edit/Button) │   qjs (QuickJS via wazero)            │
└──────────────────────┴──────────────────────────────────────┘
         │                          │
         ▼                          ▼
   Local FS / VFS              Network (HTTP simple)
   (f4 panels, archives…)      (GET documents, POST forms)
```

### Key reuse

- **vtui HelpEngine / HelpView** already knows how to:
  - Parse a practical subset of Markdown
  - Render text, headings, lists, code blocks
  - Handle topic links (`~text~@Topic@` style and modern Markdown links)
  - Keyboard navigation, search, history inside a help document

We extend this engine rather than rewrite it.

## 5. VTML Format

**VTML** = Markdown + a very small set of form-oriented extensions + external script references.

It should feel like “Markdown that can contain simple interactive forms”.

### 5.1 Base

Standard CommonMark / GitHub-flavored Markdown (the subset already supported by the help engine), plus:

- Headings, paragraphs, emphasis, lists, code blocks, blockquotes
- Links: `[text](url)` and reference-style
- Images: treated as alt-text only (or simple placeholder) in MVP
- Tables (if the help engine already supports them)

### 5.2 Form Extensions (MVP)

We introduce a lightweight, readable syntax that maps directly onto vtui widgets.

Recommended syntax (readable, MD-friendly, easy to parse):

```markdown
::: form id="login" action="submit" method="post"
name: [________________]{id=username placeholder="Username"}
pass: [________________]{id=password type=password}
[ Login ]{type=submit id=loginBtn}
:::
```

Alternative (more explicit / less ambiguous) — preferred for robustness:

```markdown
```form
id: login
action: /api/login   # or "js:handleLogin" or relative
method: post

field:
  id: username
  label: Username
  type: text
  placeholder: Enter username
  required: true

field:
  id: password
  label: Password
  type: password

button:
  id: submit
  label: Login
  type: submit
```
```

**Decision needed:** which syntax is more elegant and parseable while staying true to “MD vibe”?  
The fenced ` ```form ` block is safer and easier to extend. The `::: form` container is more lightweight.  
Recommendation: start with fenced code block of language `form` (or `vtml-form`).

Supported controls in MVP:

| Control     | VTML representation          | vtui mapping     | Notes |
|-------------|------------------------------|------------------|-------|
| Text input | `field` type=text/password  | `Edit`           | single line |
| Submit      | `button` type=submit        | `Button`         | primary action |
| (future)    | checkbox, select, textarea  | Checkbox, Combo, MultiLineEdit | later |

Form attributes:

- `id` — required for JS addressing
- `action` — URL, relative path, or `js:functionName`
- `method` — get / post (default post)

Field attributes: `id`, `label`, `type`, `placeholder`, `value`, `required`, `readonly`…

### 5.3 Script References

Strict rule: **no JavaScript inside the VTML/Markdown document**.

```markdown
```script
src: validate.js
```
```

or simply a special link / meta:

```markdown
[script]: # (validate.js)
```

Scripts are loaded from the same directory (or relative path) as the document, or from a well-defined scripts/ folder.  
Multiple scripts allowed; order preserved.

### 5.4 Document Metadata (optional)

YAML front-matter or a simple header:

```yaml
---
title: Login
scripts:
  - validate.js
  - ui.js
---
```

## 6. Navigation & Document Model

### 6.1 Loading

Sources:

1. Local file / VFS path (highest priority for speed)
2. `file://`
3. `http://` / `https://` (simple client, follow redirects limited, timeout friendly)
4. Internal `help:` or `f4:` schemes (existing help topics)

Cross-document links use normal relative or absolute URLs.  
Same-document anchors (`#section`) work as in current help.

### 6.2 History

Classic browser history stack:

- Back / Forward (keys: `Alt+Left` / `Alt+Right` or `Backspace` / dedicated keys)
- Current document + scroll position + form state (best-effort)
- Title extracted from first H1 or front-matter

### 6.3 Link Handling

- Internal anchors → scroll / topic jump (existing help logic)
- Relative VTML/MD → load in same window
- External http(s) → load if content-type looks like text/markdown or text/vtml; otherwise show “not supported” or open externally (configurable)
- `js:` links → call exported JS function (with confirmation? or only if script already loaded)

## 7. Rendering

The renderer turns the VTML AST into a vtui widget tree (or enhances the existing HelpView).

- Static content → existing help rendering (text, links, code…)
- Forms → group of vtui widgets placed in the flow (or in a dedicated form container)
- Focus order follows document order (Tab cycle)
- Keyboard navigation remains fully native (Far-style)

Visual style:

- Uses existing f4/vtui palette (`Help.*`, `Dialog.*` colors)
- No custom colors from the document in MVP
- Forms look like native dialogs / help interactive elements

## 8. JavaScript Integration

### 8.1 Runtime

- **qjs** (https://github.com/fastschema/qjs) — CGO-free QuickJS compiled to Wasm, executed via wazero.
- Already aligned with f4’s direction (wazero is already in the dependency tree for other sandboxed components).
- Secure by default: no filesystem, no network, no host access unless explicitly granted through a narrow API.

### 8.2 Execution Model

- One JS runtime (or isolate) per document (or per browser window).
- Scripts are loaded and executed when the document is rendered (after the static form widgets exist).
- Scripts can:
  - Read/write form field values
  - Show/hide or enable/disable controls
  - Validate on submit
  - Replace or enhance the form with more complex vtui-driven UI (advanced)
  - Call back into host for controlled actions (navigation, toast, simple HTTP)

### 8.3 Host API (minimal, secure)

Exposed as a global `vtml` or `f4` object:

```js
// Read / write fields
vtml.get("username")
vtml.set("username", "value")
vtml.getForm("login") // → object of values

// UI feedback
vtml.toast("Invalid password")
vtml.setError("password", "Too short")

// Navigation
vtml.navigate("other.vtml")
vtml.back()

// Controlled network (optional, capability-based)
vtml.fetch(url, options) // very limited, or disabled by default

// Events
vtml.onSubmit("login", function(values) { ... })
vtml.onChange("username", function(val) { ... })
```

Advanced (later): ability to create additional vtui widgets and attach them to a placeholder region in the document.

### 8.4 Graceful Degradation Rules

- Document must be fully usable with JS disabled.
- Forms still submit (via HTTP or built-in action) if no JS handler.
- Required validation can have a declarative subset that the host enforces even without JS.
- If a script fails to load or throws, the page continues with static behavior + error toast.

### 8.5 Security

- Scripts run in Wasm sandbox (wazero).
- No access to host filesystem, environment, or arbitrary syscalls.
- Capability flags per document or per origin (local files more trusted than remote).
- Size / memory / CPU limits on the runtime.
- User can globally disable JS in the browser settings.

## 9. Form Submission

1. User activates Submit button (Enter or click).
2. Host collects field values.
3. If JS `onSubmit` handler exists → call it. Handler may `preventDefault`, show errors, or call `vtml.submit()`.
4. Otherwise (or after JS approval):
   - `method=get` → navigate to `action` with query string
   - `method=post` → HTTP POST (or internal handler)
   - `action=js:func` → call the function

## 10. Integration with f4

- New viewer type / window: “Browser” or “VTML Viewer”.
- Opened by:
  - Enter / F3 on `.vtml` / `.md` files (configurable)
  - Explicit command / plugin
  - Links from help system
  - Future: AI panel, command line, etc.
- Lives in the same screen / window management as Editor, Viewer, Help.
- Can be opened from panels, archives, network VFS (FISH+, SFTP…).
- Keybar and menu entries for Back, Forward, Reload, View Source, Toggle JS, etc.

## 11. File Structure (proposed)

```
f4/
  browser/                 # or plugins/browser/
    browser.go             # main window / controller
    document.go            # document model + history
    parser.go              # VTML / MD + form extensions
    renderer.go            # turns model into vtui widgets
    forms.go               # form state & submission
    js_runtime.go          # qjs integration
    js_api.go              # host bindings
    network.go             # simple HTTP client
    ...
  ...

vtui/
  help_engine.go           # extended as needed
  help_view.go
```

Or keep most logic inside the existing help package and add a “mode”.

## 12. Implementation Phases

### Phase 1 – Foundation (no JS yet)
- Extend HelpEngine to load external files and follow cross-document links.
- Add proper history (Back/Forward).
- Basic URL / path resolution (local + simple HTTP).
- Anchor support improvements if needed.

### Phase 2 – VTML Forms
- Define and parse the form syntax.
- Render Edit + Button widgets inside the help view flow.
- Collect values and perform basic submission (navigate or POST).
- Keyboard & focus integration.

### Phase 3 – JavaScript
- Integrate qjs/wazero.
- Load external `.js` files.
- Implement minimal host API.
- `onSubmit` / validation example.
- Graceful degradation & error handling.
- Security limits.

### Phase 4 – Polish & Extensions
- Better form controls (checkbox, select…).
- Placeholder regions for JS-generated UI.
- Caching, offline, progress indicators.
- Settings (JS on/off, network permissions).
- Documentation & example gallery.

## 13. Example

**login.vtml**

```markdown
# System Login

Please authenticate.

```form
id: login
action: js:doLogin
method: post

field:
  id: user
  label: Username
  type: text
  required: true

field:
  id: pass
  label: Password
  type: password
  required: true

button:
  id: go
  label: Sign In
  type: submit
```

```script
src: login.js
```
```

**login.js**

```js
vtml.onSubmit("login", function(values) {
  if (values.pass.length < 8) {
    vtml.setError("pass", "Password too short");
    vtml.toast("Validation failed");
    return false; // prevent default
  }
  // proceed or call vtml.navigate(...)
  return true;
});
```

Without JS the form still appears and can POST to a real endpoint if `action` is a URL.

## 14. Open Questions & Decisions Needed

1. **Exact form syntax**  
   Fenced ` ```form ` block vs `::: form` container vs HTML-like tags?  
   Preference for maximum readability and parse simplicity.

2. **How far can JS go in MVP?**  
   Only form validation + field manipulation, or already allow creating extra vtui widgets?

3. **Network permissions model**  
   Global switch / per-origin / always ask / local-only by default?

4. **Script loading**  
   Relative to document only? Allow absolute / CDN? (Recommendation: relative + same-origin only for remote documents.)

5. **State persistence**  
   Should Back restore form field values? (Yes, best-effort.)

6. **Multiple forms per page** — supported? (Yes.)

7. **Integration point**  
   New top-level window type vs extended Help viewer with “browser mode”?

8. **Name**  
   VTML confirmed? Or MDML / F4ML / HyperMD / something else?

9. **Error UX**  
   How to surface JS errors and network failures elegantly inside the TUI.

10. **Future HTML path**  
    Explicitly out of scope for this document; only a short “Ideas for later” section at the end.

## 15. Success Metrics

- A usable multi-page documentation site or simple tool can be written entirely in VTML + optional JS.
- Works comfortably on a 9600-baud-class link.
- Zero visual “web junk” — looks and feels like native f4 help / dialogs.
- A non-JS page is indistinguishable in usability from a JS-enhanced one for basic tasks.
- Codebase stays small and auditable.

## 16. Ideas for Later (out of scope now)

- Whitelist-based simplified HTML renderer (mobile versions).
- More form controls and layout helpers.
- Client-side routing / single-page feel.
- Local storage with user permission.
- AI panel that speaks VTML natively.
- VTML as a plugin/theme format.
- Export / print to pure Markdown.
- Peer-to-peer or distributed VTML sites.

---

This design deliberately stays minimal and opinionated. The constraints are the feature.

Ready for review, critique, and the next iteration of clarifying decisions.

