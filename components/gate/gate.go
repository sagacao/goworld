package main

import (
	"flag"

	"math/rand"
	"time"

	"os"

	_ "net/http/pprof"

	"runtime"

	"os/signal"

	"syscall"

	"fmt"

	"path"

	"github.com/sagacao/goworld/engine/binutil"
	"github.com/sagacao/goworld/engine/common"
	"github.com/sagacao/goworld/engine/config"
	"github.com/sagacao/goworld/engine/dispatchercluster"
	"github.com/sagacao/goworld/engine/dispatchercluster/dispatcherclient"
	"github.com/sagacao/goworld/engine/gwlog"
	"github.com/sagacao/goworld/engine/netutil"
	"github.com/sagacao/goworld/engine/post"
	"github.com/sagacao/goworld/engine/proto"
)

var (
	args struct {
		gateid          uint16
		configFile      string
		logLevel        string
		runInDaemonMode bool
		//listenAddr      string
	}
	gateService *GateService
	signalChan  = make(chan os.Signal, 1)
)

func parseArgs() {
	var gateIdArg int
	flag.IntVar(&gateIdArg, "gid", 0, "set gateid")
	flag.StringVar(&args.configFile, "configfile", "", "set config file path")
	flag.StringVar(&args.logLevel, "log", "", "set log level, will override log level in config")
	flag.BoolVar(&args.runInDaemonMode, "d", false, "run in daemon mode")
	//flag.StringVar(&args.listenAddr, "listen-addr", "", "set listen address for gate, overriding listen_addr in config file")
	flag.Parse()
	args.gateid = uint16(gateIdArg)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	parseArgs()

	if args.runInDaemonMode {
		daemoncontext := binutil.Daemonize()
		defer daemoncontext.Release()
	}

	if args.configFile != "" {
		config.SetConfigFile(args.configFile)
	}

	if args.gateid <= 0 {
		gwlog.Errorf("gateid %d is not valid, should be positive", args.gateid)
		os.Exit(1)
	}

	gateConfig := config.GetGate(args.gateid)
	verifyGateConfig(gateConfig)
	if gateConfig.GoMaxProcs > 0 {
		gwlog.Infof("SET GOMAXPROCS = %d", gateConfig.GoMaxProcs)
		runtime.GOMAXPROCS(gateConfig.GoMaxProcs)
	}
	logLevel := args.logLevel
	if logLevel == "" {
		logLevel = gateConfig.LogLevel
	}
	binutil.SetupGWLog(fmt.Sprintf("gate%d", args.gateid), logLevel, gateConfig.LogFile, gateConfig.LogStderr)

	gateService = newGateService()
	if gateConfig.EncryptConnection {
		cfgdir := config.GetConfigDir()
		rsaCert := path.Join(cfgdir, gateConfig.RSACertificate)
		rsaKey := path.Join(cfgdir, gateConfig.RSAKey)
		binutil.SetupHTTPServerTLS(gateConfig.HTTPAddr, gateService.handleWebSocketConn, rsaCert, rsaKey)
	} else {
		binutil.SetupHTTPServer(gateConfig.HTTPAddr, gateService.handleWebSocketConn)
	}

	dispatchercluster.Initialize(args.gateid, dispatcherclient.GateDispatcherClientType, false, false, &gateDispatcherClientDelegate{})
	//dispatcherclient.Initialize(&gateDispatcherClientDelegate{}, true)
	setupSignals()
	gateService.run() // run gate service in another goroutine
}

func verifyGateConfig(gateConfig *config.GateConfig) {
}

func setupSignals() {
	gwlog.Infof("Setup signals ...")
	signal.Ignore(syscall.Signal(10), syscall.Signal(12), syscall.SIGPIPE, syscall.SIGHUP)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			sig := <-signalChan
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				// terminating gate ...
				gwlog.Infof("Terminating gate service ...")
				post.Post(func() {
					gateService.terminate()
				})

				gateService.terminated.Wait()
				gwlog.Infof("Gate %d terminated gracefully.", args.gateid)
				os.Exit(0)
			} else {
				gwlog.Errorf("unexpected signal: %s", sig)
			}
		}
	}()
}

type gateDispatcherClientDelegate struct {
}

func (delegate *gateDispatcherClientDelegate) HandleDispatcherClientPacket(msgtype proto.MsgType, packet *netutil.Packet) {
	gateService.dispatcherClientPacketQueue <- proto.Message{msgtype, packet}
}

func (delegate *gateDispatcherClientDelegate) HandleDispatcherClientDisconnect() {
	//gwlog.Errorf("Disconnected from dispatcher, try reconnecting ...")
	// if gate is disconnected from dispatcher, we just quit
	gwlog.Infof("Disconnected from dispatcher, gate has to quit.")
	signalChan <- syscall.SIGTERM // let gate quit
}

func (deleget *gateDispatcherClientDelegate) GetEntityIDsForDispatcher(dispid uint16) []common.EntityID {
	return nil
}