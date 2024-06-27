package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/oneclickvirt/nt3/model"
	"github.com/oneclickvirt/nt3/nt"
)

func main() {
	go func() {
		http.Get("https://hits.seeyoufarm.com/api/count/incr/badge.svg?url=https%3A%2F%2Fgithub.com%2Foneclickvirt%2Fnt3&count_bg=%2379C83D&title_bg=%23555555&icon=&icon_color=%23E7E7E7&title=hits&edge_flat=false")
	}()
	fmt.Println("项目地址:", "https://github.com/oneclickvirt/nt3")
	var showVersion bool
	var language, checkType, location string
	flag.BoolVar(&showVersion, "v", false, "Show version information")
	flag.StringVar(&language, "l", "", "Specify language parameter (en or zh, default is zh)")
	flag.StringVar(&checkType, "c", "", "Specify check type (both, ipv4, or ipv6, default is ipv4)")
	flag.StringVar(&location, "loc", "", "Specify location (supports GZ, BJ, SH, CD; corresponding to Guangzhou, Beijing, Shanghai, Chengdu)")
	// flag.BoolVar(&model.EnableLoger, "log", false, "Enable logging")
	flag.Parse()
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
	nt.TraceRoute(language, location, checkType)
}
