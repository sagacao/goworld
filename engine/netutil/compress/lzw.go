package compress

import (
	"bytes"

	"compress/lzw"

	"github.com/sagacao/goworld/engine/gwioutil"
)

func NewLzwCompressor() Compressor {
	fc := lzwCompressor{}
	return fc
}

type lzwCompressor struct {
}

func (fc lzwCompressor) Compress(b []byte, c []byte) ([]byte, error) {
	wb := bytes.NewBuffer(c)
	lzwWriter := lzw.NewWriter(wb, lzw.LSB, 8)
	n, err := lzwWriter.Write(b)
	if err != nil {
		return nil, err
	}
	if n != len(b) {
		return nil, errNotFullyCompressed
	}
	lzwWriter.Close()
	return wb.Bytes(), nil
}

func (fc lzwCompressor) Decompress(c []byte, b []byte) error {
	lzwReader := lzw.NewReader(bytes.NewReader(c), lzw.LSB, 8)
	return gwioutil.ReadAll(lzwReader, b)
}
