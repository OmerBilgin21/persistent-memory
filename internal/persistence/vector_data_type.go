package persistence

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
)

// Sorry about the excessive comment amount here but this is
// AI assisted code and I want to keep the version that I understood
// in here so if/when I come back to this I don't have a surprised
// pikachu face

// Vector is an embedding stored in SQLite as a BLOB.
//
// Vector is a named slice type, not a struct. Go allows methods on any
// type you define yourself, which is how it gets the three methods below.
// Each one has a different job:
//
//	GormDataType  called by gorm during auto migration: which column type to create
//	Value         called by database/sql on write: Go value to bytes
//	Scan          called by database/sql on read: bytes back to a Go value
//
// Value and Scan come from Go's standard library (driver.Valuer and
// sql.Scanner), not from gorm. gorm sits on top of database/sql, which
// notices these methods and calls them. Go interfaces are satisfied just
// by having methods with the right signatures, so nothing is registered
// anywhere. It is the same pattern as json.Marshaler and json.Unmarshaler:
// Value marshals, Scan unmarshals. Only GormDataType is specific to gorm.
//
// Byte order: every float32 is written as 4 bytes, least significant byte
// first (little endian). For the 32 bit number 0x12345678 that means the
// bytes 78 56 34 12; big endian would be 12 34 56 78. Either order works,
// but writing and reading must use the same one. A mismatch gives valid
// looking floats that are completely wrong, with no error. Little endian
// matches how x86 and ARM already store numbers.
type Vector []float32

// GormDataType tells gorm's auto migration to create the column as a BLOB.
// gorm cannot guess a column type for a custom slice type on its own.
// This method does no conversion; Value and Scan do that.
func (Vector) GormDataType() string { return "blob" }

// Value converts the vector into bytes for the database (the marshal step).
//
// A driver can only accept a few types back from Value: int64, float64,
// bool, []byte, string, time.Time or nil. Returning []byte is what makes
// the driver store it as a BLOB.
//
// It uses a value receiver because it only reads the vector.
func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	buf := make([]byte, 0, len(v)*4)
	for _, x := range v {
		// Float32bits does not convert the number. It hands back the
		// float's 32 bits unchanged as a uint32, so they can be split
		// into 4 bytes.
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(x))
	}
	return buf, nil
}

// Scan converts bytes from the database back into a vector (the unmarshal
// step). A BLOB column arrives as []byte.
//
// It uses a pointer receiver because it has to overwrite the caller's
// variable.
//
// It decodes into a new slice and never keeps a reference to src, because
// the driver may reuse that buffer for the next row.
func (v *Vector) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}
	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("vector: expected []byte, got %T", src)
	}
	// Every float32 is exactly 4 bytes, so any other length means the
	// stored data is corrupted or was not written by Value.
	if len(b)%4 != 0 {
		return fmt.Errorf("vector: %d bytes is not a multiple of 4", len(b))
	}
	out := make(Vector, len(b)/4)
	for i := range out {
		// The reverse of Value: read 4 bytes in the same little endian
		// order and reinterpret the bits as a float32.
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	*v = out
	return nil
}
