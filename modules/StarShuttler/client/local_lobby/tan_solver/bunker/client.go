package bunker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	AuthServer         string
	FBToken            string
	ProvidedPEAuthData string
	ProvidedSaAuthData string
	OwnerHint          string
	httpClient         *http.Client
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	cloned := req.Clone(req.Context())
	if t.token != "" {
		cloned.Header.Set("Authorization", "Bearer "+t.token)
	}
	return base.RoundTrip(cloned)
}

func NewClient(authServer string, token string, peAuth string, saAuth string) *Client {
	return NewClientWithOwnerHint(authServer, token, peAuth, saAuth, "")
}

func NewClientWithOwnerHint(authServer string, token string, peAuth string, saAuth string, ownerHint string) *Client {
	return &Client{
		AuthServer:         authServer,
		FBToken:            token,
		ProvidedPEAuthData: peAuth,
		ProvidedSaAuthData: saAuth,
		OwnerHint:          ownerHint,
		httpClient:         &http.Client{Transport: bearerTransport{token: token}},
	}
}

func (client *Client) post(url string, body []byte) (*http.Response, error) {
	return client.httpClient.Post(url, "application/json", bytes.NewReader(body))
}

func parseHttpResponse[T any](resp *http.Response) (result T, err error) {
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return result, fmt.Errorf("parseHttpResponse: The status code of http request is not 200 (code = %d)", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("parseHttpResponse: %v", err)
	}

	err = json.Unmarshal(bodyBytes, &result)
	if err != nil {
		return result, fmt.Errorf("parseHttpResponse: %v", err)
	}

	return result, nil
}
