#!/usr/bin/env bash
# Seeds topics and tags across the existing pages THROUGH THE REAL gRPC API
# (never raw SQL) — the same path the UI will use, so a seeding bug is a real
# bug rather than a fixture that only exists in the database.
set -euo pipefail

A=01a046a9-d238-7269-829f-5d5fb6427e7e   # actor
H="actor-id: $A"
S=marginal.document.v1.PageService
E=localhost:9001

PROTOCOL=018f2b1c-0000-7000-8000-0000000000a1
STORAGE=018f2b1c-0000-7000-8000-0000000000a2
INTERFACE=018f2b1c-0000-7000-8000-0000000000a3
OPERATIONS=018f2b1c-0000-7000-8000-0000000000a4
RESEARCH=018f2b1c-0000-7000-8000-0000000000a5

topic() { grpcurl -plaintext -H "$H" -d "{\"page_id\":\"$1\",\"topic_id\":\"$2\"}" $E $S/SetPageTopic >/dev/null; }
tag()   { grpcurl -plaintext -H "$H" -d "{\"page_id\":\"$1\",\"tag\":\"$2\"}"      $E $S/AddPageTag   >/dev/null; }

# page-id topic tag...
# Left deliberately uneven: some pages carry three tags, some one, and two
# are left untopiced on purpose so "62 untopiced" has a real analogue and the
# untopiced state is visible rather than theoretical.
while IFS='|' read -r id t tags; do
  [ -z "$id" ] && continue
  [ "$t" != "-" ] && topic "$id" "$t"
  for g in $tags; do tag "$id" "$g"; done
  echo "  $id"
done <<EOF
01a046a9-d245-7bb1-a1b8-78cb87e951d9|$PROTOCOL|crdt sync-protocol ot
01a046a9-d238-7269-829f-5d5fb6427e7e|$INTERFACE|overview onboarding
01a046a9-d24e-73ad-8fdf-c196414ab94d|$OPERATIONS|runbook oncall incident
01a046aa-2bfa-7e4a-9633-6168a461bd43|$OPERATIONS|latency benchmarks profiling
01a046a9-d259-7b93-9e4f-183acbc5acd3|$RESEARCH|crdt experiment
01a046a9-d263-7e5b-89f8-94af430b0309|$RESEARCH|experiment
01a046a9-d26d-7976-b2eb-d3f8a77a798f|$RESEARCH|crdt convergence
01a046a9-d271-7f01-9a8b-8543267355bd|$STORAGE|lsm compaction
01a046a9-d278-712a-af83-b30e5ae1846a|$STORAGE|wal durability
01a046a9-d282-7e45-91f9-efd55d4d2e08|$STORAGE|lsm indexing
01a046a9-d28a-71f5-b1ea-22f7827fe50c|$STORAGE|compaction
01a046a9-d28d-7ac5-8273-ab8fa996e1b8|$PROTOCOL|anchors rope
01a046a9-d290-780f-a124-e519835a9c12|$PROTOCOL|anchors ot
01a046a9-d293-70d1-b3ed-41831c694318|$PROTOCOL|rope
01a046a9-d296-7935-a2e0-7d0133c4f200|$INTERFACE|editor blocks
01a046a9-d298-7e30-a55f-ca722728d001|$INTERFACE|editor onboarding
01a046a9-d29c-700d-af0f-e8b95c37d2b4|$INTERFACE|blocks
01a046a9-d29e-7db9-b06e-d9347616bde1|$INTERFACE|editor marks
01a046a9-d2a2-74c5-91af-1423dc95e1f6|$OPERATIONS|deploy
01a046a9-d2a4-795d-b2ac-9554f195d0bf|$OPERATIONS|deploy incident
01a046a9-d2a7-71cf-9de3-ef355772074f|$RESEARCH|archive
01a046a9-d2a9-77ff-acde-0d419d84d6ea|-|orphan
01a046a9-d2ac-7c5f-baed-ab67274676ce|-|orphan
01a046a9-d2af-778d-af7a-02f2e83c3693|$STORAGE|archive durability
EOF

echo "seeded"
