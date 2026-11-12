package lobby_game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/bunker/auth"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

// RoomCandidate matches AuthService's /api/phoenix/lobby_search response.
// IDs remain strings throughout search, selection and login.
type RoomCandidate struct {
	RoomNumber string `json:"room_number"`
	RoomID     string `json:"room_id"`
	OwnerID    string `json:"owner_id,omitempty"`
	OwnerName  string `json:"owner_name,omitempty"`
	Version    string `json:"version,omitempty"`
	ServerCode string `json:"server_code"`
	Label      string `json:"label"`
}

func (r RoomCandidate) DisplayLabel() string {
	owner := strings.TrimSpace(r.OwnerName)
	if owner == "" && r.OwnerID != "" {
		owner = "UID " + r.OwnerID
	}
	if owner == "" {
		owner = "未知房主"
	}
	label := fmt.Sprintf("联机大厅 · 房主 %s · 房间号 %s", owner, r.RoomNumber)
	if r.Version != "" {
		label += " · 版本 " + r.Version
	}
	return label
}

// RoomSelector returns a zero-based candidate index.
type RoomSelector func(context.Context, []RoomCandidate) (int, error)

var ErrNoRooms = errors.New("找不到房间号完全匹配的联机大厅房间")

// SelectionRequiredError lets applications present their own room picker.
type SelectionRequiredError struct{ Rooms []RoomCandidate }

func (e *SelectionRequiredError) Error() string {
	var message strings.Builder
	fmt.Fprintf(&message, "找到 %d 个同号大厅，请选择房主后使用对应 -rsn 重试：", len(e.Rooms))
	for i, room := range e.Rooms {
		fmt.Fprintf(&message, "\n%d. %s（-rsn=\"%s\"）", i+1, room.DisplayLabel(), room.ServerCode)
	}
	return message.String()
}

// SearchRooms delegates pagination, owner lookup and rate-limit backoff to
// AuthService, which owns the authenticated NetEase account and API signatures.
// The search version always follows protocol.CurrentVersion in info.go.
func SearchRooms(ctx context.Context, client *auth.Client, token, roomNumber string) ([]RoomCandidate, error) {
	if client == nil || client.ClientOptions == nil {
		return nil, errors.New("LobbyGame search has no auth client")
	}
	roomNumber = strings.TrimSpace(roomNumber)
	if !validEntityID(roomNumber) {
		return nil, fmt.Errorf("无效的联机大厅房间号 %q", roomNumber)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("按房间号搜索联机大厅需要验证令牌（-ast）")
	}
	// The service may back off for 15 + 30 + 60 seconds when rate limited.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	body, err := json.Marshal(struct {
		Token   string `json:"login_token"`
		Keyword string `json:"keyword"`
		Version string `json:"version"`
	}{token, roomNumber, protocol.CurrentVersion})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(client.AuthServer, "/")+"/api/phoenix/lobby_search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.SessionToken())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("联机大厅房间搜索失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, errors.New("验证服务未提供 /api/phoenix/lobby_search，请更新 AuthService 或使用完整 entity ID")
		}
		return nil, fmt.Errorf("联机大厅房间搜索返回 HTTP %d", response.StatusCode)
	}
	var result struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Rooms   []RoomCandidate `json:"rooms"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析联机大厅搜索响应失败: %w", err)
	}
	if !result.Success {
		if result.Message == "" {
			result.Message = "验证服务未返回成功的搜索结果"
		}
		return nil, fmt.Errorf("联机大厅房间搜索失败: %s", result.Message)
	}
	rooms := make([]RoomCandidate, 0, len(result.Rooms))
	seen := make(map[string]bool)
	for _, room := range result.Rooms {
		room.RoomNumber, room.RoomID = strings.TrimSpace(room.RoomNumber), strings.TrimSpace(room.RoomID)
		if room.RoomNumber != roomNumber {
			continue
		}
		// AuthService distinguishes direct IDs by their 19-digit length. Reject
		// incomplete candidates instead of sending them back as another search.
		if !isConnectionID(room.RoomID) {
			return nil, errors.New("联机大厅搜索结果缺少有效的真实连接 ID")
		}
		if room.ServerCode != WirePrefix+room.RoomID {
			return nil, errors.New("联机大厅搜索结果的 server_code 与 room_id 不一致")
		}
		if seen[room.RoomID] {
			continue
		}
		seen[room.RoomID] = true
		rooms = append(rooms, room)
	}
	if len(rooms) == 0 {
		return nil, ErrNoRooms
	}
	return rooms, nil
}

func resolveServerCode(ctx context.Context, client *auth.Client, token, code string, selectRoom RoomSelector) (string, error) {
	target, ok := Detect(code)
	if !ok || !validEntityID(target) {
		return "", fmt.Errorf("无效的联机大厅房间号或 entity ID %q", code)
	}
	if isConnectionID(target) {
		return WirePrefix + target, nil
	}
	rooms, err := SearchRooms(ctx, client, token, target)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	index := 0
	if len(rooms) > 1 {
		if selectRoom == nil {
			return "", &SelectionRequiredError{Rooms: rooms}
		}
		index, err = selectRoom(ctx, append([]RoomCandidate(nil), rooms...))
		if err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if index < 0 || index >= len(rooms) {
		return "", errors.New("无效的联机大厅房间选择序号")
	}
	return rooms[index].ServerCode, nil
}

func isConnectionID(target string) bool { return len(target) == 19 && validEntityID(target) }
