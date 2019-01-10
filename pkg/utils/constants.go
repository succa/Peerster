package utils

import "path"

const (
	FileChunkSize  = 8 * 1024 // in bytes
	FileCommonMode = 0755     // owner=rwx, all others=rx
)

var (
	SharedFilesPath  = "_SharedFiles"
	SharedChunksPath = path.Join(SharedFilesPath, "chunks")

	DownloadsFilesPath  = "_Downloads"
	DownloadsChunksPath = path.Join(DownloadsFilesPath, "chunks")
)
