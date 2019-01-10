package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/succa/Peerster/pkg/message"
	"github.com/succa/Peerster/pkg/utils"
	"io/ioutil"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
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
	RequestDb map[[32]byte]*RequestInfo
	mux sync.RWMutex
}

func NewFileDatabase() *FileDatabase {
	return &FileDatabase{
		RootHashDB: make(map[[32]byte]*FileInfo),
		//RequestDb: make(map[[32]byte]*RequestInfo),
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

//func (f *FileDatabase) GetMeta(metaHash [32]byte) (*FileInfo, bool) {
//	f.mux.RLock()
//	defer f.mux.RUnlock()
//	fileInfo, ok := f.MetaDb[metaHash]
//	return fileInfo, ok
//}

func (f *FileDatabase) GetMetaFromRequestName(fileName string) ([32]byte, bool) {
	f.mux.RLock()
	defer f.mux.RUnlock()
	for _, val := range f.RequestDb {
		if val.FileName == fileName {
			return val.MetaHash, true
		}
    }
	return [32]byte{}, false
}

//func (f *FileDatabase) InsertChunk(chunkHash [32]byte, chunkInfo *ChunkInfo) {
//	f.mux.Lock()
//	defer f.mux.Unlock()
//
//	f.ChunkDb[chunkHash] = chunkInfo
//}

//func (f *FileDatabase) InsertSearchResult(searchResult *message.SearchResult, destination string) {
//	f.mux.Lock()
//	defer f.mux.Unlock()
//
//	//f.RequestDb[metaHash] = requestInfo
//	var metaHash32 [32]byte
//	copy(metaHash32[:], searchResult.MetafileHash)
//	requestInfo, ok := f.RequestDb[metaHash32]
//	if !ok {
//		// create a new RequestInfo
//		chunkMap := make(map[int]string)
//		for _, index := range searchResult.ChunkMap {
//			chunkMap[int(index)] = destination
//		}
//
//		requestInfo := &RequestInfo{
//			FileName:         searchResult.FileName,
//			MetaHash:         metaHash32,
//			ChunkMap:         chunkMap,
//			ChunkNum:         int(searchResult.ChunkCount),
//			Checked:          false,
//			ReturnedToClient: false,
//		}
//
//		f.RequestDb[metaHash32] = requestInfo
//	} else {
//		// Insert chunks if needed
//		for _, index := range searchResult.ChunkMap {
//			if _, ok := requestInfo.ChunkMap[int(index)]; !ok {
//				requestInfo.ChunkMap[int(index)] = destination
//			}
//		}
//	}
//}

//func (f *FileDatabase) UpdateChunkNumber(metaHash [32]byte, chunkNum int) {
//	f.mux.Lock()
//	defer f.mux.Unlock()
//	requestInfo, ok := f.RequestDb[metaHash]
//	if ok {
//		if requestInfo.ChunkNum == 0 {
//			requestInfo.ChunkNum = chunkNum
//		}
//	}
//	return
//}

/*
func (f *FileDatabase) UpdateTempFileName(metaHash [32]byte, tempFileName string) {
	f.mux.Lock()
	defer f.mux.Unlock()
	requestInfo, ok := f.RequestDb[metaHash]
	if ok {
		if requestInfo.TempFileName == "" {
			requestInfo.TempFileName = tempFileName
		}
	}
	return
}
*/

//func (f *FileDatabase) CheckSearchComplete(metaHash [32]byte) bool {
//	f.mux.RLock()
//	defer f.mux.RUnlock()
//	requestInfo, ok := f.RequestDb[metaHash]
//	if ok && !requestInfo.Checked {
//		if requestInfo.ChunkNum == len(requestInfo.ChunkMap) {
//			requestInfo.Checked = true
//			return true
//		}
//	}
//	return false
//}

func (f *FileDatabase) ResetRequestDb() {
	f.mux.Lock()
	defer f.mux.Unlock()
	f.RequestDb = make(map[[32]byte]*RequestInfo)
}

func (f *FileDatabase) ResetRequestDbKeywords(keywords []string) {
	f.mux.Lock()
	defer f.mux.Unlock()

	for _, keyword := range keywords {
		for mapKey, mapValue := range f.RequestDb {
			if match, _ := regexp.MatchString(".*"+keyword+".*", mapValue.FileName); match {
				fmt.Println("deleting " + mapValue.FileName)
				delete(f.RequestDb, mapKey)
			}
		}
	}
}

func (f *FileDatabase) CheckCompletedSearch(keywords []string, limit int) bool {
	f.mux.Lock()
	defer f.mux.Unlock()

	i := 0

	for _, keyword := range keywords {
		for _, mapValue := range f.RequestDb {
			if match, _ := regexp.MatchString(".*"+keyword+".*", mapValue.FileName); match {
				if mapValue.ChunkNum == len(mapValue.ChunkMap) {
					i = i + 1
					if i == 2 {
						return true
					}
				}
			}
		}
	}

	return false

}

func (f *FileDatabase) HandleSearchReply(searchReply *message.SearchReply) {
	f.mux.Lock()
	defer f.mux.Unlock()

	for _, result := range searchReply.Results {
		utils.PrintFoundMatch(result.FileName, searchReply.Origin, result.MetafileHash, result.ChunkMap)

		var metaHash [32]byte
		copy(metaHash[:], result.MetafileHash)
		value, ok := f.RequestDb[metaHash]
		if !ok {
			// New result! insert it
			chunkMap := make(map[int]string)
			for _, index := range result.ChunkMap {
				chunkMap[int(index)] = searchReply.Origin
			}
			f.RequestDb[metaHash] = &RequestInfo{
				FileName: result.FileName,
				MetaHash: metaHash,
				ChunkNum: int(result.ChunkCount),
				ChunkMap: chunkMap,
			}
		} else {
			// there is already a result, update it if necessary
			for i := range result.ChunkMap {
				if _, ok := value.ChunkMap[i]; !ok {
					value.ChunkMap[i] = searchReply.Origin
				}

			}
		}
	}
}

//func (f *FileDatabase) GetChunkMap(metaHash [32]byte) map[int]string {
//	f.mux.RLock()
//	defer f.mux.RUnlock()
//	requestInfo, ok := f.RequestDb[metaHash]
//	if ok {
//		return requestInfo.ChunkMap
//	}
//	return nil
//}

//func (f *FileDatabase) IncreaseFileSize(metaHash [32]byte, size int64) {
//	f.mux.Lock()
//	defer f.mux.Unlock()
//
//	f.MetaDb[metaHash].FileSize += size
//}

//func (f *FileDatabase) UpdateFileName(metaHash [32]byte, fileName string) {
//	f.mux.Lock()
//	defer f.mux.Unlock()
//
//	f.MetaDb[metaHash].FileName = fileName
//}

func (f *FileDatabase) GetByHashValue(hashValue [32]byte) ([]byte, int, bool) {
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

func (f *FileDatabase) GetFileRegex(file string) []*message.SearchResult {
	f.mux.RLock()
	defer f.mux.RUnlock()
	var ret []*message.SearchResult
	for _, value := range f.RootHashDB {
		match, _ := regexp.MatchString(".*"+file+".*", value.FileName)
		if match {
			fmt.Println("MATCH with " + filepath.Base(value.FileName))
			var chunkMap []uint64
			leavesChunkSize := int(math.Ceil(float64(value.FileSize) / utils.FileChunkSize))
			for i := 0; i < leavesChunkSize; i++ {
				chunkMap = append(chunkMap, uint64(i + 1)) // not good, but works with invariant of only full files
			}
			sort.Slice(chunkMap, func(i, j int) bool { return chunkMap[i] < chunkMap[j] })

			// create a search result
			searchResult := &message.SearchResult{
				FileName:     value.FileName,
				MetafileHash: value.RootNode.HashValue[:],
				ChunkMap:     chunkMap,
				ChunkCount:   uint64(len(chunkMap)),
			}
			ret = append(ret, searchResult)
		}
	}
	return ret
}

//func countChunks(data []byte) int {
//	// metafile has to be composed by multiple of 32
//	if len(data)%32 != 0 {
//		return 0
//	}
//	num := 0
//	for i := 0; i < len(data)/32; i++ {
//		num = num + 1
//	}
//	return num
//}

func (f *FileDatabase) GetCompletedSearches(keywords []string) []*RequestInfo {
	f.mux.Lock()
	defer f.mux.Unlock()
	var ret []*RequestInfo
	for _, keyword := range keywords {
		for _, v := range f.RequestDb {
			if match, _ := regexp.MatchString(".*"+keyword+".*", v.FileName); match {
				if v.ChunkNum == len(v.ChunkMap) && !v.Checked {
					v.Checked = true
					ret = append(ret, v)
				}
			}
		}
	}
	return ret
}

func (f *FileDatabase) GetCompletedSearchesClient(keywords []string) []*RequestInfo {
	f.mux.Lock()
	defer f.mux.Unlock()
	var ret []*RequestInfo
	for _, keyword := range keywords {
		for _, v := range f.RequestDb {
			if match, _ := regexp.MatchString(".*"+keyword+".*", v.FileName); match {
				if v.ChunkNum == len(v.ChunkMap) && !v.ReturnedToClient {
					v.ReturnedToClient = true
					ret = append(ret, v)
				}
			}
		}
	}
	return ret
}

func (f *FileDatabase) GetRequestInfo(metaHash [32]byte) (*RequestInfo, bool) {
	f.mux.RLock()
	defer f.mux.RUnlock()

	req, err := f.RequestDb[metaHash]
	return req, err
}
