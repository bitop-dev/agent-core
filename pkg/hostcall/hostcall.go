//go:build wasip1

// Package hostcall provides Go bindings for agent_host WASM host functions.
//
// Import this package in WASM tool modules compiled with GOOS=wasip1 GOARCH=wasm.
// It provides HTTP access through the host runtime, gated by the sandbox's AllowedHosts.
//
// Usage:
//
//	import "github.com/bitop-dev/agent-core/pkg/hostcall"
//
//	body, err := hostcall.HTTPGet("https://api.example.com/data")
//	resp, err := hostcall.HTTPPost("https://hooks.slack.com/...", payload)
//
// The host enforces AllowedHosts — requests to non-allowed hosts return a HostError
// with Code -3.
package hostcall

import "unsafe"

// httpRequest is the raw host function import.
//
//go:wasmimport agent_host http_request
func httpRequest(
	methodPtr, methodLen uint32,
	urlPtr, urlLen uint32,
	bodyPtr, bodyLen uint32,
	respBufPtr, respBufLen uint32,
) int32

// httpRequestHeaders is the raw host function import for HTTP with custom headers.
//
//go:wasmimport agent_host http_request_headers
func httpRequestHeaders(
	methodPtr, methodLen uint32,
	urlPtr, urlLen uint32,
	headersPtr, headersLen uint32,
	bodyPtr, bodyLen uint32,
	respBufPtr, respBufLen uint32,
) int32

// responseBuf is a pre-allocated buffer for HTTP responses.
// 2MB should handle most API responses.
var responseBuf [2 * 1024 * 1024]byte

// HTTPGet makes a GET request through the host's HTTP client.
// Returns the response body or an error.
// The host enforces AllowedHosts — requests to non-allowed hosts return an error.
func HTTPGet(url string) ([]byte, error) {
	return HTTPRequest("GET", url, nil)
}

// HTTPPost makes a POST request through the host's HTTP client.
func HTTPPost(url string, body []byte) ([]byte, error) {
	return HTTPRequest("POST", url, body)
}

// HTTPRequestWithHeaders makes an HTTP request with custom headers.
// Headers is a map of key → value pairs (e.g., {"Authorization": "Bearer token"}).
func HTTPRequestWithHeaders(method, url string, headers map[string]string, body []byte) ([]byte, error) {
	methodBytes := []byte(method)
	urlBytes := []byte(url)

	// Encode headers as "Key: Value\n" pairs
	var headerBuf []byte
	for k, v := range headers {
		headerBuf = append(headerBuf, []byte(k+": "+v+"\n")...)
	}

	var hdrPtr, hdrLen uint32
	if len(headerBuf) > 0 {
		hdrPtr = uint32(uintptr(unsafe.Pointer(&headerBuf[0])))
		hdrLen = uint32(len(headerBuf))
	}

	var bodyPtr uint32
	var bodyLen uint32
	if len(body) > 0 {
		bodyPtr = uint32(uintptr(unsafe.Pointer(&body[0])))
		bodyLen = uint32(len(body))
	}

	n := httpRequestHeaders(
		uint32(uintptr(unsafe.Pointer(&methodBytes[0]))), uint32(len(methodBytes)),
		uint32(uintptr(unsafe.Pointer(&urlBytes[0]))), uint32(len(urlBytes)),
		hdrPtr, hdrLen,
		bodyPtr, bodyLen,
		uint32(uintptr(unsafe.Pointer(&responseBuf[0]))), uint32(len(responseBuf)),
	)

	if n == -1 {
		return nil, &HostError{Code: -1, Message: "host function failed"}
	}
	if n == -2 {
		return nil, &HostError{Code: -2, Message: "response exceeds buffer size"}
	}
	if n == -3 {
		return nil, &HostError{Code: -3, Message: "host not allowed by sandbox policy"}
	}
	if n < 0 {
		return nil, &HostError{Code: int(n), Message: "unknown host error"}
	}

	result := make([]byte, n)
	copy(result, responseBuf[:n])
	return result, nil
}

// HTTPRequest makes an HTTP request through the host.
// Supported methods: GET, POST, PUT, DELETE, PATCH.
func HTTPRequest(method, url string, body []byte) ([]byte, error) {
	methodBytes := []byte(method)
	urlBytes := []byte(url)

	var bodyPtr uint32
	var bodyLen uint32
	if len(body) > 0 {
		bodyPtr = uint32(uintptr(unsafe.Pointer(&body[0])))
		bodyLen = uint32(len(body))
	}

	n := httpRequest(
		uint32(uintptr(unsafe.Pointer(&methodBytes[0]))), uint32(len(methodBytes)),
		uint32(uintptr(unsafe.Pointer(&urlBytes[0]))), uint32(len(urlBytes)),
		bodyPtr, bodyLen,
		uint32(uintptr(unsafe.Pointer(&responseBuf[0]))), uint32(len(responseBuf)),
	)

	if n == -1 {
		return nil, &HostError{Code: -1, Message: "host function failed"}
	}
	if n == -2 {
		return nil, &HostError{Code: -2, Message: "response exceeds buffer size"}
	}
	if n == -3 {
		return nil, &HostError{Code: -3, Message: "host not allowed by sandbox policy"}
	}
	if n < 0 {
		return nil, &HostError{Code: int(n), Message: "unknown host error"}
	}

	// Copy the response out of the shared buffer
	result := make([]byte, n)
	copy(result, responseBuf[:n])
	return result, nil
}

// HostError is returned when a host function call fails.
type HostError struct {
	Code    int
	Message string
}

func (e *HostError) Error() string {
	return e.Message
}
