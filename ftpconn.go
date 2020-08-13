package graval

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

type ftpConn struct {
	conn             net.Conn
	controlReader    *bufio.Reader
	controlWriter    *bufio.Writer
	dataConn         ftpDataSocket
	driver           FTPDriver
	logger           FTPLogger
	serverName       string
	sessionId        string
	namePrefix       string
	reqUser          string
	user             string
	renameFrom       string
	minDataPort      int
	maxDataPort      int
	pasvAdvertisedIp string
}

// NewftpConn constructs a new object that will handle the FTP protocol over
// an active net.TCPConn. The TCP connection should already be open before
// it is handed to this functions. driver is an instance of FTPDriver that
// will handle all auth and persistence details.
func newFtpConn(tcpConn net.Conn, driver FTPDriver, ftpLogger FTPLogger, serverName string, minPort int, maxPort int, pasvAdvertisedIp string) *ftpConn {
	c := new(ftpConn)
	c.namePrefix = "/"
	c.conn = tcpConn
	c.controlReader = bufio.NewReader(tcpConn)
	c.controlWriter = bufio.NewWriter(tcpConn)
	c.driver = driver
	c.sessionId = newSessionId()
	c.logger = ftpLogger
	c.serverName = serverName
	c.minDataPort = minPort
	c.maxDataPort = maxPort
	c.pasvAdvertisedIp = pasvAdvertisedIp
	return c
}

// returns a random 20 char string that can be used as a unique session ID
func newSessionId() string {
	hash := sha256.New()
	_, err := io.CopyN(hash, rand.Reader, 50)
	if err != nil {
		return "????????????????????"
	}
	md := hash.Sum(nil)
	mdStr := hex.EncodeToString(md)
	return mdStr[0:20]
}

// Serve starts an endless loop that reads FTP commands from the client and
// responds appropriately. terminated is a channel that will receive a true
// message when the connection closes. This loop will be running inside a
// goroutine, so use this channel to be notified when the connection can be
// cleaned up.
func (ftpConn *ftpConn) Serve() error {
	defer func() {
		if r := recover(); r != nil {
			if ftpConn.logger != nil {
				ftpConn.logger.Warnf("Recovered in ftpConn Serve %v", r)
			}
		}

		if err := ftpConn.Close(); err != nil {
			if ftpConn.logger != nil {
				ftpConn.logger.Warnf("failed to close connection %v", err)
			}
		}
	}()

	if ftpConn.logger != nil {
		ftpConn.logger.Debugf("Connection Established (local: %s, remote: %s)", ftpConn.localIP(), ftpConn.remoteIP())
	}

	// send welcome
	_, err := ftpConn.writeMessage(220, ftpConn.serverName)
	if err != nil {
		return err
	}
	// read commands
	for {
		line, err := ftpConn.controlReader.ReadString('\n')
		if err != nil {
			break
		}

		if err := ftpConn.receiveLine(line); err != nil {
			if ftpConn.logger != nil {
				ftpConn.logger.Warnf("failed to process line: %s - %v", line, err)
			}
		}
	}

	if ftpConn.logger != nil {
		ftpConn.logger.Debug("Connection Terminated")
	}
	return nil
}

// Close will manually close this connection, even if the client isn't ready.
func (ftpConn *ftpConn) Close() error {
	var errs error
	if err := ftpConn.conn.Close(); err != nil {
		errs = multierror.Append(errs, err)
	}

	if ftpConn.dataConn != nil {
		if err := ftpConn.dataConn.Close(); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	return errs
}

// receiveLine accepts a single line FTP command and co-ordinates an
// appropriate response.
func (ftpConn *ftpConn) receiveLine(line string) error {
	command, param := ftpConn.parseLine(line)
	if ftpConn.logger != nil {
		if command == "PASS" {
			ftpConn.logger.Debugf("PASS ***")
		} else {
			ftpConn.logger.Debugf("%s %s", command, param)
		}
	}

	cmdObj := commands[command]
	if cmdObj == nil {
		_, err := ftpConn.writeMessage(500, "Command not found")
		return err
	}

	if cmdObj.RequireParam() && param == "" {
		_, err := ftpConn.writeMessage(553, "action aborted, required param missing")
		return err
	}

	if cmdObj.RequireAuth() && ftpConn.user == "" {
		_, err := ftpConn.writeMessage(530, "not logged in")
		return err
	}
	return cmdObj.Execute(ftpConn, param)
}

func (ftpConn *ftpConn) parseLine(line string) (string, string) {
	params := strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)
	if len(params) == 1 {
		return params[0], ""
	}
	return params[0], strings.TrimSpace(params[1])
}

// writeMessage will send a standard FTP response back to the client.
func (ftpConn *ftpConn) writeMessage(code int, message string) (int, error) {
	if ftpConn.logger != nil {
		ftpConn.logger.Debugf("%d %s", code, message)
	}
	line := fmt.Sprintf("%d %s\r\n", code, message)
	wrote, err := ftpConn.controlWriter.WriteString(line)
	if err != nil {
		return 0, err
	}
	if err := ftpConn.controlWriter.Flush(); err != nil {
		return 0, err
	}
	return wrote, nil
}

// writeLines will send a multiline FTP response back to the client.
func (ftpConn *ftpConn) writeLines(code int, lines ...string) (int, error) {
	message := strings.Join(lines, "\r\n") + "\r\n"
	if ftpConn.logger != nil {
		ftpConn.logger.Debugf("%d %s", code, message)
	}
	wrote, err := ftpConn.controlWriter.WriteString(message)
	if err != nil {
		return 0, err
	}
	if err := ftpConn.controlWriter.Flush(); err != nil {
		return 0, err
	}
	return wrote, nil
}

// buildPath takes a client supplied path or filename and generates a safe
// absolute path within their account sandbox.
//
//    buildpath("/")
//    => "/"
//    buildpath("one.txt")
//    => "/one.txt"
//    buildpath("/files/two.txt")
//    => "/files/two.txt"
//    buildpath("files/two.txt")
//    => "files/two.txt"
//    buildpath("/../../../../etc/passwd")
//    => "/etc/passwd"
//
// The driver implementation is responsible for deciding how to treat this path.
// Obviously they MUST NOT just read the path off disk. The probably want to
// prefix the path with something to scope the users access to a sandbox.
func (ftpConn *ftpConn) buildPath(filename string) (fullPath string) {
	if len(filename) > 0 && filename[0:1] == "/" {
		fullPath = filepath.Clean(filename)
	} else if len(filename) > 0 {
		fullPath = filepath.Clean(ftpConn.namePrefix + "/" + filename)
	} else {
		fullPath = filepath.Clean(ftpConn.namePrefix)
	}
	fullPath = strings.Replace(fullPath, "//", "/", -1)
	return
}

// the server IP that is being used for this connection. May be the same for all connections,
// or may vary if the server is listening on 0.0.0.0
func (ftpConn *ftpConn) localIP() string {
	lAddr := ftpConn.conn.LocalAddr().(*net.TCPAddr)
	return lAddr.IP.String()
}

// the client IP address
func (ftpConn *ftpConn) remoteIP() string {
	rAddr := ftpConn.conn.RemoteAddr().(*net.TCPAddr)
	return rAddr.IP.String()
}

// sendOutOfBandData will copy data from reader to the client via the currently
// open data socket. Assumes the socket is open and ready to be used.
func (ftpConn *ftpConn) sendOutOfBandReader(reader io.Reader) error {
	defer ftpConn.dataConn.Close()

	var errs error
	_, err := io.Copy(ftpConn.dataConn, reader)
	if err != nil {
		errs = multierror.Append(errs, err)
		if _, err := ftpConn.writeMessage(550, "Action not taken"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}

	if _, err := ftpConn.writeMessage(226, "Transfer complete."); err != nil {
		return err
	}

	// Chrome dies on localhost if we close connection to soon
	time.Sleep(10 * time.Millisecond)
	return nil
}

// sendOutOfBandData will send a string to the client via the currently open
// data socket. Assumes the socket is open and ready to be used.
func (ftpConn *ftpConn) sendOutOfBandData(data string) error {
	return ftpConn.sendOutOfBandReader(bytes.NewReader([]byte(data)))
}

func (ftpConn *ftpConn) newPassiveSocket() (*ftpPassiveSocket, error) {
	if ftpConn.dataConn != nil {
		ftpConn.dataConn.Close()
		ftpConn.dataConn = nil
	}

	socket, err := newPassiveSocket(ftpConn.localIP(), ftpConn.minDataPort, ftpConn.maxDataPort, ftpConn.logger)
	if err != nil {
		return nil, err
	}

	ftpConn.dataConn = socket
	return socket, nil
}

func (ftpConn *ftpConn) newActiveSocket(host string, port int) (*ftpActiveSocket, error) {
	if ftpConn.dataConn != nil {
		ftpConn.dataConn.Close()
		ftpConn.dataConn = nil
	}

	socket, err := newActiveSocket(host, port, ftpConn.logger)
	if err != nil {
		return nil, err
	}

	ftpConn.dataConn = socket
	return socket, nil
}
