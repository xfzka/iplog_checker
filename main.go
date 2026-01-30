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
	SafeListData = NewIPData()
	// 初始化风险IP数据
	RiskListData = NewIPData()

	// 启动加载goroutines
	LoadIPList(config.SafeList, SafeListData, "safe_list")
	LoadIPList(config.RiskList, RiskListData, "risk_list")

	// 等待初始加载完成 (简单等待10秒)
	time.Sleep(10 * time.Second)

	// 输出总数
	safeCount := SafeListData.GetTotalCount()
	riskCount := RiskListData.GetTotalCount()
	logrus.Infof("Total safe IPs/CIDRs loaded: %d", safeCount)
	logrus.Infof("Total risk IPs/CIDRs loaded: %d", riskCount)

	// 启动IP日志文件处理goroutines
	StartIPLogFileProcessors(&config)

	return nil
}

// StartIPLogFileProcessors 启动IP日志文件处理器
func StartIPLogFileProcessors(config *Config) {
	for _, logFile := range config.IPLogFiles {
		go func(lf IPLogFile) {
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
	flag.Parse()

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
