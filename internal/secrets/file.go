// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const MaxSecretSize = 64 << 10

type FileInspector struct{}

func (FileInspector) InspectMetadata(_ context.Context, ref Reference) (Metadata, error) {
	if ref.Provider != ProviderFile || ref.Locator == "" {
		return Metadata{}, errors.New("invalid file secret reference")
	}
	file, identity, err := openValidatedFile(ref.Locator)
	if err != nil {
		return Metadata{}, err
	}
	if err := file.Close(); err != nil {
		return Metadata{}, fmt.Errorf("close validated secret file: %w", err)
	}
	return Metadata{Provider: ProviderFile, Locator: ref.Locator, Identity: identity}, nil
}

func openValidatedFile(path string) (*os.File, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, "", errors.New("secret file path must be clean and absolute")
	}
	if err := validateParentDirectories(filepath.Dir(path)); err != nil {
		return nil, "", err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, "", fmt.Errorf("open secret file without following links: %w", err)
	}
	identity, err := validateOpenFile(file)
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}
	return file, identity, nil
}

func validateParentDirectories(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect secret parent directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("secret parent %q must be a real directory", path)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("secret parent %q must not be writable by group or others", path)
		}
		if path == string(filepath.Separator) {
			return nil
		}
		next := filepath.Dir(path)
		if next == path {
			return nil
		}
		path = next
	}
}

func validateOpenFile(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect open secret file: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("secret file metadata is unavailable")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("secret source must be a regular file")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return "", errors.New("secret file must be owned by the effective user")
	}
	if info.Mode().Perm() & ^os.FileMode(0o600) != 0 {
		return "", errors.New("secret file mode must be no more permissive than 0600")
	}
	if stat.Nlink != 1 {
		return "", errors.New("secret file must have exactly one hard link")
	}
	if info.Size() < 0 || info.Size() > MaxSecretSize {
		return "", fmt.Errorf("secret file exceeds %d-byte limit", MaxSecretSize)
	}
	identity := fmt.Sprintf("%x-%x-%x-%x-%x", stat.Dev, stat.Ino, info.Size(), stat.Mtim.Sec, stat.Mtim.Nsec)
	return identity, nil
}

// Preparer creates operation-scoped Podman secrets from already validated file
// descriptors. The returned cleanup must run after container creation or error.
type Preparer struct {
	PodmanBinary string
}

func (p Preparer) PrepareService(ctx context.Context, app, service string, mounts []ResolvedMount) ([]ResolvedMount, func(context.Context) error, error) {
	prepared := append([]ResolvedMount(nil), mounts...)
	created := make([]string, 0, len(mounts))
	cleanup := func(cleanupCtx context.Context) error {
		var errs []error
		for i := len(created) - 1; i >= 0; i-- {
			if err := p.removeSecret(cleanupCtx, created[i]); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	for i := range prepared {
		mount := &prepared[i]
		switch mount.Provider {
		case ProviderPodman:
			mount.RuntimeIdentity = mount.Identity
		case ProviderFile:
			file, identity, err := openValidatedFile(mount.Locator)
			if err != nil {
				_ = cleanup(context.Background())
				return nil, nil, fmt.Errorf("secret %q changed or became unsafe: %w", mount.Name, err)
			}
			if identity != mount.Identity {
				_ = file.Close()
				_ = cleanup(context.Background())
				return nil, nil, fmt.Errorf("secret %q file identity changed after authorization", mount.Name)
			}
			id, err := p.stageFile(ctx, app, service, mount.Target, file)
			if err == nil {
				mount.RuntimeIdentity = id
				created = append(created, id)
				var after string
				after, err = validateOpenFile(file)
				if err == nil && after != identity {
					err = fmt.Errorf("secret %q file changed while it was staged", mount.Name)
				}
			}
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				_ = cleanup(context.Background())
				if err != nil {
					return nil, nil, err
				}
				return nil, nil, fmt.Errorf("close staged secret source: %w", closeErr)
			}
		default:
			_ = cleanup(context.Background())
			return nil, nil, fmt.Errorf("secret %q has unsupported provider %q", mount.Name, mount.Provider)
		}
	}
	return prepared, cleanup, nil
}

func (p Preparer) stageFile(ctx context.Context, app, service, target string, file *os.File) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate transient secret identity: %w", err)
	}
	name := "orisema-transient-" + hex.EncodeToString(random)
	bin := p.PodmanBinary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "secret", "create",
		"--label", "dev.orisema.transient=true",
		"--label", "dev.orisema.app="+app,
		"--label", "dev.orisema.service="+service,
		"--label", "dev.orisema.target="+target,
		name, "-")
	cmd.Stdin = io.LimitReader(file, MaxSecretSize)
	out, err := cmd.Output()
	if err != nil {
		_ = p.removeSecret(context.Background(), name)
		return "", fmt.Errorf("stage file secret in Podman: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if !podmanSecretIdentity.MatchString(id) {
		_ = p.removeSecret(context.Background(), name)
		return "", errors.New("Podman returned invalid transient secret identity")
	}
	return id, nil
}

func (p Preparer) removeSecret(ctx context.Context, identity string) error {
	bin := p.PodmanBinary
	if bin == "" {
		bin = "podman"
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, bin, "secret", "rm", identity).Run(); err != nil {
		return fmt.Errorf("remove transient Podman secret %q: %w", identity, err)
	}
	return nil
}
