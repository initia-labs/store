# VersionDB

VersionDB is a solution for the size issue of IAVL database, aka. `application.db`, at current stage, it's only recommended for archive and non-validator nodes to try (validator nodes are recommended to do pruning anyway).

Pruning is not supported once versiondb is enabled because it keeps the full history of change sets for historical queries. Treat it as an archival-only feature and enable it only if you need an archive node.

VersionDB stores multiple versions of on-chain state key-value pairs directly, without using a merklized tree structure like IAVL tree, both db size and query performance are much better than IAVL tree. The major lacking feature compared to IAVL tree is root hash and merkle proof generation, so we still need IAVL tree for those tasks.

Currently grpc query service don't need to support proof generation, so versiondb alone is enough to support grpc query service, there's already a `--grpc-only` flag for one to start a standalone grpc query service.

There could be different implementations for the idea of versiondb, the current implementation we delivered is based on rocksdb v7's experimental user-defined timestamp[^1], it stores the data in a standalone rocksdb instance, it don't support other db backend yet, but the other databases in the node still support multiple backends as before.

After versiondb is enabled, there's no point to keep the full the archived IAVL tree anymore, it's recommended to prune the IAVL tree to keep only recent versions, for example versions within the unbonding period or even less.

## Configuration

To enable versiondb, set the `versiondb.enable` to `true` in `app.toml`:

```toml
[versiondb]
enable = true
```

On startup, the node will create a `StreamingService` to subscribe to latest state changes in realtime and save them to versiondb, the db instance is placed at `$NODE_HOME/data/versiondb` directory, there's no way to customize the db path currently. It'll also switch grpc query service's backing store to versiondb from IAVL tree, you should migrate the legacy states in advance to make the transition smooth, otherwise, the grpc queries can't see the legacy versions.

If the versiondb is not empty and it's latest version doesn't match the IAVL db's last committed version, the startup will fail with error message `"versiondb latest version %d doesn't match iavl latest version %d"`, that's to avoid creating gaps in versiondb accidentally. When this error happens, you just need to update versiondb to the latest version in iavl tree manually, or restore IAVL db to the same version as versiondb (see [Catch Up With IAVL Tree](../README.md#catch-up-with-iavl-tree)).

## Migration

The migration guide now lives in the repository root so it can be shared with other components. See [Build VersionDB](../README.md#build-versiondb).

[^1]: <https://github.com/facebook/rocksdb/wiki/User-defined-Timestamp-%28Experimental%29>
