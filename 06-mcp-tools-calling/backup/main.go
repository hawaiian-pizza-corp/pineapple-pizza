package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// MODEL_RUNNER_BASE_URL=http://localhost:12434  MODEL_RUNNER_LLM_TOOLS=ai/qwen2.5:1.5B-F16 go run main.go
// MODEL_RUNNER_BASE_URL=http://model-runner.docker.internal MODEL_RUNNER_LLM_TOOLS=ai/qwen2.5:latest go run main.go
func main() {
	ctx := context.Background()

	// Docker Model Runner Chat base URL
	llmURL := os.Getenv("MODEL_RUNNER_BASE_URL") + "/engines/llama.cpp/v1/"
	modelTools := os.Getenv("MODEL_RUNNER_LLM_TOOLS")
	modelChat := os.Getenv("MODEL_RUNNER_LLM_CHAT")

	fmt.Println("🤖 Model Runner URL:", llmURL)
	fmt.Println("🤖 Model:", modelTools)

	client := openai.NewClient(
		option.WithBaseURL(llmURL),
		option.WithAPIKey(""),
	)

	// Start the MCP server process
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

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("😡 Failed to get stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("😡 Failed to get stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("😡 Failed to start server: %v", err)
	}
	defer cmd.Process.Kill()

	clientTransport := stdio.NewStdioServerTransportWithIO(stdout, stdin)

	// Create a new MCP client
	mcpClient := mcp_golang.NewClient(clientTransport)

	if _, err := mcpClient.Initialize(ctx); err != nil {
		log.Fatalf("😡 Failed to initialize client: %v", err)
	}

	// Get the list of the available MCP tools
	mcpTools, err := mcpClient.ListTools(ctx, nil)
	if err != nil {
		log.Fatalf("😡 Failed to list tools: %v", err)
	}

	// Convert the mcp tools to openai tools
	tools := ConvertToOpenAITools(mcpTools)

	fmt.Println("🛠️  Available Tools (OpenAI format):")
	for _, tool := range tools {
		fmt.Println("🔧 Tool:", tool.Function.Name)
		fmt.Println("  - description:", tool.Function.Description)
		fmt.Println("  - parameters:", tool.Function.Parameters)
	}

	//! questions to trigger the tools detection
	userQuestion := `
		Give me some pizzeria addresses in Lyon, France.
	`

	// Create a list of messagesTools for the chat completion request
	messagesTools := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(userQuestion),
	}

	params := openai.ChatCompletionNewParams{
		Messages:          messagesTools,
		ParallelToolCalls: openai.Bool(true),
		Tools:             tools,
		Seed:              openai.Int(0),

		Model:       modelTools,
		Temperature: openai.Opt(0.0),
	}

	// Make completion request
	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		panic(err)
	}

	detectedToolCalls := completion.Choices[0].Message.ToolCalls

	// Return early if there are no tool calls
	if len(detectedToolCalls) == 0 {
		fmt.Println("😡 No function call")
		return
	}

	fmt.Println("🤖 Tool calls:", len(detectedToolCalls))
	//Display the first tool call

	for _, toolCall := range detectedToolCalls {
		fmt.Println(JSONPretty(toolCall))
	}

	//os.Exit(0)
	// make the function calls
	// and display the results

	//! call the tools to create a list of pizzerias addresses
	addressesKnowledgeBase := "PIZZERIAS ADRESSES:\n"

	for _, toolCall := range detectedToolCalls {
		fmt.Println("📣 calling ", toolCall.Function.Name, toolCall.Function.Arguments)

		// toolCall.Function.Arguments is a JSON String
		// Convert the JSON string to a (map[string]any)
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
		openai.SystemMessage(addressesKnowledgeBase),
		openai.UserMessage(`
			Using the PIZZERIAS ADRESSES, 
			give me the name and adresses of the top 3 pizzerias in Lyon,
			and imagine a quick presentation sentence for each pizzeria.
		`),
	}

	param := openai.ChatCompletionNewParams{
		Messages:    messages,
		Model:       modelChat,
		Temperature: openai.Opt(1.2),
	}

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

func JSONPretty(toolCall openai.ChatCompletionMessageToolCall) string {
	// how to pretty print a json string
	var prettyJSON bytes.Buffer
	_ = json.Indent(&prettyJSON, []byte(toolCall.RawJSON()), "", "\t")
	// and remove escape characters
	prettyJSONString := prettyJSON.String()
	prettyJSONString = string(bytes.ReplaceAll([]byte(prettyJSONString), []byte("\\\""), []byte("\"")))
	return prettyJSONString
}

func ConvertToOpenAITools(tools *mcp_golang.ToolsResponse) []openai.ChatCompletionToolParam {
	openAITools := make([]openai.ChatCompletionToolParam, len(tools.Tools))

	for i, tool := range tools.Tools {
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
