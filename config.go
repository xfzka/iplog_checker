package main

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// initAppConfig 初始化应用配置（日志、白名单等）
func initAppConfig(config *Config) error {
	// 设置默认值并解析时间字符串
	for i := range config.RiskIPLists {
		if config.RiskIPLists[i].Format == "" {
			config.RiskIPLists[i].Format = "text"
		}
		if config.RiskIPLists[i].UpdateInterval == "" {
			config.RiskIPLists[i].UpdateInterval = "2h"
		}
		dur, err := ParseDuration(config.RiskIPLists[i].UpdateInterval)
		if err != nil {
			return fmt.Errorf("invalid update_interval for %s: %v", config.RiskIPLists[i].Name, err)
		}
		config.RiskIPLists[i].UpdateIntervalParsed = dur

		if config.RiskIPLists[i].Timeout == "" {
			config.RiskIPLists[i].Timeout = "30s"
		}
		dur, err = ParseDuration(config.RiskIPLists[i].Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout for %s: %v", config.RiskIPLists[i].Name, err)
		}
		config.RiskIPLists[i].TimeoutParsed = dur

		if config.RiskIPLists[i].RetryCount == 0 {
			config.RiskIPLists[i].RetryCount = 3
		}
	}
	for i := range config.IPLogFiles {
		if config.IPLogFiles[i].ReadMode == "" {
			config.IPLogFiles[i].ReadMode = "once"
		}
		if config.IPLogFiles[i].ReadInterval == "" {
			config.IPLogFiles[i].ReadInterval = "2h"
		}
		dur, err := ParseDuration(config.IPLogFiles[i].ReadInterval)
		if err != nil {
			return fmt.Errorf("invalid read_interval for %s: %v", config.IPLogFiles[i].Name, err)
		}
		config.IPLogFiles[i].ReadIntervalParsed = dur
	}
	for i := range config.Notifications {
		if config.Notifications[i].Threshold == 0 {
			config.Notifications[i].Threshold = 5
		}
	}

	// 初始化日志
	err := initLogger(&config.Logging)
	if err != nil {
		return fmt.Errorf("error initializing logger: %v", err)
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
