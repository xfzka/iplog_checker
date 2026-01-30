package main

import (
	"bufio"
	"os"
	"time"

	"github.com/hpcloud/tail"
	"github.com/sirupsen/logrus"
)

// processOnceMode 处理once模式
func processOnceMode(lf IPLogFile) {
	interval := lf.ReadIntervalParsed
	for {
		processFileOnce(lf)
		time.Sleep(interval)
	}
}

// processFileOnce 一次性处理文件
func processFileOnce(lf IPLogFile) {
	file, err := os.Open(lf.Path)
	if err != nil {
		logrus.Errorf("Failed to open file %s: %v", lf.Path, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		processLine(line, lf.Name, true)
	}
	if err := scanner.Err(); err != nil {
		logrus.Errorf("Error reading file %s: %v", lf.Path, err)
	}
	// once 模式下，读取完后检查通知
	CheckAndNotify(getThreshold(), lf.Name, true)
}

// processTailMode 处理tail模式
func processTailMode(lf IPLogFile) {
	t, err := tail.TailFile(lf.Path, tail.Config{
		ReOpen:   true,
		Follow:   true,
		Location: &tail.SeekInfo{Offset: 0, Whence: 0}, // 从头开始
	})
	if err != nil {
		logrus.Errorf("Failed to tail file %s: %v", lf.Path, err)
		return
	}
	defer t.Cleanup()

	for line := range t.Lines {
		processLine(line.Text, lf.Name, false)
		// tail 模式下，每行后检查通知
		CheckAndNotify(getThreshold(), lf.Name, false)
	}
}

// processLine 处理单行日志
func processLine(line, source string, isOnce bool) {
	ip, err := ExtractIPFromLine(line)
	if err != nil {
		// 没有IP，跳过
		return
	}
	if isSensitive, name := IsSensitiveIP(ip); isSensitive {
		logrus.Infof("Found sensitive IP %s from %s in line: %s", Uint32ToIPv4(ip).String(), name, line)
		AddNotificationItem(ip, source)
	}
}

// getThreshold 获取阈值（暂时取第一个通知的阈值）
func getThreshold() int {
	if len(config.Notifications) > 0 {
		return config.Notifications[0].Threshold
	}
	return 5 // 默认
}
