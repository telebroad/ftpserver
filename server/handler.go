package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func handleConnection(conn net.Conn, manager *FTPSessionManager) {
	// Generate a unique session ID for the connection
	sessionID := generateSessionID(conn)
	session := &FTPSession{
		conn:       conn,
		workingDir: "/", // Set the initial working directory
	}

	// Add the session to the manager
	manager.Add(sessionID, session)

	// Example: Authenticate the user
	authenticateUser(session)

	// Remove the session when the client disconnects
	defer manager.Remove(sessionID)

	// Handle client commands
}

func authenticateUser(session *FTPSession) {
	// Placeholder: Implement authentication logic
	session.isAuthenticated = true // Example outcome
}

func generateSessionID(conn net.Conn) string {
	// Placeholder: Generate a unique ID for the session
	return conn.RemoteAddr().String()
}

type LogWriter struct {
	io.Writer
}

func (w *LogWriter) Write(b []byte) (int, error) {
	fmt.Println("Responding:", string(b))
	return w.Writer.Write(b)
}
func (s *FTPServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	logWriter := &LogWriter{conn}
	sessionID := generateSessionID(conn)
	session := &FTPSession{
		conn:            conn,
		writer:          logWriter,
		userInfo:        nil,
		workingDir:      "/", // Set the initial working directory
		isAuthenticated: false,
		root:            s.root,
		dataListener:    nil,
		ftpServer:       s,
	}
	ftpSession := session
	// Add the session to the manager
	s.sessionManager.Add(sessionID, session)

	// Example: Authenticate the user
	authenticateUser(session)

	// Remove the session when the client disconnects
	defer s.sessionManager.Remove(sessionID)

	reader := bufio.NewReader(conn)
	// Send a welcome message
	fmt.Fprintln(conn, "220", s.WelcomeMessage)

	for {

		cmd, arg, err := s.ParseCommand(reader)
		if err != nil {
			fmt.Fprintln(logWriter, err.Error())
			return
		}
		// Handle commands
		switch cmd {
		case "USER":
			resp, err := ftpSession.UserCommand(arg)
			if err != nil {
				fmt.Fprintln(logWriter, err.Error())
				return
			}
			fmt.Fprintln(conn, resp)
		case "PASS":
			resp, err := ftpSession.PassCommand(arg)
			if err != nil {
				fmt.Fprintln(logWriter, err.Error())
				return
			}
			fmt.Fprintln(logWriter, resp)
		// Add more cases here for other commands
		case "SYST":
			fmt.Fprintln(logWriter, ftpSession.SystemCommand())
		case "FEAT":
			fmt.Fprintln(logWriter, ftpSession.FeaturesCommand())
		case "OPTS":
			ftpSession.OptsCommand(arg)
		case "PWD":
			fmt.Fprintln(logWriter, ftpSession.PrintWorkingDirectoryCommand())
		case "CWD":
			resp, err := ftpSession.ChangeDirectoryCommand(arg)
			if err != nil {
				fmt.Fprintln(logWriter, err.Error())
				return
			}
			fmt.Fprintln(logWriter, resp)

		case "REST":
			if arg == "0" {
				fmt.Fprintln(logWriter, "350 Ready for file transfer.")
			} else {
				fmt.Fprintln(logWriter, "350 Restarting at "+arg+". Send STORE or RETRIEVE.")
			}
		case "TYPE":
			ftpSession.TypeCommand(arg)

		case "PASV":
			ftpSession.PasvCommand(arg)

		case "EPSV":
			ftpSession.EpsvCommand(arg)

		case "LIST":
		case "MLSD": // MLSD is LIST with machine-readable format like $ls -l
			ftpSession.MLSDCommand(arg)
		case "MLST":
			ftpSession.MLSTCommand(arg)
		case "SIZE":

		case "STOR":

			ftpSession.StorCommand(arg)
		case "MDTM":
			ftpSession.ModifyTimeCommand(arg)

		case "RETR":
			ftpSession.RetrieveCommand(arg)
		case "QUIT":
			fmt.Fprintln(logWriter, "221 Goodbye.")
			return
		default:
			fmt.Fprintln(logWriter, "500 Unknown command.")
		}
	}

}

// ParseCommand  parses the command from the client and returns the command and argument.
func (s *FTPServer) ParseCommand(r *bufio.Reader) (cmd, arg string, err error) {
	line, err := r.ReadString('\n')
	if err != nil {
		err = fmt.Errorf("error reading from connection: %w", err)
		return
	}
	fmt.Println("Received:", line)
	command := strings.SplitN(strings.TrimSpace(line), " ", 2)
	cmd = command[0]

	if len(command) > 1 {
		arg = command[1]
	}
	return
}

// UserCommand handles the USER command from the client.
func (s *FTPSession) UserCommand(arg string) (resp string, err error) {
	if arg == "" {
		return "", fmt.Errorf("530 Error: User name not specified")
	}
	user, err := s.ftpServer.users.Get(arg)
	if err != nil {
		return "", fmt.Errorf("530 Error: Searching for user failed")
	}
	s.userInfo = user
	return "331 Please specify the password", nil
}

// PassCommand handles the PASS command from the client.
func (s *FTPSession) PassCommand(arg string) (resp string, err error) {
	if s.userInfo == nil {
		return "", fmt.Errorf("430 Invalid username or password")
	}
	if s.userInfo.Password != arg {
		return "", fmt.Errorf("430 Invalid username or password")
	}
	return "230 Login successful", nil
}

// SystemCommand returns the system type.
func (s *FTPSession) SystemCommand() string {
	// Use runtime.GOOS to get the operating system name
	os := runtime.GOOS

	// Customize the response based on the operating system
	switch os {
	case "windows":
		return "215 WINDOWS Type: L8"
	case "linux", "darwin":
		return "215 UNIX Type: L8" // macOS is Unix-based
	default:
		return "215 OS Type: " + os
	}
}

func (s *FTPSession) FeaturesCommand() string {
	f := []string{
		"211-Features:",
		" UTF8",
		" MLST type*;size*;modify*;",
		" MLSD",
		" SIZE",
		" MDTM",
		" REST STREAM",
		//" TVFS",
		" EPSV",
		//" EPRT",
	}

	if s.ftpServer.supportsTLS {
		f = append(f,
			//" AUTH TLS",
			//" AUTH SSL",
			" PBSZ",
			" PROT",
		)
	}
	f = append(f, "211 End")
	return strings.Join(f, "\n")
}

// PrintWorkingDirectoryCommand handles the PWD command from the client.
// The PWD command is used to print the current working directory on the server.
func (s *FTPSession) PrintWorkingDirectoryCommand() string {
	return fmt.Sprintf("257 \"%s\" is current directory", s.workingDir)
}

// ChangeDirectoryCommand handles the CWD command from the client.
// The CWD command is used to change the working directory on the server.
func (s *FTPSession) ChangeDirectoryCommand(arg string) (res string, err error) {
	// Resolve the requested directory to an absolute path
	requestedDir := ""
	if filepath.IsAbs(arg) {
		requestedDir = filepath.Join(s.ftpServer.fs.RootDir(), arg[1:])
	} else {
		requestedDir = filepath.Join(s.workingDir, arg)
	}
	fmt.Println("filepath.IsAbs(arg)", filepath.IsAbs(arg), requestedDir)
	//requestedDir = filepath.Clean(requestedDir)
	// if after the request is joined with the absolute path, the result is ".." then return an error
	if strings.HasPrefix(requestedDir, "..") {
		return "", fmt.Errorf("550 Failed to change directory")
	}

	err = s.ftpServer.fs.CheckDir(requestedDir)
	if err != nil {
		return "", fmt.Errorf("550 Error: %w", err)
	}

	s.workingDir = requestedDir

	return fmt.Sprintf("250 Directory successfully changed to \"%s\"", requestedDir), nil
}

// OptsCommand handles the OPTS command from the client.
// The OPTS command is used to specify options for the server.
func (s *FTPSession) OptsCommand(arg string) {
	switch arg {
	case "UTF8 ON":
		fmt.Fprintln(s.writer, "200 Always in UTF8 mode.")

	default:
		fmt.Fprintln(s.writer, "500 Unknown option.")
	}
}

// TypeCommand handles the TYPE command from the client.
// The TYPE command is used to specify the type of file being transferred.
// The two types are ASCII (A) and binary (I).
func (s *FTPSession) TypeCommand(arg string) {
	if arg == "I" {
		s.ftpServer.Type = typeI
		fmt.Fprintln(s.writer, "200 Type set to I")
	} else if arg == "A" {
		s.ftpServer.Type = typeA
		fmt.Fprintln(s.writer, "200 Type set to A")
	} else {
		fmt.Fprintln(s.writer, "500 Unknown type")
	}
}

// findAvailablePortInRange finds an available port in the given range.
// It returns a listener on the available port and the port number.
func findAvailablePortInRange(start, end int) (net.Listener, int, error) {
	for port := start; port <= end; port++ {
		address := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("no available ports found in range %d-%d", start, end)
}

// PasvEpsvCommand handles the PASV command from the client.
// The PASV command is used to enter passive mode.
func (s *FTPSession) PasvEpsvCommand(arg string) (port int, err error) {

	dataListener, port, err := findAvailablePortInRange(s.ftpServer.pasvMinPort, s.ftpServer.pasvMaxPort)
	if err != nil {
		err = fmt.Errorf("500: Server error listening for data connection: %w", err)
		fmt.Fprintln(s.writer, err.Error())
		return 0, err
	}

	s.dataListener = dataListener
	// Extract the port from the listener's address
	_, portString, err := net.SplitHostPort(dataListener.Addr().String())
	if err != nil {
		err = fmt.Errorf("500 Server error getting port: %w", err)
		fmt.Fprintln(s.writer, err.Error())
		dataListener.Close()
	}
	port, err = strconv.Atoi(portString)
	if err != nil {
		err = fmt.Errorf("500 Server error with port conversion: %w", err)
		fmt.Fprintf(s.writer, err.Error())
		dataListener.Close()
	}
	return port, nil
}

// PasvCommand handles the PASV command from the client.
// The PASV command is used to enter passive mode.
func (s *FTPSession) PasvCommand(arg string) error {
	port, err := s.PasvEpsvCommand(arg)
	if err != nil {
		return err
	}
	PublicIP := s.ftpServer.PublicServerIP

	resp := fmt.Sprintf("227 Entering Passive Mode (%d,%d,%d,%d,%d,%d)",
		PublicIP[0], PublicIP[1], PublicIP[2], PublicIP[3], port/256, port%256)
	fmt.Fprintln(s.writer, resp)
	return nil
}

// EpsvCommand handles the EPSV command from the client.
// The EPSV command is used to enter extended passive mode.
func (s *FTPSession) EpsvCommand(arg string) error {
	// Listen on a new port
	port, err := s.PasvEpsvCommand(arg)
	if err != nil {
		return err
	}

	// Respond with the port number
	// The response format is 229 Entering Extended Passive Mode (|||port|)
	resp := fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", port)
	fmt.Fprintln(s.writer, resp)
	return nil

}

// StorCommand handles the STOR command from the client.
// The STOR command is used to store a file on the server.
func (s *FTPSession) StorCommand(arg string) {
	// Close the data connection
	defer s.dataListener.Close()
	// At this point, dataConn is ready for use for data transfer
	// You can now send or receive data over dataConn
	fmt.Fprintln(s.writer, "150 Opening data connection.")
	// Wait for the client to connect on this new port
	dataConn, err := s.dataListener.Accept()
	if err != nil {
		fmt.Fprintf(s.writer, "425 Can't open data connection: %s\n", err)
		return
	}
	defer dataConn.Close()
	err = s.ftpServer.fs.Create(arg, dataConn, string(s.ftpServer.Type))
	if err != nil {
		fmt.Fprintln(s.writer, "550 Error writing to the file:", err)
		return
	}
	fmt.Fprintln(s.writer, "226 Transfer complete")

}

// ModifyTimeCommand handles the MDTM command from the client.
// The MDTM command is used to modify the modification time of a file on the server.
func (s *FTPSession) ModifyTimeCommand(arg string) {
	args := strings.SplitN(arg, " ", 2)
	if len(args) == 0 {
		fmt.Fprintln(s.writer, "501 No file name given")
		return
	} else if len(args) == 1 {
		stat, err := s.ftpServer.fs.Stat(args[0])
		if err != nil {
			fmt.Fprintln(s.writer, "501 Error getting file info:", err)
			return
		}
		fmt.Fprintln(s.writer, "213", stat)
	} else if len(args) == 2 {
		newTime, err := time.Parse("20060102150405", args[0])
		if err != nil {
			fmt.Fprintln(s.writer, "501 Invalid time format got:", args[0], "expected: YYYYMMDDHHMMSS")
			return
		}
		err = s.ftpServer.fs.ModifyTime(args[1], newTime)
		if err != nil {
			fmt.Fprintln(s.writer, "501 Error setting file modification time:", err, "for file:", args[0])
			return
		}
		fmt.Fprintln(s.writer, "213", "File modification time set to:", newTime.Format("20060102150405"))
	}

}
func (s *FTPSession) CloseDataConnection() {
	// Close the data connection
	if s.dataListener != nil {
		s.dataListener.Close()
	}
}

// MLSDCommand handles the MLSD command from the client.
// The MLSD command is used to list the contents of a directory in a machine-readable format.
func (s *FTPSession) MLSDCommand(arg string) {
	// Close the data connection

	fmt.Fprintln(s.writer, "150 Here comes the directory listing.")
	dataConn, err := s.dataListener.Accept()

	if err != nil {
		fmt.Fprintf(s.writer, "425 Can't open data connection: %s\n", err)
	}

	// Send the directory listing
	entries, err := s.ftpServer.fs.Dir(s.workingDir)
	if err != nil {
		fmt.Fprintln(s.writer, "550 Error getting directory listing.", err.Error())
		return
	}

	for _, entry := range entries {
		fmt.Println("dataConn:", entry)
		fmt.Fprintln(dataConn, entry)
	}
	dataConn.Close()
	s.dataListener.Close()
	fmt.Fprintln(s.writer, "226 Directory send OK.")
}
func (s *FTPSession) MLSTCommand(arg string) {
	filename := filepath.Join(s.workingDir, arg)
	entries, err := s.ftpServer.fs.Stat(filename)
	if err != nil {
		fmt.Fprintln(s.writer, "550 Error getting file info:", err)
		return
	}
	fmt.Fprintln(s.writer, "250-File details:")
	fmt.Fprintln(s.writer, entries)
	fmt.Fprintln(s.writer, "250 End")
}
func (s *FTPSession) RetrieveCommand(arg string) {

	// Close the data connection
	defer s.dataListener.Close()
	// At this point, dataConn is ready for use for data transfer
	// You can now send or receive data over dataConn
	fmt.Fprintln(s.writer, "150 Opening data connection.")
	// Wait for the client to connect on this new port
	dataConn, err := s.dataListener.Accept()
	if err != nil {
		fmt.Fprintf(s.writer, "425 Can't open data connection: %s\n", err)
	}
	filename := filepath.Join(s.workingDir, arg)
	fmt.Println("filename:", filename)
	_, err = s.ftpServer.fs.Read(filename, dataConn)
	if err != nil {
		fmt.Fprintf(s.writer, "550 Error reading the file: %s\n", err)
	}
	dataConn.Close()
	s.dataListener.Close()
	fmt.Fprintln(s.writer, "226 Transfer complete")
}
