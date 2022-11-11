package crypto

import (
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
