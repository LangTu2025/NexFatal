package main

import (
	"fmt"
	"time"

	"github.com/xingbaiawa/StarShuttler/client"
	"github.com/xingbaiawa/StarShuttler/game_control/game_interface"
	"github.com/xingbaiawa/StarShuttler/game_control/resources_control"

	"github.com/pterm/pterm"
)

func SystemTestingLogin() {
	var err error
	tA := time.Now()

	cfg := client.Config{
		AuthServerAddress:    "https://nv1.aetheredge.top",
		AuthServerToken:      "nv1/Ja310BrYACXpFoDvb+m9rh09+SPbEzZg/xYC63XPU24rpZoodKEBsKtIZd/CiwEBw+pSJ6lEMdClH22RW2htodpfJx/5NQZATz2TYzjWK+ILMUddzD51uWMXmyfQuzyOusK2PqP9KZcpzUMVl94ubA==",
		RentalServerCode:     "服务器：2368747",
		RentalServerPasscode: "117259",
	}

	c, err = client.LoginRentalServer(cfg)
	if err != nil {
		panic(err)
	}

	resources = resources_control.NewResourcesControl(c)
	api, err = game_interface.NewGameInterface(resources, game_interface.DefaultMaintainer)
	if err != nil {
		panic(fmt.Sprintf("SystemTestingLogin: Failed on init game interface, and the err is %v", err))
	}

	pterm.Success.Printfln("SystemTestingLogin: PASS (Time used = %v)", time.Since(tA))
}
