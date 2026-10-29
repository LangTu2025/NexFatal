module github.com/LangTu2025/NexFatal

go 1.26.0

require (
	github.com/TriM-Organization/bedrock-world-operator v1.6.0
	github.com/mattn/go-runewidth v0.0.16
	github.com/mholt/archiver/v3 v3.5.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pterm/pterm v0.12.82
	github.com/schollz/progressbar/v3 v3.18.0
	github.com/xingbaiawa/LycorisFlow v0.0.0
	github.com/LangTu2025/NexFatal/utils/safe_http v0.0.0
	github.com/xingbaiawa/StarShuttler v0.0.0
	github.com/xingbaiawa/blocks v0.0.0-20260321042356-9e0912b68e66
	github.com/xingbaiawa/g79client v0.0.0-00010101000000-000000000000
	go.uber.org/ratelimit v0.3.1
	golang.org/x/term v0.42.0
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/Happy2018new/worldupgrader v1.3.0 // indirect
	github.com/TriM-Organization/merry-memory v0.3.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/bongnv/go-container v0.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/containerd/console v1.0.5 // indirect
	github.com/database64128/chacha8-go v0.0.0-20250815115417-e0f2726d8bd0 // indirect
	github.com/deatil/go-cryptobin v1.1.1005 // indirect
	github.com/df-mc/goleveldb v1.1.9 // indirect
	github.com/df-mc/jsonc v1.0.5 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/go-gl/mathgl v1.2.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.5 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/nwaples/rardecode v1.1.0 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.17.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.2 // indirect
	github.com/pion/dtls/v3 v3.0.11 // indirect
	github.com/pion/ice/v4 v4.1.0 // indirect
	github.com/pion/interceptor v0.1.42 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/mdns/v2 v2.1.0 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.16 // indirect
	github.com/pion/rtp v1.8.26 // indirect
	github.com/pion/sdp/v3 v3.0.16 // indirect
	github.com/pion/srtp/v3 v3.0.9 // indirect
	github.com/pion/stun/v3 v3.0.2 // indirect
	github.com/pion/transport/v3 v3.1.1 // indirect
	github.com/pion/transport/v4 v4.0.1 // indirect
	github.com/pion/turn/v4 v4.1.3 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sandertv/gophertunnel v1.51.1-0.20251123190717-e4ddb111c9d1 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	github.com/ulikunitz/xz v0.5.9 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xingbaiawa/bdump v0.0.0-00010101000000-000000000000 // indirect
	github.com/xingbaiawa/unmcpk v0.0.0-00010101000000-000000000000 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace github.com/xingbaiawa/LycorisFlow => ./modules/LycorisFlow

replace github.com/xingbaiawa/blocks => ./modules/LycorisFlow/modules/blocks

replace github.com/xingbaiawa/bdump => ./modules/LycorisFlow/modules/bdump

replace github.com/xingbaiawa/g79client => ./modules/g79client

replace github.com/xingbaiawa/unmcpk => ./modules/unmcpk

replace github.com/Happy2018new/worldupgrader => ./modules/worldupgrader

replace github.com/TriM-Organization/bedrock-world-operator => ./modules/bedrock-world-operator

replace github.com/xingbaiawa/StarShuttler => ./modules/StarShuttler

replace github.com/LangTu2025/NexFatal/utils/safe_http => ./utils/safe_http
