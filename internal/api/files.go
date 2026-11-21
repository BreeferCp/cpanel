package api

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/rbac"
)

const maxUpload = 2 << 30 // 2 GiB per request

func (s *Server) fileRoutes(mux *http.ServeMux) {
	const b = "/api/v1/accounts/{aid}/files"
	s.route(mux, "GET "+b, rbac.FilesRead, s.fmList)
	s.route(mux, "GET "+b+"/content", rbac.FilesRead, s.fmRead)
	s.route(mux, "GET "+b+"/download", rbac.FilesRead, s.fmDownload)
	s.route(mux, "PUT "+b+"/content", rbac.FilesWrite, s.fmWrite)
	s.route(mux, "POST "+b+"/upload", rbac.FilesWrite, s.fmUpload)
	s.route(mux, "POST "+b+"/mkdir", rbac.FilesWrite, s.fmMkdir)
	s.route(mux, "POST "+b+"/rename", rbac.FilesWrite, s.fmRename)
	s.route(mux, "POST "+b+"/copy", rbac.FilesWrite, s.fmCopy)
	s.route(mux, "POST "+b+"/chmod", rbac.FilesWrite, s.fmChmod)
	s.route(mux, "POST "+b+"/compress", rbac.FilesWrite, s.fmCompress)
	s.route(mux, "POST "+b+"/extract", rbac.FilesWrite, s.fmExtract)
	s.route(mux, "POST "+b+"/delete", rbac.FilesWrite, s.fmDelete)
}

// openJail resolves the account and opens its home as a jail owned by the
// account's uid, so everything the panel creates stays manageable by them.
func (s *Server) openJail(r *http.Request) (*files.FS, *acct, error) {
	a, err := s.loadAccount(r)
	if err != nil {
		return nil, nil, err
	}
	if a.Status != "active" {
		return nil, nil, errConflict("account is suspended")
	}
	f, err := files.Open(a.HomeDir, a.SysUID, a.SysUID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, errNotFound()
		}
		return nil, nil, err
	}
	return f, a, nil
}

// fsErr maps filesystem errors to API errors without leaking host paths.
func fsErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		return errNotFound()
	case errors.Is(err, fs.ErrExist):
		return errConflict("the destination already exists")
	case errors.Is(err, fs.ErrPermission):
		return errForbidden()
	case errors.Is(err, files.ErrTooLarge):
		return &apiError{413, "too_large", "file or archive is too large"}
	case errors.Is(err, files.ErrInvalidPath):
		return errBadRequest("invalid path")
	}
	// os.Root reports escapes as "path escapes from parent".
	if strings.Contains(err.Error(), "escapes from parent") {
		return errBadRequest("path leaves your home directory")
	}
	return err
}

func (s *Server) fmList(w http.ResponseWriter, r *http.Request) error {
	f, _, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	list, err := f.List(r.URL.Query().Get("path"))
	if err != nil {
		return fsErr(err)
	}
	writeJSON(w, 200, list)
	return nil
}

func (s *Server) fmRead(w http.ResponseWriter, r *http.Request) error {
	f, _, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := f.ReadText(r.URL.Query().Get("path"))
	if err != nil {
		return fsErr(err)
	}
	if !isText(b) {
		return &apiError{415, "binary", "this file is not a text file"}
	}
	writeJSON(w, 200, map[string]string{"content": string(b)})
	return nil
}

func isText(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	for _, c := range b[:n] {
		if c == 0 {
			return false
		}
	}
	return true
}

// fmDownload streams a file as an attachment. Content is never rendered inline
// from the panel origin, which prevents stored-XSS via uploaded HTML/SVG.
func (s *Server) fmDownload(w http.ResponseWriter, r *http.Request) error {
	f, _, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	fl, info, err := f.OpenRead(r.URL.Query().Get("path"))
	if err != nil {
		return fsErr(err)
	}
	defer fl.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(info.Name())}))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = io.Copy(w, fl)
	return nil
}

func (s *Server) fmWrite(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	f, a, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.WriteFile(req.Path, []byte(req.Content)); err != nil {
		return fsErr(err)
	}
	s.audit(r, "file.write", a.SysUser+":"+req.Path, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) fmUpload(w http.ResponseWriter, r *http.Request) error {
	f, a, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		return errBadRequest("expected a multipart upload")
	}
	dir := r.URL.Query().Get("path")
	var saved []string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errBadRequest("upload interrupted or too large")
		}
		name := path.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
		if part.FileName() == "" || name == "." || name == ".." || name == "/" {
			continue
		}
		dest := path.Join("/", dir, name)
		out, rel, err := f.Create(dest)
		if err != nil {
			return fsErr(err)
		}
		_, cerr := io.Copy(out, part)
		if e := out.Close(); cerr == nil {
			cerr = e
		}
		if cerr != nil {
			_ = f.Remove(dest)
			return errBadRequest("upload failed: " + cerr.Error())
		}
		if err := f.Finish(rel); err != nil {
			return err
		}
		saved = append(saved, name)
	}
	s.audit(r, "file.upload", a.SysUser+":"+dir, true, strings.Join(saved, ","))
	writeJSON(w, 201, map[string]any{"saved": saved})
	return nil
}

type pathReq struct {
	Path string `json:"path"`
}

func (s *Server) fmMkdir(w http.ResponseWriter, r *http.Request) error {
	var req pathReq
	if err := decode(r, &req); err != nil {
		return err
	}
	f, _, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Mkdir(req.Path); err != nil {
		return fsErr(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type moveReq struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (s *Server) fmRename(w http.ResponseWriter, r *http.Request) error {
	var req moveReq
	if err := decode(r, &req); err != nil {
		return err
	}
	f, a, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Rename(req.From, req.To); err != nil {
		return fsErr(err)
	}
	s.audit(r, "file.rename", a.SysUser+":"+req.From, true, req.To)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) fmCopy(w http.ResponseWriter, r *http.Request) error {
	var req moveReq
	if err := decode(r, &req); err != nil {
		return err
	}
	f, _, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Copy(req.From, req.To); err != nil {
		return fsErr(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) fmChmod(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	m, err := strconv.ParseUint(req.Mode, 8, 32)
	if err != nil || len(req.Mode) < 3 || len(req.Mode) > 4 || m > 0o777 {
		return errBadRequest("mode must be an octal value such as 0644")
	}
	f, a, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(req.Path, os.FileMode(m)); err != nil {
		return fsErr(err)
	}
	s.audit(r, "file.chmod", a.SysUser+":"+req.Path, true, req.Mode)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) fmCompress(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if len(req.Paths) == 0 || len(req.Paths) > 1000 {
		return errBadRequest("select between 1 and 1000 items")
	}
	f, _, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Compress(req.Paths, req.Dest); err != nil {
		return fsErr(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) fmExtract(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Path string `json:"path"`
		Dest string `json:"dest"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	f, a, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Extract(req.Path, req.Dest); err != nil {
		return fsErr(err)
	}
	s.audit(r, "file.extract", a.SysUser+":"+req.Path, true, req.Dest)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) fmDelete(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if len(req.Paths) == 0 || len(req.Paths) > 1000 {
		return errBadRequest("select between 1 and 1000 items")
	}
	f, a, err := s.openJail(r)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, p := range req.Paths {
		if err := f.Remove(p); err != nil {
			return fsErr(fmt.Errorf("%s: %w", p, err))
		}
	}
	s.audit(r, "file.delete", a.SysUser, true, strings.Join(req.Paths, ","))
	w.WriteHeader(http.StatusNoContent)
	return nil
}
