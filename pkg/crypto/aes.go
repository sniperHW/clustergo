package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// minKeySize 是握手密钥的最小安全长度（AES-256）。短于该长度的密钥
// 会被 LoadKey 按零字节填充补齐，避免低熵密钥被轻易爆破或猜测。
const minKeySize = 32

func fixKey(key []byte) []byte {
	if len(key) > 32 {
		return key[:32]
	} else {
		var size int
		if len(key) < 16 {
			size = 16
		} else if len(key) < 24 {
			size = 24
		} else {
			size = 32
		}
		padding := size - (len(key))%size
		padtext := bytes.Repeat([]byte{byte(padding)}, padding)
		return append(key, padtext...)
	}
}

func paddingData(ciphertext []byte, blockSize int) []byte {
	paddingSize := blockSize - (len(ciphertext)+4)%blockSize
	ret := make([]byte, 4, len(ciphertext)+4+paddingSize)
	binary.BigEndian.PutUint32(ret, uint32(len(ciphertext)))
	ret = append(ret, ciphertext...)
	padding := blockSize - len(ret)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ret, padtext...)
}

/*
   AES  CBC 加密
   key:加密key
   plaintext：加密明文
   ciphertext:解密返回字节字符串[ 整型以十六进制方式显示]
*/

func AESCBCEncrypt(keybyte, plainbyte []byte) (cipherbyte []byte, err error) {

	keybyte = fixKey(keybyte)

	plainbyte = paddingData(plainbyte, aes.BlockSize)

	block, err := aes.NewCipher(keybyte)
	if err != nil {
		return
	}

	cipherbyte = make([]byte, aes.BlockSize+len(plainbyte))
	iv := cipherbyte[:aes.BlockSize]
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherbyte[aes.BlockSize:], plainbyte)
	return
}

/*
AES  CBC 解码
key:解密key
ciphertext:加密返回的串
plaintext：解密后的字符串
*/

func AESCBCDecrypter(keybyte, cipherbyte []byte) (plainbyte []byte, err error) {
	keybyte = fixKey(keybyte)

	block, err := aes.NewCipher(keybyte)
	if err != nil {
		return
	}
	if len(cipherbyte) < aes.BlockSize {
		err = errors.New("ciphertext too short")
		return
	}

	iv := cipherbyte[:aes.BlockSize]
	cipherbyte = cipherbyte[aes.BlockSize:]
	if len(cipherbyte)%aes.BlockSize != 0 {
		err = errors.New("ciphertext is not a multiple of the block size")
		return
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherbyte, cipherbyte)

	size := int(binary.BigEndian.Uint32(cipherbyte[:4]))

	plainbyte = cipherbyte[4 : 4+size]

	return
}

// LoadKey 从环境变量 envVar 读取握手密钥。若环境变量未设置或为空，
// 则回退到 def。无论来源如何，密钥都会被填充到至少 minKeySize 字节
// （零字节补齐），保证不会使用过短的弱密钥。
//
// 用法示例：
//
//	secretKey = crypto.LoadKey("CLUSTERGO_SECRET_KEY", []byte("default"))
func LoadKey(envVar string, def []byte) []byte {
	var raw []byte
	if v := os.Getenv(envVar); v != "" {
		raw = []byte(v)
	} else {
		raw = def
	}
	if len(raw) >= minKeySize {
		// 截断到 AES 支持的最大长度，避免 NewCipher 因超长密钥报错。
		if len(raw) > 32 {
			return raw[:32]
		}
		return raw
	}
	// 不足 minKeySize，用零字节补齐。补齐后的密钥长度落在 32（AES-256）。
	padded := make([]byte, minKeySize)
	copy(padded, raw)
	return padded
}

// AESEncrypt 使用 AES-GCM 加密 plain。与旧的 CBC 实现不同，GCM 是认证
// 加密模式：密文自带完整性校验（MAC），任何篡改都会在解密时被检测出来，
// 杜绝了 padding oracle / bit-flipping 攻击。
//
// 输出格式：nonce(12) || ciphertext || tag(16)
func AESEncrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Seal 把 nonce、密文、tag 顺序拼接到 nonce 前缀上，一次分配完成。
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// AESDecrypt 使用 AES-GCM 解密由 AESEncrypt 产生的 cipher。解密时
// 会校验认证标签：密文被篡改或长度不足时返回错误，而非返回错误数据
// 或 panic，消除了旧 CBC 实现中按不可信长度字段切片导致的越界问题。
func AESDecrypt(key, cipherB []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(cipherB) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := cipherB[:gcm.NonceSize()]
	// Open 在校验失败时返回 error，不输出任何明文，杜绝越界与伪造。
	return gcm.Open(nil, nonce, cipherB[gcm.NonceSize():], nil)
}
