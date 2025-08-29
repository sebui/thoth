package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/genai"
)

type ReadManyFilesTool struct {
	ProjectRoot string
}

func (t *ReadManyFilesTool) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{
		Name:        "read_many_files",
		Description: "Reads content from multiple files specified by paths or glob patterns within a configured target directory. For text files, it concatenates their content into a single string, separated by '--- {filePath} ---'. Binary files (images, PDFs, etc.) will be listed by their path with a note that their content is not included. Glob patterns like 'src/**/*.js' are supported. Paths are relative to the project root. Avoid using for single files if a more specific single-file reading tool is available, unless the user specifically requests to process a list containing just one file via this tool.",
		Parameters: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"paths": {
					Type:        genai.TypeArray,
					Description: "Required. An array of glob patterns or paths relative to the tool's target directory. Examples: ['src/**/*.ts'], ['README.md', 'docs/']",
					Items:       &genai.Schema{Type: genai.TypeString},
				},
				"exclude": {
					Type:        genai.TypeArray,
					Description: "Optional. Glob patterns for files/directories to exclude. Added to default excludes if useDefaultExcludes is true. Example: \"**/*.log\", \"temp/\"",
					Items:       &genai.Schema{Type: genai.TypeString},
				},
				"include": {
					Type:        genai.TypeArray,
					Description: "Optional. Additional glob patterns to include. These are merged with `paths`. Example: \"*.test.ts\" to specifically add test files if they were broadly excluded.",
					Items:       &genai.Schema{Type: genai.TypeString},
				},
				"recursive": {
					Type:        genai.TypeBoolean,
					Description: "Optional. Whether to search recursively (primarily controlled by `**` in glob patterns). Defaults to true.",
				},
				"useDefaultExcludes": {
					Type:        genai.TypeBoolean,
					Description: "Optional. Whether to apply a list of default exclusion patterns (e.g., node_modules, .git, binary files). Defaults to true.",
				},
				"file_filtering_options": {
					Type:        genai.TypeObject,
					Description: "Whether to respect ignore patterns from .gitignore or .geminiignore (not fully implemented in this version)",
					Properties: map[string]*genai.Schema{
						"respect_gemini_ignore": {
							Type:        genai.TypeBoolean,
							Description: "Optional: Whether to respect .geminiignore patterns when listing files. Defaults to true.",
						},
						"respect_git_ignore": {
							Type:        genai.TypeBoolean,
							Description: "Optional: Whether to respect .gitignore patterns when listing files. Only available in git repositories. Defaults to true.",
						},
					},
				},
			},
			Required: []string{"paths"},
		},
	}
}

func (t *ReadManyFilesTool) Execute(ctx context.Context, args map[string]any) (map[string]any, error) {
	paths, ok := args["paths"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'paths' argument")
	}

	var filePaths []string
	for _, p := range paths {
		pattern, ok := p.(string)
		if !ok {
			return nil, fmt.Errorf("invalid path pattern: %v", p)
		}

		absPattern := filepath.Join(t.ProjectRoot, pattern)

		if strings.ContainsAny(pattern, "*?[]") {
			matches, err := filepath.Glob(absPattern)
			if err != nil {
				return nil, fmt.Errorf("error globbing pattern %s: %w", pattern, err)
			}
			filePaths = append(filePaths, matches...)
		} else {
			filePaths = append(filePaths, absPattern)
		}
	}

	seen := make(map[string]bool)
	uniqueFilePaths := []string{}
	for _, p := range filePaths {
		if !seen[p] {
			seen[p] = true

			uniqueFilePaths = append(uniqueFilePaths, p)
		}
	}

	var contentBuilder strings.Builder
	for _, filePath := range uniqueFilePaths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				contentBuilder.WriteString(fmt.Sprintf("---%s (Not Found)---\n", filePath))
			} else {
				contentBuilder.WriteString(fmt.Sprintf("---%s (Error: %v)---\n", filePath, err))
			}
			continue
		}

		if fileInfo.IsDir() {
			contentBuilder.WriteString(fmt.Sprintf("---%s (Directory)---\n", filePath))
			continue
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			contentBuilder.WriteString(fmt.Sprintf("---%s (Error reading: %v)---\n", filePath, err))
			continue
		}

		if bytes.ContainsRune(data, 0) {
			contentBuilder.WriteString(fmt.Sprintf("---%s (Binary File, content not included)---\n", filePath))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("---%s --- %s\n", filePath, string(data)))
		}
	}

	return map[string]any{"content": contentBuilder.String()}, nil
}
