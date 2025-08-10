package nt

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

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

// tracert 现在返回输出结果
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

// tracert_v6 现在返回输出结果
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

// TraceRoute 现在可以收集所有输出并统一处理
func TraceRoute(language, location, testType string) []string {
	defer func() {
		if r := recover(); r != nil {
			if model.EnableLoger {
				InitLogger()
				Logger.Error(fmt.Sprintf("TraceRoute panic recovered: %v", r))
			}
		}
	}()
	var allOutput []string
	if language == "zh" || language == "" {
		language = "cn"
	} else if language != "en" {
		allOutput = append(allOutput, "Invalid language.")
		return allOutput
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
		allOutput = append(allOutput, "Invalid location.")
		return allOutput
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
	// 截留 wshandle.New() 的输出
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	var wsOutput []string
	done := make(chan bool)
	// 在goroutine中读取输出
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()
		if output != "" {
			// 将输出按行分割
			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			for _, line := range lines {
				if line != "" {
					wsOutput = append(wsOutput, line)
				}
			}
		}
		done <- true
	}()
	// 建立 WebSocket 连接
	wsHandle := wshandle.New()
	// 恢复标准输出
	w.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	<-done
	r.Close()
	// 将wshandle的输出添加到头部
	allOutput = append(wsOutput, allOutput...)
	wsHandle.Interrupt = make(chan os.Signal, 1)
	signal.Notify(wsHandle.Interrupt, os.Interrupt)
	defer func() {
		if wsHandle.Conn != nil {
			wsHandle.Conn.Close()
		}
	}()
	ft.TracerouteMethod = trace.ICMPTrace
	for _, T := range TL {
		func() {
			// 为每个追踪操作添加独立的recover
			defer func() {
				if r := recover(); r != nil {
					if model.EnableLoger {
						InitLogger()
						Logger.Error(fmt.Sprintf("trace for %s panic recovered: %v", T.ISPName, r))
					} else {
						allOutput = append(allOutput, fmt.Sprintf("Error: trace for %s panic recovered: %v", T.ISPName, r))
					}
				}
			}()
			switch testType {
			case "both":
				allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v4", T.ISPName)))
				output := tracert(ft, T)
				allOutput = append(allOutput, output...)
				allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v6", T.ISPName)))
				output = tracert_v6(ft, T)
				allOutput = append(allOutput, output...)
			case "ipv4":
				allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v4", T.ISPName)))
				output := tracert(ft, T)
				allOutput = append(allOutput, output...)
			case "ipv6":
				allOutput = append(allOutput, fmt.Sprintf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v6", T.ISPName)))
				output := tracert_v6(ft, T)
				allOutput = append(allOutput, output...)
			}
		}()
		time.Sleep(500 * time.Millisecond)
	}
	return allOutput
}
