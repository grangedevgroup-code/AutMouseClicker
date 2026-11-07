//go:build linux

package main

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const instancePath = dbus.ObjectPath("/io/github/grangedevgroup_code/AutMouseClicker")

type instance struct {
	conn       *dbus.Conn
	mu         sync.Mutex
	onActivate func()
}

func claimInstance() (*instance, bool) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return &instance{}, true
	}
	inst := &instance{conn: conn}
	if err := conn.Export(inst, instancePath, appID); err != nil {
		conn.Close()
		return &instance{}, true
	}
	reply, err := conn.RequestName(appID, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return &instance{}, true
	}
	if reply == dbus.RequestNameReplyPrimaryOwner || reply == dbus.RequestNameReplyAlreadyOwner {
		return inst, true
	}
	conn.Object(appID, instancePath).Call(appID+".Activate", 0)
	conn.Close()
	return nil, false
}

func (i *instance) Activate() *dbus.Error {
	i.mu.Lock()
	activate := i.onActivate
	i.mu.Unlock()
	if activate != nil {
		activate()
	}
	return nil
}

func (i *instance) OnActivate(f func()) {
	i.mu.Lock()
	i.onActivate = f
	i.mu.Unlock()
}

func (i *instance) Close() {
	if i.conn != nil {
		i.conn.Close()
	}
}
