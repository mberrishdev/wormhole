package tunnel

import "io"

func Pipe(a, b io.ReadWriteCloser) {
	done := make(chan error, 2)
	copyHalf := func(dst, src io.ReadWriteCloser) {
		_, err := io.Copy(dst, src)
		if half, ok := dst.(interface{ CloseWrite() error }); ok {
			half.CloseWrite()
		} else {
			dst.Close()
		}
		done <- err
	}
	go copyHalf(a, b)
	go copyHalf(b, a)
	if err := <-done; err != nil {
		a.Close()
		b.Close()
	}
	<-done
	a.Close()
	b.Close()
}
