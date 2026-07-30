// Self-upgrade: clone the public repo into a temp dir, build it with the Go
// toolchain, and replace the running notie binary in place. Backs `notie upgrade`.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// repoURL is the public source cloned by `notie upgrade`; override with
// NOTIE_REPO to upgrade from a fork or a local checkout.
func repoURL() string {
	if r := os.Getenv("NOTIE_REPO"); r != "" {
		return r
	}
	return "https://github.com/piyushKumar-1/notie.git"
}

// cmdUpgrade clones the public repo, builds it, and installs the fresh binary
// over the one that's running, so `notie upgrade` is entirely self-contained.
func cmdUpgrade() {
	git, err := exec.LookPath("git")
	if err != nil {
		fatal("git is required to upgrade — install it and retry")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		fatal("the Go toolchain is required to build the upgrade — install from https://go.dev/dl/ (or: brew install go)")
	}

	// Install target: the running binary's own path, resolved through any
	// symlink, so the upgrade lands exactly where notie is invoked from.
	dst, err := os.Executable()
	if err != nil {
		fatal("cannot locate the running notie binary: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(dst); err == nil {
		dst = resolved
	}

	tmp, err := os.MkdirTemp("", "notie-upgrade-")
	if err != nil {
		fatal("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "notie")

	url := repoURL()
	fmt.Printf("cloning %s…\n", url)
	if err := runStreaming(git, "", "clone", "--depth", "1", url, src); err != nil {
		fatal("git clone failed: %v", err)
	}

	fmt.Println("building…")
	if err := runStreaming(goBin, src, "build", "-o", "notie", "."); err != nil {
		fatal("build failed: %v", err)
	}

	if err := replaceBinary(filepath.Join(src, "notie"), dst); err != nil {
		fatal("installing to %s: %v\n  (if it's a system path, retry with elevated permissions)", dst, err)
	}
	fmt.Printf("notie upgraded → %s\n", dst)
}

// runStreaming runs a command in dir (cwd if empty) with output passed through
// to the user, so clone/build progress is visible.
func runStreaming(bin, dir string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// replaceBinary installs the freshly built binary at dst via a same-directory
// temp file plus atomic rename — this swaps the directory entry without
// touching the inode the current process is executing, so overwriting the
// running binary is safe.
func replaceBinary(built, dst string) error {
	data, err := os.ReadFile(built)
	if err != nil {
		return err
	}
	tmp := dst + ".upgrade"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
