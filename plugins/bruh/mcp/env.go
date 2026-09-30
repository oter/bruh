package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const stampLayout = "2006-01-02T15:04:05.000Z"

var (
	projectRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	taskRE    = regexp.MustCompile(`^[a-z0-9]+$`)
)

// RoleKey is a parsed role key: bigm, clerk-ledger, clanker-<project>, or clerk-<project>-<task>.
type RoleKey struct {
	Role    string // "bigm", "clanker", "clerk", or "ledger"
	Project string // empty for bigm and ledger
	Task    string // empty except for clerk
}

// ParseRoleKey parses a role key. The last hyphen of a clerk key separates the project from the task.
func ParseRoleKey(s string) (RoleKey, error) {
	bad := fmt.Errorf("invalid role key: %q", s)
	if len(s) > 64 {
		return RoleKey{}, bad
	}
	switch s {
	case "bigm":
		return RoleKey{Role: "bigm"}, nil
	case "clerk-ledger":
		return RoleKey{Role: "ledger"}, nil
	}
	if p, ok := strings.CutPrefix(s, "clanker-"); ok && projectRE.MatchString(p) {
		return RoleKey{Role: "clanker", Project: p}, nil
	}
	if rest, ok := strings.CutPrefix(s, "clerk-"); ok {
		if i := strings.LastIndexByte(rest, '-'); i > 0 && projectRE.MatchString(rest[:i]) && taskRE.MatchString(rest[i+1:]) {
			return RoleKey{Role: "clerk", Project: rest[:i], Task: rest[i+1:]}, nil
		}
	}
	return RoleKey{}, bad
}

func (k RoleKey) String() string {
	switch k.Role {
	case "bigm":
		return "bigm"
	case "ledger":
		return "clerk-ledger"
	case "clanker":
		return "clanker-" + k.Project
	case "clerk":
		return "clerk-" + k.Project + "-" + k.Task
	}
	return ""
}

// Parent is the role key that starts, and grants leases to, this role. bigm has none.
func (k RoleKey) Parent() string {
	switch k.Role {
	case "clerk":
		return "clanker-" + k.Project
	case "clanker", "ledger":
		return "bigm"
	}
	return ""
}

// Env is everything a tool handler needs from its process.
type Env struct {
	DataDir          string
	RoleKey          string
	PluginRoot       string
	Home             string // home folder of the user
	SettingsFile     string // user settings.json that init writes
	ClaudeBin        string // the claude binary; tests use a fake script
	PollInterval     time.Duration
	ProgressInterval time.Duration
	Now              func() time.Time
}

func EnvFromOS() Env {
	home, _ := os.UserHomeDir()
	return Env{
		DataDir:          os.Getenv("BRUH_DATA"),
		RoleKey:          os.Getenv("BRUH_ROLE_KEY"),
		PluginRoot:       os.Getenv("BRUH_PLUGIN_ROOT"),
		Home:             home,
		SettingsFile:     cmp.Or(os.Getenv("BRUH_SETTINGS_FILE"), filepath.Join(home, ".claude", "settings.json")),
		ClaudeBin:        cmp.Or(os.Getenv("BRUH_CLAUDE_BIN"), "claude"),
		PollInterval:     durationEnv("BRUH_POLL_MS", 2*time.Second),
		ProgressInterval: durationEnv("BRUH_PROGRESS_MS", time.Minute),
		Now:              time.Now,
	}
}

func durationEnv(name string, def time.Duration) time.Duration {
	ms, err := strconv.Atoi(os.Getenv(name))
	if err != nil || ms <= 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

// Caller returns the role key of the calling session.
func (e Env) Caller() (string, error) {
	if e.RoleKey == "" {
		return "", errors.New("BRUH_ROLE_KEY is not set")
	}
	return checkKey(e.RoleKey, "role key")
}

func checkKey(s, what string) (string, error) {
	if _, err := ParseRoleKey(s); err != nil {
		return "", fmt.Errorf("invalid %s: %q", what, s)
	}
	return s, nil
}

func checkID(s string, re *regexp.Regexp, what string) (string, error) {
	if !re.MatchString(s) {
		return "", fmt.Errorf("invalid %s: %q", what, s)
	}
	return s, nil
}

// Dir returns a folder under the data folder and creates it with mode 0700.
func (e Env) Dir(parts ...string) (string, error) {
	if e.DataDir == "" {
		return "", errors.New("BRUH_DATA is not set")
	}
	p := filepath.Join(append([]string{e.DataDir}, parts...)...)
	return p, os.MkdirAll(p, 0o700)
}

// Stamp is the current UTC time in the fixed layout.
func (e Env) Stamp() string {
	return e.Now().UTC().Format(stampLayout)
}

// atomicWrite writes a temporary file in the same folder and renames it.
func atomicWrite(file string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, file)
}

// WithLock runs fn while it holds an exclusive flock on <data>/locks/<name>.lock. The kernel
// releases the lock when its holder exits, so a dead holder never leaves a stale lock behind.
// flock exists on macOS and Linux, the platforms of bruh (spec 10.2).
func (e Env) WithLock(name string, fn func() error) error {
	locks, err := e.Dir("locks")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(locks, name+".lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("lock busy: %s", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

// bigmActsFor reports whether bigm may write the role settings of k, start it, or resume it
// without being its parent: its own key, and the merger clerk of any project, which runs on
// the machine of bigm also for a remote project. Every other key belongs to its parent, so
// bigm does not get past the busy-clerk cap or the leases of a clanker.
func bigmActsFor(k RoleKey) bool {
	return k.Role == "bigm" || (k.Role == "clerk" && k.Task == "merge")
}
