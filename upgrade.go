// Self-upgrade: clone the public repo into a temp dir, build it with the Go
// toolchain, and replace the running notie binary in place. Backs `notie upgrade`.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// repoURL is the public source cloned by `notie upgrade`; override with
// NOTIE_REPO to upgrade from a fork or a local checkout.
func repoURL() string {
	if r := os.Getenv("NOTIE_REPO"); r != "" {
		return r
	}
	return "https://github.com/piyushKumar-1/notie.git"
}

// goToolchain finds the Go compiler: the one on PATH, else the toolchain
// setup.sh downloads to ~/.cache/notie/go when Go isn't installed. That script
// puts it on PATH only for its own lifetime, so without this fallback an
// install created that way could never upgrade itself.
func goToolchain() string {
	if bin, err := exec.LookPath("go"); err == nil {
		return bin
	}
	if home, err := os.UserHomeDir(); err == nil {
		cached := filepath.Join(home, ".cache", "notie", "go", "bin", "go")
		if st, err := os.Stat(cached); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return cached
		}
	}
	fatal("the Go toolchain is required to build the upgrade — install from https://go.dev/dl/ (or: brew install go)")
	return ""
}

// remoteRevision asks the remote for the commit a clone would land on — one
// round-trip, nothing written to disk. HEAD rather than a branch name so it
// agrees with `git clone` for any NOTIE_REPO, including a local checkout on
// some other branch. Empty when the remote can't be reached, which leaves the
// caller to clone and find out the hard way.
func remoteRevision(git, url string) string {
	out, err := exec.Command(git, "ls-remote", url, "HEAD").Output()
	if err != nil {
		return ""
	}
	if f := strings.Fields(string(out)); len(f) > 0 && len(f[0]) == 40 {
		return f[0]
	}
	return ""
}

// cmdUpgrade clones the public repo, builds it, and installs the fresh binary
// over the one that's running, so `notie upgrade` is entirely self-contained.
// --check reports what is available and installs nothing; --force rebuilds even
// when the running binary already matches the remote.
func cmdUpgrade(args []string) {
	check, force := false, false
	for _, a := range args {
		switch a {
		case "--check", "-n":
			check = true
		case "--force", "-f":
			force = true
		default:
			fatal("usage: notie upgrade [--check] [--force]")
		}
	}

	git, err := exec.LookPath("git")
	if err != nil {
		fatal("git is required to upgrade — install it and retry")
	}
	url := repoURL()

	// Compare revisions before doing any work: a clone plus a full rebuild is a
	// poor way to discover there was nothing to upgrade to. A binary built from
	// a modified tree never counts as up to date — its revision describes the
	// commit it started from, not what it actually contains.
	have, dirty := selfRevision()
	want := remoteRevision(git, url)
	current := want != "" && want == have && !dirty

	if check {
		switch {
		case current:
			fmt.Printf("up to date (%s)\n", shortRev(have))
		case want == "":
			fmt.Printf("cannot reach %s — run 'notie upgrade' to try anyway\n", url)
		case dirty:
			fmt.Printf("upgrade available: %s (this binary was built from a modified tree)\n", shortRev(want))
		default:
			fmt.Printf("upgrade available: %s → %s\n", shortRev(have), shortRev(want))
		}
		return
	}
	if current && !force {
		fmt.Printf("already up to date (%s) — notie upgrade --force rebuilds anyway\n", shortRev(have))
		return
	}

	goBin := goToolchain()

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
	// fatal() exits the process, so the defer above never runs on the error
	// paths below — each would otherwise strand a half-made clone in TMPDIR.
	die := func(format string, a ...any) {
		os.RemoveAll(tmp)
		fatal(format, a...)
	}
	src := filepath.Join(tmp, "notie")

	fmt.Printf("cloning %s…\n", url)
	if err := runStreaming(git, "", "clone", "--depth", "1", url, src); err != nil {
		die("git clone failed: %v", err)
	}

	fmt.Println("building…")
	if err := runStreaming(goBin, src, "build", "-o", "notie", "."); err != nil {
		die("build failed: %v", err)
	}

	if err := replaceBinary(filepath.Join(src, "notie"), dst); err != nil {
		die("installing to %s: %v\n  (if it's a system path, retry with elevated permissions)", dst, err)
	}
	fmt.Printf("notie upgraded → %s\n", dst)
	if have != "" && want != "" && have != want {
		fmt.Printf("  %s → %s\n", shortRev(have), shortRev(want))
	}
	refreshSkill(src)
}

// refreshSkill re-copies the notie-review skill from the freshly cloned source,
// but only where it is already installed — setup.sh asks before installing it
// and an upgrade must not silently reverse a "no". Best-effort: the binary is
// already upgraded by the time this runs.
func refreshSkill(src string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dst := filepath.Join(home, ".claude", "skills", "notie-review", "SKILL.md")
	if _, err := os.Stat(dst); err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(src, ".claude", "skills", "notie-review", "SKILL.md"))
	if err != nil {
		return
	}
	if os.WriteFile(dst, data, 0o644) == nil {
		fmt.Println("notie-review skill refreshed")
	}
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
