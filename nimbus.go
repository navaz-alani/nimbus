package nimbus

import (
	"net/http"
)

// NimbusHTTP defines the HTTP interface for the Nimbus file server.
type NimbusHTTP interface {
	Upload(w http.ResponseWriter, r *http.Request)
	UploadMany(w http.ResponseWriter, r *http.Request)
	Download(w http.ResponseWriter, r *http.Request)
	DownloadMany(w http.ResponseWriter, r *http.Request)
	Cleanup()
}
