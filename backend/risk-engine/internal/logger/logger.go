package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var defaultLogger *zap.Logger

func Init(level, path string) error {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		lvl = zapcore.InfoLevel
	}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder = zapcore.NewJSONEncoder(encoderConfig)

	// Console output.
	consoleCore := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), lvl)

	cores := []zapcore.Core{consoleCore}

	// File output if path provided.
	if path != "" {
		if err := os.MkdirAll(path[:len(path)-len(getFileName(path))], 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		fileCore := zapcore.NewCore(encoder, zapcore.AddSync(f), lvl)
		cores = append(cores, fileCore)
	}

	defaultLogger = zap.New(zapcore.NewTee(cores...), zap.AddCallerSkip(1))
	return nil
}

func getFileName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

func L() *zap.Logger {
	if defaultLogger == nil {
		l, _ := zap.NewProduction()
		defaultLogger = l
	}
	return defaultLogger
}

func Info(msg string, fields ...zap.Field) {
	L().Info(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	L().Fatal(msg, fields...)
}
