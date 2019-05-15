package service

import (
	"fmt"
	"time"

	"strings"

	"strconv"

	"math/rand"

	"github.com/pkg/errors"
	"github.com/xiaonanln/goTimer"
	"github.com/sagacao/goworld/engine/common"
	"github.com/sagacao/goworld/engine/entity"
	"github.com/sagacao/goworld/engine/gwlog"
	"github.com/sagacao/goworld/engine/gwvar"
	"github.com/sagacao/goworld/engine/srvdis"
	"github.com/sagacao/goworld/engine/storage"
)

const (
	checkServicesInterval   = time.Second * 60
	serviceSrvdisPrefix     = "Service/"
	serviceSrvdisPrefixLen  = len(serviceSrvdisPrefix)
	checkServicesLaterDelay = time.Millisecond * 200
)

var (
	registeredServices = common.StringSet{}
	gameid             uint16
	serviceMap         = map[string]common.EntityID{} // ServiceName -> Entity ID
	checkTimer         *timer.Timer
)

func RegisterService(typeName string, entityPtr entity.IEntity) {
	entity.RegisterEntity(typeName, entityPtr, true)
	registeredServices.Add(typeName)
}

func Setup(gameid_ uint16) {
	gameid = gameid_
	srvdis.AddPostCallback(checkServicesLater)
}

func OnDeploymentReady() {
	timer.AddTimer(checkServicesInterval, checkServices)
	checkServicesLater()
}

type serviceInfo struct {
	Registered bool
	EntityID   common.EntityID
}

func checkServicesLater() {
	if checkTimer == nil || !checkTimer.IsActive() {
		checkTimer = timer.AddCallback(checkServicesLaterDelay, func() {
			checkTimer = nil
			checkServices()
		})
	}
}

func checkServices() {
	if !gwvar.IsDeploymentReady.Value() {
		// deployment is not ready
		return
	}
	gwlog.Infof("service: checking services ...")
	dispRegisteredServices := map[string]*serviceInfo{} // all services that are registered on dispatchers
	needLocalServiceEntities := common.StringSet{}
	newServiceMap := make(map[string]common.EntityID, len(registeredServices))

	getServiceInfo := func(serviceName string) *serviceInfo {
		info := dispRegisteredServices[serviceName]
		if info == nil {
			info = &serviceInfo{}
			dispRegisteredServices[serviceName] = info
		}
		return info
	}

	srvdis.TraverseByPrefix(serviceSrvdisPrefix, func(srvid string, srvinfo string) {
		servicePath := strings.Split(srvid[serviceSrvdisPrefixLen:], "/")
		//gwlog.Infof("service: found service %v = %+v", servicePath, srvinfo)

		if len(servicePath) == 1 {
			// ServiceName = gameX
			serviceName := servicePath[0]
			targetGameID, err := strconv.Atoi(srvinfo[4:])
			if err != nil {
				gwlog.Panic(errors.Wrap(err, "parse targetGameID failed"))
			}
			// XxxService = gameX
			getServiceInfo(serviceName).Registered = true

			if int(gameid) == targetGameID {
				needLocalServiceEntities.Add(serviceName)
			}
		} else if len(servicePath) == 2 {
			// ServiceName/EntityID = Xxxx
			serviceName := servicePath[0]
			fieldName := servicePath[1]
			switch fieldName {
			case "EntityID":
				getServiceInfo(serviceName).EntityID = common.EntityID(srvinfo)
			default:
				gwlog.Warnf("unknown srvdis info: %s = %s", srvid, srvinfo)
			}
		} else {
			gwlog.Panic(servicePath)
		}
	})

	for serviceName, info := range dispRegisteredServices {
		if info.Registered && !info.EntityID.IsNil() {
			newServiceMap[serviceName] = info.EntityID
		}
	}
	serviceMap = newServiceMap

	// destroy all service entities that is on this game, but is not verified by dispatcher
	for serviceName := range registeredServices {
		if !needLocalServiceEntities.Contains(serviceName) {
			// this service should not be local
			serviceEntities := entity.GetEntitiesByType(serviceName)
			for _, e := range serviceEntities {
				e.Destroy()
			}
		}
	}

	// create all service entities that should be created on this game
	for serviceName := range needLocalServiceEntities {
		serviceEntities := entity.GetEntitiesByType(serviceName)
		if len(serviceEntities) == 0 {
			createServiceEntity(serviceName)
		} else if len(serviceEntities) == 1 {
			// make sure the current service entity is the
			localEid := serviceEntities.Keys()[0]
			//gwlog.Infof("service %s: found service entity: %s, service info: %+v", serviceName, serviceEntities.Values()[0], getServiceInfo(serviceName))
			if localEid != getServiceInfo(serviceName).EntityID {
				// might happen if dispatchers recover from crash
				gwlog.Warnf("service %s: local entity is %s, but has %s on dispatchers", serviceName, localEid, getServiceInfo(serviceName).EntityID)
				srvdis.Register(getSrvID(serviceName)+"/EntityID", string(localEid), true)
			}
		} else {
			// multiple service entities ? should never happen! so just destroy all invalid service entities
			correctEid := getServiceInfo(serviceName).EntityID
			for _, e := range serviceEntities {
				if e.ID != correctEid {
					e.Destroy()
				}
			}
		}
	}

	// register all service types that are not registered to dispatcher yet
	for serviceName := range registeredServices {
		if !getServiceInfo(serviceName).Registered {
			gwlog.Warnf("service: %s not found, registering srvdis ...", serviceName)
			// delay for a random time so that each game might register servcie randomly
			randomDelay := time.Millisecond * time.Duration(rand.Intn(100))
			_serviceName := serviceName
			timer.AddCallback(randomDelay, func() {
				srvdis.Register(getSrvID(_serviceName), fmt.Sprintf("game%d", gameid), false)
			})
		}
	}
}
func createServiceEntity(serviceName string) {
	desc := entity.GetEntityTypeDesc(serviceName)
	if desc == nil {
		gwlog.Panicf("create service entity locally failed: service %s is not registered", serviceName)
	}

	if !desc.IsPersistent {
		e := entity.CreateEntityLocally(serviceName, nil)
		gwlog.Infof("Created service entity: %s: %s", serviceName, e)
		srvdis.Register(getSrvID(serviceName)+"/EntityID", string(e.ID), true)
	} else {
		createPersistentServiceEntity(serviceName)
	}
}

func createPersistentServiceEntity(serviceName string) {
	storage.ListEntityIDs(serviceName, func(ids []common.EntityID, err error) {
		if err != nil {
			gwlog.Panic(errors.Wrap(err, "storage.ListEntityIDs failed"))
		}

		if len(entity.GetEntitiesByType(serviceName)) > 0 {
			// service exists now
			gwlog.Warnf("Was creating service %s, but found existing: %v", serviceName, entity.GetEntitiesByType(serviceName))
			return
		}

		var eid common.EntityID
		if len(ids) == 0 {
			eid = entity.CreateEntityLocally(serviceName, nil).ID
			gwlog.Infof("Created service entity: %s: %s", serviceName, eid)
		} else {
			eid = ids[0]
			// try to load entity on the current game, but we need to tell dispatcher first
			entity.LoadEntityOnGame(serviceName, eid, gameid)
			gwlog.Infof("Loading service entity: %s: %s", serviceName, eid)
		}

		srvdis.Register(getSrvID(serviceName)+"/EntityID", string(eid), true)
	})
}

func getSrvID(serviceName string) string {
	return serviceSrvdisPrefix + serviceName
}

func CallService(serviceName string, method string, args []interface{}) {
	serviceEid := serviceMap[serviceName]
	if serviceEid.IsNil() {
		gwlog.Errorf("CallService %s.%s: service entity is not created yet!", serviceName, method)
		return
	}

	entity.Call(serviceEid, method, args)
}

func GetServiceEntityID(serviceName string) common.EntityID {
	return serviceMap[serviceName]
}
