package util

import (
	"github.com/sniperHW/kendynet/golog"
)

var logger golog.LoggerI

func NewLogger(basePath string, fileName string, fileMax int) golog.LoggerI {
	outLogger := golog.NewOutputLogger(basePath, fileName, fileMax)
	logger = golog.New(fileName, outLogger)
	logger.Debugf("%s logger init", fileName)
	return logger
}

func Logger() golog.LoggerI {
	return logger
}
