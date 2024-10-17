# mosdns

## 功能概述
- **高性能DNS转发器**：mosdns 是一个高性能的 DNS 转发器，支持多种插件和配置方式。
- **灵活的配置**：通过 YAML 文件进行配置，支持复杂的路由规则和策略。
- **丰富的插件系统**：内置多种插件，如缓存、过滤、负载均衡等，同时支持自定义插件。

## 配置方式
- **官方文档**：详细的功能概述、配置方式和教程，请参阅 [wiki](https://irine-sistiana.gitbook.io/mosdns-wiki/)。
- **示例配置**：
  ```yaml
  server:
    listen: ":53"
    mode: rule
    rules:
      - "+.google.com": direct
      - "+.baidu.com": proxy
    plugins:
      - type: cache
        args:
          ttl: 60
