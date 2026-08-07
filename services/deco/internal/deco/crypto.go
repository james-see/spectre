package deco

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

const (
	aesKeyBytes         = 16
	pkcs1v15HeaderBytes = 11
	minAESKey           = 1_000_000_000_000_000 // 10^15
	maxAESKeyExclusive  = 10_000_000_000_000_000
)

func randomAESDigitKey() (string, []byte, error) {
	span := big.NewInt(maxAESKeyExclusive - minAESKey)
	n, err := rand.Int(rand.Reader, span)
	if err != nil {
		return "", nil, err
	}
	val := n.Int64() + minAESKey
	s := fmt.Sprintf("%d", val)
	if len(s) != aesKeyBytes {
		return "", nil, fmt.Errorf("aes key digit length %d != %d", len(s), aesKeyBytes)
	}
	return s, []byte(s), nil
}

func byteLen(n *big.Int) int {
	return (n.BitLen() + 7) / 8
}

// rsaEncryptTPLink encrypts plaintext in PKCS#1 v1.5 blocks and concatenates hex.
func rsaEncryptTPLink(nHex string, eHex string, plaintext []byte) (string, error) {
	n := new(big.Int)
	if _, ok := n.SetString(nHex, 16); !ok {
		return "", fmt.Errorf("invalid RSA n")
	}
	e := new(big.Int)
	if _, ok := e.SetString(eHex, 16); !ok {
		return "", fmt.Errorf("invalid RSA e")
	}
	pub := &rsa.PublicKey{N: n, E: int(e.Int64())}
	blockSize := byteLen(n)
	bytesPerBlock := blockSize - pkcs1v15HeaderBytes
	if bytesPerBlock <= 0 {
		return "", fmt.Errorf("RSA modulus too small")
	}
	var out string
	for index := 0; index < len(plaintext); {
		end := index + bytesPerBlock
		if end > len(plaintext) {
			end = len(plaintext)
		}
		enc, err := rsa.EncryptPKCS1v15(rand.Reader, pub, plaintext[index:end])
		if err != nil {
			return "", err
		}
		out += hex.EncodeToString(enc)
		index = end
	}
	return out, nil
}

func aesEncryptCBC(key, iv, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

func aesDecryptCBC(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("ciphertext not multiple of block size")
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	return pkcs7Unpad(out)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty")
	}
	pad := int(data[len(data)-1])
	if pad < 1 || pad > len(data) {
		return nil, fmt.Errorf("bad padding")
	}
	return data[:len(data)-pad], nil
}

func authHash(username, password string) string {
	sum := md5.Sum([]byte(username + password))
	return hex.EncodeToString(sum[:])
}

func b64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func b64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
