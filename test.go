package main

import (
	"fmt"
	"net"
)

var limitChan = make(chan bool, 1000)

func main() {
	var laddr *net.UDPAddr
	laddr, err := net.ResolveUDPAddr("udp", ":55223")
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		fmt.Println(err.Error())
	}
	go func() {
		for {
			limitChan <- true
			go func(conn *net.UDPConn) {
				var rbuf [1024]byte
				for {
					_, _, err := conn.ReadFromUDP(rbuf[0:])
					if err != nil {
						fmt.Println(err.Error())
					}
				}
				<-limitChan
			}(conn)
		}
	}()
	dc()
}

func dc() {
	var raddr *net.UDPAddr
	raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:55223")
	conn, err := net.DialUDP("udp", nil, raddr)

	if err != nil {
		fmt.Println(err.Error())
	}

	i := 1
	for {
		go func() {
			_, err = conn.Write([]byte("ARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREAREARE-U-THEREARE-U-THEREARE-U-THEREARE"))
			if err != nil {
				fmt.Println(err.Error())
			}
			i++
		}()
	}
}
