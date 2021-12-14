module github.com/asticode/go-astiencoder

go 1.13

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/asticode/go-astikit v0.23.0
	github.com/asticode/go-astiws v1.5.0
	github.com/asticode/goav v1.4.0
	github.com/gorilla/websocket v1.4.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/shirou/gopsutil/v3 v3.21.10
	github.com/stretchr/testify v1.7.0
)

//replace github.com/asticode/go-astikit v0.23.0 => ../go-astikit
