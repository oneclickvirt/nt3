package nt

import (
	"fmt"
	"log"
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

// RetryConfig 定义重试配置
type RetryConfig struct {
	MaxRetries int
	RetryDelay time.Duration
}

func realtimePrinter(res *trace.Result, ttl int) {
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
		fmt.Printf(White("*") + "\n")
		return
	}
	for ip, v := range tmpMap {
		i, _ := strconv.Atoi(v[0])
		rtt := v[1]
		fmt.Printf(Cyan("%-12s "), rtt)
		if res.Hops[ttl][i].Geo.Asnumber != "" {
			fmt.Printf(Yellow("%-10s "), fmt.Sprintf("AS%s", res.Hops[ttl][i].Geo.Asnumber))
		} else {
			fmt.Printf(White("%-10s "), "*")
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
				fmt.Printf(Yellow("%s "), fmt.Sprintf("%-18s", whoisFormat[0]))
			default:
				fmt.Printf(Green("%s "), fmt.Sprintf("%-18s", whoisFormat[0]))
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
				fmt.Printf(strings.Join(parts, ", "))
			}
		}
		fmt.Println()
	}
}

// performTrace 执行单次追踪并返回是否成功
func performTrace(ft fastTrace.FastTracer, isp fastTrace.ISPCollection, isIPv6 bool) (bool, error) {
	var destIP net.IP
	var err error
	if isIPv6 {
		fmt.Printf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v6", isp.ISPName))
		destIP, err = util.DomainLookUp(isp.IPv6, "6", "", true)
	} else {
		fmt.Printf(Yellow("%s - "), fmt.Sprintf("%s - ICMP v4", isp.ISPName))
		destIP, err = util.DomainLookUp(isp.IP, "4", "", true)
	}
	if err != nil {
		if model.EnableLoger {
			log.Printf("DNS解析失败: %v", err)
		}
		return false, err
	}
	var conf = trace.Config{
		BeginHop:         1,
		DestIP:           destIP,
		DestPort:         80,
		MaxHops:          30,
		NumMeasurements:  3,
		ParallelRequests: 18,
		RDns:             ft.ParamsFastTrace.RDns,
		AlwaysWaitRDNS:   ft.ParamsFastTrace.AlwaysWaitRDNS,
		PacketInterval:   50,
		TTLInterval:      50,
		IPGeoSource:      ipgeo.GetSource("LeoMoeAPI"),
		Timeout:          time.Duration(1000) * time.Millisecond,
		SrcAddr:          ft.ParamsFastTrace.SrcAddr,
		PktSize:          52,
		Lang:             ft.ParamsFastTrace.Lang,
		DontFragment:     ft.ParamsFastTrace.DontFragment,
	}
	conf.RealtimePrinter = realtimePrinter
	result, err := trace.Traceroute(ft.TracerouteMethod, conf)
	if err != nil {
		if model.EnableLoger {
			log.Printf("追踪失败: %v", err)
		}
		return false, err
	}
	// 检查结果是否为空
	if result == nil || len(result.Hops) == 0 {
		return false, fmt.Errorf("追踪结果为空")
	}
	return true, nil
}

// traceWithRetry 执行带重试机制的追踪
func traceWithRetry(ft fastTrace.FastTracer, isp fastTrace.ISPCollection, isIPv6 bool, config RetryConfig) {
	attempt := 0
	maxAttempts := config.MaxRetries + 1 // 包括首次尝试
	for attempt < maxAttempts {
		if attempt > 0 {
			fmt.Printf("第 %d 次重试，等待 %v...\n", attempt, config.RetryDelay)
			time.Sleep(config.RetryDelay)
		}
		success, err := performTrace(ft, isp, isIPv6)
		if success {
			return
		}
		if err != nil && model.EnableLoger {
			log.Printf("第 %d 次尝试失败: %v\n", attempt+1, err)
		}
		attempt++
	}
	fmt.Printf("在 %d 次尝试后仍未获得有效的追踪结果\n", maxAttempts)
}

func TraceRoute(language, location, testType string, retryConfig RetryConfig) {
	if language == "zh" || language == "" {
		language = "cn"
	} else if language != "en" {
		fmt.Println("无效的语言选项")
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
	default:
		fmt.Println("无效的位置选项")
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
	// 建立 WebSocket 连接
	w := wshandle.New()
	w.Interrupt = make(chan os.Signal, 1)
	signal.Notify(w.Interrupt, os.Interrupt)
	defer func() {
		w.Conn.Close()
	}()
	ft.TracerouteMethod = trace.ICMPTrace
	if TL != nil {
		for _, T := range TL {
			switch testType {
			case "both":
				traceWithRetry(ft, T, false, retryConfig) // IPv4
				traceWithRetry(ft, T, true, retryConfig)  // IPv6
			case "ipv4":
				traceWithRetry(ft, T, false, retryConfig)
			case "ipv6":
				traceWithRetry(ft, T, true, retryConfig)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
