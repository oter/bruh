package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// object is a JSON object that keeps the order of its keys, so a rewritten
// settings file differs from the old one only where bruh changed it.
type object struct {
	keys []string
	vals map[string]any
}

func newObject() *object { return &object{vals: map[string]any{}} }

func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

// child returns the object under k and creates it when k is absent or not an object.
func (o *object) child(k string) *object {
	if c, ok := o.vals[k].(*object); ok {
		return c
	}
	c := newObject()
	o.set(k, c)
	return c
}

// parseOrdered decodes JSON into *object, []any, string, json.Number, bool, and nil values.
func parseOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("extra data after the JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		o := newObject()
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			o.set(k.(string), v)
		}
		_, err := dec.Token()
		return o, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token()
		return arr, err
	}
	return tok, nil
}

// encodeOrdered writes v with two-space indentation, no HTML escaping, and a final newline.
func encodeOrdered(v any) []byte {
	var b, out bytes.Buffer
	writeValue(&b, v)
	_ = json.Indent(&out, b.Bytes(), "", "  ")
	out.WriteByte('\n')
	return out.Bytes()
}

func writeValue(b *bytes.Buffer, v any) {
	switch t := v.(type) {
	case *object:
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeValue(b, k)
			b.WriteByte(':')
			writeValue(b, t.vals[k])
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			writeValue(b, e)
		}
		b.WriteByte(']')
	default:
		enc := json.NewEncoder(b)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(t)
		b.Truncate(b.Len() - 1) // Encode adds a newline
	}
}
