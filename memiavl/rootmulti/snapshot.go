package rootmulti

import (
	"errors"
	"fmt"
	"math"

	protoio "github.com/cosmos/gogoproto/io"
	"github.com/initia-labs/store/memiavl"

	"cosmossdk.io/store/snapshots/types"
)

// snapshotExporter holds a MultiTree and its associated file lock.
type snapshotExporter struct {
	version int64
	mtree   *memiavl.MultiTree
	fl      memiavl.FileLock
}

// Close closes the MultiTree and releases the file lock.
func (se *snapshotExporter) Close() error {
	var errs []error

	if se.fl != nil {
		if err := se.fl.Destroy(); err != nil {
			errs = append(errs, err)
		}
	}

	if se.mtree != nil {
		if err := se.mtree.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Snapshot Implements interface Snapshotter
func (rs *Store) Snapshot(version uint64, protoWriter protoio.Writer) (returnErr error) {
	if version > math.MaxInt64 {
		return fmt.Errorf("version overflows int64: %d", version)
	}

	// try to lock the snapshot file to prevent deletion while exporting
	fl, err := memiavl.AcquireExporterLock(rs.dir, int64(version))
	if err != nil {
		return err
	}

	success := false
	defer func() {
		if !success {
			fl.Destroy()
		}
	}()

	exporter, err := memiavl.NewMultiTreeExporter(rs.dir, version, rs.supportExportNonSnapshotVersion)
	if err != nil {
		return err
	}

	defer exporter.Close()

	for {
		item, err := exporter.Next()
		if err != nil {
			if errors.Is(err, memiavl.ErrorExportDone) {
				break
			}

			return err
		}

		switch item := item.(type) {
		case *memiavl.ExportNode:
			if err := protoWriter.WriteMsg(&types.SnapshotItem{
				Item: &types.SnapshotItem_IAVL{
					IAVL: &types.SnapshotIAVLItem{
						Key:     item.Key,
						Value:   item.Value,
						Height:  int32(item.Height),
						Version: item.Version,
					},
				},
			}); err != nil {
				return err
			}
		case string:
			if err := protoWriter.WriteMsg(&types.SnapshotItem{
				Item: &types.SnapshotItem_Store{
					Store: &types.SnapshotStoreItem{
						Name: item,
					},
				},
			}); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown item type %T", item)
		}
	}

	// close will be called at `PruneSnapshotHeight`
	cloned := exporter.MultiTree().Copy(0)
	if err := rs.queryCache.SetSnapshotExporter(int64(version), cloned, fl); err != nil {
		_ = cloned.Close()
		return err
	}

	// mark success to avoid cleanup in defer
	success = true

	return nil
}

// PruneSnapshotHeight closes and removes the snapshot MultiTree at the given height.
func (rs *Store) PruneSnapshotHeight(height int64) {
	rs.queryCache.PurgeSnapshotExporter(height)
}

// SetSnapshotInterval Implements interface Snapshotter
// not needed, memiavl manage its own snapshot/pruning strategy
func (rs *Store) SetSnapshotInterval(snapshotInterval uint64) {
}
