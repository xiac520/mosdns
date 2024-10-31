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
