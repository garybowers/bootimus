# Multi-Site Deployments

Run one Bootimus as the **primary** where you manage everything, and a **replica** at each branch site that mirrors the primary and boots local clients from local copies. Replicas are read-only: every change is made on the primary and arrives at the replicas on their next sync.

## Table of Contents

- [Topology](#topology)
- [Set up the primary](#set-up-the-primary)
- [Set up a replica](#set-up-a-replica)
- [What syncs and what does not](#what-syncs-and-what-does-not)
- [Serving ISOs locally or from the primary](#serving-isos-locally-or-from-the-primary)
- [Bandwidth](#bandwidth)
- [When the primary is unreachable](#when-the-primary-is-unreachable)
- [Monitoring](#monitoring)
- [API](#api)

## Topology

```
                 HQ                                    Branch office
  +------------------------------+          +------------------------------+
  | Bootimus primary             |  HTTPS   | Bootimus replica             |
  | admin :8081  <---------------+----------+ pulls config and assets      |
  | boot  :8080                  |  every   | admin :8081 (read-only UI)   |
  | all changes are made here    |  5 min   | boot  :8080 serves clients   |
  +------------------------------+          +------------------------------+
                                                     | PXE / TFTP / HTTP
                                                     v
                                               branch clients
```

Each replica keeps its own database and data directory, so a branch keeps booting from its last good copy even when the WAN link is down. Replicas never write to the primary.

## Set up the primary

Nothing changes on the primary beyond creating a token for each replica:

1. Open **Users** → **API Tokens** and create a token named after the site.
2. Choose the scope:
   - `read` is enough for clients, images, groups, SAN targets, the theme, custom profiles, tools and custom files.
   - `admin` additionally syncs **auto-install files**, BMC and CHAP passwords, and boot password verifiers. Use this scope if any boot menu or image needs password protection. With a `read` token, existing protected boots keep their last synced policy; new protected boots cannot be unlocked at the replica.
3. Copy the `btk_...` secret; it is shown once.

Serve the primary's admin interface over HTTPS (`--admin-tls`, see the [deployment guide](deployment.md#securing-the-admin-interface)). A replica only accepts boot password verifiers over HTTPS with normal certificate validation; install a certificate trusted by the replica host. Each replica also needs its own admin HTTPS listener with a certificate trusted by its boot clients, since they submit passwords to that replica.

## Set up a replica

Install Bootimus at the branch as usual, then point it at the primary:

```bash
bootimus serve \
  --replica-primary https://bootimus.hq.example.com:8081 \
  --replica-token btk_... \
  --replica-interval 5m
```

Or in `bootimus.yaml`:

```yaml
replica:
  primary_url: https://bootimus.hq.example.com:8081
  token: btk_...
  interval: 5m
  sync_isos: true
```

The environment variables are `BOOTIMUS_REPLICA_PRIMARY_URL`, `BOOTIMUS_REPLICA_TOKEN`, `BOOTIMUS_REPLICA_INTERVAL` and `BOOTIMUS_REPLICA_SYNC_ISOS`. The minimum interval is 30 seconds.

The replica syncs once at start-up and then on every interval. Its admin interface shows a banner naming the primary and the time of the last successful sync, and every create, edit or delete answers `409 Conflict` with a pointer to the primary. Local users and API tokens can still be managed on the replica so you can log in there.

Enable `--proxy-dhcp` on the replica if the branch has no PXE-aware DHCP server, exactly as you would for a standalone install.

## What syncs and what does not

| Synced from the primary | Kept local to each instance |
|-------------------------|-----------------------------|
| Clients (name, group, allowed images, BMC settings, auto-install file, SAN target) | Boot logs, boot counts and last-boot times |
| Client groups | Hardware inventory |
| Images and image groups (metadata, boot method, boot parameters, WinPE selection, boot password policy) | Next-boot actions and Wake-on-LAN |
| SAN targets | Discovered clients that have only ever booted at the branch |
| Boot menu theme and boot password policy | Install callbacks and status |
| Custom distro profiles | Users, API tokens and the audit log |
| Enabled boot tools and their files | Scheduled tasks and webhooks |
| Auto-install files (admin token only) | Kubernetes clusters and node enrolment |
| Custom files | Housekeeping settings |
| The active bootloader set name (the set's files must already exist on the replica) | Custom bootloader set files |

Rows that disappear from the primary are removed from the replica, with one exception: clients that were discovered at the branch and never promoted on the primary are left alone. Next-boot actions are deliberately not mirrored, because a one-shot action consumed at the branch would otherwise be re-applied on the next sync.

With an admin token, protected boot verifiers sync through a separate HTTPS endpoint. The ordinary image and theme APIs never return them. Each verifier must match the policy revision and update time of its corresponding image or theme before the replica saves either field. Password rotation and removal reach a branch on its next successful sync. If the primary is wholly unavailable, the replica keeps its last synced protection. If it observes a newer protected policy but cannot fetch a matching verifier, it blocks that boot until a successful sync. An unavailable, inconsistent or unsupported feed is reported as a sync failure. Older primaries without the verifier endpoint cannot supply protected boots to an updated replica; older replicas retain their existing fail-closed behaviour and must be upgraded to unlock them.

Replica database backups and snapshots contain the same password verifiers as the primary. Restrict access to them accordingly.

## Serving ISOs locally or from the primary

With `sync_isos: true` (the default) the replica downloads every enabled ISO from the primary, extracts kernels and initrds locally, and serves clients from its own disk. The first sync of a large library takes a while; the replica keeps serving the images it already has in the meantime, and later syncs only transfer files whose size changed.

With `sync_isos: false` the replica stores only the metadata and points boot menu entries at the primary's boot port, so clients at the branch stream ISOs and boot files across the WAN. Images using the NBD or NFS boot methods need the files locally and will not boot in this mode. Auto-install scripts are always served by the replica itself.

If a download fails, that image falls back to the primary's URL until the next successful sync.

## Bandwidth

- Metadata syncs are small JSON lists and run every interval.
- ISOs are fetched once and re-fetched only when their size changes on the primary.
- A manifest from the primary lets the replica skip kinds that have not changed since its last run.
- Lower the interval only if you need faster propagation; the replica also has a **Sync now** button.

## When the primary is unreachable

The replica keeps serving its last good state while the primary is unreachable. The sync is marked failed and the banner shows the error. A newly observed protected revision without a valid verifier is blocked. Image sync failure also defers menu updates, so a failed move from global to image protection cannot expose the image. Password removal waits for its matching verifier. Changes made on the primary while the WAN is down take effect at a branch only after a successful sync. Syncing resumes automatically when the primary answers again.

## Monitoring

The **Server Info** tab on a replica has a **Replica** card listing, per kind, whether the last sync succeeded, how many rows the primary has and how many changed or were removed. The same data is available from the API, and `GET /api/me` includes a `replica` object so scripts can tell which instance they are talking to.

## API

```bash
# Replica: status of the last sync
curl -H "Authorization: Bearer $TOKEN" https://branch:8081/api/replica/status

# Replica: sync now
curl -X POST -H "Authorization: Bearer $TOKEN" https://branch:8081/api/replica/sync

# Primary: what a replica reads
curl -H "Authorization: Bearer $TOKEN" https://hq:8081/api/sync/manifest
```

The primary exposes `GET /api/sync/manifest`, `GET /api/sync/tool-files?name=` and `GET /api/sync/asset?kind=iso|tool|file` for replicas; all three accept read-scope tokens. `GET /api/sync/boot-verifiers` requires an admin-scope token and HTTPS. Its response contains password verifiers and must be treated as sensitive; it is marked `Cache-Control: no-store`.
