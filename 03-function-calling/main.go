package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// MODEL_RUNNER_BASE_URL=http://localhost:12434 go run main.go
func main() {
	// Docker Model Runner Chat base URL
	llmURL := os.Getenv("MODEL_RUNNER_BASE_URL") + "/engines/llama.cpp/v1/"
	modelTools := os.Getenv("MODEL_RUNNER_LLM_TOOLS")

	client := openai.NewClient(
		option.WithBaseURL(llmURL),
		option.WithAPIKey(""),
	)

	ctx := context.Background()

	pizzeriaAddresses := openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        "pizzeria_addresses",
			Description: openai.String("Give pizzeria addresses in a given city"),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"city"},
			},
		},
	}

	sayHelloTool := openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        "say_hello",
			Description: openai.String("Say hello to the given person with their first and last name"),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"firstName": map[string]string{
						"type": "string",
					},
					"lastName": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"firstName", "lastName"},
			},
		},
	}

	tools := []openai.ChatCompletionToolParam{
		pizzeriaAddresses,
		sayHelloTool,
	}

	userQuestion := openai.UserMessage(`
		Give me some pizzeria addresses in Lyon, France.
		Say Hello to Bob Morane.
		Give me some pizzeria addresses in Tokyo, Japan.
	`)

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			userQuestion,
		},
		ParallelToolCalls: openai.Bool(true),
		Tools:             tools,
		Model:             modelTools,
		Temperature:       openai.Opt(0.0),
	}

	// Make completion request
	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		panic(err)
	}

	toolCalls := completion.Choices[0].Message.ToolCalls

	// Return early if there are no tool calls
	if len(toolCalls) == 0 {
		fmt.Println("😡 No function call")
		return
	}

	fmt.Println("🤖 Tool calls:", len(toolCalls))
	//Display the first tool call

	for _, toolCall := range toolCalls {
		fmt.Println(JSONPretty(toolCall))
	}

	os.Exit(0)
	// make the function calls
	// and display the results
	for _, toolCall := range toolCalls {
		fmt.Println("--------------------------------------------")
		fmt.Println("🤖 Function call:", toolCall.Function.Name)
		fmt.Println("--------------------------------------------")
		var args map[string]any

		switch toolCall.Function.Name {
		case "say_hello":
			args, _ = JsonStringToMap(toolCall.Function.Arguments)
			fmt.Println(sayHello(args))

		case "pizzeria_addresses":
			args, _ = JsonStringToMap(toolCall.Function.Arguments)
			fmt.Println(pizerriaAddresses(args))

		default:
			fmt.Println("Unknown function call:", toolCall.Function.Name)
		}
	}

}

func sayHello(arguments map[string]interface{}) string {

	firstName, ok1 := arguments["firstName"].(string)
	lastName, ok2 := arguments["lastName"].(string)
	if !ok1 || !ok2 {
		return "Invalid arguments for say_hello function"
	}
	return fmt.Sprintf("👋 Hello %s %s! 🙂", firstName, lastName)

}

func pizerriaAddresses(arguments map[string]interface{}) string {
	city, ok := arguments["city"].(string)
	if !ok {
		return "Invalid arguments for pizzeria_addresses function"
	}

	addresses := map[string][]string{
		"Lyon": {
			"123 Pizza St, Lyon, France",
			"456 Pizzeria Ave, Lyon, France",
			"789 Italian Bistro, Lyon, France",
		},
		"Tokyo": {
			"123 Sushi St, Tokyo, Japan",
			"456 Ramen Ave, Tokyo, Japan",
			"789 Izakaya Bistro, Tokyo, Japan",
		},
	}
	if addresses, ok := addresses[city]; ok {
		return fmt.Sprintf("🍕 Pizzerias in %s: \n1. %s\n2. %s\n3. %s", city, addresses[0], addresses[1], addresses[2])
	}
	// If city not found, return a default message
	// You can also return an empty list or a message indicating no pizzerias found
	// depending on your requirements.
	// For now, let's return a default message
	return fmt.Sprintf("🍕 No Pizzerias in %s:", city)

}

func JsonStringToMap(jsonString string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		return nil, err
	}
	return result, nil
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
