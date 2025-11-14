package config

import (
	"github.com/spf13/cast"
	"github.com/spf13/cobra"

	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

const DefaultCacheSize = 1000

const (
	flagVersionDBEnable = "versiondb.enable"
)

// VersionDBConfig defines the configuration for the versiondb store.
type VersionDBConfig struct {
	// Enable toggles versiondb; enabling it disables pruning because
	// versiondb keeps the full history of change sets for archival nodes.
	Enable bool `mapstructure:"enable"`
}

// GetVersionDBConfig loads config values from the app options
func DefaultVersionDBConfig() VersionDBConfig {
	return VersionDBConfig{
		Enable: false,
	}
}

// GetVersionDBConfig loads config values from the app options
func GetVersionDBConfig(appOpts servertypes.AppOptions) VersionDBConfig {
	return VersionDBConfig{
		Enable: cast.ToBool(appOpts.Get(flagVersionDBEnable)),
	}
}

// AddVersionDBConfigFlags adds the versiondb configuration flags to the start command
func AddVersionDBConfigFlags(startCmd *cobra.Command) {
	startCmd.Flags().Bool(flagVersionDBEnable, false, "Enable versiondb as the commit multi-store")
}

// DefaultVersionDBConfigTemplate defines the configuration template for the versiondb configuration
const DefaultVersionDBConfigTemplate = `
###############################################################################
###                             VersionDB Configuration                      ###
###############################################################################

[versiondb]
	
# Enable toggles versiondb. When true, pruning is not supported because
# versiondb keeps the full history of change sets; only enable it for
# archival nodes.
enable = {{ .VersionDB.Enable }}
`
