package xmltree

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

var declarationEncoding = regexp.MustCompile(`(?i)^<\?xml\s+[^?]*\bencoding\s*=\s*["']([^"']+)["']`)

// Format describes physical encoding independently of XML's normalized values.
type Format struct {
	Encoding string
	BOM      []byte
	codec    encoding.Encoding
}

func decode(data []byte) (string, Format, error) {
	f := Format{Encoding: "UTF-8"}
	body := data
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe, 0, 0}), bytes.HasPrefix(data, []byte{0, 0, 0xfe, 0xff}):
		return "", f, fmt.Errorf("UTF-32 is not supported")
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		f.BOM, body = append([]byte(nil), data[:3]...), data[3:]
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		f.Encoding, f.codec = "UTF-16LE", unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
		f.BOM, body = append([]byte(nil), data[:2]...), data[2:]
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		f.Encoding, f.codec = "UTF-16BE", unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
		f.BOM, body = append([]byte(nil), data[:2]...), data[2:]
	case bytes.HasPrefix(data, []byte{'<', 0, '?', 0}):
		f.Encoding, f.codec = "UTF-16LE", unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case bytes.HasPrefix(data, []byte{0, '<', 0, '?'}):
		f.Encoding, f.codec = "UTF-16BE", unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	default:
		if m := declarationEncoding.FindSubmatch(data); m != nil {
			switch strings.ToLower(string(m[1])) {
			case "utf-8", "utf8":
			case "windows-1251", "cp1251":
				f.Encoding, f.codec = "windows-1251", charmap.Windows1251
			default:
				return "", f, fmt.Errorf("unsupported or inconsistent XML encoding %q", m[1])
			}
		}
	}
	decoded := body
	var err error
	if f.codec != nil {
		decoded, err = f.codec.NewDecoder().Bytes(body)
		if err != nil {
			return "", f, fmt.Errorf("decode %s: %w", f.Encoding, err)
		}
	}
	if !utf8.Valid(decoded) {
		return "", f, fmt.Errorf("invalid UTF-8 data")
	}
	if m := declarationEncoding.FindSubmatch(decoded); m != nil {
		declared := strings.ToLower(string(m[1]))
		actual := strings.ToLower(f.Encoding)
		matches := declared == actual || actual == "utf-8" && declared == "utf8" || actual == "windows-1251" && declared == "cp1251" || strings.HasPrefix(actual, "utf-16") && declared == "utf-16"
		if !matches {
			return "", f, fmt.Errorf("XML declaration %q conflicts with physical encoding %s", declared, f.Encoding)
		}
	}
	// Some decoders replace malformed sequences; reject every lossy conversion.
	encoded, err := f.Encode(string(decoded))
	if err != nil || !bytes.Equal(encoded, data) {
		return "", f, fmt.Errorf("invalid or lossy %s input", f.Encoding)
	}
	return string(decoded), f, nil
}

func (f Format) Encode(source string) ([]byte, error) {
	if !utf8.ValidString(source) {
		return nil, fmt.Errorf("invalid UTF-8 source")
	}
	data := []byte(source)
	var err error
	if f.codec != nil {
		data, err = f.codec.NewEncoder().Bytes(data)
		if err != nil {
			return nil, fmt.Errorf("result cannot be represented in %s: %w", f.Encoding, err)
		}
	}
	return append(append([]byte(nil), f.BOM...), data...), nil
}
