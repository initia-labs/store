package types

import (
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

type App interface {
	Logger() log.Logger
	SetQueryMultiStore(store storetypes.MultiStore)
	CommitMultiStore() storetypes.CommitMultiStore
	GetQueryMultiStore() storetypes.MultiStore
	GetKVStoreKey() map[string]*storetypes.KVStoreKey
	GetTransientStoreKey() map[string]*storetypes.TransientStoreKey
}

type AppCreator interface {
	App() servertypes.Application
	AppOpts() servertypes.AppOptions
}
