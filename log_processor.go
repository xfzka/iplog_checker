package main

import (
	"bufio"
	"os"
	"time"

	"github.com/hpcloud/tail"
	"github.com/sirupsen/logrus"
)

// processOnceMode 处理once模式
func processOnceMode(lf TargetLog) {
	interval := lf.ReadIntervalParsed
	for {
		processFileOnce(lf)
		time.Sleep(interval)
	}
}

// processFileOnce 一次性处理文件
func processFileOnce(lf TargetLog) {
	// 检查文件是否存在，不存在则跳过本次读取
	if _, err := os.Stat(lf.Path); os.IsNotExist(err) {
		logrus.Warnf("File %s does not exist, skipping this read cycle", lf.Path)
		return
	}

	file, err := os.Open(lf.Path)
	if err != nil {
		logrus.Errorf("Failed to open file %s: %v", lf.Path, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		processLine(line, lf.Name)
	}
	if err := scanner.Err(); err != nil {
		logrus.Errorf("Error reading file %s: %v", lf.Path, err)
	}
	// once 模式下，读取完后检查通知
	CheckAndNotify(getThreshold(), lf.Name, true)

	// 如果配置了 clean_after_read，则清空文件
	if lf.CleanAfterRead {
		if err := os.Truncate(lf.Path, 0); err != nil {
			logrus.Errorf("Failed to truncate file %s: %v", lf.Path, err)
		} else {
			logrus.Infof("File %s truncated after read", lf.Path)
		}
	}
}

// processTailMode 处理tail模式
func processTailMode(lf TargetLog) {
	for {
		// 等待文件存在
		for {
			if _, err := os.Stat(lf.Path); err == nil {
				break
			}
			logrus.Warnf("File %s does not exist, retrying in 1 second...", lf.Path)
			time.Sleep(1 * time.Second)
		}

		t, err := tail.TailFile(lf.Path, tail.Config{
			ReOpen:   true,
			Follow:   true,
			Location: &tail.SeekInfo{Offset: 0, Whence: 0}, // 从头开始
		})
		if err != nil {
			logrus.Errorf("Failed to tail file %s: %v, retrying in 1 second...", lf.Path, err)
			time.Sleep(1 * time.Second)
			continue
		}

		for line := range t.Lines {
			if line.Err != nil {
				logrus.Errorf("Error reading line from %s: %v", lf.Path, line.Err)
				continue
			}
			processLine(line.Text, lf.Name)
			// tail 模式下，每行后检查通知
			CheckAndNotify(getThreshold(), lf.Name, false)
		}

		// 如果 tail 退出（例如文件被删除），重新开始循环
		t.Cleanup()
		logrus.Warnf("Tail for %s ended, will retry...", lf.Path)
	}
}

// processLine 处理单行日志
func processLine(line, source string) {
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
	if len(config.Notifications.Services) > 0 {
		return config.Notifications.Services[0].Threshold
	}
	return 5 // 默认
}
