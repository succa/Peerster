package onion

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"

	"github.com/dedis/protobuf"
	"github.com/succa/Peerster/pkg/message"
)

type OnionAgent struct {
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
}

type OnionRouter struct {
	Name      string
	PublicKey *rsa.PublicKey
}

func NewOnionAgent(publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) *OnionAgent {
	return &OnionAgent{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}
}

func (o *OnionAgent) OnionEncrypt(msg *message.GossipPacket, destination, node3, node2, node1 *OnionRouter) (*message.GossipPacket, error) {
	//Encrypt message with dest PK
	gossipPacketByte, err := protobuf.Encode(msg)
	if err != nil {
		return nil, err
	}
	encryptedMsg, errPKCS1v15 := rsa.EncryptPKCS1v15(rand.Reader, destination.PublicKey, gossipPacketByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	// First layer
	firstLayer := &message.OnionMessage{
		Cipher:      encryptedMsg,
		Destination: destination.Name,
		LastNode:    true,
		HopLimit:    10,
	}
	firstLayerGossip := &message.GossipPacket{OnionMessage: firstLayer}
	fistLayerByte, err := protobuf.Encode(firstLayerGossip)
	if err != nil {
		return nil, err
	}
	firstLayerEncrypted, errPKCS1v15 := rsa.EncryptPKCS1v15(rand.Reader, node1.PublicKey, fistLayerByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	//Second layer
	secondLayer := &message.OnionMessage{
		Cipher:      firstLayerEncrypted,
		Destination: node1.Name,
		LastNode:    false,
		HopLimit:    10,
	}
	secondLayerGossip := &message.GossipPacket{OnionMessage: secondLayer}
	secondLayerByte, err := protobuf.Encode(secondLayerGossip)
	if err != nil {
		return nil, err
	}
	secondLayerEncrypted, errPKCS1v15 := rsa.EncryptPKCS1v15(rand.Reader, node2.PublicKey, secondLayerByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	//Third layer
	thirdLayer := &message.OnionMessage{
		Cipher:      secondLayerEncrypted,
		Destination: node2.Name,
		LastNode:    false,
		HopLimit:    10,
	}
	thirdLayerGossip := &message.GossipPacket{OnionMessage: thirdLayer}
	thirdLayerByte, err := protobuf.Encode(thirdLayerGossip)
	if err != nil {
		return nil, err
	}
	thirdLayerEncrypted, errPKCS1v15 := rsa.EncryptPKCS1v15(rand.Reader, node3.PublicKey, thirdLayerByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	//Final Packet
	finalPacket := &message.OnionMessage{
		Cipher:      thirdLayerEncrypted,
		Destination: node3.Name,
		HopLimit:    10,
	}
	finalGossipPacket := &message.GossipPacket{OnionMessage: finalPacket}

	return finalGossipPacket, nil
}

func (o *OnionAgent) DecryptLayer(msg *message.GossipPacket) (*message.GossipPacket, error) {
	decryptedMessageByte, err := rsa.DecryptPKCS1v15(rand.Reader, o.PrivateKey, msg.OnionMessage.Cipher)
	if err != nil {
		return nil, err
	}
	var decryptedMessage *message.GossipPacket
	err = protobuf.Decode(decryptedMessageByte, decryptedMessage)
	return decryptedMessage, err
}
