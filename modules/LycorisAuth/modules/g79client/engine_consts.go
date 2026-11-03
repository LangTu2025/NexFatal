package g79client

const (
	g79AndroidStep  = "695616851"
	g79AndroidStep2 = "2146985406"
	g79LibraryHash  = "c4fbc0625fe2ff8f3aa0b931feb04217"
	// 3.10.0.300878 libminecraftpe.so: 0x0e706468/0x0e70646c。
	// 原生签名函数直接使用 0-based 索引，不需要再减一。
	g79PESignGroupIndex = 2
	g79PESignRounds     = 8
	// 所给 APK Signing Block v2 中 885 字节 X.509 证书 DER 的 MD5。
	g79SignatureHash = "2b3e7ca013bb30a74d822579860c042b"
	g79BaseMCPHash   = ""
	// Android UniSDK 会话使用的渠道和 SDK 版本必须与 Cookie 的渠道一致。
	g79Android4399Channel    = "4399com"
	g79AndroidMpayChannel    = "netease"
	g79Android4399SDKVersion = "3.18.0"
	g79AndroidMpaySDKVersion = "5.16.0"
)

// 3.10.0.300878 APK: libraryHash 与证书 signatureHash 均来自同一安装包。
