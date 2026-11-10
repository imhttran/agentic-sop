package redact

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Inventory limits bound the operator-provided sensitive-value file.
const (
	DefaultInventoryMaxValues     = 256
	DefaultInventoryMinValueBytes = 8
	DefaultInventoryMaxValueBytes = 4096
)

// InventoryLimits bounds one inventory load. A non-positive value selects the
// corresponding default.
type InventoryLimits struct {
	MaxValues     int
	MinValueBytes int
	MaxValueBytes int
}

func (l InventoryLimits) normalized() InventoryLimits {
	if l.MaxValues <= 0 {
		l.MaxValues = DefaultInventoryMaxValues
	}
	if l.MinValueBytes <= 0 {
		l.MinValueBytes = DefaultInventoryMinValueBytes
	}
	if l.MaxValueBytes <= 0 {
		l.MaxValueBytes = DefaultInventoryMaxValueBytes
	}
	return l
}

// LoadInventory reads exact sensitive values from an operator-controlled file.
// It refuses a symlink, a non-regular file, a file without 0600 permissions, an
// oversized file, a short/long value, and more than the configured number of
// values. Errors never include a value.
//
// The feature is Unix-only because POSIX 0600 permissions are part of the
// approved contract. On other platforms an enabled inventory fails closed.
func LoadInventory(path string, limits InventoryLimits) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("redact: inventory path is required")
	}
	if runtime.GOOS == "windows" {
		return nil, errors.New("redact: sensitive-value inventory requires a Unix-like filesystem with 0600 permissions")
	}
	limits = limits.normalized()
	if limits.MinValueBytes < 1 || limits.MaxValueBytes < limits.MinValueBytes {
		return nil, fmt.Errorf("redact: invalid inventory limits (min=%d max=%d)", limits.MinValueBytes, limits.MaxValueBytes)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("redact: inventory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("redact: inventory %s is a symlink; refusing", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("redact: inventory %s is not a regular file", path)
	}
	if info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("redact: inventory %s must have 0600 permissions, got %04o", path, info.Mode().Perm())
	}
	maxFileBytes := int64(limits.MaxValues) * (int64(limits.MaxValueBytes) + 2)
	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("redact: inventory %s exceeds the %d byte bound", path, maxFileBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("redact: open inventory %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64), limits.MaxValueBytes+2)
	seen := make(map[string]bool)
	values := make([]string, 0, limits.MaxValues)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSuffix(scanner.Text(), "\r")
		if raw == "" {
			continue
		}
		if len(raw) < limits.MinValueBytes {
			return nil, fmt.Errorf("redact: inventory %s line %d is shorter than %d bytes", path, line, limits.MinValueBytes)
		}
		if len(raw) > limits.MaxValueBytes {
			return nil, fmt.Errorf("redact: inventory %s line %d exceeds %d bytes", path, line, limits.MaxValueBytes)
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		values = append(values, raw)
		if len(values) > limits.MaxValues {
			return nil, fmt.Errorf("redact: inventory %s has more than %d values", path, limits.MaxValues)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("redact: read inventory %s: %w", path, err)
	}
	return values, nil
}
