- Go to Docker Desktop
- Click on Docker Hub -> Search MCP -> mcp/sqlite
- Then -> Claude.AI Desktop

```text
create a table friends with these fields first_name last_name
and add 5 friends (choose the names)
```

```text
display all the records of the friends table in markdown format
```



```json
{
	"mcpServers":{
		"sqlite":{
			"command":"docker",
			"args":["run","-i","--rm","mcp/sqlite"]
		}
	},
	"globalShortcut":"Alt+C"
}
```