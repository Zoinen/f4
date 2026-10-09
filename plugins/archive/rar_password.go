package archive

import (
	"errors"
	"fmt"
	"io"

	"github.com/nwaples/rardecode/v2"
)

// rarPasswordError reclassifies the way a wrong password shows up in a RAR
// 1.5-4.x archive whose headers are encrypted (rar a -ma4 -hp...). That
// format keeps no password check value, so the first header decrypted with a
// wrong password is simply noise: rardecode reports ErrBadHeaderCRC, or
// io.ErrUnexpectedEOF when the noise claims a header longer than the file,
// and neither says "password", so the password dialog never came back
// (#1250). RAR itself cannot tell these apart either and reports "Corrupt
// file or wrong password"; RAR5 archives have a check value, and rardecode
// already returns ErrBadPassword for them.
//
// The error is only reclassified when the archive at path really has
// encrypted headers and the password fails on its very first header. An
// archive whose first header decrypts with this password and that fails
// later is damaged, and keeps its original error.
func rarPasswordError(path, password string, err error) error {
	if err == nil || password == "" {
		return err
	}
	if !errors.Is(err, rardecode.ErrBadHeaderCRC) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	if !rarFirstHeaderRejectsPassword(path, password) {
		return err
	}
	return fmt.Errorf("%w: %w", newArchivePasswordValidationError("RAR headers do not decrypt with this password"), err)
}

// rarFirstHeaderRejectsPassword reports whether the RAR archive at path has
// encrypted headers and its first one cannot be read with password.
func rarFirstHeaderRejectsPassword(path, password string) bool {
	plain, err := rardecode.OpenReader(path)
	if err == nil {
		_ = plain.Close() // The headers are readable without a password.
		return false
	}
	if !errors.Is(err, rardecode.ErrArchiveEncrypted) {
		return false
	}
	reader, err := rardecode.OpenReader(path, rardecode.Password(password))
	if err != nil {
		return rarHeaderNoise(err)
	}
	defer func() { _ = reader.Close() }()
	_, err = reader.Next()
	return rarHeaderNoise(err)
}

func rarHeaderNoise(err error) bool {
	return errors.Is(err, rardecode.ErrBadHeaderCRC) || errors.Is(err, io.ErrUnexpectedEOF)
}
