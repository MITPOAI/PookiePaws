package state

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

const (
	auditChunkMagic      = "PAU1"
	auditChunkHeaderSize = 64
	activeAuditChunk     = "audit.active.ppc"
)

type auditChunkHeader struct {
	EventCount  uint64
	MinTime     int64
	MaxTime     int64
	Checksum    uint32
	PayloadSize uint64
}

func appendCompactAudit(path string, event engine.Event) error {
	normalizeEventTime(&event)
	record, err := encodeEventCompact(event)
	if err != nil {
		return err
	}

	if err := ensureAuditFile(path); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	var scratch [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(scratch[:], uint64(len(record)))
	entryBytes := append(append([]byte{}, scratch[:n]...), record...)
	_, err = file.Write(entryBytes)
	return err
}

func ensureAuditFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(auditHeaderBytes(auditChunkHeader{}))
	return err
}

func readAuditHeader(file *os.File) (auditChunkHeader, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return auditChunkHeader{}, err
	}
	raw := make([]byte, auditChunkHeaderSize)
	if _, err := io.ReadFull(file, raw); err != nil {
		return auditChunkHeader{}, err
	}
	if string(raw[:4]) != auditChunkMagic {
		return auditChunkHeader{}, errors.New("invalid audit chunk magic")
	}
	if raw[4] != 1 || raw[5] != 1 {
		return auditChunkHeader{}, fmt.Errorf("unsupported audit chunk version %d/%d", raw[4], raw[5])
	}
	return auditChunkHeader{
		EventCount:  binary.LittleEndian.Uint64(raw[8:16]),
		MinTime:     int64(binary.LittleEndian.Uint64(raw[16:24])),
		MaxTime:     int64(binary.LittleEndian.Uint64(raw[24:32])),
		Checksum:    binary.LittleEndian.Uint32(raw[32:36]),
		PayloadSize: binary.LittleEndian.Uint64(raw[36:44]),
	}, nil
}

func writeAuditHeader(file *os.File, header auditChunkHeader) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	raw := auditHeaderBytes(header)
	if _, err := file.Write(raw); err != nil {
		return err
	}
	return nil
}

func auditHeaderBytes(header auditChunkHeader) []byte {
	raw := make([]byte, auditChunkHeaderSize)
	copy(raw[:4], []byte(auditChunkMagic))
	raw[4] = 1
	raw[5] = 1
	binary.LittleEndian.PutUint64(raw[8:16], header.EventCount)
	binary.LittleEndian.PutUint64(raw[16:24], uint64(header.MinTime))
	binary.LittleEndian.PutUint64(raw[24:32], uint64(header.MaxTime))
	binary.LittleEndian.PutUint32(raw[32:36], header.Checksum)
	binary.LittleEndian.PutUint64(raw[36:44], header.PayloadSize)
	return raw
}

func readCompactAuditChunk(path string) ([]engine.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseCompactAuditChunk(data)
}

func readCompressedAuditChunk(path string) ([]engine.Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return parseCompactAuditChunk(data)
}

func parseCompactAuditChunk(data []byte) ([]engine.Event, error) {
	if len(data) < auditChunkHeaderSize {
		return nil, errors.New("audit chunk too small")
	}
	if string(data[:4]) != auditChunkMagic {
		return nil, errors.New("invalid audit chunk magic")
	}
	checksum := binary.LittleEndian.Uint32(data[32:36])
	payloadSize := binary.LittleEndian.Uint64(data[36:44])
	payload := data[auditChunkHeaderSize:]
	if payloadSize != 0 || checksum != 0 {
		if uint64(len(payload)) != payloadSize {
			return nil, fmt.Errorf("audit chunk payload size mismatch: want %d got %d", payloadSize, len(payload))
		}
		if crc32.ChecksumIEEE(payload) != checksum {
			return nil, errors.New("audit chunk checksum mismatch")
		}
	}

	reader := bytes.NewReader(payload)
	events := make([]engine.Event, 0, 16)
	for reader.Len() > 0 {
		length, err := binary.ReadUvarint(reader)
		if err != nil {
			return nil, err
		}
		if length > uint64(reader.Len()) {
			return nil, io.ErrUnexpectedEOF
		}
		record := make([]byte, int(length))
		if _, err := io.ReadFull(reader, record); err != nil {
			return nil, err
		}
		event, err := decodeEventCompact(record)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func rotateCompactAudit(path string) error {
	dir := filepath.Dir(path)
	oldest := filepath.Join(dir, fmt.Sprintf("audit.%d.ppc.gz", maxAuditRotations))
	_ = os.Remove(oldest)

	for i := maxAuditRotations - 1; i >= 1; i-- {
		from := filepath.Join(dir, fmt.Sprintf("audit.%d.ppc.gz", i))
		to := filepath.Join(dir, fmt.Sprintf("audit.%d.ppc.gz", i+1))
		_ = os.Rename(from, to)
	}

	if err := sealCompactAuditChunk(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sealed := filepath.Join(dir, "audit.1.ppc.gz")
	file, err := os.Create(sealed)
	if err != nil {
		return err
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write(data); err != nil {
		writer.Close()
		file.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Remove(path)
}

func sealCompactAuditChunk(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	header, err := readAuditHeader(file)
	if err != nil {
		return err
	}
	if header.PayloadSize != 0 || header.Checksum != 0 || header.EventCount != 0 {
		return nil
	}

	if _, err := file.Seek(auditChunkHeaderSize, io.SeekStart); err != nil {
		return err
	}
	payload, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	header.PayloadSize = uint64(len(payload))
	header.Checksum = crc32.ChecksumIEEE(payload)

	reader := bytes.NewReader(payload)
	for reader.Len() > 0 {
		length, err := binary.ReadUvarint(reader)
		if err != nil {
			return err
		}
		if length > uint64(reader.Len()) {
			return io.ErrUnexpectedEOF
		}
		record := make([]byte, int(length))
		if _, err := io.ReadFull(reader, record); err != nil {
			return err
		}
		event, err := decodeEventCompact(record)
		if err != nil {
			return err
		}
		eventUnix := event.Time.UTC().UnixNano()
		if header.EventCount == 0 || header.MinTime == 0 || eventUnix < header.MinTime {
			header.MinTime = eventUnix
		}
		if header.EventCount == 0 || eventUnix > header.MaxTime {
			header.MaxTime = eventUnix
		}
		header.EventCount++
	}

	return writeAuditHeader(file, header)
}

func ReadRecentAuditEntries(root string, limit int) ([]engine.Event, error) {
	auditsDir := filepath.Join(root, "audits")
	if limit == 0 {
		return nil, nil
	}

	var all []engine.Event
	compactPaths, err := filepath.Glob(filepath.Join(auditsDir, "audit*.ppc*"))
	if err != nil {
		return nil, err
	}
	sort.Slice(compactPaths, func(i, j int) bool {
		return auditPathRank(compactPaths[i]) < auditPathRank(compactPaths[j])
	})
	for _, path := range compactPaths {
		var events []engine.Event
		switch {
		case strings.HasSuffix(path, ".ppc.gz"):
			events, err = readCompressedAuditChunk(path)
		case strings.HasSuffix(path, ".ppc"):
			events, err = readCompactAuditChunk(path)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		all = append(all, events...)
	}

	jsonPaths, err := filepath.Glob(filepath.Join(auditsDir, "audit*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Slice(jsonPaths, func(i, j int) bool {
		return auditPathRank(jsonPaths[i]) < auditPathRank(jsonPaths[j])
	})
	for _, path := range jsonPaths {
		events, err := readJSONLAudit(path)
		if err != nil {
			return nil, err
		}
		all = append(all, events...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.Before(all[j].Time)
	})
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

func readJSONLAudit(path string) ([]engine.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	events := make([]engine.Event, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event engine.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func auditPathRank(path string) int {
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(base, "audit.active"):
		return maxAuditRotations + 1
	case base == "audit.jsonl":
		return maxAuditRotations + 2
	case strings.HasPrefix(base, "audit.1"):
		return maxAuditRotations
	case strings.HasPrefix(base, "audit.2"):
		return maxAuditRotations - 1
	case strings.HasPrefix(base, "audit.3"):
		return maxAuditRotations - 2
	default:
		return 0
	}
}
