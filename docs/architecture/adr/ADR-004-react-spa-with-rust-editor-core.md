# ADR-004 — React SPA with a Rust Editor Core

**Date:** 2026-08-06
**Status:** Accepted
**Related:** ADR-002 (Rust depth), RFC-001 (document model)
**Deciders:** @genuinebasilnt

---

## Context

The client is a block-based WYSIWYG editor with live multiplayer, diagnostics squiggles, and undo. Three questions needed answering: what framework, who writes it, and where the boundary between TypeScript and Rust falls.

UI work appears nowhere in the learning objectives. That makes the frontend the best candidate in the project for full AI authorship — with one exception, below.

---

## Decision

### Stack

| Layer | Choice |
|---|---|
| Build | Vite |
| Language | TypeScript (strict) |
| UI | React 19 |
| Routing | TanStack Router — typed params and search |
| Server state | TanStack Query — optimistic updates are mandatory for an editor |
| Styling | Tailwind CSS v4 (`@theme` CSS-first config) |
| Primitives | Radix UI — unstyled, accessible |
| Complex layout | CSS Modules — block tree indentation, diagnostics gutter |
| Rust interop | `wasm-bindgen` + `vite-plugin-wasm` |
| API contract | `utoipa` → OpenAPI → `openapi-typescript` |
| Tests | Vitest + Playwright |
| Deployment | Static bundle → Cloud Storage + Cloud CDN (or any static host when self-hosted) |

### The editor core is Rust, not TypeScript

This is the exception to AI authorship. Per ADR-002, where a feature could live on either side of the boundary, it goes in Rust:

| Rust, compiled to `wasm32` | React / TypeScript |
|---|---|
| Document model — block tree, rope, position-anchored marks | Rendering model state to DOM |
| Selection mapping — DOM range ↔ model coordinate, stable under concurrent remote edits | Capturing events, emitting intents |
| Operation application and inversion | Menus, dialogs, toolbars, gutter, styling |
| Span normalisation, input-rule scanning | Presence avatars, connection state |
| Diagnostic span anchoring | Tooltip and quick-fix presentation |

React is a **pure view**: it receives model state and emits intents. It never mutates the tree.

**Cross-block selection** is the hard part and the reason this belongs in Rust. Per-block `contenteditable` means the DOM will not hand you a selection spanning blocks, so position mapping is yours to build — the same problem class as source maps.

### No editor framework

**TipTap/ProseMirror, Lexical, and Slate are prohibited.** Each ships its own document model *and* its own collaboration layer (ProseMirror pairs with Yjs; Lexical has its own). Adopting one imports a finished CRDT and deletes the deepest phase in the project — the rope, vector clocks, the lock-free op buffer, epoch reclamation, and the CRC32 WAL.

The editor is per-block `contenteditable`, hand-rolled and deliberately thin.

### No SSR

A client-rendered SPA. No Next.js, no Remix.

The app is entirely behind auth with no SEO surface, and its main screen mutates local CRDT state on every keystroke — Server Components hold no client state, so the principal surface would be a client component regardless. SSR would also add a Node process in front of the Rust gateway: a second container, a second CVE surface, a second HPA target, the dual-URL problem (in-cluster DNS for SSR vs public hostname for the browser), and awkward `wasm32` bundling.

A static bundle also keeps a **desktop shell (Tauri) available later at near-zero cost**, since Tauri wraps exactly this artifact. SSR would have foreclosed that.

---

## Consequences

### The API contract becomes load-bearing

TypeScript gives no compile-time guarantee that client and server agree — a renamed field becomes a runtime `undefined`, not a build error.

So `utoipa` annotations on Axum handlers are **mandatory, not decorative**. They emit the OpenAPI document that generates the typed client, and CI regenerates it and **fails on a dirty diff**. A stale generated client is the one failure mode this stack has that a full-Rust stack would not.

### The `wasm-bindgen` boundary must be designed, not discovered

What crosses, how structs are marshalled (`serde_wasm_bindgen`), and who owns the memory are explicit decisions. Constraints:

- **Keys never cross into JS.** Encryption material stays in WASM linear memory
- Bundle budget: `wasm-opt -Oz` in release, `twiggy` in CI to catch regressions
- `console_error_panic_hook` in dev builds only

### Frontend is a continuous track, not a phase

Every backend phase ships the UI that exercises it. A real client consuming the APIs as they are written catches contract mistakes that integration tests structurally cannot.

### Trade-offs accepted

- **No full-stack Rust type sharing.** Compensated by the explicit WASM boundary, which is more transferable learning than shared DTOs would have been
- **Node in the toolchain, not in production.** `npm` and Vite at build time; the deployed artifact is static files
- **TypeScript enters code review.** Strict review mode now covers two languages

---

## Alternatives Considered

| Alternative | Why not |
|---|---|
| Leptos / Dioxus (Rust frontend) | Frontend is not a learning goal, so AI authorship is correct — and AI-authored Rust conflicts with the mentor rules. The editor core in `wasm32` captures the Rust value without the conflict. **See § Amendment — that premise is now in question, and the revisit is scheduled after the 🏁** |
| Next.js | Five objections above; principally the Node process and hiding the gateway |
| TipTap / Lexical / Slate | Ships a finished CRDT; deletes the deepest phase |
| Sass Modules | Native CSS absorbed nesting, custom properties, `@layer`, and `color-mix()`; and it supplies no accessible primitives |
| Vanilla Extract | Genuine runner-up — type-safe tokens, zero runtime. Loses the Radix + shadcn ecosystem |
| Canvas text layout | Forfeits screen readers, find-in-page, and native selection; teaches text layout, not a goal |

---

## Amendment — a Rust frontend is revisitable, after the 🏁

**Added 2026-08-09.**

The Alternatives table rejects Leptos/Dioxus because *"frontend is not a learning goal."*
**That premise is now in question**, so the rejection is out of date rather than wrong, and this
records the trade so it is decided once rather than re-litigated every few weeks.

### The argument for, and it is architectural rather than preferential

**If the whole frontend is Rust, `libs/doc` stops being a wasm boundary and becomes a plain
dependency.** That removes § *The `wasm-bindgen` boundary must be designed, not discovered*
entirely — no `serde_wasm_bindgen` marshalling, no crossing budget, no debate about exposing an
AST to a debug panel. Passing a `Rope` to a component becomes passing a Rust value.

For an editor specifically that is a real simplification. It is the strongest reason to consider it
and it is not the reason usually given.

### The ecosystem is adequate; the maturity is not

| Concern | State |
|---|---|
| **CSS / Tailwind** | **No difference.** Tailwind scans `**/*.rs` for class strings; `mockup.css` is plain CSS with custom properties and ports unchanged. One sharper gotcha than React: `format!("bg-{}-500", c)` is invisible to the scanner |
| **Accessible primitives** | They exist — [RustForWeb/radix](https://github.com/RustForWeb/radix), [DioxusLabs/components](https://github.com/DioxusLabs/components), [leptail](https://github.com/leptail/leptail) — but with a fraction of Radix's edge-case history. **This is the gap that matters**: a dialog that does not restore focus is invisible in a screenshot and breaking for some users |
| **Composables / utilities** | [leptos-use](https://github.com/Synphonyte/leptos-use), [thaw](https://github.com/thaw-ui/thaw), [leptonic](https://github.com/lpotthast/leptonic) |

### What users would and would not notice

| | Verdict |
|---|---|
| Visual design, motion vocabulary, theming | **Identical** — it is all CSS, and the design is already specified |
| Mid-session responsiveness | **Equal or better.** Fine-grained reactivity, no VDOM diff, on an update-heavy surface |
| **Initial load / time-to-interactive** | **Worse, and users feel this one.** A wasm bundle must download, compile, and instantiate before first paint. Partly pre-paid because the editor core ships wasm regardless, so the delta is smaller here than for a typical app — but real |
| Animation beyond CSS | **Worse ecosystem.** Phase 16's spring scroll is JS-driven physics; React has Framer Motion, Rust has no equivalent. Buildable, but it is work not otherwise on the list |
| Accessibility | **Worse today** unless invested in deliberately |

> **Ceiling: the same. Achieved polish at a given date: lower** — because polish comes from
> iteration count, and Rust/wasm rebuild cycles are much slower than Vite HMR with a fraction of
> the example surface to crib from. The risk is not that Leptos cannot do it; it is the twentieth
> refinement pass that never happens.

### The cost that decides it

- ADR-005's amendment removes **~80–120h that teaches no Rust** and takes it **off the critical path**. In Rust that time returns, **on** the critical path
- **The mentor rules forbid AI-authored Rust**, so none of it can be delegated — the parallel frontend track disappears entirely
- `TIMELINE.md`'s week-10 demoable and week-12 interview-ready bands both slip materially

### Decision

**Revisit after the 🏁, never before.** A port is materially cheaper than a rewrite:

1. Porting a design you have **seen working** is mechanical and side-by-side comparable; designing polish for the first time *in* Leptos pays the slow-iteration cost exactly when iteration matters most
2. By then you will know whether you want frontend Rust or like the idea of it
3. **Nothing is lost by waiting.** `mockup.css` is framework-agnostic, the mockups are HTML, and the only discarded work is the `wasm-bindgen` boundary design — which is itself a Phase 16 learning item you would have done regardless

**If browser-side Rust is wanted sooner, Phase 16 already has it** — IME composition, selection APIs, grapheme-aware cursor movement, `twiggy` bundle work. That is real frontend Rust on the critical path today, just not framework Rust.

This amendment does not change the decision. It records what would have to be true to change it,
and when to ask.

---

## Resources

| Resource | For |
|---|---|
| [wasm-bindgen guide](https://rustwasm.github.io/wasm-bindgen/) | Designing the Rust ↔ TS boundary |
| [utoipa](https://docs.rs/utoipa) / [openapi-typescript](https://openapi-ts.dev/) | Contract generation |
| [TanStack Query — optimistic updates](https://tanstack.com/query/latest/docs/framework/react/guides/optimistic-updates) | The hard part of an editor client |
| [Radix primitives](https://www.radix-ui.com/primitives) | Accessible menus, dialogs, popovers |
| [Why ContentEditable Is Terrible](https://medium.engineering/why-contenteditable-is-terrible-122d8a40e480) | Read before the editor phase |
