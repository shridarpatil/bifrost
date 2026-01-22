package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MediaStore struct {
	basePath   string
	maxSizeMB  int
	metadata   map[string]*MediaMetadata
	metadataMu sync.RWMutex
}

type MediaMetadata struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	MediaType string    `json:"media_type"`
	CreatedAt time.Time `json:"created_at"`
}

func NewMediaStore(basePath string, maxSizeMB int) (*MediaStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create media directory: %w", err)
	}

	return &MediaStore{
		basePath:  basePath,
		maxSizeMB: maxSizeMB,
		metadata:  make(map[string]*MediaMetadata),
	}, nil
}

func (ms *MediaStore) Store(data []byte, mimeType, mediaType string) (string, error) {
	maxBytes := ms.maxSizeMB * 1024 * 1024
	if len(data) > maxBytes {
		return "", fmt.Errorf("file size %d exceeds maximum %d bytes", len(data), maxBytes)
	}

	id := uuid.New().String()
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	ext := getExtension(mimeType)
	filename := fmt.Sprintf("%s%s", id, ext)
	filePath := filepath.Join(ms.basePath, filename)

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	metadata := &MediaMetadata{
		ID:        id,
		Path:      filePath,
		MimeType:  mimeType,
		Size:      int64(len(data)),
		SHA256:    hashStr,
		MediaType: mediaType,
		CreatedAt: time.Now(),
	}

	ms.metadataMu.Lock()
	ms.metadata[id] = metadata
	ms.metadataMu.Unlock()

	return id, nil
}

func (ms *MediaStore) Get(id string) (*MediaMetadata, error) {
	ms.metadataMu.RLock()
	metadata, ok := ms.metadata[id]
	ms.metadataMu.RUnlock()

	if ok {
		return metadata, nil
	}

	files, err := os.ReadDir(ms.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read media directory: %w", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), id) {
			filePath := filepath.Join(ms.basePath, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}

			mimeType := getMimeType(file.Name())
			return &MediaMetadata{
				ID:        id,
				Path:      filePath,
				MimeType:  mimeType,
				Size:      info.Size(),
				CreatedAt: info.ModTime(),
			}, nil
		}
	}

	return nil, fmt.Errorf("media not found: %s", id)
}

func (ms *MediaStore) GetData(id string) ([]byte, *MediaMetadata, error) {
	metadata, err := ms.Get(id)
	if err != nil {
		return nil, nil, err
	}

	data, err := os.ReadFile(metadata.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, metadata, nil
}

func (ms *MediaStore) StoreFromReader(reader io.Reader, mimeType, mediaType string) (string, error) {
	maxBytes := ms.maxSizeMB * 1024 * 1024
	limitedReader := io.LimitReader(reader, int64(maxBytes+1))

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read data: %w", err)
	}

	if len(data) > maxBytes {
		return "", fmt.Errorf("file size exceeds maximum %d bytes", maxBytes)
	}

	return ms.Store(data, mimeType, mediaType)
}

func (ms *MediaStore) Delete(id string) error {
	ms.metadataMu.Lock()
	metadata, ok := ms.metadata[id]
	if ok {
		delete(ms.metadata, id)
	}
	ms.metadataMu.Unlock()

	if ok && metadata.Path != "" {
		return os.Remove(metadata.Path)
	}

	files, err := os.ReadDir(ms.basePath)
	if err != nil {
		return err
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), id) {
			return os.Remove(filepath.Join(ms.basePath, file.Name()))
		}
	}

	return nil
}

func (ms *MediaStore) Cleanup(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)

	files, err := os.ReadDir(ms.basePath)
	if err != nil {
		return err
	}

	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(ms.basePath, file.Name()))
		}
	}

	ms.metadataMu.Lock()
	for id, metadata := range ms.metadata {
		if metadata.CreatedAt.Before(cutoff) {
			delete(ms.metadata, id)
		}
	}
	ms.metadataMu.Unlock()

	return nil
}

func getExtension(mimeType string) string {
	exts, err := mime.ExtensionsByType(mimeType)
	if err == nil && len(exts) > 0 {
		return exts[0]
	}

	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/3gpp":
		return ".3gp"
	case "audio/aac":
		return ".aac"
	case "audio/mp4":
		return ".m4a"
	case "audio/mpeg":
		return ".mp3"
	case "audio/amr":
		return ".amr"
	case "audio/ogg":
		return ".ogg"
	case "application/pdf":
		return ".pdf"
	case "application/vnd.ms-powerpoint":
		return ".ppt"
	case "application/msword":
		return ".doc"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	default:
		return ".bin"
	}
}

func getMimeType(filename string) string {
	ext := filepath.Ext(filename)
	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		return mimeType
	}

	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".3gp":
		return "video/3gpp"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".amr":
		return "audio/amr"
	case ".ogg":
		return "audio/ogg"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
