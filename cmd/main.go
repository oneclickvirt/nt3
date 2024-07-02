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
		http.Get("https://hits.seeyoufarm.com/api/count/incr/badge.svg?url=https%3A%2F%2Fgithub.com%2Foneclickvirt%2Fnt3&count_bg=%2379C83D&title_bg=%23555555&icon=&icon_color=%23E7E7E7&title=hits&edge_flat=false")
	}()
	fmt.Println("项目地址:", "https://github.com/oneclickvirt/nt3")
	var showVersion, help bool
	var language, checkType, location string
	nt3Flag := flag.NewFlagSet("nt3", flag.ContinueOnError)
	nt3Flag.BoolVar(&help, "h", false, "Show help information")
	nt3Flag.BoolVar(&showVersion, "v", false, "Show version information")
	nt3Flag.StringVar(&language, "l", "zh", "Specify language parameter (en or zh)")
	nt3Flag.StringVar(&checkType, "c", "ipv4", "Specify check type (both, ipv4, or ipv6)")
	nt3Flag.StringVar(&location, "loc", "GZ", "Specify location (supports GZ, BJ, SH, CD; corresponding to Guangzhou, Beijing, Shanghai, Chengdu)")
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
	nt.TraceRoute(language, location, checkType)
}
