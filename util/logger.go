package util

import (
	"fmt"

	"github.com/sniperHW/kendynet/golog"
)

func NewLogger(basePath string, fileName string, fileMax int) golog.LoggerI {
	outLogger := golog.NewOutputLogger(basePath, fileName, fileMax)
	logger := golog.New(fileName, outLogger)
	var fullname string
	if len(fileName) > 0 {
		fullname = fmt.Sprintf("(node_chat|%s)", fileName[0])
	} else {
		fullname = "(node_chat)"
	}
	logger.Debugf("%s logger init", fullname)
	return logger
}
