//go:build rocksdb

package store

import (
	"os"
	"path/filepath"

	storetypes "cosmossdk.io/store/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"

	"github.com/initia-labs/store/config"
	"github.com/initia-labs/store/types"
	"github.com/initia-labs/store/versiondb"
	"github.com/initia-labs/store/versiondb/tsrocksdb"
)

// SetupVersionDB sets up the versiondb multi-store if enabled in app options.
func SetupVersionDB(
	app types.App,
	streamingManager *storetypes.StreamingManager,
	appOpts servertypes.AppOptions,
) error {
	if config := config.GetVersionDBConfig(appOpts); config.Enable {
		dataDir := filepath.Join(getDBDir(appOpts), "versiondb")
		if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
			return err
		}

		versionDB, err := tsrocksdb.NewStore(dataDir)
		if err != nil {
			return err
		}

		cms := app.CommitMultiStore()
		cmsVersion := cms.LatestVersion()
		qmsVersion, err := versionDB.GetLatestVersion()
		if err != nil {
			return err
		}
		if cmsVersion < qmsVersion {
			app.Logger().Info(
				"versiondb is ahead of commit multi store, resetting versiondb to match",
				"versiondb_version", qmsVersion,
				"commit_multi_store_version", cmsVersion,
			)

			versionDB.Close()
			versionDB, err = tsrocksdb.NewStoreAtVersion(dataDir, cmsVersion)
			if err != nil {
				return err
			}
		}

		keys := app.GetKVStoreKey()
		tkeys := app.GetTransientStoreKey()

		// always listen for all keys to simplify configuration
		exposedKeys := make([]storetypes.StoreKey, 0, len(keys))
		for _, key := range keys {
			exposedKeys = append(exposedKeys, key)
		}

		cms.AddListeners(exposedKeys)

		// register in app streaming manager
		streamingManager.ABCIListeners = append(streamingManager.ABCIListeners,
			versiondb.NewStreamingService(versionDB),
		)

		delegatedStoreKeys := make(map[storetypes.StoreKey]struct{})
		for _, k := range tkeys {
			delegatedStoreKeys[k] = struct{}{}
		}

		app.SetQueryMultiStore(versiondb.NewMultiStore(cms, versionDB, keys, delegatedStoreKeys))

		return nil
	}

	return nil
}
