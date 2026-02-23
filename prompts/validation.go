package prompts

// DefaultValidationPrompt is a prompt template for validating context relevance.
// It asks the model to determine if the provided context is helpful for answering
// the user's question. The expected response is "yes" or "no".
var DefaultValidationPrompt = NewPromptTemplate(
	`You are an expert at evaluating whether a given context can help answer a user's question.

Context:
---
{{.context}}
---

Question: {{.query}}

Does the context contain information that is likely to be helpful in answering the question?
Answer only with "yes" or "no".

Answer:`)
