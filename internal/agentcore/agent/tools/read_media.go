package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadMediaTool reads image, video, and audio files and returns them
// as base64-encoded content with appropriate MIME type information.
type ReadMediaTool struct{}

func NewReadMediaTool() *ReadMediaTool { return &ReadMediaTool{} }

type readMediaInput struct {
	Path string `json:"path"`
}

func (t *ReadMediaTool) Definition() Definition {
	return Definition{
		Name:        "ReadMedia",
		Description: "Read an image, video, or audio file. Returns base64-encoded content with MIME type. Supported formats: images (png, jpg, gif, webp, svg, bmp, tiff), video (mp4, webm, mov), audio (mp3, wav, ogg, flac).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to the media file",
				},
			},
			"required": []string{"path"},
		},
	}
}

// mediaMIMETypes maps file extensions to MIME types.
var mediaMIMETypes = map[string]string{
	// Images
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".tiff": "image/tiff",
	".tif":  "image/tiff",
	".ico":  "image/x-icon",
	// Video
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".mkv":  "video/x-matroska",
	// Audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".aac":  "audio/aac",
	".m4a":  "audio/mp4",
}

// maxMediaSize is the maximum file size for media reading (10MB).
const maxMediaSize = 10 * 1024 * 1024

func (t *ReadMediaTool) Execute(_ context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params readMediaInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Path == "" {
		return &Result{Output: "Path is required", IsError: true}, nil
	}

	path := resolvePath(params.Path, exec.WorkDir)

	// Check file info
	info, err := os.Stat(path)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	if info.IsDir() {
		return &Result{Output: fmt.Sprintf("%s is a directory, not a file", path), IsError: true}, nil
	}

	// Check file size
	if info.Size() > maxMediaSize {
		return &Result{
			Output: fmt.Sprintf("File too large: %d bytes (max %d bytes)", info.Size(), maxMediaSize),
			IsError: true,
		}, nil
	}

	// Determine MIME type
	ext := strings.ToLower(filepath.Ext(path))
	mimeType, ok := mediaMIMETypes[ext]
	if !ok {
		return &Result{
			Output: fmt.Sprintf("Unsupported media format: %s. Supported: %s", ext, supportedFormats()),
			IsError: true,
		}, nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	// Encode as base64
	encoded := base64.StdEncoding.EncodeToString(data)

	// Format output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s\n", filepath.Base(path)))
	sb.WriteString(fmt.Sprintf("MIME Type: %s\n", mimeType))
	sb.WriteString(fmt.Sprintf("Size: %d bytes\n", len(data)))
	sb.WriteString(fmt.Sprintf("Data URI: data:%s;base64,%s\n", mimeType, encoded))

	// For images, also output a markdown image reference
	if strings.HasPrefix(mimeType, "image/") && ext != ".svg" {
		sb.WriteString(fmt.Sprintf("\n![%s](%s)\n", filepath.Base(path), path))
	}

	// For SVG, include the raw text content
	if ext == ".svg" {
		sb.WriteString(fmt.Sprintf("\nSVG Content:\n%s\n", string(data)))
	}

	return &Result{Output: sb.String()}, nil
}

// supportedFormats returns a comma-separated list of supported formats.
func supportedFormats() string {
	exts := make([]string, 0, len(mediaMIMETypes))
	for ext := range mediaMIMETypes {
		exts = append(exts, ext)
	}
	return strings.Join(exts, ", ")
}
