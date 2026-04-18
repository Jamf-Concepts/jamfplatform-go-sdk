// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

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

// MultipartField represents one part of a multipart/form-data request body.
// Exactly one of Filename (file upload) or Value (text field) must be set.
type MultipartField struct {
	Name     string    // form field name
	Filename string    // if non-empty, part is a file upload with this filename
	Content  io.Reader // file content; read to EOF
	Value    string    // text value when Filename is empty
}

// DoMultipart performs an authenticated API request with a multipart/form-data
// body. The transport builds the body (including boundary) and sets
// Content-Type appropriately. result follows the same rules as Do — either a
// JSON-unmarshal target or *[]byte for raw responses.
func (c *Transport) DoMultipart(ctx context.Context, method, path string, fields []MultipartField, expectedStatus int, result any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range fields {
		if f.Filename != "" {
			part, err := w.CreateFormFile(f.Name, f.Filename)
			if err != nil {
				return fmt.Errorf("multipart CreateFormFile(%q): %w", f.Name, err)
			}
			if f.Content != nil {
				if _, err := io.Copy(part, f.Content); err != nil {
					return fmt.Errorf("multipart copy(%q): %w", f.Name, err)
				}
			}
		} else {
			if err := w.WriteField(f.Name, f.Value); err != nil {
				return fmt.Errorf("multipart WriteField(%q): %w", f.Name, err)
			}
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("multipart close: %w", err)
	}

	fullURL := c.buildURL(path)
	if err := checkDeniedPath(method, fullURL); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	if c.logger != nil {
		c.logger.LogRequest(ctx, method, fullURL, []byte("<multipart body>"))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	c.logDeprecation(resp)

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read response body: %w", readErr)
	}

	if c.logger != nil {
		c.logger.LogResponse(ctx, resp.StatusCode, resp.Header, body)
	}

	if resp.StatusCode != expectedStatus {
		respErr := &APIResponseError{
			StatusCode: resp.StatusCode,
			Method:     resp.Request.Method,
			URL:        resp.Request.URL.String(),
			Body:       string(body),
		}
		var apiErr ApiError
		if err := json.Unmarshal(body, &apiErr); err == nil && len(apiErr.Errors) > 0 {
			respErr.StatusCode = apiErr.HTTPStatus
			respErr.TraceID = apiErr.TraceID
			respErr.Errors = apiErr.Errors
		}
		return respErr
	}

	if result != nil {
		if bp, ok := result.(*[]byte); ok {
			*bp = append((*bp)[:0], body...)
		} else if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}
