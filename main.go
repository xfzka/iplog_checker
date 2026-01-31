package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

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

	// 启动加载goroutines
	LoadIPList(config.SafeList, SafeListData, "safe_list")
	LoadIPList(config.RiskList, RiskListData, "risk_list")

	// 等待初始加载完成 (简单等待10秒)
	time.Sleep(10 * time.Second)

	// 启动目标日志文件处理goroutines
	StartTargetLogProcessors(&config)

	return nil
}

// StartTargetLogProcessors 启动目标日志文件处理器
func StartTargetLogProcessors(config *Config) {
	for _, logFile := range config.TargetLogs {
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
