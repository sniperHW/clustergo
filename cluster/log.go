package cluster

import (
	"github.com/sniperHW/kendynet/golog"
)

func InitLogger(l golog.LoggerI) {
	logger = l
}
