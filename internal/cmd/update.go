package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// DefaultRepo is the canonical upstream cloma repository, used to discover
// release tags when --repo is not provided.
const DefaultRepo = "https://github.com/fsan/cloma.git"

// InstallDir is where `make install` copies the binary. Its writability
// decides whether the install step needs to be prefixed with sudo.
const InstallDir = "/usr/local/bin"

var (
	updateCheck   bool
	updateForce   bool
	updateRepo    string
	updateVersion string
)

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update cloma to the latest release",
	Long: `Update cloma to the latest tagged release.

This command discovers the latest vX.Y.Z tag from the upstream repository,
downloads the source, builds it, and installs the resulting binary to
` + InstallDir + `. Installing there usually requires root, so the install
step is run with sudo when ` + InstallDir + ` is not writable by the
current user.

Use --check to only report the latest available tag without changing the
installation. Use --version to install a specific tag instead of the latest.`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)

	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Only report the latest available tag; do not download, build, or install")
	updateCmd.Flags().BoolVarP(&updateForce, "force", "f", false, "Reinstall even when the local version is already the latest (or running dev)")
	updateCmd.Flags().StringVar(&updateRepo, "repo", DefaultRepo, "Git repository to fetch releases from")
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "Install a specific tag (e.g. v0.1.0) instead of the latest")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// git is required both to discover tags and to clone the source. Fail
	// early with an actionable hint instead of letting ls-remote fail opaquely.
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git is not installed or not in PATH\nHint: install git from https://git-scm.com/downloads")
	}
	if _, err := exec.LookPath("make"); err != nil {
		return fmt.Errorf("make is not installed or not in PATH")
	}

	repo := updateRepo
	if repo == "" {
		repo = DefaultRepo
	}

	// Discover the available tags from the remote.
	remoteTags, err := remoteTagSet(repo)
	if err != nil {
		return fmt.Errorf("failed to fetch tags from %s: %w", repo, err)
	}

	// Resolve the target tag: the one requested via --version, or the
	// highest semver among the remote tags.
	var targetTag string
	if updateVersion != "" {
		requested := normalizeTag(updateVersion)
		if !remoteTags[requested] {
			return fmt.Errorf("tag %s not found in %s", requested, repo)
		}
		targetTag = requested
	} else {
		targetTag, err = latestTag(remoteTags)
		if err != nil {
			return fmt.Errorf("no release tags found in %s: %w", repo, err)
		}
	}

	current := Version
	outdated := isOutdated(current, targetTag)

	// Report current vs. target.
	fmt.Printf("Current version: %s\n", current)
	fmt.Printf("Latest release: %s\n", targetTag)

	switch {
	case updateCheck:
		if outdated {
			printYellow("An update is available.")
		} else {
			printGreen("Already up to date.")
		}
		return nil
	case !outdated && !updateForce:
		printGreen("Already up to date. Use --force to reinstall.")
		return nil
	}

	if !outdated && updateForce {
		fmt.Println("Forcing reinstall of the latest version...")
	}

	// Download: shallow clone at the target tag. Preserves git metadata so the
	// Makefile's `git describe` produces the correct Version string.
	tmpDir, err := os.MkdirTemp("", "cloma-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("Downloading %s from %s...\n", targetTag, repo)
	cloneCmd := exec.Command("git", "clone", "--depth", "1", "--branch", targetTag, repo, tmpDir)
	if err := runStream(cloneCmd); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Build the binary.
	fmt.Println("Building cloma...")
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = tmpDir
	if err := runStream(buildCmd); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Install. Use sudo only when the install dir is not writable.
	useSudo := needsSudo(InstallDir)
	if useSudo {
		fmt.Printf("Installing cloma to %s (sudo required)...\n", InstallDir)
	} else {
		fmt.Printf("Installing cloma to %s...\n", InstallDir)
	}

	var installCmd *exec.Cmd
	if useSudo {
		installCmd = exec.Command("sudo", "make", "install")
	} else {
		installCmd = exec.Command("make", "install")
	}
	installCmd.Dir = tmpDir
	if err := runStream(installCmd); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	printGreen(fmt.Sprintf("cloma updated to %s", targetTag))
	fmt.Printf("Run `cloma version` to verify.\n")
	return nil
}

// remoteTagSet returns the set of tag names reported by `git ls-remote` for
// the given repository. Only `refs/tags/` references are considered.
func remoteTagSet(repo string) (map[string]bool, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", repo).Output()
	if err != nil {
		return nil, err
	}

	tags := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		// Lines look like: <sha>\trefs/tags/v0.1.0
		ref := strings.TrimSpace(line)
		if ref == "" {
			continue
		}
		fields := strings.SplitN(ref, "\t", 2)
		if len(fields) < 2 {
			continue
		}
		const prefix = "refs/tags/"
		name := fields[1]
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		tags[strings.TrimPrefix(name, prefix)] = true
	}
	return tags, nil
}

// latestTag returns the highest semantic version tag from the given set.
// Non-semver tags are ignored.
func latestTag(tags map[string]bool) (string, error) {
	var best string
	var bestSV semver
	found := false
	for tag := range tags {
		sv, ok := parseSemver(tag)
		if !ok {
			continue
		}
		if !found || compareSemver(sv, bestSV) > 0 {
			best = tag
			bestSV = sv
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("no semver tags (vX.Y.Z) found")
	}
	return best, nil
}

type semver struct {
	major, minor, patch int
}

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// parseSemver parses a "vX.Y.Z" (or "X.Y.Z") tag into a semver. The pre-release
// / build suffixes of git-describe output are stripped so "v0.1.0-1-gabc"
// compares as "v0.1.0".
func parseSemver(tag string) (semver, bool) {
	tag = strings.SplitN(tag, "-", 2)[0]
	m := semverRe.FindStringSubmatch(tag)
	if m == nil {
		return semver{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	return semver{maj, min, pat}, true
}

// compareSemver returns -1, 0, or 1.
func compareSemver(a, b semver) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	return 0
}

// isOutdated reports whether the current version is older than the target.
// Unreleased builds ("dev", empty, or git-describe-dirty forms that fail to
// parse cleanly) are always considered outdated.
func isOutdated(current, target string) bool {
	if current == "" || current == "dev" {
		return true
	}
	cur, okC := parseSemver(current)
	if !okC {
		return true
	}
	tgt, okT := parseSemver(target)
	if !okT {
		return false
	}
	return compareSemver(cur, tgt) < 0
}

// normalizeTag ensures a leading "v" on the requested tag so that "--version
// 0.1.0" and "--version v0.1.0" behave identically.
func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if !strings.HasPrefix(tag, "v") {
		return "v" + tag
	}
	return tag
}

// runStream runs a command with its stdout/stderr connected to the parent so
// the user sees clone/build/install output in real time.
func runStream(c *exec.Cmd) error {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// needsSudo reports whether writing to dir requires elevated privileges. It
// does so by attempting to create and remove a sentinel file; on any error
// it falls back to assuming sudo is needed.
func needsSudo(dir string) bool {
	sentinel := filepath.Join(dir, ".cloma-update-probe")
	if err := os.WriteFile(sentinel, []byte{}, 0644); err != nil {
		return true
	}
	_ = os.Remove(sentinel)
	return false
}
