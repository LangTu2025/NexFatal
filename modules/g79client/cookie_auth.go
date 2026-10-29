package g79client

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	x19_sdk "github.com/xingbaiawa/g79client/account/x19/sdk"
	"github.com/xingbaiawa/g79client/utils"
)

// Gitee 账号池中的 Android Cookie
type CookieData struct {
	SauthJSON string `json:"sauth_json"`
	MacAddr   string `json:"mac_addr"`
	RAM       string `json:"ram"`
	ROM       string `json:"rom"`
	IsGuest   *bool  `json:"is_guest"`
	Emulator  *int   `json:"emulator"`
}

func (c CookieData) macAddress() string {
	macAddress := strings.TrimSpace(c.MacAddr)
	if macAddress == "" {
		return "02:00:00:00:00:00"
	}
	return macAddress
}

func (c CookieData) ramValue() string {
	if strings.TrimSpace(c.RAM) == "" {
		return "4294967296"
	}
	return c.RAM
}

func (c CookieData) emulatorValue() int {
	if c.Emulator == nil {
		return 0
	}
	return *c.Emulator
}

// Cookie 内嵌的 UniSDK 会话
type SauthData struct {
	GameID         string `json:"gameid"`
	LoginChannel   string `json:"login_channel"`
	AppChannel     string `json:"app_channel"`
	Platform       string `json:"platform"`
	SDKUID         string `json:"sdkuid"`
	SessionID      string `json:"sessionid"`
	SDKVersion     string `json:"sdk_version"`
	UDID           string `json:"udid"`
	DeviceID       string `json:"deviceid"`
	TDID           string `json:"tdid"`
	AimInfo        string `json:"aim_info"`
	ClientLoginSN  string `json:"client_login_sn"`
	GasToken       string `json:"gas_token"`
	SourcePlatform string `json:"source_platform"`
	IP             string `json:"ip"`
	AccessToken    string `json:"access_token"`
}

// 最新 Android PE 认证请求
type PeAuthData struct {
	EngineVersion string         `json:"engine_version"`
	ExtraParam    string         `json:"extra_param"`
	Message       string         `json:"message"`
	PatchVersion  string         `json:"patch_version"`
	PayChannel    string         `json:"pay_channel"`
	SaData        string         `json:"sa_data"`
	SauthJSON     map[string]any `json:"sauth_json"`
	Seed          string         `json:"seed"`
	Sign          string         `json:"sign"`
}

// 使用最新版 Android 流程登录 Gitee Cookie 账号
func (c *Client) G79AuthenticateWithCookie(cookieString string) error {
	if c == nil {
		return fmt.Errorf("G79AuthenticateWithCookie: Client is nil")
	}

	// 解析账号池中的 Cookie
	cookie, sauth, err := parseCookie(cookieString)
	if err != nil {
		return fmt.Errorf("G79AuthenticateWithCookie: %v", err)
	}

	// 刷新当前 UniSDK 会话
	clientLoginSN, err := generateClientLoginSN()
	if err != nil {
		return fmt.Errorf("G79AuthenticateWithCookie: %v", err)
	}
	err = c.refreshAndroidUniSauth(cookieString, &sauth, clientLoginSN)
	if err != nil {
		return fmt.Errorf("G79AuthenticateWithCookie: %v", err)
	}

	// 完成 PE 认证并加载当前账号资料
	err = c.authenticatePE(&sauth, &cookie, clientLoginSN)
	if err != nil {
		return fmt.Errorf("G79AuthenticateWithCookie: %v", err)
	}
	userDetail, err := c.GetUserDetail()
	if err != nil {
		return fmt.Errorf("G79AuthenticateWithCookie: %v", err)
	}
	if userDetail.Code != 0 {
		return fmt.Errorf("G79AuthenticateWithCookie: 获取用户资料失败 (code: %d): %s", userDetail.Code, userDetail.Message)
	}
	c.UserDetail = &userDetail.Entity
	return nil
}

func parseCookie(cookieString string) (CookieData, SauthData, error) {
	if strings.TrimSpace(cookieString) == "" {
		return CookieData{}, SauthData{}, fmt.Errorf("parseCookie: Cookie is empty")
	}

	var cookie CookieData
	err := json.Unmarshal([]byte(cookieString), &cookie)
	if err != nil {
		return CookieData{}, SauthData{}, fmt.Errorf("parseCookie: %v", err)
	}
	if strings.TrimSpace(cookie.SauthJSON) == "" {
		return CookieData{}, SauthData{}, fmt.Errorf("parseCookie: Sauth JSON is empty")
	}

	var sauth SauthData
	err = json.Unmarshal([]byte(cookie.SauthJSON), &sauth)
	if err != nil {
		return CookieData{}, SauthData{}, fmt.Errorf("parseCookie: %v", err)
	}
	return cookie, sauth, nil
}

func generateClientLoginSN() (string, error) {
	randomValue, err := randomBytes(16)
	if err != nil {
		return "", fmt.Errorf("generateClientLoginSN: %v", err)
	}
	return hex.EncodeToString(randomValue), nil
}

func (c *Client) refreshAndroidUniSauth(cookieString string, sauth *SauthData, clientLoginSN string) error {
	sdkVersion := currentAndroidSDKVersion(sauth)
	request, err := x19_sdk.BuildUniSauthRequestFromCookieString(cookieString, &x19_sdk.UniSauthBuildOptions{
		SDKVersion: sdkVersion,
	})
	if err != nil {
		return fmt.Errorf("refreshAndroidUniSauth: %v", err)
	}
	request.HostID = 0
	request.ClientLoginSN = clientLoginSN

	response, err := x19_sdk.NewClient(c.httpClient).UniSauth(request)
	if err != nil {
		return fmt.Errorf("refreshAndroidUniSauth: %v", err)
	}
	if response.Code != http.StatusOK || response.Subcode != 0 {
		message := strings.TrimSpace(response.Msg)
		if message == "" {
			message = strings.TrimSpace(response.Status)
		}
		return fmt.Errorf("refreshAndroidUniSauth: 登录失败 (code: %d/%d): %s", response.Code, response.Subcode, message)
	}

	loginPayload, err := response.DecodeUniSDKLoginJSON()
	if err != nil {
		return fmt.Errorf("refreshAndroidUniSauth: %v", err)
	}
	accessToken := strings.TrimSpace(loginPayload.OAuth2.AccessToken)
	refreshToken := strings.TrimSpace(loginPayload.OAuth2.RefreshToken)
	if accessToken == "" || refreshToken == "" {
		return fmt.Errorf("refreshAndroidUniSauth: OAuth credentials are missing")
	}

	sauth.AccessToken = accessToken
	sauth.ClientLoginSN = clientLoginSN
	sauth.SDKVersion = sdkVersion
	if strings.TrimSpace(loginPayload.SDKUID) != "" {
		sauth.SDKUID = loginPayload.SDKUID
	}
	if strings.TrimSpace(loginPayload.GasToken) != "" {
		sauth.GasToken = loginPayload.GasToken
	} else if strings.TrimSpace(loginPayload.SDKToken) != "" {
		sauth.GasToken = loginPayload.SDKToken
	}
	return nil
}

func (c *Client) authenticatePE(sauth *SauthData, cookie *CookieData, clientLoginSN string) error {
	if sauth == nil || cookie == nil {
		return fmt.Errorf("authenticatePE: Authentication data is nil")
	}
	if c.G79LatestVersion == "" || c.patchResourcesHash == "" {
		return fmt.Errorf("authenticatePE: Patch metadata is missing")
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "sdkuid", value: sauth.SDKUID},
		{name: "sessionid", value: sauth.SessionID},
		{name: "udid", value: sauth.UDID},
		{name: "deviceid", value: sauth.DeviceID},
	} {
		if strings.TrimSpace(required.value) == "" {
			return fmt.Errorf("authenticatePE: Sauth JSON is missing %s", required.name)
		}
	}

	// 组装最新版客户端认证载荷
	seed := uuid.NewString()
	sauthPayload, err := buildAndroidSauthPayload(sauth, cookie.SauthJSON, clientLoginSN)
	if err != nil {
		return fmt.Errorf("authenticatePE: %v", err)
	}
	saDataJSON, err := json.Marshal(buildAndroidSaDataPayload(c, sauth, cookie))
	if err != nil {
		return fmt.Errorf("authenticatePE: %v", err)
	}
	message := c.EngineVersion + g79LibraryHash + c.G79LatestVersion + c.patchResourcesHash + g79SignatureHash + seed
	signature, err := utils.PeAuthSign(message, 3, 6)
	if err != nil {
		return fmt.Errorf("authenticatePE: %v", err)
	}
	payload, err := json.Marshal(PeAuthData{
		EngineVersion: c.EngineVersion,
		ExtraParam:    "extra",
		Message:       message,
		PatchVersion:  c.G79LatestVersion,
		PayChannel:    currentAndroidChannel(sauth),
		SaData:        string(saDataJSON),
		SauthJSON:     sauthPayload,
		Seed:          seed,
		Sign:          signature,
	})
	if err != nil {
		return fmt.Errorf("authenticatePE: %v", err)
	}

	loginResponse, err := c.sendPEAuthentication(payload)
	if err != nil {
		return fmt.Errorf("authenticatePE: %v", err)
	}
	if loginResponse.Code != 0 {
		return fmt.Errorf("authenticatePE: 登录失败 (code: %d): %s", loginResponse.Code, loginResponse.Message)
	}
	if strings.TrimSpace(loginResponse.Entity.EntityID) == "" || strings.TrimSpace(loginResponse.Entity.Token) == "" {
		return fmt.Errorf("authenticatePE: Login response is missing credentials")
	}

	c.SetCredentials(loginResponse.Entity.EntityID, loginResponse.Entity.Token)
	c.Sead = loginResponse.Entity.Sead
	return nil
}

func (c *Client) sendPEAuthentication(payload []byte) (*LoginResponse, error) {
	encryptedPayload, err := G79HttpEncrypt(payload)
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(c.ReleaseJSON.CoreServerURL, "/")+"/pe-authentication",
		strings.NewReader(hex.EncodeToString(encryptedPayload)),
	)
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}
	request.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("Content-Type", "application/json")

	response, err := c.do(request)
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := readResponseBody(response)
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}
	encryptedResponse, err := hex.DecodeString(strings.TrimSpace(string(responseBody)))
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}
	decryptedResponse, err := G79HttpDecrypt(encryptedResponse)
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}

	var result LoginResponse
	err = json.Unmarshal(GetValidJSON(decryptedResponse), &result)
	if err != nil {
		return nil, fmt.Errorf("sendPEAuthentication: %v", err)
	}
	return &result, nil
}

func buildAndroidSauthPayload(sauth *SauthData, rawSauthJSON, clientLoginSN string) (map[string]any, error) {
	if sauth == nil {
		return nil, fmt.Errorf("buildAndroidSauthPayload: Sauth data is nil")
	}

	payload := make(map[string]any)
	if strings.TrimSpace(rawSauthJSON) != "" {
		err := json.Unmarshal([]byte(rawSauthJSON), &payload)
		if err != nil {
			return nil, fmt.Errorf("buildAndroidSauthPayload: %v", err)
		}
	}

	// UniSDK 刷新会话后由原生客户端覆盖这些运行时字段
	channel := currentAndroidChannel(sauth)
	payload["aim_info"] = firstNonEmpty(sauth.AimInfo, defaultAimInfoJSON)
	payload["app_channel"] = firstNonEmpty(sauth.AppChannel, channel)
	payload["client_login_sn"] = clientLoginSN
	payload["deviceid"] = sauth.DeviceID
	payload["gameid"] = firstNonEmpty(sauth.GameID, "x19")
	payload["gas_token"] = strings.TrimSpace(sauth.GasToken)
	payload["get_access_token"] = "1"
	payload["ip"] = firstNonEmpty(sauth.IP, defaultLoopbackIP)
	payload["is_unisdk_guest"] = 0
	payload["login_channel"] = firstNonEmpty(sauth.LoginChannel, channel)
	payload["platform"] = g79AndroidPlatform
	payload["sdk_version"] = currentAndroidSDKVersion(sauth)
	payload["sdkuid"] = strings.TrimSpace(sauth.SDKUID)
	payload["sessionid"] = sauth.SessionID
	payload["source_app_channel"] = firstNonEmpty(sauth.AppChannel, channel)
	payload["source_platform"] = firstNonEmpty(sauth.SourcePlatform, g79AndroidPlatform)
	payload["udid"] = sauth.UDID
	payload["access_token"] = strings.TrimSpace(sauth.AccessToken)
	payload["tdid"] = strings.TrimSpace(sauth.TDID)
	return payload, nil
}

func buildAndroidSaDataPayload(c *Client, sauth *SauthData, cookie *CookieData) map[string]any {
	udid := firstNonEmpty(sauth.UDID, "0000000000000000")
	channel := currentAndroidChannel(sauth)
	return map[string]any{
		"app_channel":   firstNonEmpty(sauth.AppChannel, channel),
		"app_ver":       c.G79LatestVersion,
		"core_num":      "u0004",
		"cpu_digit":     "64",
		"cpu_hz":        "2465600",
		"cpu_name":      "placeholder",
		"device_height": "900",
		"device_model":  "SAMSUNG#SM-G977N",
		"device_width":  "1600",
		"disk":          "",
		"emulator":      cookie.emulatorValue(),
		"first_udid":    udid,
		"is_guest":      0,
		"launcher_type": "PE_C++",
		"mac_addr":      cookie.macAddress(),
		"network":       "CHANNEL_UNKNOW",
		"os_name":       g79AndroidOSName,
		"os_ver":        "5.1.1",
		"ram":           cookie.ramValue(),
		"rom":           cookie.ROM,
		"root":          false,
		"sdk_ver":       currentAndroidSDKVersion(sauth),
		"start_type":    "default",
		"udid":          udid,
	}
}

func currentAndroidChannel(sauth *SauthData) string {
	if sauth != nil {
		for _, channel := range []string{sauth.LoginChannel, sauth.AppChannel} {
			switch strings.ToLower(strings.TrimSpace(channel)) {
			case g79Android4399Channel:
				return g79Android4399Channel
			case g79AndroidMpayChannel:
				return g79AndroidMpayChannel
			}
		}
	}
	return g79Android4399Channel
}

func currentAndroidSDKVersion(sauth *SauthData) string {
	if currentAndroidChannel(sauth) == g79AndroidMpayChannel {
		return g79AndroidMpaySDKVersion
	}
	return g79Android4399SDKVersion
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
