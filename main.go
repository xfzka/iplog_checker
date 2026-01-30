package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

func main() {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		return
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		return
	}

	// 初始化日志
	err = initLogger(&config.Logging)
	if err != nil {
		fmt.Printf("Error initializing logger: %v\n", err)
		return
	}

	logrus.Info("Configuration loaded successfully")
	logrus.Debugf("Config: %+v", config)

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

	// 如果启用了自动重载配置，则启动文件监控
	if config.AutoReloadConfig {
		go watchConfigFile("config.yaml", &config)
	}

	// 启动日志监控或其他逻辑
	// ... (假设有其他逻辑)
}
