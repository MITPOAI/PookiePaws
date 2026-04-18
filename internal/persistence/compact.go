package persistence

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

const (
	compactRecordMagic   = "PPCR"
	compactRecordVersion = 1
)

type Writer struct {
	buf bytes.Buffer
	err error
}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) Uvarint(value uint64) {
	if w.err != nil {
		return
	}
	var scratch [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(scratch[:], value)
	_, w.err = w.buf.Write(scratch[:n])
}

func (w *Writer) Uint32(value uint32) {
	if w.err != nil {
		return
	}
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], value)
	_, w.err = w.buf.Write(scratch[:])
}

func (w *Writer) Bool(value bool) {
	if value {
		w.Uvarint(1)
		return
	}
	w.Uvarint(0)
}

func (w *Writer) String(value string) {
	w.Bytes([]byte(value))
}

func (w *Writer) Bytes(value []byte) {
	if w.err != nil {
		return
	}
	w.Uvarint(uint64(len(value)))
	if w.err != nil || len(value) == 0 {
		return
	}
	_, w.err = w.buf.Write(value)
}

func (w *Writer) Time(value time.Time) {
	if value.IsZero() {
		w.Bool(false)
		return
	}
	w.Bool(true)
	w.Uvarint(uint64(value.UTC().UnixNano()))
}

func (w *Writer) OptionalTime(value *time.Time) {
	if value == nil || value.IsZero() {
		w.Bool(false)
		return
	}
	w.Bool(true)
	w.Uvarint(uint64(value.UTC().UnixNano()))
}

func (w *Writer) Finish() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}

type Reader struct {
	r *bytes.Reader
}

func NewReader(payload []byte) *Reader {
	return &Reader{r: bytes.NewReader(payload)}
}

func (r *Reader) Uvarint() (uint64, error) {
	value, err := binary.ReadUvarint(r.r)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (r *Reader) Uint32() (uint32, error) {
	var scratch [4]byte
	if _, err := io.ReadFull(r.r, scratch[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(scratch[:]), nil
}

func (r *Reader) Bool() (bool, error) {
	value, err := r.Uvarint()
	if err != nil {
		return false, err
	}
	return value == 1, nil
}

func (r *Reader) String() (string, error) {
	value, err := r.Bytes()
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (r *Reader) Bytes() ([]byte, error) {
	length, err := r.Uvarint()
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, nil
	}
	if length > uint64(r.r.Len()) {
		return nil, io.ErrUnexpectedEOF
	}
	value := make([]byte, int(length))
	if _, err := io.ReadFull(r.r, value); err != nil {
		return nil, err
	}
	return value, nil
}

func (r *Reader) Time() (time.Time, error) {
	present, err := r.Bool()
	if err != nil {
		return time.Time{}, err
	}
	if !present {
		return time.Time{}, nil
	}
	value, err := r.Uvarint()
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, int64(value)).UTC(), nil
}

func (r *Reader) OptionalTime() (*time.Time, error) {
	present, err := r.Bool()
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	value, err := r.Uvarint()
	if err != nil {
		return nil, err
	}
	parsed := time.Unix(0, int64(value)).UTC()
	return &parsed, nil
}

func (r *Reader) Remaining() int {
	return r.r.Len()
}

func EncodeRecord(schema string, schemaVersion uint64, payload []byte) ([]byte, error) {
	writer := NewWriter()
	writer.String(compactRecordMagic)
	writer.Uvarint(compactRecordVersion)
	writer.String(schema)
	writer.Uvarint(schemaVersion)
	writer.Uvarint(uint64(len(payload)))
	writer.Uint32(crc32.ChecksumIEEE(payload))
	writer.Bytes(payload)
	return writer.Finish()
}

func DecodeRecord(data []byte, schema string) ([]byte, uint64, error) {
	reader := NewReader(data)

	magic, err := reader.String()
	if err != nil {
		return nil, 0, err
	}
	if magic != compactRecordMagic {
		return nil, 0, errors.New("invalid compact record magic")
	}

	version, err := reader.Uvarint()
	if err != nil {
		return nil, 0, err
	}
	if version != compactRecordVersion {
		return nil, 0, fmt.Errorf("unsupported compact record version %d", version)
	}

	recordSchema, err := reader.String()
	if err != nil {
		return nil, 0, err
	}
	if schema != "" && recordSchema != schema {
		return nil, 0, fmt.Errorf("unexpected compact schema %q", recordSchema)
	}

	schemaVersion, err := reader.Uvarint()
	if err != nil {
		return nil, 0, err
	}
	length, err := reader.Uvarint()
	if err != nil {
		return nil, 0, err
	}
	checksum, err := reader.Uint32()
	if err != nil {
		return nil, 0, err
	}
	payload, err := reader.Bytes()
	if err != nil {
		return nil, 0, err
	}
	if uint64(len(payload)) != length {
		return nil, 0, fmt.Errorf("payload length mismatch: want %d got %d", length, len(payload))
	}
	if crc32.ChecksumIEEE(payload) != checksum {
		return nil, 0, errors.New("compact record checksum mismatch")
	}
	if reader.Remaining() != 0 {
		return nil, 0, errors.New("unexpected trailing bytes in compact record")
	}
	return payload, schemaVersion, nil
}
