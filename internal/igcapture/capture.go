// Package capture is a temporary login diagnostic collector; remove it before the fix PR.
package capture

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

const Format = "ig-login-capture-v1"
const maxBody = 2 << 20
const maxFile = 16 << 20

// TEMPORARY: public encryption key only; private key never leaves the operator's machine.
const temporaryPublicKey = "MIIBojANBgkqhkiG9w0BAQEFAAOCAY8AMIIBigKCAYEAnPOzm2Zl8fJlVVRe4GFTwBFqf1XRkqHwXlwctR+jhq8FF2Q+6qanYXKWbZzoAh2v3mfyGw/PSfug8YOPP6IXuYHBhnc3vEdyTXq1cjnHKTUrKF8oCzGWkwEk3igexR+UAsTFwYCy1nbH9pldAVuYmNcOZpyht9M13WEqTkE41HZgl8vunaKhd3Crrpa1PDJnYqC44EX7pvtssICGwzDNIabnxuhr1tqJCVVSrY1setUBha2u1sxsi5gCa/KhWYGokShZAZNOUy5czZgbzKDN9VIPE/f1jhyT/Q5R4U2aQ9yivxM1+YCbFx/cm8OeMk9hHSJslD1E5mkIN24cBqoxEOIL27V4BCzGN7Y1YWKNFoGlhxZ1yScbQ4alRRjKUUbhTxrEAwgmqhKgPkqUakYpuEI8i42vw/Lw47tUYjFUBh7/a6+oL7phITeoZgwaWi9iyL9K8FhK4JG0HJPjOeCu3STNNX14rjlDNnda1VNx8FHT3Y2SiHHKEjIyraQ272iRAgMBAAE="
const temporaryExpires = "2026-09-07T17:27:00Z"
const temporaryFilename = "ig-login-capture.enc"

var claimed atomic.Bool

type Header struct {
	Format string `json:"format"`
	Key    []byte `json:"wrapped_key"`
}

type Envelope struct {
	Sequence int    `json:"sequence"`
	Nonce    []byte `json:"nonce"`
	Data     []byte `json:"ciphertext"`
}

type Record struct {
	Kind            string      `json:"kind"`
	Time            time.Time   `json:"time"`
	DurationMS      int64       `json:"duration_ms,omitempty"`
	Method          string      `json:"method,omitempty"`
	Operation       string      `json:"operation,omitempty"`
	URL             string      `json:"url,omitempty"`
	RequestHeaders  http.Header `json:"request_headers,omitempty"`
	Status          int         `json:"status,omitempty"`
	ResponseHeaders http.Header `json:"response_headers,omitempty"`
	ResponseURL     string      `json:"response_url,omitempty"`
	Protocol        string      `json:"protocol,omitempty"`
	Uncompressed    bool        `json:"uncompressed,omitempty"`
	Body            []byte      `json:"body,omitempty"`
	Truncated       bool        `json:"truncated,omitempty"`
	TransportError  bool        `json:"transport_error,omitempty"`
	ReadError       bool        `json:"read_error,omitempty"`
	Reason          string      `json:"reason,omitempty"`
}

type Recorder struct {
	base     http.RoundTripper
	file     captureOutput
	aead     cipher.AEAD
	deadline time.Time
	secrets  []string
	mu       sync.Mutex
	seq      int
	written  int
	closed   bool
	timer    *time.Timer
}

type captureOutput interface {
	io.WriteCloser
	Sync() error
}

// Open captures one new flow per process, independently of any old capture file.
func Open(user string, base http.RoundTripper, password string, log zerolog.Logger) (*Recorder, error) {
	if user != "@chr13:beeper.com" || base == nil {
		return nil, nil
	}
	return openConfigured(user, base, password, temporaryPublicKey, temporaryExpires, func() (captureOutput, error) {
		return newLogOutput(log, "new_login")
	})
}

func openConfigured(user string, base http.RoundTripper, password, key, expires string, output func() (captureOutput, error)) (*Recorder, error) {
	if user != "@chr13:beeper.com" || base == nil {
		return nil, nil
	}
	deadline, err := time.Parse(time.RFC3339, expires)
	if err != nil || !deadline.After(time.Now()) || time.Until(deadline) > 24*time.Hour {
		return nil, errors.New("invalid capture configuration")
	}
	der, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, errors.New("invalid capture public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	pub, ok := parsed.(*rsa.PublicKey)
	if err != nil || !ok || pub.N.BitLen() < 3072 || pub.N.BitLen() > 4096 {
		return nil, errors.New("invalid capture public key")
	}
	if !claimed.CompareAndSwap(false, true) {
		return nil, nil
	}
	return newRecorder(base, password, pub, output, deadline)
}

func newRecorder(base http.RoundTripper, password string, pub *rsa.PublicKey, output func() (captureOutput, error), deadline time.Time) (*Recorder, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, key, []byte(Format))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	f, err := output()
	if err != nil {
		return nil, errors.New("capture output unavailable")
	}
	if deadline.After(time.Now().Add(10 * time.Minute)) {
		deadline = time.Now().Add(10 * time.Minute)
	}
	r := &Recorder{base: base, file: f, aead: aead, deadline: deadline}
	r.addSecret(password)
	header, _ := json.Marshal(Header{Format, wrapped})
	header = append(header, '\n')
	if _, err = f.Write(header); err != nil {
		_ = f.Close()
		return nil, errors.New("capture header write failed")
	}
	r.written = len(header)
	r.mu.Lock()
	r.timer = time.AfterFunc(time.Until(deadline), r.Close)
	r.mu.Unlock()
	return r, nil
}

func (r *Recorder) addSecret(value string) {
	if value == "" {
		return
	}
	quoted, _ := json.Marshal(value)
	r.secrets = append(r.secrets, value, url.QueryEscape(value), url.PathEscape(value), string(quoted[1:len(quoted)-1]))
}

func (r *Recorder) redact(value string) string {
	for _, secret := range r.secrets {
		value = strings.ReplaceAll(value, secret, "[OMITTED]")
	}
	return value
}

func (r *Recorder) headers(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, values := range h {
		for _, value := range values {
			out.Add(r.redact(k), r.redact(value))
		}
	}
	return out
}

func allowed(req *http.Request) bool {
	if req.URL.Scheme != "https" || req.URL.Host != "www.instagram.com" || req.URL.User != nil {
		return false
	}
	path := req.URL.Path
	if req.Method == http.MethodGet {
		return path == "/" || path == "/accounts/login/" || strings.HasPrefix(path, "/auth_platform/")
	}
	return req.Method == http.MethodPost && (path == "/api/v1/web/accounts/login/ajax/" || path == "/api/graphql")
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	active := !r.closed && time.Now().Before(r.deadline)
	if !active || !allowed(req) {
		r.mu.Unlock()
		return r.base.RoundTrip(req)
	}
	if r.seq >= 32 {
		r.finish("exchange_limit")
		r.mu.Unlock()
		return r.base.RoundTrip(req)
	}
	// Only inspect a replayable copy of the form to exclude its password envelope.
	// No request body or submitted verification code is ever recorded.
	var operation string
	if req.Method == http.MethodPost && req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			data, readErr := io.ReadAll(io.LimitReader(body, 64<<10))
			_ = body.Close()
			if form, err := url.ParseQuery(string(data)); err == nil && readErr == nil {
				r.addSecret(form.Get("enc_password"))
				operation = form.Get("fb_api_req_friendly_name")
			}
			clear(data)
		}
	}
	if req.URL.Path == "/api/graphql" && operation != "AuthPlatformCodeEntryViewQuery" && operation != "AuthPlatformChallengePickerViewQuery" {
		r.mu.Unlock()
		return r.base.RoundTrip(req)
	}
	record := Record{Kind: "exchange", Time: time.Now().UTC(), Method: req.Method, URL: r.redact(req.URL.String()), RequestHeaders: r.headers(req.Header)}
	record.Operation = r.redact(operation)
	r.mu.Unlock()
	start := time.Now()
	resp, err := r.base.RoundTrip(req)
	// Tee while the caller reads: no eager reads, changed errors, or extra requests.
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return resp, err
	}
	record.DurationMS, record.TransportError = time.Since(start).Milliseconds(), err != nil
	if resp != nil {
		record.Status, record.ResponseHeaders = resp.StatusCode, r.headers(resp.Header)
		record.Protocol, record.Uncompressed = resp.Proto, resp.Uncompressed
		if resp.Request != nil && resp.Request.URL != nil {
			record.ResponseURL = r.redact(resp.Request.URL.String())
		}
		if resp.Body != nil {
			resp.Body = &captureBody{ReadCloser: resp.Body, recorder: r, record: record}
			return resp, err
		}
	}
	r.write(record)
	return resp, err
}

type captureBody struct {
	io.ReadCloser
	mu       sync.Mutex
	recorder *Recorder
	record   Record
	data     bytes.Buffer
	once     sync.Once
}

func (b *captureBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxBody - b.data.Len()
	_, _ = b.data.Write(p[:min(n, remaining)])
	b.record.Truncated = b.record.Truncated || n > remaining
	if err != nil {
		b.record.ReadError = err != io.EOF
		b.save()
	}
	return n, err
}

func (b *captureBody) Close() error {
	err := b.ReadCloser.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record.Truncated = true
	b.save()
	return err
}

func (b *captureBody) save() {
	b.once.Do(func() {
		r := b.recorder
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.closed {
			b.record.Body = []byte(r.redact(b.data.String()))
			r.write(b.record)
		}
		clear(b.data.Bytes())
		b.data.Reset()
	})
}

func (r *Recorder) write(record Record) {
	if r.closed {
		return
	}
	plain, err := json.Marshal(record)
	if err != nil {
		r.disable()
		return
	}
	defer clear(plain)
	nonce := make([]byte, r.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		r.disable()
		return
	}
	r.seq++
	sealed := r.aead.Seal(nil, nonce, plain, []byte(Format+":"+strconv.Itoa(r.seq)))
	line, _ := json.Marshal(Envelope{r.seq, nonce, sealed})
	line = append(line, '\n')
	if r.written+len(line) > maxFile-4096 && record.Kind != "end" {
		r.seq--
		r.finish("byte_limit")
		return
	}
	n, err := r.file.Write(line)
	r.written += n
	if err != nil || n != len(line) || r.file.Sync() != nil {
		r.disable()
	}
}

func (r *Recorder) disable() {
	r.closed = true
	if r.timer != nil {
		r.timer.Stop()
	}
	_ = r.file.Close()
	r.secrets = nil
}

func (r *Recorder) finish(reason string) {
	if !r.closed {
		r.write(Record{Kind: "end", Time: time.Now().UTC(), Reason: reason})
		r.disable()
	}
}

func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	reason := "initial_step_returned"
	if !time.Now().Before(r.deadline) {
		reason = "expired"
	}
	r.finish(reason)
}
