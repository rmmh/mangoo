package main

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"io"
	"os"
)

const hashChunkSize = 256 * 1024

func computeMHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()

	first := make([]byte, min(int64(hashChunkSize), size))
	if _, err := io.ReadFull(f, first); err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}

	seekPos := size - hashChunkSize
	if seekPos < 0 {
		seekPos = 0
	}
	if _, err := f.Seek(seekPos, io.SeekStart); err != nil {
		return "", err
	}
	last := make([]byte, min(int64(hashChunkSize), size))
	if _, err := io.ReadFull(f, last); err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}

	var sizeBuf [8]byte
	binary.LittleEndian.PutUint64(sizeBuf[:], uint64(size))

	h := sha256.New()
	h.Write(sizeBuf[:])
	h.Write(first)
	h.Write(last)
	sum := h.Sum(nil)

	encoded := base32.StdEncoding.EncodeToString(sum)
	return encoded[:16], nil
}

