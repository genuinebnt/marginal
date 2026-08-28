# Marginal V2 — Implementation Spec

**Read this before writing a single line of UI. Then keep it open while you
write.** The reference is [`index.html`](./index.html) — 40 screens, one file.
This document is the *rules* behind those screens; the file is the *evidence*.
Where they disagree, the file wins and this document is wrong and should be
fixed.

---

## 0. Read this part twice

The failure mode this document exists to prevent is: **an implementer glances
at a screen, forms a general impression ("dark, technical, monospace"), and
then builds something that shares the impression but none of the decisions.**
The result looks plausible in isolation and wrong beside the reference.

Concretely, that failure looks like:

| What gets built | What the reference actually does |
|---|---|
| Rounded corners, `border-radius: 8px` | **Zero radius. Everywhere. No exceptions.** |
| A purple/blue "tech" accent | Ember `#E8873C`, and it is chrome-only |
| Colour used decoratively | Every hue means one thing; using it for a second thing is a bug |
| `gap: 16px`, `padding: 24px` | 8/9/10/12/14, `padding: 12px 14px` |
| Body text at 14–16px | Chrome runs 9–12.5px. Prose runs 15.5–18.5px. Nothing between |
| Drop shadows on cards | Cards have a 1px border and no shadow. Only overlays get shadow |
| A generic sans everywhere | Three families with strict jobs (§2) |
| Placeholder copy ("Lorem", "Item 1") | Every string is a specific, true claim (§9) |
| Empty regions left empty | Measured and filled, or deliberately empty with a reason |
| Icons from a library | Text glyphs: `◌ ✓ ✕ ● ━ → ↕ ⌕ ✦ ◆ ⌾ ⌘ ↵` |

If your output would fill in the left column, stop and re-read the relevant
section.

**The single most useful habit:** before writing a component, open
`index.html`, find the screen that already contains that component, and copy
its markup. Do not re-derive it. The reference is a component library that
happens to be laid out as screens.

---

## 1. Frame and canvas

Every screen in the reference is a fixed artboard. This is a **documentation
device**, not a viewport constraint.

```css
.sc {
  position: relative;
  width: 1440px;            /* artboard only — the real app is fluid */
  height: 860px;            /* artboard only */
  background: #0E0F10;
  color: #E4E2DC;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 40px 100px -40px rgba(0,0,0,.9);
  margin: 0 auto 22px;
}
```

**When you implement the real app:** drop `width`/`height`/`box-shadow`/
`margin`, keep everything else. The screen becomes `height: 100vh`. The
internal layout — a fixed-height bar, a `flex: 1` body, a fixed-height status
bar — is already correct and needs no change.

**The scanline overlay** (`.scan`) is a presentation affectation for the
design doc. **Do not ship it.** It is the one element in the file with no
product meaning.

### 1.1 Vertical structure — every in-app screen

```
┌─────────────────────────────────────────┐
│ .bar          54px   fixed              │  always
├─────────────────────────────────────────┤
│ .sub          34px   fixed              │  optional — section strips
├─────────────────────────────────────────┤
│ .body         flex:1  min-height:0      │  always
│   ┌────────┬──────────────┬──────────┐  │
│   │ .rail  │  main        │ .insp    │  │
│   │ 238px  │  flex:1      │ 296px    │  │
│   │ fixed  │  min-width:0 │ fixed    │  │
│   └────────┴──────────────┴──────────┘  │
├─────────────────────────────────────────┤
│ .status       27px   fixed              │  always
└─────────────────────────────────────────┘
```

`min-height: 0` on `.body` and `min-width: 0` on the main column are
**load-bearing**. Without them flex children refuse to shrink and the layout
overflows. This is the single most common layout bug when reimplementing.

### 1.2 Column widths

| Element | Width | Notes |
|---|---|---|
| `.rail` (left) | `272px` | `250–290px` where a screen justifies it |
| main | `flex: 1; min-width: 0` | never a fixed width |
| `.insp` (right) | `332px` | `300–352px` where a screen justifies it |

These were `238` / `296` and were widened once real content arrived. Page
titles in the mockup were single words (`Inbox`, `Product`); real ones are
sentences, and at `238px` every descriptive title wrapped to two lines —
which doubles the height of exactly the rows carrying the most information
and destroys the scan-down-the-left-edge reading the rail exists for.

**A tree row is one line, always.** `.tr-t` truncates with an ellipsis; the
title is the only thing that may shrink, while markers and counts keep their
width. Row actions live in `.tr-a` and appear on hover — shown always, they
are two glyphs of noise on every row competing with the titles for the same
space.

Deviations exist and are deliberate — Discover uses `352px` because its HNSW
layer diagram needs the room. **Do not deviate without a reason you can
state.**

### 1.3 No scrollbars

The reference has **no visible scrollbars anywhere**. Position is carried by:

- the reading-progress hairline under the top bar (Reader, Published)
- the status bar
- the fact that content visibly continues past the fold

Long columns use `overflow: hidden` in the artboard. **In the real app** they
become `overflow-y: auto` with the scrollbar styled to zero width, or
`scrollbar-width: none`. Content continuing past the bottom edge is correct
and expected — a document does that.

---

## 2. Typography

Three families. Each has one job. **Using the wrong family is the fastest way
to make output look nothing like the reference.**

```html
<link href="https://fonts.googleapis.com/css2?family=Archivo:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600&family=Spectral:wght@400;500;600&display=swap" rel="stylesheet">
```

| Family | Job | Never used for |
|---|---|---|
| **Archivo** (sans) | UI labels, buttons, body copy inside chrome | headings, data, code |
| **IBM Plex Mono** | every number, id, route, key, code, timestamp, tiny caps label | prose |
| **Spectral** (serif) | page titles, document prose, result titles | anything in the chrome |

The rule that produces the look: **if it is a value, it is mono. If it is
prose, it is serif. Everything else is sans.** A count next to a label is
mono. A person's name in a list is sans. A page title is serif — including
in a search result, a card, and the inspector.

### 2.1 The type scale — use these, do not invent

Sizes are in px and are **not** on a neat scale. They were tuned. Use them.

| Size | Weight | Family | Used for |
|---|---|---|---|
| `44 / 40` | 500 | Spectral | published page title / reader title |
| `36` | 500 | Spectral | editor page title |
| `29 / 27` | 500 | Spectral | dashboard, lab, section heroes |
| `23 / 22 / 21 / 20` | 500 | Spectral | screen headings (`.h1`) |
| `19` | 560 | Archivo/Spectral | scrubber timestamp, fact value |
| `18.5 / 18 / 17` | 400 | Spectral | document prose |
| `16 / 15.5` | 400/500 | Spectral | inspector titles, assistant prose |
| `15 / 14.5` | 500/400 | Spectral | card titles, sidenotes |
| `13.5 / 13` | 400 | Archivo | list rows, body copy |
| `12.5 / 12` | 400 | Archivo | dense rows, panel body |
| `11.5` | 400 | Archivo | inspector body copy — **the workhorse** |
| `11 / 10.5` | 400 | Mono/Archivo | notes, metadata |
| `10 / 9.5` | 400 | Mono | status bar, table metadata |
| `9 / 8.5` | 600 | Mono | `.lbl`, `.rd-k`, column headers — **always uppercase, letter-spaced** |

**Letter-spacing matters.** Small caps labels carry `letter-spacing: .19em`
(`.lbl`), `.14em` (`.rd-k`), `.1em`–`.12em` (table headers). Large serif
headings carry **negative** tracking: `-.015em` to `-.032em`. Getting this
wrong is immediately visible.

### 2.2 The two canonical label styles

```html
<!-- Section label. Above every panel and group. -->
<span class="lbl">TAGS INSIDE THIS TOPIC</span>
<!-- font: 600 8.5px mono; letter-spacing: .19em; color: #585550 -->

<!-- Readout. A label with a value, in the top bar. -->
<div class="rd"><span class="rd-k">OPS/S</span><span class="rd-v">1 412</span></div>
<!-- key: 8.5px mono .14em #585550 · value: 11px mono 500 #E4E2DC -->
```

`.lbl` text is **always uppercase in the markup**, not via `text-transform`.

### 2.3 Numerals

Numbers use a **thin space as the thousands separator**: `1 412`, `1 908`,
`118 402`. Not commas. In HTML write the literal character or `&nbsp;`. Never
`1,412`.

---

## 3. Colour

**The palette is closed. Do not add to it.** Nothing here is themeable.

### 3.1 Surfaces — darkest to lightest

| Token | Hex | Used for |
|---|---|---|
| page behind artboards | `#08090A` | body background only |
| published-page ground | `#0B0C0D` | the one screen with no chrome |
| **screen ground** | `#0E0F10` | `.sc` background — the default |
| rail / inspector | `#0F1012` | `.rail`, `.insp`, sub-nav |
| bar / status | `#111214` | `.bar`, `.status`, floating panels |
| raised / overlay | `#131415` | palette, dropdowns, cards |
| input / sunken | `#141617` | text inputs, filter fields |
| selected row | `#181A1B` | `.tr-on` |

Borders are **always** `1px solid rgba(255,255,255,.07)`. Stronger dividers
use `.09`–`.14`. There is no border colour token — it is always white at low
alpha, so it works over any surface.

### 3.2 Ink — text, five steps

| Hex | Use |
|---|---|
| `#EFEDE7` | headings, the single most important string on screen |
| `#E4E2DC` | primary text, active values |
| `#D2CFC8` | body text in lists and panels |
| `#C3BFB7` | emphasis inside muted copy (`<b>` in a note) |
| `#9B968D` | secondary text |
| `#8C8880` | tertiary — **most panel body copy lives here** |
| `#6E6A63` | quaternary |
| `#585550` | labels, metadata, the explanatory sentence under a panel |
| `#4B4842` | faintest — separators in text, `#` prefix on a tag |

### 3.3 Semantic hues — **each means exactly one thing**

| Hex | Name | Means | Never means |
|---|---|---|---|
| `#E8873C` | ember | brand, current selection, chrome accent | a state, a category |
| `#E0A34E` | amber | **a diagnostic, a warning, a lag** | an error, a category |
| `#3FCFA8` | teal | **you**, and healthy state | a category |
| `#A98CE8` | violet | **another person** | a category |
| `#7D9EC9` | slate | **the assistant** (not a person) | a category |

**This is the rule most often broken.** If you need to distinguish five
things that are just "different from each other", you may **not** reach for
amber/teal/violet/slate. Use the categorical ramp below. If the reader learns
that amber means "diagnostic" *and* "topic: operations", amber now means
nothing.

**There is no red anywhere.** A notebook has no compile step; nothing in it is
broken. The most severe state available is amber, and it reads as advisory.

### 3.4 Categorical ramp — topics only

| Hex | Topic |
|---|---|
| `#7AA8E8` | Protocol |
| `#C48AE0` | Storage |
| `#5AC8B4` | Interface |
| `#D6A660` | Operations |
| `#D07C8A` | Research |

Deliberately disjoint from §3.3. These colour graph nodes, topic chips, and
topic-sliced charts — nothing else.

### 3.5 How colour is applied

Coloured elements use a **three-part treatment**: a border at ~42% alpha, a
background at ~7% alpha, and full-strength text.

```css
.tpc-proto {
  border-color: rgba(122,168,232,.42);
  color: #7AA8E8;
  background: rgba(122,168,232,.07);
}
```

Never a solid coloured fill with white text. The only saturated fills in the
entire reference are the notification badge and `aria-pressed="true"` toggles,
both of which invert to `#0E0F10` text.

---

## 4. Geometry — the non-negotiable rule

**`border-radius` is `0` on every element except three, which use `999px`:**

1. `.chip` — status pills
2. `.facet`, `.filters button`, `.pal-toggle`, `.mode` — pill toggles
3. `.bdg` — the notification badge (also fully round)

Everything else — panels, cards, inputs, buttons, overlays, the palette, code
blocks, avatars — is a **hard rectangle**. This single decision does more to
produce the reference's character than any other. If your output has rounded
cards, it is wrong regardless of how correct the colours are.

### 4.1 Corner brackets

Primary buttons carry two 5×5px ember corner marks:

```html
<div class="btn">SHARE<div class="brk-tl"></div><div class="brk-br"></div></div>
```

```css
.btn      { position: relative; padding: 6px 14px;
            border: 1px solid rgba(255,255,255,.18);
            font: 600 10px 'IBM Plex Mono',monospace; letter-spacing: .13em; }
.brk-tl   { position: absolute; top:-1px; left:-1px; width:5px; height:5px;
            border-top:1px solid #E8873C; border-left:1px solid #E8873C; }
.brk-br   { position: absolute; bottom:-1px; right:-1px; width:5px; height:5px;
            border-bottom:1px solid #E8873C; border-right:1px solid #E8873C; }
```

Use on the primary action of a screen. Not on secondary buttons, not on chips.

### 4.2 Left accent borders

A panel that needs emphasis uses a **2px left border** in a semantic hue plus
a 4–7% wash, never a full outline:

```html
<div style="border-left:2px solid #E8873C;background:rgba(232,135,60,.05);padding:13px 17px">
```

Seen on: transclusion callouts, assistant proposals, diagnostics, the
palimpsest panel, active list rows.

---

## 5. Spacing

Measured from the reference. **Use these numbers.**

| Context | Value |
|---|---|
| Icon-to-label gap | `7px` / `8px` / `9px` |
| Row gap in a list | `7px`–`9px` |
| Between panel groups | `12px`–`14px` |
| Between page sections | `20px`–`30px` |
| Panel body padding | `14px` |
| Card padding | `12px 14px` / `13px 15px` / `11px 13px` |
| Chip padding | `2px 7px` / `2px 8px` / `3px 9px` |
| Topic chip padding | `1px 6px` (inline) / `2px 8px` (standalone) |
| Bar horizontal padding | `16px` |
| Main column padding | `22px 30px` → `34px 40px` |

**There is no 16/24/32 scale.** Do not impose one.

Dividers between panel groups are always:

```html
<div style="height:1px;background:rgba(255,255,255,.07)"></div>
```

Not `<hr>`, not `border-top`.

---

## 6. Component contracts

Copy these exactly. Every one appears many times in `index.html`.

### 6.1 Top bar

```html
<div class="bar">
  <span class="wm">m<span style="color:#E8873C">/</span>arginal</span>
  <div class="tabs">
    <span class="tb tb-on">Write</span><span class="tb">Read</span>
    <span class="tb">Search</span><span class="tb">Graph</span>
    <span class="tb">History</span><span class="tb">Lab</span>
  </div>
  <span class="crumb">product / <b>sync-protocol-notes</b></span>
  <div style="flex:1"></div>
  <!-- 0–3 readouts, screen-specific -->
  <div class="rd"><span class="rd-k">OPS/S</span><span class="rd-v">1 412</span></div>
  <div class="vr"></div>
  <!-- UTILITY CLUSTER — fixed order, every in-app screen -->
  <div class="clk"><b>14:06</b><span>THU 12 MAR</span></div>
  <span class="kbd">⌘K</span>
  <div class="icb">◎<div class="bdg">6</div></div>
  <div class="icb">⚙</div>
  <div class="av av-you">GN</div>
</div>
```

**The utility cluster order is fixed: clock → ⌘K → bell → admin → you.** It
never reorders between routes. Muscle memory is a feature; a control that
relocates is one you re-find. Pre-auth screens (Home, Register, Log in), meta
screens (Build map, Page graph, Components) and the Published page carry a
bare bar with no cluster — a stranger has no session to show.

The wordmark is always `m/arginal` with the slash in ember.

### 6.2 Left rail

```html
<div class="rail">
  <div class="rail-h">PAGE TREE<div></div><span style="color:#585550">31</span></div>
  <div class="filt">filter…</div>
  <div style="display:flex;flex-direction:column;gap:1px;padding:0 8px">
    <div class="tr"><span class="tr-n">01</span>Inbox</div>
    <div class="tr tr-on"><i></i><span class="tr-n" style="color:#E8873C">03</span>Sync protocol notes</div>
  </div>
  <div class="wal"><span class="lbl">LOCAL WAL</span>…</div>
</div>
```

- `.rail-h` contains an **empty `<div>`** — it is the flex-grow hairline that
  extends the label to the panel edge. Do not omit it.
- `.tr-on` needs a child `<i></i>` — the 2px ember left marker.
- `.wal` is `margin-top: auto` — it pins a summary to the rail bottom.

### 6.3 Inspector

```html
<div class="insp">
  <div class="insp-t">
    <span class="it">OUTLINE</span>
    <span class="it it-on">CHECKS <span style="color:#E0A34E">2</span></span>
    <span class="it">LINKS</span>
  </div>
  <div style="padding:14px;display:flex;flex-direction:column;gap:12px">
    <span class="lbl">STRUCTURAL</span>
    …
    <div style="height:1px;background:rgba(255,255,255,.07)"></div>
    <span class="lbl">NEXT GROUP</span>
  </div>
</div>
```

Tab counts are coloured by meaning: amber for diagnostics, violet for
comments, teal for healthy.

### 6.4 Status bar

```html
<div class="status">
  <span>/topics/protocol</span>
  <span>topic by id · rename is free</span>
  <div style="flex:1"></div>
  <span style="color:#3FCFA8">● 62 untopiced · assignment is yours</span>
</div>
```

**Contract:** first span is the route. Middle spans are the mechanism in a few
words. The right-hand span is the screen's honest state, prefixed `●` (teal,
healthy) or `◌` (amber, degraded). **Every screen has one and it is never
decorative** — it says the true current state including when that state is
bad.

### 6.5 Chips, topics, tags

```html
<span class="chip">NEUTRAL</span>
<span class="chip chip-a">AMBER · DIAGNOSTIC</span>
<span class="chip chip-t">TEAL · HEALTHY</span>
<span class="chip chip-s">SLATE · ASSISTANT</span>
<span class="chip chip-e">EMBER · SELECTED</span>

<span class="tpc tpc-proto"><i></i>PROTOCOL</span>   <!-- one per page -->
<span class="tg">crdt</span>                          <!-- many per page -->
<span class="tg tg-on">crdt</span>                    <!-- filter active -->
```

`.tpc` requires the empty `<i></i>` — the 5px colour square. `.tg` renders its
`#` via `::before`; **do not type the `#`**.

**Topic vs tag is a real modelling distinction, not two styles.** A topic is
singular, owned, a column — it clusters the graph and scopes Discover. A tag
is free-form, many, hueless — it facets search and never boosts rank. If you
collapse them into one field you get folders, and a page that is genuinely two
things has to lie.

### 6.6 Avatars

```html
<div class="av av-you">GN</div>     <!-- teal   — you -->
<div class="av av-them">AD</div>    <!-- violet — another person -->
<div class="av av-ai">✦</div>       <!-- slate  — the assistant -->
```

23×23px default; `17–19px` inline in dense rows; `56px` on a profile. Square,
1px border, 16% background wash. The assistant is always `✦`, never initials.

### 6.7 Bar charts and sparklines

Built from divs, never a chart library:

```html
<!-- vertical -->
<div style="display:flex;align-items:flex-end;gap:2px;height:44px">
  <div style="flex:1;height:35%;background:rgba(232,135,60,.3)"></div>
  <div style="flex:1;height:88%;background:#E8873C"></div>
</div>

<!-- horizontal, with value -->
<div style="display:flex;align-items:center;gap:9px">
  <span style="flex:1;font-size:11.5px;color:#9B968D">Protocol</span>
  <div style="width:64px;height:4px;background:rgba(255,255,255,.06)">
    <div style="width:82%;height:100%;background:#7AA8E8"></div>
  </div>
  <span class="mono" style="font-size:9.5px;color:#8C8880;width:34px;text-align:right">.82</span>
</div>

<!-- proportional stack -->
<div style="display:flex;height:9px;gap:1px">
  <div style="flex:38;background:#7AA8E8"></div>
  <div style="flex:26;background:#C48AE0"></div>
</div>
```

Track height is `3–5px`. Bar opacity encodes recency or magnitude. Values are
right-aligned mono in a fixed-width span so columns line up.

---

## 7. Motion

Six motions exist. **Adding a seventh requires a reason.**

| Animation | Duration | Where |
|---|---|---|
| `blink` | `1.1s steps(1,end)` | carets only |
| `pulse` | `2.2s ease-out` | presence ping — **on join only, never idling** |
| `rise` | one-shot | lists on mount, 40ms stagger |
| `ticker` | `8s linear` | the op stream in the status bar |
| `shimmer` | `1.4s ease-in-out` | determinate progress |
| `spin` | `.9s linear` | **the only spinner in the app** — reconnect |

Rules:

- Content reveals on mount. **Chrome never does.**
- **Nothing animates on scroll.**
- Everything pressable moves half a pixel on `:active`.
- **Any motion over 320ms is a bug.**
- `prefers-reduced-motion` drops all of it to a cross-fade.

---

## 8. Layout patterns

### 8.1 Fill the frame — and measure it

The reference has **no accidental empty regions**. This was enforced by
measuring, not by eye:

```js
// For each column: distance from its last rendered child to its own bottom.
const gap = column.getBoundingClientRect().bottom - lastChildBottom;
if (gap > 150) { /* underfilled — add content or restructure */ }
```

Run the equivalent check on your implementation. A column with 400px of dead
space at the bottom is the most common visible difference between a faithful
implementation and an approximate one.

**The inverse also matters:** content overflowing its own frame gets cut
mid-sentence. Measure that too. Panels should land within ~50px of their
bottom edge.

**Three regions are deliberately empty** and must stay that way: the palette's
dimmed backdrop, the login card's centring, and the components grid.

### 8.2 Standard closing block

Most main columns end with a 2–3 column grid pinned to the bottom, separated
by a rule. This is the reference's signature layout move:

```html
<div style="margin-top:auto;padding-top:22px;border-top:1px solid rgba(255,255,255,.07);
            display:grid;grid-template-columns:1fr 1fr 1.1fr;gap:26px">
  <div><div class="lbl" style="margin-bottom:11px">METRIC GROUP</div>…</div>
  <div><div class="lbl" style="margin-bottom:11px">SECOND GROUP</div>…</div>
  <div><div class="lbl" style="margin-bottom:10px">WHY THIS WORKS THIS WAY</div>
       <div style="font-size:11.5px;line-height:1.7;color:#8C8880">…</div></div>
</div>
```

The final cell is **almost always prose explaining a decision**, not more data.

### 8.3 Floating panels over a canvas

```css
.float {
  position: absolute; z-index: 5;
  background: var(--glass-bg);
  backdrop-filter: blur(var(--glass-blur)) saturate(1.5);
  border: 1px solid var(--rule-strong);
  box-shadow: var(--shadow-2), var(--glass-edge);
}
```

Anchored to corners with `16–22px` insets. Used on Graph and Graph algorithms.

### 8.4 Overlays

The palette and notification panel are **overlays, not routes**. The screen
behind stays exactly where it was, blurred `1.2px` at `opacity: .5`, under a
`rgba(6,7,8,.72)` scrim. Escape returns you to the caret you left — which is
the only reason it is safe to reach for mid-sentence.

The palette is centred at `top: 76px`, `width: 748px`. The notification panel
is anchored **under its own trigger** at `right: 76px; top: 52px` — a panel
that drops from its trigger tells you what opened it without a title bar
saying so.

---

## 9. Content rules — this is half the design

**Every string in the reference is a specific, true claim about the system.**
This is not decoration; it is the point. Generic copy is the single loudest
signal that an implementation is approximate.

### 9.1 Never write these

| Don't | Do |
|---|---|
| "Lorem ipsum" | Real prose about the actual subject |
| "Item 1", "Card title" | The real name of the real thing |
| "Settings", "Options" | "Per-topic defaults", "Startup vs runtime" |
| "Loading…" as a permanent state | The real state, with a number |
| "An error occurred" | What happened and what to do — and **never in red** |
| "No data" | What would put data here, and how to do it |
| Round marketing numbers (100, 1000) | Specific ones: `1 908`, `12 402`, `41 ms` |

### 9.2 The explanatory note

Nearly every panel ends with one or two sentences at
`font-size:11px; line-height:1.6; color:#585550` that state **why the design
is this way**, not what it does. Examples from the reference:

> A tag that lives in three topics is doing real work — it names a technique,
> not a subject.

> Suggestion runs the same embedding index Discover uses and proposes the
> nearest topic. It never assigns one — a wrong topic is worse than none,
> because none is visibly none.

> Momentum is reads, not writes. A tag can climb while nobody edits it — which
> is precisely the week to go and check whether what it points at is still
> true.

These notes carry the argument. **A panel without one is usually a panel that
has not decided what it is for.** When implementing, write the note first; if
you cannot, the panel probably should not exist.

### 9.3 State honestly

The reference shows problems rather than hiding them, and this is a **design
requirement**:

- `◌ index lag 2.4 s — results may trail the tree`
- `◌ 62 untopiced · suggestion available, assignment is yours`
- `◌ 1 key unused for 94 days`
- `◌ p99.9 62 ms — GC pause, not the wire`
- `◌ 1 delete resuming from step 3 after a restart`

Never smooth these over. A UI that admits its index lags is more trustworthy
than one that implies a transaction it does not have.

### 9.4 Mark what is not built

If a nav lists more routes than exist, **mark which is which** — full contrast
with a `→` for drawn destinations, `opacity: .45` for the rest, plus a legend
stating the count. A nav that quietly omits what is unfinished is how a design
starts lying about its own coverage.

---

## 10. Implementation checklist

Work through this in order. Do not skip to components.

**Foundation**
- [ ] Three fonts linked with the exact weights in §2
- [ ] Palette tokens from §3 — **no additional colours**
- [ ] Global `border-radius: 0`; pill radius only on chips/toggles/badge
- [ ] Base classes copied verbatim from `index.html`'s `<style>` block

**Shell**
- [ ] `.bar` / `.sub` / `.body` / `.status` with correct fixed heights
- [ ] `min-height: 0` on `.body`, `min-width: 0` on the main column
- [ ] Utility cluster in the fixed order, on in-app screens only
- [ ] Status bar with route + mechanism + honest state on every screen

**Per screen**
- [ ] Find the same screen in `index.html` and copy its structure
- [ ] Real content — no placeholders, specific numbers (§9)
- [ ] Explanatory note at the end of each panel (§9.2)
- [ ] Standard closing grid on the main column (§8.2)
- [ ] Topic/tag chips wherever a page is shown

**Verification — do this, do not eyeball it**
- [ ] Measure bottom gaps; nothing over 150px unless deliberate (§8.1)
- [ ] Measure overflow; panels within ~50px of their bottom edge
- [ ] Screenshot beside the reference screen and diff them by eye
- [ ] Grep your output for `border-radius` — should only match the pill cases
- [ ] Grep for hex colours — every one should be in §3
- [ ] Grep for `Lorem`, `Item `, `Example`, `TODO`, `placeholder` — zero hits

---

## 11. When the spec runs out

If you need something this document does not cover:

1. **Search `index.html` first.** 40 screens; the pattern probably exists.
2. If it genuinely does not, derive from the nearest neighbour rather than
   inventing — a new panel looks like existing panels.
3. **Adding a colour, a radius, a font, or a motion is a spec change**, not an
   implementation detail. It needs a stated reason and it belongs in this
   document.

The reference is the authority. This document explains it; it does not
replace it.
