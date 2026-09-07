package main

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

func newSHA256() hash.Hash       { return sha256.New() }
func hashHex(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }
