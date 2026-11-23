//go:build ignore

// mkico packs PNG files into one .ico, each image kept as PNG (Windows
// Vista and later read them at every size). Usage: go run mkico.go out.ico a.png b.png …
package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("usage: go run mkico.go out.ico a.png …")
	}
	files := os.Args[2:]
	var head, body bytes.Buffer
	le := func(w *bytes.Buffer, v any) { binary.Write(w, binary.LittleEndian, v) }
	le(&head, [3]uint16{0, 1, uint16(len(files))})
	offset := 6 + 16*len(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			log.Fatal(f, ": ", err)
		}
		if cfg.Width > 256 || cfg.Height > 256 {
			log.Fatal(f, ": larger than 256 px")
		}
		// 256 is written as 0.
		le(&head, [4]uint8{uint8(cfg.Width), uint8(cfg.Height), 0, 0})
		le(&head, [2]uint16{1, 32})
		le(&head, [2]uint32{uint32(len(b)), uint32(offset)})
		body.Write(b)
		offset += len(b)
	}
	if err := os.WriteFile(os.Args[1], append(head.Bytes(), body.Bytes()...), 0o644); err != nil {
		log.Fatal(err)
	}
}
