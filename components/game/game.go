package game

import (
	"flag"

	"math/rand"
	"time"

	"os"

	// for go tool pprof
	_ "net/http/pprof"

	"runtime"

	"os/signal"

	"syscall"

	"fmt"

	"context"

	"github.com/sagacao/goworld/components/game/lbc"
	"github.com/sagacao/goworld/engine/binutil"
	"github.com/sagacao/goworld/engine/common"
	"github.com/sagacao/goworld/engine/config"
	"github.com/sagacao/goworld/engine/crontab"
	"github.com/sagacao/goworld/engine/dispatchercluster"
	"github.com/sagacao/goworld/engine/dispatchercluster/dispatcherclient"
	"github.com/sagacao/goworld/engine/entity"
	"github.com/sagacao/goworld/engine/gwlog"
	"github.com/sagacao/goworld/engine/kvdb"
	"github.com/sagacao/goworld/engine/netutil"
	"github.com/sagacao/goworld/engine/post"
	"github.com/sagacao/goworld/engine/proto"
	"github.com/sagacao/goworld/engine/service"
	"github.com/sagacao/goworld/engine/storage"
)

var (
	gameid          uint16
	configFile      string
	logLevel        string
	restore         bool
	runInDaemonMode bool
	gameService     *GameService
	signalChan      = make(chan os.Signal, 1)
	gameCtx         = context.Background()
)

func parseArgs() {
	var gameidArg int
	flag.IntVar(&gameidArg, "gid", 0, "set gameid")
	flag.StringVar(&configFile, "configfile", "", "set config file path")
	flag.StringVar(&logLevel, "log", "", "set log level, will override log level in config")
	flag.BoolVar(&restore, "restore", false, "restore from freezed state")
	flag.BoolVar(&runInDaemonMode, "d", false, "run in daemon mode")
	flag.Parse()
	gameid = uint16(gameidArg)
}

// Run runs the game server
//
// This is the main game server loop
func Run() {
	rand.Seed(time.Now().UnixNano())
	parseArgs()

	if runInDaemonMode {
		daemoncontext := binutil.Daemonize()
		defer daemoncontext.Release()
	}

	if configFile != "" {
		config.SetConfigFile(configFile)
	}

	if gameid <= 0 {
		gwlog.Errorf("gameid %d is not valid, should be positive", gameid)
		os.Exit(1)
	}

	gameConfig := config.GetGame(gameid)
	if gameConfig == nil {
		gwlog.Errorf("game %d's config is not found", gameid)
		os.Exit(1)
	}

	if gameConfig.GoMaxProcs > 0 {
		gwlog.Infof("SET GOMAXPROCS = %d", gameConfig.GoMaxProcs)
		runtime.GOMAXPROCS(gameConfig.GoMaxProcs)
	}
	if logLevel == "" {
		logLevel = gameConfig.LogLevel
	}
	binutil.SetupGWLog(fmt.Sprintf("game%d", gameid), logLevel, gameConfig.LogFile, gameConfig.LogStderr)

	gwlog.Infof("Initializing storage ...")
	storage.Initialize()
	gwlog.Infof("Initializing KVDB ...")
	kvdb.Initialize()
	gwlog.Infof("Initializing crontab ...")
	crontab.Initialize()

	gwlog.Infof("Setup http server ...")
	binutil.SetupHTTPServer(gameConfig.HTTPAddr, nil)

	entity.SetSaveInterval(gameConfig.SaveInterval)

	gwlog.Infof("Start game service ...")
	gameService = newGameService(gameid)

	if !restore {
		gwlog.Infof("Creating nil space ...")
		entity.CreateNilSpace(gameid) // create the nil space
	} else {
		// restoring from freezed states
		gwlog.Infof("Restoring freezed entities ...")
		err := restoreFreezedEntities()
		if err != nil {
			gwlog.Fatalf("Restore from freezed states failed: %+v", err)
		}
	}

	gwlog.Infof("Start dispatchercluster ...")
	dispatchercluster.Initialize(gameid, dispatcherclient.GameDispatcherClientType, restore, gameConfig.BanBootEntity, &_GameDispatcherClientDelegate{})

	gamelbc.Initialize(gameCtx, time.Second*1)

	setupSignals()

	service.Setup(gameid)
	gwlog.Infof("Game service start running ...")
	gameService.run()
}

func setupSignals() {
	gwlog.Infof("Setup signals ...")
	signal.Ignore(syscall.Signal(12), syscall.SIGPIPE, syscall.Signal(10))
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, binutil.FreezeSignal)

	go func() {
		for {
			sig := <-signalChan
			if sig == syscall.SIGTERM || sig == syscall.SIGINT {
				// terminating game ...
				gwlog.Infof("Terminating game service ...")
				gameService.terminate()
				waitGameServiceStateSatisfied(func(rs int) bool {
					return rs != rsTerminating
				})
				if gameService.runState.Load() != rsTerminated {
					// game service is not terminated successfully, abort
					gwlog.Errorf("Game service is not terminated successfully, back to running ...")
					continue
				}

				waitEntityStorageFinish()

				gwlog.Infof("Game %d shutdown gracefully.", gameid)
				os.Exit(0)
			} else if sig == binutil.FreezeSignal {
				// SIGHUP => dump game and close
				// freezing game ...
				gwlog.Infof("Freezing game service ...")

				post.Post(func() {
					gameService.startFreeze()
				})

				waitGameServiceStateSatisfied(func(rs int) bool { // wait until not running
					return rs != rsRunning
				})
				waitGameServiceStateSatisfied(func(rs int) bool {
					return rs != rsFreezing
				})

				if gameService.runState.Load() != rsFreezed {
					// game service is not freezed successfully, abort
					gwlog.Errorf("Game service is not freezed successfully, back to running ...")
					continue
				}

				waitEntityStorageFinish()

				gwlog.Infof("Game %d freezed gracefully.", gameid)
				os.Exit(0)
			} else {
				gwlog.Errorf("unexpected signal: %s", sig)
			}
		}
	}()
}

func waitGameServiceStateSatisfied(s func(rs int) bool) {
	waitCounter := 0
	for {
		state := gameService.runState.Load()
		if s(state) {
			break
		}
		waitCounter++
		if waitCounter%100 == 0 {
			gwlog.Infof("game service status: %d", state)
		}
		time.Sleep(time.Millisecond * 10)
	}
}

func waitEntityStorageFinish() {
	// wait until entity storage's queue is empty
	gwlog.Infof("Closing Entity Storage ...")
	storage.Shutdown()
	gwlog.Infof("*** DB OK ***")
}

type _GameDispatcherClientDelegate struct {
}

var lastWarnGateServiceQueueLen = 0

func (delegate *_GameDispatcherClientDelegate) HandleDispatcherClientPacket(msgtype proto.MsgType, packet *netutil.Packet) {
	gameService.packetQueue <- proto.Message{ // may block the dispatcher client routine
		MsgType: msgtype,
		Packet:  packet,
	}
}

func (delegate *_GameDispatcherClientDelegate) HandleDispatcherClientDisconnect() {
	gwlog.Errorf("Disconnected from dispatcher, try reconnecting ...")
}

func (delegate *_GameDispatcherClientDelegate) GetEntityIDsForDispatcher(dispid uint16) (eids []common.EntityID) {
	for eid := range entity.Entities() {
		if dispatchercluster.EntityIDToDispatcherID(eid) == dispid {
			eids = append(eids, eid)
		}
	}
	return
}

// GetGameID returns the current Game Server ID
func GetGameID() uint16 {
	return gameid
}
