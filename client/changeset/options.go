package changeset

import (
	"github.com/linxGnu/grocksdb"

	dbm "github.com/cosmos/cosmos-db"
)

// Options defines the customizable settings of ChangeSetGroupCmd
type Options struct {
	DefaultStores     []string
	OpenReadOnlyDB    func(home string, backend dbm.BackendType) (dbm.DB, error)
	AppRocksDBOptions func(sstFileWriter bool) *grocksdb.Options
}
