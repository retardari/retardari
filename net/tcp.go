package net

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

type ConnSet map[net.Conn]struct{}

type TCPServer struct {
	Addr            string
	MaxConnNum      int
	PendingWriteNum int
	NewAgent        func(*TCPConn) Agent
	ln              net.Listener
	conns           ConnSet
	mutexConns      sync.Mutex
	wgLn            sync.WaitGroup
	wgConns         sync.WaitGroup
	LenMsgLen       int
	MinMsgLen       uint32
	MaxMsgLen       uint32
	LittleEndian    bool
	msgParser       *MsgParser
}

func (server *TCPServer) init() {
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatal("%v", err)
	}

	if server.MaxConnNum <= 0 {
		server.MaxConnNum = 100
		log.Printf("invalid MaxConnNum, reset to %v", server.MaxConnNum)
	}
	if server.PendingWriteNum <= 0 {
		server.PendingWriteNum = 100
		log.Printf("invalid PendingWriteNum, reset to %v", server.PendingWriteNum)
	}
	if server.NewAgent == nil {
		log.Fatal("NewAgent must not be nil")
	}

	server.ln = ln
	server.conns = make(ConnSet)

	//msg parser
	msgParser := NewMsgParser()
	msgParser.SetMsgLen(server.LenMsgLen, server.MinMsgLen, server.MaxMsgLen)
	msgParser.SetByteOrder(server.LittleEndian)
	server.msgParser = msgParser
}

// 开启server监听
func (server *TCPServer) run() {
	server.wgLn.Add(1)
	defer server.wgLn.Done()

	var tempDelay time.Duration
	for {
		conn, err := server.ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Printf("accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		tempDelay = 0

		server.mutexConns.Lock()
		// 连接数超过则直接关闭
		if len(server.conns) >= server.MaxConnNum {
			server.mutexConns.Unlock()
			err := conn.Close()
			if err != nil {
				log.Printf("too many connections close err %v", err)
			}
			log.Printf("too many connections")
			continue
		}
		server.conns[conn] = struct{}{}
		server.mutexConns.Unlock()

		server.wgConns.Add(1)

		tcpConn := newTCPConn(conn, server.PendingWriteNum, server.msgParser)
		// 创建agent代理维持tcp通信。
		agent := server.NewAgent(tcpConn)
		go func() {
			agent.Run()
			// cleanup
			tcpConn.Close()
			server.mutexConns.Lock()
			delete(server.conns, conn)
			server.mutexConns.Unlock()
			agent.OnClose()

			server.wgConns.Done()
		}()
	}
}

// 关闭tcp server
func (server *TCPServer) Close() {
	// 停止监听
	err := server.ln.Close()
	if err != nil {
		log.Printf("close tcp server err %v", err)
	}
	server.wgLn.Wait()

	server.mutexConns.Lock()
	// 断开tcp连接
	for conn := range server.conns {
		if err = conn.Close(); err != nil {
			log.Printf("close tcp server stop conns err %v", err)
		}
	}
	server.conns = nil
	server.mutexConns.Unlock()
	server.wgConns.Wait()
}

// tcp连接
// 缓冲写入数据 管理连接生命周期
type TCPConn struct {
	sync.Mutex
	conn      net.Conn
	writeChan chan []byte
	closeFlag bool
	msgParser *MsgParser
}

// 新建tcp连接
func newTCPConn(conn net.Conn, pendingWriteNum int, msgParser *MsgParser) *TCPConn {
	tcpConn := new(TCPConn)
	tcpConn.conn = conn
	tcpConn.writeChan = make(chan []byte, pendingWriteNum)
	tcpConn.msgParser = msgParser

	go func() {
		for b := range tcpConn.writeChan {
			if b == nil {
				break
			}
			_, err := conn.Write(b)
			if err != nil {
				break
			}
		}

		conn.Close()
		tcpConn.Lock()
		tcpConn.closeFlag = true
		tcpConn.Unlock()
	}()

	return tcpConn
}

// 强制结束连接
func (tcpConn *TCPConn) doDestroy() {
	_ = tcpConn.conn.(*net.TCPConn).SetLinger(0)
	_ = tcpConn.conn.Close()
	if !tcpConn.closeFlag {
		close(tcpConn.writeChan)
		tcpConn.closeFlag = true
	}
}

// 强制结束连接，并发安全
func (tcpConn *TCPConn) Destroy() {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	tcpConn.doDestroy()
}

// 关闭连接
func (tcpConn *TCPConn) Close() {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	if tcpConn.closeFlag {
		return
	}

	tcpConn.doWrite(nil)
	tcpConn.closeFlag = true
}

// 强制写入，在缓冲池满的情况链接将断开
func (tcpConn *TCPConn) doWrite(b []byte) {
	if len(tcpConn.writeChan) == cap(tcpConn.writeChan) {
		log.Printf("close conn: channel full")
		tcpConn.doDestroy()
		return
	}
	tcpConn.writeChan <- b
}

// 写入数据
func (tcpConn *TCPConn) Write(b []byte) {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	if tcpConn.closeFlag || b == nil {
		return
	}

	tcpConn.doWrite(b)
}

// 从conn中读取数据
func (tcpConn *TCPConn) Read(b []byte) (int, error) {
	return tcpConn.conn.Read(b)
}

// 从conn中读取本地地址
func (tcpConn *TCPConn) LocalAddr() net.Addr {
	return tcpConn.conn.LocalAddr()
}

// 从conn中读取对方地址
func (tcpConn *TCPConn) RemoteAddr() net.Addr {
	return tcpConn.conn.RemoteAddr()
}

// 读取消息数据
func (tcpConn *TCPConn) ReadMsg() ([]byte, error) {
	return tcpConn.msgParser.Read(tcpConn)
}

// 写入数据
func (tcpConn *TCPConn) WriteMsg(args ...[]byte) error {
	return tcpConn.msgParser.Write(tcpConn, args...)
}

// lm格式消息
// 处理TCP粘包
// 消息格式 [len:data]
type MsgParser struct {
	lenMsgLen    int
	minMsgLen    uint32
	maxMsgLen    uint32
	littleEndian bool
}

func NewMsgParser() *MsgParser {
	p := new(MsgParser)
	p.lenMsgLen = 2
	p.minMsgLen = 1
	p.maxMsgLen = 4096
	p.littleEndian = false
	return p
}

// It's dangerous to call the method on reading or writing
func (p *MsgParser) SetMsgLen(lenMsgLen int, minMsgLen uint32, maxMsgLen uint32) {
	if lenMsgLen == 1 || lenMsgLen == 2 || lenMsgLen == 4 {
		p.lenMsgLen = lenMsgLen
	}
	if minMsgLen != 0 {
		p.minMsgLen = minMsgLen
	}
	if maxMsgLen != 0 {
		p.maxMsgLen = maxMsgLen
	}

	var max uint32
	switch p.lenMsgLen {
	case 1:
		max = math.MaxUint8
	case 2:
		max = math.MaxUint16
	case 4:
		max = math.MaxUint32
	}
	if p.minMsgLen > max {
		p.minMsgLen = max
	}
	if p.maxMsgLen > max {
		p.maxMsgLen = max
	}
}

// 设置大小端 注意: 并发不可靠
func (p *MsgParser) SetByteOrder(littleEndian bool) {
	p.littleEndian = littleEndian
}

// 线程安全
func (p *MsgParser) Read(conn *TCPConn) ([]byte, error) {
	var b [4]byte
	bufMsgLen := b[:p.lenMsgLen]

	// read len
	if _, err := io.ReadFull(conn, bufMsgLen); err != nil {
		return nil, err
	}

	// parse len
	var msgLen uint32
	switch p.lenMsgLen {
	case 1:
		msgLen = uint32(bufMsgLen[0])
	case 2:
		if p.littleEndian {
			msgLen = uint32(binary.LittleEndian.Uint16(bufMsgLen))
		} else {
			msgLen = uint32(binary.BigEndian.Uint16(bufMsgLen))
		}
	case 4:
		if p.littleEndian {
			msgLen = binary.LittleEndian.Uint32(bufMsgLen)
		} else {
			msgLen = binary.BigEndian.Uint32(bufMsgLen)
		}
	}

	// check len
	if msgLen > p.maxMsgLen {
		return nil, errors.New("message too long")
	} else if msgLen < p.minMsgLen {
		return nil, errors.New("message too short")
	}

	// data
	msgData := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, msgData); err != nil {
		return nil, err
	}

	return msgData, nil
}

// 线程安全
func (p *MsgParser) Write(conn *TCPConn, args ...[]byte) error {
	// get len
	var msgLen uint32
	for i := 0; i < len(args); i++ {
		msgLen += uint32(len(args[i]))
	}

	// check len
	if msgLen > p.maxMsgLen {
		return errors.New("message too long")
	} else if msgLen < p.minMsgLen {
		return errors.New("message too short")
	}

	msg := make([]byte, uint32(p.lenMsgLen)+msgLen)

	// write len
	switch p.lenMsgLen {
	case 1:
		msg[0] = byte(msgLen)
	case 2:
		if p.littleEndian {
			binary.LittleEndian.PutUint16(msg, uint16(msgLen))
		} else {
			binary.BigEndian.PutUint16(msg, uint16(msgLen))
		}
	case 4:
		if p.littleEndian {
			binary.LittleEndian.PutUint32(msg, msgLen)
		} else {
			binary.BigEndian.PutUint32(msg, msgLen)
		}
	}

	// write data
	l := p.lenMsgLen
	for i := 0; i < len(args); i++ {
		copy(msg[l:], args[i])
		l += len(args[i])
	}

	conn.Write(msg)

	return nil
}
