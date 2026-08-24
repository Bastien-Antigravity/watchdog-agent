module github.com/Bastien-Antigravity/watchdog-agent

go 1.25.8

require (
	github.com/Bastien-Antigravity/microservice-toolbox v0.0.1
	github.com/Bastien-Antigravity/safe-socket v0.0.1
	github.com/Bastien-Antigravity/universal-logger v0.0.1
)



replace (
	github.com/Bastien-Antigravity/microservice-toolbox => ../microservice-toolbox
	github.com/Bastien-Antigravity/safe-socket => ../safe-socket
	github.com/Bastien-Antigravity/universal-logger => ../universal-logger
)
