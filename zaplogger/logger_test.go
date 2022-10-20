package zaplogger

import (
	"fmt"
	"testing"
)

func TestGetLogger(t *testing.T) {
	NewZapLogger("zapLogger.log", "log", "debug", 1, 1, 10, true)
	GetLogger().Info("hello world.")
	GetLogger().Debug("hello world.")
	GetLogger().Error("hello world.")

}

func TestGetSugar(t *testing.T) {
	NewZapLogger("zapLogger.log", "log", "debug", 1, 1, 10, true)
	GetSugar().Info("hello world.")
	GetSugar().Infof("hello %s.", "world")

	GetSugar().Debug("hello world.")
	GetSugar().Debug("hello world.")
	GetSugar().Debugf("hello %s.", "world")
	GetSugar().Debugf("hello %s.", "world")

	GetSugar().Error("hello world.")
	GetSugar().Error("hello world.")
	GetSugar().Errorf("hello %s.", "world")
	GetSugar().Errorf("hello %s.", "world")

	GetSugar().Info("hello world.\n")
	GetSugar().Infof("hello %s.\n", "world")

}

func TestFmt(t *testing.T) {
	msg := fmt.Sprint("aaa bbb")

	fmt.Println(msg)
	fmt.Println(msg)

	msg = fmt.Sprintf("%s", "hello")
	fmt.Println(msg)
	fmt.Println(msg)
}
