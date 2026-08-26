package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// L 全局日志器
var L *zap.SugaredLogger

// Init 初始化日志。debug=true 用开发模式(彩色、易读)，否则用生产模式(JSON)。
// Uber 开源的高性能日志库 zap 的初始化封装。
// 通过一个debug开关，在开发调试模式和生产部署模式之间切换日志配置，最终生成一个全局可用的日志实例。
func Init(debug bool) {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.EncoderConfig.TimeKey = "ts"
	logger, _ := cfg.Build(zap.AddCallerSkip(1)) //根据配置构建一个可用的 Logger 实例，并修正日志中显示的文件行号”
	L = logger.Sugar()                           //将“高性能但写法繁琐”的 Logger，转换为“灵活易用但性能略低”的 SugaredLogger
}

func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}
