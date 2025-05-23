package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func main() {
	ctx := context.Background()

	//! THE "Prompt"
	userQuestion := `
		Give me some pizzeria addresses in Lyon, France.
		Using the result, 
		- give me the name and adresses of the top 3 pizzerias in Lyon,
		- and imagine a quick presentation sentence for each pizzeria (with fancy emojis).
	`

	// Docker Model Runner Chat base URL
	llmURL := os.Getenv("MODEL_RUNNER_BASE_URL") + "/engines/llama.cpp/v1/"
	//! Use a model for the function calling
	//! and another one fot the chat completion
	modelTools := os.Getenv("MODEL_RUNNER_LLM_TOOLS")
	modelChat := os.Getenv("MODEL_RUNNER_LLM_CHAT")

	client := openai.NewClient(
		option.WithBaseURL(llmURL),
		option.WithAPIKey(""),
	)

	//! Start and setup the connection to MCP server process
	cmd := exec.Command(
		"docker",
		"run",
		"-i",
		"--rm",
		"alpine/socat",
		"STDIO",
		"TCP:host.docker.internal:8811",
	)
	// To run it in a container (with compose for example), the image needs to have docker installed

	stdin, stdout, err := setupCommand(cmd)
	if err != nil {
		log.Fatalf("😡 %v", err)
	}

	defer cmd.Process.Kill()

	clientTransport := stdio.NewStdioServerTransportWithIO(stdout, stdin)

	//! Create and initialize a new MCP client
	mcpClient := mcp_golang.NewClient(clientTransport)

	if _, err := mcpClient.Initialize(ctx); err != nil {
		log.Fatalf("😡 Failed to initialize client: %v", err)
	}

	//! Get the list of the available MCP tools
	//! Request: tools/list
	mcpTools, err := mcpClient.ListTools(ctx, nil)
	if err != nil {
		log.Fatalf("😡 Failed to list tools: %v", err)
	}

	filteredTools := []mcp_golang.ToolRetType{}
	for _, tool := range mcpTools.Tools {

		// If you want to use only the Brave API
		// You need a key (free for brave_web_search)
		// https://api-dashboard.search.brave.com/app/keys
		if tool.Name == "brave_web_search" {
			filteredTools = append(filteredTools, tool)
		}

		// If you want to use only the DuckDuckGo API
		/* 		if tool.Name == "search" {
		   			filteredTools = append(filteredTools, tool)
		   		}
		*/
		// If you want to use only the google API
		// You need a key (free)
		/*
			if tool.Name == "maps_search_places" {
				filteredTools = append(filteredTools, tool)
			}
		*/
	}

	//? Convert the mcp tools to OpenAI tools
	tools := ConvertToOpenAITools(filteredTools)

	//! Display the tools that are available on the MCP server
	fmt.Println("🛠️  Available Tools (OpenAI format) on the MCP server:")
	for _, tool := range tools {
		fmt.Println("🔧 Tool:", tool.Function.Name)
		fmt.Println("  - description:", tool.Function.Description)
		fmt.Println("  - parameters:", tool.Function.Parameters)
	}

	//! Setup of the "tools calling" request
	messagesTools := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(userQuestion),
	}

	params := openai.ChatCompletionNewParams{
		Messages:          messagesTools,
		ParallelToolCalls: openai.Bool(true),
		Tools:             tools,
		Seed:              openai.Int(0),
		Model:             modelTools,
		Temperature:       openai.Opt(0.0),
	}

	//? in "tools mode", the LLM will use only the parts of the prompt, related to an existing tool
	/*
		Give me some pizzeria addresses in Lyon, France.
		Using the result,
		- give me the name and adresses of the top 3 pizzerias in Lyon,
		- and imagine a quick presentation sentence for each pizzeria.
	*/

	//! Make completion request
	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		panic(err)
	}

	//! List of the detected tool calls by the LLM
	detectedToolCalls := completion.Choices[0].Message.ToolCalls

	// Return early if there are no tool calls
	if len(detectedToolCalls) == 0 {
		fmt.Println("😡 No function call")
		return
	}

	//! Display the tool calls to execute
	fmt.Println("🤖 Tool calls:", len(detectedToolCalls))

	for _, toolCall := range detectedToolCalls {
		fmt.Println(JSONPretty(toolCall))
	}

	//os.Exit(0)

	//! call the tools to create a list of pizzerias addresses
	addressesKnowledgeBase := "PIZZERIAS ADRESSES:\n"

	for _, toolCall := range detectedToolCalls {
		fmt.Println("📣 calling ", toolCall.Function.Name, toolCall.Function.Arguments)

		var args map[string]any
		err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		if err != nil {
			log.Println("😡 Failed to unmarshal arguments:", err)
		}
		fmt.Println("📝 Arguments:", args)

		// Call the tool with the arguments
		toolResponse, err := mcpClient.CallTool(ctx, toolCall.Function.Name, args)
		if err != nil {
			log.Println("😡 Failed to call tool:", err)
		}
		if toolResponse != nil && len(toolResponse.Content) > 0 && toolResponse.Content[0].TextContent != nil {
			fmt.Println("🎉📝 Tool response:", toolResponse.Content[0].TextContent.Text)
			addressesKnowledgeBase += toolResponse.Content[0].TextContent.Text
		}
	}

	//os.Exit(0)

	//! using the result with a chat completion
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a pizza expert."),
		openai.AssistantMessage(addressesKnowledgeBase),
		openai.UserMessage(userQuestion), //! <-- use the same prompt
	}

	param := openai.ChatCompletionNewParams{
		Messages:    messages,
		Model:       modelChat,
		Temperature: openai.Opt(1.2),
	}

	//! Make a streaming completion request
	stream := client.Chat.Completions.NewStreaming(ctx, param)

	for stream.Next() {
		chunk := stream.Current()
		// Stream each chunk as it arrives
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}

	if err := stream.Err(); err != nil {
		log.Fatalln("😡:", err)
	}

}

// MODEL_RUNNER_BASE_URL=http://localhost:12434  MODEL_RUNNER_LLM_TOOLS=ai/qwen2.5:1.5B-F16 go run main.go
// From a container
// MODEL_RUNNER_BASE_URL=http://model-runner.docker.internal MODEL_RUNNER_LLM_TOOLS=ai/qwen2.5:latest go run main.go

// -----------------------------------------
// my list of utility functions
// -----------------------------------------

func setupCommand(cmd *exec.Cmd) (stdin io.WriteCloser, stdout io.ReadCloser, err error) {
	stdin, err = cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start server: %w", err)
	}

	return stdin, stdout, nil
}

func JSONPretty(toolCall openai.ChatCompletionMessageToolCall) string {
	// how to pretty print a json string
	var prettyJSON bytes.Buffer
	_ = json.Indent(&prettyJSON, []byte(toolCall.RawJSON()), "", "\t")
	// and remove escape characters
	prettyJSONString := prettyJSON.String()
	prettyJSONString = string(bytes.ReplaceAll([]byte(prettyJSONString), []byte("\\\""), []byte("\"")))
	return prettyJSONString
}

func ConvertToOpenAITools(tools []mcp_golang.ToolRetType) []openai.ChatCompletionToolParam {
	openAITools := make([]openai.ChatCompletionToolParam, len(tools))

	for i, tool := range tools {
		schema := tool.InputSchema.(map[string]any)
		openAITools[i] = openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(*tool.Description),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": schema["properties"],
					"required":   schema["required"],
				},
			},
		}
	}
	return openAITools
}
