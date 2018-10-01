package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	pt = fmt.Printf
	ce = func(err error) {
		if err != nil {
			panic(err)
		}
	}
)

func main() {

	go func() {
		ce(http.ListenAndServe(":8899", nil))
	}()

	start := func() {
		runtime.LockOSThread()

		sock, err := syscall.Socket(
			syscall.AF_INET,
			syscall.SOCK_STREAM,
			0,
		)
		ce(err)
		ce(syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1))
		ce(syscall.Bind(sock, &syscall.SockaddrInet4{
			Port: 8888,
			Addr: [4]byte{127, 0, 0, 1},
		}))
		ce(syscall.Listen(sock, 65536))
		defer syscall.Close(sock)

		epoll, err := syscall.EpollCreate1(0)
		ce(err)
		defer syscall.Close(epoll)

		ce(syscall.EpollCtl(epoll, syscall.EPOLL_CTL_ADD, sock, &syscall.EpollEvent{
			Events: syscall.EPOLLIN,
			Fd:     int32(sock),
		}))

		events := make([]syscall.EpollEvent, 1024)
		var fdLimit syscall.Rlimit
		ce(syscall.Getrlimit(syscall.RLIMIT_NOFILE, &fdLimit))
		conns := make([]*Conn, fdLimit.Max)
		for {
			n, err := syscall.EpollWait(epoll, events, -1)
			ce(err)

			for _, ev := range events[:n] {

				if ev.Events&syscall.EPOLLIN > 0 {
					if ev.Fd == int32(sock) {
						fd, _, err := syscall.Accept(sock)
						if err != nil {
							pt("accept: %v\n", err)
						}
						e := &syscall.EpollEvent{
							Events: syscall.EPOLLIN | syscall.EPOLLHUP | syscall.EPOLLRDHUP | syscall.EPOLLERR,
							Fd:     int32(fd),
						}
						ce(syscall.EpollCtl(epoll, syscall.EPOLL_CTL_ADD, fd, e))
						conns[e.Fd] = NewConn(e)

					} else {
						if conn := conns[ev.Fd]; conn != nil {
							n, err := syscall.Read(int(ev.Fd), conn.Buf)
							if err != nil {
								pt("read: %v\n", err)
								continue
							}
							conn.Feed(conn.Buf[:n])
						}

					}

				}

				if ev.Events&(syscall.EPOLLERR|syscall.EPOLLHUP|syscall.EPOLLRDHUP) > 0 {
					if conn := conns[ev.Fd]; conn != nil {
						ce(syscall.EpollCtl(epoll, syscall.EPOLL_CTL_DEL, int(ev.Fd), conn.Ev))
						conns[ev.Fd] = nil
						bufPool.Put(conn.Buf)
					}
					syscall.Close(int(ev.Fd))
				}

			}
		}
	}

	for i := 0; i < runtime.NumCPU(); i++ {
		go start()
	}

	select {}

}

type Conn struct {
	Ev  *syscall.EpollEvent
	Buf []byte
}

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 1024)
	},
}

func NewConn(ev *syscall.EpollEvent) *Conn {
	return &Conn{
		Ev:  ev,
		Buf: bufPool.Get().([]byte),
	}
}

func (c *Conn) Feed(data []byte) {
	if _, err := syscall.Write(int(c.Ev.Fd), data); err != nil {
		pt("write: %v\n", err)
	}
}
