# ops

Host units for the VPS, installed from this checkout so the box stays
reproducible from a commit.

## Nightly reseed

The showcase workspace is shared by every account — `ADR-001` keeps the product
non-multi-tenant, and there is no authorization on destructive operations, so
anyone who registers can edit or delete the seeded pages. Rather than lock that
down (which `docs/api/pages.md` records was tried and reverted, because owner
scoping broke sharing), the workspace is simply rebuilt every night.

### Install

```sh
cd ~/marginal && git pull

# the demo account the seeder logs in as
printf 'SEED_EMAIL=ui-demo@example.com\nSEED_PASSWORD=<password>\n' >> .env

sudo install -m644 ops/reseed.service ops/reseed.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now reseed.timer
```

### Check on it

```sh
systemctl list-timers reseed.timer     # when it next runs
journalctl -u reseed.service -n 40     # what the last run did
sudo systemctl start reseed.service    # run it now
```

A successful run ends with `seeded 38 pages (all)`. It clears first, so the page
count should stay flat rather than climbing.

## Deploying

`docker-compose.prod.yml` is **standalone, not an override.** It declares
every service itself, so deploy with one `-f`:

```sh
cd ~/marginal && git pull
BUILD_ID=$(git rev-parse --short HEAD) docker compose -f docker-compose.prod.yml up -d --build
```

Merging it onto the base file (`-f docker-compose.yml -f
docker-compose.prod.yml`) *looks* right and breaks the site: the base's
`web` is the Vite dev server, so its `image: node:22-alpine` and
`command: npm install && npm run dev` survive the merge and are applied
to the built static image, which has no npm. The container restart-loops
on `sh: npm: not found` and every HTML route 502s while the wasm files
keep returning 200 — the confusing part, since Caddy is up and only the
container behind it is gone.

`BUILD_ID` is what makes each build's wasm URLs unique so they can be
cached `immutable`; it defaults to the date, which degrades to a daily
cache rather than a stale-forever one.

### After a deploy

```sh
curl -sI https://marginal.genuinebasil.dev/ | grep -i cache-control   # no-cache, must-revalidate
curl -so /dev/null -w '%{http_code} %{content_type}\n' \
  https://marginal.genuinebasil.dev/documentcore.wasm                 # 200 application/wasm
```

All nine wasm modules should answer `application/wasm`: documentcore,
graph, diff, trie, syntax, mdc, sketch, netsim, bench. A missing one
serves `index.html` instead, and the browser reports it as a bad magic
word (`<!do`) rather than a 404.
