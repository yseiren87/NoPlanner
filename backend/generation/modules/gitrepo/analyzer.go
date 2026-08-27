package gitrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrOutsideRepository = errors.New("path outside repository")

type File struct {
	Path, Language, ContentHash string
	Size                        uint64
	Symbols                     []string
}
type Commit struct{ Revision, Author, CommittedAt, Subject string }
type Analysis struct {
	Root, Revision string
	Files          []File
	History        []Commit
	Features       []string
}

func Allowed(path string, roots []string) bool {
	requested, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	requested, err = filepath.EvalSymlinks(requested)
	if err != nil {
		return false
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		allowed, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		allowed, err = filepath.EvalSymlinks(allowed)
		if err == nil && within(allowed, requested) {
			return true
		}
	}
	return false
}

func Analyze(ctx context.Context, path string, includes []string, historyLimit uint32) (Analysis, error) {
	for _, include := range includes {
		clean := filepath.Clean(include)
		if filepath.IsAbs(include) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Analysis{}, ErrOutsideRepository
		}
	}
	rootBytes, err := run(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return Analysis{}, err
	}
	root := strings.TrimSpace(string(rootBytes))
	requested, err := filepath.Abs(path)
	if err != nil {
		return Analysis{}, err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Analysis{}, err
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return Analysis{}, err
	}
	requested, err = filepath.EvalSymlinks(requested)
	if err != nil {
		return Analysis{}, err
	}
	if !within(rootAbs, requested) {
		return Analysis{}, ErrOutsideRepository
	}
	revisionBytes, err := run(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return Analysis{}, err
	}
	result := Analysis{Root: rootAbs, Revision: strings.TrimSpace(string(revisionBytes))}
	args := []string{"ls-files", "-z"}
	args = append(args, includes...)
	filesBytes, err := run(ctx, root, args...)
	if err != nil {
		return Analysis{}, err
	}
	for _, relative := range strings.Split(string(filesBytes), "\x00") {
		if relative == "" {
			continue
		}
		full := filepath.Join(rootAbs, relative)
		if !within(rootAbs, full) {
			return Analysis{}, ErrOutsideRepository
		}
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() > 2*1024*1024 {
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			return Analysis{}, err
		}
		sum := sha256.Sum256(content)
		file := File{Path: relative, Language: language(relative), Size: uint64(info.Size()), ContentHash: hex.EncodeToString(sum[:]), Symbols: symbols(string(content))}
		result.Files = append(result.Files, file)
		result.Features = appendUnique(result.Features, feature(relative))
	}
	if historyLimit == 0 {
		historyLimit = 20
	}
	logBytes, err := run(ctx, root, "log", "-n", strconv.FormatUint(uint64(historyLimit), 10), "--format=%H%x1f%an%x1f%aI%x1f%s%x1e")
	if err == nil {
		for _, row := range strings.Split(string(logBytes), "\x1e") {
			parts := strings.Split(strings.TrimSpace(row), "\x1f")
			if len(parts) == 4 {
				result.History = append(result.History, Commit{Revision: parts[0], Author: parts[1], CommittedAt: parts[2], Subject: parts[3]})
			}
		}
	}
	return result, nil
}

func ValidatePatch(ctx context.Context, root, baseRevision string, diffs []string) error {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ErrOutsideRepository
	}
	rootBytes, err := run(ctx, canonicalRoot, "rev-parse", "--show-toplevel")
	discoveredRoot, discoverErr := filepath.EvalSymlinks(strings.TrimSpace(string(rootBytes)))
	if err != nil || discoverErr != nil || discoveredRoot != canonicalRoot {
		return ErrOutsideRepository
	}
	revision, err := run(ctx, canonicalRoot, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(revision)) != strings.TrimSpace(baseRevision) {
		return errors.New("repository revision changed")
	}
	var input bytes.Buffer
	for _, diff := range diffs {
		lower := strings.ToLower(diff)
		if strings.Contains(lower, "todo: wire") || strings.Contains(lower, "placeholder") || strings.Contains(lower, "return true, nil") {
			return errors.New("patch contains a non-verifying placeholder")
		}
		input.WriteString(diff)
		if !strings.HasSuffix(diff, "\n") {
			input.WriteByte('\n')
		}
	}
	command := exec.CommandContext(ctx, "git", "apply", "--check", "--whitespace=error-all", "-")
	command.Dir = canonicalRoot
	command.Stdin = &input
	if output, err := command.CombinedOutput(); err != nil {
		return errors.New("patch does not apply: " + strings.TrimSpace(string(output)))
	}
	return nil
}
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	return command.Output()
}
func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func language(path string) string {
	return map[string]string{".go": "go", ".ts": "typescript", ".tsx": "typescript-react", ".js": "javascript", ".py": "python", ".java": "java", ".md": "markdown", ".proto": "protobuf"}[strings.ToLower(filepath.Ext(path))]
}
func symbols(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range []string{"func ", "type ", "export function ", "export class ", "message ", "service "} {
			if strings.HasPrefix(trimmed, prefix) {
				name := strings.Fields(strings.TrimPrefix(trimmed, prefix))
				if len(name) > 0 {
					out = append(out, strings.Trim(name[0], "{}()"))
				}
			}
		}
		if len(out) >= 40 {
			break
		}
	}
	return out
}
func feature(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) > 1 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}
func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}
