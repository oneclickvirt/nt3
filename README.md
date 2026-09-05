# nt3

[![Hits](https://hits.spiritlhl.net/nt3.svg?action=hit&title=Hits&title_bg=%23555555&count_bg=%230eecf8&edge_flat=false)](https://hits.spiritlhl.net)

[![Build and Release](https://github.com/oneclickvirt/nt3/actions/workflows/main.yaml/badge.svg)](https://github.com/oneclickvirt/nt3/actions/workflows/main.yaml)

三网路由查询模块

## 说明

- [x] 使用[nexttrace](https://github.com/nxtrace/NTrace-core)进行ICMP测试，预先加载定义好的广州、上海、北京、成都的三网地址
- [x] 支持双语输出，以```-l```指定```zh```或```en```可指定输出的语言，未指定时默认使用中文输出
- [x] 全平台编译支持(除了WIN)
- [x] 默认启用并发测试，单地点三网测试至多需要1~2分钟

## 使用

下载、安装、更新

```
curl https://raw.githubusercontent.com/oneclickvirt/nt3/main/nt3_install.sh -sSf | bash
```

或

```
curl https://cdn.spiritlhl.net/https://raw.githubusercontent.com/oneclickvirt/nt3/main/nt3_install.sh -sSf | bash
```

使用

```
nt3
```

或

```
./nt3
```

进行测试

无环境依赖，理论上适配所有系统和主流架构，更多架构请查看 https://github.com/oneclickvirt/nt3/releases/tag/output

```
Usage: nt3 [options]
  -c string
        Specify check type (both, ipv4, or ipv6) (default "ipv4")
  -deep
        Run Go NTrace routes for the selected province targets
  -h    Show help information
  -json
        Print province-mode results as structured JSON
  -l string
        Specify language parameter (en or zh) (default "zh")
  -loc string
        Specify location (supports GZ, BJ, SH, CD, ALL; corresponding to Guangzhou, Beijing, Shanghai, Chengdu and All) (default "GZ")
  -log
        Enable logging
  -province-attempts int
        Attempts per province latency target (default 2)
  -province-concurrency int
        Maximum concurrent province targets (default 12)
  -province-ip string
        Province target IP version: ipv4, ipv6, or both (default "both")
  -province-port int
        TCP port used by province latency probes (default 80)
  -province-registry
        Load the embedded 31-province, three-carrier dual-stack registry
  -province-target string
        Custom targets: code,carrier,ipversion,host; separate targets with semicolons
  -province-timeout duration
        Per-attempt or per-route timeout (default 1.5s)
  -v    Show version information
```

省级延迟示例：

```bash
nt3 -province-registry -province-ip both
nt3 -province-registry -province-ip ipv4 -json
nt3 -province-target 'BJ,ct,ipv4,example.com;SH,cu,ipv6,2001:db8::1' -deep -province-timeout 10s
```

## 卸载

```
rm -rf /root/nt3
rm -rf /usr/bin/nt3
```

## 在Golang中使用

```
go get github.com/oneclickvirt/nt3@v0.0.25-20260905215719
```

## 示例图

![图片](https://github.com/oneclickvirt/nt3/assets/103393591/e287cfde-3a22-4532-972e-31da95241df0)

![图片](https://github.com/oneclickvirt/nt3/assets/103393591/1f0ab6be-7900-438c-98a4-c338fd515933)

![图片](https://github.com/oneclickvirt/nt3/assets/103393591/0484fadd-6887-4db4-afb9-e32e7f382463)

## Thanks

https://github.com/nxtrace/NTrace-core
