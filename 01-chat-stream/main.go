package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// MODEL_RUNNER_BASE_URL=http://localhost:12434 go run main.go
func main() {
	// Docker Model Runner Chat base URL
	llmURL := os.Getenv("MODEL_RUNNER_BASE_URL") + "/engines/llama.cpp/v1/"
	model := os.Getenv("MODEL_RUNNER_LLM_CHAT")

	client := openai.NewClient(
		option.WithBaseURL(llmURL),
		option.WithAPIKey(""),
	)

	ctx := context.Background()

	systemInstructions := `
	You are a Hawaiian pizza expert. Your name is Bob.
	Provide accurate, enthusiastic information about Hawaiian pizza's history 
	(invented in Canada in 1962 by Sam Panopoulos), 
	ingredients (ham, pineapple, cheese on tomato sauce), preparation methods, and cultural impact.
	Use a friendly tone with occasional pizza puns. 
	Defend pineapple on pizza good-naturedly while respecting differing opinions. 
	If asked about other pizzas, briefly answer but return focus to Hawaiian pizza. 
	Emphasize the sweet-savory flavor combination that makes Hawaiian pizza special.
	USE ONLY THE INFORMATION PROVIDED IN THE KNOWLEDGE BASE.	
	`

	knowledgeBase := `
	## Traditional Ingredients
	- Base: Traditional pizza dough
	- Sauce: Tomato-based pizza sauce
	- Cheese: Mozzarella cheese
	- Key toppings: Ham (or Canadian bacon) and pineapple
	- Optional additional toppings: Bacon, mushrooms, bell peppers, jalapeños

	## Regional Variations
	- Australia: "Hawaiian and bacon" adds extra bacon to the traditional recipe
	- Brazil: "Portuguesa com abacaxi" combines the traditional Portuguese pizza (with ham, onions, hard-boiled eggs, olives) with pineapple
	- Japan: Sometimes includes teriyaki chicken instead of ham
	- Germany: "Hawaii-Toast" is a related open-faced sandwich with ham, pineapple, and cheese
	- Sweden: "Flying Jacob" pizza includes banana, pineapple, curry powder, and chicken
	`

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemInstructions),
		openai.SystemMessage(knowledgeBase),
		openai.UserMessage("What is your name?"),
	}
	// openai.UserMessage("What are the ingredients of the hawaiian pizza?"),
	// openai.UserMessage("What is the best pizza in the world?"),

	param := openai.ChatCompletionNewParams{
		Messages:    messages,
		Model:       model,
		Temperature: openai.Opt(0.5),
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
