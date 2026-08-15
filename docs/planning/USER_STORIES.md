# User Stories

What Marginal does, told from the outside. `ROADMAP.md` says what to *build* and in what
order; this says what someone can *do* once it is built, and what "done" means for each one.

**How to use it.** Pick the next unfinished story in execution order, and stop when its
*Done when* line is true. If a story cannot be finished without starting the next one, they
were split wrong — merge them and say so here.

Every story carries its phase. If you find yourself building something that has no story,
either write the story first or you are gold-plating.

---

## Personas

| | Who they are | What they care about |
|---|---|---|
| **Author** | Writes in their own space. The default user. | Getting a thought down without the tool interrupting |
| **Collaborator** | Edits the same page as someone else, live | Never losing their work, never seeing a merge dialog |
| **Reader** | Has a link. May not have an account | Reading comfortably, finding their way back |
| **Operator** | Runs the self-hosted instance. Often the Author too | It stays up, it backs up, it does not surprise them |

A fifth, the **Space Admin**, appears at Phase 13 and not before — until then every user has
every permission on their own content.

---

## Track 1 — MVP

Phases 1 → 2 → 3. The finish line: *log in, write a page, edit it live with someone else.*
Nothing in Tracks 4 or 5 starts before this is true (ADR-009 § Guard Rails).

### Documents — Phase 1

| # | Story | Done when |
|---|---|---|
| **D-01** | As an Author, I can create a page so I have somewhere to write | A new page exists, is empty, and reopening the app still shows it |
| **D-02** | As an Author, I can type into a block and my text is saved without me asking | Text survives a reload. No save button exists |
| **D-03** | As an Author, I can press Enter to start a new block and Backspace on an empty one to remove it | Block count changes; the caret lands where a writer would expect |
| **D-04** | As an Author, I can turn a block into a heading, quote, list item or code block by typing its prefix | `## ` becomes a heading and the `## ` characters are gone. Cmd+Z restores the literal text before undoing the typing |
| **D-05** | As an Author, I can apply bold, italic, code and links inline, by shortcut or by typing the delimiters | The mark applies, the delimiters vanish, and re-editing the text does not re-trigger the rule |
| **D-06** | As an Author, I can reorder blocks by dragging | The new order survives a reload, and moving one block does not rewrite the others |
| **D-07** | As an Author, I can nest pages under other pages and see the tree in a sidebar | The tree renders to arbitrary depth and reflects moves immediately |
| **D-08** | As an Author, I can paste from a browser, Word or Google Docs and get clean blocks | No style soup, no scripts, no `<o:p>`. Structure that maps to a block kind survives; everything else degrades to a paragraph |
| **D-09** | As an Author, I can drop an image into a page and see it inline | The file uploads to object storage and the page holds a reference, not the bytes |
| **D-10** | As an Author, I can delete a page and it stops appearing anywhere | Gone from the tree, and from anything downstream that had indexed it |

### Accounts — Phase 2

| # | Story | Done when |
|---|---|---|
| **A-01** | As an Operator, the first screen on a fresh instance sets up my account, not a login I cannot pass | A brand-new instance is usable by exactly one person with no seeded credentials |
| **A-02** | As an Author, I can register, log in, and stay logged in across restarts | A session survives a browser restart and a server restart |
| **A-03** | As an Author, I can log out, and that session cannot be used again | The old token is refused even before it expires |
| **A-04** | As an Author, only I can read or change my pages | Another account gets a refusal, and the refusal does not reveal whether the page exists |

### Collaboration — Phase 3

| # | Story | Done when |
|---|---|---|
| **C-01** | As a Collaborator, I can open a page someone else is editing and see their changes as they type | Both screens converge with no refresh and no prompt |
| **C-02** | As a Collaborator, I never see a merge conflict, a version chooser, or a "someone else edited this" dialog | No such UI exists anywhere in the product |
| **C-03** | As a Collaborator, I can see where other people are — their caret, their selection, their name | Presence appears within a second of them arriving and disappears when they leave |
| **C-04** | As a Collaborator, my typing stays responsive on a bad connection | Characters appear immediately and reconcile afterwards; the caret does not jump under a remote edit |
| **C-05** | As a Collaborator, I can lose my connection, reconnect, and keep my work | Offline edits land after reconnect and nothing typed is lost |
| **C-06** | As a Collaborator, my edit means the same thing after it merges as it did when I typed it | Two people editing one paragraph produce the text both intended, not merely the same text |

🏁 **The MVP gate.** D-01…D-05, A-01…A-04, C-01…C-04 all true at once.

---

## Track 2 — The differentiators

Phases 4 → 5 → 6. These are why Marginal exists rather than being another editor.

| # | Story | Phase | Done when |
|---|---|---|---|
| **G-01** | As an Author, I am shown problems in my prose as I write — never as errors, always as hints | 4 | Amber, inline, dismissible. Nothing is ever marked "broken" |
| **G-02** | As an Author, diagnostics keep up with my typing and never block it | 4 | Editing stays smooth with the analyzer running; only what changed is re-analysed |
| **G-03** | As an Author, I can turn an individual analyzer off and it stays off | 4 | The choice persists and is per-user |
| **U-01** | As a Collaborator, undo reverts *my* last change, not whatever happened most recently | 5 | Undoing after someone else types reverts my work and leaves theirs |
| **U-02** | As an Author, undoing a run of typing removes the run, not one character | 5 | One Cmd+Z after typing a word removes the word |
| **U-03** | As an Author, redo restores exactly what undo removed | 5 | Undo/redo round-trips to the identical document |
| **H-01** | As an Author, I can scrub a page back through time and watch it change | 6 | Scrubbing is instant; the document itself morphs rather than a diff opening in a panel |
| **H-02** | As an Author, I can see who changed what, and when | 6 | Each change is attributable to a person and a moment |
| **H-03** | As an Author, I can restore an earlier version | 6 | Restoring is itself an ordinary change — undoable, and it does not erase the history it came from |

---

## Track 3 — Distributed

Phases 7 → 8 → 9 → 10. Mostly invisible. The stories are the *user-visible* consequence,
because "we added a gateway" is not something anyone can want.

| # | Story | Phase | Done when |
|---|---|---|---|
| **S-01** | As an Author, I can search everything I have written and get results as I type | 7 | Results appear per keystroke on a real corpus |
| **S-02** | As an Author, I can link pages with `[[name]]` and see what links back | 7 | Backlinks are listed on the target page and update when a link is added or removed |
| **S-03** | As an Author, search tolerates my typos | 7 | A misspelled query still finds the page |
| **S-04** | As an Author, a page I just wrote is findable, and I am told if the index is briefly behind | 7 | Staleness is surfaced, not hidden |
| **X-01** | As an Author, deleting a page removes it everywhere, or nowhere | 8 | A failure mid-delete leaves no half-deleted state visible to anyone |
| **X-02** | As an Author, the app is one address, not eleven | 9 | Every client call goes to a single origin |
| **X-03** | As a Collaborator, joining a busy page is as fast as joining a quiet one | 10 | Everyone on a page reaches the same instance, and that fact is invisible |

---

## Track 4 — Platform

Phases 13 → 14 → 15 → 16 → 20. **Gated on the 🏁** (ADR-009 § Guard Rails).

| # | Story | Phase | Done when |
|---|---|---|---|
| **R-01** | As a Space Admin, I can group pages into a space and decide who may read or write it | 13 | Permission is enforced at the edge, not in the UI |
| **R-02** | As an Author, I can share one page without sharing the space around it | 13 | The recipient sees exactly that page |
| **M-01** | As a Reader, I can comment on a passage and the comment stays attached to it as the text moves | 14 | Editing the surrounding paragraph does not orphan the thread |
| **M-02** | As a Collaborator, I can resolve a thread and it stops competing for attention without being destroyed | 14 | Resolved threads are retrievable |
| **M-03** | As a Collaborator, I can @mention someone and they find out | 14 | The mention resolves to a person and produces a notification |
| **N-01** | As an Author, I can see what happened while I was away, and clear it | 15 | An inbox with a read state; a lost notification costs nothing |
| **N-02** | As an Author, I can decide what is worth notifying me about | 15 | Per-category, persisted |
| **E-01** | As an Author, I can use tables, callouts, footnotes, math and embeds | 16 | Each is a block kind, each survives a reload and a paste |
| **E-02** | As an Author, I can reach any command from the keyboard | 16 | ⌘K finds pages, blocks and actions without the mouse |
| **T-01** | As an Operator, I can change instance settings without editing a file or restarting | 20 | Runtime settings apply live; startup-only settings say so plainly |

---

## Track 5 — Reach

Phases 17 → 18 → 19 → 21. **Also gated on the 🏁.**

| # | Story | Phase | Done when |
|---|---|---|---|
| **P-01** | As an Author, I can publish a page to a public URL that needs no account | 17 | An anonymous visitor can read it, and it is fast |
| **P-02** | As a Reader, I can subscribe to an author's published pages | 17 | A feed exists and updates |
| **L-01** | As an Author, I can install a plugin and it cannot damage my notebook | 18 | A misbehaving plugin degrades itself and nothing else |
| **I-01** | As an Author, I can ask a question about my own notes and get an answer grounded in them | 19 | Answers cite the pages they came from |
| **I-02** | As an Author, the assistant is never in my way while writing | 19 | It never sits on the editing path; if it is down, writing is unaffected |
| **I-03** | As an Author, I can find a page by describing it rather than quoting it | 19 | Semantic results, alongside exact ones |
| **K-01** | As an Author, I am shown pages related to the one I am writing | 21 | Suggestions are relevant and dismissible |

---

## Track 6 — Operations

Phases 11 → 12. Interleaved throughout, not saved for the end — every phase deploys its own
service (`CLOUD_ROADMAP.md` §2).

| # | Story | Phase | Done when |
|---|---|---|---|
| **O-01** | As an Operator, I can stand up the whole stack with one command | 11 | A clean machine to a working instance, no manual steps |
| **O-02** | As an Operator, I get a self-hosted instance with every feature, not a cut-down build | 11 | Feature parity with the hosted deployment (ADR-001) |
| **O-03** | As an Operator, I can back up and restore, and I have tested the restore | 11 | A restore drill is documented and has actually been run |
| **O-04** | As an Operator, I can see whether the instance is healthy and what is slow | 12 | Outbox depth and op-log lag are visible — the two that matter |
| **O-05** | As an Operator, I can trace one user's slow request across every service it touched | 12 | One trace, end to end |

---

## Not stories

Out of scope per ADR-001, and deliberately absent. If one of these turns up in a request it
needs an ADR before it needs a story.

- Databases, tables, views, relations, rollups — the hard one: `docs.ops.page_id` is
  `NOT NULL` and `collaboration-service` owns exactly one page per instance, so cross-page
  aggregation has no owner. That is a second ownership tier, not a feature
- A formula language
- A spatial/infinite canvas
- Native mobile apps

---

## Keeping this honest

- A story with no *Done when* that could fail is not a story, it is a wish.
- A story that needs a paragraph of explanation is two stories.
- When a story ships, the mockup that drew it and the phase that owned it should both still
  describe it correctly — if not, one of the three is wrong (`.agents/agents.md`
  § Continuous Documentation).
