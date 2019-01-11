package onion

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"

	"github.com/dedis/protobuf"
	"github.com/succa/Peerster/pkg/message"
)

type OnionAgent struct {
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
}

type OnionRouter struct {
	Name      string
	PublicKey string
}

type AESEncryption struct {
	Key   []byte
	Nonce []byte
}

func NewOnionAgent() *OnionAgent {
	privateKey, publicKey := GenerateRsaKeyPair()
	return &OnionAgent{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}
}

func GenerateRsaKeyPair() (*rsa.PrivateKey, *rsa.PublicKey) {
	privkey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return privkey, &privkey.PublicKey
}

func ExportRsaPrivateKeyAsPemStr(privkey *rsa.PrivateKey) string {
	privkey_bytes := x509.MarshalPKCS1PrivateKey(privkey)
	privkey_pem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privkey_bytes,
		},
	)
	return string(privkey_pem)
}

func ParseRsaPrivateKeyFromPemStr(privPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return priv, nil
}

func ExportRsaPublicKeyAsPemStr(pubkey *rsa.PublicKey) (string, error) {
	pubkey_bytes, err := x509.MarshalPKIXPublicKey(pubkey)
	if err != nil {
		return "", err
	}
	pubkey_pem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: pubkey_bytes,
		},
	)

	return string(pubkey_pem), nil
}

func ParseRsaPublicKeyFromPemStr(pubPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pubPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	switch pub := pub.(type) {
	case *rsa.PublicKey:
		return pub, nil
	default:
		break // fall through
	}
	return nil, errors.New("Key type is not RSA")
}

func (o *OnionAgent) GetPublicKeyString() string {
	pk, _ := ExportRsaPublicKeyAsPemStr(o.PublicKey)
	return pk
}

func (o *OnionAgent) OnionEncrypt(msg *message.GossipPacket, destination, node3, node2, node1 *OnionRouter) (*message.GossipPacket, error) {
	//fmt.Println("Called OnionEncrypt")
	//fmt.Println(destination.Name)
	//fmt.Println(node3.Name)
	//fmt.Println(node2.Name)
	//fmt.Println(node1.Name)

	// Each layer is encrypted with AES GMC, its simmetric key is encrypted with the RSA Pk and sent in the message
	AESEncryption := &AESEncryption{}

	//Message layer
	//AES Encryption
	gossipPacketByte, err := protobuf.Encode(msg)
	if err != nil {
		return nil, err
	}
	aesKey, Nonce, cipherText, err := NewGCMEncrypter(gossipPacketByte)
	if err != nil {
		return nil, err
	}
	AESEncryption.Key = aesKey
	AESEncryption.Nonce = Nonce
	AESEncryptionByte, err := protobuf.Encode(AESEncryption)
	if err != nil {
		return nil, err
	}
	//RSA Encryption
	destinationPk, err := ParseRsaPublicKeyFromPemStr(destination.PublicKey)
	if err != nil {
		return nil, err
	}
	encryptedAESKey, errPKCS1v15 := rsa.EncryptPKCS1v15(rand.Reader, destinationPk, AESEncryptionByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	// First layer
	firstLayer := &message.OnionMessage{
		Cipher:          cipherText,
		AESKeyEncrypted: encryptedAESKey,
		Destination:     destination.Name,
		LastNode:        true,
		HopLimit:        10,
	}
	firstLayerGossip := &message.GossipPacket{OnionMessage: firstLayer}
	fistLayerByte, err := protobuf.Encode(firstLayerGossip)
	if err != nil {
		return nil, err
	}
	aesKey, Nonce, cipherText, err = NewGCMEncrypter(fistLayerByte)
	if err != nil {
		return nil, err
	}
	AESEncryption.Key = aesKey
	AESEncryption.Nonce = Nonce
	AESEncryptionByte, err = protobuf.Encode(AESEncryption)
	if err != nil {
		return nil, err
	}
	node1Pk, err := ParseRsaPublicKeyFromPemStr(node1.PublicKey)
	if err != nil {
		return nil, err
	}
	encryptedAESKey, errPKCS1v15 = rsa.EncryptPKCS1v15(rand.Reader, node1Pk, AESEncryptionByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	//fmt.Println("After first layer")

	//Second layer
	secondLayer := &message.OnionMessage{
		Cipher:          cipherText,
		AESKeyEncrypted: encryptedAESKey,
		Destination:     node1.Name,
		LastNode:        false,
		HopLimit:        10,
	}
	secondLayerGossip := &message.GossipPacket{OnionMessage: secondLayer}
	secondLayerByte, err := protobuf.Encode(secondLayerGossip)
	if err != nil {
		return nil, err
	}
	aesKey, Nonce, cipherText, err = NewGCMEncrypter(secondLayerByte)
	if err != nil {
		return nil, err
	}
	AESEncryption.Key = aesKey
	AESEncryption.Nonce = Nonce
	AESEncryptionByte, err = protobuf.Encode(AESEncryption)
	if err != nil {
		return nil, err
	}
	node2Pk, err := ParseRsaPublicKeyFromPemStr(node2.PublicKey)
	if err != nil {
		return nil, err
	}
	encryptedAESKey, errPKCS1v15 = rsa.EncryptPKCS1v15(rand.Reader, node2Pk, AESEncryptionByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	//fmt.Println("After Second layer")

	//Third layer
	thirdLayer := &message.OnionMessage{
		Cipher:          cipherText,
		AESKeyEncrypted: encryptedAESKey,
		Destination:     node2.Name,
		LastNode:        false,
		HopLimit:        10,
	}
	thirdLayerGossip := &message.GossipPacket{OnionMessage: thirdLayer}
	thirdLayerByte, err := protobuf.Encode(thirdLayerGossip)
	if err != nil {
		return nil, err
	}
	aesKey, Nonce, cipherText, err = NewGCMEncrypter(thirdLayerByte)
	if err != nil {
		return nil, err
	}
	AESEncryption.Key = aesKey
	AESEncryption.Nonce = Nonce
	AESEncryptionByte, err = protobuf.Encode(AESEncryption)
	if err != nil {
		return nil, err
	}
	node3Pk, err := ParseRsaPublicKeyFromPemStr(node3.PublicKey)
	if err != nil {
		return nil, err
	}
	encryptedAESKey, errPKCS1v15 = rsa.EncryptPKCS1v15(rand.Reader, node3Pk, AESEncryptionByte)
	if errPKCS1v15 != nil {
		fmt.Println(errPKCS1v15)
		return nil, errPKCS1v15
	}

	//fmt.Println("After Third layer")

	//Final Packet
	finalPacket := &message.OnionMessage{
		Cipher:          cipherText,
		AESKeyEncrypted: encryptedAESKey,
		Destination:     node3.Name,
		HopLimit:        10,
	}
	finalGossipPacket := &message.GossipPacket{OnionMessage: finalPacket}

	//fmt.Println("After final")

	return finalGossipPacket, nil
}

func (o *OnionAgent) DecryptLayer(msg *message.GossipPacket) (*message.GossipPacket, error) {
	// Decrypt AES Key
	decryptedAESKeyByte, err := rsa.DecryptPKCS1v15(rand.Reader, o.PrivateKey, msg.OnionMessage.AESKeyEncrypted)
	if err != nil {
		return nil, err
	}
	//fmt.Println("Riccardo decryptedAESKeyByte")
	var decryptedAESKey AESEncryption
	err = protobuf.Decode(decryptedAESKeyByte, &decryptedAESKey)
	if err != nil {
		return nil, err
	}
	//fmt.Println("Riccardo decryptedAESKey")

	decryptedMessage, err := NewGCMDecrypter(decryptedAESKey.Key, decryptedAESKey.Nonce, msg.OnionMessage.Cipher)
	if err != nil {
		return nil, err
	}
	//fmt.Println("Riccardo decryptedMessage")

	return decryptedMessage, nil
}

func NewGCMEncrypter(plaintext []byte) ([]byte, []byte, []byte, error) {
	// The key argument should be the AES key, either 16 or 32 bytes
	// to select AES-128 or AES-256.
	key := make([]byte, 32)
	//plaintext := []byte("exampleplaintext")

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)
	return key, nonce, ciphertext, nil
}

func NewGCMDecrypter(key []byte, nonce []byte, cipherText []byte) (*message.GossipPacket, error) {
	// The key argument should be the AES key, either 16 or 32 bytes
	// to select AES-128 or AES-256.
	//key := []byte("AES256Key-32Characters1234567890")
	//ciphertext, _ := hex.DecodeString("f90fbef747e7212ad7410d0eee2d965de7e890471695cddd2a5bc0ef5da1d04ad8147b62141ad6e4914aee8c512f64fba9037603d41de0d50b718bd665f019cdcd")

	//nonce, _ := hex.DecodeString("bb8ef84243d2ee95a41c6c57")

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, err
	}

	var decryptedMessage message.GossipPacket
	err = protobuf.Decode(plaintext, &decryptedMessage)
	if err != nil {
		return nil, err
	}

	return &decryptedMessage, nil
}
