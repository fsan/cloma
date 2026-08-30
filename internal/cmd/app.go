package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// App constants.
const (
	// companionDir is the subdirectory inside the cloma source tree that holds
	// the Electron app.
	companionDir = "electron-app"

	// companionAppName is the produced .app bundle name on macOS.
	companionAppName = "Cloma.app"
)

var appForce bool

// appCmd represents the app command.
var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage the cloma menu bar app",
	Long: `Manage the cloma Electron app.

The app lives in the macOS menu bar and lets you view, start, stop,
inspect logs, and delete cloma sandboxes without using the terminal.

Subcommands:
  install   Build and install the app.
  update    Rebuild and reinstall the app (alias for install --force).
  uninstall Remove the app from /Applications.
  clean     Remove app build artifacts (dist, icons, cache).`,
}

// appInstallCmd builds and installs the app.
var appInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Build and install the app",
	Long: `Build the cloma Electron app and install it to the host
Applications directory.

By default this command builds from the app sources bundled
alongside the cloma source tree (./electron-app). Use --force to rebuild even
when the app appears to already be installed.`,
	RunE: runAppInstall,
}

// appUpdateCmd rebuilds and reinstalls the app.
var appUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the app to the latest build",
	Long: `Rebuild and reinstall the cloma app.

This is equivalent to ` + "`cloma app install --force`" + ` and ensures the
installed app matches the current sources.`,
	RunE: runAppUpdate,
}

// appCleanCmd removes app build artifacts.
var appCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove app build artifacts",
	Long: `Remove the cloma app build artifacts (dist, generated icons,
and cache) from the electron-app directory.

This does not uninstall the app from /Applications; it only cleans the
build outputs so the next build starts fresh.`,
	RunE: runAppClean,
}

// appUninstallCmd removes the app from /Applications.
var appUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the app from /Applications",
	Long: `Remove the cloma menu bar app from the host Applications
directory (/Applications and ~/Applications).

This does not remove the CLI binary; use ` + "`make uninstall`" + ` for that.`,
	RunE: runAppUninstall,
}

func init() {
	rootCmd.AddCommand(appCmd)
	appCmd.AddCommand(appInstallCmd)
	appCmd.AddCommand(appUpdateCmd)
	appCmd.AddCommand(appCleanCmd)
	appCmd.AddCommand(appUninstallCmd)

	appInstallCmd.Flags().BoolVarP(&appForce, "force", "f", false, "Rebuild even when the app is already installed")
	appUpdateCmd.Flags().BoolVarP(&appForce, "force", "f", true, "Rebuild even when the app is already installed")
}

// runAppInstall builds and installs the app.
func runAppInstall(cmd *cobra.Command, args []string) error {
	srcDir, err := findCompanionSources()
	if err != nil {
		return err
	}

	// Check prerequisites for the build.
	if _, err := exec.LookPath("python3"); err != nil {
		return fmt.Errorf("python3 is not installed or not in PATH")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("curl is not installed or not in PATH")
	}

	// Unless --force, skip the build if the app is already installed.
	if !appForce {
		if installed, _ := isCompanionInstalled(); installed {
			printGreen("cloma app is already installed. Use --force to rebuild.")
			return nil
		}
	}

	fmt.Println("Building and installing cloma app...")
	if err := buildAndInstallCompanion(srcDir); err != nil {
		return err
	}

	printGreen("cloma app installed.")
	fmt.Printf("Find it in your menu bar (the “C” icon) or launch it from Applications.\n")
	fmt.Printf("If macOS blocks it with “cannot be opened” or “Launch failed”, run:\n  %s/scripts/sign-app.sh \"/Applications/%s\"\n  xattr -cr \"/Applications/%s\"\n", srcDir, companionAppName, companionAppName)
	return nil
}

// runAppUpdate rebuilds and reinstalls the app.
func runAppUpdate(cmd *cobra.Command, args []string) error {
	// update is install --force.
	appForce = true
	return runAppInstall(cmd, args)
}

// runAppClean removes app build artifacts.
func runAppClean(cmd *cobra.Command, args []string) error {
	srcDir, err := findCompanionSources()
	if err != nil {
		return err
	}

	script := filepath.Join(srcDir, "build-app.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("build script not found: %s", script)
	}

	fmt.Println("Cleaning cloma app build artifacts...")
	c := exec.Command("bash", script, "--clean")
	c.Dir = srcDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("cloma app clean failed: %w", err)
	}
	return nil
}

// runAppUninstall removes the app from /Applications.
func runAppUninstall(cmd *cobra.Command, args []string) error {
	srcDir, err := findCompanionSources()
	if err != nil {
		return err
	}

	script := filepath.Join(srcDir, "build-app.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("build script not found: %s", script)
	}

	fmt.Println("Removing cloma app from /Applications...")
	c := exec.Command("bash", script, "--uninstall")
	c.Dir = srcDir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("cloma app uninstall failed: %w", err)
	}
	return nil
}

// findCompanionSources locates the electron-app directory. It first checks
// alongside the running binary (so a source checkout works), then falls back
// to a few well-known locations.
func findCompanionSources() (string, error) {
	// 1. Relative to the executable: <bin>/../electron-app or <bin>/../../electron-app.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		for _, rel := range []string{"..", "../.."} {
			cand := filepath.Join(exeDir, rel, companionDir)
			if isCompanionSourceDir(cand) {
				return cand, nil
			}
		}
	}

	// 2. Common development locations.
	if home, err := os.UserHomeDir(); err == nil {
		for _, base := range []string{
			filepath.Join(home, "workspace", "personal", "gloma", "cloma", companionDir),
			filepath.Join(home, "src", "cloma", companionDir),
			filepath.Join(home, "code", "cloma", companionDir),
			filepath.Join(home, "projects", "cloma", companionDir),
		} {
			if isCompanionSourceDir(base) {
				return base, nil
			}
		}
	}

	// 3. Current directory and its parents.
	if cwd, err := os.Getwd(); err == nil {
		if isCompanionSourceDir(filepath.Join(cwd, companionDir)) {
			return filepath.Join(cwd, companionDir), nil
		}
		// Walk up from cwd looking for an electron-app sibling.
		d := cwd
		for i := 0; i < 6; i++ {
			cand := filepath.Join(d, companionDir)
			if isCompanionSourceDir(cand) {
				return cand, nil
			}
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			d = parent
		}
	}

	return "", fmt.Errorf("could not find the app sources (looking for ./%s with a package.json)", companionDir)
}

// isCompanionSourceDir reports whether dir looks like the app root.
func isCompanionSourceDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil && !info.IsDir()
}

// buildAndInstallCompanion runs the build-app.sh script with --install in the
// app sources, building and installing in a single pass.
func buildAndInstallCompanion(srcDir string) error {
	script := filepath.Join(srcDir, "build-app.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("build script not found: %s", script)
	}
	cmd := exec.Command("bash", script, "--install")
	cmd.Dir = srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cloma app build/install failed: %w", err)
	}
	return nil
}

// isCompanionInstalled reports whether the app appears to be
// installed in /Applications or ~/Applications.
func isCompanionInstalled() (bool, error) {
	locations := []string{
		filepath.Join("/Applications", companionAppName),
	}
	if home, err := os.UserHomeDir(); err == nil {
		locations = append(locations, filepath.Join(home, "Applications", companionAppName))
	}
	for _, p := range locations {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}