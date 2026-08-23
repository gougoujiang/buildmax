// Package adapter runs a trial against a built BuildMax artifact and returns a
// canonical bundle. It is the black-box half of the evaluation system: nothing
// here calls the agent runtime in process, because an in-process helper cannot
// tell whether the shipped binary behaves the way the library does.
package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/evaluation/contract"
)

// Materialize copies a task's visible initial state into the trial workspace.
//
// Only contract.StateDir is copied. The grader, the oracle, and the task
// definition stay in the task directory, which the trial never sees, so a task
// cannot leak its own answer by listing the wrong path: there is no path to
// list. A task with no state directory yields an empty workspace, which is
// valid — some tasks start from nothing.
func Materialize(taskDir, workspace string) error {
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	state := filepath.Join(taskDir, contract.StateDir)
	info, err := os.Stat(state)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read initial state: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("initial state %s is not a directory", state)
	}
	return copyTree(state, workspace)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			// A symlink in the initial state would let a task point the agent
			// outside the workspace it is supposed to be confined to, and the
			// trial boundary is the thing being measured.
			return fmt.Errorf("initial state contains symlink %s; the trial boundary cannot survive it", rel)
		case !d.Type().IsRegular():
			return fmt.Errorf("initial state contains irregular file %s", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// DigestDir returns a content identity for a directory tree: the SHA-256 over
// every path, mode, and body, in sorted order.
//
// Section 7.1 makes final state the authoritative outcome, and a digest is what
// lets a bundle carry that fact without carrying the workspace. Sorting is what
// makes it comparable at all — filesystem walk order differs between machines,
// so an unsorted digest would report every re-run as a changed outcome.
func DigestDir(root string) (string, error) {
	type entry struct {
		rel  string
		mode fs.FileMode
		sum  []byte
		link string
	}
	var entries []entry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			// A directory contributes its own entry so an empty one is part of
			// the state: a task whose outcome is "created the directory" would
			// otherwise digest identically before and after.
			entries = append(entries, entry{rel: rel + "/", mode: info.Mode().Perm() | fs.ModeDir})
			return nil
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, entry{rel: rel, mode: fs.ModeSymlink, link: target})
			return nil
		case !d.Type().IsRegular():
			// Sockets and devices have no content a re-run could reproduce, so
			// naming one is more honest than hashing whatever it reads as.
			entries = append(entries, entry{rel: rel, mode: info.Mode().Type()})
			return nil
		}
		sum, err := digestFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, entry{rel: rel, mode: info.Mode().Perm(), sum: sum})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("digest %s: %w", root, err)
		}
		return "", fmt.Errorf("digest %s: %w", root, err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	for _, e := range entries {
		// The separators are part of the input so two different trees cannot
		// concatenate into the same byte stream.
		fmt.Fprintf(h, "%s\x00%o\x00%s\x00", e.rel, e.mode, e.link)
		h.Write(e.sum)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func digestFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// VerifyBoundary reports files in the workspace whose content matches hidden
// task material. It is the adversarial half of section 18.4: keeping graders
// out of the copy is the mechanism, and this is the check that the mechanism
// worked — including when the leak came from a task author committing the
// answer into the initial state rather than from the copier.
//
// It returns the offending workspace paths, empty when the boundary held.
func VerifyBoundary(taskDir, workspace string) ([]string, error) {
	hidden := map[string]string{}
	for _, dir := range []string{contract.GradersDir, contract.OracleDir} {
		root := filepath.Join(taskDir, dir)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return fs.SkipAll
				}
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			sum, err := digestFile(path)
			if err != nil {
				return err
			}
			// Empty files carry no answer and would match anything empty the
			// agent happened to create.
			if info, err := d.Info(); err == nil && info.Size() == 0 {
				return nil
			}
			hidden[hex.EncodeToString(sum)] = filepath.ToSlash(filepath.Join(dir, strings.TrimPrefix(path, root)))
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read hidden material: %w", err)
		}
	}
	if len(hidden) == 0 {
		return nil, nil
	}

	var leaked []string
	err := filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		sum, err := digestFile(path)
		if err != nil {
			return err
		}
		if _, ok := hidden[hex.EncodeToString(sum)]; ok {
			rel, _ := filepath.Rel(workspace, path)
			leaked = append(leaked, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	sort.Strings(leaked)
	return leaked, nil
}
