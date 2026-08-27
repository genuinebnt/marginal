#!/usr/bin/env node
// Seeds a real, interesting [[link]] graph through the actual live
// pipeline — REST page creation + real WebSocket block content carrying
// [[Page Title]] text, the same path a real user's typing takes through
// collaboration-service -> collab.ops_flushed -> blockproj -> docs.page_links
// (internal/blockproj's own regex scanner resolves these for real; this
// script never writes to docs.page_links directly).
//
// Run against a running `docker compose up` stack:
//   node scripts/seed-graph-demo.mjs
//
// Deliberately shaped to exercise every internal/graphalgo algorithm
// (v2.2.0, docs/planning/RELEASES.md) with something genuinely there to
// find:
//   - a "Home" hub, reachable from a few other root pages
//   - a plain 3-cycle (Alpha/Beta/Gamma) — Betti's own "filling this
//     triangle kills its one loop" case
//   - a plain 4-cycle (Corner 1..4, no diagonal) — a loop nothing fills
//   - a hollow tetrahedron (Node W/X/Y/Z, K4, every face filled) — the
//     one shape with a genuinely nonzero β₂
//   - a 5-step chain off Home — gives BFS/diameter/wavefront something
//     to walk
//   - an orphaned mutually-linked pair, nested under "Archive" (not a
//     root itself) so it's actually unreachable, not just "no parent"
//   - one isolated page with no links at all, same nesting reasoning

const GATEWAY = process.env.GATEWAY_URL ?? "http://localhost:8000";
const COLLAB = process.env.COLLAB_URL ?? "http://localhost:8002";
const EMAIL = process.env.SEED_EMAIL ?? "graph-demo@example.com";
const PASSWORD = "correct horse battery staple";

async function jsonFetch(url, opts = {}) {
  const res = await fetch(url, {
    ...opts,
    headers: { "Content-Type": "application/json", ...(opts.headers ?? {}) },
  });
  const text = await res.text();
  const body = text ? JSON.parse(text) : undefined;
  if (!res.ok) throw new Error(`${opts.method ?? "GET"} ${url} -> ${res.status}: ${text}`);
  return body;
}

function actorIdFromToken(accessToken) {
  const payload = JSON.parse(Buffer.from(accessToken.split(".")[1], "base64url").toString("utf-8"));
  return payload.sub;
}

async function registerOrLogin() {
  try {
    const tokens = await jsonFetch(`${GATEWAY}/auth/register`, {
      method: "POST",
      body: JSON.stringify({ email: EMAIL, password: PASSWORD, display_name: "Graph Demo" }),
    });
    console.log(`registered ${EMAIL}`);
    return tokens;
  } catch (err) {
    console.log(`register failed (${err.message.split("\n")[0]}), trying login instead`);
    return jsonFetch(`${GATEWAY}/auth/login`, {
      method: "POST",
      body: JSON.stringify({ email: EMAIL, password: PASSWORD }),
    });
  }
}

async function createPage(actorId, title, parentId) {
  const page = await jsonFetch(`${GATEWAY}/pages`, {
    method: "POST",
    headers: { "X-Actor-Id": actorId },
    body: JSON.stringify({ title, parent_id: parentId }),
  });
  return page;
}

/** Opens a real WebSocket to collaboration-service, inserts one block
 * whose text carries [[links]] to every title in `mentions`, waits for
 * the ack, and closes — the exact path a real user typing those links
 * would take. */
function writeLinkingBlock(actorId, pageId, mentions) {
  return new Promise((resolve, reject) => {
    const url = new URL(`${COLLAB}/collab/pages/${pageId}`);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("actor_id", actorId);
    const ws = new WebSocket(url);
    const timeout = setTimeout(() => {
      ws.close();
      reject(new Error(`timed out writing links for page ${pageId}`));
    }, 10000);

    let sentOp = false;
    ws.addEventListener("message", (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === "snapshot" && !sentOp) {
        sentOp = true;
        const blockId = crypto.randomUUID();
        const text = mentions.length ? `See also ${mentions.map((m) => `[[${m}]]`).join(", ")}.` : "";
        ws.send(
          JSON.stringify({
            type: "op",
            op: {
              scope: "block",
              type: "InsertBlock",
              id: blockId,
              parent: null,
              after: null,
              kind: { tag: "paragraph" },
              content: { text },
            },
          }),
        );
      } else if (msg.type === "ack") {
        clearTimeout(timeout);
        ws.close();
        resolve();
      } else if (msg.type === "error") {
        clearTimeout(timeout);
        ws.close();
        reject(new Error(`collab error for page ${pageId}: ${msg.message}`));
      }
    });
    ws.addEventListener("error", (err) => {
      clearTimeout(timeout);
      reject(err);
    });
  });
}

async function main() {
  const tokens = await registerOrLogin();
  const actorId = actorIdFromToken(tokens.access_token);
  console.log(`actor_id = ${actorId}`);

  const byTitle = {};
  async function page(title, parentTitle) {
    const parentId = parentTitle ? byTitle[parentTitle].id : undefined;
    const p = await createPage(actorId, title, parentId);
    byTitle[title] = p;
    return p;
  }

  console.log("creating pages...");
  await page("Home");
  await page("Architecture");
  await page("Runbook");
  for (const t of ["Alpha", "Beta", "Gamma"]) await page(t);
  for (const t of ["Corner 1", "Corner 2", "Corner 3", "Corner 4"]) await page(t);
  for (const t of ["Node W", "Node X", "Node Y", "Node Z"]) await page(t);
  for (let i = 1; i <= 5; i++) await page(`Step ${i}`);
  await page("Archive");
  await page("Orphan A", "Archive");
  await page("Orphan B", "Archive");
  await page("Forgotten Page", "Archive");

  const links = [
    ["Home", ["Architecture", "Runbook", "Alpha", "Corner 1", "Node W", "Step 1"]],
    ["Alpha", ["Beta"]],
    ["Beta", ["Gamma"]],
    ["Gamma", ["Alpha"]],
    ["Corner 1", ["Corner 2"]],
    ["Corner 2", ["Corner 3"]],
    ["Corner 3", ["Corner 4"]],
    ["Corner 4", ["Corner 1"]],
    ["Node W", ["Node X", "Node Y", "Node Z"]],
    ["Node X", ["Node W", "Node Y", "Node Z"]],
    ["Node Y", ["Node W", "Node X", "Node Z"]],
    ["Node Z", ["Node W", "Node X", "Node Y"]],
    ["Step 1", ["Step 2"]],
    ["Step 2", ["Step 3"]],
    ["Step 3", ["Step 4"]],
    ["Step 4", ["Step 5"]],
    ["Orphan A", ["Orphan B"]],
    ["Orphan B", ["Orphan A"]],
    ["Architecture", []],
    ["Runbook", []],
    ["Forgotten Page", []],
  ];

  console.log("writing linking blocks over real WebSocket connections...");
  for (const [from, mentions] of links) {
    await writeLinkingBlock(actorId, byTitle[from].id, mentions);
    console.log(`  ${from} -> [${mentions.join(", ")}]`);
  }

  console.log("waiting for blockproj to materialize docs.page_links...");
  let analysis;
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => setTimeout(r, 500));
    const graph = await jsonFetch(`${GATEWAY}/graph`, { headers: { "X-Actor-Id": actorId } });
    if (graph.edges.length >= links.reduce((n, [, m]) => n + m.length, 0)) {
      analysis = await jsonFetch(`${GATEWAY}/graph/analysis`, { headers: { "X-Actor-Id": actorId } });
      break;
    }
  }

  console.log(`\nseeded ${Object.keys(byTitle).length} pages.`);
  if (analysis) {
    console.log("graph analysis:", JSON.stringify(analysis, null, 2));
  } else {
    console.log("warning: edges did not fully materialize within the wait window — check blockproj/NATS.");
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
