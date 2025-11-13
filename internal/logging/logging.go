package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
)

type Logger struct {
	level Level
	base  *log.Logger
}

func New(level string) (Logger, error) {
	lvl, err := ParseLevel(level)
	if err != nil {
		return Logger{}, err
	}
	return Logger{
		level: lvl,
		base:  log.New(os.Stdout, "", log.LstdFlags),
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

func (l Logger) Debug(message string, args ...interface{}) {
	if l.level > LevelDebug {
		return
	}
	l.base.Printf("DEBUG "+message, args...)
}

func (l Logger) Info(message string, args ...interface{}) {
	if l.level > LevelInfo {
		return
	}
	l.base.Printf("INFO "+message, args...)
}

func (l Logger) Error(message string, args ...interface{}) {
	if l.level > LevelError {
		return
	}
	l.base.Printf("ERROR "+message, args...)
}
