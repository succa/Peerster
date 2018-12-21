package utils

import "github.com/succa/Peerster/pkg/message"

type SearchHelper struct {
	PendingSearch int32
	Ch            chan message.GossipPacket
}

func NewSearchHelper() *SearchHelper {
	return &SearchHelper{PendingSearch: 0}
}
