package youtube

import (
	"bytes"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sealed marks a file Seal wrote; a file without it is read as plain JSON.
var sealed = []byte("DPAPI1:")

// Seal encrypts b for the Windows user running the program (DPAPI), so
// another account on the PC, or a copy of the folder, can't read the login.
func Seal(b []byte) ([]byte, error) {
	out, err := dpapi(b, true)
	if err != nil {
		return nil, errors.New("não consegui proteger o login do YouTube: " + err.Error())
	}
	return append(append([]byte{}, sealed...), out...), nil
}

// Unseal reverses Seal.
func Unseal(b []byte) ([]byte, error) {
	rest, ok := bytes.CutPrefix(b, sealed)
	if !ok {
		return b, nil
	}
	out, err := dpapi(rest, false)
	if err != nil {
		return nil, errors.New("o login do YouTube foi guardado por outro utilizador do Windows; ligue outra vez: " + err.Error())
	}
	return out, nil
}

func dpapi(b []byte, protect bool) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("vazio")
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	var err error
	if protect {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size)), nil
}
