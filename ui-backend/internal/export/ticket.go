// ticket.go
//
// Download tickets. A browser download cannot send the X-Migration-Token header,
// so the UI first asks (with its token) for a ticket and then navigates to a URL
// carrying it. The ticket is an HMAC-signed statement "this export, served by
// this pod IP, until this time", minted only after the caller was authorised
// against the export Job with their own identity. The download route verifies
// the signature and needs no Kubernetes access of its own.
//
// The same key derives the bearer token the serve pod expects, so the backend
// never has to read a Secret or a pod to talk to it.
package export

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// ticketTTL bounds a ticket. It is reusable inside the window because browsers
// resume an interrupted download with a new request carrying the same URL.
const ticketTTL = 30 * time.Minute

var (
	errBadTicket     = errors.New("invalid download ticket")
	errExpiredTicket = errors.New("download ticket expired")
)

type ticketClaims struct {
	Namespace string `json:"ns"`
	ID        string `json:"id"`
	IP        string `json:"ip"`
	Expires   int64  `json:"exp"`
}

var (
	keyOnce sync.Once
	keyVal  []byte
)

// ticketKey is the HMAC key: EXPORT_TICKET_KEY (the chart generates one so every
// replica and every restart agree), else a random per-process key, which works
// for a single replica but invalidates tickets when the pod restarts.
func ticketKey() []byte {
	keyOnce.Do(func() {
		if v := os.Getenv("EXPORT_TICKET_KEY"); len(v) >= 16 {
			keyVal = []byte(v)
			return
		}
		keyVal = make([]byte, 32)
		if _, err := rand.Read(keyVal); err != nil {
			log.Fatalf("cannot create a download ticket key: %v", err)
		}
		log.Warn("EXPORT_TICKET_KEY is unset or shorter than 16 bytes; using a random key, so download tickets are valid for this process only")
	})
	return keyVal
}

func mac(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// signTicket returns "<claims>.<signature>", both base64url.
func signTicket(key []byte, c ticketClaims) string {
	raw, _ := json.Marshal(c)
	p := base64.RawURLEncoding.EncodeToString(raw)
	return p + "." + base64.RawURLEncoding.EncodeToString(mac(key, p))
}

// verifyTicket checks the signature, the expiry, and that the ticket is for
// exactly this export. It returns the pod IP the ticket was minted for.
func verifyTicket(key []byte, ticket, namespace, id string, now time.Time) (string, error) {
	p, sig, ok := strings.Cut(ticket, ".")
	if !ok || p == "" || sig == "" {
		return "", errBadTicket
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac(key, p)) {
		return "", errBadTicket
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return "", errBadTicket
	}
	var c ticketClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", errBadTicket
	}
	if c.Namespace != namespace || c.ID != id {
		return "", errBadTicket
	}
	if now.Unix() >= c.Expires {
		return "", errExpiredTicket
	}
	if net.ParseIP(c.IP) == nil {
		return "", errBadTicket
	}
	return c.IP, nil
}

// serveToken is the bearer token the serve pod of one export accepts. It is
// derived, never stored, and differs per export and per key.
func serveToken(key []byte, namespace, id string) string {
	return hex.EncodeToString(mac(key, "serve|"+namespace+"|"+id))
}
