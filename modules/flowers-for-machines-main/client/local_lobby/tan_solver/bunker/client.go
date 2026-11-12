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
	SessionToken       string
	httpClient         *http.Client
}

func NewClient(authServer string, token string, peAuth string, saAuth string) *Client {
	return NewClientWithOwnerHint(authServer, token, peAuth, saAuth, "")
}

func NewClientWithOwnerHint(authServer string, token string, peAuth string, saAuth string, ownerHint string) *Client {
	return NewClientWithSession(authServer, token, peAuth, saAuth, ownerHint, "")
}

func NewClientWithSession(authServer string, token string, peAuth string, saAuth string, ownerHint string, sessionToken string) *Client {
	return &Client{
		AuthServer:         authServer,
		FBToken:            token,
		ProvidedPEAuthData: peAuth,
		ProvidedSaAuthData: saAuth,
		OwnerHint:          ownerHint,
		SessionToken:       sessionToken,
		httpClient:         &http.Client{},
	}
}

func (client *Client) post(url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if client.SessionToken != "" {
		req.Header.Set("Authorization", "Bearer "+client.SessionToken)
	}
	return client.httpClient.Do(req)
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
