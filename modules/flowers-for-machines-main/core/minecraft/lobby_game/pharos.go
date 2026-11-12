package lobby_game

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"time"
)

// SpeedUpAddress bounds the optional Pharos lookup before direct RakNet fallback.
func SpeedUpAddress(ctx context.Context, address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return "", fmt.Errorf("loopback LobbyGame does not need Pharos")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://dual.pharos.netease.com/api/v1/harbor/mcrealms?ips="+url.QueryEscape(address), nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Pharos HTTP %d", res.StatusCode)
	}
	var response struct {
		Harbor struct {
			Servers map[string]map[string][][]string `json:"servers"`
			Warning string                           `json:"warning"`
		} `json:"harbor"`
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", err
	}
	nodes := response.Harbor.Servers[host][port]
	if response.Harbor.Warning != "" || len(nodes) == 0 {
		return "", fmt.Errorf("no LobbyGame Pharos node")
	}
	node := nodes[rand.Intn(len(nodes))]
	if len(node) < 2 {
		return "", fmt.Errorf("invalid LobbyGame Pharos node")
	}
	return net.JoinHostPort(node[0], node[1]), nil
}
