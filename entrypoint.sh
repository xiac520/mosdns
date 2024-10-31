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
