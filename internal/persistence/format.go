package persistence

import "strings"

type Format string

const (
	FormatJSON      Format = "json"
	FormatCompactV1 Format = "compact_v1"
)

func NormalizeFormat(raw string) Format {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(FormatCompactV1):
		return FormatCompactV1
	case string(FormatJSON):
		return FormatJSON
	default:
		return FormatCompactV1
	}
}

func FileExtension(format Format) string {
	switch NormalizeFormat(string(format)) {
	case FormatJSON:
		return ".json"
	default:
		return ".ppc"
	}
}

func PreferredReadOrder(format Format) []Format {
	switch NormalizeFormat(string(format)) {
	case FormatJSON:
		return []Format{FormatJSON, FormatCompactV1}
	default:
		return []Format{FormatCompactV1, FormatJSON}
	}
}
