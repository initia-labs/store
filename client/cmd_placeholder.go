//go:build !rocksdb

package client

import (
	"github.com/spf13/cobra"
)

func ChangeSetGroupCmd(storeNames []string) *cobra.Command {
	return nil
}
