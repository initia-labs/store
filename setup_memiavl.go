package store

import (
	"path/filepath"

	"github.com/spf13/cast"

	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/initia-labs/store/config"
	"github.com/initia-labs/store/memiavl"
	"github.com/initia-labs/store/rootmulti"
)

// SetupMemIAVL insert the memiavl setter in front of baseapp options, so that
// the default rootmulti store is replaced by memiavl store,
func SetupMemIAVL(
	logger log.Logger,
	appOpts servertypes.AppOptions,
	supportExportNonSnapshotVersion bool,
	baseAppOptions []func(*baseapp.BaseApp),
) []func(*baseapp.BaseApp) {
	if memIAVLConfig := config.GetMemIAVLConfig(appOpts); memIAVLConfig.Enable {
		opts := memiavl.Options{
			AsyncCommitBuffer:   memIAVLConfig.AsyncCommitBuffer,
			ZeroCopy:            memIAVLConfig.ZeroCopy,
			SnapshotKeepRecent:  memIAVLConfig.SnapshotKeepRecent,
			SnapshotInterval:    memIAVLConfig.SnapshotInterval,
			CacheSize:           memIAVLConfig.CacheSize,
			SnapshotWriterLimit: memIAVLConfig.SnapshotWriterLimit,
		}

		if opts.ZeroCopy {
			// it's unsafe to cache zero-copied byte slices without copying them
			sdk.SetAddrCacheEnabled(false)
		}

		// cms must be overridden before the other options, because they may use the cms,
		// make sure the cms aren't be overridden by the other options later on.
		baseAppOptions = append([]func(*baseapp.BaseApp){func(bapp *baseapp.BaseApp) {
			// trigger state-sync snapshot creation by memiavl
			opts.TriggerStateSyncExport = func(height int64) {
				go bapp.SnapshotManager().SnapshotIfApplicable(height)
			}
			cms := rootmulti.NewStore(filepath.Join(getDBDir(appOpts), "memiavl.db"), logger, supportExportNonSnapshotVersion)
			cms.SetMemIAVLOptions(opts)
			bapp.SetCMS(cms)
		}}, baseAppOptions...)
	}

	return baseAppOptions
}

// getDBDir returns the database configuration for the EVM indexer
func getDBDir(appOpts servertypes.AppOptions) string {
	rootDir := cast.ToString(appOpts.Get(flags.FlagHome))
	dbDir := cast.ToString(appOpts.Get("db_dir"))

	return rootify(dbDir, rootDir)
}

// helper function to make config creation independent of root dir
func rootify(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}
