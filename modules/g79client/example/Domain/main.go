package main

import (
	"fmt"
	"log"

	"github.com/xingbaiawa/g79client"
)

func main() {
	// 替换为你的 Cookie（与 example/main.go 相同来源）
	cookie := `{"sauth_json":"<REPLACE_WITH_YOUR_COOKIE>"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	// 认证
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	inviteCode := "5A04CC782B5"
	_ = inviteCode
	resp, _ := client.GetOtherDomainServers()
	for _, server := range resp.Entities {
		_, _ = client.DeleteOtherDomainServer(server.Sid)
	}
	inviteResp, _ := client.JoinDomainServerWithInviteCode(inviteCode)
	if inviteResp.Code != 0 {
		panic(fmt.Errorf("main: Entry failed: %s", inviteResp.Message))
	}
	serversResp, _ := client.GetOtherDomainServers()
	serverID := serversResp.Entities[0].Sid
	fmt.Println(serverID)
	enterResp, _ := client.RequestEnterDomainServer(serverID)
	address := fmt.Sprintf("%s:%d", enterResp.Entity.ServerHost, enterResp.Entity.ServerPort.Int64())
	fmt.Println(address)
	authv2, err := client.GenerateDomainGameAuthV2(serverID, "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW")
	fmt.Println(string(authv2))
	chainInfo, err := client.SendAuthV2Request(authv2)
	if err != nil {
		log.Fatalf("发送认证v2请求失败: %v", err)
	}
	fmt.Println(string(chainInfo))
}
