// Package reviewreceipt records one explicit, local acceptance of the exact
// focused findings produced from an immutable Git index snapshot.
package reviewreceipt

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
	"sort"
	"strings"

	"github.com/Cyberlane/mori/internal/buildinfo"
)

const (
	// SchemaVersion is the current local review-receipt contract.
	SchemaVersion = 1
	// MaxDocumentBytes bounds a receipt before JSON decoding.
	MaxDocumentBytes int64 = 1024 * 1024
	decision               = "accept-focused-structural-matches"
)

// Receipt binds an explicit decision to every input that can change the
// focused finding set. It intentionally stores no timestamps or source text.
type Receipt struct {
	SchemaVersion        int      `json:"schema_version"`
	Decision             string   `json:"decision"`
	MoriVersion          string   `json:"mori_version"`
	NormalizationVersion int      `json:"normalization_version"`
	HeadCommit           string   `json:"head_commit"`
	IndexDigest          string   `json:"index_digest"`
	ScanProfileDigest    string   `json:"scan_profile_digest"`
	FocusedMatchIDs      []string `json:"focused_match_ids"`
}

// Evidence is the exact active state a receipt must match.
type Evidence struct {
	Tool                 buildinfo.Info
	NormalizationVersion int
	HeadCommit           string
	IndexDigest          string
	ScanProfileDigest    string
	FocusedMatchIDs      []string
}

// New constructs one canonical receipt and refuses an empty acceptance.
func New(evidence Evidence) (Receipt, error) {
	ids, err := normalizedIDs(evidence.FocusedMatchIDs)
	if err != nil {
		return Receipt{}, err
	}
	if len(ids) == 0 {
		return Receipt{}, errors.New("no focused structural matches require acknowledgment")
	}
	if err := validateEvidence(evidence); err != nil {
		return Receipt{}, err
	}
	return Receipt{
		SchemaVersion:        SchemaVersion,
		Decision:             decision,
		MoriVersion:          evidence.Tool.Version,
		NormalizationVersion: evidence.NormalizationVersion,
		HeadCommit:           evidence.HeadCommit,
		IndexDigest:          evidence.IndexDigest,
		ScanProfileDigest:    evidence.ScanProfileDigest,
		FocusedMatchIDs:      ids,
	}, nil
}

// Load reads and validates one bounded local receipt.
func Load(path string) (Receipt, error) {
	file, err := os.Open(path)
	if err != nil {
		return Receipt{}, fmt.Errorf("read review receipt: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxDocumentBytes+1))
	if err != nil {
		return Receipt{}, fmt.Errorf("read review receipt: %w", err)
	}
	if int64(len(content)) > MaxDocumentBytes {
		return Receipt{}, fmt.Errorf("read review receipt: document exceeds %d bytes", MaxDocumentBytes)
	}
	var receipt Receipt
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, fmt.Errorf("decode review receipt: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Receipt{}, errors.New("decode review receipt: trailing JSON content")
	}
	if receipt.SchemaVersion != SchemaVersion {
		return Receipt{}, fmt.Errorf("review receipt schema version %d is unsupported; expected %d", receipt.SchemaVersion, SchemaVersion)
	}
	if receipt.Decision != decision {
		return Receipt{}, errors.New("review receipt has an unsupported decision")
	}
	ids, err := normalizedIDs(receipt.FocusedMatchIDs)
	if err != nil {
		return Receipt{}, err
	}
	if len(ids) == 0 || !equalStrings(ids, receipt.FocusedMatchIDs) {
		return Receipt{}, errors.New("review receipt focused match IDs must be non-empty, unique, and sorted")
	}
	return receipt, nil
}

// Validate requires exact equality with the current immutable scan evidence.
func Validate(receipt Receipt, evidence Evidence) error {
	if err := validateEvidence(evidence); err != nil {
		return err
	}
	ids, err := normalizedIDs(evidence.FocusedMatchIDs)
	if err != nil {
		return err
	}
	checks := []struct {
		name   string
		stored string
		active string
	}{
		{"Mori version", receipt.MoriVersion, evidence.Tool.Version},
		{"HEAD commit", receipt.HeadCommit, evidence.HeadCommit},
		{"Git index digest", receipt.IndexDigest, evidence.IndexDigest},
		{"scan profile digest", receipt.ScanProfileDigest, evidence.ScanProfileDigest},
	}
	for _, check := range checks {
		if check.stored != check.active {
			return fmt.Errorf("review receipt %s is stale", check.name)
		}
	}
	if receipt.NormalizationVersion != evidence.NormalizationVersion {
		return errors.New("review receipt normalization version is stale")
	}
	if !equalStrings(receipt.FocusedMatchIDs, ids) {
		return errors.New("review receipt focused findings are stale")
	}
	return nil
}

// Write stores a canonical receipt atomically with owner-only permissions.
func Write(path string, receipt Receipt) error {
	content, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode review receipt: %w", err)
	}
	content = append(content, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create review receipt directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".mori-review-*")
	if err != nil {
		return fmt.Errorf("create review receipt: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure review receipt: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write review receipt: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync review receipt: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close review receipt: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace review receipt: %w", err)
	}
	return nil
}

// Digest returns a source-free identity for one canonical receipt.
func Digest(receipt Receipt) string {
	content, err := json.Marshal(receipt)
	if err != nil {
		panic(fmt.Sprintf("encode review receipt: %v", err))
	}
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func validateEvidence(evidence Evidence) error {
	if evidence.Tool.Version == "" || evidence.NormalizationVersion < 1 ||
		(!validHex(evidence.HeadCommit, 40) && !validHex(evidence.HeadCommit, 64)) ||
		!validHex(evidence.IndexDigest, 64) || !validHex(evidence.ScanProfileDigest, 64) {
		return errors.New("review receipt evidence is incomplete")
	}
	return nil
}

func normalizedIDs(values []string) ([]string, error) {
	result := append([]string{}, values...)
	for _, value := range result {
		left, right, ok := strings.Cut(value, ":")
		if !ok || !validHex(left, 16) || !validHex(right, 16) {
			return nil, errors.New("review receipt contains an invalid focused match ID")
		}
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, errors.New("review receipt contains a duplicate focused match ID")
		}
	}
	return result, nil
}

func validHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
