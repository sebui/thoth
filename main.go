package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"thoth/tools"

	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	toolRegistry := make(map[string]tools.Tool)

	var functionDeclarations []*genai.FunctionDeclaration
	for _, tool := range toolRegistry {
		functionDeclarations = append(functionDeclarations, tool.Declaration())
	}

	var genaiTools []*genai.Tool
	if len(functionDeclarations) > 0 {
		genaiTools = []*genai.Tool{{
			FunctionDeclarations: functionDeclarations,
		}}
	}

	chat, err := client.Chats.Create(ctx, "gemini-1.5-flash", &genai.GenerateContentConfig{Tools: genaiTools}, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Enter your messages (type 'quit' to exit):")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := scanner.Text()
		if input == "quit" {
			break
		}

		resp, err := chat.SendMessage(ctx, genai.Part{Text: input})
		if err != nil {
			log.Printf("Error sending message: %v", err)
			continue
		}

	toolCallLoop:
		for {
			if resp == nil || len(resp.Candidates) == 0 {
				fmt.Println("No response candidates")
				break toolCallLoop
			}

			var toolResponses []genai.Part
			var modelTextResponse string

			for _, cand := range resp.Candidates {
				if cand.Content == nil {
					continue
				}
				for _, part := range cand.Content.Parts {
					if part.FunctionCall != nil {
						call := part.FunctionCall
						log.Printf("Model called tool: %s with args: %v", call.Name, call.Args)

						var toolResult map[string]any
						var toolErr error

						if tool, ok := toolRegistry[call.Name]; ok {
							toolResult, toolErr = tool.Execute(ctx, call.Args)
						} else {
							toolErr = fmt.Errorf("unknown tool: %s", call.Name)
						}

						if toolErr != nil {
							log.Printf("Error executing tool %s: %v", call.Name, toolErr)
							toolResponses = append(toolResponses, genai.Part{
								FunctionResponse: &genai.FunctionResponse{
									Name: call.Name,
									Response: map[string]any{
										"error": toolErr.Error(),
									},
								},
							})
						} else {
							toolResponses = append(toolResponses, genai.Part{
								FunctionResponse: &genai.FunctionResponse{
									Name:     call.Name,
									Response: toolResult,
								},
							})
						}
					} else if part.Text != "" {
						modelTextResponse += part.Text
					}
				}
			}

			if len(toolResponses) > 0 {
				var sendErr error
				resp, sendErr = chat.SendMessage(ctx, toolResponses...)
				if sendErr != nil {
					log.Printf("Error sending tool responses: %v", sendErr)
					break toolCallLoop
				}
			} else {
				if modelTextResponse != "" {
					fmt.Printf("Gemini: %s\n", modelTextResponse)
				}
				break toolCallLoop
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading input: %v", err)
	}
}
