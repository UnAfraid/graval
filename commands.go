package graval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/jehiah/go-strftime"
)

type ftpCommand interface {
	RequireParam() bool
	RequireAuth() bool
	Execute(*ftpConn, string) error
}

type commandMap map[string]ftpCommand

var (
	commands = commandMap{
		"ALLO": commandAllo{},
		"CDUP": commandCdup{},
		"CWD":  commandCwd{},
		"DELE": commandDele{},
		"EPRT": commandEprt{},
		"EPSV": commandEpsv{},
		"FEAT": commandFeat{},
		"LIST": commandList{},
		"NLST": commandNlst{},
		"MDTM": commandMdtm{},
		"MKD":  commandMkd{},
		"MODE": commandMode{},
		"NOOP": commandNoop{},
		"OPTS": commandOpts{},
		"PASS": commandPass{},
		"PASV": commandPasv{},
		"PORT": commandPort{},
		"PWD":  commandPwd{},
		"QUIT": commandQuit{},
		"RETR": commandRetr{},
		"RNFR": commandRnfr{},
		"RNTO": commandRnto{},
		"RMD":  commandRmd{},
		"SIZE": commandSize{},
		"STOR": commandStor{},
		"STRU": commandStru{},
		"SYST": commandSyst{},
		"TYPE": commandType{},
		"USER": commandUser{},
		"XCUP": commandCdup{},
		"XCWD": commandCwd{},
		"XPWD": commandPwd{},
		"XRMD": commandRmd{},
	}

	// Some FTP clients send flags to the LIST and NLST commands. Server support for these varies,
	// and implementing them all would be a lot of work with uncertain payoff. For now, we ignore them
	listFlagsRegexp = `^-[alt]+$`
)

// commandAllo responds to the ALLO FTP command.
//
// This is essentially a ping from the client so we just respond with an
// basic OK message.
type commandAllo struct{}

func (cmd commandAllo) RequireParam() bool {
	return false
}

func (cmd commandAllo) RequireAuth() bool {
	return false
}

func (cmd commandAllo) Execute(conn *ftpConn, _ string) error {
	_, err := conn.writeMessage(202, "Obsolete")
	return err
}

// commandCdup responds to the CDUP FTP command.
//
// Allows the client change their current directory to the parent.
type commandCdup struct{}

func (cmd commandCdup) RequireParam() bool {
	return false
}

func (cmd commandCdup) RequireAuth() bool {
	return true
}

func (cmd commandCdup) Execute(conn *ftpConn, _ string) error {
	otherCmd := &commandCwd{}
	return otherCmd.Execute(conn, "..")
}

// commandCwd responds to the CWD FTP command. It allows the client to change the
// current working directory.
type commandCwd struct{}

func (cmd commandCwd) RequireParam() bool {
	return true
}

func (cmd commandCwd) RequireAuth() bool {
	return true
}

func (cmd commandCwd) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	changeDir, err := conn.driver.ChangeDir(path)
	if err != nil {
		return fmt.Errorf("failed to execute CWD path: %s - %w", path, err)
	}

	if changeDir {
		conn.namePrefix = path
		_, err := conn.writeMessage(250, "Directory changed to "+path)
		return err
	} else {
		_, err := conn.writeMessage(550, "Action not taken")
		return err
	}
}

// commandDele responds to the DELE FTP command. It allows the client to delete
// a file
type commandDele struct{}

func (cmd commandDele) RequireParam() bool {
	return true
}

func (cmd commandDele) RequireAuth() bool {
	return true
}

func (cmd commandDele) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	deleteFile, err := conn.driver.DeleteFile(path)
	if err != nil {
		return fmt.Errorf("failed to execute DELE path: path - %w", err)
	}

	if deleteFile {
		_, err := conn.writeMessage(250, "File deleted")
		return err
	} else {
		_, err := conn.writeMessage(550, "Action not taken")
		return err
	}
}

// commandEprt responds to the EPRT FTP command. It allows the client to
// request an active data socket with more options than the original PORT
// command. It mainly adds ipv6 support.
type commandEprt struct{}

func (cmd commandEprt) RequireParam() bool {
	return true
}

func (cmd commandEprt) RequireAuth() bool {
	return true
}

func (cmd commandEprt) Execute(conn *ftpConn, param string) error {
	delimiter := param[0:1]
	parts := strings.Split(param, delimiter)
	addressFamily, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	host := parts[2]
	port, err := strconv.ParseUint(parts[3], 10, 16)
	if err != nil {
		return err
	}

	if addressFamily != 1 && addressFamily != 2 {
		_, err := conn.writeMessage(522, "Network protocol not supported, use (1,2)")
		return err
	}

	var errs error
	_, err = conn.newActiveSocket(host, uint16(port))
	if err != nil {
		errs = multierror.Append(errs, err)
		if _, err := conn.writeMessage(425, "Data connection failed"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}
	_, err = conn.writeMessage(200, fmt.Sprintf("Connection established (%d)", port))
	return err
}

// commandEpsv responds to the EPSV FTP command. It allows the client to
// request a passive data socket with more options than the original PASV
// command. It mainly adds ipv6 support, although we don't support that yet.
type commandEpsv struct{}

func (cmd commandEpsv) RequireParam() bool {
	return false
}

func (cmd commandEpsv) RequireAuth() bool {
	return true
}

func (cmd commandEpsv) Execute(conn *ftpConn, _ string) error {
	var errs error
	socket, err := conn.newPassiveSocket()
	if err != nil {
		errs = multierror.Append(errs, err)
		if _, err := conn.writeMessage(425, "Data connection failed"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}
	msg := fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", socket.Port())
	_, err = conn.writeMessage(229, msg)
	return err
}

// commandFeat responds to the FEAT FTP command.
//
// List all new features supported as defined in RFC-2398.
type commandFeat struct{}

func (cmd commandFeat) RequireParam() bool {
	return false
}

func (cmd commandFeat) RequireAuth() bool {
	return false
}

func (cmd commandFeat) Execute(conn *ftpConn, _ string) error {
	_, err := conn.writeLines(211,
		"211-Features supported:",
		" EPRT",
		" EPSV",
		" MDTM",
		" SIZE",
		" UTF8",
		"211 End FEAT.",
	)
	return err
}

// commandList responds to the LIST FTP command. It allows the client to retreive
// a detailed listing of the contents of a directory.
type commandList struct{}

func (cmd commandList) RequireParam() bool {
	return false
}

func (cmd commandList) RequireAuth() bool {
	return true
}

func (cmd commandList) Execute(conn *ftpConn, param string) error {
	_, err := conn.writeMessage(150, "Opening ASCII mode data connection for file list")
	if err != nil {
		return err
	}

	matched, _ := regexp.MatchString(listFlagsRegexp, param)
	if matched {
		param = ""
	}
	path := conn.buildPath(param)
	files, err := conn.driver.DirContents(path)
	if err != nil {
		return fmt.Errorf("failed to execute LIST path: %s - %w", path, err)
	}
	formatter := newListFormatter(files)
	return conn.sendOutOfBandData(formatter.Detailed())
}

// commandNlst responds to the NLST FTP command. It allows the client to
// retreive a list of filenames in the current directory.
type commandNlst struct{}

func (cmd commandNlst) RequireParam() bool {
	return false
}

func (cmd commandNlst) RequireAuth() bool {
	return true
}

func (cmd commandNlst) Execute(conn *ftpConn, param string) error {
	_, err := conn.writeMessage(150, "Opening ASCII mode data connection for file list")
	if err != nil {
		return err
	}
	matched, _ := regexp.MatchString(listFlagsRegexp, param)
	if matched {
		param = ""
	}
	path := conn.buildPath(param)
	files, err := conn.driver.DirContents(path)
	if err != nil {
		return fmt.Errorf("failed to execute NLST path: %s - %w", path, err)
	}

	formatter := newListFormatter(files)
	return conn.sendOutOfBandData(formatter.Short())
}

// commandMdtm responds to the MDTM FTP command. It allows the client to
// retreive the last modified time of a file.
type commandMdtm struct{}

func (cmd commandMdtm) RequireParam() bool {
	return true
}

func (cmd commandMdtm) RequireAuth() bool {
	return true
}

func (cmd commandMdtm) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	var errs error
	time, err := conn.driver.ModifiedTime(path)
	if err != nil {
		errs = multierror.Append(errs, err)
		if _, err := conn.writeMessage(450, "File not available"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}
	_, err = conn.writeMessage(213, strftime.Format("%Y%m%d%H%M%S", time))
	return err
}

// commandMkd responds to the MKD FTP command. It allows the client to create
// a new directory
type commandMkd struct{}

func (cmd commandMkd) RequireParam() bool {
	return true
}

func (cmd commandMkd) RequireAuth() bool {
	return true
}

func (cmd commandMkd) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	makeDir, err := conn.driver.MakeDir(path)
	if err != nil {
		return fmt.Errorf("failed to execute MKD: %s - %w", path, err)
	}

	if makeDir {
		_, err := conn.writeMessage(257, "Directory created")
		return err
	}

	_, err = conn.writeMessage(550, "Action not taken")
	return err
}

// commandMode responds to the MODE FTP command.
//
// the original FTP spec had various options for hosts to negotiate how data
// would be sent over the data socket, In reality these days (S)tream mode
// is all that is used for the mode - data is just streamed down the data
// socket unchanged.
type commandMode struct{}

func (cmd commandMode) RequireParam() bool {
	return true
}

func (cmd commandMode) RequireAuth() bool {
	return true
}

func (cmd commandMode) Execute(conn *ftpConn, param string) error {
	if strings.ToUpper(param) == "S" {
		_, err := conn.writeMessage(200, "OK")
		return err
	}

	_, err := conn.writeMessage(504, "MODE is an obsolete command")
	return err
}

// commandNoop responds to the NOOP FTP command.
//
// This is essentially a ping from the client so we just respond with an
// basic 200 message.
type commandNoop struct{}

func (cmd commandNoop) RequireParam() bool {
	return false
}

func (cmd commandNoop) RequireAuth() bool {
	return false
}

func (cmd commandNoop) Execute(conn *ftpConn, _ string) error {
	_, err := conn.writeMessage(200, "OK")
	return err
}

// commandOpts responds to the OPTS FTP command.
//
// This is essentially a ping from the client so we just respond with an
// basic 200 message.
type commandOpts struct{}

func (cmd commandOpts) RequireParam() bool {
	return false
}

func (cmd commandOpts) RequireAuth() bool {
	return true
}

func (cmd commandOpts) Execute(conn *ftpConn, param string) error {
	if param == "UTF8 ON" || param == "UTF8" {
		_, err := conn.writeMessage(200, "OK")
		return err
	}

	_, err := conn.writeMessage(500, "Command not found")
	return err
}

// commandPass respond to the PASS FTP command by asking the driver if the
// supplied username and password are valid
type commandPass struct{}

func (cmd commandPass) RequireParam() bool {
	return true
}

func (cmd commandPass) RequireAuth() bool {
	return false
}

func (cmd commandPass) Execute(conn *ftpConn, param string) error {
	var errs error
	ok, err := conn.driver.Authenticate(conn.reqUser, param, conn.remoteIP())
	if err != nil || !ok {
		if _, err := conn.writeMessage(530, "Incorrect password, not logged in"); err != nil {
			errs = multierror.Append(errs, err)
		}
		if _, err := conn.writeMessage(221, "Goodbye."); err != nil {
			errs = multierror.Append(errs, err)
		}
		if err := conn.Close(); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}

	conn.user = conn.reqUser
	conn.reqUser = ""
	_, err = conn.writeMessage(230, "Password ok, continue")
	return err
}

// commandPasv responds to the PASV FTP command.
//
// The client is requesting us to open a new TCP listing socket and wait for them
// to connect to it.
type commandPasv struct{}

func (cmd commandPasv) RequireParam() bool {
	return false
}

func (cmd commandPasv) RequireAuth() bool {
	return true
}

func (cmd commandPasv) Execute(conn *ftpConn, _ string) error {
	var errs error
	socket, err := conn.newPassiveSocket()
	if err != nil {
		errs = multierror.Append(errs, err)
		if _, err := conn.writeMessage(425, "Data connection failed"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return err
	}

	p1 := socket.Port() / 256
	p2 := socket.Port() - (p1 * 256)

	// if the server has been configured to send a specific IP for clients to connect to, use it. Otherwise
	// fallback to the IP that the passive port is listening on
	host := conn.pasvAdvertisedIp
	if host == "" {
		host = socket.Host()
	}
	quads := strings.Split(host, ".")
	target := fmt.Sprintf("(%s,%s,%s,%s,%d,%d)", quads[0], quads[1], quads[2], quads[3], p1, p2)
	msg := "Entering Passive Mode " + target
	_, err = conn.writeMessage(227, msg)
	return err
}

// commandPort responds to the PORT FTP command.
//
// The client has opened a listening socket for sending out of band data and
// is requesting that we connect to it
type commandPort struct{}

func (cmd commandPort) RequireParam() bool {
	return true
}

func (cmd commandPort) RequireAuth() bool {
	return true
}

func (cmd commandPort) Execute(conn *ftpConn, param string) error {
	nums := strings.Split(param, ",")
	if len(nums) < 6 {
		_, err := conn.writeMessage(425, "Data connection failed")
		return err
	}

	portOne, err := strconv.ParseUint(nums[4], 10, 16)
	if err != nil {
		return err
	}

	portTwo, err := strconv.ParseUint(nums[5], 10, 16)
	if err != nil {
		return err
	}

	port := uint16((portOne * 256) + portTwo)
	host := nums[0] + "." + nums[1] + "." + nums[2] + "." + nums[3]

	var errs error
	_, err = conn.newActiveSocket(host, port)
	if err != nil {
		errs = multierror.Append(errs, err)

		if _, err := conn.writeMessage(425, "Data connection failed"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}
	_, err = conn.writeMessage(200, fmt.Sprintf("Connection established (%d)", port))
	return err
}

// commandPwd responds to the PWD FTP command.
//
// Tells the client what the current working directory is.
type commandPwd struct{}

func (cmd commandPwd) RequireParam() bool {
	return false
}

func (cmd commandPwd) RequireAuth() bool {
	return true
}

func (cmd commandPwd) Execute(conn *ftpConn, _ string) error {
	_, err := conn.writeMessage(257, "\""+conn.namePrefix+"\" is the current directory")
	return err
}

// CommandQuit responds to the QUIT FTP command. The client has requested the
// connection be closed.
type commandQuit struct{}

func (cmd commandQuit) RequireParam() bool {
	return false
}

func (cmd commandQuit) RequireAuth() bool {
	return false
}

func (cmd commandQuit) Execute(conn *ftpConn, _ string) error {
	return conn.Close()
}

// commandRetr responds to the RETR FTP command. It allows the client to
// download a file.
type commandRetr struct{}

func (cmd commandRetr) RequireParam() bool {
	return true
}

func (cmd commandRetr) RequireAuth() bool {
	return true
}

func (cmd commandRetr) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	var errs error
	reader, err := conn.driver.GetFile(path)
	if err != nil {
		errs = multierror.Append(errs, err)
		if _, err := conn.writeMessage(551, "File not available"); err != nil {
			errs = multierror.Append(errs, err)
		}
		return errs
	}

	defer reader.Close()
	if _, err = conn.writeMessage(150, "Data connection open. Transfer starting."); err != nil {
		return err
	}
	return conn.sendOutOfBandReader(reader)
}

// commandRnfr responds to the RNFR FTP command. It's the first of two commands
// required for a client to rename a file.
type commandRnfr struct{}

func (cmd commandRnfr) RequireParam() bool {
	return true
}

func (cmd commandRnfr) RequireAuth() bool {
	return true
}

func (cmd commandRnfr) Execute(conn *ftpConn, param string) error {
	conn.renameFrom = conn.buildPath(param)
	_, err := conn.writeMessage(350, "Requested file action pending further information.")
	return err
}

// commandRnto responds to the RNTO FTP command. It's the second of two commands
// required for a client to rename a file.
type commandRnto struct{}

func (cmd commandRnto) RequireParam() bool {
	return true
}

func (cmd commandRnto) RequireAuth() bool {
	return true
}

func (cmd commandRnto) Execute(conn *ftpConn, param string) error {
	if conn.renameFrom == "" {
		_, err := conn.writeMessage(503, "Bad sequence of commands: use RNFR first.")
		return err
	}

	toPath := conn.buildPath(param)
	rename, err := conn.driver.Rename(conn.renameFrom, toPath)
	if err != nil {
		return fmt.Errorf("failed to execute RNTO from: %s to: %s - %w", conn.renameFrom, toPath, err)
	}

	if rename {
		_, err := conn.writeMessage(250, "File renamed")
		return err
	}

	_, err = conn.writeMessage(550, "Action not taken")
	return err
}

// commandRmd responds to the RMD FTP command. It allows the client to delete a
// directory.
type commandRmd struct{}

func (cmd commandRmd) RequireParam() bool {
	return true
}

func (cmd commandRmd) RequireAuth() bool {
	return true
}

func (cmd commandRmd) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	deleteDir, err := conn.driver.DeleteDir(path)
	if err != nil {
		return fmt.Errorf("failed to execute RMD path: %s - %w", path, err)
	}

	if deleteDir {
		_, err := conn.writeMessage(250, "Directory deleted")
		return err
	}

	_, err = conn.writeMessage(550, "Action not taken")
	return err
}

// commandSize responds to the SIZE FTP command. It returns the size of the
// requested path in bytes.
type commandSize struct{}

func (cmd commandSize) RequireParam() bool {
	return true
}

func (cmd commandSize) RequireAuth() bool {
	return true
}

func (cmd commandSize) Execute(conn *ftpConn, param string) error {
	path := conn.buildPath(param)
	bytes, err := conn.driver.Bytes(path)
	if err != nil {
		return fmt.Errorf("failed to execute SIZE path: %s - %w", path, err)
	}

	if bytes >= 0 {
		_, err := conn.writeMessage(213, fmt.Sprintf("%d", bytes))
		return err
	}

	_, err = conn.writeMessage(450, "file not available")
	return err
}

// commandStor responds to the STOR FTP command. It allows the user to upload a
// new file.
type commandStor struct{}

func (cmd commandStor) RequireParam() bool {
	return true
}

func (cmd commandStor) RequireAuth() bool {
	return true
}

func (cmd commandStor) Execute(conn *ftpConn, param string) error {
	targetPath := conn.buildPath(param)
	if _, err := conn.writeMessage(150, "Data transfer starting"); err != nil {
		return err
	}

	putFile, err := conn.driver.PutFile(targetPath, conn.dataConn)
	if err != nil {
		return fmt.Errorf("failed to execute STOR path: %s - %w", targetPath, err)
	}

	if putFile {
		_, err := conn.writeMessage(226, "Transfer complete.")
		return err
	}

	_, err = conn.writeMessage(450, "error during transfer")
	return err
}

// commandStru responds to the STRU FTP command.
//
// like the MODE and TYPE commands, stru[cture] dates back to a time when the
// FTP protocol was more aware of the content of the files it was transferring,
// and would sometimes be expected to translate things like EOL markers on the
// fly.
//
// These days files are sent unmodified, and F(ile) mode is the only one we
// really need to support.
type commandStru struct{}

func (cmd commandStru) RequireParam() bool {
	return true
}

func (cmd commandStru) RequireAuth() bool {
	return true
}

func (cmd commandStru) Execute(conn *ftpConn, param string) error {
	if strings.ToUpper(param) == "F" {
		_, err := conn.writeMessage(200, "OK")
		return err
	}

	_, err := conn.writeMessage(504, "STRU is an obsolete command")
	return err
}

// commandSyst responds to the SYST FTP command by providing a canned response.
type commandSyst struct{}

func (cmd commandSyst) RequireParam() bool {
	return false
}

func (cmd commandSyst) RequireAuth() bool {
	return true
}

func (cmd commandSyst) Execute(conn *ftpConn, _ string) error {
	_, err := conn.writeMessage(215, "UNIX Type: L8")
	return err
}

// commandType responds to the TYPE FTP command.
//
//  like the MODE and STRU commands, TYPE dates back to a time when the FTP
//  protocol was more aware of the content of the files it was transferring, and
//  would sometimes be expected to translate things like EOL markers on the fly.
//
//  Valid options were A(SCII), I(mage), E(BCDIC) or LN (for local type). Since
//  we plan to just accept bytes from the client unchanged, I think Image mode is
//  adequate. The RFC requires we accept ASCII mode however, so accept it, but
//  ignore it.
type commandType struct{}

func (cmd commandType) RequireParam() bool {
	return false
}

func (cmd commandType) RequireAuth() bool {
	return true
}

func (cmd commandType) Execute(conn *ftpConn, param string) error {
	if strings.ToUpper(param) == "A" {
		_, err := conn.writeMessage(200, "Type set to ASCII")
		return err
	}

	if strings.ToUpper(param) == "I" {
		_, err := conn.writeMessage(200, "Type set to binary")
		return err
	}

	_, err := conn.writeMessage(500, "Invalid type")
	return err
}

// commandUser responds to the USER FTP command by asking for the password
type commandUser struct{}

func (cmd commandUser) RequireParam() bool {
	return true
}

func (cmd commandUser) RequireAuth() bool {
	return false
}

func (cmd commandUser) Execute(conn *ftpConn, param string) error {
	conn.reqUser = param
	_, err := conn.writeMessage(331, "User name ok, password required")
	return err
}
