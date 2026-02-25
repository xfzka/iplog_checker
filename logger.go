package main

import (
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// Logging 日志配置
type Logging struct {
	Level string `yaml:"level"` // 日志级别: debug, info, warn, error
	To    string `yaml:"to"`    // 日志文件路径
}

// currentLogFile 保存当前打开的日志文件句柄，用于热重载时关闭旧句柄
var currentLogFile *os.File

// initLogger 初始化日志配置
func initLogger(logging *Logging) error {
	// 设置日志级别
	level, err := logrus.ParseLevel(logging.Level)
	if err != nil {
		return fmt.Errorf("invalid log level: %v", err)
	}
	logrus.SetLevel(level)

	// 关闭旧的日志文件句柄（如果存在）
	if currentLogFile != nil {
		currentLogFile.Close()
		currentLogFile = nil
	}

	// 设置输出
	if logging.To == "" {
		logrus.SetOutput(os.Stdout)
		return nil
	}

	// 输出到文件和屏幕
	file, err := os.OpenFile(logging.To, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	currentLogFile = file
	logrus.SetOutput(io.MultiWriter(os.Stdout, file))

	return nil
}
