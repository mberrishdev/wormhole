package tunnel

import (
	"io"
	"net"
	"sync"
)

func Pipe(connA net.Conn, connB net.Conn) {

	defer connA.Close()
	defer connB.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(connA, connB)
	}()

	go func() {
		defer wg.Done()
		io.Copy(connB, connA)
	}()

	wg.Wait()
}
