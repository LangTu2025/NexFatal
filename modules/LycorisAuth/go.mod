module github.com/xingbaiawa/LycorisAuth

go 1.26.0

require (
	github.com/xingbaiawa/g79client v0.0.0-00010101000000-000000000000
	github.com/xingbaiawa/unmcpk v0.0.0-00010101000000-000000000000
)

require (
	github.com/database64128/chacha8-go v0.0.0-20250815115417-e0f2726d8bd0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)

replace github.com/xingbaiawa/g79client => ./modules/g79client

replace github.com/xingbaiawa/unmcpk => ./modules/unmcpk
