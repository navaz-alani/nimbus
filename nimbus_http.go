package nimbus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
)

const (
	DefaultTransferBuffSize = 256
	Mb10                    = (10 << 20)
)

// Extension sets
var (
	// Set of extension indicating that all extensions should be allowed.
	ExtAll = []string{"_all_"}
	// Set of image extensions which are most commonly used on the web.
	// From: developer.mozilla.org/en-US/docs/Web/Media/Formats/Image_types
	ExtImg = []string{
		".apng", ".avif", ".gif", ".jpg", ".jpeg", ".jfif",
		".pjpeg", ".pjp", ".png", ".svg", ".webp", ".bmp",
	}
	// Set of extensions only allowing compressed files
	ExtComp = []string{".zip", ".tar", ".tgz", ".gz", ".bz2"}
	// Set of extensions only allowing text files
	ExtTxt = []string{".txt"}
)

// NimbusHTTPFormImpl is a NimbusHTTP implementation which handles file uploads
// performed using HTML forms or the FormData web API.
type NimbusHTTPFormImpl struct {
	mu                sync.RWMutex
	maxSize           int64
	tBuffSize         int64
	dfk               string
	mimeCache         map[string][]string
	tmpDir            string
	allowedExtensions []string
	allowNoExt        bool
}

// NNewHTTPFormImpl creates and returns the form implementation of NimbusHTTP.
// The `dfk` argument is the "default file key" which is a string indicating
// the name of the file field in requests to be received. `maxSize` specifies
// the maximum supported file size and `buffSize` indicates the copy buffer
// size. `tmpDir` is the directory in which the uploaded files will be stored
// as temporary files. `exts` is a slice containing the extensions which should
// be permitted. `allowNoExt` specifies whether files without extensions should
// be handled.
func NewHTTPFormImpl(dfk string, maxSize, buffSize int64, tmpDir string,
	exts []string, allowNoExt bool) (NimbusHTTP, error) {
	// create tmpdir if it doesn't already exist
	_ = os.Mkdir(tmpDir, 0755)
	return &NimbusHTTPFormImpl{
		maxSize:           maxSize,
		tBuffSize:         buffSize,
		dfk:               dfk,
		mimeCache:         make(map[string][]string),
		tmpDir:            tmpDir,
		allowedExtensions: exts,
		allowNoExt:        allowNoExt,
	}, nil
}

func (n *NimbusHTTPFormImpl) Cleanup() {
	// delete tmpdir (and all contents) created during initialization
	_ = os.RemoveAll(n.tmpDir)
}

// get path to saved file with given name
func (n *NimbusHTTPFormImpl) tmpFilePath(name string) string {
	// no need to acquire mutex since `tmpDir` never changes
	return fmt.Sprintf("%s/%s", n.tmpDir, path.Base(name))
}

// isExtAllowed checks whether the extension provided is allowed to be handled
// or not. If the error returned is nil, then the extension may be handled.
// Otherwise, the Error() string of the returned error indicates the reason the
// extension is not allowed.
func (n *NimbusHTTPFormImpl) isExtAllowed(ext string) error {
	if ext == "" && !n.allowNoExt {
		return fmt.Errorf("no-extension files not permitted")
	} else {
		if len(n.allowedExtensions) == 1 && n.allowedExtensions[0] == "_all_" {
			return nil // all extensions are allowed
		}
		var allowed bool
		for _, e := range n.allowedExtensions {
			if ext == e {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("file extension not permitted")
		}
		return nil
	}
}

// write is a helper which writes the contents of the file `f` to the writer `w`
// in chunks of `buffSize`.
func write(f multipart.File, w io.Writer, buffSize int64) error {
	buff := make([]byte, buffSize)
	for {
		n, err := f.Read(buff)
		if err == io.EOF && n == 0 {
			break
		} else if err != nil {
			return err
		}
		w.Write(buff)
	}
	return nil
}

type downloadedFile struct {
	name        string
	contentType []string
}

func (n *NimbusHTTPFormImpl) downloadFromRequest(r *http.Request, fileKey string) (*downloadedFile, error) {
	uploaded, hdr, err := r.FormFile(fileKey)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain file from request")
	}
	defer uploaded.Close()

	fExt := filepath.Ext(hdr.Filename)
	if err := n.isExtAllowed(fExt); err != nil {
		return nil, err
	}

	n.mu.RLock()
	tempFile, err := ioutil.TempFile(n.tmpDir, fmt.Sprintf("*%s", fExt))
	n.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	defer tempFile.Close()

	// read file into transfer buffer and write in chunks to avoid reading the
	// whole file at once
	if err := write(uploaded, tempFile, n.tBuffSize); err != nil {
		return nil, err
	}
	return &downloadedFile{
		name:        tempFile.Name(),
		contentType: hdr.Header["Content-Type"],
	}, nil
}

// Upload defines the endpoint which performs the download to the server.
// A single file is expected, with the form
// It is assumed that files will contain extensions and that the extension is
// the part of the filename from the last '.' to the end of the filename. This
// substring is required to be non-empty.
func (n *NimbusHTTPFormImpl) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(n.maxSize); err != nil {
		http.Error(w, "error parsing form", http.StatusBadRequest)
		return
	}
	if downloadedFile, err := n.downloadFromRequest(r, n.dfk); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		// cache hdr for this file so that it can be downloaded with the same hdr
		n.mu.Lock()
		n.mimeCache[downloadedFile.name] = downloadedFile.contentType
		n.mu.Unlock()
		w.Write([]byte(path.Base(downloadedFile.name)))
	}
}

// Download defines the endpoint which writes the first requested file from the
// request queries under the specified "default file key" to the user.
func (n *NimbusHTTPFormImpl) Download(w http.ResponseWriter, r *http.Request) {
	files := r.URL.Query()[n.dfk]
	if len(files) == 0 {
		http.Error(w, "expected file name", http.StatusBadRequest)
		return
	}
	fName := n.tmpFilePath(files[0])
	f, err := os.Open(fName)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot open: %s", path.Base(fName)), http.StatusBadRequest)
		return
	}
	// set headers as they were when the file was uploaded (obtain mu for reading)
	n.mu.RLock()
	for _, t := range n.mimeCache[files[0]] {
		r.Header.Add("Content-Type", t)
	}
	n.mu.RUnlock()
	if err := write(f, w, n.tBuffSize); err != nil {
		http.Error(w, fmt.Sprintf("cannot write: %s", path.Base(fName)), http.StatusInternalServerError)
		return
	}
}

func (n *NimbusHTTPFormImpl) UploadMany(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// DownloadMany decodes a JSON body from the response with a string[] in the
// `filenames` field which specifies which files to archive and download.
// A zip archive is returned when all specified files exist and the archiving
// process does not encounter any errors. Otherwise, the encountered error is
// reported.
func (n *NimbusHTTPFormImpl) DownloadMany(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filenames []string `json:"filenames"`
	}
	// decode filenames
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println(err)
		http.Error(w, "failed to decode request", http.StatusBadRequest)
		return
	}
	// create new zip archive
	archiveErrStub := "failed to compile archive: "
	archive := new(bytes.Buffer)
	archiver := NewZipper(archive)
	for _, filename := range req.Filenames {
		// add to zip arhive
		if err := archiver.AddFile(n.tmpFilePath(filename)); err != nil {
			http.Error(w, archiveErrStub+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := archiver.Close(); err != nil {
		http.Error(w, archiveErrStub+err.Error(), http.StatusInternalServerError)
		return
	}
	// write archive to response
	w.Header().Add("Content-Type", "application/zip")
	w.Header().Add("Content-Disposition", "attachment; filename=\"archive.zip\"")
	w.Write(archive.Bytes())
}

func (n *NimbusHTTPFormImpl) Delete(w http.ResponseWriter, r *http.Request) {
	files := r.URL.Query()[n.dfk]
	if len(files) == 0 {
		http.Error(w, "expected file name", http.StatusBadRequest)
		return
	}
	fName := n.tmpFilePath(files[0])
	err := os.Remove(n.tmpFilePath(fName))
	if err != nil {
		http.Error(w, "failed to delete file: "+err.Error(), http.StatusBadRequest)
		return
	}
	n.mu.Lock()
	delete(n.mimeCache, path.Base(fName))
	n.mu.Unlock()
}
