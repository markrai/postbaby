package httpcache

import "net/http"

func SetNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}
