package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// UploadImage uploads an image to GroupMe's image service and returns the URL.
// imageData should be the raw image bytes.
// Note: GroupMe's API has a quirk where image/jpeg works more reliably than image/png.
func (c *Client) UploadImage(ctx context.Context, imageData []byte, contentType string) (string, error) {
	if contentType == "" {
		// Default to image/jpeg - GroupMe API quirk: "image/png" doesn't always work
		// but you can still send .png files under "image/jpeg"
		contentType = "image/jpeg"
	}

	// Create request to GroupMe's image service
	reqURL := "https://image.groupme.com/pictures"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(imageData))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Access-Token", c.token)

	c.logger.Debug("Uploading image", "bytes", len(imageData), "type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("image upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("image upload error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Payload struct {
			URL        string `json:"url"`
			PictureURL string `json:"picture_url"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse upload response: %w", err)
	}

	imageURL := result.Payload.URL
	if imageURL == "" {
		imageURL = result.Payload.PictureURL
	}

	c.logger.Debug("Image uploaded successfully", "url", imageURL)
	return imageURL, nil
}

// UploadFile uploads a generic file to file.groupme.com.
func (c *Client) UploadFile(ctx context.Context, groupID, filename string, fileData []byte) (string, error) {
	// URL: https://file.groupme.com/v1/[GROUP_ID]/files?name=[FILE_NAME]
	url := fmt.Sprintf("https://file.groupme.com/v1/%s/files?name=%s", groupID, filename)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(fileData))
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json") // Required by API even for binary data
	req.Header.Set("X-Access-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("file upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("file upload error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		StatusURL string `json:"status_url"` // We might need to check this, but usually just need to know it started?
		// Actually, usually it returns the file info directly or we have to wait?
		// Docs say: 201 OK { "status_url": "..." }
		// Wait... the file upload is async?
		// Usually for simple files it's quick. But to get the file_id/url we might need to poll the status_url.
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	return result.StatusURL, nil
}

// UploadVideo uploads a video to video.groupme.com.
func (c *Client) UploadVideo(ctx context.Context, groupID, filename string, videoData []byte) (string, error) {
	// URL: https://video.groupme.com/transcode
	url := "https://video.groupme.com/transcode"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(videoData)); err != nil {
		return "", fmt.Errorf("failed to copy video data: %w", err)
	}
	writer.Close() // Close to write boundary

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Access-Token", c.token)
	if groupID != "" {
		req.Header.Set("X-Conversation-Id", groupID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("video upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("video upload error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		StatusURL string `json:"status_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode upload response: %w", err)
	}

	return result.StatusURL, nil
}
