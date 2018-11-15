package database

import (
	"fmt"
	"math"
	"os"
	"sync"
)

const fileChunk = 1 * (1 << 13) //8Kb

type FileInfo struct {
	FileName string
	FileSize int64
	MetaFile []byte
	MetaHash [32]byte
}

type ChunkInfo struct {
	MetaHash [32]byte
	Index    int
}

type FileDatabase struct {
	MetaDb  map[[32]byte]*FileInfo
	ChunkDb map[[32]byte]*ChunkInfo
	mux     sync.RWMutex
}

func NewFileDatabase() *FileDatabase {
	return &FileDatabase{MetaDb: make(map[[32]byte]*FileInfo), ChunkDb: make(map[[32]byte]*ChunkInfo)}
}

func (f *FileDatabase) InsertMeta(metaHash [32]byte, fileInfo *FileInfo) {
	f.mux.Lock()
	defer f.mux.Unlock()

	f.MetaDb[metaHash] = fileInfo
}

func (f *FileDatabase) InsertChunk(chunkHash [32]byte, chunkInfo *ChunkInfo) {
	f.mux.Lock()
	defer f.mux.Unlock()

	f.ChunkDb[chunkHash] = chunkInfo
}

func (f *FileDatabase) IncreaseFileSize(metaHash [32]byte, size int64) {
	f.mux.Lock()
	defer f.mux.Unlock()

	f.MetaDb[metaHash].FileSize += size
}

func (f *FileDatabase) UpdateFileName(metaHash [32]byte, fileName string) {
	f.mux.Lock()
	defer f.mux.Unlock()

	f.MetaDb[metaHash].FileName = fileName
}

func (f *FileDatabase) GetHashValue(hashValue [32]byte) ([]byte, bool) {
	f.mux.Lock()
	defer f.mux.Unlock()
	//fmt.Printf("want to get: %x\n", hashValue)
	//Fist check if hashValue is a MetaHash
	fileInfo, ok := f.MetaDb[hashValue]
	if ok {
		return fileInfo.MetaFile, ok
	}

	//If not it is a chunk hash
	chunkInfo, ok := f.ChunkDb[hashValue]
	if ok {
		fileInfo, ok := f.MetaDb[chunkInfo.MetaHash]
		if !ok {
			//This should not happen
			return nil, false
		}

		//Open the file and get the right chunk
		file, err := os.Open(fileInfo.FileName)
		if err != nil {
			fmt.Println(err)
			return nil, false
		}
		defer file.Close()

		partSize := int(math.Min(fileChunk, float64(fileInfo.FileSize-int64(chunkInfo.Index*fileChunk))))
		partBuffer := make([]byte, partSize)

		_, err = file.ReadAt(partBuffer, fileChunk*int64(chunkInfo.Index))
		if err != nil {
			fmt.Println(err)
			return nil, false
		}

		return partBuffer, true
	}

	return nil, false
}
