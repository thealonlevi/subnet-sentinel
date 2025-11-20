package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
)

type Logger struct {
	level      Level
	base       *log.Logger
	jsonFormat bool
}

func New(level, format string) (Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return Logger{}, err
	}
	isJSON := strings.ToLower(strings.TrimSpace(format)) == "json"
	return Logger{
		level:      lvl,
		base:       log.New(os.Stdout, "", log.LstdFlags),
		jsonFormat: isJSON,
	}, nil
}

func ParseLevel(level string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("invalid log level %s", level)
	}
}

func (l Logger) log(level string, message string, args ...interface{}) {
	if l.jsonFormat {
		msg := fmt.Sprintf(message, args...)
		entry := map[string]interface{}{
			"ts":      time.Now().Format(time.RFC3339),
			"level":   level,
			"message": msg,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			l.base.Printf("ERROR failed to marshal log entry: %v", err)
			return
		}
		l.base.Println(string(data))
	} else {
		l.base.Printf(level+" "+message, args...)
	}
}

func (l Logger) Debug(message string, args ...interface{}) {
	if l.level > LevelDebug {
		return
	}
	l.log("DEBUG", message, args...)
}

func (l Logger) Info(message string, args ...interface{}) {
	if l.level > LevelInfo {
		return
	}
	l.log("INFO", message, args...)
}

func (l Logger) Error(message string, args ...interface{}) {
	if l.level > LevelError {
		return
	}
	l.log("ERROR", message, args...)
}

func (l Logger) Warn(message string, args ...interface{}) {
	if l.level > LevelInfo {
		return
	}
	l.log("WARN", message, args...)
}
