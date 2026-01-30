package main

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// initAppConfig 初始化应用配置（日志、IP列表等）
func initAppConfig(config *Config) error {
	// 设置 safe_list 默认值并解析时间字符串
	for i := range config.SafeList {
		if err := initIPListConfig(&config.SafeList[i]); err != nil {
			return fmt.Errorf("invalid safe_list config for %s: %v", config.SafeList[i].Name, err)
		}
	}

	// 设置 risk_list 默认值并解析时间字符串
	for i := range config.RiskList {
		if err := initIPListConfig(&config.RiskList[i]); err != nil {
			return fmt.Errorf("invalid risk_list config for %s: %v", config.RiskList[i].Name, err)
		}
	}

	for i := range config.TargetLogs {
		if config.TargetLogs[i].ReadMode == "" {
			config.TargetLogs[i].ReadMode = "once"
		}
		if config.TargetLogs[i].ReadInterval == "" {
			config.TargetLogs[i].ReadInterval = "2h"
		}
		dur, err := ParseDuration(config.TargetLogs[i].ReadInterval)
		if err != nil {
			return fmt.Errorf("invalid read_interval for %s: %v", config.TargetLogs[i].Name, err)
		}
		config.TargetLogs[i].ReadIntervalParsed = dur
	}
	for i := range config.Notifications.Services {
		if config.Notifications.Services[i].Threshold == 0 {
			config.Notifications.Services[i].Threshold = 5
		}
	}

	// 初始化日志
	err := initLogger(&config.Logging)
	if err != nil {
		return fmt.Errorf("error initializing logger: %v", err)
	}
	return nil
}

// initIPListConfig 初始化 IPList 配置项
func initIPListConfig(list *IPList) error {
	// 验证来源：file, url, ips 三选一且必选一
	sourceCount := 0
	if list.File != "" {
		sourceCount++
	}
	if list.URL != "" {
		sourceCount++
	}
	if len(list.IPs) > 0 {
		sourceCount++
	}

	if sourceCount == 0 {
		return fmt.Errorf("must specify one of: file, url, or ips")
	}
	if sourceCount > 1 {
		return fmt.Errorf("can only specify one of: file, url, or ips (not multiple)")
	}

	// 仅对 file/url 来源设置默认值
	if list.File != "" || list.URL != "" {
		if list.Format == "" {
			list.Format = "text"
		}
		if list.UpdateInterval == "" {
			list.UpdateInterval = "2h"
		}
		dur, err := ParseDuration(list.UpdateInterval)
		if err != nil {
			return fmt.Errorf("invalid update_interval: %v", err)
		}
		list.UpdateIntervalParsed = dur
	}

	// 仅对 url 来源设置超时和重试
	if list.URL != "" {
		if list.Timeout == "" {
			list.Timeout = "30s"
		}
		dur, err := ParseDuration(list.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %v", err)
		}
		list.TimeoutParsed = dur

		if list.RetryCount == 0 {
			list.RetryCount = 3
		}
	}

	return nil
}

// watchConfigFile 监控配置文件变更并自动重载
func watchConfigFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("Failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(ConfigFilePath)
	if err != nil {
		logrus.Errorf("Failed to watch config file: %v", err)
		return
	}

	logrus.Info("Started watching config file for changes")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				logrus.Info("Config file changed, reloading...")
				err := initAPP()
				if err != nil {
					logrus.Errorf("Failed to reload config: %v", err)
				} else {
					logrus.Info("Config reloaded successfully")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logrus.Errorf("File watcher error: %v", err)
		}
	}
}
