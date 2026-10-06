package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Header names of a delivery request.
const (
	HeaderEvent     = "LevelUp-Event"
	HeaderDelivery  = "LevelUp-Delivery"
	HeaderSignature = "LevelUp-Signature"
)

// Sign returns the hex HMAC-SHA256 of "<unix ts>.<body>" keyed with the
// endpoint secret (the full "whsec_..." string, as bytes).
func Sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SignatureHeader is the LevelUp-Signature value: "t=<unix>,v1=<hex>".
func SignatureHeader(secret string, at time.Time, body []byte) string {
	ts := at.Unix()
	return "t=" + strconv.FormatInt(ts, 10) + ",v1=" + Sign(secret, ts, body)
}

// VerifySignature is the receiver-side check (documented for integrators,
// used by tests): the header must carry a v1 signature of body under secret
// whose timestamp is within tolerance of now.
func VerifySignature(secret, header string, body []byte, tolerance time.Duration, now time.Time) bool {
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return false
			}
			ts = n
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return false
	}
	if d := now.Sub(time.Unix(ts, 0)); d > tolerance || d < -tolerance {
		return false
	}
	want := []byte(Sign(secret, ts, body))
	for _, s := range sigs {
		if hmac.Equal(want, []byte(s)) {
			return true
		}
	}
	return false
}
