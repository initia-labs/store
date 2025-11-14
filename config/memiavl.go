package config

import (
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"

	"github.com/initia-labs/store/memiavl"
)

const (
	DefaultMemIAVLCacheSize          = 1000
	DefaultMemIAVLAsyncCommitBuffer  = 10
	DefaultMemIAVLSnapshotKeepRecent = 1
)

const (
	flagMemIAVLEnable              = "memiavl.enable"
	flagMemIAVLAsyncCommitBuffer   = "memiavl.async-commit-buffer"
	flagMemIAVLZeroCopy            = "memiavl.zero-copy"
	flagMemIAVLSnapshotKeepRecent  = "memiavl.snapshot-keep-recent"
	flagMemIAVLSnapshotInterval    = "memiavl.snapshot-interval"
	flagMemIAVLCacheSize           = "memiavl.cache-size"
	flagMemIAVLSnapshotWriterLimit = "memiavl.snapshot-writer-limit"
)

// MemIAVLConfig defines the configuration for the memiavl store.
type MemIAVLConfig struct {
	// Enable defines if the memiavl should be enabled.
	Enable bool `mapstructure:"enable"`
	// ZeroCopy defines if the memiavl should return slices pointing to mmap-ed buffers directly (zero-copy),
	// the zero-copied slices must not be retained beyond current block's execution.
	// the sdk address cache will be disabled if zero-copy is enabled.
	ZeroCopy bool `mapstructure:"zero-copy"`
	// AsyncCommitBuffer defines the size of asynchronous commit queue, this greatly improve block catching-up
	// performance, -1 means synchronous commit.
	AsyncCommitBuffer int `mapstructure:"async-commit-buffer"`
	// SnapshotKeepRecent defines what many old snapshots (excluding the latest one) to keep after new snapshots are
	// taken, defaults to 1 to make sure ibc relayers work.
	SnapshotKeepRecent uint32 `mapstructure:"snapshot-keep-recent"`
	// SnapshotInterval defines the block interval the memiavl snapshot is taken, default to 1000.
	SnapshotInterval uint32 `mapstructure:"snapshot-interval"`
	// SnapshotWriterLimit defines the maximum number of concurrent snapshot writers.
	SnapshotWriterLimit int `mapstructure:"snapshot-writer-limit"`
	// CacheSize defines the size of the cache for each memiavl store.
	CacheSize int `mapstructure:"cache-size"`
}

// DefaultMemIAVLConfig returns the default memiavl configuration.
func DefaultMemIAVLConfig() MemIAVLConfig {
	return MemIAVLConfig{
		Enable:              false,
		ZeroCopy:            false,
		AsyncCommitBuffer:   DefaultMemIAVLAsyncCommitBuffer,
		SnapshotKeepRecent:  DefaultMemIAVLSnapshotKeepRecent,
		SnapshotInterval:    memiavl.DefaultSnapshotInterval,
		SnapshotWriterLimit: memiavl.DefaultSnapshotWriterLimit,
		CacheSize:           DefaultMemIAVLCacheSize,
	}
}

// GetConfig load config values from the app options
func GetMemIAVLConfig(appOpts servertypes.AppOptions) MemIAVLConfig {
	return MemIAVLConfig{
		Enable:              cast.ToBool(appOpts.Get(flagMemIAVLEnable)),
		ZeroCopy:            cast.ToBool(appOpts.Get(flagMemIAVLZeroCopy)),
		AsyncCommitBuffer:   cast.ToInt(appOpts.Get(flagMemIAVLAsyncCommitBuffer)),
		SnapshotKeepRecent:  cast.ToUint32(appOpts.Get(flagMemIAVLSnapshotKeepRecent)),
		SnapshotInterval:    cast.ToUint32(appOpts.Get(flagMemIAVLSnapshotInterval)),
		SnapshotWriterLimit: cast.ToInt(appOpts.Get(flagMemIAVLSnapshotWriterLimit)),
		CacheSize:           cast.ToInt(appOpts.Get(flagMemIAVLCacheSize)),
	}
}

// AddConfigFlags implements servertypes.EVMConfigFlags interface.
func AddMemIAVLConfigFlags(startCmd *cobra.Command) {
	startCmd.Flags().Bool(flagMemIAVLEnable, false, "Enable memiavl store as the commit multi-store")
	startCmd.Flags().Int(flagMemIAVLAsyncCommitBuffer, DefaultMemIAVLAsyncCommitBuffer, "Maximum simulation gas amount for evm contract execution")
	startCmd.Flags().Bool(flagMemIAVLZeroCopy, false, "Enable zero-copy mode for memiavl store")
	startCmd.Flags().Uint32(flagMemIAVLSnapshotKeepRecent, DefaultMemIAVLSnapshotKeepRecent, "Number of recent memiavl snapshots to keep")
	startCmd.Flags().Uint32(flagMemIAVLSnapshotInterval, memiavl.DefaultSnapshotInterval, "Block interval the memiavl snapshot is taken")
	startCmd.Flags().Int(flagMemIAVLSnapshotWriterLimit, memiavl.DefaultSnapshotWriterLimit, "Maximum number of concurrent memiavl snapshot writers")
	startCmd.Flags().Int(flagMemIAVLCacheSize, DefaultMemIAVLCacheSize, "Size of the cache for each memiavl store")
}

// DefaultMemIAVLConfigTemplate defines the configuration template for the memiavl configuration
const DefaultMemIAVLConfigTemplate = `
###############################################################################
###                             MemIAVL Configuration                       ###
###############################################################################

[memiavl]

# Enable defines if the memiavl should be enabled.
enable = {{ .MemIAVL.Enable }}

# ZeroCopy defines if the memiavl should return slices pointing to mmap-ed buffers directly (zero-copy),
# the zero-copied slices must not be retained beyond current block's execution.
# the sdk address cache will be disabled if zero-copy is enabled.
zero-copy = {{ .MemIAVL.ZeroCopy }}

# AsyncCommitBuffer defines the size of asynchronous commit queue, this greatly improve block catching-up
# performance, -1 means synchronous commit.
async-commit-buffer = {{ .MemIAVL.AsyncCommitBuffer }}

# SnapshotKeepRecent defines what many old snapshots (excluding the latest one) to keep after new snapshots are
# taken, defaults to 1 to make sure ibc relayers work.
snapshot-keep-recent = {{ .MemIAVL.SnapshotKeepRecent }}

# SnapshotInterval defines the block interval the memiavl snapshot is taken, default to 1000.
# The [state-sync] snapshot-interval should be divisible by this value.
snapshot-interval = {{ .MemIAVL.SnapshotInterval }}

# SnapshotWriterLimit defines the maximum number of concurrent snapshot writers.
snapshot-writer-limit = {{ .MemIAVL.SnapshotWriterLimit }}

# CacheSize defines the size of the cache for each memiavl store, default to 1000.
cache-size = {{ .MemIAVL.CacheSize }}
`
