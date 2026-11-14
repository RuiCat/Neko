package emote

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestReadVecDBFixture decodes a numpy-written npz fixture.
func TestReadVecDBFixture(t *testing.T) {
	database, err := readVecDB(filepath.Join("testdata", "sample.npz"))
	if err != nil {
		t.Fatalf("readVecDB: %v", err)
	}
	if database.Rows != 3 || database.Dim != 4 || database.Clusters != 2 {
		t.Fatalf("shape = %d rows / %d dim / %d clusters, want 3/4/2", database.Rows, database.Dim, database.Clusters)
	}
	want := []float32{1, 0, 0, 0, 0, 1, 0, 0, 0.9, 0.1, 0, 0}
	for i, value := range want {
		if math.Abs(float64(database.X[i]-value)) > 1e-6 {
			t.Fatalf("X[%d] = %v, want %v", i, database.X[i], value)
		}
	}
	if database.Labels[0] != 0 || database.Labels[2] != 1 {
		t.Fatalf("labels = %v, want [0 0 1]", database.Labels)
	}
}

// TestReadVecDBFloat64Narrows verifies float64 storage is narrowed to float32.
func TestReadVecDBFloat64Narrows(t *testing.T) {
	database, err := readVecDB(filepath.Join("testdata", "sample_f64.npz"))
	if err != nil {
		t.Fatalf("readVecDB: %v", err)
	}
	if database.Dim != 4 || database.Rows != 3 {
		t.Fatalf("shape = %d/%d, want 3/4", database.Rows, database.Dim)
	}
	want := []float32{1, 0, 0, 0, 0, 1, 0, 0, 0.9, 0.1, 0, 0}
	for i, value := range want {
		if math.Abs(float64(database.X[i]-value)) > 1e-6 {
			t.Fatalf("X[%d] = %v, want %v", i, database.X[i], value)
		}
	}
}

// TestVecDBRoundTrip writes and reads the database back without loss.
func TestVecDBRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), VECTOR_DB_FILE_NAME)
	x := []float32{1, 2, 3, 4}
	centers := []float32{1.5, 2.5, 3.5, 4.5}
	labels := []int64{0, 1}
	if err := writeVecDB(path, x, centers, labels, 2, 2, 2); err != nil {
		t.Fatalf("writeVecDB: %v", err)
	}
	database, err := readVecDB(path)
	if err != nil {
		t.Fatalf("readVecDB: %v", err)
	}
	if database.Rows != 2 || database.Dim != 2 || database.Clusters != 2 {
		t.Fatalf("shape = %d/%d/%d, want 2/2/2", database.Rows, database.Dim, database.Clusters)
	}
	for i := range x {
		if database.X[i] != x[i] {
			t.Fatalf("X[%d] = %v, want %v", i, database.X[i], x[i])
		}
	}
	if database.Labels[0] != 0 || database.Labels[1] != 1 {
		t.Fatalf("labels = %v, want [0 1]", database.Labels)
	}
}

// TestWriteVecDBShapeGuard rejects inconsistent arguments.
func TestWriteVecDBShapeGuard(t *testing.T) {
	err := writeVecDB(filepath.Join(t.TempDir(), VECTOR_DB_FILE_NAME), []float32{1}, []float32{1}, []int64{0}, 2, 1, 1)
	if err == nil {
		t.Fatal("want shape error")
	}
}

// TestReadVecDBCorrupt wraps decode failures in ErrVectorDBCorrupt.
func TestReadVecDBCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.npz")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := readVecDB(path); !errors.Is(err, ErrVectorDBCorrupt) {
		t.Fatalf("err = %v, want ErrVectorDBCorrupt", err)
	}
}

// TestEncodeNpyHeaderAlignment checks the 64-byte aligned header numpy expects.
func TestEncodeNpyHeaderAlignment(t *testing.T) {
	payload := encodeNpy("<f4", []int{2, 2}, make([]byte, 16))
	headerLen := int(payload[8]) | int(payload[9])<<8
	if (10+headerLen)%64 != 0 {
		t.Fatalf("header length %d breaks alignment (total %d)", headerLen, 10+headerLen)
	}
	if got := string(payload[10+headerLen-1]); got != "\n" {
		t.Fatalf("header does not end with newline: %q", got)
	}
	array, err := decodeNpy(payload)
	if err != nil {
		t.Fatalf("decodeNpy: %v", err)
	}
	if len(array.shape) != 2 || array.shape[0] != 2 || array.shape[1] != 2 {
		t.Fatalf("shape = %v, want [2 2]", array.shape)
	}
}
