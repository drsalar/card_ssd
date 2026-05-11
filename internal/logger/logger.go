// Package logger 简单日志封装
// 输出格式 [时间][级别] msg
package logger

import (
	"log"
	"os"
)

var (
	infoLog  = log.New(os.Stdout, "[INFO] ", log.LstdFlags|log.Lmicroseconds)
	warnLog  = log.New(os.Stdout, "[WARN] ", log.LstdFlags|log.Lmicroseconds)
	errorLog = log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lmicroseconds)
)

// Info 输出信息级别日志
func Info(format string, a ...any) {
	infoLog.Printf(format, a...)
}

// Warn 输出警告级别日志
func Warn(format string, a ...any) {
	warnLog.Printf(format, a...)
}

// Error 输出错误级别日志
func Error(format string, a ...any) {
	errorLog.Printf(format, a...)
}
