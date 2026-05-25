package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// L 全局日志器
var L *zap.SugaredLogger

// Init 初始化日志。debug=true 用开发模式(彩色、易读)，否则用生产模式(JSON)。
func Init(debug bool) {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.EncoderConfig.TimeKey = "ts"
	logger, _ := cfg.Build(zap.AddCallerSkip(1))
	L = logger.Sugar()
}

func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}
