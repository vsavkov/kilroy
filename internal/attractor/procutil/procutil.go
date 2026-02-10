package procutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ProcFSAvailable reports whether procfs is available for process introspection.
func ProcFSAvailable() bool {
	_, err := os.Stat("/proc/self/stat")
	return err == nil
}

// PIDAlive reports whether a process exists and is not a zombie.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if PIDZombie(pid) {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

// PIDZombie checks whether a PID is in a zombie/dead state.
func PIDZombie(pid int) bool {
	if !ProcFSAvailable() {
		return pidZombieFromPS(pid)
	}
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	b, err := os.ReadFile(statPath)
	if err != nil {
		return false
	}
	line := string(b)
	closeIdx := strings.LastIndexByte(line, ')')
	if closeIdx < 0 || closeIdx+2 >= len(line) {
		return false
	}
	state := line[closeIdx+2]
	return state == 'Z' || state == 'X'
}

func pidZombieFromPS(pid int) bool {
	out, err := exec.Command("ps", "-o", "state=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	state := strings.TrimSpace(string(out))
	if state == "" {
		return false
	}
	c := state[0]
	return c == 'Z' || c == 'X'
}
