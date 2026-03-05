package jsonutil

import (
	"encoding/json"
	"io"
)

// Marshal serializes v to JSON bytes.
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// MustMarshal serializes v to JSON bytes, panics on error.
func MustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic("jsonutil: marshal: " + err.Error())
	}
	return data
}

// Unmarshal deserializes JSON data into dst.
func Unmarshal(data []byte, dst any) error {
	return json.Unmarshal(data, dst)
}

// MustUnmarshal deserializes JSON data into dst, panics on error.
func MustUnmarshal(data []byte, dst any) {
	if err := json.Unmarshal(data, dst); err != nil {
		panic("jsonutil: unmarshal: " + err.Error())
	}
}

// ToString serializes v to a JSON string.
func ToString(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MustToString serializes v to a JSON string, panics on error.
func MustToString(v any) string {
	return string(MustMarshal(v))
}

// FromString deserializes a JSON string into dst.
func FromString(s string, dst any) error {
	return json.Unmarshal([]byte(s), dst)
}

// Pretty serializes v to indented JSON bytes.
func Pretty(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// PrettyString serializes v to an indented JSON string.
func PrettyString(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsValid reports whether data is valid JSON.
func IsValid(data []byte) bool {
	return json.Valid(data)
}

// IsValidString reports whether s is valid JSON.
func IsValidString(s string) bool {
	return json.Valid([]byte(s))
}

// Decode reads JSON from r and decodes into dst.
func Decode(r io.Reader, dst any) error {
	return json.NewDecoder(r).Decode(dst)
}

// Encode writes v as JSON to w.
func Encode(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// ToMap converts a struct or any value to map[string]any via JSON round-trip.
func ToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// FromMap converts a map to a struct via JSON round-trip.
func FromMap(m map[string]any, dst any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
