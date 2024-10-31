
```markdown
# Simple MosDNS

## 简介

Simple MosDNS 是一个预配置的 MosDNS 服务器 Docker 镜像，旨在为用户提供开箱即用、无需配置的 MosDNS 解决方案。MosDNS 是一个用 Go 编写的轻量级 DNS 服务器，旨在提供快速高效的 DNS 解析。

## 功能

- **自动规则更新**：包含一个 cron 作业，每周六凌晨 2 点从 `http://oss.dnscron.com/mosdns` 获取最新的规则文件并重启 MosDNS。**仅在 `auto` 模式下生效**。
- **DNS 去污染和分流**：使用 `geoip.dat` 和 `geosite.dat` 数据文件实现 DNS 去污染和分流功能，确保国内网站使用国内 DNS 服务器，国外网站使用国外 DNS 服务器。**仅在 `auto` 模式下生效**。
- **三配置源**：您可以选择自动从 `https://github.com/xiac520/simple-mosdns` 拉取配置文件，或使用用户定义的 GitHub 仓库，或使用用户定义的挂载配置目录。

### 使用方法

### 自动拉取配置文件

```bash
docker run -d --name simple-mosdns -p 53:53/udp -p 53:53/tcp -e CONFIG_SOURCE=auto xiac520/simple-mosdns
```

### 使用用户定义的 GitHub 仓库

```bash
docker run -d --name simple-mosdns -p 53:53/udp -p 53:53/tcp -e CONFIG_REPO=https://github.com/yourusername/yourrepo xiac520/simple-mosdns
```

### 使用用户定义的配置目录

```bash
docker run -d --name simple-mosdns -p 53:53/udp -p 53:53/tcp -v /path/to/your/config:/etc/mosdns xiac520/simple-mosdns
```

## 环境变量

- **CONFIG_SOURCE**：配置文件源。设置为 `auto` 以自动从 `https://github.com/xiac520/simple-mosdns` 拉取配置文件；设置为其他值以使用用户定义的挂载配置目录。
- **CONFIG_REPO**：用户定义的 GitHub 仓库地址。如果设置了此变量，将使用该仓库的配置文件。**特别提醒：`config.yaml` 文件必须在仓库的根目录**。

## 验证

您可以使用 `nslookup` 或其他 DNS 查询工具来验证 MosDNS 是否正常工作：

```bash
nslookup example.com 127.0.0.1
```

## 基于 MosDNS V5

该镜像基于 MosDNS V5 构建，提供最新的功能和改进。

## 规则配置

该镜像的规则配置参考了 [MosDNS-Config GitHub 仓库](https://github.com/moreoronce/MosDNS-Config)，确保开箱即用的 DNS 去污染和分流功能。

## 参考 MosDNS 库

该镜像使用了 MosDNS 库，其源代码可以在 [MosDNS GitHub 仓库](https://github.com/IrineSistiana/mosdns) 中找到。
```

### Dockerfile

```dockerfile
# 使用官方的 Golang 1.23.2 镜像作为基础镜像
FROM golang:1.23.2-alpine AS builder

# 安装 Git
RUN apk add --no-cache git

# 设置工作目录
WORKDIR /app

# 克隆 MosDNS 的源代码
RUN git clone https://github.com/IrineSistiana/mosdns.git .

# 下载依赖
RUN go mod download

# 编译 MosDNS
RUN go build -o mosdns .

# 使用轻量级的 Alpine 镜像作为最终镜像
FROM alpine:3.14

# 安装必要的依赖
RUN apk add --no-cache ca-certificates wget unzip git

# 创建工作目录
RUN mkdir -p /etc/mosdns

# 从 builder 阶段复制编译好的 MosDNS 二进制文件
COPY --from=builder /app/mosdns /usr/local/bin/mosdns

# 设置工作目录
WORKDIR /etc/mosdns

# 复制 entrypoint.sh 脚本
COPY entrypoint.sh /entrypoint.sh

# 设置 entrypoint.sh 脚本为可执行
RUN chmod +x /entrypoint.sh

# 暴露 MosDNS 的端口
EXPOSE 53/udp
EXPOSE 53/tcp

# 设置 entrypoint.sh 为启动命令
ENTRYPOINT ["/entrypoint.sh"]
```

### entrypoint.sh

```bash
#!/bin/sh

# 定义拉取规则文件的函数
fetch_rules() {
    wget -O /etc/mosdns/rules/geoip_cn.txt http://oss.dnscron.com/mosdns/geoip_cn.txt
    wget -O /etc/mosdns/rules/geoip_private.txt http://oss.dnscron.com/mosdns/geoip_private.txt
    wget -O /etc/mosdns/rules/geosite_cn.txt http://oss.dnscron.com/mosdns/geosite_cn.txt
    wget -O /etc/mosdns/rules/geosite_gfw.txt http://oss.dnscron.com/mosdns/geosite_gfw.txt
    wget -O /etc/mosdns/rules/geosite_geolocation-!cn.txt http://oss.dnscron.com/mosdns/geosite_geolocation-%21cn.txt
    wget -O /etc/mosdns/rules/geosite_category-ads-all.txt http://oss.dnscron.com/mosdns/geosite_category-ads-all.txt
}

# 检查环境变量 CONFIG_SOURCE
if [ "$CONFIG_SOURCE" = "auto" ]; then
    # 下载配置文件并覆盖目标目录
    wget -O /tmp/mosdns.zip https://gh.llkk.cc/https://github.com/xiac520/simple-mosdns/archive/refs/heads/main.zip
    unzip -o /tmp/mosdns.zip -d /tmp
    cp -rf /tmp/simple-mosdns-main/config/* /etc/mosdns
    rm -rf /tmp/mosdns.zip /tmp/simple-mosdns-main

    # 检测缺失的规则文件并重新拉取
    fetch_rules

    # 确保所有必要的文件都存在
    required_files="/etc/mosdns/config.yaml /etc/mosdns/rules/geoip_cn.txt /etc/mosdns/rules/geoip_private.txt /etc/mosdns/rules/geosite_cn.txt /etc/mosdns/rules/geosite_gfw.txt /etc/mosdns/rules/geosite_geolocation-!cn.txt /etc/mosdns/rules/geosite_category-ads-all.txt"

    for file in $required_files; do
        if [ ! -f "$file" ]; then
            echo "File '$file' not found. Please ensure it is included in the configuration directory."
            exit 1
        fi
    done

    # 设置定时任务，每周六凌晨2点拉取规则文件并重启 MosDNS
    echo "0 2 * * 6 /bin/sh -c 'fetch_rules && pkill mosdns && mosdns start -c /etc/mosdns/config.yaml'" | crontab -
elif [ -n "$CONFIG_REPO" ]; then
    # 使用用户定义的 GitHub 仓库下载配置文件
    git clone --depth 1 --branch main "$CONFIG_REPO" /tmp/custom-mosdns
    cp -rf /tmp/custom-mosdns/* /etc/mosdns
    rm -rf /tmp/custom-mosdns

    # 检测缺失的规则文件并重新拉取
    fetch_rules
else
    # 使用用户定义的挂载配置目录
    echo "Using user-defined configuration directory."
fi

# 启动 MosDNS
mosdns start -c /etc/mosdns/config.yaml &

# 启动 cron 服务
crond -f
```
