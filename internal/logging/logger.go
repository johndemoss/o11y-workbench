package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var (
	currentLevel = INFO // default to INFO
	levelNames   = map[Level]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
	}
)

func SetLevel(level Level) {
	currentLevel = level
}

func SetLevelFromString(s string) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		currentLevel = DEBUG
	case "INFO":
		currentLevel = INFO
	case "WARN":
		currentLevel = WARN
	case "ERROR":
		currentLevel = ERROR
	default:
		// Invalid level, keep current
		log.Printf("Invalid log level '%s', keeping current level %s", s, levelNames[currentLevel])
	}
}

func logf(level Level, format string, args ...interface{}) {
	if level >= currentLevel {
		msg := fmt.Sprintf(format, args...)
		log.Printf("[%s] %s", levelNames[level], msg)
	}
}

func Debugf(format string, args ...interface{}) {
	logf(DEBUG, format, args...)
}

func Infof(format string, args ...interface{}) {
	logf(INFO, format, args...)
}

func Warnf(format string, args ...interface{}) {
	logf(WARN, format, args...)
}

func Errorf(format string, args ...interface{}) {
	logf(ERROR, format, args...)
}

// init reads O11Y_WORKBENCH_LOG_LEVEL from environment on package import
func init() {
	if logLevel := os.Getenv("O11Y_WORKBENCH_LOG_LEVEL"); logLevel != "" {
		SetLevelFromString(logLevel)
	}
}
