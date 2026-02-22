// Package prompts provides prompt templates for LLM interactions.
package prompts

import (
	"strings"
)

// PromptTemplate represents a string template that can be formatted with variables.
// Variables are in the format `{{.variable_name}}` and are replaced with values.
type PromptTemplate struct {
	// Template is the raw template string with placeholders.
	Template string
}

// NewPromptTemplate creates a new prompt template from the given string.
func NewPromptTemplate(template string) PromptTemplate {
	return PromptTemplate{Template: template}
}

// Format substitutes variables in the template string.
// Variables are in the format `{{.variable_name}}`.
func (p PromptTemplate) Format(vars map[string]string) string {
	prompt := p.Template
	for key, value := range vars {
		placeholder := "{{." + key + "}}"
		prompt = strings.ReplaceAll(prompt, placeholder, value)
	}
	return prompt
}