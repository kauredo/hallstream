//go:build !windows

package youtube

// Seal leaves b as it is: outside Windows the file's 0600 mode protects it.
func Seal(b []byte) ([]byte, error) { return b, nil }

// Unseal reverses Seal.
func Unseal(b []byte) ([]byte, error) { return b, nil }
