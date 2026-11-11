go env -w GOOS=windows
go build -ldflags "-s -w" -trimpath -o NexFatal_windows_amd64.exe
go env -w GOOS=linux
set CGO_ENABLED=0
go build -ldflags "-s -w" -trimpath -o NexFatal_linux_amd64
go env -w GOOS=windows