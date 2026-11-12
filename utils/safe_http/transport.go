package safe_http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// RequiresEncryption matches the user centre itself, never a similarly named host.
func RequiresEncryption(address string) bool {
	u, err := url.Parse(address)
	return err == nil && strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), "user.aurelrune.com")
}

// NewTransport preserves NovaBuilder's RSA/AES envelope only for the user centre.
// Other services receive their original HTTP request and response unchanged.
func NewTransport(serverPublicPEM, clientPrivatePEM string, base http.RoundTripper) (http.RoundTripper, error) {
	crypto, err := NewCrypto(serverPublicPEM, clientPrivatePEM)
	if err != nil {
		return nil, err
	}
	return newDomainTransport(crypto, base), nil
}

type domainTransport struct {
	crypto *Crypto
	base   http.RoundTripper
}

func newDomainTransport(crypto *Crypto, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &domainTransport{crypto: crypto, base: base}
}

func (t *domainTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !RequiresEncryption(req.URL.String()) {
		return t.base.RoundTrip(req)
	}
	out := req.Clone(req.Context())
	if out.Header.Get("Accept") == "" {
		out.Header.Set("Accept", "application/json")
	}
	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		encrypted, err := t.crypto.Encrypt(body)
		if err != nil {
			return nil, err
		}
		out.Body = io.NopCloser(strings.NewReader(encrypted))
		out.ContentLength = int64(len(encrypted))
		out.Header.Del("Content-Length")
		out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(encrypted)), nil }
	}
	if out.Header.Get("X-Client-Public-Key") == "" {
		out.Header.Set("X-Client-Public-Key", base64.StdEncoding.EncodeToString([]byte(t.crypto.ClientPublicKey)))
	}
	resp, err := t.base.RoundTrip(out)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	decoded, err := (&SafeClient{crypto: t.crypto}).decodeBody(body)
	if err != nil {
		// Flowers /api/new may return a plain opaque session token.
		if resp.StatusCode < 400 && (req.Method != http.MethodGet || !strings.HasSuffix(req.URL.Path, "/api/new") || json.Valid(body)) {
			return nil, err
		}
		decoded = body
	}
	resp.Body = io.NopCloser(bytes.NewReader(decoded))
	resp.ContentLength = int64(len(decoded))
	resp.Header.Set("Content-Length", strconv.Itoa(len(decoded)))
	return resp, nil
}
