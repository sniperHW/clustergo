package codec

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/sniperHW/netgo"
)

const sizeLen int = 4

// 接收一个length|payload类型的数据包
type LengthPayloadPacketReceiver struct {
	MaxPacketSize int
	Buff          []byte
	w             int
	r             int
}

func (ss *LengthPayloadPacketReceiver) read(readable netgo.ReadAble, deadline time.Time) (int, error) {
	if err := readable.SetReadDeadline(deadline); err != nil {
		return 0, err
	} else {
		return readable.Read(ss.Buff[ss.w:])
	}
}

// 接收一个length|payload类型的数据包,返回payload
func (ss *LengthPayloadPacketReceiver) Recv(readable netgo.ReadAble, deadline time.Time) (pkt []byte, err error) {
	for {
		unpackSize := ss.w - ss.r
		if unpackSize >= sizeLen {
			payload := int(binary.BigEndian.Uint32(ss.Buff[ss.r:]))
			totalSize := payload + sizeLen

			if payload == 0 {
				return nil, fmt.Errorf("zero payload")
			} else if totalSize > ss.MaxPacketSize {
				return nil, fmt.Errorf("packet too large:%d", totalSize)
			} else if totalSize <= unpackSize {
				ss.r += sizeLen
				pkt := ss.Buff[ss.r : ss.r+payload]
				ss.r += payload
				if ss.r == ss.w {
					ss.r = 0
					ss.w = 0
				}
				return pkt, nil
			} else {
				if totalSize > cap(ss.Buff) {
					buff := make([]byte, totalSize)
					copy(buff, ss.Buff[ss.r:ss.w])
					ss.Buff = buff
				} else if ss.r > 0 {
					copy(ss.Buff, ss.Buff[ss.r:ss.w])
				}
				ss.w = ss.w - ss.r
				ss.r = 0
			}
		}

		var n int
		n, err = ss.read(readable, deadline)
		if n > 0 {
			ss.w += n
		}
		if nil != err {
			return
		}
	}
}
