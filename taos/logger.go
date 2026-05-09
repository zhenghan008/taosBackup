package taos

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
)

func Logger(LogToFile bool, LogToConsole bool) *zap.Logger {
	var cores []zapcore.Core
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if LogToFile {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   "./backup.log",
			MaxSize:    500,
			MaxBackups: 100,
			MaxAge:     30,
			Compress:   true,
		})
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			fileWriter,
			zap.WarnLevel,
		)
		cores = append(cores, fileCore)
	}
	if LogToConsole {
		consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		consoleCore := zapcore.NewCore(
			consoleEncoder,
			zapcore.AddSync(os.Stdout),
			zap.DebugLevel,
		)
		cores = append(cores, consoleCore)
	}
	core := zapcore.NewTee(cores...)
	logger := zap.New(core, zap.AddCaller())
	return logger
}
