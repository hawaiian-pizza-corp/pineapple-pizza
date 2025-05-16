package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/redis/go-redis/v9"
)

// MODEL_RUNNER_BASE_URL=http://localhost:12434 go run main.go
func main() {
	// Docker Model Runner Chat base URL
	llmURL := os.Getenv("MODEL_RUNNER_BASE_URL") + "/engines/llama.cpp/v1/"
	chatModel := os.Getenv("MODEL_RUNNER_LLM_CHAT")
	embeddingsModel := os.Getenv("MODEL_RUNNER_LLM_EMBEDDINGS")

	client := openai.NewClient(
		option.WithBaseURL(llmURL),
		option.WithAPIKey(""),
	)

	ctx := context.Background()

	// -------------------------------------------------
	// Make chunks from files
	// -------------------------------------------------
	contents, _ := GetContentFiles("/docs", ".md")
	chunks := []string{}
	for _, content := range contents {
		chunks = append(chunks, ChunkText(content, 512, 210)...)
	}

	// -------------------------------------------------
	// Generate embeddings from chunks
	// -------------------------------------------------
	rdb, _ := InitializeRedisAndIndex(ctx)
	log.Println("⏳ Creating embeddings from chunks...")

	for idx, chunk := range chunks {
		//! create the embedding
		embeddingsResponse, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
			Input: openai.EmbeddingNewParamsInputUnion{
				OfString: openai.String(chunk),
			},
			Model: embeddingsModel,
		})

		if err != nil {
			log.Println("😡 Error creating embedding:", err)
		}

		// convert the embedding to a []float32
		embedding := make([]float32, len(embeddingsResponse.Data[0].Embedding))
		for i, f := range embeddingsResponse.Data[0].Embedding {
			embedding[i] = float32(f)
		}
		buffer := floatsToBytes(embedding)

		//! store the embedding in Redis
		_, errIndex := rdb.HSet(ctx,
			fmt.Sprintf("doc:%v", idx),
			map[string]any{
				"content":   chunk,
				"embedding": buffer,
			},
		).Result()

		if errIndex != nil {
			log.Println("😡 Error storing embedding:", err)
		}
		//log.Println("✅ Embedding created for chunk:", chunk, embeddingsResponse.Data[0].Embedding)
	}

	// -------------------------------------------------
	// User question about 🍍🥓 Hawaiian pizza
	// -------------------------------------------------

	userQuestion := "Is Hawaiian pizza really from Hawaii?"
	//userQuestion := "Give me regional variations of Hawaiian pizza?"

	systemInstructions := `
	You are a Hawaiian pizza expert. Your name is Bob.
	Provide accurate, enthusiastic information about Hawaiian pizza, 
	Use a friendly tone with occasional pizza puns. 
	Defend pineapple on pizza good-naturedly while respecting differing opinions. 
	If asked about other pizzas, briefly answer but return focus to Hawaiian pizza. 
	Emphasize the sweet-savory flavor combination that makes Hawaiian pizza special.
	USE ONLY THE INFORMATION PROVIDED IN THE KNOWLEDGE BASE.	
	`

	// -------------------------------------------------
	// Generate embeddings from user question
	// -------------------------------------------------

	fmt.Println("⏳ Creating embeddings from user question...")

	embeddingsFromUserQuestion, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(userQuestion),
		},
		Model: embeddingsModel,
	})

	if err != nil {
		log.Fatalln("😡 Error creating embedding:", err)
	}

	fmt.Println("✋ embeddings from the user question:\n", embeddingsFromUserQuestion.Data[0].Embedding)

	// convert the embedding to a []float32
	embedding := make([]float32, len(embeddingsFromUserQuestion.Data[0].Embedding))
	for i, f := range embeddingsFromUserQuestion.Data[0].Embedding {
		embedding[i] = float32(f)
	}

	buffer := floatsToBytes(embedding)

	// Search for similar documents in Redis
	fmt.Println("⏳ Searching for similar documents in Redis...")
	results, err := rdb.FTSearchWithArgs(ctx,
		"vector_idx",
		"*=>[KNN 3 @embedding $vec AS vector_distance]",
		&redis.FTSearchOptions{
			Return: []redis.FTSearchReturn{
				{FieldName: "vector_distance"},
				{FieldName: "content"},
			},
			DialectVersion: 2,
			Params: map[string]any{
				"vec": buffer,
			},
		},
	).Result()

	if err != nil {
		log.Fatalln("😡 Error searching similarities:", err)
	}

	fmt.Println("🎉 Found", len(results.Docs), "similarities")

	knowledgeBase := ""

	for _, doc := range results.Docs {
		fmt.Println("📝 ID:", doc.ID, "Distance:", doc.Fields["vector_distance"])
		fmt.Println("📝 Content:\n", doc.Fields["content"])
		knowledgeBase += doc.Fields["content"]
	}
	/*
		The results are ordered according to the value of the vector_distance field,
		with the lowest distance indicating the greatest similarity to the query.
	*/

	// -------------------------------------------------
	// Ask the question to the LLM
	// -------------------------------------------------
	fmt.Println("--------------------------------------")
	fmt.Println("⏳ Asking the question to the LLM...")
	fmt.Println("--------------------------------------")

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemInstructions),
		openai.SystemMessage(knowledgeBase),
		openai.UserMessage(userQuestion),
	}

	param := openai.ChatCompletionNewParams{
		Messages:    messages,
		Model:       chatModel,
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
	fmt.Println("\n--------------------------------------")
	fmt.Println("🤖 Done!")
}

// ChunkText takes a text string and divides it into chunks of a specified size with a given overlap.
// It returns a slice of strings, where each string represents a chunk of the original text.
//
// Parameters:
//   - text: The input text to be chunked.
//   - chunkSize: The size of each chunk.
//   - overlap: The amount of overlap between consecutive chunks.
//
// Returns:
//   - []string: A slice of strings representing the chunks of the original text.
func ChunkText(text string, chunkSize, overlap int) []string {
	chunks := []string{}
	for start := 0; start < len(text); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[start:end])
	}
	return chunks
}

// GetContentFiles searches for files with a specific extension in the given directory and its subdirectories.
//
// Parameters:
// - dirPath: The directory path to start the search from.
// - ext: The file extension to search for.
//
// Returns:
// - []string: A slice of file paths that match the given extension.
// - error: An error if the search encounters any issues.
func GetContentFiles(dirPath string, ext string) ([]string, error) {
	content := []string{}
	_, err := ForEachFile(dirPath, ext, func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		content = append(content, string(data))
		return nil
	})
	if err != nil {
		return nil, err
	}

	return content, nil
}

// ForEachFile iterates over all files with a specific extension in a directory and its subdirectories.
//
// Parameters:
// - dirPath: The root directory to start the search from.
// - ext: The file extension to search for.
// - callback: A function to be called for each file found.
//
// Returns:
// - []string: A slice of file paths that match the given extension.
// - error: An error if the search encounters any issues.
func ForEachFile(dirPath string, ext string, callback func(string) error) ([]string, error) {
	var textFiles []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ext {
			textFiles = append(textFiles, path)
			err = callback(path)
			// generate an error to stop the walk
			if err != nil {
				return err
			}
		}
		return nil
	})
	return textFiles, err
}

func InitializeRedisAndIndex(ctx context.Context) (*redis.Client, error) {
	// connect to Redis and delete any index previously created with the name vector_idx:
	rdb := redis.NewClient(&redis.Options{
		//Addr:     "redis-server:6379",
		//Addr:     "0.0.0.0:6379",
		Addr:     "host.docker.internal:6379",
		Password: "", // no password docs
		DB:       0,  // use default DB
		Protocol: 2,
	})

	rdb.FTDropIndexWithArgs(ctx,
		"vector_idx",
		&redis.FTDropIndexOptions{
			DeleteDocs: true,
		},
	)
	/*
		Next, create the index.
		The schema in the example below specifies hash objects for storage and includes three fields:
		 - the text content to index,
		 - a tag field to represent the "genre" of the text,
		 - and the embedding vector generated from the original text content.
		The embedding field specifies HNSW indexing, the L2 vector distance metric, Float32 values to represent the vector's components,
		and 384 dimensions, as required by the all-MiniLM-L6-v2 embedding model.
	*/
	_, err := rdb.FTCreate(ctx,
		"vector_idx",
		&redis.FTCreateOptions{
			OnHash: true,
			Prefix: []any{"doc:"},
		},
		&redis.FieldSchema{
			FieldName: "content",
			FieldType: redis.SearchFieldTypeText,
		},
		//&redis.FieldSchema{
		//	FieldName: "genre",
		//	FieldType: redis.SearchFieldTypeTag,
		//},
		&redis.FieldSchema{
			FieldName: "embedding",
			FieldType: redis.SearchFieldTypeVector,
			VectorArgs: &redis.FTVectorArgs{
				HNSWOptions: &redis.FTHNSWOptions{
					Dim:            1024,
					DistanceMetric: "L2",
					Type:           "FLOAT32",
				},
			},
		},
	).Result()

	if err != nil {
		log.Println("😡 Error creating index:", err)
		return nil, err
	}
	return rdb, nil

}

func floatsToBytes(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)

	for i, f := range fs {
		u := math.Float32bits(f)
		binary.NativeEndian.PutUint32(buf[i*4:], u)
	}

	return buf
}
