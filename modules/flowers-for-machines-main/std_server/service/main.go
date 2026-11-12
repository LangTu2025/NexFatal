package main

import (
	"flag"
	"log"

	service "github.com/OmineDev/flowers-for-machines/std_server/service/src"
)

var (
	rentalServerCode     *string
	rentalServerPasscode *string
	authServerAddress    *string
	authServerToken      *string
	standardServerPort   *int
	consoleDimensionID   *int
	consoleCenterX       *int
	consoleCenterY       *int
	consoleCenterZ       *int
	localLobbyOwnerHint  *string
)

func init() {
	rentalServerCode = flag.String("rsn", "", "Rental server number, 联机大厅:<room number or entity ID>, LobbyGame:<room number or entity ID>, 山头:<invite>, 本地联机:<room ID>, or TanLobby:<room ID>.")
	rentalServerPasscode = flag.String("rsp", "", "The passcode of the target server or lobby room.")
	authServerAddress = flag.String("asa", "", "The auth server address.")
	authServerToken = flag.String("ast", "", "The auth server token.")
	standardServerPort = flag.Int("ssp", 0, "The server port to running.")
	consoleDimensionID = flag.Int("cdi", 0, "The dimension ID of the console. (e.g. overworld = 0, nether = 1, end = 2, dmT = T, etc.)")
	consoleCenterX = flag.Int("ccx", 0, "The X position of the center of the console.")
	consoleCenterY = flag.Int("ccy", 0, "The Y position of the center of the console.")
	consoleCenterZ = flag.Int("ccz", 0, "The Z position of the center of the console.")
	localLobbyOwnerHint = flag.String("loh", "", "Optional owner name or UID for TanLobby authentication.")

	flag.Parse()
	if len(*rentalServerCode) == 0 {
		log.Fatalln("Please provide a target code.\n\te.g. -rsn=\"123456\", -rsn=\"联机大厅:7973\", -rsn=\"LobbyGame:ENTITY_ID\", -rsn=\"山头:INVITE_CODE\", -rsn=\"本地联机:ROOM_ID\", or -rsn=\"TanLobby:ROOM_ID\"")
	}
	if len(*authServerAddress) == 0 {
		log.Fatalln("Please provide your auth server address.\n\te.g. -asa=\"http://127.0.0.1\"")
	}
	if *standardServerPort == 0 {
		log.Fatalln("Please provide the server port to running.\n\te.g. -ssp=0")
	}
}

func main() {
	service.RunServer(
		*rentalServerCode,
		*rentalServerPasscode,
		*authServerAddress,
		*authServerToken,
		*standardServerPort,
		*consoleDimensionID,
		*consoleCenterX,
		*consoleCenterY,
		*consoleCenterZ,
		*localLobbyOwnerHint,
	)
}
