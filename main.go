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

	// 初始化风险IP数据
	riskIPData := NewRiskIPData()

	// 启动下载goroutines
	DownloadRiskIPs(&config, riskIPData)

	// 等待初始下载完成 (简单等待10秒)
	time.Sleep(10 * time.Second)

	// 输出总行数
	totalLines := riskIPData.GetTotalLines()
	logrus.Infof("Total risk IPs loaded: %d", totalLines)
}
