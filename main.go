package main

import (
	"net"

	"github.com/cristalhq/aconfig"
	"github.com/sower-proxy/conns/relay"
	"github.com/sower-proxy/conns/socks5"
	"github.com/sower-proxy/deferlog/log"
	"k8s.io/client-go/informers"
)

var (
	version, date string

	conf = struct {
		Namespace string `default:"default" flag:"n"`
		Addr      string `default:":10086" flag:"p"`
	}{}
)

func init() {
	loader := aconfig.LoaderFor(&conf, aconfig.Config{})
	log.InfoFatal(loader.Load()).
		Str("version", version).
		Str("date", date).
		Interface("config", conf).
		Msg("Starting")
}

func main() {
	_, cli := getClient()
	factory := informers.NewSharedInformerFactory(cli, 0)
	svcLister := factory.Core().V1().Services().Lister()
	endpointLister := factory.Core().V1().Endpoints().Lister()

	svcTarget := SvcTarget{
		DefaultNS:      conf.Namespace,
		SvcLister:      svcLister,
		EndpointLister: endpointLister,
		TargetM:        make(map[string]*forwardTarget),
	}
	factory.Core().V1().Services().Informer().AddEventHandler(&svcTarget)
	factory.Core().V1().Endpoints().Informer().AddEventHandler(&svcTarget)
	factory.Start(nil)
	factory.WaitForCacheSync(nil)

	ln, err := net.Listen("tcp", conf.Addr)
	log.InfoFatal(err).
		Str("addr", conf.Addr).
		Msg("listen")

	handle(ln, &socks5.Socks5{}, svcTarget.TargetM)

	select {}
}

func handle(ln net.Listener, socks *socks5.Socks5, targetM map[string]*forwardTarget) {
	conn, err := ln.Accept()
	log.DebugFatal(err).Msg("accept")
	go handle(ln, socks, targetM)
	defer conn.Close()

	tgtAddr, err := socks.Unwrap(conn)
	if err != nil {
		log.Error().Err(err).Msg("unwrap socks5")
		return
	}

	tgtAddrStr := tgtAddr.String()
	if tgt, ok := targetM[tgtAddrStr]; ok {
		log.DebugWarn(tgt.forword(conn)).
			Str("addr", tgtAddrStr).
			Msg("forword")
	}

	dur, err := relay.RelayTo(conn, tgtAddrStr)
	log.DebugWarn(err).
		Dur("took", dur).
		Str("tgt", tgtAddrStr).
		Msg("relay")
}
