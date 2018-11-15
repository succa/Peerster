package peer

import (
	"net"
)

type Peer struct {
	Address *net.UDPAddr
}

func New(address string) (*Peer, error) {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, err
	}
	return &Peer{Address: udpAddr}, nil
}
