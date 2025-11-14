# Store

This repository started as a fork of the [`crypto-org-chain/cronos`](https://github.com/crypto-org-chain/cronos) store utilities.

This repository hosts Initia's data-store components:

- **MemIAVL** – an alternative in-memory snapshot-backed IAVL implementation that the consensus state machine uses in place of the legacy `application.db`.
- **VersionDB** – a RocksDB-based append-only store that powers historical queries (gRPC, explorers, etc.) without merklized proofs. It is intended for archival or gRPC-only nodes; validators should keep using pruned IAVL because VersionDB keeps the full change history.

Each component has its own detailed guide ([`memiavl/README.md`](memiavl/README.md), [`versiondb/README.md`](versiondb/README.md)). This root README ties the workflows together so you can jump into the subdirectories with proper context.

## Table of Contents

1. [VersionDB Migration](#versiondb-migration)
2. [MemIAVL Bootstrap (State Sync Snapshot)](#memiavl-bootstrap-state-sync-snapshot)
3. [Catch Up With IAVL Tree](#catch-up-with-iavl-tree)

VersionDB ingestion relies on archived change sets, while MemIAVL now bootstraps directly from state-sync snapshots. The sections below walk through both workflows.

## VersionDB Migration

At a high level you must (commands below assume `$APPD` points to your blockchain binary, e.g. `initiad`, and `$NODE_HOME` is the node home directory such as `~/.initia`):

1. Dump historical change sets from the legacy IAVL (`application.db`).
2. Verify those files to make sure the app hash / commit info matches the chain.
3. Feed the verified artifacts into VersionDB.
4. (Optional) Rebuild a compact `application.db` if you want the IAVL tree pruned.

### Extract Change Sets

```bash
$APPD changeset dump data.dump --home $NODE_HOME
```

`dump` extracts change sets from the IAVL tree and stores each store in separate directories. It uses the store list registered in the current version of `App` by default, and you can customize it with `--stores`. Files are segmented by block ranges, compressed with zlib level 6, and laid out like:

```bash
data/acc/block-0.zz
data/acc/block-1000000.zz
data/acc/block-2000000.zz
...
data/authz/block-0.zz
data/authz/block-1000000.zz
data/authz/block-2000000.zz
...
```

Extraction is the slowest step (≈11h on an 8-core SSD archive node), but once produced these archives can be shared via CDN and verified by other operators. RocksDB backends can be opened in read-only mode so you may run the dump on a live node; goleveldb lacks this ability.

### Verify Change Sets

```bash
$APPD changeset verify data.dump
```

`verify` replays the change sets, rebuilds the target IAVL tree, and outputs the app hash plus commit info of the chosen version (default: latest). When `--save-snapshot` is set it also writes MemIAVL snapshot files under the provided directory, one subfolder per store plus a `metadata` file that records the MultiTree commit info (handy when rebuilding `application.db`, though MemIAVL can also import state-sync snapshots directly; see below).

`verify` consumes several gigabytes of RAM and takes minutes. If memory is tight you can run it incrementally by exporting an intermediate snapshot and resuming from it:

```bash
$APPD changeset verify data.dump --save-snapshot snapshot --target-version 3000000
$APPD changeset verify data.dump --load-snapshot snapshot
```

### Build VersionDB

To maximize ingestion speed into RocksDB, write SST files first, then ingest them into the final VersionDB instance. The SST writers for each store can run in parallel, and data is externally sorted so SST files do not overlap.

```bash
$APPD changeset build-versiondb-sst ./data.dump ./sst
$APPD changeset ingest-versiondb-sst ~/.initia/data/versiondb sst/*.sst --move-files --maximum-version $VERSION
```

Control peak RAM usage with `--concurrency` and `--sorter-chunk-size`. With defaults it finishes in roughly 12 minutes on an 8-core machine (≈2 GB peak RSS).

### Restore application.db (Optional)

When migrating an archive node it is often useful to rebuild `application.db` from scratch to reclaim disk space faster:

```bash
$APPD changeset verify data.dump --save-snapshot snapshot
$APPD changeset restore-app-db snapshot application.db
```

Replace the existing `application.db` with the generated one afterward. The command currently produces a RocksDB backend, so set `app-db-backend="rocksdb"` in `app.toml`.

## MemIAVL Bootstrap (State Sync Snapshot)

MemIAVL nodes are bootstrapped via the state-sync snapshot service. The typical workflow is:

1. **Export from the legacy store (MemIAVL disabled):**

   ```bash
   $APPD snapshots export --home $NODE_HOME
   ```

   This writes the latest snapshot into `$NODE_HOME/data/snapshots/$VERSION` (the node keeps one directory per exported height). The on-disk format version used by the state-sync service is currently fixed at `3`.

2. **Enable MemIAVL in `app.toml`** (`[memiavl] enable = true`, plus any tuning knobs) and restart so the new store is active.

3. **Restore the exported snapshot after MemIAVL is enabled:**

   ```bash
   $APPD snapshots restore $VERSION 3 --home $NODE_HOME
   ```

   The restore command consumes the exported directory under `data/snapshots/$VERSION`, rewrites `memiavl.db`, and updates the `current` symlink. On the next start the node loads directly from that snapshot and continues committing new versions into MemIAVL.

Once MemIAVL owns consensus state you can prune or discard the legacy `application.db` unless you keep it for fallback.

### Catch Up With IAVL Tree

If a non-empty VersionDB or MemIAVL snapshot lags behind the current `application.db`, the node refuses to start. You can either:

- Sync VersionDB/MemIAVL forward by dumping or exporting a newer range and replaying/restoring it, or
- Restore `application.db` to a version that matches the archival stores.

For VersionDB-specific behavior and configuration, see `versiondb/README.md`. For MemIAVL internals, see `memiavl/README.md`.
