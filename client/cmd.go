//go:build rocksdb

package client

import (
	"sort"

	"github.com/linxGnu/grocksdb"
	"github.com/spf13/cobra"

	"github.com/initia-labs/store/client/changeset"
	"github.com/initia-labs/store/opendb"
)

func ChangeSetGroupCmd(storeNames []string) *cobra.Command {
	sort.Strings(storeNames)
	opts := &changeset.Options{
		OpenReadOnlyDB: opendb.OpenReadOnlyDB,
		DefaultStores:  storeNames,
		AppRocksDBOptions: func(sstFileWriter bool) *grocksdb.Options {
			return opendb.NewRocksdbOptions(nil, sstFileWriter)
		},
	}
	cmd := &cobra.Command{
		Use:     "changeset",
		Aliases: []string{"cs"},
		Short:   "dump and manage change sets files and ingest into versiondb",
	}
	cmd.AddCommand(
		changeset.ListDefaultStoresCmd(opts),
		changeset.DumpChangeSetCmd(opts),
		changeset.PrintChangeSetCmd(),
		changeset.VerifyChangeSetCmd(opts),
		changeset.BuildVersionDBSSTCmd(opts),
		changeset.IngestVersionDBSSTCmd(),
		changeset.ChangeSetToVersionDBCmd(),
		changeset.RestoreAppDBCmd(opts),
		changeset.RestoreVersionDBCmd(),
		changeset.FixDataCmd(opts.DefaultStores),
	)
	return cmd
}
