# nt3

[![Hits](https://hits.seeyoufarm.com/api/count/incr/badge.svg?url=https%3A%2F%2Fgithub.com%2Foneclickvirt%2Fnt3&count_bg=%232EFFF8&title_bg=%23555555&icon=&icon_color=%23E7E7E7&title=hits&edge_flat=false)](https://hits.seeyoufarm.com) [![Build and Release](https://github.com/oneclickvirt/nt3/actions/workflows/main.yaml/badge.svg)](https://github.com/oneclickvirt/nt3/actions/workflows/main.yaml)

三网路由查询模块

## 说明

- [x] 使用[nexttrace](https://github.com/nxtrace/NTrace-core)进行ICMP测试，预先加载定义好的广州、上海、北京、成都的三网地址
- [x] 支持双语输出，以```-l```指定```zh```或```en```可指定输出的语言，未指定时默认使用中文输出
- [x] 全平台编译支持(除了WIN)

## 使用

下载及安装

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
Usage of nt3:
  -c string
        Specify check type (both, ipv4, or ipv6) (default "ipv4")
  -l string
        Specify language parameter (en or zh) (default "zh")
  -loc string
        Specify location (supports GZ, BJ, SH, CD; corresponding to Guangzhou, Beijing, Shanghai, Chengdu) (default "GZ")
  -v    Show version information
```

## 在Golang中使用

```
go get github.com/oneclickvirt/nt3@latest
```

## Thanks

https://github.com/nxtrace/NTrace-core
