package region

import (
	"bytes"
	"compress/zlib"
	"io"
)

// decompressZlib はZlibデータを解凍するヘルパー関数です
func decompressZlib(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}
