package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrChunkNotGenerated は該当座標のチャンクがまだ生成されていないことを示すセンチネルエラーです。
// 「読み込みエラー」と区別するために使用します。
var ErrChunkNotGenerated = errors.New("チャンクは未生成です")

type Region struct {
	file    *os.File
	rootDir string // .mcc ファイルを参照するためにディレクトリパスを保持
}

type RegionPos struct {
	X int
	Z int
}

func OpenRegion(root string, regionX, regionZ int) (*Region, error) {
	filePath := filepath.Join(root, fmt.Sprintf("r.%d.%d.mca", regionX, regionZ))
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open file: %w", err)
	}
	return &Region{
		file:    file,
		rootDir: root,
	}, nil
}

func (r *Region) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

func (r *Region) ReadChunk(chunkX, chunkZ int) (*ChunkData, error) {
	// 1. リージョン内のローカル座標 (0 ~ 31) に変換
	localX := (chunkX%32 + 32) % 32
	localZ := (chunkZ%32 + 32) % 32

	// 2. 位置テーブルのインデックスとオフセット(バイト位置)を計算
	tableOffset := int64((localX + localZ*32) * 4)

	locationBuf := make([]byte, 4)
	if _, err := r.file.ReadAt(locationBuf, tableOffset); err != nil {
		return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
			fmt.Errorf("位置テーブルの読み込みエラー: %w", err)
	}

	// 3. オフセット（上位3バイト）とサイズ（下位1バイト）をデコード
	sectorOffset := int64(locationBuf[0])<<16 | int64(locationBuf[1])<<8 | int64(locationBuf[2])
	sectorCount := int(locationBuf[3])

	// 未生成のチャンクチェック（オフセットとサイズが共に0）
	if sectorOffset == 0 && sectorCount == 0 {
		return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusNotGenerated},
			fmt.Errorf("チャンク (%d, %d) %w", chunkX, chunkZ, ErrChunkNotGenerated)
	}
	// セクタ0-1はヘッダー/タイムスタンプ用で予約済み
	if sectorOffset < 2 {
		return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
			fmt.Errorf("不正なセクタオフセットです: %d", sectorOffset)
	}

	// 4. 実際のファイル内バイト位置へジャンプ (1セクタ = 4096バイト)
	chunkByteOffset := sectorOffset * 4096

	// 5. チャンクヘッダー（5バイト）を読み込み
	headerBuf := make([]byte, 5)
	if _, err := r.file.ReadAt(headerBuf, chunkByteOffset); err != nil {
		return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
			fmt.Errorf("チャンクヘッダーの読み込みエラー: %w", err)
	}

	dataLength := binary.BigEndian.Uint32(headerBuf[0:4])
	rawCompressionType := headerBuf[4]

	isExternal := (rawCompressionType & 0x80) != 0
	actualCompressionType := rawCompressionType & 0x7F

	var compressedBuf []byte

	if isExternal {
		// 外部の c.X.Z.mcc ファイルから圧縮データを読み出す
		var err error
		compressedBuf, err = r.readMCC(chunkX, chunkZ)
		if err != nil {
			return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
				fmt.Errorf("外部MCCファイルの読み込みエラー: %w", err)
		}
	} else {
		if dataLength <= 1 {
			return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
				fmt.Errorf("不正なデータ長です: %d", dataLength)
		}
		compressedDataSize := dataLength - 1
		compressedBuf = make([]byte, compressedDataSize)
		if _, err := r.file.ReadAt(compressedBuf, chunkByteOffset+5); err != nil {
			return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
				fmt.Errorf("圧縮データの読み込みエラー: %w", err)
		}
	}

	// 6. 解凍処理
	var uncompressedData []byte

	switch actualCompressionType {
	case 1: // GZip
		gz, err := gzip.NewReader(bytes.NewReader(compressedBuf))
		if err != nil {
			return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
				fmt.Errorf("GZip解凍エラー: %w", err)
		}
		defer gz.Close()
		uncompressedData, err = io.ReadAll(gz)
		if err != nil {
			return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
				fmt.Errorf("GZip解凍エラー: %w", err)
		}

	case 2: // Zlib (Minecraft標準)
		var err error
		uncompressedData, err = decompressZlib(compressedBuf)
		if err != nil {
			return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
				fmt.Errorf("Zlib解凍エラー: %w", err)
		}
	case 3: // 非圧縮
		uncompressedData = compressedBuf
	default:
		return &ChunkData{X: chunkX, Z: chunkZ, Status: ChunkStatusError},
			fmt.Errorf("未知の圧縮方式です: %d", actualCompressionType)
	}

	return &ChunkData{
		X:                chunkX,
		Z:                chunkZ,
		CompressionType:  actualCompressionType,
		UncompressedData: uncompressedData,
		IsExternal:       isExternal,
		Status:           ChunkStatusGenerated,
	}, nil
}

// readMCC は外部の c.X.Z.mcc ファイルを開いて圧縮データ部分のみを取得します
func (r *Region) readMCC(chunkX, chunkZ int) ([]byte, error) {
	// ファイル名形式: c.X.Z.mcc (絶対チャンク座標)
	mccPath := filepath.Join(r.rootDir, fmt.Sprintf("c.%d.%d.mcc", chunkX, chunkZ))
	mccFile, err := os.Open(mccPath)
	if err != nil {
		return nil, fmt.Errorf("MCCファイルを開けませんでした: %w", err)
	}
	defer mccFile.Close()

	// MCCファイルも先頭5バイトにヘッダー（データ長4B + 圧縮方式1B）を持ちます
	headerBuf := make([]byte, 5)
	if _, err := io.ReadFull(mccFile, headerBuf); err != nil {
		return nil, fmt.Errorf("MCCヘッダー読み込みエラー: %w", err)
	}

	dataLength := binary.BigEndian.Uint32(headerBuf[0:4])
	if dataLength <= 1 {
		return nil, fmt.Errorf("不正なMCCデータ長です: %d", dataLength)
	}

	// ヘッダー直後から残りの圧縮データ部分を一括読み込み
	compressedBuf := make([]byte, dataLength-1)
	if _, err := io.ReadFull(mccFile, compressedBuf); err != nil {
		return nil, fmt.Errorf("MCC圧縮データ読み込みエラー: %w", err)
	}

	return compressedBuf, nil
}

// Zlibデータを解凍するヘルパー関数
func decompressZlib(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}
