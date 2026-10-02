/*
 * KustMusic - Telegram Music Bot
 *  Copyright (c) 2026 KustBots
 *  Based on TgMusicBot, Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/kustbots/kustmusic
 */

package sources

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/kustbots/kustmusic/internal/config"
	"github.com/kustbots/kustmusic/internal/utils"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	defaultFilePerm = 0644
)

var (
	errMissingKey    = errors.New("missing CDN key")
	errFileNotFound  = errors.New("file not found")
	errInvalidHexKey = errors.New("invalid hex key")
	errInvalidAESIV  = errors.New("invalid AES IV")
)

func processSpotify(track *utils.TrackInfo) (string, error) {
	downloadsDir := config.DownloadsDir
	sanitizedTrackID := filepath.Base(track.Id)

	outputFile := filepath.Join(downloadsDir, fmt.Sprintf("%s.ogg", sanitizedTrackID))
	if _, err := os.Stat(outputFile); err == nil {
		slog.Info("The file already exists", "arg1", outputFile)
		return outputFile, nil
	}

	if track.Key == "" {
		return "", errMissingKey
	}

	startTime := time.Now()
	defer func() {
		slog.Info("The process was completed in .", "duration", time.Since(startTime))
	}()

	encryptedFile := filepath.Join(downloadsDir, fmt.Sprintf("%s.encrypted", sanitizedTrackID))
	decryptedFile := filepath.Join(downloadsDir, fmt.Sprintf("%s_decrypted.ogg", sanitizedTrackID))

	defer func() {
		_ = os.Remove(encryptedFile)
		_ = os.Remove(decryptedFile)
	}()

	if err := downloadAndDecrypt(track, encryptedFile, decryptedFile); err != nil {
		slog.Info("Failed to download and decrypt the file", "error", err)
		return "", err
	}

	if err := rebuildOGG(decryptedFile); err != nil {
		slog.Info("Failed to rebuild the OGG headers", "error", err)
	}

	return fixOGG(decryptedFile, track)
}

func downloadAndDecrypt(track *utils.TrackInfo, encryptedPath, decryptedPath string) error {
	resp, err := http.Get(track.CdnURL)
	if err != nil {
		return fmt.Errorf("failed to download the file: %w", err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read the response body: %w", err)
	}

	if err = os.WriteFile(encryptedPath, data, defaultFilePerm); err != nil {
		return fmt.Errorf("failed to write the encrypted file: %w", err)
	}

	decryptedData, decryptTime, err := decryptAudioFile(encryptedPath, track.Key)
	if err != nil {
		return fmt.Errorf("failed to decrypt the audio file: %w", err)
	}

	slog.Info("Decryption was completed in .", "duration", decryptTime)
	return os.WriteFile(decryptedPath, decryptedData, defaultFilePerm)
}

// decryptAudioFile decrypts an audio file using AES-CTR encryption.
func decryptAudioFile(filePath, hexKey string) ([]byte, string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("%w: %s", errFileNotFound, filePath)
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errInvalidHexKey, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read the file: %w", err)
	}

	audioAesIv, err := hex.DecodeString("72e067fbddcbcf77ebe8bc643f630d93")
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", errInvalidAESIV, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create the AES cipher: %w", err)
	}

	startTime := time.Now()
	ctr := cipher.NewCTR(block, audioAesIv)
	decrypted := make([]byte, len(data))
	ctr.XORKeyStream(decrypted, data)

	return decrypted, fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()), nil
}

func rebuildOGG(filename string) error {
	file, err := os.OpenFile(filename, os.O_RDWR, defaultFilePerm)
	if err != nil {
		return fmt.Errorf("error opening the file: %w", err)
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	writeAt := func(offset int64, data string) error {
		_, err := file.WriteAt([]byte(data), offset)
		return err
	}

	patches := map[int64]string{
		0:  "OggS",
		6:  "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
		26: "\x01\x1E\x01vorbis",
		39: "\x02",
		40: "\x44\xAC\x00\x00",
		48: "\x00\xE2\x04\x00",
		56: "\xB8\x01",
		58: "OggS",
		62: "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
	}

	for offset, data := range patches {
		if err = writeAt(offset, data); err != nil {
			return fmt.Errorf("failed to write at offset %d: %w", offset, err)
		}
	}

	return nil
}

func fixOGG(inputFile string, track *utils.TrackInfo) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sanitizedTrackID := filepath.Base(track.Id)
	outputFile := filepath.Join(config.DownloadsDir, fmt.Sprintf("%s.ogg", sanitizedTrackID))
	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", inputFile, "-c", "copy", outputFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg failed with error: %w\nOutput: %s", err, string(output))
	}

	return outputFile, nil
}
