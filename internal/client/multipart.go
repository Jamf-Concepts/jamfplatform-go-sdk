// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
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
//
// Unlike Do, DoMultipart does not apply the 429/Retry-After retry from execute:
// file-upload bodies arrive as io.Reader (not guaranteed rewindable), so a
// blind retry could produce a truncated second request. A 429 surfaces as an
// APIResponseError and the caller is expected to re-invoke with a fresh
// Content reader.
func (c *Transport) DoMultipart(ctx context.Context, method, path string, fields []MultipartField, expectedStatus int, result any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range fields {
		if f.Filename != "" {
			// CreateFormFile hardcodes Content-Type: application/octet-stream,
			// which Jamf's enrollment-customization image-upload endpoint
			// rejects with 400 "Bad Request". Sniff the MIME type from the
			// filename extension instead — PNG fixture → image/png — so
			// the server's content-type validation is satisfied. Fall back
			// to octet-stream if the extension is unknown.
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f.Name, f.Filename))
			ct := mime.TypeByExtension(filepath.Ext(f.Filename))
			if ct == "" {
				ct = "application/octet-stream"
			}
			// mime.TypeByExtension appends "; charset=utf-8" for text/*;
			// strip it — the server's own content-type validator may be
			// literal about values, and the boundary shouldn't carry a
			// charset parameter on binary uploads.
			if semi := strings.Index(ct, ";"); semi >= 0 {
				ct = strings.TrimSpace(ct[:semi])
			}
			h.Set("Content-Type", ct)
			part, err := w.CreatePart(h)
			if err != nil {
				return fmt.Errorf("multipart CreatePart(%q): %w", f.Name, err)
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
