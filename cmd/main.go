package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oneclickvirt/nt3/model"
	"github.com/oneclickvirt/nt3/nt"
)

type cliAction int

const (
	cliLegacy cliAction = iota
	cliHelp
	cliVersion
	cliProvince
)

func main() {
	go func() {
		http.Get("https://hits.spiritlhl.net/nt3.svg?action=hit&title=Hits&title_bg=%23555555&count_bg=%230eecf8&edge_flat=false")
	}()
	var showVersion, help bool
	var language, checkType, location, provinceTargets, provinceIP string
	var provinceJSON, deep, provinceRegistry bool
	var provinceAttempts, provinceConcurrency, provincePort int
	var provinceTimeout time.Duration
	nt3Flag := flag.NewFlagSet("nt3", flag.ContinueOnError)
	nt3Flag.BoolVar(&help, "h", false, "Show help information")
	nt3Flag.BoolVar(&showVersion, "v", false, "Show version information")
	nt3Flag.StringVar(&language, "l", "zh", "Specify language parameter (en or zh)")
	nt3Flag.StringVar(&checkType, "c", "ipv4", "Specify check type (both, ipv4, or ipv6)")
	nt3Flag.StringVar(&location, "loc", "GZ", "Specify location (supports GZ, BJ, SH, CD, ALL; corresponding to Guangzhou, Beijing, Shanghai, Chengdu and All)")
	nt3Flag.BoolVar(&model.EnableLoger, "log", false, "Enable logging")
	nt3Flag.StringVar(&provinceTargets, "province-target", "", "省级 TCP 目标，格式 code,carrier,ipversion,host；多个目标用分号分隔")
	nt3Flag.StringVar(&provinceIP, "province-ip", "both", "省级目标 IP 版本: ipv4, ipv6 或 both")
	nt3Flag.IntVar(&provinceAttempts, "province-attempts", 2, "省级延迟每个目标的尝试次数")
	nt3Flag.DurationVar(&provinceTimeout, "province-timeout", 1500*time.Millisecond, "省级延迟单次连接或 deep 单目标路由超时")
	nt3Flag.IntVar(&provinceConcurrency, "province-concurrency", 12, "省级延迟最大并发数")
	nt3Flag.IntVar(&provincePort, "province-port", 80, "省级延迟 TCP 端口")
	nt3Flag.BoolVar(&provinceJSON, "json", false, "省级模式输出结构化 JSON")
	nt3Flag.BoolVar(&deep, "deep", false, "省级模式执行 Go NTrace 路由探测")
	nt3Flag.BoolVar(&provinceRegistry, "province-registry", false, "加载仓库内置的 31 省三运营商双栈目标")
	if err := nt3Flag.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	action, actionErr := selectCLIAction(help, showVersion, provinceJSON, deep, provinceTargets, provinceRegistry)
	if !(action == cliProvince && provinceJSON) {
		fmt.Println("Repo:", "https://github.com/oneclickvirt/nt3")
	}
	if actionErr != nil {
		fmt.Fprintln(os.Stderr, actionErr)
		return
	}
	if action == cliHelp {
		fmt.Printf("Usage: %s [options]\n", os.Args[0])
		nt3Flag.PrintDefaults()
		fmt.Fprintln(nt3Flag.Output(), "Province example: -province-target 'BJ,ct,ipv4,example.com;SH,cu,ipv6,2001:db8::1' -json")
		return
	}
	if action == cliVersion {
		fmt.Println(model.NextTraceVersion)
		return
	}
	if action == cliProvince {
		if err := runProvinceMode(context.Background(), os.Stdout, provinceTargets, provinceIP, provinceAttempts, provinceTimeout, provinceConcurrency, provincePort, deep, provinceJSON, provinceRegistry); err != nil {
			fmt.Fprintf(os.Stderr, "province mode failed: %v\n", err)
			return
		}
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
			// 不要退出程序，仅输出错误信息并继续
			for _, res := range result.Output {
				res = strings.TrimSpace(res)
				if res != "" {
					fmt.Println(res)
				}
			}
			continue // 改为continue而不是return
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

func selectCLIAction(help, version, jsonOutput, deep bool, provinceTargets string, provinceRegistry ...bool) (cliAction, error) {
	if help {
		return cliHelp, nil
	}
	if version {
		return cliVersion, nil
	}
	if strings.TrimSpace(provinceTargets) != "" {
		return cliProvince, nil
	}
	if len(provinceRegistry) > 0 && provinceRegistry[0] {
		return cliProvince, nil
	}
	if jsonOutput || deep {
		return cliLegacy, fmt.Errorf("-json and -deep require -province-target")
	}
	return cliLegacy, nil
}

func parseProvinceTargets(spec, defaultIP string) ([]model.ProvinceLatencyTarget, error) {
	defaultIP = strings.ToLower(strings.TrimSpace(defaultIP))
	if defaultIP != "ipv4" && defaultIP != "ipv6" && defaultIP != "both" {
		return nil, fmt.Errorf("invalid province IP version %q", defaultIP)
	}
	var targets []model.ProvinceLatencyTarget
	seen := make(map[string]struct{})
	for _, raw := range strings.Split(spec, ";") {
		parts := strings.SplitN(strings.TrimSpace(raw), ",", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid province target %q; want code,carrier,ipversion,host", raw)
		}
		code, carrier, ipVersion, host := strings.TrimSpace(parts[0]), strings.ToLower(strings.TrimSpace(parts[1])), strings.ToLower(strings.TrimSpace(parts[2])), strings.TrimSpace(parts[3])
		if code == "" || carrier == "" || host == "" {
			return nil, fmt.Errorf("province target %q has empty fields", raw)
		}
		if ipVersion == "" {
			ipVersion = defaultIP
		}
		if ipVersion != "ipv4" && ipVersion != "ipv6" && ipVersion != "both" {
			return nil, fmt.Errorf("province target %q has invalid IP version", raw)
		}
		versions := []string{ipVersion}
		if ipVersion == "both" {
			versions = []string{"ipv4", "ipv6"}
		}
		for _, version := range versions {
			key := strings.Join([]string{code, carrier, version, host}, "|")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, model.ProvinceLatencyTarget{ProvinceCode: code, ProvinceName: code, Carrier: carrier, IPVersion: version, Host: host})
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no province targets")
	}
	return targets, nil
}

func runProvinceMode(ctx context.Context, output io.Writer, spec, ipVersion string, attempts int, timeout time.Duration, concurrency, port int, deep, jsonOutput bool, useRegistry ...bool) error {
	ipVersion = strings.ToLower(strings.TrimSpace(ipVersion))
	if ipVersion != "ipv4" && ipVersion != "ipv6" && ipVersion != "both" {
		return fmt.Errorf("invalid province IP version %q", ipVersion)
	}
	var targets []model.ProvinceLatencyTarget
	var err error
	if len(useRegistry) > 0 && useRegistry[0] {
		loaded, loadErr := model.LoadProvinceRoutes(ctx, nil, model.DefaultProvinceRouteRegistrySources())
		if loadErr != nil {
			return loadErr
		}
		targets = model.BuildProvinceLatencyTargets(loaded.Routes, ipVersion)
		if len(targets) == 0 {
			return fmt.Errorf("province registry returned no targets")
		}
	} else {
		targets, err = parseProvinceTargets(spec, ipVersion)
		if err != nil {
			return err
		}
	}
	if timeout <= 0 || concurrency < 1 {
		return fmt.Errorf("province timeout and concurrency must be positive")
	}
	if !deep && (attempts < 1 || port < 1 || port > 65535) {
		return fmt.Errorf("province attempts and port must be valid")
	}
	if deep {
		results := runDeepProvinceTargets(ctx, targets, concurrency, timeout, nt.NTraceProvinceTracer)
		if jsonOutput {
			return json.NewEncoder(output).Encode(results)
		}
		for _, result := range results {
			fmt.Fprintf(output, "%s/%s/%s %s hops=%d", result.Target.ProvinceCode, result.Target.Carrier, result.Target.IPVersion, result.Status, len(result.Hops))
			if result.Error != "" {
				fmt.Fprintf(output, " error=%s", result.Error)
			}
			fmt.Fprintln(output)
		}
		return nil
	}
	results := nt.RunProvinceLatency(ctx, targets, nt.ProvinceLatencyConfig{Attempts: attempts, Timeout: timeout, Concurrency: concurrency, Port: port})
	if jsonOutput {
		return json.NewEncoder(output).Encode(results)
	}
	fmt.Fprintln(output, "目标\t成功\t丢包\t平均\tP95")
	for _, result := range results {
		fmt.Fprintf(output, "%s/%s/%s\t%d/%d\t%.1f%%\t%s\t%s\n", result.Target.ProvinceCode, result.Target.Carrier, result.Target.IPVersion, result.Successful, result.Attempts, result.LossPercent, result.Mean, result.P95)
	}
	return nil
}

func runDeepProvinceTargets(ctx context.Context, targets []model.ProvinceLatencyTarget, concurrency int, timeout time.Duration, tracer nt.ProvinceRouteTracer) []nt.DetailedProvinceRouteResult {
	results := make([]nt.DetailedProvinceRouteResult, len(targets))
	if len(targets) == 0 {
		return results
	}
	if tracer == nil {
		tracer = nt.NTraceProvinceTracer
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	wg.Add(concurrency)
	for worker := 0; worker < concurrency; worker++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				target := targets[index]
				started := time.Now()
				probeCtx, cancel := context.WithTimeout(ctx, timeout)
				hops, err := tracer(probeCtx, target)
				cancel()
				result := nt.DetailedProvinceRouteResult{Target: target, Status: nt.ProvinceRouteStatusOK, Duration: time.Since(started), Hops: hops}
				if err != nil {
					result.Status, result.Error = nt.ProvinceRouteStatusError, err.Error()
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						result.Status = nt.ProvinceRouteStatusCanceled
					}
				}
				results[index] = result
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}
