package kaos

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// DetectEnvironment detects the current OS environment.
func DetectEnvironment() (Environment, error) {
	osKind := resolveOsKind(runtime.GOOS)
	osArch := runtime.GOARCH
	osVersion := ""

	if runtime.GOOS == "windows" {
		shellPath, err := locateWindowsGitBash()
		if err != nil {
			return Environment{}, err
		}
		return Environment{
			OsKind:    osKind,
			OsArch:    osArch,
			OsVersion: osVersion,
			ShellName: ShellBash,
			ShellPath: shellPath,
		}, nil
	}

	// Unix-like: try bash candidates
	candidates := []string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return Environment{
				OsKind:    osKind,
				OsArch:    osArch,
				OsVersion: osVersion,
				ShellName: ShellBash,
				ShellPath: p,
			}, nil
		}
	}

	// Fallback to sh
	return Environment{
		OsKind:    osKind,
		OsArch:    osArch,
		OsVersion: osVersion,
		ShellName: ShellSh,
		ShellPath: "/bin/sh",
	}, nil
}

func resolveOsKind(platform string) OsKind {
	switch platform {
	case "darwin":
		return OsKindMacOS
	case "linux":
		return OsKindLinux
	case "windows":
		return OsKindWindows
	default:
		return OsKind(platform)
	}
}

func locateWindowsGitBash() (string, error) {
	// Check KIMI_SHELL_PATH override
	if override := os.Getenv("KIMI_SHELL_PATH"); override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, nil
		}
	}

	// Common Git install locations
	candidates := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", errors.New("kaos: Git Bash not found on Windows")
}

// cached environment
var (
	detectedEnv     Environment
	detectedEnvOnce sync.Once
	detectedEnvErr  error
)

// DetectEnvironmentCached returns the cached environment detection.
func DetectEnvironmentCached() (Environment, error) {
	detectedEnvOnce.Do(func() {
		detectedEnv, detectedEnvErr = DetectEnvironment()
	})
	return detectedEnv, detectedEnvErr
}

// LocalKaos is a Kaos implementation backed by the local filesystem and OS.
type LocalKaos struct {
	env Environment
	cwd string
	extraEnv map[string]string
	mu       sync.RWMutex
}

// NewLocalKaos creates a LocalKaos with the given environment.
func NewLocalKaos(env Environment) (*LocalKaos, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &LocalKaos{env: env, cwd: cwd, extraEnv: make(map[string]string)}, nil
}

func (k *LocalKaos) Name() string        { return "local" }
func (k *LocalKaos) OsEnv() Environment   { return k.env }
func (k *LocalKaos) PathClass() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return "posix"
}

func (k *LocalKaos) Normpath(p string) string { return filepath.Clean(p) }

func (k *LocalKaos) Gethome() string {
	home, _ := os.UserHomeDir()
	return home
}

func (k *LocalKaos) Getcwd() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.cwd
}

func (k *LocalKaos) Chdir(_ context.Context, path string) error {
	abs := k.resolve(path)
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "chdir", Path: abs, Err: errors.New("not a directory")}
	}
	k.mu.Lock()
	k.cwd = abs
	k.mu.Unlock()
	return nil
}

func (k *LocalKaos) WithCwd(cwd string) Kaos {
	return &LocalKaos{env: k.env, cwd: cwd, extraEnv: k.extraEnv}
}

func (k *LocalKaos) WithEnv(env map[string]string) Kaos {
	merged := make(map[string]string)
	for key, val := range k.extraEnv {
		merged[key] = val
	}
	for key, val := range env {
		merged[key] = val
	}
	return &LocalKaos{env: k.env, cwd: k.cwd, extraEnv: merged}
}

func (k *LocalKaos) Stat(_ context.Context, path string, followSymlinks bool) (*StatResult, error) {
	abs := k.resolve(path)
	var info os.FileInfo
	var err error
	if followSymlinks {
		info, err = os.Stat(abs)
	} else {
		info, err = os.Lstat(abs)
	}
	if err != nil {
		return nil, err
	}
	return &StatResult{
		StMode:  info.Mode(),
		StSize:  info.Size(),
		StMtime: info.ModTime().Unix(),
		StAtime: info.ModTime().Unix(), // Approximate: Go doesn't expose atime easily
		StCtime: info.ModTime().Unix(), // Approximate
	}, nil
}

func (k *LocalKaos) Iterdir(_ context.Context, path string) (<-chan string, error) {
	abs := k.resolve(path)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, e := range entries {
			ch <- e.Name()
		}
	}()
	return ch, nil
}

func (k *LocalKaos) Glob(_ context.Context, path, pattern string, _ bool) (<-chan string, error) {
	abs := k.resolve(filepath.Join(path, pattern))
	matches, err := filepath.Glob(abs)
	if err != nil {
		return nil, err
	}
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, m := range matches {
			ch <- m
		}
	}()
	return ch, nil
}

func (k *LocalKaos) ReadBytes(_ context.Context, path string, n int) ([]byte, error) {
	abs := k.resolve(path)
	if n > 0 {
		f, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		buf := make([]byte, n)
		m, err := f.Read(buf)
		return buf[:m], err
	}
	return os.ReadFile(abs)
}

func (k *LocalKaos) ReadText(_ context.Context, path string, _ string) (string, error) {
	abs := k.resolve(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (k *LocalKaos) ReadLines(_ context.Context, path string, _ string) (<-chan string, error) {
	abs := k.resolve(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, line := range lines {
			ch <- line
		}
	}()
	return ch, nil
}

func (k *LocalKaos) WriteBytes(_ context.Context, path string, data []byte) (int, error) {
	abs := k.resolve(path)
	err := os.WriteFile(abs, data, 0644)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (k *LocalKaos) WriteText(_ context.Context, path, data string, mode string) (int, error) {
	abs := k.resolve(path)
	var flag int
	if mode == "a" {
		flag = os.O_WRONLY | os.O_APPEND | os.O_CREATE
	} else {
		flag = os.O_WRONLY | os.O_TRUNC | os.O_CREATE
	}
	f, err := os.OpenFile(abs, flag, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.WriteString(data)
}

func (k *LocalKaos) Mkdir(_ context.Context, path string, parents, existOk bool) error {
	abs := k.resolve(path)
	if parents {
		return os.MkdirAll(abs, 0755)
	}
	err := os.Mkdir(abs, 0755)
	if err != nil && existOk && os.IsExist(err) {
		return nil
	}
	return err
}

func (k *LocalKaos) Exec(ctx context.Context, args ...string) (KaosProcess, error) {
	return k.ExecWithEnv(ctx, args, nil)
}

func (k *LocalKaos) ExecWithEnv(ctx context.Context, args []string, env map[string]string) (KaosProcess, error) {
	if len(args) == 0 {
		return nil, errors.New("kaos: no command provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = k.Getcwd()

	// Build environment
	var envSlice []string
	for key, val := range k.extraEnv {
		envSlice = append(envSlice, key+"="+val)
	}
	for key, val := range env {
		envSlice = append(envSlice, key+"="+val)
	}
	if len(envSlice) > 0 {
		cmd.Env = append(os.Environ(), envSlice...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &localProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

func (k *LocalKaos) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(k.Getcwd(), path)
}

// localProcess wraps os/exec.Cmd as a KaosProcess.
type localProcess struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   io.ReadCloser
	exitCode int
	done     bool
	mu       sync.Mutex
}

func (p *localProcess) Stdin() io.WriteCloser  { return p.stdin }
func (p *localProcess) Stdout() io.ReadCloser  { return p.stdout }
func (p *localProcess) Stderr() io.ReadCloser  { return p.stderr }
func (p *localProcess) Pid() int { return p.cmd.Process.Pid }

func (p *localProcess) ExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.done {
		return -1
	}
	return p.exitCode
}

func (p *localProcess) Wait(_ context.Context) (int, error) {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.done = true
	p.exitCode = p.cmd.ProcessState.ExitCode()
	p.mu.Unlock()
	return p.exitCode, err
}

func (p *localProcess) Kill(_ context.Context, _ string) error {
	return p.cmd.Process.Kill()
}

func (p *localProcess) Dispose() error {
	p.stdin.Close()
	p.stdout.Close()
	p.stderr.Close()
	return nil
}
