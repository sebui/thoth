package tools

import (
	"context"

	"google.golang.org/genai"
)

type Tool interface {
	Declaration() *genai.FunctionDeclaration
	Execute(ctx context.Context, args map[string]any) (map[string]any, error)
}
