package kaos

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectEnvironment(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" && env.OsKind != OsKindMacOS {
		t.Fatalf("expected macOS, got %s", env.OsKind)
	}
	if runtime.GOOS == "linux" && env.OsKind != OsKindLinux {
		t.Fatalf("expected Linux, got %s", env.OsKind)
	}
	if env.ShellName != ShellBash && env.ShellName != ShellSh {
		t.Fatalf("unexpected shell: %s", env.ShellName)
	}
}

func TestLocalKaos_PathOps(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil {
		t.Skip("cannot detect environment")
	}
	k, err := NewLocalKaos(env)
	if err != nil {
		t.Fatal(err)
	}

	if k.Name() != "local" {
		t.Fatalf("expected name 'local', got %q", k.Name())
	}

	home := k.Gethome()
	if home == "" {
		t.Fatal("expected non-empty home")
	}

	cwd := k.Getcwd()
	if cwd == "" {
		t.Fatal("expected non-empty cwd")
	}

	norm := k.Normpath("/a/b/../c")
	expected := filepath.Clean("/a/b/../c")
	if norm != expected {
		t.Fatalf("normpath: got %q, want %q", norm, expected)
	}
}

func TestLocalKaos_FileOps(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil {
		t.Skip("cannot detect environment")
	}
	k, err := NewLocalKaos(env)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	k2 := k.WithCwd(dir)

	ctx := context.Background()

	// WriteText
	n, err := k2.WriteText(ctx, "test.txt", "hello world", "w")
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected bytes written > 0")
	}

	// ReadText
	text, err := k2.ReadText(ctx, "test.txt", "strict")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" {
		t.Fatalf("got %q, want %q", text, "hello world")
	}

	// ReadBytes
	data, err := k2.ReadBytes(ctx, "test.txt", 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", string(data), "hello")
	}

	// WriteBytes
	n, err = k2.WriteBytes(ctx, "binary.bin", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("got %d bytes, want 3", n)
	}

	// Append mode
	n, err = k2.WriteText(ctx, "test.txt", " appended", "a")
	if err != nil {
		t.Fatal(err)
	}
	text, err = k2.ReadText(ctx, "test.txt", "strict")
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world appended" {
		t.Fatalf("got %q, want %q", text, "hello world appended")
	}
}

func TestLocalKaos_DirOps(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil {
		t.Skip("cannot detect environment")
	}
	k, err := NewLocalKaos(env)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	k2 := k.WithCwd(dir)
	ctx := context.Background()

	// Mkdir
	err = k2.Mkdir(ctx, "subdir", true, false)
	if err != nil {
		t.Fatal(err)
	}

	// Write a file in subdir
	_, err = k2.WriteText(ctx, filepath.Join("subdir", "file.txt"), "content", "w")
	if err != nil {
		t.Fatal(err)
	}

	// Iterdir
	ch, err := k2.Iterdir(ctx, "subdir")
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	for name := range ch {
		entries = append(entries, name)
	}
	if len(entries) != 1 || entries[0] != "file.txt" {
		t.Fatalf("iterdir: got %v", entries)
	}

	// Stat
	stat, err := k2.Stat(ctx, filepath.Join("subdir", "file.txt"), true)
	if err != nil {
		t.Fatal(err)
	}
	if stat.StSize != 7 {
		t.Fatalf("stat size: got %d, want 7", stat.StSize)
	}

	// Chdir
	err = k2.Chdir(ctx, "subdir")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(k2.Getcwd()) != "subdir" {
		t.Fatalf("chdir: cwd is %q", k2.Getcwd())
	}
}

func TestLocalKaos_Exec(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil {
		t.Skip("cannot detect environment")
	}
	k, err := NewLocalKaos(env)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	proc, err := k.Exec(ctx, "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Dispose()

	if proc.Pid() <= 0 {
		t.Fatal("expected positive PID")
	}

	code, err := proc.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestLocalKaos_WithEnv(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil {
		t.Skip("cannot detect environment")
	}
	k, err := NewLocalKaos(env)
	if err != nil {
		t.Fatal(err)
	}

	k2 := k.WithEnv(map[string]string{"TEST_VAR": "test_value"})
	if k2.Name() != "local" {
		t.Fatal("expected local name")
	}
	_ = k2 // Just verify it doesn't panic
}

func TestLocalKaos_ReadLines(t *testing.T) {
	env, err := DetectEnvironment()
	if err != nil {
		t.Skip("cannot detect environment")
	}
	k, err := NewLocalKaos(env)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	k2 := k.WithCwd(dir)
	ctx := context.Background()

	_, err = k2.WriteText(ctx, "lines.txt", "line1\nline2\nline3", "w")
	if err != nil {
		t.Fatal(err)
	}

	ch, err := k2.ReadLines(ctx, "lines.txt", "strict")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for line := range ch {
		lines = append(lines, line)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}
