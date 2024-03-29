package zap

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	//"clustergo/node/common/config"
	"os"
)

type stdoutWriteSyncer struct {
}

func (s stdoutWriteSyncer) Write(p []byte) (n int, err error) {
	n = len(p)
	os.Stdout.Write(p)
	return
}

func (s stdoutWriteSyncer) Sync() error {
	return nil
}

var levelMap = map[string]zapcore.Level{

	"debug": zapcore.DebugLevel,

	"info": zapcore.InfoLevel,

	"warn": zapcore.WarnLevel,

	"error": zapcore.ErrorLevel,

	"dpanic": zapcore.DPanicLevel,

	"panic": zapcore.PanicLevel,

	"fatal": zapcore.FatalLevel,
}

func getLoggerLevel(lvl string) zapcore.Level {
	if level, ok := levelMap[lvl]; ok {
		return level
	}
	return zapcore.InfoLevel
}

func NewZapLogger(name string, path string, level string, maxLogfileSize int, maxAge int, maxBackups int, enableLogStdout bool) *zap.Logger /**zap.SugaredLogger*/ {

	syncWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   path + "/" + name,
		MaxSize:    maxLogfileSize,
		MaxAge:     maxAge,
		MaxBackups: maxBackups,
		LocalTime:  true,
		//Compress: true,
	})

	var w zapcore.WriteSyncer

	encoder := zap.NewProductionEncoderConfig()

	encoder.EncodeTime = zapcore.ISO8601TimeEncoder

	if enableLogStdout {
		//encoder.EncodeLevel = zapcore.CapitalColorLevelEncoder
		w = zap.CombineWriteSyncers(syncWriter, stdoutWriteSyncer{})
	} else {
		w = zap.CombineWriteSyncers(syncWriter)
	}
	/*NewConsoleEncoder*/
	/*NewJSONEncoder*/

	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoder), w, zap.NewAtomicLevelAt(getLoggerLevel(level)))

	return zap.New(core, zap.AddCaller()) //.Sugar()
}
