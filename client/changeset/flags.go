package changeset

const (
	IAVLV0 = iota
	IAVLV1
)

const (
	flagStartVersion     = "start-version"
	flagEndVersion       = "end-version"
	flagConcurrency      = "concurrency"
	flagCheck            = "check"
	flagSave             = "save"
	flagNoParseChangeset = "no-parse-changeset"
	flagChunkSize        = "chunk-size"
	flagZlibLevel        = "zlib-level"
	flagSSTFileSize      = "sst-file-size"
	flagMoveFiles        = "move-files"
	flagStore            = "store"
	flagStores           = "stores"
	flagMaximumVersion   = "maximum-version"
	flagTargetVersion    = "target-version"
	flagSaveSnapshot     = "save-snapshot"
	flagLoadSnapshot     = "load-snapshot"
	flagSorterChunkSize  = "sorter-chunk-size"
	flagSDK64Compact     = "sdk64-compact"
	flagIAVLVersion      = "iavl-version"
)
