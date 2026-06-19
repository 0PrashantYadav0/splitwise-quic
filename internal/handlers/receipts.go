package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxReceiptBytes = 6 << 20 // 6 MiB

// allowedReceiptExt maps accepted MIME types to file extensions.
var allowedReceiptExt = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// parseAnyForm parses either a multipart (file upload) or urlencoded form.
func parseAnyForm(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		return r.ParseMultipartForm(maxReceiptBytes)
	}
	return r.ParseForm()
}

// saveReceipt persists an uploaded "receipt" image (if present) to the upload
// directory, named by the expense id. Returns the stored filename ("" if none).
//
// On HTTP/3 the multipart body streams in over a dedicated QUIC stream, so
// large receipts don't block other multiplexed requests on the connection.
func (h *Handlers) saveReceipt(r *http.Request, expenseID string) (string, error) {
	if r.MultipartForm == nil {
		return "", nil
	}
	file, hdr, err := r.FormFile("receipt")
	if err != nil {
		return "", nil // no file supplied
	}
	defer file.Close()
	if hdr.Size == 0 {
		return "", nil
	}
	if hdr.Size > maxReceiptBytes {
		return "", fmt.Errorf("receipt too large")
	}

	// Sniff the content type from the first 512 bytes.
	head := make([]byte, 512)
	n, _ := file.Read(head)
	ct := http.DetectContentType(head[:n])
	ext, ok := allowedReceiptExt[ct]
	if !ok {
		return "", fmt.Errorf("unsupported receipt type %q", ct)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	if err := os.MkdirAll(h.uploadDir, 0o755); err != nil {
		return "", err
	}
	fname := expenseID + ext
	dst, err := os.Create(filepath.Join(h.uploadDir, fname))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, io.LimitReader(file, maxReceiptBytes)); err != nil {
		return "", err
	}
	return fname, nil
}

// serveReceipt streams a stored receipt image to authorized group members.
func (h *Handlers) serveReceipt(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	e, err := h.store.ExpenseByID(g.ID, r.PathValue("eid"))
	if err != nil || e.ReceiptPath == "" {
		http.NotFound(w, r)
		return
	}
	// Guard against path traversal: only ever serve a bare filename.
	clean := filepath.Base(e.ReceiptPath)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, filepath.Join(h.uploadDir, clean))
}
