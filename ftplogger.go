package graval

import (
	"log"
	"strings"
	"time"
)

type FtpLogLevel int

const (
	ErrorLevel FtpLogLevel = iota
	WarnLevel
	InfoLevel
	DebugLevel
)

var FtpLogTimeFormat = "2006-01-02 15:04:05"

type FTPLogger interface {
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
}

// Use an instance of this to log in a standard format
type ftpLogger struct {
}

func NewDefaultFtpLogger() FTPLogger {
	return &ftpLogger{}
}

func (logger *ftpLogger) header(level string) {
	log.Printf("[%s] %s ", time.Now().Format(FtpLogTimeFormat), level)
}

func (logger *ftpLogger) Info(args ...interface{}) {
	logger.header("INFO")
	log.Println(args...)
}

func (logger *ftpLogger) Infof(format string, args ...interface{}) {
	logger.header("INFO")
	log.Printf(format, args...)
	if !strings.HasPrefix(format, "\n") {
		log.Println()
	}
}
func (logger *ftpLogger) Warn(args ...interface{}) {
	logger.header("WARN")
	log.Println(args...)
}

func (logger *ftpLogger) Warnf(format string, args ...interface{}) {
	logger.header("WARN")
	log.Printf(format, args...)
	if !strings.HasPrefix(format, "\n") {
		log.Println()
	}
}

func (logger *ftpLogger) Error(args ...interface{}) {
	logger.header("ERROR")
	log.Println(args...)
}

func (logger *ftpLogger) Errorf(format string, args ...interface{}) {
	logger.header("WARN")
	log.Printf(format, args...)
	if !strings.HasPrefix(format, "\n") {
		log.Println()
	}
}

func (logger *ftpLogger) Debug(args ...interface{}) {
	logger.header("WARN")
	log.Println(args...)
}

func (logger *ftpLogger) Debugf(format string, args ...interface{}) {
	logger.header("DEBUG")
	log.Printf(format, args...)
	if !strings.HasPrefix(format, "\n") {
		log.Println()
	}
}
