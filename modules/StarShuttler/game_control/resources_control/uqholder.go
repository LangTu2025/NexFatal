package resources_control

import (
	"github.com/xingbaiawa/StarShuttler/core/minecraft"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/uqholder"
)

// UQHolder 是简单的 MicroUQHolder 维护器
type UQHolder struct {
	conn  *minecraft.Conn
	micro *uqholder.MicroUQHolder
}

// NewUQHolder 创建并返回一个新的 UQHolder
func NewUQHolder(conn *minecraft.Conn) *UQHolder {
	micro := uqholder.NewMicroUQHolder(conn)
	uq := &UQHolder{
		conn:  conn,
		micro: micro,
	}
	return uq
}

// Micro 返回 MicroUQHolder
func (uq *UQHolder) Micro() *uqholder.MicroUQHolder {
	return uq.micro
}

// onAnyPacket ..
func (uq *UQHolder) onAnyPacket(pk packet.Packet) {
	uq.Micro().UpdateFromPacket(pk)
}
