package nt

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	fastTrace "github.com/nxtrace/NTrace-core/fast_trace"
	"github.com/nxtrace/NTrace-core/ipgeo"
	"github.com/nxtrace/NTrace-core/trace"
	"github.com/nxtrace/NTrace-core/util"
	"github.com/nxtrace/NTrace-core/wshandle"
	. "github.com/oneclickvirt/defaultset"
	"github.com/oneclickvirt/nt3/model"
)

var lastPrintedStar = false

type OutputBuffer struct {
	lines []string
}

func (ob *OutputBuffer) Add(line string) {
	ob.lines = append(ob.lines, line)
}

func (ob *OutputBuffer) GetAll() []string {
	return ob.lines
}

func (ob *OutputBuffer) Clear() {
	ob.lines = nil
}

// TraceResult 包含追踪结果和目标信息
type TraceResult struct {
	ISPName  string
	TestType string
	Output   []string
	Index    int // 用于保持原始顺序
}

// realtimePrinter 现在接收 OutputBuffer 参数
func realtimePrinterWithBuffer(res *trace.Result, ttl int, buffer *OutputBuffer) {
	var latestIP string
	tmpMap := make(map[string][]string)
	for i, v := range res.Hops[ttl] {
		if v.Address == nil && latestIP != "" {
			tmpMap[latestIP] = append(tmpMap[latestIP], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
			continue
		} else if v.Address == nil {
			continue
		}
		if _, exist := tmpMap[v.Address.String()]; !exist {
			tmpMap[v.Address.String()] = append(tmpMap[v.Address.String()], strconv.Itoa(i))
			if latestIP == "" {
				for j := 0; j < i; j++ {
					tmpMap[v.Address.String()] = append(tmpMap[v.Address.String()], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
				}
			}
			latestIP = v.Address.String()
		}
		tmpMap[v.Address.String()] = append(tmpMap[v.Address.String()], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
	}
	if latestIP == "" {
		// 如果上一次没有打印*，则打印*，否则跳过
		if !lastPrintedStar {
			buffer.Add(White("*"))
			lastPrintedStar = true
		}
		time.Sleep(3 * time.Second) // Wait 3 seconds before retry
		return
	}
	// 重置星号标志，因为这次有实际内容
	lastPrintedStar = false
	for ip, v := range tmpMap {
		i, _ := strconv.Atoi(v[0])
		rtt := v[1]
		line := ""
		line += fmt.Sprintf(Cyan("%-12s "), rtt)
		if res.Hops[ttl][i].Geo.Asnumber != "" {
			line += fmt.Sprintf(Yellow("%-10s "), fmt.Sprintf("AS%s", res.Hops[ttl][i].Geo.Asnumber))
		} else {
			line += fmt.Sprintf(White("%-10s "), "*")
		}
		if net.ParseIP(ip).To4() != nil {
			whoisFormat := strings.Split(res.Hops[ttl][i].Geo.Whois, "-")
			if len(whoisFormat) > 1 {
				whoisFormat[0] = strings.Join(whoisFormat[:2], "-")
			}
			if whoisFormat[0] != "" {
				if !(strings.HasPrefix(whoisFormat[0], "RFC") ||
					strings.HasPrefix(whoisFormat[0], "DOD")) {
					whoisFormat[0] = "[" + whoisFormat[0] + "]"
				} else {
					whoisFormat[0] = ""
				}
			}
			switch {
			case res.Hops[ttl][i].Geo.Asnumber == "58807":
				fallthrough
			case res.Hops[ttl][i].Geo.Asnumber == "10099":
				fallthrough
			case res.Hops[ttl][i].Geo.Asnumber == "4809":
				fallthrough
			case res.Hops[ttl][i].Geo.Asnumber == "9929":
				fallthrough
			case res.Hops[ttl][i].Geo.Asnumber == "23764":
				fallthrough
			case whoisFormat[0] == "[CTG-CN]":
				fallthrough
			case whoisFormat[0] == "[CNC-BACKBONE]":
				fallthrough
			case whoisFormat[0] == "[CUG-BACKBONE]":
				fallthrough
			case whoisFormat[0] == "[CMIN2-NET]":
				fallthrough
			case strings.HasPrefix(res.Hops[ttl][i].Address.String(), "59.43."):
				line += fmt.Sprintf(Yellow("%s "), fmt.Sprintf("%-18s", whoisFormat[0]))
			default:
				line += fmt.Sprintf(Green("%s "), fmt.Sprintf("%-18s", whoisFormat[0]))
			}
			var parts []string
			country := res.Hops[ttl][i].Geo.Country
			prov := res.Hops[ttl][i].Geo.Prov
			city := res.Hops[ttl][i].Geo.City
			owner := res.Hops[ttl][i].Geo.Owner
			if country != "" {
				parts = append(parts, White(country))
			}
			if prov != "" {
				parts = append(parts, White(prov))
			}
			if city != "" {
				parts = append(parts, White(city))
			}
			if owner != "" {
				parts = append(parts, White(owner))
			}
			if len(parts) > 0 {
				line += strings.Join(parts, ", ")
			}
		}
		buffer.Add(line)
	}
}

// tracert
func tracert(f fastTrace.FastTracer, ispCollection fastTrace.ISPCollection) []string {
	defer func() {
		if r := recover(); r != nil {
			if model.EnableLoger {
				InitLogger()
				Logger.Error(fmt.Sprintf("tracert panic recovered: %v", r))
			}
		}
	}()
	buffer := &OutputBuffer{}
	// 重置星号标志
	lastPrintedStar = false
	buffer.Add(fmt.Sprintf("traceroute to %s, %d hops max, %d byte packets", ispCollection.IP, f.ParamsFastTrace.MaxHops, f.ParamsFastTrace.PktSize))
	ip, err := util.DomainLookUp(ispCollection.IP, "4", "", true)
	if err != nil {
		if model.EnableLoger {
			InitLogger()
			Logger.Error("domain lookup failed: " + err.Error())
		}
		buffer.Add(fmt.Sprintf("Error: domain lookup failed: %v", err))
		return buffer.GetAll()
	}
	var conf = trace.Config{
		BeginHop:         1,
		DestIP:           ip,
		DestPort:         80,
		MaxHops:          30,
		NumMeasurements:  3,
		ParallelRequests: 18,
		RDns:             f.ParamsFastTrace.RDns,
		AlwaysWaitRDNS:   f.ParamsFastTrace.AlwaysWaitRDNS,
		PacketInterval:   50,
		TTLInterval:      50,
		IPGeoSource:      ipgeo.GetSource("LeoMoeAPI"),
		Timeout:          time.Duration(1000) * time.Millisecond,
		SrcAddr:          f.ParamsFastTrace.SrcAddr,
		PktSize:          52,
		Lang:             f.ParamsFastTrace.Lang,
		DontFragment:     f.ParamsFastTrace.DontFragment,
	}
	// 使用带buffer的printer
	conf.RealtimePrinter = func(res *trace.Result, ttl int) {
		realtimePrinterWithBuffer(res, ttl, buffer)
	}
	// 第一次尝试
	res, err := trace.Traceroute(f.TracerouteMethod, conf)
	if err != nil && model.EnableLoger {
		InitLogger()
		Logger.Info("trace failed: " + err.Error())
	}
	// 检查结果是否为空或hop长度为0
	if res == nil || len(res.Hops) == 0 {
		buffer.Add("\nNo results received, retrying after 3 seconds...")
		time.Sleep(3 * time.Second)
		_, err = trace.Traceroute(f.TracerouteMethod, conf)
		if err != nil && model.EnableLoger {
			Logger.Info("second trace attempt failed: " + err.Error())
		}
	}
	return buffer.GetAll()
}

// tracert_v6
func tracert_v6(f fastTrace.FastTracer, ispCollection fastTrace.ISPCollection) []string {
	defer func() {
		if r := recover(); r != nil {
			if model.EnableLoger {
				InitLogger()
				Logger.Error(fmt.Sprintf("tracert_v6 panic recovered: %v", r))
			}
		}
	}()
	buffer := &OutputBuffer{}
	// 重置星号标志
	lastPrintedStar = false
	buffer.Add(fmt.Sprintf("traceroute to %s, %d hops max, %d byte packets", ispCollection.IPv6, f.ParamsFastTrace.MaxHops, f.ParamsFastTrace.PktSize))
	ip, err := util.DomainLookUp(ispCollection.IPv6, "6", "", true)
	if err != nil {
		if model.EnableLoger {
			InitLogger()
			Logger.Error("domain lookup failed: " + err.Error())
		}
		buffer.Add(fmt.Sprintf("Error: domain lookup failed: %v", err))
		return buffer.GetAll()
	}
	var conf = trace.Config{
		BeginHop:         1,
		DestIP:           ip,
		DestPort:         80,
		MaxHops:          30,
		NumMeasurements:  3,
		ParallelRequests: 18,
		RDns:             f.ParamsFastTrace.RDns,
		AlwaysWaitRDNS:   f.ParamsFastTrace.AlwaysWaitRDNS,
		PacketInterval:   50,
		TTLInterval:      50,
		IPGeoSource:      ipgeo.GetSource("LeoMoeAPI"),
		Timeout:          time.Duration(1000) * time.Millisecond,
		SrcAddr:          f.ParamsFastTrace.SrcAddr,
		PktSize:          52,
		Lang:             f.ParamsFastTrace.Lang,
		DontFragment:     f.ParamsFastTrace.DontFragment,
	}
	// 使用带buffer的printer
	conf.RealtimePrinter = func(res *trace.Result, ttl int) {
		realtimePrinterWithBuffer(res, ttl, buffer)
	}
	// 第一次尝试
	res, err := trace.Traceroute(f.TracerouteMethod, conf)
	if err != nil && model.EnableLoger {
		InitLogger()
		Logger.Info("trace failed: " + err.Error())
	}
	// 检查结果是否为空或hop长度为0
	if res == nil || len(res.Hops) == 0 {
		buffer.Add("\nNo results received, retrying after 3 seconds...")
		time.Sleep(3 * time.Second)
		_, err = trace.Traceroute(f.TracerouteMethod, conf)
		if err != nil && model.EnableLoger {
			Logger.Info("second trace attempt failed: " + err.Error())
		}
	}
	return buffer.GetAll()
}

// processTarget 处理单个目标的追踪
func processTarget(ft fastTrace.FastTracer, target fastTrace.ISPCollection, testType string, index int, resultChan chan<- TraceResult) {
	defer func() {
		if r := recover(); r != nil {
			if model.EnableLoger {
				InitLogger()
				Logger.Error(fmt.Sprintf("processTarget panic recovered: %v", r))
			}
			resultChan <- TraceResult{
				ISPName:  target.ISPName,
				TestType: testType,
				Output:   []string{fmt.Sprintf("Error: trace for %s panic recovered: %v", target.ISPName, r)},
				Index:    index,
			}
		}
	}()
	var allOutput []string
	switch testType {
	case "both":
		// IPv4
		allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v4", target.ISPName)))
		output := tracert(ft, target)
		allOutput = append(allOutput, output...)
		// IPv6
		allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v6", target.ISPName)))
		output = tracert_v6(ft, target)
		allOutput = append(allOutput, output...)
	case "ipv4":
		allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v4", target.ISPName)))
		output := tracert(ft, target)
		allOutput = append(allOutput, output...)
	case "ipv6":
		allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v6", target.ISPName)))
		output := tracert_v6(ft, target)
		allOutput = append(allOutput, output...)
	}
	resultChan <- TraceResult{
		ISPName:  target.ISPName,
		TestType: testType,
		Output:   allOutput,
		Index:    index,
	}
}

// TraceRoute 通过通道返回结果，支持并发处理
func TraceRoute(language, location, testType string, resultChan chan<- TraceResult) {
	defer func() {
		if r := recover(); r != nil {
			if model.EnableLoger {
				InitLogger()
				Logger.Error(fmt.Sprintf("TraceRoute panic recovered: %v", r))
			}
		}
		close(resultChan)
	}()
	
	if language == "zh" || language == "" {
		language = "cn"
	} else if language != "en" {
		resultChan <- TraceResult{
			ISPName:  "Error",
			TestType: testType,
			Output:   []string{"Invalid language."},
			Index:    0,
		}
		return
	}
	
	var TL []fastTrace.ISPCollection
	switch location {
	case "GZ":
		TL = []fastTrace.ISPCollection{model.GuangZhouCT, model.GuangZhouCU, model.GuangZhouCMCC}
	case "BJ":
		TL = []fastTrace.ISPCollection{model.BeiJingCT, model.BeiJingCU, model.BeiJingCMCC}
	case "SH":
		TL = []fastTrace.ISPCollection{model.ShangHaiCT, model.ShangHaiCU, model.ShangHaiCMCC}
	case "CD":
		TL = []fastTrace.ISPCollection{model.ChengDuCT, model.ChengDuCU, model.ChengDuCMCC}
	case "ALL":
		TL = []fastTrace.ISPCollection{model.BeiJingCT, model.BeiJingCU, model.BeiJingCMCC,
			model.ShangHaiCT, model.ShangHaiCU, model.ShangHaiCMCC,
			model.GuangZhouCT, model.GuangZhouCU, model.GuangZhouCMCC,
			model.ChengDuCT, model.ChengDuCU, model.ChengDuCMCC}
	default:
		resultChan <- TraceResult{
			ISPName:  "Error",
			TestType: testType,
			Output:   []string{"Invalid location."},
			Index:    0,
		}
		return
	}
	
	pFastTrace := fastTrace.ParamsFastTrace{
		SrcDev:         "",
		SrcAddr:        "",
		BeginHop:       1,
		MaxHops:        30,
		RDns:           false,
		AlwaysWaitRDNS: false,
		Lang:           language,
		PktSize:        52,
	}
	ft := fastTrace.FastTracer{ParamsFastTrace: pFastTrace}
	// 检测是否已经被外部重定向
	isRedirected := isOutputRedirected()
	var wsOutputLines []string
	if isRedirected {
		// 如果已被重定向，直接创建 wshandle，不再重定向
		wsHandle := wshandle.New()
		// 由于输出已被外部捕获，这里不需要特殊处理
	} else {
		// 原有的重定向逻辑
		oldColorOutput := color.Output
		var buf bytes.Buffer
		color.Output = &buf
		// 建立 WebSocket 连接
		wsHandle := wshandle.New()
		// 恢复 color.Output
		color.Output = oldColorOutput
		// 获取截留的输出
		wsOutput := buf.String()
		if wsOutput != "" {
			// 将输出按行分割
			lines := strings.Split(strings.TrimRight(wsOutput, "\n"), "\n")
			for _, line := range lines {
				if line != "" {
					wsOutputLines = append(wsOutputLines, line)
				}
			}
		}
	}
	// 只有在非重定向状态下且有输出时才发送 wshandle 输出
	if len(wsOutputLines) > 0 {
		resultChan <- TraceResult{
			ISPName:  "WSHandle",
			TestType: "info",
			Output:   wsOutputLines,
			Index:    -1, // 特殊索引表示这是初始化输出
		}
	}
	wsHandle.Interrupt = make(chan os.Signal, 1)
	signal.Notify(wsHandle.Interrupt, os.Interrupt)
	defer func() {
		if wsHandle.Conn != nil {
			wsHandle.Conn.Close()
		}
	}()
	ft.TracerouteMethod = trace.ICMPTrace
	const maxConcurrent = 3
	totalTargets := len(TL)
	for i := 0; i < totalTargets; i += maxConcurrent {
		end := i + maxConcurrent
		if end > totalTargets {
			end = totalTargets
		}
		var wg sync.WaitGroup
		batchResultChan := make(chan TraceResult, end-i)
		for j := i; j < end; j++ {
			wg.Add(1)
			go func(index int, target fastTrace.ISPCollection) {
				defer wg.Done()
				processTarget(ft, target, testType, index, batchResultChan)
			}(j, TL[j])
		}
		go func() {
			wg.Wait()
			close(batchResultChan)
		}()
		batchResults := make([]TraceResult, end-i)
		for result := range batchResultChan {
			batchResults[result.Index-i] = result
		}
		for _, result := range batchResults {
			resultChan <- result
		}
		if end < totalTargets {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// 检测输出是否已被重定向的辅助函数
func isOutputRedirected() bool {
	// 方法1：检查 os.Stdout 是否指向管道或文件
	if stat, err := os.Stdout.Stat(); err == nil {
		// 如果不是字符设备（终端），说明被重定向了
		mode := stat.Mode()
		if (mode & os.ModeCharDevice) == 0 {
			return true
		}
		// 检查是否是管道
		if (mode & os.ModeNamedPipe) != 0 {
			return true
		}
	}
	// 方法2：检查 color.Output 是否不等于 os.Stdout
	return color.Output != os.Stdout
}
