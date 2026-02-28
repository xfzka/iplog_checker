package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hpcloud/tail"
	"github.com/sirupsen/logrus"
)

// ipv4Regex 用于匹配IPv4地址的正则表达式
var ipv4Regex = regexp.MustCompile(`\b((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}\b`)

// ExtractIPFromLine 从日志行中提取IP地址
func ExtractIPFromLine(line string) (uint32, error) {
	// 正则表达式匹配IPv4地址
	ip := ipv4Regex.FindString(line)
	if ip == "" {
		return 0, fmt.Errorf("no valid IPv4 address found")
	}
	ip32, _ := IPv4ToUint32(ip)
	return ip32, nil
}

// processOnceMode 处理once模式
func processOnceMode(ctx context.Context, lf TargetLog) {
	interval := lf.ReadIntervalParsed
	for {
		processFileOnce(lf)
		// Debug: 输出下一次读取间隔
		logrus.Debugf("Next read for %s after %s", lf.Name, interval.String())
		select {
		case <-ctx.Done():
			logrus.Infof("Stopping once-mode processor for %s", lf.Name)
			return
		case <-time.After(interval):
		}
	}
}

// processFileOnce 一次性处理文件
func processFileOnce(lf TargetLog) {
	var info = NewNetListInfo(lf.Name, lf.Level)
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
		processLine(line, info, lf.IgnoreKeys)
	}
	if err := scanner.Err(); err != nil {
		logrus.Errorf("Error reading file %s: %v", lf.Path, err)
	}
	// once 模式下，读取完后检查通知
	CheckAndNotify(info, true)

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
func processTailMode(ctx context.Context, lf TargetLog) {
	for {
		// 每次循环重新创建info，避免重复计数
		var info = NewNetListInfo(lf.Name, lf.Level)

		// 等待文件存在
		for {
			if _, err := os.Stat(lf.Path); err == nil {
				break
			}
			logrus.Warnf("File %s does not exist, retrying in 1 second...", lf.Path)
			select {
			case <-ctx.Done():
				logrus.Infof("Stopping tail-mode processor for %s", lf.Name)
				return
			case <-time.After(1 * time.Second):
			}
		}

		t, err := tail.TailFile(lf.Path, tail.Config{
			ReOpen:   true,
			Follow:   true,
			Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}, // 从尾部开始, 从头开始 io.SeekStart
		})
		if err != nil {
			logrus.Errorf("Failed to tail file %s: %v, retrying in 1 second...", lf.Path, err)
			select {
			case <-ctx.Done():
				logrus.Infof("Stopping tail-mode processor for %s", lf.Name)
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}

		// 当 context 取消时，停止 tailer
		go func() {
			<-ctx.Done()
			t.Stop()
		}()

		for line := range t.Lines {
			if line.Err != nil {
				logrus.Errorf("Error reading line from %s: %v", lf.Path, line.Err)
				continue
			}
			logrus.Debugf("Read line from %s, level: %d, line: %s", lf.Name, info.Level, line.Text)
			processLine(line.Text, info, lf.IgnoreKeys)
			// tail 模式下，每行后检查通知
			CheckAndNotify(info, false)
		}

		// 如果 tail 退出（例如文件被删除），清理并重新开始循环
		t.Cleanup()

		// 检查是否是 context 取消导致的退出
		select {
		case <-ctx.Done():
			logrus.Infof("Stopping tail-mode processor for %s", lf.Name)
			return
		default:
		}

		logrus.Warnf("Tail for %s ended, will retry with fresh state...", lf.Path)
		// info 会在下次循环开始时重新创建，避免累积旧数据
	}
}

// processLine 处理单行日志
func processLine(line string, finfo ListInfo, ignoreKeys []string) {
	for _, key := range ignoreKeys {
		if strings.Contains(line, key) {
			logrus.Debugf("Line contains ignore key %q, skipping: %s", key, line)
			return
		}
	}

	ip, err := ExtractIPFromLine(line)
	if err != nil {
		// 没有找到有效IP，记录调试信息后跳过
		logrus.Debugf("No valid IP in line from %s: %v", finfo.Name, err)
		return
	}
	if isSensitive, linfo := IsSensitiveIP(ip); isSensitive {
		logrus.Warnf("Found sensitive IP %s from %s, level: %d in line: %s", Uint32ToIPv4(ip).String(), linfo.Name, linfo.Level, line)
		AddNotificationItem(ip, finfo, linfo)
	}
}
