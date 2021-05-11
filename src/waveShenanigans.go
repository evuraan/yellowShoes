package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

func fixWav(wav string) bool {

	if getFileSize(wav) < 79 {
		return false
	}

	f, err := os.OpenFile(wav, os.O_RDWR, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Err opening %s %v\n", wav, err)
		return false
	}
	defer f.Close()
	var maxVal uint32
	maxVal = 0xffffffff

	chunkSize, ok := readU32(f, 4)
	if !ok {
		return ok
	}

	subChunk2Size, ok := readU32(f, 40)
	if !ok {
		return ok
	}

	var k, l bool
	if chunkSize != maxVal {
		k = writeU32(f, 4, maxVal)
	} else {
		k = true
	}

	if subChunk2Size != maxVal {
		l = writeU32(f, 40, maxVal)
	} else {
		l = true
	}

	return (k && l)
}

func readU32(f *os.File, seekTo int64) (uint32, bool) {
	_, err := f.Seek(seekTo, 0)
	if err != nil {
		return 0, false
	}
	var temp uint32 = 0
	err = binary.Read(f, binary.LittleEndian, &temp)
	if err != nil {
		return 0, false
	}
	return temp, true
}

func writeU32(f *os.File, seekTo int64, writeThis uint32) bool {
	_, err := f.Seek(seekTo, 0)
	if err != nil {
		return false
	}
	err = binary.Write(f, binary.LittleEndian, &writeThis)
	return err == nil
}
