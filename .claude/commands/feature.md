Design the architecture for the feature described. Update all required documentation before any code is written. Give me the blueprint — I will implement it.

Per agents.md: never write production Rust implementations. Provide patterns, resources, and the design. Use non-Rust code examples only to illustrate patterns.

---

## Step 1 — Identify the feature scope

From the conversation context (or argument passed to this command):
- Which service(s) does this feature touch? (`auth-service`, `document-service`, etc.)
- Is this a new endpoint, a new domain concept, a background worker, or a cross-service flow?
- Which phase does this belong to per `docs/planning/ROADMAP.md`?

---

## Step 2 — Update `docs/planning/FEATURE_LIST.md`

Add the feature to the correct phase and service section. If it is a new sub-task under an existing phase, add it as a checklist item. If it is a new phase feature, add the full section.

---

## Step 3 — Update `docs/architecture/DATA_MODEL.md`

If this feature requires a schema change (new table, new column, new index, new LTREE path, new JSONB field):
- Update the Mermaid ER diagram
- Add the table definition (columns, types, constraints, indexes)
- Note the migration file name that will implement it (`migrations/<timestamp>_<name>.sql`)

If no schema change is needed, state that explicitly.

---

## Step 4 — Update or create `docs/api/<service>.md`

For every new or modified HTTP endpoint:
- Method + path
- Auth requirement (JWT required, guest token, public)
- Request body (JSON shape with field types)
- Success response (status code + JSON shape)
- Error responses (status codes + error variants)
- gRPC RPC signature if this involves a `libs/proto` change

---

## Step 5 — Architecture blueprint

Provide:

### Clean Architecture placement
Where does each new type live?
```
Presentation layer:  handler name, route, extractor types
Domain layer:        entity/struct names, repository trait methods, use-case struct
Infrastructure layer: concrete impl name, external dependency used
```

### NATS events (if any)
- Topic name (e.g., `docs.page_created`)
- Publisher service
- Subscriber services and what they do

### Relevant Rust patterns
Name the patterns that apply and link to where I can learn them (use the resource library in `.agents/agents.md`):
- e.g., Typestate, Builder, Repository trait, Outbox pattern, etc.

### DSA concepts encountered
Name the data structure or algorithm this feature naturally exercises, link to a visualization or reference, and describe the operations — per agents.md DSA guidance.

### Potential failure modes
In a distributed system, what can go wrong? (network partition, duplicate delivery, race condition, permission escalation vector) — name each and the pattern that handles it.

---

## Step 6 — Create an ADR if needed

If this feature involves a major architectural decision (new technology, new communication pattern, new consistency model), create a new ADR in `docs/architecture/adr/ADR-00N-<slug>.md` following the existing ADR format.

---

## Step 7 — Actionable next step

End with: "Now implement X" — the single most important first thing to build, sized so it can be done in one session.
