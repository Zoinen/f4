package panel

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestTransformBase64FileDataRoundTripBinary(t *testing.T) {
	original := []byte{0x00, 0x01, 0x7f, 0x80, 0xff, '\n'}
	encoded, err := transformBase64FileData(original, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := base64.StdEncoding.EncodeToString(original); string(encoded) != want {
		t.Fatalf("encoded = %q, want %q", encoded, want)
	}
	decoded, err := transformBase64FileData([]byte("\n"+string(encoded[:4])+" \t"+string(encoded[4:])+"\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, original) {
		t.Fatalf("decoded = %x, want %x", decoded, original)
	}
}

func TestTransformBase64FileDataRejectsInvalidInput(t *testing.T) {
	if _, err := transformBase64FileData([]byte("not base64!"), false); err == nil {
		t.Fatal("invalid Base64 was accepted")
	}
}
