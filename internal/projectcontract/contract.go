// Package projectcontract defines the small, tracked contract document that
// binds a project to the Mori artifacts it has elected to manage.
package projectcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileName is deliberately distinct from .mori.json: the latter is project
// policy, while this file records Mori-managed compatibility evidence.
const FileName = ".mori-project.json"

const SchemaVersion = 1

// Contract is intentionally data-only so it can be consumed by other tools.
type Contract struct {
	SchemaVersion        int      `json:"schema_version"`
	MoriVersion          string   `json:"mori_version"`
	EmbeddedSkill        Artifact `json:"embedded_skill"`
	HookContract         Artifact `json:"hook_contract"`
	ConfigSchemaVersion  int      `json:"config_schema_version"`
	ReportSchemaVersion  int      `json:"report_schema_version"`
	ReviewReceiptSchema  int      `json:"review_receipt_schema_version"`
	BaselineSchema       int      `json:"baseline_schema_version"`
	NormalizationVersion int      `json:"normalization_version"`
}

type Artifact struct {
	Revision string `json:"revision"`
	Digest   string `json:"digest"`
}

func (c Contract) Valid() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("project contract schema version %d is unsupported; expected %d", c.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(c.MoriVersion) != c.MoriVersion || c.MoriVersion == "" ||
		strings.TrimSpace(c.EmbeddedSkill.Revision) != c.EmbeddedSkill.Revision || c.EmbeddedSkill.Revision == "" ||
		!validDigest(c.EmbeddedSkill.Digest) {
		return errors.New("project contract is missing required managed artifact evidence")
	}
	if strings.TrimSpace(c.HookContract.Revision) != c.HookContract.Revision || c.HookContract.Revision == "" ||
		!validDigest(c.HookContract.Digest) {
		return errors.New("project contract is missing required hook contract evidence")
	}
	if c.ConfigSchemaVersion < 1 || c.ReportSchemaVersion < 1 || c.ReviewReceiptSchema < 1 ||
		c.BaselineSchema < 1 || c.NormalizationVersion < 1 {
		return errors.New("project contract contains an invalid schema or normalization version")
	}
	return nil
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

// Digest returns a stable identity for one contract. It is useful for
// preconditions and does not include filesystem metadata.
func Digest(c Contract) string {
	content, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func Load(path string) (Contract, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Contract{}, false, nil
	}
	if err != nil {
		return Contract{}, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Contract{}, false, errors.New("project contract must be a regular file, not a symlink")
	}
	if info.Size() > 64*1024 {
		return Contract{}, false, errors.New("project contract exceeds 65536 bytes")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, false, err
	}
	var contract Contract
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, false, fmt.Errorf("decode project contract: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return Contract{}, false, fmt.Errorf("decode project contract: %w", err)
	}
	if err := contract.Valid(); err != nil {
		return Contract{}, false, err
	}
	return contract, true, nil
}

// Marshal produces stable human-readable JSON. Struct field order is part of
// the contract so repeated dry-runs are byte-for-byte deterministic.
func Marshal(c Contract) ([]byte, error) {
	if err := c.Valid(); err != nil {
		return nil, err
	}
	content, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

// WriteAtomic creates or replaces a managed contract. Existing symlinks are
// rejected and replacement verifies the inode observed before the write.
func WriteAtomic(path string, content []byte, replace bool) (string, error) {
	if !bytes.HasSuffix(content, []byte{'\n'}) {
		return "", errors.New("project contract content must end with a newline")
	}
	directory := filepath.Dir(path)
	var original os.FileInfo
	if replace {
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New("project contract must be a regular file, not a symlink")
		}
		original = info
	}
	backup := ""
	if replace {
		old, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		f, err := os.CreateTemp(directory, ".mori-project.backup-*")
		if err != nil {
			return "", err
		}
		backup = f.Name()
		if _, err := f.Write(old); err != nil {
			_ = f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
	}
	tmp, err := os.CreateTemp(directory, ".mori-project.tmp-*")
	if err != nil {
		return backup, err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return backup, err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return backup, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return backup, err
	}
	if err := tmp.Close(); err != nil {
		return backup, err
	}
	if replace {
		before, err := os.Lstat(path)
		if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || !os.SameFile(original, before) {
			return backup, errors.New("project contract changed identity before replacement")
		}
	} else if _, err := os.Lstat(path); err == nil {
		return backup, errors.New("project contract appeared before creation")
	} else if !errors.Is(err, os.ErrNotExist) {
		return backup, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return backup, err
	}
	committed = true
	return backup, nil
}
