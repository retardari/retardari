package net

// udp必须维持心跳 心跳间隔默认未10秒 心跳超时则跑去
// 数据包的间隔时间为5秒，超时则丢弃
import (
	"net"
	"sync"
)

type UDPServer struct {
	Addr            string
	MaxConnNum      int
	PendingWriteNum int
	ln              net.Listener
	mutexConns      sync.Mutex
	LenMsgLen       int
	MinMsgLen       uint32
	MaxMsgLen       uint32
	LittleEndian    bool
}
