package httpclient

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

// UploadFile posts one local file as a multipart request and returns the
// response for the caller to decode and close.
//
// The body is streamed through a pipe rather than buffered: an artifact is
// whatever size the deployment allows, and a client that had to hold one in
// memory to send it would make the limit twice as expensive as it looks.
func UploadFile(ctx context.Context, client *http.Client, url, token, fieldName, path, filename string) (*http.Response, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		defer func() { _ = file.Close() }()
		part, err := writer.CreateFormFile(fieldName, filename)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		// Closing the writer emits the trailing boundary; closing the pipe then
		// ends the request body. Both errors reach the reader, so a partial
		// upload fails the request rather than arriving truncated.
		_ = pw.CloseWithError(writer.Close())
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}
