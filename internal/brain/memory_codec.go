package brain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/persistence"
)

const (
	memorySchema      = "brain_memory"
	memorySchemaV1    = 1
	defaultMemoryBase = "brain-memory"
	defaultWindowBase = "conversation-window"
)

func PersistentMemoryPath(runtimeRoot string, format persistence.Format) string {
	return filepath.Join(runtimeRoot, "state", "runtime", defaultMemoryBase+persistence.FileExtension(format))
}

func DetectPersistentMemoryPath(runtimeRoot string) string {
	for _, format := range persistence.PreferredReadOrder(persistence.FormatCompactV1) {
		path := PersistentMemoryPath(runtimeRoot, format)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return PersistentMemoryPath(runtimeRoot, persistence.FormatCompactV1)
}

func encodeMemorySnapshotCompact(snapshot MemorySnapshot) ([]byte, error) {
	writer := persistence.NewWriter()
	writer.String(snapshot.Narrative)
	if err := writeStringMap(writer, snapshot.Variables); err != nil {
		return nil, err
	}
	writer.Uvarint(uint64(len(snapshot.Recent)))
	for _, entry := range snapshot.Recent {
		writer.String(entry.WorkflowID)
		writer.String(entry.Name)
		writer.String(entry.Skill)
		writer.String(entry.Status)
		writer.String(entry.Summary)
		if err := writeStringMap(writer, entry.Variables); err != nil {
			return nil, err
		}
		writer.Time(entry.RecordedAt)
	}
	writer.Time(snapshot.LastFlush)
	payload, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	return persistence.EncodeRecord(memorySchema, memorySchemaV1, payload)
}

func decodeMemorySnapshotCompact(data []byte) (MemorySnapshot, error) {
	payload, _, err := persistence.DecodeRecord(data, memorySchema)
	if err != nil {
		return MemorySnapshot{}, err
	}
	reader := persistence.NewReader(payload)
	var snapshot MemorySnapshot
	if snapshot.Narrative, err = reader.String(); err != nil {
		return MemorySnapshot{}, err
	}
	if snapshot.Variables, err = readStringMap(reader); err != nil {
		return MemorySnapshot{}, err
	}
	recentCount, err := reader.Uvarint()
	if err != nil {
		return MemorySnapshot{}, err
	}
	snapshot.Recent = make([]MemoryEntry, 0, int(recentCount))
	for i := uint64(0); i < recentCount; i++ {
		var entry MemoryEntry
		if entry.WorkflowID, err = reader.String(); err != nil {
			return MemorySnapshot{}, err
		}
		if entry.Name, err = reader.String(); err != nil {
			return MemorySnapshot{}, err
		}
		if entry.Skill, err = reader.String(); err != nil {
			return MemorySnapshot{}, err
		}
		if entry.Status, err = reader.String(); err != nil {
			return MemorySnapshot{}, err
		}
		if entry.Summary, err = reader.String(); err != nil {
			return MemorySnapshot{}, err
		}
		if entry.Variables, err = readStringMap(reader); err != nil {
			return MemorySnapshot{}, err
		}
		if entry.RecordedAt, err = reader.Time(); err != nil {
			return MemorySnapshot{}, err
		}
		snapshot.Recent = append(snapshot.Recent, entry)
	}
	if snapshot.LastFlush, err = reader.Time(); err != nil {
		return MemorySnapshot{}, err
	}
	if snapshot.Variables == nil {
		snapshot.Variables = map[string]string{}
	}
	return snapshot, nil
}

func writeStringMap(writer *persistence.Writer, values map[string]string) error {
	if len(values) == 0 {
		writer.Uvarint(0)
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	writer.Uvarint(uint64(len(keys)))
	for _, key := range keys {
		writer.String(key)
		writer.String(values[key])
	}
	return nil
}

func readStringMap(reader *persistence.Reader) (map[string]string, error) {
	count, err := reader.Uvarint()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return map[string]string{}, nil
	}
	values := make(map[string]string, int(count))
	for i := uint64(0); i < count; i++ {
		key, err := reader.String()
		if err != nil {
			return nil, err
		}
		value, err := reader.String()
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

func writeMemoryFile(path string, format persistence.Format, snapshot MemorySnapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var data []byte
	var err error
	switch persistence.NormalizeFormat(string(format)) {
	case persistence.FormatJSON:
		data, err = json.MarshalIndent(snapshot, "", "  ")
	default:
		data, err = encodeMemorySnapshotCompact(snapshot)
	}
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readMemoryFile(path string, format persistence.Format) (MemorySnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MemorySnapshot{}, err
	}
	switch persistence.NormalizeFormat(string(format)) {
	case persistence.FormatJSON:
		var snapshot MemorySnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return MemorySnapshot{}, err
		}
		if snapshot.Variables == nil {
			snapshot.Variables = map[string]string{}
		}
		return snapshot, nil
	default:
		return decodeMemorySnapshotCompact(data)
	}
}

func emptyMemorySnapshot(now time.Time) MemorySnapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return MemorySnapshot{
		Narrative: "",
		Variables: map[string]string{},
		Recent:    []MemoryEntry{},
		LastFlush: now.UTC(),
	}
}

func PrunePersistentMemory(runtimeRoot string) error {
	path := DetectPersistentMemoryPath(runtimeRoot)
	format, err := detectFormatFromPath(path)
	if err != nil {
		return err
	}
	return writeMemoryFile(path, format, emptyMemorySnapshot(time.Now().UTC()))
}

func detectFormatFromPath(path string) (persistence.Format, error) {
	switch {
	case strings.HasSuffix(path, persistence.FileExtension(persistence.FormatCompactV1)):
		return persistence.FormatCompactV1, nil
	case strings.HasSuffix(path, persistence.FileExtension(persistence.FormatJSON)):
		return persistence.FormatJSON, nil
	default:
		return "", fmt.Errorf("unknown persistence format for %s", path)
	}
}
