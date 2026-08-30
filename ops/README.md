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
