package addr

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"
)

const ClusterMask uint32 = 0xFFFC0000 //高14
const TypeMask uint32 = 0x0003FC00    //中8
const ServerMask uint32 = 0x000003FF  //低10
const HarbarType uint32 = 255

var ErrInvaildAddrFmt error = fmt.Errorf("invaild addr format")
var ErrHarborType error = fmt.Errorf("type should be 255")
var ErrInvaildType error = fmt.Errorf("type should between(1,254)")
var ErrInvaildCluster error = fmt.Errorf("cluster should between(1,16383)")
var ErrInvaildServer error = fmt.Errorf("server should between(0,1023)")

type LogicAddr uint32

type Addr struct {
	logicAddr LogicAddr
	netAddr   *net.TCPAddr
}

func MakeAddr(logic string, tcpAddr string) (Addr, error) {
	logicAddr, err := MakeLogicAddr(logic)
	if nil != err {
		return Addr{}, err
	}

	netAddr, err := net.ResolveTCPAddr("tcp", tcpAddr)
	if nil != err {
		return Addr{}, err
	}

	return Addr{
		logicAddr: logicAddr,
		netAddr:   netAddr,
	}, nil
}

func MakeHarborAddr(logic string, tcpAddr string) (Addr, error) {
	logicAddr, err := MakeHarborLogicAddr(logic)
	if nil != err {
		return Addr{}, err
	}

	netAddr, err := net.ResolveTCPAddr("tcp", tcpAddr)
	if nil != err {
		return Addr{}, err
	}

	return Addr{
		logicAddr: logicAddr,
		netAddr:   netAddr,
	}, nil
}

func (a Addr) LogicAddr() LogicAddr {
	return a.logicAddr
}

func (a Addr) NetAddr() *net.TCPAddr {
	return (*net.TCPAddr)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&a.netAddr))))
}

func (a *Addr) UpdateNetAddr(addr *net.TCPAddr) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&a.netAddr)), unsafe.Pointer(addr))
}

func (a LogicAddr) Cluster() uint32 {
	return (uint32(a) & ClusterMask) >> 18
}

func (a LogicAddr) Type() uint32 {
	return (uint32(a) & TypeMask) >> 10
}

func (a LogicAddr) Server() uint32 {
	return uint32(a) & ServerMask
}

func (a LogicAddr) String() string {
	return fmt.Sprintf("%d.%d.%d", a.Cluster(), a.Type(), a.Server())
}

func (a LogicAddr) Empty() bool {
	return uint32(a) == 0
}

func (a *LogicAddr) Clear() {
	(*a) = 0
}

func MakeLogicAddr(addr string) (LogicAddr, error) {
	var err error
	v := strings.Split(addr, ".")
	if len(v) != 3 {
		return LogicAddr(0), ErrInvaildAddrFmt
	}

	cluster, err := strconv.Atoi(v[0])

	if nil != err {
		return LogicAddr(0), ErrInvaildCluster
	}

	if cluster == 0 || uint32(cluster) > (ClusterMask>>18) {
		return LogicAddr(0), ErrInvaildCluster
	}

	tt, err := strconv.Atoi(v[1])
	if nil != err {
		return LogicAddr(0), ErrInvaildType
	}

	if tt == 0 || uint32(tt) > ((TypeMask>>10)-1) {
		return LogicAddr(0), ErrInvaildType
	}

	server, err := strconv.Atoi(v[2])
	if nil != err {
		return LogicAddr(0), ErrInvaildServer
	}

	if uint32(server) > ServerMask {
		return LogicAddr(0), ErrInvaildServer
	}

	return LogicAddr(0 | (uint32(tt) << 10) | (uint32(cluster) << 18) | (uint32(server))), nil
}

func MakeHarborLogicAddr(addr string) (LogicAddr, error) {

	var err error
	v := strings.Split(addr, ".")
	if len(v) != 3 {
		return LogicAddr(0), ErrInvaildAddrFmt
	}

	cluster, err := strconv.Atoi(v[0])

	if nil != err {
		return LogicAddr(0), ErrInvaildCluster
	}

	if cluster == 0 || uint32(cluster) > (ClusterMask>>18) {
		return LogicAddr(0), ErrInvaildCluster
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

	if uint32(server) > ServerMask {
		return LogicAddr(0), ErrInvaildServer
	}

	return LogicAddr(0 | (uint32(tt) << 10) | (uint32(cluster) << 18) | (uint32(server))), nil
}
