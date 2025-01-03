package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oneclickvirt/nt3/model"
	"github.com/oneclickvirt/nt3/nt"
)

func main() {
	go func() {
		http.Get("https://hits.seeyoufarm.com/api/count/incr/badge.svg?url=https%3A%2F%2Fgithub.com%2Foneclickvirt%2Fnt3&count_bg=%2379C83D&title_bg=%23555555&icon=&icon_color=%23E7E7E7&title=hits&edge_flat=false")
	}()
	fmt.Println("项目地址:", "https://github.com/oneclickvirt/nt3")
	var showVersion, help bool
	var language, checkType, location string
	var maxRetries int
	var retryDelay time.Duration
	nt3Flag := flag.NewFlagSet("nt3", flag.ContinueOnError)
	nt3Flag.BoolVar(&help, "h", false, "显示帮助信息")
	nt3Flag.BoolVar(&showVersion, "v", false, "显示版本信息")
	nt3Flag.StringVar(&language, "l", "zh", "指定语言参数 (en 或 zh)")
	nt3Flag.StringVar(&checkType, "c", "ipv4", "指定检查类型 (both, ipv4, 或 ipv6)")
	nt3Flag.StringVar(&location, "loc", "GZ", "指定位置 (支持 GZ, BJ, SH, CD; 对应广州、北京、上海、成都)")
	nt3Flag.BoolVar(&model.EnableLoger, "log", false, "启用日志记录")
	nt3Flag.IntVar(&maxRetries, "retries", 1, "失败重试次数")
	nt3Flag.DurationVar(&retryDelay, "retry-delay", 5*time.Minute, "重试间隔时间")
	nt3Flag.Parse(os.Args[1:])
	if help {
		fmt.Printf("用法: %s [选项]\n", os.Args[0])
		nt3Flag.PrintDefaults()
		return
	}
	if showVersion {
		fmt.Println(model.NextTraceVersion)
		return
	}
	if language == "" {
		language = "zh"
	} else {
		language = strings.ToLower(language)
	}
	if checkType == "" || checkType == "ipv4" {
		checkType = "ipv4"
	} else if strings.ToLower(checkType) == "both" {
		checkType = "both"
	} else if strings.ToLower(checkType) == "ipv6" {
		checkType = "ipv6"
	}
	// 创建重试配置
	retryConfig := nt.RetryConfig{
		MaxRetries: maxRetries,
		RetryDelay: retryDelay,
	}
	nt.TraceRoute(language, location, checkType, retryConfig)
}
