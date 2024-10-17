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

 package quic_server

 import (
	 "crypto/tls"
	 "errors"
	 "fmt"
	 "net"
	 "sync"
	 "time"
 
	 "github.com/IrineSistiana/mosdns/v5/coremain"
	 "github.com/IrineSistiana/mosdns/v5/pkg/server"
	 "github.com/IrineSistiana/mosdns/v5/pkg/utils"
	 "github.com/IrineSistiana/mosdns/v5/plugin/server/server_utils"
	 "github.com/quic-go/quic-go"
 )
 
 const PluginType = "quic_server"
 
 func init() {
	 coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
 }
 
 type Args struct {
	 Entry       string `yaml:"entry"`
	 Listen      string `yaml:"listen"`
	 Cert        string `yaml:"key"`
	 Key         string `yaml:"key"`
	 IdleTimeout int    `yaml:"idle_timeout"`
 }
 
 func (a *Args) init() error {
	 if len(a.Key) == 0 || len(a.Cert) == 0 {
		 return errors.New("quic server requires a tls certificate")
	 }
	 utils.SetDefaultNum(&a.IdleTimeout, 30)
	 return nil
 }
 
 type QuicServer struct {
	 args *Args
 
	 l *quic.Listener
 }
 
 func (s *QuicServer) Close() error {
	 return s.l.Close()
 }
 
 func Init(bp *coremain.BP, args any) (any, error) {
	 return StartServer(bp, args.(*Args))
 }
 
 var udpConnPool = sync.Pool{
	 New: func() interface{} {
		 conn, _ := net.ListenPacket("udp", "")
		 return conn
	 },
 }
 
 func getUDPConn(addr string) (net.PacketConn, error) {
	 conn := udpConnPool.Get().(net.PacketConn)
	 defer func() {
		 udpConnPool.Put(conn)
	 }()
 
	 err := conn.SetDeadline(time.Now().Add(5 * time.Second))
	 if err != nil {
		 return nil, fmt.Errorf("failed to set deadline, %w", err)
	 }
 
	 return conn, nil
 }
 
 func StartServer(bp *coremain.BP, args *Args) (*QuicServer, error) {
	 if err := args.init(); err != nil {
		 return nil, fmt.Errorf("failed to initialize args, %w", err)
	 }
 
	 dh, err := server_utils.NewHandler(bp, args.Entry)
	 if err != nil {
		 return nil, fmt.Errorf("failed to init dns handler, %w", err)
	 }
 
	 tlsConfig := new(tls.Config)
	 if err := server.LoadCert(tlsConfig, args.Cert, args.Key); err != nil {
		 return nil, fmt.Errorf("failed to read tls cert, %w", err)
	 }
	 tlsConfig.NextProtos = []string{"doq"}
 
	 uc, err := getUDPConn(args.Listen)
	 if err != nil {
		 return nil, fmt.Errorf("failed to listen socket, %w", err)
	 }
	 defer uc.Close()
 
	 idleTimeout := time.Duration(args.IdleTimeout) * time.Second
 
	 quicConfig := &quic.Config{
		 MaxIdleTimeout:                 idleTimeout,
		 InitialStreamReceiveWindow:     4 * 1024,
		 MaxStreamReceiveWindow:         4 * 1024,
		 InitialConnectionReceiveWindow: 8 * 1024,
		 MaxConnectionReceiveWindow:     16 * 1024,
		 Allow0RTT:                      false,
		 MaxIncomingUniStreams:          -1,
	 }
 
	 srk, _, err := utils.InitQUICSrkFromIfaceMac()
	 if err != nil {
		 // No logging here
	 }
	 qt := &quic.Transport{
		 Conn:              uc,
		 StatelessResetKey: (*quic.StatelessResetKey)(srk),
	 }
 
	 quicListener, err := qt.Listen(tlsConfig, quicConfig)
	 if err != nil {
		 qt.Close()
		 return nil, fmt.Errorf("failed to listen quic, %w", err)
	 }
 
	 go func() {
		 defer quicListener.Close()
		 serverOpts := server.DoQServerOpts{Logger: bp.L(), IdleTimeout: idleTimeout}
		 err := server.ServeDoQ(quicListener, dh, serverOpts)
		 bp.M().GetSafeClose().SendCloseSignal(err)
	 }()
 
	 return &QuicServer{
		 args: args,
		 l:    quicListener,
	 }, nil
 }