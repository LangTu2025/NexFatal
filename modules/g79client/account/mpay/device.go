package mpay

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xingbaiawa/g79client/utils"
)

const (
	deviceEndpoint          = "/mpay/games/aecfrxodyqaaaajp-g-x19/devices"
	deviceUploadEndpoint    = "/mpay/api/devices/upload"
	realNameEndpoint        = "/mpay/api/users/realname/update_by_token"
	realNameVerifyEndpoint  = "/mpay/api/users/realname/verify"
	defaultGameID           = "aecfrxodyqaaaajp-g-x19"
	defaultGV               = "840297099"
	defaultGVN              = "3.9.5.297099"
	defaultCV               = "a5.16.0"
	defaultSV               = "32"
	defaultAppType          = "games"
	defaultAppMode          = "2"
	defaultAppChannel       = "netease"
	defaultPkgChannel       = "netease"
	defaultJFGameID         = "x19"
	defaultMCountAppKey     = "EEkEEXLymcNjM42yLY3Bn6AO15aGy4yq"
	defaultCloudExtraBase64 = "e30="
	defaultOptFields        = "nickname,avatar,realname_status,mobile_bind_status,exit_popup_info,mask_related_mobile,related_login_status,detect_is_new_user"
	defaultRAMBytes         = "1035337728"
	defaultROMBytes         = "134208294912"
	defaultDeviceBrand      = "vivo"
	defaultDeviceModel      = "V2166A"
	defaultSystemVersion    = "12"
	defaultResolution       = "900*1600"
	defaultSC               = "0"
)

// 描述网易 MPay 服务返回的设备信息
type Device struct {
	Key                 string `json:"key"`
	ID                  string `json:"id"`
	MCountTransactionID string `json:"mcount_transaction_id"`
	TransID             string `json:"transid"`
	UniqueID            string `json:"unique_id"`
	UDID                string `json:"udid"`
	Mac                 string `json:"mac"`
	Ram                 string `json:"ram"`
	Rom                 string `json:"rom"`
	ExtCI               string `json:"ext_ci"`
	Timestamp           int64  `json:"timestamp"`

	client *Client
}

// 返回设备的浅拷贝，保留原有 client 以便继续调用接口
func (d *Device) Clone() *Device {
	if d == nil {
		return nil
	}
	cloned := *d
	return &cloned
}

// 便捷函数，使用默认客户端生成设备
func GenerateDevice(ctx context.Context) (*Device, error) {
	client := NewClient(nil)
	return client.GenerateDevice(ctx)
}

// 调用 MPay 接口生成设备凭据
func (c *Client) GenerateDevice(ctx context.Context) (*Device, error) {
	if c == nil {
		return nil, errors.New("GenerateDevice: Client is not initialized")
	}

	timestamp := time.Now().UnixMilli()
	macHex, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("GenerateDevice: %v", err)
	}
	ursUDID, err := randomHex(20)
	if err != nil {
		return nil, fmt.Errorf("GenerateDevice: %v", err)
	}
	udid, err := randomHex(8)
	if err != nil {
		return nil, fmt.Errorf("GenerateDevice: %v", err)
	}
	extCI, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("GenerateDevice: %v", err)
	}

	uniqueID := fmt.Sprintf("%s%d", uuid.NewString(), timestamp)
	mcountID := newMPayTransactionID(udid, timestamp)
	transID := newMPayTransactionID(udid, timestamp)

	form := c.buildDeviceForm(macHex, ursUDID, uniqueID, udid, extCI, mcountID, transID)

	body, status, err := c.postForm(ctx, deviceEndpoint, form)
	if err != nil {
		return nil, fmt.Errorf("GenerateDevice: %v", err)
	}
	if status != http.StatusCreated {
		if err := checkMPayError(body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("GenerateDevice: 生成设备失败 (status: %d)", status)
	}

	var parsed struct {
		Device struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"device"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("GenerateDevice: %v", err)
	}
	if parsed.Device.ID == "" || parsed.Device.Key == "" {
		return nil, errors.New("GenerateDevice: 设备响应缺少必要字段")
	}

	device := &Device{
		Key:                 parsed.Device.Key,
		ID:                  parsed.Device.ID,
		MCountTransactionID: mcountID,
		TransID:             transID,
		UniqueID:            uniqueID,
		UDID:                udid,
		Mac:                 macHex,
		Ram:                 defaultRAMBytes,
		Rom:                 defaultROMBytes,
		ExtCI:               extCI,
		Timestamp:           timestamp,
		client:              c,
	}
	_ = device.Upload(ctx)
	return device, nil
}

// 使用邮箱账号登录
func (d *Device) LoginEmail(ctx context.Context, email, password string) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, fmt.Errorf("LoginEmail: %v", err)
	}
	if email == "" || password == "" {
		return nil, errors.New("LoginEmail: 邮箱和密码不能为空")
	}

	hash := md5.Sum([]byte(password))
	return d.loginEmailWithHash(ctx, email, hex.EncodeToString(hash[:]), password)
}

// 使用邮箱账号登录，接受 32 位 MD5 密码
func (d *Device) LoginEmailMD5(ctx context.Context, email, passwordMD5 string) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, fmt.Errorf("LoginEmailMD5: %v", err)
	}
	if email == "" || passwordMD5 == "" {
		return nil, errors.New("LoginEmailMD5: 邮箱和密码摘要不能为空")
	}
	hash := strings.ToLower(strings.TrimSpace(passwordMD5))
	if len(hash) != 32 {
		return nil, errors.New("LoginEmailMD5: 密码摘要长度无效")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return nil, fmt.Errorf("LoginEmailMD5: %v", err)
	}
	return d.loginEmailWithHash(ctx, email, hash, hash)
}

func (d *Device) loginEmailWithHash(ctx context.Context, email, passwordHash, storedPassword string) (*User, error) {
	if email == "" || passwordHash == "" {
		return nil, errors.New("loginEmailWithHash: 邮箱和密码不能为空")
	}
	loginURL := fmt.Sprintf("%s/%s/users?un=ZmdsbmIxMjNAMTYzLmNvbQ%%3D%%3D", deviceEndpoint, d.ID)

	payload := map[string]string{
		"username":  email,
		"password":  strings.ToLower(passwordHash),
		"unique_id": d.UniqueID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("loginEmailWithHash: %v", err)
	}

	aesKey, err := hex.DecodeString(d.Key)
	if err != nil {
		return nil, fmt.Errorf("loginEmailWithHash: %v", err)
	}
	encrypted, err := utils.AesECBEncrypt(payloadBytes, aesKey)
	if err != nil {
		return nil, fmt.Errorf("loginEmailWithHash: %v", err)
	}

	form := d.baseForm()
	form.Set("opt_fields", defaultOptFields)
	form.Set("params", hex.EncodeToString(encrypted))

	body, status, err := d.client.postForm(ctx, loginURL, form)
	if err != nil {
		return nil, fmt.Errorf("loginEmailWithHash: %v", err)
	}
	if status != http.StatusOK {
		if err := checkMPayError(body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("loginEmailWithHash: 邮箱登录失败 (status: %d)", status)
	}
	if err := checkMPayError(body); err != nil {
		return nil, err
	}
	if storedPassword == "" {
		storedPassword = strings.ToLower(passwordHash)
	}
	return d.decodeUser(body, false, email, storedPassword)
}

// 生成游客账号
func (d *Device) Guest(ctx context.Context) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, fmt.Errorf("Guest: %v", err)
	}
	guestURL := fmt.Sprintf("%s/%s/users/by_guest", deviceEndpoint, d.ID)

	aesKey, err := hex.DecodeString(d.Key)
	if err != nil {
		return nil, fmt.Errorf("Guest: %v", err)
	}
	encrypted, err := utils.AesECBEncrypt([]byte("{}"), aesKey)
	if err != nil {
		return nil, fmt.Errorf("Guest: %v", err)
	}

	form := d.baseForm()
	form.Set("opt_fields", defaultOptFields)
	form.Set("params", hex.EncodeToString(encrypted))

	body, status, err := d.client.postForm(ctx, guestURL, form)
	if err != nil {
		return nil, fmt.Errorf("Guest: %v", err)
	}
	if status != http.StatusCreated {
		if err := checkMPayError(body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Guest: 生成游客账号失败 (status: %d)", status)
	}
	if err := checkMPayError(body); err != nil {
		return nil, err
	}
	return d.decodeUser(body, true, "", "")
}

// 上传设备信息。上传失败由调用方决定是否忽略
func (d *Device) Upload(ctx context.Context) error {
	if err := d.ensureClient(); err != nil {
		return fmt.Errorf("Upload: %v", err)
	}
	if d.ID == "" {
		return errors.New("Upload: 设备 ID 不能为空")
	}

	form := d.client.buildDeviceForm(d.Mac, "", d.UniqueID, d.UDID, d.ExtCI, d.MCountTransactionID, d.TransID)
	form.Del("init_urs_device")
	form.Set("device_id", d.ID)

	body, status, err := d.client.postForm(ctx, deviceUploadEndpoint, form)
	if err != nil {
		return fmt.Errorf("Upload: %v", err)
	}
	if status != http.StatusOK {
		if err := checkMPayError(body); err != nil {
			return err
		}
		return fmt.Errorf("Upload: 上传设备信息失败 (status: %d)", status)
	}
	if err := checkMPayError(body); err != nil {
		return err
	}
	return nil
}

// 调用 MPay 接口完成实名认证
func (d *Device) AuthRealName(ctx context.Context, user *User, realName, idNum string) error {
	if err := d.postRealName(ctx, realNameEndpoint, user, realName, idNum); err != nil {
		return fmt.Errorf("AuthRealName: %v", err)
	}
	return nil
}

// 调用 MPay 实名验证接口
func (d *Device) VerifyRealName(ctx context.Context, user *User, realName, idNum string) error {
	if err := d.postRealName(ctx, realNameVerifyEndpoint, user, realName, idNum); err != nil {
		return fmt.Errorf("VerifyRealName: %v", err)
	}
	return nil
}

func (d *Device) postRealName(ctx context.Context, path string, user *User, realName, idNum string) error {
	if err := d.ensureClient(); err != nil {
		return fmt.Errorf("postRealName: %v", err)
	}
	if user == nil {
		return errors.New("postRealName: 用户不能为空")
	}
	if realName == "" || idNum == "" {
		return errors.New("postRealName: 姓名和身份证号不能为空")
	}

	form := d.baseForm()
	form.Set("device_id", d.ID)
	form.Set("user_id", user.Sauth.SDKUID)
	form.Set("token", user.Sauth.SessionID)
	form.Set("realname", realName)
	form.Set("id_region", "86")
	form.Set("id_num", idNum)

	body, status, err := d.client.postForm(ctx, path, form)
	if err != nil {
		return fmt.Errorf("postRealName: %v", err)
	}
	if status != http.StatusOK {
		if err := checkMPayError(body); err != nil {
			return err
		}
		return fmt.Errorf("postRealName: 实名认证失败 (status: %d)", status)
	}
	return checkMPayError(body)
}

func (d *Device) baseForm() url.Values {
	values := url.Values{}
	values.Set("game_id", defaultGameID)
	values.Set("gv", defaultGV)
	values.Set("gvn", defaultGVN)
	values.Set("cv", defaultCV)
	values.Set("sv", defaultSV)
	values.Set("app_type", defaultAppType)
	values.Set("app_mode", defaultAppMode)
	values.Set("jf_game_id", defaultJFGameID)
	values.Set("pkg_channel", defaultPkgChannel)
	values.Set("app_channel", defaultAppChannel)
	values.Set("mcount_app_key", defaultMCountAppKey)
	values.Set("mcount_transaction_id", d.MCountTransactionID)
	values.Set("transid", d.TransID)
	values.Set("_cloud_extra_base64", defaultCloudExtraBase64)
	values.Set("sc", defaultSC)
	return values
}

func (d *Device) decodeUser(body []byte, isGuest bool, email, password string) (*User, error) {
	var payload struct {
		User map[string]any `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decodeUser: %v", err)
	}
	if payload.User == nil {
		return nil, errors.New("decodeUser: 响应缺少 user 字段")
	}

	tokenVal, ok := payload.User["token"]
	if !ok {
		return nil, errors.New("decodeUser: 用户信息缺少 token 字段")
	}
	token, err := toString(tokenVal)
	if err != nil || token == "" {
		if err != nil {
			return nil, fmt.Errorf("decodeUser: %v", err)
		}
		return nil, errors.New("decodeUser: token 字段为空")
	}

	idVal, ok := payload.User["id"]
	if !ok {
		return nil, errors.New("decodeUser: 用户信息缺少 ID 字段")
	}
	sdkuid, err := toString(idVal)
	if err != nil || sdkuid == "" {
		if err != nil {
			return nil, fmt.Errorf("decodeUser: %v", err)
		}
		return nil, errors.New("decodeUser: ID 字段为空")
	}

	if udidVal, ok := payload.User["udid"]; ok {
		if parsed, err := toString(udidVal); err == nil && parsed != "" {
			d.UDID = parsed
		}
	}

	return newUser(d.ID, sdkuid, token, d.UDID, d.Mac, isGuest, d.Ram, d.Rom, email, password), nil
}

func (d *Device) ensureClient() error {
	if d == nil {
		return errors.New("ensureClient: Device is not initialized")
	}
	if d.client == nil {
		d.client = NewClient(nil)
	}
	return nil
}

func checkMPayError(body []byte) error {
	var resp struct {
		Code      *int   `json:"code"`
		Reason    string `json:"reason"`
		VerifyURL string `json:"verify_url"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Code != nil {
		return &NeedVerifyError{Code: *resp.Code, Reason: resp.Reason, VerifyURL: resp.VerifyURL}
	}
	return nil
}

func newMPayTransactionID(udid string, timestamp int64) string {
	return fmt.Sprintf("%s_%d_%s", udid, timestamp, randomDigits(9))
}

func randomDigits(length int) string {
	if length <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(length)
	bytes, err := randomBytes(length)
	if err != nil {
		for i := 0; i < length; i++ {
			b.WriteByte('0')
		}
		return b.String()
	}
	for _, value := range bytes {
		b.WriteByte('0' + value%10)
	}
	return b.String()
}

func toString(v any) (string, error) {
	switch value := v.(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	case fmt.Stringer:
		return value.String(), nil
	case float64:
		return fmt.Sprintf("%.0f", value), nil
	case float32:
		return fmt.Sprintf("%.0f", value), nil
	case int:
		return fmt.Sprintf("%d", value), nil
	case int64:
		return fmt.Sprintf("%d", value), nil
	case uint64:
		return fmt.Sprintf("%d", value), nil
	case int32:
		return fmt.Sprintf("%d", value), nil
	case uint32:
		return fmt.Sprintf("%d", value), nil
	default:
		return "", fmt.Errorf("toString: Unsupported type %T", v)
	}
}
