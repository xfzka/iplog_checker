package main

import (
	"fmt"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// initAppConfig 初始化应用配置（日志、白名单等）
func initAppConfig(config *Config) error {
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
