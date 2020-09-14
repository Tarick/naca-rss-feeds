package zaplogger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config mapstructure is for Viper to unmarshal
// TODO: add validation
type Config struct {
	Development       bool     `mapstructure:"development"`
	Level             string   `mapstructure:"level"`
	Encoding          string   `mapstructure:"encoding"`
	DisableCaller     bool     `mapstructure:"disable_caller"`
	DisableStacktrace bool     `mapstructure:"disable_stacktrace"`
	DisableColor      bool     `mapstructure:"disable_color"`
	OutputPaths       []string `mapstructure:"output_paths"`
	ErrorOutputPaths  []string `mapstructure:"error_output_paths"`
}

// New returns initialised logger
func New(logCfg *Config) *zap.Logger {
	zapCfg := zap.Config{Encoding: logCfg.Encoding,
		Development:       logCfg.Development,
		DisableCaller:     logCfg.DisableCaller,
		DisableStacktrace: logCfg.DisableStacktrace,
		ErrorOutputPaths:  logCfg.ErrorOutputPaths,
		OutputPaths:       logCfg.OutputPaths,
	}
	var zapLvl zapcore.Level
	if err := zapLvl.UnmarshalText([]byte(logCfg.Level)); err != nil {
		fmt.Println("Incorrect logging.level value,", logCfg.Level)
		os.Exit(1)
	}
	zapCfg.Level = zap.NewAtomicLevelAt(zapLvl)
	zapCfg.EncoderConfig = zapcore.EncoderConfig{}
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	if logCfg.DisableColor || logCfg.Encoding == "json" {
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	} else {
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	zapCfg.EncoderConfig.TimeKey = "time"
	zapCfg.EncoderConfig.MessageKey = "message"
	zapCfg.EncoderConfig.LevelKey = "severity"
	zapCfg.EncoderConfig.CallerKey = "caller"
	zapCfg.EncoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
	logger, err := zapCfg.Build()
	if err != nil {
		fmt.Println("Failure initialising logger:", err)
		os.Exit(1)
	}
	return logger
}
