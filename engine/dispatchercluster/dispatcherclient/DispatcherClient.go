package dispatcherclient

import (
	"net"

	"github.com/sagacao/goworld/engine/consts"
	"github.com/sagacao/goworld/engine/gwlog"
	"github.com/sagacao/goworld/engine/netutil"
	"github.com/sagacao/goworld/engine/proto"
)

type DispatcherClientType int

const (
	GameDispatcherClientType DispatcherClientType = 1 + iota
	GateDispatcherClientType
)

// DispatcherClient is a client connection to the dispatcher
type DispatcherClient struct {
	*proto.GoWorldConnection
	dctype        DispatcherClientType
	isReconnect   bool
	isRestoreGame bool
}

func newDispatcherClient(dctype DispatcherClientType, conn net.Conn, isReconnect bool, isRestoreGame bool) *DispatcherClient {
	gwc := proto.NewGoWorldConnection(netutil.NewBufferedConnection(netutil.NetConnection{conn}), false, "")
	if dctype != GameDispatcherClientType && dctype != GateDispatcherClientType {
		gwlog.Fatalf("invalid dispatcher client type: %v", dctype)
	}

	dc := &DispatcherClient{
		GoWorldConnection: gwc,
		dctype:            dctype,
		isReconnect:       isReconnect,
		isRestoreGame:     isRestoreGame,
	}
	dc.SetAutoFlush(consts.DISPATCHER_CLIENT_FLUSH_INTERVAL)
	return dc
}

// Close the dispatcher client
func (dc *DispatcherClient) Close() error {
	return dc.GoWorldConnection.Close()
}
