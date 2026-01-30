package main

import (
	"fmt"
	"os"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
)

// initLogger 初始化日志配置
func initLogger(logging *Logging) error {
	// 设置日志级别
	level, err := logrus.ParseLevel(logging.Level)
	if err != nil {
		return fmt.Errorf("invalid log level: %v", err)
	}
	logrus.SetLevel(level)

	// 如果没有指定输出文件，使用 stdout
	if logging.To == "" {
		logrus.SetOutput(os.Stdout)
		return nil
	}

	// 如果没有轮转配置，直接输出到文件
	if logging.MaxSize == "" && logging.MaxAge == "" {
		file, err := os.OpenFile(logging.To, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to open log file: %v", err)
		}
		logrus.SetOutput(file)
		return nil
	}

	// 解析 max_size 和 max_age
	var maxSize int64
	if logging.MaxSize != "" {
		maxSize, err = parseSize(logging.MaxSize)
		if err != nil {
			return fmt.Errorf("invalid max_size: %v", err)
		}
	}

	var maxAge time.Duration
	if logging.MaxAge != "" {
		maxAge, err = parseDuration(logging.MaxAge)
		if err != nil {
			return fmt.Errorf("invalid max_age: %v", err)
		}
	}

	// 使用 rotatelogs 实现轮转 (由于 logrus_mate 有兼容性问题，使用 rotatelogs)
	writer, err := rotatelogs.New(
		logging.To+".%Y%m%d%H%M%S",
		rotatelogs.WithMaxAge(maxAge),
		rotatelogs.WithRotationSize(maxSize),
	)
	if err != nil {
		return fmt.Errorf("failed to create rotatelogs: %v", err)
	}
	logrus.SetOutput(writer)

	return nil
}
