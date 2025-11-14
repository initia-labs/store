//go:build !rocksdb

package store

import (
	"fmt"

	storetypes "cosmossdk.io/store/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"

	"github.com/initia-labs/store/config"
	"github.com/initia-labs/store/types"
)

// SetupVersionDB returns an error when versiondb is enabled without the rocksdb build tag.
func SetupVersionDB(
	app types.App,
	streamingManager *storetypes.StreamingManager,
	appOpts servertypes.AppOptions,
) error {
	if config := config.GetVersionDBConfig(appOpts); config.Enable {
		return fmt.Errorf("versiondb requires store to be built with the 'rocksdb' build tag")
	}

	return nil
}
