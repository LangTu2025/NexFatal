package defines

import (
	"encoding/json"
	"strings"

	wsdefine "github.com/xingbaiawa/LycorisFlow/define"
	"github.com/LangTu2025/NexFatal/constants"
)

var defaultAuthServerAddress = constants.UserCenterURL

// ControlConfig 保存连接目标服务器所需的配置
type ControlConfig struct {
	AuthServerAddress          string            `json:"-"`
	AuthServerToken            string            `json:"-"`
	AuthServerTokenVolatile    bool              `json:"-"`
	ForcePromptAuthServerToken bool              `json:"-"`
	LoginCookie                string            `json:"-"`
	ConsoleWorldPos            wsdefine.BlockPos `json:"-"`
	ServerKind                 string            `json:"server_kind,omitempty"`
	RentalServerCode           string            `json:"rental_server_code"`
	RentalServerPasscode       string            `json:"rental_server_passcode,omitempty"`
}

type controlConfigJSON struct {
	AuthServerAddress    string `json:"auth_server_address,omitempty"`
	AuthServerToken      string `json:"auth_server_token,omitempty"`
	ServerKind           string `json:"server_kind,omitempty"`
	RentalServerCode     string `json:"rental_server_code"`
	RentalServerPasscode string `json:"rental_server_passcode,omitempty"`
}

const (
	// ServerKindRental 表示租赁服
	ServerKindRental = "rental"
	// ServerKindDomain 表示山头
	ServerKindDomain = "domain"
	// ServerKindTanLobby 表示本地联机
	ServerKindTanLobby = "tan_lobby"
	// ServerKindLobby 表示联机大厅
	ServerKindLobby = "lobby"
)

// NewControlConfig 创建带默认值的控制配置
func NewControlConfig() *ControlConfig {
	cfg := &ControlConfig{}
	cfg.Normalize()
	return cfg
}

// DefaultAuthServerAddress 返回当前默认认证服务器地址
func DefaultAuthServerAddress() string {
	return defaultAuthServerAddress
}

// SetDefaultAuthServerAddress 设置默认认证服务器地址
func SetDefaultAuthServerAddress(addr string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		defaultAuthServerAddress = constants.UserCenterURL
		return
	}
	defaultAuthServerAddress = addr
}

// Normalize 规范化服务器地址、服务器类型、服务器号和密码
func (c *ControlConfig) Normalize() {
	c.AuthServerAddress = strings.TrimSpace(c.AuthServerAddress)
	if c.AuthServerAddress == "" {
		c.AuthServerAddress = DefaultAuthServerAddress()
	}
	c.ServerKind = strings.TrimSpace(c.ServerKind)
	c.RentalServerCode = fixServerColon(c.RentalServerCode)
	c.RentalServerPasscode = strings.TrimSpace(c.RentalServerPasscode)
	if c.ServerKind == "" || c.ServerKind == ServerKindRental {
		c.ServerKind = DetectServerKind(c.RentalServerCode)
	}
	c.RentalServerCode = ensureServerTargetPrefix(c.ServerKind, c.RentalServerCode)
	c.RentalServerCode, c.RentalServerPasscode = splitServerTarget(c.RentalServerCode, c.RentalServerPasscode)
	c.ServerKind = DetectServerKind(c.RentalServerCode)
}

// DetectServerKind 根据服务器号前缀或形态推断服务器类型
func DetectServerKind(serverCode string) string {
	normalized := fixServerColon(serverCode)
	prefix, _, ok := strings.Cut(normalized, ":")
	if ok {
		switch strings.TrimSpace(prefix) {
		case "山头", "DomainGame", "PCDomainGame":
			return ServerKindDomain
		case "本地联机", "TanLobby":
			return ServerKindTanLobby
		case "联机大厅", "大厅", "LobbyGame":
			return ServerKindLobby
		case "服务器":
			return ServerKindRental
		}
	}
	if normalized != "" && !rentalServerCode(normalized) && normalized != "MainCity" {
		return ServerKindDomain
	}
	return ServerKindRental
}

func fixServerColon(serverCode string) string {
	return strings.ReplaceAll(strings.TrimSpace(serverCode), "：", ":")
}

func ensureServerTargetPrefix(kind, serverCode string) string {
	normalized := fixServerColon(serverCode)
	if normalized == "" || hasServerTargetPrefix(normalized) {
		return normalized
	}
	switch kind {
	case ServerKindDomain:
		return "DomainGame:" + normalized
	case ServerKindTanLobby:
		return "TanLobby:" + normalized
	case ServerKindLobby:
		return "LobbyGame:" + normalized
	default:
		return normalized
	}
}

func hasServerTargetPrefix(serverCode string) bool {
	prefix, _, ok := strings.Cut(fixServerColon(serverCode), ":")
	if !ok {
		return false
	}
	switch strings.TrimSpace(prefix) {
	case "服务器", "山头", "本地联机", "联机大厅", "大厅",
		"DomainGame", "PCDomainGame", "TanLobby", "LobbyGame":
		return true
	default:
		return false
	}
}

func splitServerTarget(serverCode, passcode string) (string, string) {
	normalized := fixServerColon(serverCode)
	prefix, value, ok := strings.Cut(normalized, ":")
	if !ok {
		if normalized != "" && !rentalServerCode(normalized) && normalized != "MainCity" {
			return "DomainGame:" + normalized, ""
		}
		return normalized, strings.TrimSpace(passcode)
	}
	value = strings.TrimSpace(value)
	switch strings.TrimSpace(prefix) {
	case "山头":
		return "DomainGame:" + value, ""
	case "联机大厅", "大厅":
		return "LobbyGame:" + value, strings.TrimSpace(passcode)
	case "本地联机":
		return "TanLobby:" + value, strings.TrimSpace(passcode)
	case "DomainGame", "PCDomainGame":
		return strings.TrimSpace(prefix) + ":" + value, ""
	case "LobbyGame":
		return "LobbyGame:" + value, strings.TrimSpace(passcode)
	case "TanLobby":
		return "TanLobby:" + value, strings.TrimSpace(passcode)
	case "服务器":
		return value, strings.TrimSpace(passcode)
	default:
		return normalized, strings.TrimSpace(passcode)
	}
}

func rentalServerCode(serverCode string) bool {
	if len(serverCode) < 4 || len(serverCode) > 8 {
		return false
	}
	for _, ch := range serverCode {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (c ControlConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(controlConfigJSON{
		ServerKind:           c.ServerKind,
		RentalServerCode:     c.RentalServerCode,
		RentalServerPasscode: c.RentalServerPasscode,
	})
}

func (c *ControlConfig) UnmarshalJSON(data []byte) error {
	var payload controlConfigJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	c.AuthServerToken = payload.AuthServerToken
	c.ServerKind = payload.ServerKind
	c.RentalServerCode = payload.RentalServerCode
	c.RentalServerPasscode = payload.RentalServerPasscode
	c.Normalize()
	return nil
}

// FromJSON 从 JSON 字符串恢复控制配置
func (c *ControlConfig) FromJSON(str string) error {
	return json.Unmarshal([]byte(str), c)
}

// ToJSON 将控制配置序列化为 JSON 字符串
func (c *ControlConfig) ToJSON() (string, error) {
	c.Normalize()
	bs, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}
