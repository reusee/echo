// +build ignore

package main

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"sync/atomic"
	"time"
)

func main() {
	var n int64
	const payload = "foobar"
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			buf := make([]byte, len(payload))
			for {
				conn, err := net.Dial("tcp", "localhost:8888")
				if err != nil {
					panic(err)
				}
				if _, err := conn.Write([]byte(payload)); err != nil {
					panic(err)
				}
				if _, err := io.ReadFull(conn, buf); err != nil {
					panic(err)
				}
				conn.Close()
				atomic.AddInt64(&n, 1)
			}
		}()
	}
	for range time.NewTicker(time.Second * 3).C {
		c := atomic.SwapInt64(&n, 0)
		fmt.Printf("%d\n", c)
	}
}
