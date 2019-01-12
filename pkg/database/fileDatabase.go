package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/succa/Peerster/pkg/utils"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"
)

type FileInfo struct {
	FileName       string
	FileSize       int64

	RootNode *MerkleNode
	ChunkDb  map[[32]byte]*MerkleNode
}

// in chunk num & chunk map only leaves count
type RequestInfo struct {
	FileName string
	//TempFileName string
	MetaHash         [32]byte
	ChunkNum         int
	ChunkMap         map[int]string //string is the name of the peerster to download from
	Checked          bool
	ReturnedToClient bool
}

type MerkleNode struct {
	Height    int
	HashValue [32]byte

	Children []*MerkleNode // |children| <= chunk_size / 32
}

type FileDatabase struct {
	RootHashDB map[[32]byte]*FileInfo
	mux sync.RWMutex
}

func NewFileDatabase() *FileDatabase {
	return &FileDatabase{
		RootHashDB: make(map[[32]byte]*FileInfo),
	}
}

func GetMerkleChunkFileName(hashValue [32]byte) string {
	return hex.EncodeToString(hashValue[:]) + ".merkle"
}

func ConstructInnerMerkleTree(children []*MerkleNode, chunksPath string) *MerkleNode {
	if children == nil || len(children) == 0 {
		fmt.Println("empty children passed")
		return nil
	}

	height := children[0].Height
	data := make([]byte, 0, utils.FileChunkSize)
	for _, c := range children {
		data = append(data, c.HashValue[:]...)
		if c.Height != height {
			fmt.Println("children vary in height!")
			return nil
		}
	}

	if len(data) > utils.FileChunkSize {
		fmt.Println("got very strange file node!")
		return nil
	}

	hashValue := sha256.Sum256(data)
	ioutil.WriteFile(filepath.Join(chunksPath, GetMerkleChunkFileName(hashValue)), data, utils.FileCommonMode)

	return &MerkleNode{Height:height + 1, HashValue:hashValue, Children:children}
}

func (f *FileDatabase) InsertFileinfo(fileInfo *FileInfo) {
	f.mux.Lock()
	defer f.mux.Unlock()

	fmt.Println("Adding new fileinfo to filedb: ", fileInfo.FileName)
	f.RootHashDB[fileInfo.RootNode.HashValue] = fileInfo
}

func (f *FileDatabase) GetChunkByHash(hashValue [32]byte) ([]byte, int, bool) {
	f.mux.Lock()
	defer f.mux.Unlock()
	//fmt.Printf("want to get: %x\n", hashValue)

	for _, fileinfo := range f.RootHashDB {
		node, ok := fileinfo.ChunkDb[hashValue]
		if !ok {
			continue
		}

		sharedChunksPath := path.Join(utils.SharedChunksPath, fileinfo.FileName)
		sharedChunkPath := path.Join(sharedChunksPath, GetMerkleChunkFileName(hashValue))
		file, err := os.Open(sharedChunkPath)
		if err != nil {
			downloadsChunksPath := path.Join(utils.DownloadsChunksPath, fileinfo.FileName)
			downloadChunkPath := path.Join(downloadsChunksPath, GetMerkleChunkFileName(hashValue))
			file, err = os.Open(downloadChunkPath)
			if err != nil {
				fmt.Println("cannot find chunk, which must be present either in downloads or shared!")
				return nil, 0, false
			}
		}
		defer file.Close()

		buffer := make([]byte, utils.FileChunkSize)
		n, err := file.Read(buffer)
		if err != nil {
			fmt.Println(err)
			return nil, 0, false
		}

		return buffer[0:n], node.Height, true
	}

	return nil, 0, false
}
