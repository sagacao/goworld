package gwlog

import (
	"os"
	"runtime/debug"

	"strings"

	"encoding/json"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	// DebugLevel level
	DebugLevel Level = Level(zap.DebugLevel)
	// InfoLevel level
	InfoLevel Level = Level(zap.InfoLevel)
	// WarnLevel level
	WarnLevel Level = Level(zap.WarnLevel)
	// ErrorLevel level
	ErrorLevel Level = Level(zap.ErrorLevel)
	// PanicLevel level
	PanicLevel Level = Level(zap.PanicLevel)
	// FatalLevel level
	FatalLevel Level = Level(zap.FatalLevel)
)

type logFormatFunc func(format string, args ...interface{})

// Level is type of log levels
type Level = zapcore.Level

var (
	cfg          zap.Config
	logger       *zap.Logger
	sugar        *zap.SugaredLogger
	source       string
	filename     string
	logStd       bool
	currentLevel Level
)

func init() {
	var err error
	cfgJson := []byte(`{
		"level": "debug",
		"outputPaths": ["stderr"],
		"errorOutputPaths": ["stderr"],
		"encoding": "console",
		"encoderConfig": {
			"messageKey": "message",
			"levelKey": "level",
			"levelEncoder": "lowercase"
		}
	}`)
	currentLevel = DebugLevel

	if err = json.Unmarshal(cfgJson, &cfg); err != nil {
		panic(err)
	}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	rebuildLoggerFromCfg()
}

// SetSource sets the component name (dispatcher/gate/game) of gwlog module
func SetSource(source_ string) {
	source = source_
	rebuildLoggerFromCfg()
}

// SetLevel sets the log level
func SetLevel(lv Level) {
	currentLevel = lv
	cfg.Level.SetLevel(lv)
}

// GetLevel get the current log level
func GetLevel() Level {
	return currentLevel
}

// TraceError prints the stack and error
func TraceError(format string, args ...interface{}) {
	Error(string(debug.Stack()))
	Errorf(format, args...)
}

// SetOutput sets the output writer
func SetOutput(outputs []string) {
	// cfg.OutputPaths = outputs
	// rebuildLoggerFromCfg()
}

// SetRotateFile sets the output writer
func SetRotateFile(logfile string, logstd bool) {
	filename = logfile
	logStd = logstd
	rebuildLoggerFromCfg()
}

// ParseLevel converts string to Levels
func ParseLevel(s string) Level {
	if strings.ToLower(s) == "debug" {
		return DebugLevel
	} else if strings.ToLower(s) == "info" {
		return InfoLevel
	} else if strings.ToLower(s) == "warn" || strings.ToLower(s) == "warning" {
		return WarnLevel
	} else if strings.ToLower(s) == "error" {
		return ErrorLevel
	} else if strings.ToLower(s) == "panic" {
		return PanicLevel
	} else if strings.ToLower(s) == "fatal" {
		return FatalLevel
	}
	Errorf("ParseLevel: unknown level: %s", s)
	return DebugLevel
}

func rebuildLoggerFromCfg() {
	syncWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:  filename,
		MaxSize:   128, //MB //1 << 30, //1G
		LocalTime: true,
		Compress:  true,
	})
	logPriority := zap.NewAtomicLevelAt(currentLevel)

	var allCore []zapcore.Core

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)
	if logStd {
		consoleConfig := zap.NewDevelopmentEncoderConfig()
		consoleConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		consoleEncoder := zapcore.NewConsoleEncoder(consoleConfig)
		consoleDebugging := zapcore.Lock(os.Stderr) //zapcore.Lock(os.Stdout)
		allCore = append(allCore, zapcore.NewCore(consoleEncoder, consoleDebugging, logPriority))
	}
	allCore = append(allCore, zapcore.NewCore(jsonEncoder, syncWriter, logPriority))

	core := zapcore.NewTee(allCore...)
	logger := zap.New(core).WithOptions(zap.AddCaller(), zap.AddCallerSkip(1))
	if source != "" {
		// logger = logger.With(zap.String("source", source))
		logger = logger.WithOptions(zap.Fields(zap.String("source", source)))
	}
	setSugar(logger.Sugar())

	// if newLogger, err := cfg.Build(); err == nil {
	// 	if logger != nil {
	// 		logger.Sync()
	// 	}
	// 	logger = newLogger
	// 	//logger = logger.With(zap.Time("ts", time.Now()))
	// 	if source != "" {
	// 		logger = logger.With(zap.String("source", source))
	// 	}
	// 	setSugar(logger.Sugar())
	// } else {
	// 	panic(err)
	// }
}

func Debugf(format string, args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Debugf(format, args...)
	sugar.Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Infof(format, args...)
	sugar.Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Warnf(format, args...)
	sugar.Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Errorf(format, args...)
	sugar.Errorf(format, args...)
}

func Panicf(format string, args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Panicf(format, args...)
	sugar.Panicf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	debug.PrintStack()
	// sugar.With(zap.Time("ts", time.Now())).Fatalf(format, args...)
	sugar.Fatalf(format, args...)
}

func Debug(args ...interface{}) {
	sugar.Debug(args...)
}

func Info(args ...interface{}) {
	sugar.Info(args...)
}

func Warn(args ...interface{}) {
	sugar.Warn(args...)
}

func Error(args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Error(args...)
	sugar.Error(args...)
}

func Panic(args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Panic(args...)
	sugar.Panic(args...)
}

func Fatal(args ...interface{}) {
	// sugar.With(zap.Time("ts", time.Now())).Fatal(args...)
	sugar.Fatal(args...)
}

func setSugar(sugar_ *zap.SugaredLogger) {
	sugar = sugar_
}
