package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var config Config

func initAPP() error {
	data, err := os.ReadFile("config.yaml")
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

	// 设置白名单
	Whitelist = config.WhitelistIPs
	InitWhitelist(config.WhitelistIPs)

	// 初始化风险IP数据
	riskIPData := NewRiskIPData()

	// 设置全局实例
	RiskIPDataInstance = riskIPData

	// 启动下载goroutines
	DownloadRiskIPs(&config, riskIPData)

	// 等待初始下载完成 (简单等待10秒)
	time.Sleep(10 * time.Second)

	// 输出总行数
	totalLines := riskIPData.GetTotalLines()
	logrus.Infof("Total risk IPs loaded: %d", totalLines)

	return nil
}

func main() {
	err := initAPP()
	if err != nil {
		fmt.Printf("Failed to initialize app: %v\n", err)
		return
	}

	// 如果启用了自动重载配置，则启动文件监控
	if config.AutoReloadConfig {
		go watchConfigFile("config.yaml")
	}

	// 主程序逻辑在此处继续...
	select {}
}
