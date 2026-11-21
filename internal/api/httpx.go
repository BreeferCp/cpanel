package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

// apiError is an error with an HTTP status and a user-safe message.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

func errBadRequest(msg string) *apiError { return &apiError{400, "bad_request", msg} }
func errUnauthorized() *apiError         { return &apiError{401, "unauthorized", "authentication required"} }
func errForbidden() *apiError {
	return &apiError{403, "forbidden", "you do not have access to this resource"}
}
func errNotFound() *apiError           { return &apiError{404, "not_found", "resource not found"} }
func errConflict(msg string) *apiError { return &apiError{409, "conflict", msg} }
func errLimit(what string) *apiError {
	return &apiError{422, "limit_reached", "your plan does not allow more " + what}
}
func errTooMany() *apiError {
	return &apiError{429, "rate_limited", "too many attempts, try again later"}
}

// handler is an http handler that returns an error instead of writing it.
type handler func(w http.ResponseWriter, r *http.Request) error

// wrap converts a handler into an http.HandlerFunc with uniform error output.
// Unexpected errors are logged and returned as an opaque 500 so internals
// (paths, SQL, command output) never leak to clients.
func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		var ae *apiError
		switch {
		case errors.As(err, &ae):
			writeJSON(w, ae.Status, map[string]any{"error": map[string]string{"code": ae.Code, "message": ae.Message}})
		case errors.Is(err, store.ErrNotFound):
			writeJSON(w, 404, map[string]any{"error": map[string]string{"code": "not_found", "message": "resource not found"}})
		case errors.Is(err, system.ErrInvalid):
			writeJSON(w, 400, map[string]any{"error": map[string]string{"code": "invalid", "message": invalidMessage(err)}})
		case store.IsUnique(err):
			writeJSON(w, 409, map[string]any{"error": map[string]string{"code": "conflict", "message": "a resource with that name already exists"}})
		default:
			s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
			writeJSON(w, 500, map[string]any{"error": map[string]string{"code": "internal", "message": "internal error"}})
		}
	}
}

func invalidMessage(err error) string {
	parts := strings.Split(err.Error(), "\n")
	for _, p := range parts {
		if p != "" && p != system.ErrInvalid.Error() {
			return p
		}
	}
	return "invalid value"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// decode reads a strict JSON body (1 MiB cap, unknown fields rejected).
func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errBadRequest("request body is empty")
		}
		return errBadRequest("malformed JSON: " + err.Error())
	}
	return nil
}

// clientIP returns the peer address, honouring X-Forwarded-For only when the
// direct peer is a configured trusted proxy.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if s.isTrustedProxy(host) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if net.ParseIP(first) != nil {
				return first
			}
		}
	}
	return host
}

func (s *Server) isTrustedProxy(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, c := range s.trusted {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}
