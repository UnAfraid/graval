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
	ftpLogLevel FtpLogLevel
}

func NewDefaultFtpLogger() FTPLogger {
	return NewDefaultFtpLoggerWithLevel(InfoLevel)
}

func NewDefaultFtpLoggerWithLevel(ftpLogLevel FtpLogLevel) FTPLogger {
	return &ftpLogger{
		ftpLogLevel: ftpLogLevel,
	}
}

func (logger *ftpLogger) header(level string) {
	log.Printf("[%s] %s ", time.Now().Format(FtpLogTimeFormat), level)
}

func (logger *ftpLogger) Info(args ...interface{}) {
	if logger.ftpLogLevel <= InfoLevel {
		logger.header("INFO")
		log.Println(args...)
	}
}

func (logger *ftpLogger) Infof(format string, args ...interface{}) {
	if logger.ftpLogLevel <= InfoLevel {
		logger.header("INFO")
		log.Printf(format, args...)
		if !strings.HasPrefix(format, "\n") {
			log.Println()
		}
	}
}
func (logger *ftpLogger) Warn(args ...interface{}) {
	if logger.ftpLogLevel <= WarnLevel {
		logger.header("WARN")
		log.Println(args...)
	}
}

func (logger *ftpLogger) Warnf(format string, args ...interface{}) {
	if logger.ftpLogLevel <= WarnLevel {
		logger.header("WARN")
		log.Printf(format, args...)
		if !strings.HasPrefix(format, "\n") {
			log.Println()
		}
	}
}

func (logger *ftpLogger) Error(args ...interface{}) {
	if logger.ftpLogLevel <= ErrorLevel {
		logger.header("ERROR")
		log.Println(args...)
	}
}

func (logger *ftpLogger) Errorf(format string, args ...interface{}) {
	if logger.ftpLogLevel <= ErrorLevel {
		logger.header("ERROR")
		log.Printf(format, args...)
		if !strings.HasPrefix(format, "\n") {
			log.Println()
		}
	}
}

func (logger *ftpLogger) Debug(args ...interface{}) {
	if logger.ftpLogLevel <= DebugLevel {
		logger.header("DEBUG")
		log.Println(args...)
	}
}

func (logger *ftpLogger) Debugf(format string, args ...interface{}) {
	if logger.ftpLogLevel <= DebugLevel {
		logger.header("DEBUG")
		log.Printf(format, args...)
		if !strings.HasPrefix(format, "\n") {
			log.Println()
		}
	}
}
