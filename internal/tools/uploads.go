package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Quazmoz/groupme-mcp/internal/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterUploadTools registers upload-related MCP tools.
func RegisterUploadTools(s *server.MCPServer, c *client.Client) {
	// Upload image tool
	uploadImageTool := mcp.NewTool("groupme_upload_image",
		mcp.WithDescription("Upload a base64-encoded image, returns a URL for use in messages."),
		mcp.WithString("image_data",
			mcp.Required(),
			mcp.Description("Base64 encoded image data"),
		),
		mcp.WithString("content_type",
			mcp.Description("MIME type of the image. Defaults to image/jpeg (required by GroupMe API even for PNG files)."),
		),
	)
	s.AddTool(uploadImageTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		imageDataBase64, ok := getArgs(request)["image_data"].(string)
		if !ok || imageDataBase64 == "" {
			return mcp.NewToolResultError("image_data is required"), nil
		}

		contentType := "image/png"
		if ct, ok := getArgs(request)["content_type"].(string); ok && ct != "" {
			contentType = ct
		}

		// Decode base64
		// Remove header if present (e.g. "data:image/png;base64,")
		if idx := strings.Index(imageDataBase64, ","); idx != -1 {
			imageDataBase64 = imageDataBase64[idx+1:]
		}

		imgBytes, err := base64.StdEncoding.DecodeString(imageDataBase64)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to decode base64 image: %v", err)), nil
		}

		url, err := c.UploadImage(ctx, imgBytes, contentType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to upload image: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Image uploaded successfully.\nURL: %s", url)), nil
	})

	// Upload File tool
	uploadFileTool := mcp.NewTool("groupme_upload_file",
		mcp.WithDescription("Upload a file (PDF, doc, TXT, ZIP, etc.) to GroupMe using either a local path or base64 payload. Use this for posting documents instead of groupme_send_message."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group where the file will be shared"),
		),
		mcp.WithString("path",
			mcp.Description("Absolute path to the local file to upload. Optional if file_data_base64 is provided."),
		),
		mcp.WithString("file_data_base64",
			mcp.Description("Base64-encoded file data. Optional alternative to path."),
		),
		mcp.WithString("filename",
			mcp.Description("Filename to use when uploading base64 data (required when using file_data_base64)."),
		),
	)
	s.AddTool(uploadFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		path := getFirstStringArg(args, "path", "file_path")
		fileDataBase64 := getFirstStringArg(args, "file_data_base64", "base64_data")

		if path == "" && fileDataBase64 == "" {
			return mcp.NewToolResultError("Either path or file_data_base64 is required"), nil
		}

		filename := getFirstStringArg(args, "filename", "name")
		var data []byte
		var err error

		if path != "" {
			data, err = os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to read file: %v. Ensure path points to a real file on the MCP server host, or provide file_data_base64 + filename.", err)), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to read file: %v", err)), nil
			}

			if filename == "" {
				filename = filepath.Base(path)
			}
		} else {
			if filename == "" {
				return mcp.NewToolResultError("filename is required when using file_data_base64"), nil
			}

			// Support data URI prefix: data:application/pdf;base64,<payload>
			if idx := strings.Index(fileDataBase64, ","); idx != -1 {
				fileDataBase64 = fileDataBase64[idx+1:]
			}

			data, err = base64.StdEncoding.DecodeString(fileDataBase64)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to decode file_data_base64: %v", err)), nil
			}
		}

		// Check size limit (50MB)
		if len(data) > 50*1024*1024 {
			return mcp.NewToolResultError("File too large. Max size is 50MB."), nil
		}

		statusURL, err := c.UploadFile(ctx, groupID, filename, data)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to upload file: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("File upload started! Status Check URL: %s\n\nNote: File uploads are asynchronous. The file should appear in the group shortly.", statusURL)), nil
	})

	// Upload Video tool
	uploadVideoTool := mcp.NewTool("groupme_upload_video",
		mcp.WithDescription("Upload a video to GroupMe. Async processing. Accepts either local path or base64 payload."),
		mcp.WithString("group_id",
			mcp.Required(),
			mcp.Description("The ID of the group (used for conversation context)"),
		),
		mcp.WithString("path",
			mcp.Description("Absolute path to the local video file. Optional if video_data_base64 is provided."),
		),
		mcp.WithString("video_data_base64",
			mcp.Description("Base64-encoded video bytes. Optional alternative to path."),
		),
		mcp.WithString("filename",
			mcp.Description("Filename to use when uploading base64 data (required with video_data_base64)."),
		),
	)
	s.AddTool(uploadVideoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c := client.Get(ctx, c)
		args := getArgs(request)
		groupID := getFirstStringArg(args, "group_id")
		if groupID == "" {
			return mcp.NewToolResultError("group_id is required"), nil
		}

		path := getFirstStringArg(args, "path", "file_path")
		videoDataBase64 := getFirstStringArg(args, "video_data_base64", "file_data_base64", "base64_data")

		if path == "" && videoDataBase64 == "" {
			return mcp.NewToolResultError("Either path or video_data_base64 is required"), nil
		}

		filename := getFirstStringArg(args, "filename", "name")
		var data []byte
		var err error

		if path != "" {
			data, err = os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to read video file: %v. Ensure path points to a real file on the MCP server host, or provide video_data_base64 + filename.", err)), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to read video file: %v", err)), nil
			}
			if filename == "" {
				filename = filepath.Base(path)
			}
		} else {
			if filename == "" {
				return mcp.NewToolResultError("filename is required when using video_data_base64"), nil
			}
			if idx := strings.Index(videoDataBase64, ","); idx != -1 {
				videoDataBase64 = videoDataBase64[idx+1:]
			}
			data, err = base64.StdEncoding.DecodeString(videoDataBase64)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to decode video_data_base64: %v", err)), nil
			}
		}

		if len(data) > 50*1024*1024 {
			return mcp.NewToolResultError("Video too large. Max size is 50MB."), nil
		}

		statusURL, err := c.UploadVideo(ctx, groupID, filename, data)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to upload video: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Video upload started! Track status here: %s", statusURL)), nil
	})
}
