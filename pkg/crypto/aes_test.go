package crypto

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"
)

func TestAes(t *testing.T) {
	b, err := AESCBCEncrypt([]byte("123456"), []byte("123456"))
	if err != nil {
		panic(err)
	} else {
		fmt.Println(base64.StdEncoding.EncodeToString(b))
		//b64 := base64.NewEncoder()
	}
}

func TestAESGCMRoundTrip(t *testing.T) {
	key := LoadKey("CLUSTERGO_SECRET_KEY_TEST", []byte("short"))
	if len(key) != minKeySize {
		t.Fatalf("LoadKey padded length = %d, want %d", len(key), minKeySize)
	}
	plain := []byte(`{"LogicAddr":123456,"NetAddr":"1.2.3.4:8080","IsStream":false}`)
	ct, err := AESEncrypt(key, plain)
	if err != nil {
		t.Fatalf("AESEncrypt: %v", err)
	}
	pt, err := AESDecrypt(key, ct)
	if err != nil {
		t.Fatalf("AESDecrypt: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", pt, plain)
	}
}

func TestAESGCMTamperDetected(t *testing.T) {
	key := LoadKey("CLUSTERGO_SECRET_KEY_TEST", []byte("short"))
	ct, err := AESEncrypt(key, []byte("secret"))
	if err != nil {
		t.Fatalf("AESEncrypt: %v", err)
	}
	// 翻转密文中的一个字节，GCM 认证标签校验应失败。
	ct[len(ct)-1] ^= 0xff
	if _, err := AESDecrypt(key, ct); err == nil {
		t.Fatal("AESDecrypt on tampered ciphertext: expected error, got nil")
	}
}

func TestAESGCMWrongKey(t *testing.T) {
	enc, dec := LoadKey("", []byte("key-a")), LoadKey("", []byte("key-b"))
	ct, err := AESEncrypt(enc, []byte("secret"))
	if err != nil {
		t.Fatalf("AESEncrypt: %v", err)
	}
	if _, err := AESDecrypt(dec, ct); err == nil {
		t.Fatal("AESDecrypt with wrong key: expected error, got nil")
	}
}
