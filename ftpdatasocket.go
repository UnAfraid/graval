package graval

import (
	"errors"
	"math/rand"
	"net"
	"strconv"
	"time"
)

// A data socket is used to send non-control data between the client and
// server.
type ftpDataSocket interface {
	Host() string

	Port() uint16

	// the standard io.Reader interface
	Read(p []byte) (n int, err error)

	// the standard io.Writer interface
	Write(p []byte) (n int, err error)

	// the standard io.Closer interface
	Close() error
}

type ftpActiveSocket struct {
	conn   *net.TCPConn
	host   string
	port   uint16
	logger FTPLogger
}

func newActiveSocket(host string, port uint16, logger FTPLogger) (*ftpActiveSocket, error) {
	connectTo := buildTcpString(host, port)
	if logger != nil {
		logger.Debug("Opening active data connection to ", connectTo)
	}

	remoteAddress, err := net.ResolveTCPAddr("tcp", connectTo)
	if err != nil {
		return nil, err
	}

	tcpConn, err := net.DialTCP("tcp", nil, remoteAddress)
	if err != nil {
		return nil, err
	}

	socket := new(ftpActiveSocket)
	socket.conn = tcpConn
	socket.host = host
	socket.port = port
	socket.logger = logger
	return socket, nil
}

func (socket *ftpActiveSocket) Host() string {
	return socket.host
}

func (socket *ftpActiveSocket) Port() uint16 {
	return socket.port
}

func (socket *ftpActiveSocket) Read(p []byte) (n int, err error) {
	return socket.conn.Read(p)
}

func (socket *ftpActiveSocket) Write(p []byte) (n int, err error) {
	return socket.conn.Write(p)
}

func (socket *ftpActiveSocket) Close() error {
	return socket.conn.Close()
}

type ftpPassiveSocket struct {
	conn     *net.TCPConn
	port     uint16
	listenIP string
	logger   FTPLogger
}

func newPassiveSocket(listenIP string, minPort uint16, maxPort uint16, logger FTPLogger) (*ftpPassiveSocket, error) {
	socket := new(ftpPassiveSocket)
	socket.logger = logger
	socket.listenIP = listenIP
	go socket.ListenAndServe(minPort, maxPort)
	for {
		if socket.Port() > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return socket, nil
}

func (socket *ftpPassiveSocket) Host() string {
	return socket.listenIP
}

func (socket *ftpPassiveSocket) Port() uint16 {
	return socket.port
}

func (socket *ftpPassiveSocket) Read(p []byte) (n int, err error) {
	if socket.waitForOpenSocket() == false {
		return 0, errors.New("data socket unavailable")
	}
	return socket.conn.Read(p)
}

func (socket *ftpPassiveSocket) Write(p []byte) (n int, err error) {
	if socket.waitForOpenSocket() == false {
		return 0, errors.New("data socket unavailable")
	}
	return socket.conn.Write(p)
}

func (socket *ftpPassiveSocket) Close() error {
	if socket.logger != nil {
		socket.logger.Debug("closing passive data socket")
	}
	if socket.conn != nil {
		return socket.conn.Close()
	}
	return nil
}

func (socket *ftpPassiveSocket) ListenAndServe(minPort, maxPort uint16) error {
	listener, err := socket.netListenerInRange(minPort, maxPort)
	if err != nil {
		return err
	}
	defer listener.Close()

	add := listener.Addr().(*net.TCPAddr)
	socket.port = uint16(add.Port)

	tcpConn, err := listener.AcceptTCP()
	if err != nil {
		return err
	}

	socket.conn = tcpConn
	return nil
}

func (socket *ftpPassiveSocket) waitForOpenSocket() bool {
	retries := 0
	for {
		if socket.conn != nil {
			break
		}
		if retries > 3 {
			return false
		}
		if socket.logger != nil {
			socket.logger.Debug("sleeping, socket isn't open")
		}
		duration := time.Duration(500*(retries+1)) * time.Millisecond
		time.Sleep(duration)
		retries += 1
	}
	return true
}

func (socket *ftpPassiveSocket) netListenerInRange(min, max uint16) (*net.TCPListener, error) {
	for retries := 1; retries < 100; retries++ {
		port := randomPort(min, max)
		l, err := net.Listen("tcp", net.JoinHostPort(socket.Host(), strconv.Itoa(int(port))))
		if err == nil {
			return l.(*net.TCPListener), nil
		}
	}
	return nil, errors.New("unable to find available port to listen on")
}

func randomPort(min, max uint16) uint16 {
	if min == 0 && max == 0 {
		return 0
	} else {
		return uint16(int(min) + rand.Intn(int(max-min-1)))
	}
}
