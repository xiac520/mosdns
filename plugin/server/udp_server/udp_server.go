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

 package udp_server

 import (
	 "context"
	 "fmt"
	 "net"
	 "sync"
 
	 "github.com/IrineSistiana/mosdns/v5/coremain"
	 "github.com/IrineSistiana/mosdns/v5/pkg/server"
	 "github.com/IrineSistiana/mosdns/v5/pkg/utils"
	 "github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
	 "go.uber.org/zap"
 )
 
 const PluginType = "udp_server"
 
 func init() {
	 coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
 }
 
 type Args struct {
	 Entry  string `yaml:"entry"`
	 Listen string `yaml:"listen"`
 }
 
 func (a *Args) init() {
	 utils.SetDefaultString(&a.Listen, "127.0.0.1:53")
 }
 
 type UdpServer struct {
	 args *Args
 
	 c net.PacketConn
	 wg sync.WaitGroup
 }
 
 func (s *UdpServer) Close() error {
	 s.wg.Wait()
	 return s.c.Close()
 }
 
 func Init(bp *coremain.BP, args any) (any, error) {
	 return StartServer(bp, args.(*Args))
 }
 
 func StartServer(bp *coremain.BP, args *Args) (*UdpServer, error) {
	 dh, err := server_utils.NewHandler(bp, args.Entry)
	 if err != nil {
		 return nil, fmt.Errorf("failed to init dns handler, %w", err)
	 }
 
	 socketOpt := server_utils.ListenerSocketOpts{
		 SO_REUSEPORT: true,
		 SO_RCVBUF:    256 * 1024, // 增加接收缓冲区大小
	 }
	 lc := net.ListenConfig{Control: server_utils.ListenerControl(socketOpt)}
	 c, err := lc.ListenPacket(context.Background(), "udp", args.Listen)
	 if err != nil {
		 return nil, fmt.Errorf("failed to create socket, %w", err)
	 }
	 bp.L().Info("udp server started", zap.Stringer("addr", c.LocalAddr()))
 
	 // 启动多个 goroutine 处理请求
	 const numWorkers = 8
	 s := &UdpServer{
		 args: args,
		 c:    c,
	 }
	 for i := 0; i < numWorkers; i++ {
		 s.wg.Add(1)
		 go s.handleRequests(dh, bp)
	 }
 
	 return s, nil
 }
 
 func (s *UdpServer) handleRequests(dh server.Handler, bp *coremain.BP) {
	 defer s.wg.Done()
	 buf := make([]byte, 65536) // 增加缓冲区大小
	 for {
		 n, addr, err := s.c.ReadFrom(buf)
		 if err != nil {
			 bp.L().Error("failed to read from socket", zap.Error(err))
			 continue
		 }
		 go func(addr net.Addr, data []byte) {
			 err := server.ServeUDPRequest(s.c, addr, data, dh, server.UDPServerOpts{Logger: bp.L()})
			 if err != nil {
				 bp.L().Error("failed to serve UDP request", zap.Error(err))
			 }
		 }(addr, buf[:n])
	 }
 }