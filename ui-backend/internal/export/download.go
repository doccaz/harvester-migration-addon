// download.go
//
// Downloading a finished OVA through the UI. Two routes:
//
//	POST /exports/{namespace}/{id}/download-ticket   (user token, like every route)
//	  Authorises the caller against the export Job with their own identity,
//	  makes sure the serve pod exists and is ready, and returns a signed URL.
//	GET  /exports/{namespace}/{id}/download?ticket=...   (ticket only)
//	  Streams the OVA from the serve pod to a plain browser download.
//
// The second route is the one deliberate exception to "every API route needs a
// user token": a browser download manager cannot send the header. It accepts
// only a valid ticket for exactly that export. See docs/export-download-design.md.
package export

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
)

// serveURL is where a serve pod listens; tests replace it.
var serveURL = func(ip string) *url.URL {
	return &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", ip, ServePort), Path: ServePath}
}

// DownloadTicket starts (if needed) the serve pod for a finished export and,
// once it is ready, returns the URL to download from. While the pod is still
// starting it answers 202 and the UI asks again.
func DownloadTicket(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ns, id := vars["namespace"], vars["id"]
		cfg := loadExportConfig()

		job, err := clients.Clientset.BatchV1().Jobs(ns).Get(r.Context(), exportJobName(id), metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Export not found: "+err.Error())
			return
		}
		if jobPhase(job) != PhaseReady {
			httpx.RespondWithError(w, http.StatusConflict, "The export has not finished; there is nothing to download yet")
			return
		}
		target := job.Annotations[exportAnnTargetName]
		if target == "" {
			httpx.RespondWithError(w, http.StatusNotFound, "Export has no recorded target file")
			return
		}

		key := ticketKey()
		st, err := ensureServePod(r.Context(), clients, job, cfg, key)
		if err != nil {
			log.Errorf("Could not prepare the download of export %s/%s: %v", ns, id, err)
			httpx.RespondWithAPIErrorMsg(w, err, "Could not start the download service for this export: "+err.Error())
			return
		}
		if !st.Ready {
			httpx.RespondWithJSON(w, http.StatusAccepted, map[string]interface{}{"state": "starting"})
			return
		}

		exp := time.Now().Add(ticketTTL)
		ticket := signTicket(key, ticketClaims{Namespace: ns, ID: id, IP: st.IP, Expires: exp.Unix()})
		httpx.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"state":     "ready",
			"url":       fmt.Sprintf("/api/v1/exports/%s/%s/download?ticket=%s", url.PathEscape(ns), url.PathEscape(id), url.QueryEscape(ticket)),
			"fileName":  target + ".ova",
			"expiresAt": exp.UTC().Format(time.RFC3339),
		})
	}
}

// DownloadProxy streams the OVA from the export's serve pod. It needs no
// Kubernetes access: the ticket carries the pod's address, and the pod's bearer
// token is derived from the ticket key.
func DownloadProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ns, id := vars["namespace"], vars["id"]
		key := ticketKey()
		ip, err := verifyTicket(key, r.URL.Query().Get("ticket"), ns, id, time.Now())
		if err != nil {
			httpx.RespondWithError(w, http.StatusUnauthorized, err.Error())
			return
		}
		token := serveToken(key, ns, id)
		target := serveURL(ip)

		rp := &httputil.ReverseProxy{
			// Build the upstream request from scratch: never forward the client's
			// Authorization, Cookie or query (the ticket), only what resuming and
			// caching need.
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL = target
				pr.Out.Host = target.Host
				pr.Out.Header = http.Header{"Authorization": {"Bearer " + token}}
				for _, h := range []string{"Range", "If-Range", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
					if v := pr.In.Header.Get(h); v != "" {
						pr.Out.Header.Set(h, v)
					}
				}
			},
			// Flush as data arrives: these are multi-gigabyte streams.
			FlushInterval: -1,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				log.Errorf("Download of export %s/%s failed: %v", ns, id, err)
				httpx.RespondWithError(w, http.StatusBadGateway,
					"The export's download service is not reachable; request a new download")
			},
		}
		rp.ServeHTTP(w, r)
	}
}
