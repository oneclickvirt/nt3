package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/oneclickvirt/nt3/model"
	"github.com/oneclickvirt/nt3/nt"
)

func main() {
	go func() {
		http.Get("https://hits.spiritlhl.net/nt3.svg?action=hit&title=Hits&title_bg=%23555555&count_bg=%230eecf8&edge_flat=false")
	}()
	fmt.Println("Repo:", "https://github.com/oneclickvirt/nt3")

	var showVersion, help bool
	var language, checkType, location string
	nt3Flag := flag.NewFlagSet("nt3", flag.ContinueOnError)
	nt3Flag.BoolVar(&help, "h", false, "Show help information")
	nt3Flag.BoolVar(&showVersion, "v", false, "Show version information")
	nt3Flag.StringVar(&language, "l", "zh", "Specify language parameter (en or zh)")
	nt3Flag.StringVar(&checkType, "c", "ipv4", "Specify check type (both, ipv4, or ipv6)")
	nt3Flag.StringVar(&location, "loc", "GZ", "Specify location (supports GZ, BJ, SH, CD, ALL; corresponding to Guangzhou, Beijing, Shanghai, Chengdu and All)")
	nt3Flag.BoolVar(&model.EnableLoger, "log", false, "Enable logging")
	nt3Flag.Parse(os.Args[1:])

	if help {
		fmt.Printf("Usage: %s [options]\n", os.Args[0])
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
	// 创建结果通道
	resultChan := make(chan nt.TraceResult, 100) // 使用缓冲通道
	// 启动TraceRoute goroutine
	go nt.TraceRoute(language, location, checkType, resultChan)
	// 处理结果
	for result := range resultChan {
		// 处理WSHandle初始化输出
		if result.Index == -1 {
			for index, res := range result.Output {
				res = strings.TrimSpace(res)
				if res != "" && index == 0 {
					fmt.Println(res)
				}
			}
			continue
		}
		if result.ISPName == "Error" {
			for _, res := range result.Output {
				res = strings.TrimSpace(res)
				if res != "" {
					fmt.Println(res)
				}
			}
			return
		}
		for _, res := range result.Output {
			res = strings.TrimSpace(res)
			if res == "" {
				continue
			}
			if strings.Contains(res, "ICMP") {
				fmt.Print(res)
			} else {
				fmt.Println(res)
			}
		}
	}
}
