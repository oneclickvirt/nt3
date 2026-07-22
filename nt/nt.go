package nt

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	fastTrace "github.com/nxtrace/NTrace-core/fast_trace"
	"github.com/nxtrace/NTrace-core/ipgeo"
	"github.com/nxtrace/NTrace-core/trace"
	"github.com/nxtrace/NTrace-core/util"
	"github.com/nxtrace/NTrace-core/wshandle"
	. "github.com/oneclickvirt/defaultset"
	"github.com/oneclickvirt/nt3/model"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripAnsi 去除字符串中的 ANSI 颜色码
func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

// OutputBuffer 用于缓存路由追踪的输出行
type OutputBuffer struct {
	lines           []string
	lastPrintedStar bool
	mu              sync.Mutex
}

// Add 添加一行输出到缓冲区
func (ob *OutputBuffer) Add(line string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.lines = append(ob.lines, line)
}

// GetAll 获取所有缓冲的输出行，并合并连续的 * 行
func (ob *OutputBuffer) GetAll() []string {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if len(ob.lines) == 0 {
		return ob.lines
	}

	result := make([]string, 0, len(ob.lines))
	lastWasStar := false

	for _, line := range ob.lines {
		plainText := strings.TrimSpace(stripAnsi(line))
		if plainText == "*" {
			if !lastWasStar {
				result = append(result, line)
				lastWasStar = true
			}
			continue
		}
		lastWasStar = false
		result = append(result, line)
	}

	return result
}

// Clear 清空缓冲区
func (ob *OutputBuffer) Clear() {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.lines = nil
	ob.lastPrintedStar = false
}

// TraceResult 包含追踪结果和目标信息
type TraceResult struct {
	ISPName  string
	TestType string
	Output   []string
	Index    int // 用于保持原始顺序
}

// realtimePrinterWithBuffer 实时打印追踪结果到缓冲区
func realtimePrinterWithBuffer(res *trace.Result, ttl int, buffer *OutputBuffer) {
	if buffer == nil {
		return
	}
	if res == nil || ttl < 0 || ttl >= len(res.Hops) {
		buffer.mu.Lock()
		if !buffer.lastPrintedStar {
			buffer.lines = append(buffer.lines, White("*"))
			buffer.lastPrintedStar = true
		}
		buffer.mu.Unlock()
		return
	}
	hops := res.Hops[ttl]
	if len(hops) == 0 {
		buffer.mu.Lock()
		if !buffer.lastPrintedStar {
			buffer.lines = append(buffer.lines, White("*"))
			buffer.lastPrintedStar = true
		}
		buffer.mu.Unlock()
		return
	}

	var latestIP string
	tmpMap := make(map[string][]string)
	hasValidData := false

	for i, v := range hops {
		if v.RTT > 0 {
			hasValidData = true
		}

		if v.Address == nil && latestIP != "" {
			tmpMap[latestIP] = append(tmpMap[latestIP], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
			continue
		} else if v.Address == nil {
			if v.RTT > 0 {
				tmpMap["*"] = append(tmpMap["*"], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
			}
			continue
		}
		addr := v.Address.String()
		if _, exist := tmpMap[addr]; !exist {
			tmpMap[addr] = append(tmpMap[addr], strconv.Itoa(i))
			if latestIP == "" {
				for j := 0; j < i; j++ {
					tmpMap[addr] = append(tmpMap[addr], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
				}
			}
			latestIP = addr
		}
		tmpMap[addr] = append(tmpMap[addr], fmt.Sprintf("%-10s", fmt.Sprintf("%.2f ms", v.RTT.Seconds()*1000)))
	}

	if !hasValidData && latestIP == "" {
		buffer.mu.Lock()
		if !buffer.lastPrintedStar {
			buffer.lines = append(buffer.lines, White("*"))
			buffer.lastPrintedStar = true
		}
		buffer.mu.Unlock()
		time.Sleep(3 * time.Second)
		return
	}

	buffer.mu.Lock()
	buffer.lastPrintedStar = false
	buffer.mu.Unlock()

	for ip, v := range tmpMap {
		// 处理没有IP但有延迟的情况
		if ip == "*" {
			// 输出延迟但是地址为*的情况
			for idx := 0; idx < len(v); idx++ {
				line := ""
				line += fmt.Sprintf(Cyan("%-12s "), v[idx])
				line += fmt.Sprintf(White("%-10s "), "*") // AS号为*
				line += fmt.Sprintf(White("%-18s "), "*") // Whois为*
				line += White("*")                        // 地理位置为*
				buffer.Add(line)
			}
			continue
		}

		if len(v) == 0 {
			continue
		}
		i, _ := strconv.Atoi(v[0])
		rtt := "*"
		if len(v) > 1 {
			rtt = v[1]
		}
		line := ""
		line += fmt.Sprintf(Cyan("%-12s "), rtt)
		if i < 0 || i >= len(hops) {
			line += fmt.Sprintf(White("%-10s "), "*")
			line += fmt.Sprintf(White("%-18s "), "*")
			line += White("*")
			buffer.Add(line)
			continue
		}
		hop := hops[i]
		if hop.Geo.Asnumber != "" {
			line += fmt.Sprintf(Yellow("%-10s "), fmt.Sprintf("AS%s", hop.Geo.Asnumber))
		} else {
			line += fmt.Sprintf(White("%-10s "), "*")
		}

		// 处理 Whois 信息（IPv4 和 IPv6 都适用）
		whoisFormat := strings.Split(hop.Geo.Whois, "-")
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
		// 如果whoisFormat[0]为空，显示*
		displayWhois := whoisFormat[0]
		if displayWhois == "" {
			displayWhois = "*"
		}

		// 根据 AS 号或 Whois 信息决定颜色（IPv4 和 IPv6 都适用）
		switch {
		case hop.Geo.Asnumber == "58807":
			fallthrough
		case hop.Geo.Asnumber == "10099":
			fallthrough
		case hop.Geo.Asnumber == "4809":
			fallthrough
		case hop.Geo.Asnumber == "9929":
			fallthrough
		case hop.Geo.Asnumber == "23764":
			fallthrough
		case whoisFormat[0] == "[CTG-CN]":
			fallthrough
		case whoisFormat[0] == "[CNC-BACKBONE]":
			fallthrough
		case whoisFormat[0] == "[CUG-BACKBONE]":
			fallthrough
		case whoisFormat[0] == "[CMIN2-NET]":
			fallthrough
		case hop.Address != nil && strings.HasPrefix(hop.Address.String(), "59.43."):
			line += fmt.Sprintf(Yellow("%s "), fmt.Sprintf("%-18s", displayWhois))
		default:
			line += fmt.Sprintf(Green("%s "), fmt.Sprintf("%-18s", displayWhois))
		}

		// 处理地理信息（IPv4 和 IPv6 都适用）
		var parts []string
		country := hop.Geo.Country
		prov := hop.Geo.Prov
		city := hop.Geo.City
		owner := hop.Geo.Owner
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
		} else {
			// 如果没有地理信息，显示*
			line += White("*")
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
			// 不要让panic导致程序退出，仅记录日志
		}
	}()
	buffer := &OutputBuffer{}
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
		DstIP:            ip,
		DstPort:          80,
		MaxHops:          30,
		NumMeasurements:  3,
		ParallelRequests: 18,
		RDNS:             f.ParamsFastTrace.RDNS,
		AlwaysWaitRDNS:   f.ParamsFastTrace.AlwaysWaitRDNS,
		PacketInterval:   50,
		TTLInterval:      50,
		IPGeoSource:      ipgeo.GetSource("LeoMoeAPI"),
		Timeout:          time.Duration(1000) * time.Millisecond,
		SrcAddr:          f.ParamsFastTrace.SrcAddr,
		PktSize:          52,
		Lang:             f.ParamsFastTrace.Lang,
	}
	// 使用带buffer的printer
	conf.RealtimePrinter = func(res *trace.Result, ttl int) {
		realtimePrinterWithBuffer(res, ttl, buffer)
	}
	// 第一次尝试
	res, err := trace.Traceroute(f.TracerouteMethod, conf)
	if err != nil {
		errMsg := err.Error()
		// 检查是否是权限问题，权限问题不输出日志
		if strings.Contains(errMsg, "permission") || strings.Contains(errMsg, "operation not permitted") {
			buffer.Add("Error: Insufficient permissions (try with sudo)")
			return buffer.GetAll()
		}
		// 其他错误只在日志模式下显示
		if model.EnableLoger {
			InitLogger()
			Logger.Info("tracert IPv4 failed: " + errMsg)
			buffer.Add(fmt.Sprintf("Warning: %v", err))
		}
	}
	// 检查结果是否为空或hop长度为0
	if res == nil || len(res.Hops) == 0 {
		if model.EnableLoger {
			buffer.Add("No results, retrying...")
		}
		time.Sleep(3 * time.Second)
		res, err = trace.Traceroute(f.TracerouteMethod, conf)
		if err != nil {
			errMsg := err.Error()
			// 第二次尝试失败时也检查权限问题，不输出日志
			if strings.Contains(errMsg, "permission") || strings.Contains(errMsg, "operation not permitted") {
				buffer.Add("Error: Insufficient permissions (try with sudo)")
				return buffer.GetAll()
			}
			if model.EnableLoger {
				Logger.Info("tracert IPv4 second attempt failed: " + errMsg)
			}
		}
		// 如果第二次尝试后仍然没有结果，只在日志模式下显示
		if (res == nil || len(res.Hops) == 0) && model.EnableLoger {
			buffer.Add("Warning: No traceroute results")
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
			// 不要让panic导致程序退出，仅记录日志
		}
	}()
	buffer := &OutputBuffer{}
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
		DstIP:            ip,
		DstPort:          80,
		MaxHops:          30,
		NumMeasurements:  3,
		ParallelRequests: 18,
		RDNS:             f.ParamsFastTrace.RDNS,
		AlwaysWaitRDNS:   f.ParamsFastTrace.AlwaysWaitRDNS,
		PacketInterval:   50,
		TTLInterval:      50,
		IPGeoSource:      ipgeo.GetSource("LeoMoeAPI"),
		Timeout:          time.Duration(1000) * time.Millisecond,
		SrcAddr:          f.ParamsFastTrace.SrcAddr,
		PktSize:          52,
		Lang:             f.ParamsFastTrace.Lang,
	}
	// 使用带buffer的printer
	conf.RealtimePrinter = func(res *trace.Result, ttl int) {
		realtimePrinterWithBuffer(res, ttl, buffer)
	}
	// 第一次尝试
	res, err := trace.Traceroute(f.TracerouteMethod, conf)
	if err != nil {
		errMsg := err.Error()
		// 检查是否是权限问题，权限问题不输出日志
		if strings.Contains(errMsg, "permission") || strings.Contains(errMsg, "operation not permitted") {
			buffer.Add("Error: Insufficient permissions (try with sudo)")
			return buffer.GetAll()
		}
		// 其他错误只在日志模式下显示
		if model.EnableLoger {
			InitLogger()
			Logger.Info("tracert IPv6 failed: " + errMsg)
			buffer.Add(fmt.Sprintf("Warning: %v", err))
		}
	}
	// 检查结果是否为空或hop长度为0
	if res == nil || len(res.Hops) == 0 {
		if model.EnableLoger {
			buffer.Add("No results, retrying...")
		}
		time.Sleep(3 * time.Second)
		res, err = trace.Traceroute(f.TracerouteMethod, conf)
		if err != nil {
			errMsg := err.Error()
			// 第二次尝试失败时也检查权限问题，不输出日志
			if strings.Contains(errMsg, "permission") || strings.Contains(errMsg, "operation not permitted") {
				buffer.Add("Error: Insufficient permissions (try with sudo)")
				return buffer.GetAll()
			}
			if model.EnableLoger {
				Logger.Info("tracert IPv6 second attempt failed: " + errMsg)
			}
		}
		// 如果第二次尝试后仍然没有结果，只在日志模式下显示
		if (res == nil || len(res.Hops) == 0) && model.EnableLoger {
			buffer.Add("Warning: No traceroute results")
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
			// 不输出大段内容，只在日志模式下记录
			resultChan <- TraceResult{
				ISPName:  target.ISPName,
				TestType: testType,
				Output:   []string{"Error: Test failed (enable -log for details)"},
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
	if apiInfo := prepareNextTraceAPIInfo(); apiInfo != "" {
		resultChan <- TraceResult{
			ISPName: "NextTrace API",
			Output:  []string{apiInfo},
			Index:   -1,
		}
	}
	pFastTrace := fastTrace.ParamsFastTrace{
		SrcDev:         "",
		SrcAddr:        "",
		BeginHop:       1,
		MaxHops:        30,
		RDNS:           false,
		AlwaysWaitRDNS: false,
		Lang:           language,
		PktSize:        52,
	}
	ft := fastTrace.FastTracer{ParamsFastTrace: pFastTrace}
	// 创建 wsHandle（内部会启动 goroutine）
	// 注意：不设置 Interrupt 字段以避免数据竞争
	// wshandle 库内部的 goroutine 会在启动时读取 Interrupt 字段
	// 如果在 New() 之后设置会导致数据竞争
	wsHandle := wshandle.New()
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
			pos := result.Index - i
			if pos >= 0 && pos < len(batchResults) {
				batchResults[pos] = result
				continue
			}
			resultChan <- result
		}
		for _, result := range batchResults {
			resultChan <- result
		}
		if end < totalTargets {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func prepareNextTraceAPIInfo() string {
	ip := util.GetFastIPCache()
	if ip == "" {
		selected, err := util.GetFastIPWithContext(context.Background(), "api.nxtrace.org", "443", false)
		if err == nil {
			ip = selected
		}
	}
	meta := util.GetFastIPMetaCache()
	if meta.IP != "" {
		ip = meta.IP
	}
	if strings.TrimSpace(ip) == "" {
		return ""
	}
	parts := []string{"[NextTrace API] preferred API IP", strings.TrimSpace(ip)}
	if latency := strings.TrimSpace(meta.Latency); latency != "" {
		parts = append(parts, latency+"ms")
	}
	if node := strings.TrimSpace(meta.NodeName); node != "" {
		parts = append(parts, node)
	}
	return strings.Join(parts, " - ")
}
