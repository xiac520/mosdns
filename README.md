
```markdown
# Simple MosDNS

## 简介

Simple MosDNS 是一个预配置的 MosDNS 服务器 Docker 镜像，旨在为用户提供开箱即用、无需配置的 MosDNS 解决方案。MosDNS 是一个用 Go 编写的轻量级 DNS 服务器，旨在提供快速高效的 DNS 解析。

## 功能

- **自动规则更新**：包含一个 cron 作业，每周六凌晨 2 点从 `http://oss.dnscron.com/mosdns` 获取最新的规则文件并重启 MosDNS。**仅在 `auto` 模式下生效**。
- **DNS 去污染和分流**：使用 `geoip.dat` 和 `geosite.dat` 数据文件实现 DNS 去污染和分流功能，确保国内网站使用国内 DNS 服务器，国外网站使用国外 DNS 服务器。**仅在 `auto` 模式下生效**。
- **三配置源**：您可以选择自动从 `https://github.com/xiac520/simple-mosdns` 拉取配置文件，或使用用户定义的 GitHub 仓库，或使用用户定义的挂载配置目录。

```
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

## Docker镜像

[Simple MosDNS]([https://github.com/moreoronce/MosDNS-Config](https://hub.docker.com/r/xiac520/simple-mosdns)

## 参考 MosDNS 库

该镜像使用了 MosDNS 库，其源代码可以在 [MosDNS GitHub 仓库](https://github.com/IrineSistiana/mosdns) 中找到。
```
