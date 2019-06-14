package addr

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const GroupMask uint32 = 0xFFF00000
const TypeMask uint32 = 0x000FF000
const ServerMask uint32 = 0x00000FFF

var ErrInvaildAddrFmt error = fmt.Errorf("invaild addr format")
var ErrHarborType error = fmt.Errorf("type should be 255")
var ErrInvaildType error = fmt.Errorf("type should between(1,254)")
var ErrInvaildGroup error = fmt.Errorf("group should between(1,4095)")
var ErrInvaildServer error = fmt.Errorf("server should between(1,4095)")

type LogicAddr uint32

type Addr struct {
	Logic LogicAddr
	Net   *net.TCPAddr
}

func MakeAddr(logic string, tcpAddr string) (Addr, error) {
	logicAddr, err := MakeLogicAddr(logic)
	if nil != err {
		return Addr{}, err
	}

	netAddr, err := net.ResolveTCPAddr("tcp4", tcpAddr)
	if nil != err {
		return Addr{}, err
	}

	return Addr{
		Logic: logicAddr,
		Net:   netAddr,
	}, nil
}

func MakeHarborAddr(logic string, tcpAddr string) (Addr, error) {
	logicAddr, err := MakeHarborLogicAddr(logic)
	if nil != err {
		return Addr{}, err
	}

	netAddr, err := net.ResolveTCPAddr("tcp4", tcpAddr)
	if nil != err {
		return Addr{}, err
	}

	return Addr{
		Logic: logicAddr,
		Net:   netAddr,
	}, nil
}

func (this LogicAddr) Group() uint32 {
	return (uint32(this) & GroupMask) >> 20
}

func (this LogicAddr) Type() uint32 {
	return (uint32(this) & TypeMask) >> 12
}

func (this LogicAddr) Server() uint32 {
	return uint32(this) & ServerMask
}

func (this LogicAddr) String() string {
	return fmt.Sprintf("%d.%d.%d", this.Group(), this.Type(), this.Server())
}

func (this LogicAddr) Empty() bool {
	return uint32(this) == 0
}

func (this *LogicAddr) Clear() {
	(*this) = 0
}

func MakeLogicAddr(addr string) (LogicAddr, error) {
	var err error
	v := strings.Split(addr, ".")
	if len(v) != 3 {
		return LogicAddr(0), ErrInvaildAddrFmt
	}

	group, err := strconv.Atoi(v[0])

	if nil != err {
		return LogicAddr(0), ErrInvaildGroup
	}

	if 0 == group || uint32(group) > (GroupMask>>20) {
		return LogicAddr(0), ErrInvaildGroup
	}

	tt, err := strconv.Atoi(v[1])
	if nil != err {
		return LogicAddr(0), ErrInvaildType
	}

	if 0 == tt || uint32(tt) > ((TypeMask>>12)-1) {
		return LogicAddr(0), ErrInvaildType
	}

	server, err := strconv.Atoi(v[2])
	if nil != err {
		return LogicAddr(0), ErrInvaildServer
	}

	if 0 == server || uint32(server) > ServerMask {
		return LogicAddr(0), ErrInvaildServer
	}

	return LogicAddr(0 | (uint32(tt) << 12) | (uint32(group) << 20) | (uint32(server))), nil
}

func MakeHarborLogicAddr(addr string) (LogicAddr, error) {

	var err error
	v := strings.Split(addr, ".")
	if len(v) != 3 {
		return LogicAddr(0), ErrInvaildAddrFmt
	}

	group, err := strconv.Atoi(v[0])

	if nil != err {
		return LogicAddr(0), ErrInvaildGroup
	}

	if 0 == group || uint32(group) > (GroupMask>>20) {
		return LogicAddr(0), ErrInvaildGroup
	}

	tt, err := strconv.Atoi(v[1])
	if nil != err {
		return LogicAddr(0), ErrInvaildType
	}

	if uint32(tt) != uint32(255) {
		return LogicAddr(0), ErrHarborType
	}

	server, err := strconv.Atoi(v[2])
	if nil != err {
		return LogicAddr(0), ErrInvaildServer
	}

	if 0 == server || uint32(server) > ServerMask {
		return LogicAddr(0), ErrInvaildServer
	}

	return LogicAddr(0 | (uint32(tt) << 12) | (uint32(group) << 20) | (uint32(server))), nil
}
