- Go to Docker Desktop
- Click on MCP Toolkit extension -> Google MAP
- Then -> Claude.AI Desktop


```text
Search for pizzerias in lyon
```




```json
{
	"mcpServers":{
		"sqlite":{
			"command":"docker",
			"args":["run","-i","--rm","mcp/sqlite"]
		},
		"MCP_DOCKER":{
			"command":"docker",
			"args":[
				"run",
				"-l",
				"mcp.client=claude-desktop",
				"--rm",
				"-i",
				"alpine/socat",
				"STDIO",
				"TCP:host.docker.internal:8811"
			]
		}
	},
	"globalShortcut":"Alt+C"
}
```