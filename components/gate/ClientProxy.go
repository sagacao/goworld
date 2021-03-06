package main

import (
	"fmt"

	"github.com/xiaonanln/goTimer"

	"time"

	"github.com/sagacao/goworld/engine/common"
	"github.com/sagacao/goworld/engine/config"
	"github.com/sagacao/goworld/engine/consts"
	"github.com/sagacao/goworld/engine/gwioutil"
	"github.com/sagacao/goworld/engine/gwlog"
	"github.com/sagacao/goworld/engine/netutil"
	"github.com/sagacao/goworld/engine/post"
	"github.com/sagacao/goworld/engine/proto"
)

type clientSyncInfo struct {
	EntityID common.EntityID
	X, Y, Z  float32
	Yaw      float32
}

func (info *clientSyncInfo) IsEmpty() bool {
	return info.EntityID == ""
}

// ClientProxy is a game client connections managed by gate
type ClientProxy struct {
	*proto.GoWorldConnection
	clientid       common.ClientID
	filterProps    map[string]string
	clientSyncInfo clientSyncInfo
	heartbeatTime  time.Time
	ownerEntityID  common.EntityID // owner entity's ID
	heartTimer     *timer.Timer
}

func newClientProxy(conn netutil.Connection, cfg *config.GateConfig) *ClientProxy {
	gwc := proto.NewGoWorldConnection(netutil.NewBufferedConnection(conn), cfg.CompressConnection, cfg.CompressFormat)
	return &ClientProxy{
		GoWorldConnection: gwc,
		clientid:          common.GenClientID(), // each client has its unique clientid
		filterProps:       map[string]string{},
		heartbeatTime:     time.Now(),
	}
}

func (cp *ClientProxy) String() string {
	return fmt.Sprintf("ClientProxy<%s@%s>", cp.clientid, cp.RemoteAddr())
}

func (cp *ClientProxy) heartbeatTimer(heartInterval time.Duration) {
	cp.heartTimer = timer.AddTimer(time.Second, func() {
		now := time.Now()
		// gwlog.Debugf("heartbeatTimer %s timer ...", cp)
		if cp.heartbeatTime.Add(heartInterval).Before(now) {
			gwlog.Infof("Connection %s timeout ...", cp)
			cp.Close()
		}
	})
}

// func (cp *ClientProxy) check() {
// 	now := time.Now()
// 	gwlog.Debugf("heartbeatTimer %s timer ...", cp)
// 	//if cp.heartbeatTime.Add(gs.checkHeartbeatsInterval).Before(now) {
// 	if cp.heartbeatTime.Add(60).Before(now) {
// 		gwlog.Infof("Connection %s timeout ...", cp)
// 		cp.Close()
// 	}
// }

// func (cp *ClientProxy) Destory() {
// 	cp.closeChan <- true
// 	close(cp.closeChan)

// 	cp.Close()
// }

//func (cp *ClientProxy) SendPacket(packet *netutil.Packet) error {
//	err := cp.GoWorldConnection.SendPacket(packet)
//	if err != nil {
//		return err
//	}
//	return cp.Flush("ClientProxy")
//}

func (cp *ClientProxy) serve() {
	defer func() {
		cp.heartTimer.Cancel()
		cp.Close()
		// tell the gate service that this client is down
		post.Post(func() {
			gateService.onClientProxyClose(cp)
		})

		if err := recover(); err != nil && !netutil.IsConnectionError(err.(error)) {
			gwlog.TraceError("%s error: %s", cp, err.(error))
		} else {
			gwlog.Debugf("%s disconnected", cp)
		}
	}()

	cp.SetAutoFlush(consts.CLIENT_PROXY_WRITE_FLUSH_INTERVAL)
	//cp.SendSetClientClientID(cp.cp) // set the cp on the client side

	for {
		var msgtype proto.MsgType
		pkt, err := cp.Recv(&msgtype)
		if pkt != nil {
			gateService.clientPacketQueue <- clientProxyMessage{cp, proto.Message{msgtype, pkt}}
		} else if err != nil && !gwioutil.IsTimeoutError(err) {
			if netutil.IsConnectionError(err) {
				break
			} else {
				panic(err)
			}
		}
	}
}
