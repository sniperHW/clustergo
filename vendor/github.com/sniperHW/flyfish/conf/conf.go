package conf

import (
	"github.com/BurntSushi/toml"
	"sync/atomic"
	"unsafe"
)

const (
	MaxPacketSize = 8 * 1024 * 1024 // 8mb
)

var (
	defConfig *Config
)

func LoadConfigStr(str string) error {
	config := &Config{}
	_, err := toml.Decode(str, config)
	if nil != err {
		return err
	} else {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&defConfig)), unsafe.Pointer(config))
		return nil
	}
}

func LoadConfig(path string) error {
	config := &Config{}
	_, err := toml.DecodeFile(path, config)
	if nil != err {
		return err
	} else {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&defConfig)), unsafe.Pointer(config))
		return nil
	}
}

func GetConfig() *Config {
	return (*Config)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&defConfig))))
}

type Config struct {
	CacheGroupSize       int
	MaxCachePerGroupSize int

	SqlLoadPipeLineSize int
	SqlLoadQueueSize    int
	SqlLoaderCount      int
	SqlUpdaterCount     int

	ServiceHost string
	ServicePort int
	//BinlogDir             string
	//BinlogPrefix          string
	ReplyBusyOnQueueFull  bool
	Compress              bool
	BatchByteSize         int
	BatchCount            int
	ProposalFlushInterval int
	ReadFlushInterval     int

	MaxPendingCmdCount      int // = int64(300000) //整个物理节点待处理的命令上限
	MaxPendingCmdCountPerKv int // = 100        //单个kv待处理命令上限

	DBConfig struct {
		SqlType string

		DbHost     string
		DbPort     int
		DbUser     string
		DbPassword string
		DbDataBase string

		ConfDbHost     string
		ConfDbPort     int
		ConfDbUser     string
		ConfDbPassword string
		ConfDataBase   string
	}

	Log struct {
		MaxLogfileSize  int
		LogDir          string
		LogPrefix       string
		LogLevel        string
		EnableLogStdout bool
	}
}
