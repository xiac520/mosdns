/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package tools

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/miekg/dns"
	"github.com/spf13/cobra"
)

var connPool = sync.Pool{
	New: func() interface{} {
		return &net.Conn{}
	},
}

func newIdleTimeoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "idle-timeout {tcp|tls}://server_addr[:port]",
		Args:  cobra.ExactArgs(1),
		Short: "Probe server's idle timeout.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := ProbServerTimeout(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
}

func newConnReuseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "conn-reuse {tcp|tls}://server_addr[:port]",
		Args:  cobra.ExactArgs(1),
		Short: "Test connection reuse.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := TestConnReuse(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
		DisableFlagsInUseLine: true,
	}
}

func ProbServerTimeout(server string) error {
	// 解析服务器地址
	addr, port, err := parseServerAddr(server)
	if err != nil {
		return err
	}

	// 创建连接
	conn, err := createConnection(addr, port)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 发送请求
	err = sendRequest(conn)
	if err != nil {
		return err
	}

	// 接收响应
	resp, err := receiveResponse(conn)
	if err != nil {
		return err
	}

	// 处理响应
	if resp == nil {
		return fmt.Errorf("no response received")
	}

	return nil
}

func TestConnReuse(server string) error {
	// 解析服务器地址
	addr, port, err := parseServerAddr(server)
	if err != nil {
		return err
	}

	// 创建连接池
	pool := &sync.Pool{
		New: func() interface{} {
			conn, err := createConnection(addr, port)
			if err != nil {
				mlog.S().Error(err)
				return nil
			}
			return conn
		},
	}

	// 并发测试连接复用
	var wg sync.WaitGroup
	var successCount int32
	var failureCount int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn := pool.Get().(*net.Conn)
			if conn == nil {
				atomic.AddInt32(&failureCount, 1)
				return
			}
			defer pool.Put(conn)

			// 发送请求
			err := sendRequest(conn)
			if err != nil {
				atomic.AddInt32(&failureCount, 1)
				return
			}

			// 接收响应
			resp, err := receiveResponse(conn)
			if err != nil || resp == nil {
				atomic.AddInt32(&failureCount, 1)
				return
			}

			atomic.AddInt32(&successCount, 1)
		}()
	}

	wg.Wait()

	mlog.S().Infof("Success: %d, Failure: %d", successCount, failureCount)
	return nil
}

func parseServerAddr(server string) (string, int, error) {
	// 解析服务器地址和端口
	parts := strings.Split(server, "://")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid server address format")
	}

	addrPort := parts[1]
	addrParts := strings.Split(addrPort, ":")
	if len(addrParts) == 1 {
		return addrParts[0], 53, nil
	}

	port, err := strconv.Atoi(addrParts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid port number")
	}

	return addrParts[0], port, nil
}

func createConnection(addr string, port int) (*net.Conn, error) {
	// 创建连接
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// 启用 TCP 快速打开
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return nil, err
	}

	conn, err := dialer.Dial("tcp", tcpAddr.String())
	if err != nil {
		return nil, err
	}

	// 启用 TCP 快速打开
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(30 * time.Second)
	tcpConn.SetLinger(0)

	return &conn, nil
}

func sendRequest(conn *net.Conn) error {
	// 发送请求
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn("example.com."), dns.TypeA)
	req.RecursionDesired = true

	wire, err := req.Pack()
	if err != nil {
		return err
	}

	_, err = (*conn).Write(wire)
	return err
}

func receiveResponse(conn *net.Conn) (*dns.Msg, error) {
	// 接收响应
	buf := make([]byte, 512)
	n, err := (*conn).Read(buf)
	if err != nil {
		return nil, err
	}

	resp := new(dns.Msg)
	err = resp.Unpack(buf[:n])
	if err != nil {
		return nil, err
	}

	return resp, nil
}