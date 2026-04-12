package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Layout is the on-disk daemon layout under ~/.flume.
type Layout struct {
	Home       string // ~/.flume
	SocketPath string // ~/.flume/flumed.sock
	PIDPath    string // ~/.flume/flumed.pid
	LogPath    string // ~/.flume/flumed.log
	DataDir    string // ~/.flume/data (BadgerDB)
}

// LayoutFor builds the layout from a flume home directory.
func LayoutFor(home string) Layout {
	return Layout{
		Home:       home,
		SocketPath: filepath.Join(home, "flumed.sock"),
		PIDPath:    filepath.Join(home, "flumed.pid"),
		LogPath:    filepath.Join(home, "flumed.log"),
		DataDir:    filepath.Join(home, "data"),
	}
}

// AliveDaemon returns (true, pid) if a daemon is currently listening on the
// socket and the PID file matches a running process.
func AliveDaemon(layout Layout) (bool, int) {
	pidBytes, err := os.ReadFile(layout.PIDPath)
	if err != nil {
		if pingSocket(layout.SocketPath) {
			return true, 0
		}
		return false, 0
	}
	pid, err := strconv.Atoi(string(bytesTrimSpace(pidBytes)))
	if err != nil {
		return false, 0
	}
	if !processAlive(pid) {
		return false, 0
	}
	if !pingSocket(layout.SocketPath) {
		return false, 0
	}
	return true, pid
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func pingSocket(path string) bool {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func bytesTrimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\n' || b[start] == '\r' || b[start] == '\t') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}
