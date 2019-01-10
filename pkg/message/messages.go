package message

type GossipPacket struct {
	Simple            *SimpleMessage
	Rumor             *RumorMessage
	Status            *StatusPacket
	Private           *PrivateMessage
	DataRequest       *DataRequest
	MDataReply         *MerkleDataReply
	SearchRequest     *SearchRequest
	SearchReply       *SearchReply
	TxPublish         *TxPublish
	BlockPublish      *BlockPublish
	TxOnionPeer       *TxOnionPeer
	BlockOnionPublish *BlockOnionPublish
	BlockRequest      *BlockRequest
	BlockReply        *BlockReply
	OnionMessage      *OnionMessage
}

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}

type PeerStatus struct {
	Identifier string
	NexId      uint32
}

type StatusPacket struct {
	Want []PeerStatus
}

type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
}

type DataRequest struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
}

type MerkleDataReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
	Data        []byte
	Height      int // 0 height stays for real file contents
}

type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}

type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}

type SearchResult struct {
	FileName     string
	MetafileHash []byte
	ChunkMap     []uint64
	ChunkCount   uint64
}

type TxPublish struct {
	File     File
	HopLimit uint32
}

type BlockPublish struct {
	Block    Block
	HopLimit uint32
}

type OnionMessage struct {
	Cipher          []byte
	AESKeyEncrypted []byte
	Destination     string
	LastNode        bool
	HopLimit        uint32
}

type TxOnionPeer struct {
	NodeName  string
	PublicKey string
	HopLimit  uint32
}

type BlockRequest struct {
	Origin    string
	HopLimit  uint32
	BlockHash [32]byte
}

type BlockReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	BlockHash   [32]byte
	Block       BlockOnion
}

type BlockOnionPublish struct {
	Block    BlockOnion
	HopLimit uint32
}
