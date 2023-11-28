package main

import (
	"github.com/katzenpost/katzenpost/client/utils"
	"github.com/katzenpost/katzenpost/katzensocks/client"
	kquic "github.com/katzenpost/katzenpost/quic"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"context"
	"flag"
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	cfgFile   = flag.String("cfg", "katzensocks.toml", "config file")
	gateway   = flag.String("gw", "", "gateway provider name, default uses random gateway for each connection")
	pkiOnly   = flag.Bool("list", false, "fetch and display pki and gateways, does not connect")
	bind      = flag.String("bind", "127.0.0.1", "address to bind to")
	socksPort = flag.Int("socks5", 4242, "socks5 listening port")
	http3Port = flag.Int("http3", 4343, "http3 proxy listening port")
	retry     = flag.Int("retry", -1, "limit number of reconnection attempts")
	delay     = flag.Int("delay", 30, "time to wait between connection attempts (seconds)>")
	wg        = new(sync.WaitGroup)
)

func showPKI() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*delay)*time.Second)
	defer cancel()

	_, doc, err := client.GetPKI(ctx, *cfgFile)
	if err != nil {
		panic(err)
	}
	// display the pki.Document
	fmt.Println(doc.String())

	// display the gateway services
	descs := utils.FindServices("katzensocks", doc)
	for _, desc := range descs {
		fmt.Println(desc)
	}
}

func main() {
	flag.Parse()
	if *pkiOnly {
		showPKI()
		return
	}
	s, err := client.GetSession(*cfgFile, *delay, *retry)
	if err != nil {
		panic(err)
	}

	c, err := client.NewClient(s)
	if err != nil {
		panic(err)
	}

	listenSOCKS(c)
	listenQUIC(c)
	wg.Wait()
}

func listenQUIC(c *client.Client) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *bind, *http3Port))
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	qcfg := &quic.Config{}
	tlsConf := kquic.GenerateTLSConfig()
	server := http3.Server{QuicConfig: qcfg, TLSConfig: tlsConf, Handler: c, StreamHijacker: c.MapStream}
	wg.Add(1)
	go func() {
		server.Serve(conn)
		wg.Done()
	}()
}

func listenSOCKS(c *client.Client) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *socksPort))
	if err != nil {
		panic(err)
	}

	wg.Add(1)
	go func() {
		_ = socksAcceptLoop(c, ln)
		wg.Done()
	}()
}

func socksAcceptLoop(c *client.Client, ln net.Listener) error {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if e, ok := err.(net.Error); ok && !e.Temporary() {
				return err
			}
			continue
		}
		go c.SocksHandler(conn)
	}
}
