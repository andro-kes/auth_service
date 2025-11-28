package logger

import (
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Config contains options for initializing the zap logger.
type Config struct {
	// Level is the minimum log level. e.g. "debug", "info", "warn", "error".
	Level string

	// Encoding specifies the encoder: "json" or "console".
	// Default: "json"
	Encoding string

	// OutputPaths are additional output targets besides the default.
	// If Filename is set and FileRotation is true, logs will also be written to the rotated file.
	// Default: stdout
	OutputPaths []string

	// ErrorOutputPaths specifies where internal zap errors are written.
	ErrorOutputPaths []string

	// File rotation options: if Filename is non-empty and FileRotation true,
	// logs will be written to that file using lumberjack for rotation.
	FileRotation bool
	Filename     string
	MaxSize      int  // megabytes
	MaxBackups   int  // number of backups
	MaxAge       int  // days
	Compress     bool // compress rotated files

	// Development toggles development settings (more stack traces, console encoder defaults)
	Development bool

	// TimeEncoder optionally override time encoder; if nil, a sensible default is used.
	TimeEncoder zapcore.TimeEncoder
}

// package-level logger instances (singletons)
var (
	zapLogger  *zap.Logger
	sugar      *zap.SugaredLogger
	initialized = false
)

// Init initializes the package logger with the given config.
// It sets package-global logger and sugared logger used by helper functions.
// Calling Init multiple times will replace the previous logger (Sync will be attempted).
func Init(cfg Config) error {
	// If previously initialized, attempt to Sync old logger.
	if initialized {
		_ = Sync()
		zapLogger = nil
		sugar = nil
		initialized = false
	}

	if cfg.Encoding == "" {
		if cfg.Development {
			cfg.Encoding = "console"
		} else {
			cfg.Encoding = "json"
		}
	}

	// Parse level
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}

	// Encoder config
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "ts",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     cfg.TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Provide default time encoder if none set
	if encoderCfg.EncodeTime == nil {
		encoderCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			// ISO8601-ish format
			enc.AppendString(t.UTC().Format(time.RFC3339Nano))
		}
	}

	var encoder zapcore.Encoder
	if strings.EqualFold(cfg.Encoding, "console") {
		// In console mode, prefer capitalized level for readability
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	// Build write syncers
	var syncers []zapcore.WriteSyncer

	// Always include stdout as a default sink (so logs appear in containers)
	syncers = append(syncers, zapcore.AddSync(os.Stdout))

	// If user provided explicit output paths, add them (except stdout/stderr which are handled)
	for _, p := range cfg.OutputPaths {
		lower := strings.ToLower(p)
		switch lower {
		case "stdout":
			// already added
		case "stderr":
			syncers = append(syncers, zapcore.AddSync(os.Stderr))
		default:
			// treat as file path
			f, ferr := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if ferr != nil {
				// return error to caller; don't silently ignore
				return fmt.Errorf("failed to open output path %s: %w", p, ferr)
			}
			syncers = append(syncers, zapcore.AddSync(f))
		}
	}

	// If file rotation enabled and filename provided, create a lumberjack logger
	if cfg.FileRotation && cfg.Filename != "" {
		if cfg.MaxSize == 0 {
			cfg.MaxSize = 100 // sensible default
		}
		if cfg.MaxBackups == 0 {
			cfg.MaxBackups = 7
		}
		if cfg.MaxAge == 0 {
			cfg.MaxAge = 30
		}
		l := &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		syncers = append(syncers, zapcore.AddSync(l))
	} else if cfg.Filename != "" && !cfg.FileRotation {
		// if FileRotation is false but a filename is provided, open file without rotation
		f, ferr := os.OpenFile(cfg.Filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr != nil {
			return fmt.Errorf("failed to open file %s: %w", cfg.Filename, ferr)
		}
		syncers = append(syncers, zapcore.AddSync(f))
	}

	// Combine syncers into one core sink
	var core zapcore.Core
	if len(syncers) == 1 {
		core = zapcore.NewCore(encoder, syncers[0], level)
	} else {
		core = zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(syncers...), level)
	}

	// Options
	opts := []zap.Option{
		zap.AddCaller(),             // include caller info
		zap.AddCallerSkip(1),        // adjust for wrapper functions
		zap.AddStacktrace(zapcore.ErrorLevel),
	}

	if cfg.Development {
		opts = append(opts, zap.Development())
	}

	zapLogger = zap.New(core, opts...)
	sugar = zapLogger.Sugar()
	initialized = true

	return nil
}

// Sync flushes any buffered logs. It is safe to call multiple times.
func Sync() error {
	if sugar != nil {
		_ = sugar.Sync() // sugar.Sync delegates to underlying logger
	}
	if zapLogger != nil {
		return zapLogger.Sync()
	}
	return nil
}

// Logger returns the underlying *zap.Logger. If Init hasn't been called it will create
// a sensible default logger (production json to stdout, info level).
func Logger() *zap.Logger {
	if !initialized {
		_ = Init(Config{})
	}
	return zapLogger
}

// Sugar returns the package-wide *zap.SugaredLogger. If Init hasn't been called it will initialize defaults.
func Sugar() *zap.SugaredLogger {
	if !initialized {
		_ = Init(Config{})
	}
	return sugar
}

// parseLevel converts a string to zapcore.LevelEnabler. Default is info.
func parseLevel(l string) (zapcore.LevelEnabler, error) {
	if l == "" {
		return zapcore.InfoLevel, nil
	}
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "dpanic":
		return zapcore.DPanicLevel, nil
	case "panic":
		return zapcore.PanicLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		// try to parse numeric level (e.g., "-1")
		var zl zapcore.Level
		if err := zl.UnmarshalText([]byte(l)); err == nil {
			return zl, nil
		}
		return zapcore.InfoLevel, fmt.Errorf("unknown log level: %s", l)
	}
}