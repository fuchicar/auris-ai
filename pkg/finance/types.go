package finance

import (
	"encoding/json"
	"math"
)

// Float is a float64 that marshals NaN and ±Inf as JSON null and parses JSON
// null back to math.NaN. Use it only on fields that can legitimately be
// undefined (e.g. the warm-up positions of an indicator series). Using it on
// ordinary numeric fields would silently coerce bad data to NaN.
type Float float64

// MarshalJSON encodes the value as a JSON number when finite, or null when
// NaN/±Inf. This keeps indicator output JSON-clean even though the warm-up
// positions of a moving average are mathematically undefined.
func (f Float) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

// UnmarshalJSON parses a JSON number into Float, or maps JSON null to NaN.
// Any other JSON type (string, bool, object) is rejected so callers fail
// loudly on schema violations rather than getting NaN by surprise.
func (f *Float) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = Float(math.NaN())
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = Float(v)
	return nil
}

// FloatSlice marshals/unmarshals a []float64 where individual elements may be
// NaN/±Inf. JSON null elements round-trip to math.NaN.
type FloatSlice []float64

// MarshalJSON emits a JSON array, mapping each non-finite element to null.
func (s FloatSlice) MarshalJSON() ([]byte, error) {
	out := make([]byte, 0, len(s)*8)
	out = append(out, '[')
	for i, v := range s {
		if i > 0 {
			out = append(out, ',')
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out = append(out, "null"...)
		} else {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		}
	}
	out = append(out, ']')
	return out, nil
}

// UnmarshalJSON accepts a JSON array of numbers/nulls. Nulls become math.NaN.
func (s *FloatSlice) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make([]float64, len(raw))
	for i, r := range raw {
		if string(r) == "null" {
			out[i] = math.NaN()
			continue
		}
		var v float64
		if err := json.Unmarshal(r, &v); err != nil {
			return err
		}
		out[i] = v
	}
	*s = out
	return nil
}
