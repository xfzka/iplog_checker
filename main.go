package main

import (
	"flag"
	"fmt"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Version will be set at build time via -ldflags "-X main.Version=..."
// If not set during build, it defaults to "dev".
var Version string = "dev"

func initAPP() error {
	data, err := os.ReadFile(ConfigFilePath)
	if err != nil {
		return fmt.Errorf("Error reading config file: %v\n", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("Error parsing YAML: %v\n", err)
	}

	// 初始化应用配置
	err = initAppConfig(&config)
	if err != nil {
		return fmt.Errorf("Error initializing app config: %v\n", err)
	}

	// 初始化安全IP数据 (白名单)
	SafeListData = NewListGroup()
	// 初始化风险IP数据
	RiskListData = NewListGroup()

	// 启动加载goroutines，使用WaitGroup等待初始加载完成
	var wg sync.WaitGroup
	configMutex.RLock()
	LoadIPList(config.SafeList, SafeListData, "safe_list", &wg)
	LoadIPList(config.RiskList, RiskListData, "risk_list", &wg)
	configMutex.RUnlock()

	// 等待初始加载完成
	logrus.Info("Waiting for IP lists to load...")
	wg.Wait()
	logrus.Info("IP lists loaded successfully")

	// 启动通知工作器 (独立 goroutine, 每 1s 检查一次，不阻塞)
	StartNotificationWorker()

	// 启动目标日志文件处理goroutines
	StartTargetLogProcessors(&config)

	return nil
}

// StartTargetLogProcessors 启动目标日志文件处理器
func StartTargetLogProcessors(config *Config) {
	configMutex.RLock()
	targetLogs := make([]TargetLog, len(config.TargetLogs))
	copy(targetLogs, config.TargetLogs)
	configMutex.RUnlock()

	for _, logFile := range targetLogs {
		go func(lf TargetLog) {
			if lf.ReadMode == "once" {
				processOnceMode(lf)
			} else if lf.ReadMode == "tail" {
				processTailMode(lf)
			} else {
				logrus.Errorf("Unknown read_mode for %s: %s", lf.Name, lf.ReadMode)
			}
		}(logFile)
	}
}

func main() {
	flag.StringVar(&ConfigFilePath, "config", ConfigFilePath, "path to config file")
	flag.StringVar(&ConfigFilePath, "c", ConfigFilePath, "path to config file")

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit")

	flag.Parse()

	if showVersion {
		fmt.Println(Version)
		return
	}

	err := initAPP()
	if err != nil {
		fmt.Printf("Failed to initialize app: %v\n", err)
		return
	}

	// 启动配置文件监控
	go watchConfigFile()

	// 主程序逻辑在此处继续...
	select {}
}
