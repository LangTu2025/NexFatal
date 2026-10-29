package g79client

// 登录成功后返回的会话信息
type LoginEntity struct {
	EntityID string `json:"entity_id"`
	Token    string `json:"token"`
	Sead     string `json:"sead"`
}

// 登录响应
type LoginResponse struct {
	Response
	Entity LoginEntity `json:"entity"`
}

// Android 客户端服务地址
type G79ReleaseJSON struct {
	CoreServerURL            string `json:"CoreServerUrl"`
	AuthServerURL            string `json:"AuthServerUrl"`
	WebServerUrl             string `json:"WebServerUrl"`
	ApiGatewayUrl            string `json:"ApiGatewayUrl"`
	TransferServerUrl        string `json:"TransferServerUrl"`
	TransferServerHttpUrl    string `json:"TransferServerHttpUrl"`
	TransferServerNewHttpUrl string `json:"TransferServerNewHttpUrl"`
	MomentURL                string `json:"MomentUrl"`
	DCWebURL                 string `json:"DCWebUrl"`
	ChatServerURL            string `json:"ChatServerUrl"`
	LinkServerURL            string `json:"LinkServerUrl"`
}
