package main

import (
    "fmt"
    "log"
    "math/rand"
    "time"

    "github.com/xingbaiawa/g79client"
)

func main() {
    // 初始化随机数种子
    rand.Seed(time.Now().UnixNano())

    fmt.Println("启动 G79 客户端...")

    // Cookie字符串
    // cookie := `{"sauth_json":"<REPLACE_WITH_YOUR_COOKIE>"}`
    cookie := `{"sauth_json":"{\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"Asia/Shanghai\\\",\\\"celluar_ip\\\":\\\"\\\",\\\"operator\\\":\\\"\\\",\\\"is_vpn_enabled\\\":false}\",\"app_channel\":\"netease\",\"client_login_sn\":\"EB59AE11382CC31B9FFFE80D3EF1871B\",\"deviceid\":\"amawvzaac7h7vubb-d\",\"gameid\":\"x19\",\"gas_token\":\"\",\"get_access_token\":\"1\",\"ip\":\"127.0.0.1\",\"is_unisdk_guest\":1,\"login_channel\":\"netease\",\"platform\":\"ad\",\"sdk_version\":\"5.16.0\",\"sdkuid\":\"aibgvzfkq6p244l5\",\"sessionid\":\"1-eyJzaSI6ICI1ZjZlMGM1OWU5ZWEwODQ4MmRmOWEyYjQ0ZDY5MmU5NjAyMzk0MzM5IiwgIm9kaSI6ICJhbWF3dnphYWM3aDd2dWJiLWQiLCAicyI6ICJzamxvbnY5YmkzbWxuaTZ0cXkyOWJoM2RzczRieTAyMiIsICJ1IjogImFpYmd2emZrcTZwMjQ0bDUiLCAidCI6IDIsICJwcnMiOiAxMjgsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCIsICJzYSI6IC05OX0g\",\"source_app_channel\":\"netease\",\"source_platform\":\"ad\",\"udid\":\"28oy76r262obxhmlvor0hftl18fwwrp3\"}"}`

    client, err := g79client.NewClient()
    if err != nil {
        log.Fatalf("创建客户端失败: %v", err)
    }

    // 执行认证
    fmt.Println("开始认证...")
    err = client.G79AuthenticateWithCookie(cookie)
    if err != nil {
        log.Fatalf("认证失败: %v", err)
    }
    fmt.Printf("认证成功，用户ID: %s\n", client.UserID)

    // 获取用户详情
    fmt.Println("获取用户信息...")
    username := client.UserDetail.Name
    if username == "" {
        // 生成随机用户名
        name := fmt.Sprintf("NE_%06d", rand.Intn(1000000))
        err = client.UpdateNickname(name)
        if err != nil {
            log.Fatalf("修改名字失败: %v", err)
        }
        username = name
    }

    growthLevel := client.UserDetail.Level
    fmt.Printf("用户名: %s\n", username)
    fmt.Printf("等级: %v\n", growthLevel)

    // 搜索租赁服
    fmt.Println("搜索租赁服...")
    searchResp, err := client.SearchRentalServerByName("48285363", 0)
    if err != nil {
        log.Fatalf("搜索租赁服失败: %v", err)
    }

    if searchResp.Code != 0 || len(searchResp.Entities) == 0 {
        log.Fatalf("获取租赁服信息失败")
    }

    serverID := searchResp.Entities[0].EntityID

    // 进入租赁服世界
    fmt.Println("进入租赁服...")
    enterResp, err := client.EnterRentalServerWorld(serverID.String(), "123456")
    if err != nil {
        log.Fatalf("进入租赁服失败: %v", err)
    }

    if enterResp.Code != 0 {
        log.Fatalf("获取租赁服地址失败: %s", enterResp.Message)
    }

    ipAddress := fmt.Sprintf("%s:%v", enterResp.Entity.McserverHost, enterResp.Entity.McserverPort)
    fmt.Printf("服务器地址: %s\n", ipAddress)

    // 生成认证v2数据
    fmt.Println("生成认证信息...")
    authv2Data, err := client.GenerateRentalGameAuthV2(serverID.String(), "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW")
    if err != nil {
        log.Fatalf("生成认证v2数据失败: %v", err)
    }

    // 发送认证v2请求
    chainInfo, err := client.SendAuthV2Request(authv2Data)
    if err != nil {
        log.Fatalf("发送认证v2请求失败: %v", err)
    }

    fmt.Printf("认证链信息: %s\n", string(chainInfo))
    fmt.Println("运行完成!")
}