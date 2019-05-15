package srvdis

import (
	"testing"

	"github.com/sagacao/goworld/engine/gwlog"
)

func init() {
	gwlog.Infof("init")
}

func TestOver(t *testing.T) {
	gwlog.Infof("fini")
}
