package gitrepo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeIsReadOnlyAndScoped(t *testing.T) {
	dir := t.TempDir()
	runTest(t, dir, "init")
	runTest(t, dir, "config", "user.email", "test@example.com")
	runTest(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Run() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runTest(t, dir, "add", "main.go")
	runTest(t, dir, "commit", "-m", "initial")
	before := status(t, dir)
	result, err := Analyze(context.Background(), dir, nil, 5)
	if err != nil || len(result.Files) != 1 || result.Files[0].Symbols[0] != "Run" || before != status(t, dir) {
		t.Fatalf("analysis changed repository: %#v %v", result, err)
	}
}
func TestAnalyzeRejectsIncludeOutsideRepository(t *testing.T) {
	if _, err := Analyze(context.Background(), t.TempDir(), []string{"../secret"}, 1); !errors.Is(err, ErrOutsideRepository) {
		t.Fatalf("outside include must be rejected: %v", err)
	}
}
func TestAllowedRepositoryRootsRejectOutsideAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	outside := t.TempDir()
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	if !Allowed(inside, []string{root}) || Allowed(outside, []string{root}) {
		t.Fatal("allowed root boundary failed")
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if Allowed(link, []string{root}) {
		t.Fatal("symlink escape was allowed")
	}
}

func TestValidatePatchRequiresApplicableNonPlaceholderDiff(t *testing.T) {
	dir := t.TempDir()
	runTest(t, dir, "init")
	runTest(t, dir, "config", "user.email", "test@example.com")
	runTest(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Login() bool { return true }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runTest(t, dir, "add", "main.go")
	runTest(t, dir, "commit", "-m", "initial")
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = dir
	revision, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	valid := "--- /dev/null\n+++ b/main_test.go\n@@ -0,0 +1,3 @@\n+package main\n+\n+// verifies Login\n"
	if err = ValidatePatch(context.Background(), dir, strings.TrimSpace(string(revision)), []string{valid}); err != nil {
		t.Fatalf("valid patch rejected: %v", err)
	}
	if err = ValidatePatch(context.Background(), dir, strings.TrimSpace(string(revision)), []string{"--- /dev/null\n+++ b/fake.go\n@@ -0,0 +1 @@\n+func fake() (bool, error) { return true, nil }\n"}); err == nil {
		t.Fatal("placeholder patch accepted")
	}
	if err = ValidatePatch(context.Background(), dir, strings.TrimSpace(string(revision)), []string{"--- /dev/null\n+++ b/broken.go\n@@ -0,0 +1,4 @@\n+broken\n"}); err == nil {
		t.Fatal("malformed patch accepted")
	}
}
func runTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", err, output)
	}
}
func status(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	value, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}
