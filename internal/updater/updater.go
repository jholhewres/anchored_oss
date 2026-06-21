// Package updater performs in-place self-updates of the anchored-oss server
// binary. It compares the running version against the published VERSION.md,
// downloads the matching release asset for the current OS/arch, verifies its
// SHA-256 against the release checksums file, atomically swaps the binary with
// backup/rollback, and asks pm2 to restart the (now-updated) process.
//
// The server is expected to run under pm2 in self-host deployments; if pm2 is
// absent or the restart fails, the binary is already swapped, so we log a
// warning and leave the operator to restart manually rather than failing.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/version"
)

// ErrNoUpdate is returned by Apply when CheckLatest reports no available update
// (already current, a downgrade, or an unparseable current version). Callers
// should treat it as a benign 409, not a server error.
var ErrNoUpdate = errors.New("no update available")

// maxDownloadBytes caps a single release-asset download to guard against a
// runaway or hostile response body exhausting disk.
const maxDownloadBytes = 512 << 20 // 512 MiB

// pm2RestartTimeout bounds the pm2 restart independently of the apply context so
// a nearly-exhausted apply deadline can't cancel the restart mid-swap.
const pm2RestartTimeout = 30 * time.Second

const (
	// defaultVersionURL is the canonical location of the published version
	// marker. The first line is expected to be a vX.Y.Z tag.
	defaultVersionURL = "https://raw.githubusercontent.com/jholhewres/anchored_oss/main/VERSION.md"
	// defaultReleaseAssetBase is the per-tag release asset directory; the tag
	// is appended by the downloader.
	defaultReleaseAssetBase = "https://github.com/jholhewres/anchored_oss/releases/download/"
	// defaultPM2Name is the pm2 process name restarted after a swap; overridable
	// via ANCHORED_PM2_NAME.
	defaultPM2Name = "anchored-oss"
)

// Updater checks for and applies server self-updates. Its fields are
// overridable so tests can point VersionURL/ReleaseAssetBase at httptest
// servers and avoid touching the real binary.
type Updater struct {
	VersionURL       string
	ReleaseAssetBase string
	PM2Name          string
	HTTPClient       *http.Client
	Logger           *slog.Logger
}

// New returns an Updater with production defaults. ANCHORED_PM2_NAME overrides
// the pm2 process name.
func New(logger *slog.Logger) *Updater {
	if logger == nil {
		logger = slog.Default()
	}
	pm2 := os.Getenv("ANCHORED_PM2_NAME")
	if pm2 == "" {
		pm2 = defaultPM2Name
	}
	return &Updater{
		VersionURL:       defaultVersionURL,
		ReleaseAssetBase: defaultReleaseAssetBase,
		PM2Name:          pm2,
		HTTPClient:       &http.Client{Timeout: 5 * time.Minute},
		Logger:           logger.With("component", "updater"),
	}
}

func (u *Updater) client() *http.Client {
	if u.HTTPClient != nil {
		return u.HTTPClient
	}
	return http.DefaultClient
}

// CheckLatest fetches the published version and compares it against the running
// build. available is true only when the latest parses as semver and is newer
// than current. A non-semver current version (e.g. "dev") is treated
// conservatively: available stays false, since we cannot prove the published
// build is newer than an unversioned local build.
func (u *Updater) CheckLatest(ctx context.Context) (current, latest string, available bool, err error) {
	current = version.Version

	latest, err = u.fetchLatestVersion(ctx)
	if err != nil {
		return current, "", false, err
	}

	latestSV, ok := parseSemver(latest)
	if !ok {
		return current, latest, false, fmt.Errorf("published version %q is not semver", latest)
	}
	currentSV, ok := parseSemver(current)
	if !ok {
		// Unparseable current (dev build): be conservative, do not offer update.
		return current, latest, false, nil
	}

	return current, latest, compareSemver(latestSV, currentSV) > 0, nil
}

func (u *Updater) fetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.VersionURL, nil)
	if err != nil {
		return "", fmt.Errorf("build version request: %w", err)
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch version: HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	if !sc.Scan() {
		return "", fmt.Errorf("version file is empty")
	}
	return strings.TrimSpace(sc.Text()), nil
}

// assetName is the release binary name for the current platform.
func assetName() string {
	name := fmt.Sprintf("anchored-oss-selfhosted-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// Apply downloads the latest release asset for this platform, verifies its
// checksum, swaps it in for the running binary (with rollback on failure), and
// asks pm2 to restart. A pm2 failure is logged but not returned: the binary is
// already swapped, so the next manual restart picks up the new version.
func (u *Updater) Apply(ctx context.Context) error {
	_, latest, available, err := u.CheckLatest(ctx)
	if err != nil {
		return fmt.Errorf("resolve latest version: %w", err)
	}
	if !available {
		return ErrNoUpdate
	}

	tmpDir, err := os.MkdirTemp("", "anchored-oss-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	asset := assetName()
	base := strings.TrimRight(u.ReleaseAssetBase, "/") + "/" + latest + "/"

	binPath := filepath.Join(tmpDir, asset)
	if err := u.downloadFile(ctx, base+asset, binPath); err != nil {
		return fmt.Errorf("download binary: %w", err)
	}

	sumsPath := filepath.Join(tmpDir, "checksums-sha256.txt")
	if err := u.downloadFile(ctx, base+"checksums-sha256.txt", sumsPath); err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	want, err := checksumFor(sumsPath, asset)
	if err != nil {
		return fmt.Errorf("locate checksum: %w", err)
	}
	if err := verifySHA256(binPath, want); err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}

	// Probe the downloaded binary before swapping it in. A cgo/dynamic binary
	// built against glibc fails to start on musl/Alpine (dynamic-link error);
	// aborting here keeps the current binary in place instead of bricking the
	// deployment on the next pm2 restart.
	if err := probeBinary(binPath, u.Logger); err != nil {
		return fmt.Errorf("new binary failed to start on this host; update aborted (keeping current version): %w", err)
	}

	if err := swapBinary(binPath, u.Logger); err != nil {
		return err
	}

	u.restartPM2()
	return nil
}

// downloadFile streams url into dest.
func (u *Updater) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	// Cap the copy so a runaway/hostile body can't fill the disk. +1 lets us
	// detect an over-limit body rather than silently truncating it.
	n, err := io.Copy(out, io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return err
	}
	if n > maxDownloadBytes {
		return fmt.Errorf("download exceeds %d byte limit", maxDownloadBytes)
	}
	return nil
}

// restartPM2 asks pm2 to restart the server process. Best-effort: a missing or
// failing pm2 is logged, not returned, because the binary is already swapped.
// It uses a fresh, short context derived from context.Background() so a
// nearly-exhausted apply deadline can't cancel the restart mid-swap.
func (u *Updater) restartPM2() {
	ctx, cancel := context.WithTimeout(context.Background(), pm2RestartTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pm2", "restart", u.PM2Name)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		u.Logger.Warn("pm2 restart failed; restart manually to load the new binary",
			"pm2_name", u.PM2Name, "error", err)
	}
}

// checksumFor reads a "<hex>  <filename>" checksums file and returns the hex
// digest for the named file.
func checksumFor(sumsPath, fileName string) (string, error) {
	f, err := os.Open(sumsPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		// The filename column may carry a leading "*" (binary mode marker).
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == fileName {
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no checksum entry for %q", fileName)
}

// verifySHA256 hashes path and compares it to wantHex (case-insensitive).
func verifySHA256(path, wantHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantHex) {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, wantHex)
	}
	return nil
}

// probeBinary runs the candidate binary with -version to confirm it starts on
// the current host. A glibc/musl mismatch (or any dynamic-link failure) makes
// the exec fail before any Go code runs; a short timeout bounds hangs. Used
// before swapBinary so an incompatible release is rejected instead of bricking
// the deployment.
func probeBinary(path string, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if logger != nil {
			logger.Error("new binary probe failed; aborting update to avoid brick",
				"binary", path, "error", err, "output", string(out))
		}
		return fmt.Errorf("probe -version: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// swapBinary replaces the running executable with newBinary, keeping a .bak for
// rollback on failure. The new binary is made executable (0755).
func swapBinary(newBinary string, logger *slog.Logger) error {
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	backup := current + ".bak"
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := copyFile(newBinary, current); err != nil {
		if rbErr := os.Rename(backup, current); rbErr != nil {
			return fmt.Errorf("copy new binary failed (%w) AND rollback failed (%v)", err, rbErr)
		}
		return fmt.Errorf("copy new binary (rolled back): %w", err)
	}

	if err := os.Chmod(current, 0755); err != nil {
		_ = os.Remove(current)
		if rbErr := os.Rename(backup, current); rbErr != nil {
			return fmt.Errorf("chmod new binary failed (%w) AND rollback failed (%v)", err, rbErr)
		}
		return fmt.Errorf("chmod new binary (rolled back): %w", err)
	}

	if logger != nil {
		logger.Info("binary swapped", "path", current, "backup", backup)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// semver is a parsed vX.Y.Z (pre-release/build metadata ignored).
type semver struct{ major, minor, patch int }

// parseSemver accepts "vX.Y.Z" or "X.Y.Z" (with optional "-pre"/"+build"
// suffix). Returns ok=false for anything that does not start with X.Y.Z.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Drop pre-release / build metadata.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var out semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		switch i {
		case 0:
			out.major = n
		case 1:
			out.minor = n
		case 2:
			out.patch = n
		}
	}
	return out, true
}

// compareSemver returns >0 if a>b, <0 if a<b, 0 if equal.
func compareSemver(a, b semver) int {
	if a.major != b.major {
		return a.major - b.major
	}
	if a.minor != b.minor {
		return a.minor - b.minor
	}
	return a.patch - b.patch
}
